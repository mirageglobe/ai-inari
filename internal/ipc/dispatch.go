package ipc

import (
	"github.com/mirageglobe/ai-inari/internal/session"
)

// defaultNewAgentModel is attached to every session.create call so a new
// agent can chat immediately without a manual /model select.
const defaultNewAgentModel = "gemma4:e2b"

// toInfo converts a session to the wire summary sent to inari.
// ContextChars sums all message content (including system prompt) so inari can
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
		CWD:          sess.CWD,
		ContextChars: ctxChars,
	}
}

// dispatch routes a JSON-RPC request to its handler. session.* handlers live in
// dispatch_session.go; ollama.* and daemon.quit handlers live in dispatch_ollama.go.
func (s *Server) dispatch(req Request) Response {
	switch req.Method {
	case "ping":
		return Response{JSONRPC: "2.0", Result: "pong", ID: req.ID}
	case "session.list":
		return s.handleSessionList(req)
	case "session.create":
		return s.handleSessionCreate(req)
	case "session.delete":
		return s.handleSessionDelete(req)
	case "session.unassign":
		return s.handleSessionUnassign(req)
	case "session.assign":
		return s.handleSessionAssign(req)
	case "session.setcontext":
		return s.handleSessionSetContext(req)
	case "session.history":
		return s.handleSessionHistory(req)
	case "session.clear":
		return s.handleSessionClear(req)
	case "session.compact":
		return s.handleSessionCompact(req)
	case "session.recap":
		return s.handleSessionRecap(req)
	case "session.chat":
		return s.handleSessionChat(req)
	case "session.interrupt":
		return s.handleSessionInterrupt(req)
	case "ollama.load":
		return s.handleOllamaLoad(req)
	case "ollama.unload":
		return s.handleOllamaUnload(req)
	case "ollama.delete":
		return s.handleOllamaDelete(req)
	case "ollama.running":
		return s.handleOllamaRunning(req)
	case "ollama.models":
		return s.handleOllamaModels(req)
	case "ollama.show":
		return s.handleOllamaShow(req)
	case "ollama.context":
		return s.handleOllamaContext(req)
	case "daemon.quit":
		return s.handleDaemonQuit(req)
	default:
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32601, Message: "method not found"}, ID: req.ID}
	}
}
