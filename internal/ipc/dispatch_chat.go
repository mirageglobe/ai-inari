// dispatch_chat.go owns the conversation-oriented session handlers: history,
// clear, compact, and chat. it does NOT own session CRUD/config
// (dispatch_session.go), ollama.* (dispatch_ollama.go), or the switch (dispatch.go).

package ipc

import (
	"time"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// recapIdleThreshold is how long a session must sit with no new messages before
// session.recap returns a "where you left off" summary instead of an empty string.
const recapIdleThreshold = 10 * time.Minute

// session.interrupt aborts a session's in-flight stream. it returns
// {"interrupted": true} when a stream was cancelled, false when none was active.
// the stream connection itself finalises the partial reply and signals done.
func (s *Server) handleSessionInterrupt(req Request) Response {
	var params struct {
		ID string `json:"id"`
	}
	if r := decodeParams(req, &params); r != nil {
		return *r
	}
	if params.ID == "" {
		return badParams(req, "invalid params")
	}
	interrupted := s.interruptStream(params.ID)
	return Response{JSONRPC: "2.0", Result: map[string]bool{"interrupted": interrupted}, ID: req.ID}
}

// session.history returns the full message history for a session.
// inari calls this when opening a session to restore the display from inarid's store.
func (s *Server) handleSessionHistory(req Request) Response {
	var params struct {
		ID string `json:"id"`
	}
	if r := decodeParams(req, &params); r != nil {
		return *r
	}
	sess, r := s.getSession(req, params.ID)
	if r != nil {
		return *r
	}
	// filter system messages; inari display shows only user/assistant turns.
	all := sess.ChatHistory()
	visible := all[:0:len(all)]
	for _, m := range all {
		if m.Role != "system" {
			visible = append(visible, m)
		}
	}
	return Response{JSONRPC: "2.0", Result: visible, ID: req.ID}
}

// session.clear removes all user/assistant messages, retaining the system prompt.
func (s *Server) handleSessionClear(req Request) Response {
	var params struct {
		ID string `json:"id"`
	}
	if r := decodeParams(req, &params); r != nil {
		return *r
	}
	sess, r := s.getSession(req, params.ID)
	if r != nil {
		return *r
	}
	sess.ClearHistory()
	s.store.Persist(params.ID)
	return Response{JSONRPC: "2.0", Result: true, ID: req.ID}
}

// session.compact summarises the conversation with the assigned model, then replaces
// all user/assistant messages with the summary. the system prompt is preserved.
func (s *Server) handleSessionCompact(req Request) Response {
	if r, ok := s.providerErr(req); !ok {
		return r
	}
	var params struct {
		ID string `json:"id"`
	}
	if r := decodeParams(req, &params); r != nil {
		return *r
	}
	sess, r := s.getSession(req, params.ID)
	if r != nil {
		return *r
	}
	model := s.modelFor(sess)
	if model == "" {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "no model assigned to session"}, ID: req.ID}
	}
	compactPrompt := append(sess.ChatHistory(), provider.Message{
		Role:    "user",
		Content: "write a detailed summary of this conversation that preserves enough context for the conversation to continue naturally. include: the main topic and goal, all questions asked and answers given, any code or commands discussed, decisions made and their rationale, current state and what was left to do. use bullet points grouped by topic. do not omit technical details.",
	})
	summary, err := s.provider.Chat(model, compactPrompt)
	if err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
	}
	sess.ReplaceWithSummary(summary)
	s.store.Persist(params.ID)
	return Response{JSONRPC: "2.0", Result: summary, ID: req.ID}
}

// session.recap returns a one-sentence "where you left off" summary for a session
// that has gone idle, generated with the assigned model. it is non-destructive:
// the history is left untouched (unlike session.compact). returns an empty string
// when the session is not idle long enough, has no real conversation, or has no
// model, so the client simply shows nothing.
func (s *Server) handleSessionRecap(req Request) Response {
	if r, ok := s.providerErr(req); !ok {
		return r
	}
	var params struct {
		ID string `json:"id"`
	}
	if r := decodeParams(req, &params); r != nil {
		return *r
	}
	sess, r := s.getSession(req, params.ID)
	if r != nil {
		return *r
	}
	// gate: only recap a session that has gone quiet and has an exchange to recap.
	convo := 0
	for _, m := range sess.ChatHistory() {
		if m.Role == "user" || m.Role == "assistant" {
			convo++
		}
	}
	model := s.modelFor(sess)
	if time.Since(sess.UpdatedAt) < recapIdleThreshold || convo < 2 || model == "" {
		return Response{JSONRPC: "2.0", Result: "", ID: req.ID}
	}
	recapPrompt := append(sess.ChatHistory(), provider.Message{
		Role:    "user",
		Content: "in one short sentence, recap where this conversation left off so i can pick it back up. plain text, no preamble.",
	})
	recap, err := s.provider.Chat(model, recapPrompt)
	if err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
	}
	return Response{JSONRPC: "2.0", Result: recap, ID: req.ID}
}

// session.chat appends a user message, sends the full history to Ollama,
// stores the reply, and returns the assistant's text.
func (s *Server) handleSessionChat(req Request) Response {
	if r, ok := s.providerErr(req); !ok {
		return r
	}
	var params struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	if r := decodeParams(req, &params); r != nil {
		return *r
	}
	sess, r := s.getSession(req, params.ID)
	if r != nil {
		return *r
	}
	model := s.modelFor(sess)
	if model == "" {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "no model assigned to session"}, ID: req.ID}
	}
	sess.AppendMessage(provider.Message{Role: "user", Content: params.Text})
	reply, err := s.provider.Chat(model, sess.ChatHistory())
	if err != nil {
		// roll back the user message so the history stays consistent on retry.
		sess.Messages = sess.Messages[:len(sess.Messages)-1]
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
	}
	sess.AppendMessage(provider.Message{Role: "assistant", Content: reply})
	s.store.Persist(sess.ID)
	return Response{JSONRPC: "2.0", Result: reply, ID: req.ID}
}
