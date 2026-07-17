// server.go owns the Server struct and its lifecycle (listen/close), the
// model-default helper, and the idle watchdog. it does NOT own the JSON-RPC wire
// types (types.go), the connection read loop (conn.go), or dispatch (dispatch.go).

package ipc

import (
	"context"
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

// Server listens on a Unix Domain Socket and dispatches JSON-RPC calls.
type Server struct {
	listener net.Listener
	store    *session.Store
	sched    *scheduler.Scheduler
	mcpHost  *mcp.Host
	auditor  *audit.Auditor
	provider provider.Provider

	// ctxLen memoises ModelContextLength per model name. the context window is a
	// static property of a model, so the per-turn /api/show round-trip it replaces
	// is pure redundant first-token latency. process-lifetime cache keyed by model
	// name; a re-pull under the same name would keep a stale value, an accepted
	// tradeoff for taking a blocking call off every stream.
	ctxLen sync.Map

	quit     chan struct{}
	quitOnce sync.Once
	verbose  bool

	// streams maps a session ID to the cancel func of its in-flight stream, so a
	// session.interrupt RPC can abort a long generation. guarded by streamsMu.
	streamsMu sync.Mutex
	streams   map[string]context.CancelFunc

	// defaultModel is the thinker-tier model (config.json's models.thinker) used
	// for chat/stream/compact when a session has no model explicitly assigned.
	defaultModel string

	// globalSystemPrompt (config.json's context.system_prompt) is prepended to
	// every new session's system prompt; empty means no global prompt.
	globalSystemPrompt string

	// idleTimeout is how long the daemon may sit with no client activity before
	// shutting itself down; zero disables the watchdog. lastActive holds the unix
	// nano of the most recent RPC and is read by monitorIdle.
	idleTimeout time.Duration
	lastActive  atomic.Int64
}

// ServerConfig carries the daemon's collaborators and tunables. zero values are
// valid: a nil Provider degrades to "provider not configured", a zero IdleTimeout
// disables the watchdog, and empty DefaultModel/GlobalSystemPrompt mean "none".
// keyed literals at the call site keep the (formerly 10-positional) args readable
// and make adjacent same-typed fields impossible to transpose.
type ServerConfig struct {
	Socket             string
	Store              *session.Store
	Scheduler          *scheduler.Scheduler
	MCPHost            *mcp.Host
	Auditor            *audit.Auditor
	Provider           provider.Provider
	Verbose            bool
	IdleTimeout        time.Duration
	DefaultModel       string
	GlobalSystemPrompt string
}

func NewServer(cfg ServerConfig) (*Server, error) {
	// remove stale socket left by a previous unclean shutdown; Listen fails if the file exists.
	os.Remove(cfg.Socket)

	l, err := net.Listen("unix", cfg.Socket)
	if err != nil {
		return nil, err
	}
	// restrict to the owning user; the socket carries unencrypted prompts and session data.
	if err := os.Chmod(cfg.Socket, 0600); err != nil {
		l.Close()
		return nil, err
	}

	s := &Server{
		listener: l,
		store:    cfg.Store,
		sched:    cfg.Scheduler,
		mcpHost:  cfg.MCPHost,
		auditor:  cfg.Auditor,
		provider: cfg.Provider,
		quit:     make(chan struct{}),
		verbose:  cfg.Verbose,
		streams:  make(map[string]context.CancelFunc),

		defaultModel:       cfg.DefaultModel,
		globalSystemPrompt: cfg.GlobalSystemPrompt,
		idleTimeout:        cfg.IdleTimeout,
	}
	s.touch()     // seed the idle clock so the daemon does not shut down before its first call
	go s.accept() // accept loop runs in background; NewServer returns immediately
	if cfg.IdleTimeout > 0 {
		go s.monitorIdle()
	}
	return s, nil
}

// modelFor returns the model to use for a session: its own assignment if set,
// otherwise the configured thinker-tier default (empty if none is configured).
func (s *Server) modelFor(sess *session.Session) string {
	if sess.Model != "" {
		return sess.Model
	}
	return s.defaultModel
}

// registerStream records the cancel func for a session's in-flight stream so a
// session.interrupt RPC can abort it. any prior stream for the same session is
// cancelled first (a session streams one turn at a time).
func (s *Server) registerStream(id string, cancel context.CancelFunc) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	if prev, ok := s.streams[id]; ok {
		prev()
	}
	s.streams[id] = cancel
}

// unregisterStream drops a session's stream cancel func once the stream ends.
func (s *Server) unregisterStream(id string) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	delete(s.streams, id)
}

// interruptStream cancels a session's in-flight stream if one is active;
// reports whether a stream was found to cancel.
func (s *Server) interruptStream(id string) bool {
	s.streamsMu.Lock()
	cancel, ok := s.streams[id]
	s.streamsMu.Unlock()
	if ok {
		cancel()
	}
	return ok
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

func (s *Server) Close() {
	s.listener.Close()
}
