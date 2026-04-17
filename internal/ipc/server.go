package ipc

import (
	"encoding/json"
	"net"
	"os"

	"github.com/mirageglobe/ai-inari/internal/audit"
	"github.com/mirageglobe/ai-inari/internal/mcp"
	"github.com/mirageglobe/ai-inari/internal/ollama"
	"github.com/mirageglobe/ai-inari/internal/scheduler"
	"github.com/mirageglobe/ai-inari/internal/session"
)

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      any             `json:"id"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string `json:"jsonrpc"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
	ID      any    `json:"id"`
}

// Error is a JSON-RPC 2.0 error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Server listens on a Unix Domain Socket and dispatches JSON-RPC calls.
type Server struct {
	listener net.Listener
	store    *session.Store
	sched    *scheduler.Scheduler
	mcpHost  *mcp.Host
	auditor  *audit.Auditor
	ollama   *ollama.Client
	quit     chan struct{}
}

func NewServer(socket string, store *session.Store, sched *scheduler.Scheduler, mcpHost *mcp.Host, auditor *audit.Auditor, ollamaClient *ollama.Client) (*Server, error) {
	os.Remove(socket)

	l, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socket, 0600); err != nil {
		l.Close()
		return nil, err
	}

	s := &Server{
		listener: l,
		store:    store,
		sched:    sched,
		mcpHost:  mcpHost,
		auditor:  auditor,
		ollama:   ollamaClient,
		quit:     make(chan struct{}),
	}
	go s.accept()
	return s, nil
}

// Quit returns a channel that is closed when a daemon.quit RPC is received.
func (s *Server) Quit() <-chan struct{} {
	return s.quit
}

// ollamaErr returns an "ollama not configured" error response when s.ollama is nil.
// ok is false when the caller should return the response immediately.
func (s *Server) ollamaErr(req Request) (Response, bool) {
	if s.ollama == nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: "ollama not configured"}, ID: req.ID}, false
	}
	return Response{}, true
}

func (s *Server) accept() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	for {
		var req Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		s.auditor.Log(req.Method, req.Params)
		resp := s.dispatch(req)
		enc.Encode(resp)
	}
}

func (s *Server) dispatch(req Request) Response {
	switch req.Method {
	case "ping":
		return Response{JSONRPC: "2.0", Result: "pong", ID: req.ID}
	case "session.list":
		return Response{JSONRPC: "2.0", Result: s.store.List(), ID: req.ID}
	case "session.chat":
		if r, ok := s.ollamaErr(req); !ok {
			return r
		}
		var params struct {
			Model    string           `json:"model"`
			Messages []ollama.Message `json:"messages"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		reply, err := s.ollama.Chat(params.Model, params.Messages)
		if err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		return Response{JSONRPC: "2.0", Result: reply, ID: req.ID}
	case "ollama.load":
		if r, ok := s.ollamaErr(req); !ok {
			return r
		}
		var params struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		if err := s.ollama.LoadModel(params.Model); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		return Response{JSONRPC: "2.0", Result: "loaded", ID: req.ID}
	case "ollama.unload":
		if r, ok := s.ollamaErr(req); !ok {
			return r
		}
		var params struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		if err := s.ollama.UnloadModel(params.Model); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		return Response{JSONRPC: "2.0", Result: "unloaded", ID: req.ID}
	case "ollama.running":
		if r, ok := s.ollamaErr(req); !ok {
			return r
		}
		running, err := s.ollama.ListRunning()
		if err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		return Response{JSONRPC: "2.0", Result: running, ID: req.ID}
	case "ollama.models":
		if r, ok := s.ollamaErr(req); !ok {
			return r
		}
		models, err := s.ollama.ListModels()
		if err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		return Response{JSONRPC: "2.0", Result: models, ID: req.ID}
	case "daemon.quit":
		// Signal main to shut down; close is idempotent via sync.Once pattern.
		select {
		case <-s.quit:
		default:
			close(s.quit)
		}
		return Response{JSONRPC: "2.0", Result: "shutting down", ID: req.ID}
	default:
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32601, Message: "method not found"}, ID: req.ID}
	}
}

func (s *Server) Close() {
	s.listener.Close()
}
