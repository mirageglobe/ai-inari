// dispatch_session.go owns the session lifecycle/config handlers: list, create,
// delete, assign/unassign, and setcontext. it does NOT own conversation ops
// (dispatch_chat.go), ollama.* (dispatch_ollama.go), or the switch (dispatch.go).

package ipc

import (
	"encoding/json"
	"time"

	"github.com/mirageglobe/ai-inari/internal/session"
)

// session.list returns a summary of every session; no message history on the wire.
func (s *Server) handleSessionList(req Request) Response {
	list := s.store.List()
	infos := make([]SessionInfo, len(list))
	for i, sess := range list {
		infos[i] = toInfo(sess)
	}
	return Response{JSONRPC: "2.0", Result: infos, ID: req.ID}
}

// session.create initialises a new named session, pre-assigned to
// defaultNewAgentModel so it can chat immediately.
// if cwd is provided, a shallow file tree is injected into the system prompt so
// the model is aware of the project layout without reading any file content.
func (s *Server) handleSessionCreate(req Request) Response {
	var params struct {
		Name string `json:"name"`
		CWD  string `json:"cwd,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
	}
	sess := session.New(params.Name)
	sess.Model = defaultNewAgentModel
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
			"- execute_shell_command(command, args): run an allowlisted command in the working directory; permitted commands: " + sortedAllowedCommands() + "\n" +
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
}

// session.delete removes a session and its full chat history.
func (s *Server) handleSessionDelete(req Request) Response {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
	}
	s.store.Remove(params.ID)
	return Response{JSONRPC: "2.0", Result: "ok", ID: req.ID}
}

// session.unassign detaches the current model from a session.
// the session and its full chat history are preserved; a new model can be assigned later.
func (s *Server) handleSessionUnassign(req Request) Response {
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
}

// session.assign attaches a model to an existing session.
// chat history from any prior model is preserved and will be sent as context.
func (s *Server) handleSessionAssign(req Request) Response {
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
	// a model-name mismatch (e.g. missing tag) reaches Ollama as an opaque
	// error later in the chat loop; check against the backend's own list now
	// so the failure surfaces immediately, at assign time, with a clear cause.
	models, err := s.provider.ListModels()
	if err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32000, Message: "could not verify model: " + err.Error()}, ID: req.ID}
	}
	found := false
	for _, m := range models {
		if m.Name == params.Model {
			found = true
			break
		}
	}
	if !found {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "model " + params.Model + " not found; pull it first"}, ID: req.ID}
	}
	sess.Model = params.Model
	sess.UpdatedAt = time.Now()
	s.store.Persist(sess.ID)
	return Response{JSONRPC: "2.0", Result: "ok", ID: req.ID}
}

// session.setcontext sets the system prompt for a session.
// the prompt is stored as Messages[0] (role:"system") so it is sent to Ollama
// exactly once per conversation. send an empty string to clear it.
func (s *Server) handleSessionSetContext(req Request) Response {
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
}
