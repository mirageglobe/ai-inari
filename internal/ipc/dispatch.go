package ipc

import (
	"encoding/json"
	"time"

	"github.com/mirageglobe/ai-inari/internal/provider"
	"github.com/mirageglobe/ai-inari/internal/session"
)

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

func (s *Server) dispatch(req Request) Response {
	switch req.Method {
	case "ping":
		return Response{JSONRPC: "2.0", Result: "pong", ID: req.ID}

	// session.list returns a summary of every session; no message history on the wire.
	case "session.list":
		list := s.store.List()
		infos := make([]SessionInfo, len(list))
		for i, sess := range list {
			infos[i] = toInfo(sess)
		}
		return Response{JSONRPC: "2.0", Result: infos, ID: req.ID}

	// session.create initialises a new named session with no model assigned yet.
	// if cwd is provided, a shallow file tree is injected into the system prompt so
	// the model is aware of the project layout without reading any file content.
	case "session.create":
		var params struct {
			Name string `json:"name"`
			CWD  string `json:"cwd,omitempty"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		sess := session.New(params.Name)
		if params.CWD != "" {
			sess.CWD = params.CWD
			tree := buildFileTree(params.CWD, 3)
			// omit "respond in plain text only" from the default prompt because it conflicts
			// with structured function calling and causes the model to output tool invocations
			// as text rather than structured tool_calls.
			combined := "keep all responses concise and short." +
				"\n\nworking directory: " + params.CWD + "\n" + tree +
				"\n\nyou have access to the following tools to explore the working directory:\n" +
				"- read_file(path): read the full text of a file\n" +
				"- list_dir(path): list files and directories inside a path\n" +
				"- grep_file(path, pattern): search for a regex pattern across files, returns matching lines\n" +
				"- stat_file(path): return size, modification time, and type for a file or directory\n" +
				"- run(command, args): run an allowlisted command in the working directory; permitted commands: " + sortedAllowedCommands() + "\n" +
				"use these tools whenever the user asks about files, code, or the project structure."
			// inject a project-level context file (AGENTS.md / .inari/context.md) so the
			// model picks up local conventions without manual copy-paste; absent file is fine.
			if ctx := readAgentContext(params.CWD); ctx != "" {
				combined += "\n\nproject context:\n" + ctx
			}
			sess.SetSystemPrompt(combined)
		}
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
	// the session and its full chat history are preserved; a new model can be assigned later.
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
	// chat history from any prior model is preserved and will be sent as context.
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
	// inari calls this when opening a session to restore the display from inarid's store.
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
		// filter system messages; inari display shows only user/assistant turns.
		all := sess.ChatHistory()
		visible := all[:0:len(all)]
		for _, m := range all {
			if m.Role != "system" {
				visible = append(visible, m)
			}
		}
		return Response{JSONRPC: "2.0", Result: visible, ID: req.ID}

	// session.clear removes all user/assistant messages, retaining the system prompt.
	case "session.clear":
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

	// session.compact summarises the conversation with the assigned model, then replaces
	// all user/assistant messages with the summary. the system prompt is preserved.
	case "session.compact":
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
		if sess.Model == "" {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "no model assigned to session"}, ID: req.ID}
		}
		compactPrompt := append(sess.ChatHistory(), provider.Message{
			Role:    "user",
			Content: "write a detailed summary of this conversation that preserves enough context for the conversation to continue naturally. include: the main topic and goal, all questions asked and answers given, any code or commands discussed, decisions made and their rationale, current state and what was left to do. use bullet points grouped by topic. do not omit technical details.",
		})
		summary, err := s.provider.Chat(sess.Model, compactPrompt)
		if err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		sess.ReplaceWithSummary(summary)
		s.store.Persist(params.ID)
		return Response{JSONRPC: "2.0", Result: summary, ID: req.ID}

	// session.chat appends a user message, sends the full history to Ollama,
	// stores the reply, and returns the assistant's text.
	case "session.chat":
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
		if sess.Model == "" {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "no model assigned to session"}, ID: req.ID}
		}
		sess.AppendMessage(provider.Message{Role: "user", Content: params.Text})
		reply, err := s.provider.Chat(sess.Model, sess.ChatHistory())
		if err != nil {
			// roll back the user message so the history stays consistent on retry.
			sess.Messages = sess.Messages[:len(sess.Messages)-1]
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		sess.AppendMessage(provider.Message{Role: "assistant", Content: reply})
		s.store.Persist(sess.ID)
		return Response{JSONRPC: "2.0", Result: reply, ID: req.ID}

	case "ollama.load":
		if r, ok := s.providerErr(req); !ok {
			return r
		}
		var params struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		if err := s.provider.LoadModel(params.Model); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		return Response{JSONRPC: "2.0", Result: "loaded", ID: req.ID}
	case "ollama.unload":
		if r, ok := s.providerErr(req); !ok {
			return r
		}
		var params struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		if err := s.provider.UnloadModel(params.Model); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		return Response{JSONRPC: "2.0", Result: "unloaded", ID: req.ID}
	case "ollama.running":
		if r, ok := s.providerErr(req); !ok {
			return r
		}
		running, err := s.provider.ListRunning()
		if err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		return Response{JSONRPC: "2.0", Result: running, ID: req.ID}
	case "ollama.models":
		if r, ok := s.providerErr(req); !ok {
			return r
		}
		models, err := s.provider.ListModels()
		if err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		return Response{JSONRPC: "2.0", Result: models, ID: req.ID}
	case "ollama.show":
		if r, ok := s.providerErr(req); !ok {
			return r
		}
		var params struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
		}
		caps, err := s.provider.ModelCaps(params.Model)
		if err != nil {
			return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
		}
		return Response{JSONRPC: "2.0", Result: caps, ID: req.ID}
	case "daemon.quit":
		s.closeQuit() // idempotent; also used by the idle watchdog
		return Response{JSONRPC: "2.0", Result: "shutting down", ID: req.ID}
	default:
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32601, Message: "method not found"}, ID: req.ID}
	}
}
