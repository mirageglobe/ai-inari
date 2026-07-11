// dispatch_chat.go owns the conversation-oriented session handlers: history,
// clear, compact, and chat. it does NOT own session CRUD/config
// (dispatch_session.go), ollama.* (dispatch_ollama.go), or the switch (dispatch.go).

package ipc

import (
	"encoding/json"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// session.history returns the full message history for a session.
// inari calls this when opening a session to restore the display from inarid's store.
func (s *Server) handleSessionHistory(req Request) Response {
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
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
	}
	sess, ok := s.store.Get(params.ID)
	if !ok {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
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
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
	}
	sess, ok := s.store.Get(params.ID)
	if !ok {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
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
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
	}
	sess, ok := s.store.Get(params.ID)
	if !ok {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
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
