// server.go owns the Server struct and its lifecycle (listen/close), the
// model-default helper, and the idle watchdog. it does NOT own the JSON-RPC wire
// types (types.go), the connection read loop (conn.go), or dispatch (dispatch.go).

package ipc

import (
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
	quit     chan struct{}
	quitOnce sync.Once
	verbose  bool

	// defaultModel is the thinker-tier model (config.json's models.thinker) used
	// for chat/stream/compact when a session has no model explicitly assigned.
	defaultModel string

	// idleTimeout is how long the daemon may sit with no client activity before
	// shutting itself down; zero disables the watchdog. lastActive holds the unix
	// nano of the most recent RPC and is read by monitorIdle.
	idleTimeout time.Duration
	lastActive  atomic.Int64
}

func NewServer(socket string, store *session.Store, sched *scheduler.Scheduler, mcpHost *mcp.Host, auditor *audit.Auditor, p provider.Provider, verbose bool, idleTimeout time.Duration, defaultModel string) (*Server, error) {
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

		defaultModel: defaultModel,
		idleTimeout:  idleTimeout,
	}
	s.touch()     // seed the idle clock so the daemon does not shut down before its first call
	go s.accept() // accept loop runs in background; NewServer returns immediately
	if idleTimeout > 0 {
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
