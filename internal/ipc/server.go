package ipc

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mirageglobe/ai-inari/internal/audit"
	"github.com/mirageglobe/ai-inari/internal/mcp"
	"github.com/mirageglobe/ai-inari/internal/provider"
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
	provider provider.Provider
	quit     chan struct{}
	quitOnce sync.Once
	verbose  bool

	// idleTimeout is how long the daemon may sit with no client activity before
	// shutting itself down; zero disables the watchdog. lastActive holds the unix
	// nano of the most recent RPC and is read by monitorIdle.
	idleTimeout time.Duration
	lastActive  atomic.Int64
}

func NewServer(socket string, store *session.Store, sched *scheduler.Scheduler, mcpHost *mcp.Host, auditor *audit.Auditor, p provider.Provider, verbose bool, idleTimeout time.Duration) (*Server, error) {
	// remove stale socket left by a previous unclean shutdown; Listen fails if the file exists.
	os.Remove(socket)

	l, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	// restrict to the owning user; the socket carries unencrypted prompts and session data.
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
		provider: p,
		quit:     make(chan struct{}),
		verbose:  verbose,

		idleTimeout: idleTimeout,
	}
	s.touch()     // seed the idle clock so the daemon does not shut down before its first call
	go s.accept() // accept loop runs in background; NewServer returns immediately
	if idleTimeout > 0 {
		go s.monitorIdle()
	}
	return s, nil
}

// touch records the time of the latest client activity for the idle watchdog.
func (s *Server) touch() {
	s.lastActive.Store(time.Now().UnixNano())
}

// closeQuit signals main to shut down. it is safe to call from multiple goroutines;
// the daemon.quit RPC and the idle watchdog both use it.
func (s *Server) closeQuit() {
	s.quitOnce.Do(func() { close(s.quit) })
}

// monitorIdle shuts the daemon down once no client activity has been seen for
// idleTimeout. it checks on a coarse ticker rather than a precise timer because the
// shutdown deadline does not need second-level accuracy.
func (s *Server) monitorIdle() {
	// check at a fraction of the timeout so the actual shutdown lands within ~1/4
	// of the configured window, capped at one minute for short timeouts.
	interval := s.idleTimeout / 4
	if interval > time.Minute {
		interval = time.Minute
	}
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-ticker.C:
			last := time.Unix(0, s.lastActive.Load())
			if time.Since(last) >= s.idleTimeout {
				log.Printf("idle for %s; auto-shutting down", s.idleTimeout)
				s.closeQuit()
				return
			}
		}
	}
}

// Quit returns a channel that is closed when a daemon.quit RPC is received.
func (s *Server) Quit() <-chan struct{} {
	return s.quit
}

// providerErr returns a "provider not configured" error response when s.provider is nil.
// ok is false when the caller should return the response immediately.
func (s *Server) providerErr(req Request) (Response, bool) {
	if s.provider == nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: "provider not configured"}, ID: req.ID}, false
	}
	return Response{}, true
}

// accept runs until the listener is closed (on shutdown). each connection gets its own goroutine
// so a slow inari call (e.g. a long Ollama reply) doesn't block other clients.
func (s *Server) accept() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

// handle reads JSON-RPC requests from conn in a loop. the connection stays open across multiple
// calls so inari can reuse it without re-dialing for every operation.
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
		s.touch() // any RPC (including ping heartbeats) resets the idle watchdog
		s.auditor.Log(req.Method, req.Params)

		if req.Method == "session.stream" {
			if s.verbose {
				log.Printf("[inariui->inarid] session.stream %s", req.Params)
			}
			s.handleStream(conn, dec, req)
			return
		}

		// suppress ping; fires on every heartbeat and adds no signal.
		if req.Method != "ping" && s.verbose {
			log.Printf("[inariui->inarid] %s %s", req.Method, req.Params)
		}
		resp := s.dispatch(req)
		if req.Method != "ping" && s.verbose {
			if resp.Error != nil {
				log.Printf("[inarid->inariui] %s error: %s", req.Method, resp.Error.Message)
			} else {
				log.Printf("[inarid->inariui] %s ok", req.Method)
			}
		}
		enc.Encode(resp)
	}
}

func (s *Server) Close() {
	s.listener.Close()
}
