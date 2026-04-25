package ipc

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"strings"
	"time"

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
	verbose  bool
}

func NewServer(socket string, store *session.Store, sched *scheduler.Scheduler, mcpHost *mcp.Host, auditor *audit.Auditor, ollamaClient *ollama.Client, verbose bool) (*Server, error) {
	// Remove stale socket left by a previous unclean shutdown; Listen fails if the file exists.
	os.Remove(socket)

	l, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	// Restrict to the owning user — the socket carries unencrypted prompts and session data.
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
		verbose:  verbose,
	}
	go s.accept() // accept loop runs in background; NewServer returns immediately
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

// accept runs until the listener is closed (on shutdown). Each connection gets its own goroutine
// so a slow fox call (e.g. a long Ollama reply) doesn't block other clients.
func (s *Server) accept() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

// handle reads JSON-RPC requests from conn in a loop. The connection stays open across multiple
// calls so kitsune can reuse it without re-dialing for every operation.
// session.stream is handled specially: it takes over the connection for the duration of the
// stream and closes it when done, so the loop exits after one streaming call.
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

		if req.Method == "session.stream" {
			if s.verbose {
				log.Printf("rpc → session.stream %s", req.Params)
			}
			s.handleStream(conn, req)
			return
		}

		if s.verbose {
			log.Printf("rpc → %s %s", req.Method, req.Params)
		}
		resp := s.dispatch(req)
		if s.verbose {
			if resp.Error != nil {
				log.Printf("rpc ← %s error: %s", req.Method, resp.Error.Message)
			} else {
				log.Printf("rpc ← %s ok", req.Method)
			}
		}
		enc.Encode(resp)
	}
}

// handleStream serves a session.stream request over a dedicated connection.
// it streams token chunks as newline-delimited JSON frames, persists the full reply on done,
// and rolls back the user message on error. the connection is closed by the caller (handle).
func (s *Server) handleStream(conn net.Conn, req Request) {
	enc := json.NewEncoder(conn)

	var params struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		enc.Encode(map[string]string{"error": "invalid params"})
		return
	}

	sess, ok := s.store.Get(params.ID)
	if !ok {
		enc.Encode(map[string]string{"error": "session not found"})
		return
	}
	if sess.Model == "" {
		enc.Encode(map[string]string{"error": "no model assigned to session"})
		return
	}

	sess.AppendMessage(ollama.Message{Role: "user", Content: params.Text})

	chunks := make(chan ollama.ChatResponse, 32)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ollama.ChatStream(ollama.ChatRequest{
			Model:    sess.Model,
			Messages: sess.ChatHistory(),
			Stream:   true,
		}, chunks)
		close(chunks)
	}()

	var fullReply strings.Builder
	for chunk := range chunks {
		if chunk.Message.Content != "" {
			fullReply.WriteString(chunk.Message.Content)
			enc.Encode(map[string]string{"token": chunk.Message.Content})
		}
	}

	if err := <-errCh; err != nil {
		sess.Messages = sess.Messages[:len(sess.Messages)-1]
		enc.Encode(map[string]string{"error": err.Error()})
		if s.verbose {
			log.Printf("rpc ← session.stream error: %v", err)
		}
		return
	}

	sess.AppendMessage(ollama.Message{Role: "assistant", Content: fullReply.String()})
	s.store.Persist(sess.ID)
	enc.Encode(map[string]bool{"done": true})
	if s.verbose {
		log.Printf("rpc ← session.stream ok (%d chars)", fullReply.Len())
	}
}

// toInfo converts a session to the wire summary sent to fox.
// ContextChars sums all message content (including system prompt) so fox can
// display an estimated token count without fetching the full history.
func toInfo(sess *session.Session) SessionInfo {
	history := sess.ChatHistory()
	var ctxChars int
	for _, m := range history {
		ctxChars += len(m.Content)
	}
	return SessionInfo{
		ID:           sess.ID,
		Name:         sess.Name,
		Model:        sess.Model,
		SystemPrompt: sess.SystemPrompt,
		ContextChars: ctxChars,
	}
}

func (s *Server) dispatch(req Request) Response {
	switch req.Method {
	case "ping":
		return Response{JSONRPC: "2.0", Result: "pong", ID: req.ID}

	// session.list returns a summary of every session — no message history on the wire.
	case "session.list":
		list := s.store.List()
		infos := make([]SessionInfo, len(list))
		for i, sess := range list {
			infos[i] = toInfo(sess)
		}
		return Response{JSONRPC: "2.0", Result: infos, ID: req.ID}

	// session.create initialises a new named session with no model assigned yet.
	case "session.create":
		var params struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		sess := session.New(params.Name)
		s.store.Add(sess)
		return Response{JSONRPC: "2.0", Result: toInfo(sess), ID: req.ID}

	// session.delete removes a session and its full chat history.
	case "session.delete":
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		s.store.Remove(params.ID)
		return Response{JSONRPC: "2.0", Result: "ok", ID: req.ID}

	// session.unassign detaches the current model from a session.
	// The session and its full chat history are preserved — a new model can be
	// assigned at any time and will continue the same conversation.
	case "session.unassign":
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		sess, ok := s.store.Get(params.ID)
		if !ok {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
		}
		sess.Model = ""
		sess.UpdatedAt = time.Now()
		s.store.Persist(sess.ID)
		return Response{JSONRPC: "2.0", Result: "ok", ID: req.ID}

	// session.assign attaches a model to an existing session.
	// Chat history from any prior model is preserved and will be sent as context.
	case "session.assign":
		var params struct {
			ID    string `json:"id"`
			Model string `json:"model"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		sess, ok := s.store.Get(params.ID)
		if !ok {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
		}
		sess.Model = params.Model
		sess.UpdatedAt = time.Now()
		s.store.Persist(sess.ID)
		return Response{JSONRPC: "2.0", Result: "ok", ID: req.ID}

	// session.setcontext sets the system prompt for a session.
	// the prompt is stored as Messages[0] (role:"system") so it is sent to Ollama
	// exactly once per conversation. send an empty string to clear it.
	case "session.setcontext":
		var params struct {
			ID     string `json:"id"`
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		sess, ok := s.store.Get(params.ID)
		if !ok {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
		}
		sess.SetSystemPrompt(params.Prompt)
		s.store.Persist(sess.ID)
		return Response{JSONRPC: "2.0", Result: "ok", ID: req.ID}

	// session.history returns the full message history for a session.
	// fox calls this when opening a session to restore the display from inarid's store.
	case "session.history":
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		sess, ok := s.store.Get(params.ID)
		if !ok {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
		}
		// filter system messages — fox display shows only user/assistant turns.
		all := sess.ChatHistory()
		visible := all[:0:len(all)]
		for _, m := range all {
			if m.Role != "system" {
				visible = append(visible, m)
			}
		}
		return Response{JSONRPC: "2.0", Result: visible, ID: req.ID}

	// session.chat appends a user message, sends the full history to Ollama,
	// stores the reply, and returns the assistant's text. History is never sent
	// over the wire — fox sends only the new user text each turn.
	case "session.chat":
		if r, ok := s.ollamaErr(req); !ok {
			return r
		}
		var params struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		sess, ok := s.store.Get(params.ID)
		if !ok {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
		}
		if sess.Model == "" {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "no model assigned to session"}, ID: req.ID}
		}
		sess.AppendMessage(ollama.Message{Role: "user", Content: params.Text})
		reply, err := s.ollama.Chat(sess.Model, sess.ChatHistory())
		if err != nil {
			// Roll back the user message so the history stays consistent on retry.
			sess.Messages = sess.Messages[:len(sess.Messages)-1]
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		sess.AppendMessage(ollama.Message{Role: "assistant", Content: reply})
		s.store.Persist(sess.ID)
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
