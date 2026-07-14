// dispatch_session.go owns the session lifecycle/config handlers: list, create,
// delete, rename, assign/unassign, setcontext, setcwd, tag, setrole, and
// setnumctx. it does NOT own conversation ops (dispatch_chat.go), ollama.*
// (dispatch_ollama.go), or the switch (dispatch.go).

package ipc

import (
	"encoding/json"
	"os"
	"time"

	"github.com/mirageglobe/ai-inari/internal/config"
	"github.com/mirageglobe/ai-inari/internal/provider"
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
	// base prompt: the cwd file-tree/AGENTS.md context when a cwd is given, else
	// the session's default concise-response prompt set by session.New.
	base := sess.SystemPrompt
	if params.CWD != "" {
		sess.CWD = params.CWD
		base = buildCWDSystemPrompt(params.CWD)
	}
	// prepend the effective context prompt: a project overlay's context.system_prompt
	// (.inari/config.json in the session cwd) replaces the global one when set, since
	// the more-specific project prompt wins; otherwise the global prompt applies. the
	// base cwd/tree/AGENTS.md context is retained either way.
	prompt := s.globalSystemPrompt
	if params.CWD != "" {
		if proj := config.LoadProject(params.CWD); proj.Context.SystemPrompt != "" {
			prompt = proj.Context.SystemPrompt
		}
	}
	if prompt != "" {
		base = prompt + "\n\n" + base
	}
	sess.SetSystemPrompt(base)
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

// session.rename changes a session's display name in place, preserving its model,
// history, and cwd. mirrors setcwd/setcontext: validate, mutate, persist, and
// return the updated info so any open view can refresh its label.
func (s *Server) handleSessionRename(req Request) Response {
	var params struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "invalid params: name required"}, ID: req.ID}
	}
	sess, ok := s.store.Get(params.ID)
	if !ok {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
	}
	sess.Name = params.Name
	sess.UpdatedAt = time.Now()
	s.store.Persist(sess.ID)
	return Response{JSONRPC: "2.0", Result: toInfo(sess), ID: req.ID}
}

// session.tag toggles a label on a session (adds if absent, removes if present)
// for grouping and filtering. returns the updated info so open views refresh.
func (s *Server) handleSessionTag(req Request) Response {
	var params struct {
		ID  string `json:"id"`
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Tag == "" {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "invalid params: tag required"}, ID: req.ID}
	}
	sess, ok := s.store.Get(params.ID)
	if !ok {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
	}
	sess.ToggleTag(params.Tag)
	s.store.Persist(sess.ID)
	return Response{JSONRPC: "2.0", Result: toInfo(sess), ID: req.ID}
}

// validRoles are the task roles a session may be assigned; mirrors the curated
// model table's Role values. an empty role (clearing) is always allowed.
var validRoles = map[string]bool{"general": true, "coding": true}

// session.setrole records a session's task role ("general"/"coding"), used by the
// client to default to the recommended model for that role. an empty role clears
// it. returns the updated info so open views refresh.
func (s *Server) handleSessionSetRole(req Request) Response {
	var params struct {
		ID   string `json:"id"`
		Role string `json:"role"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
	}
	if params.Role != "" && !validRoles[params.Role] {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "invalid role: " + params.Role}, ID: req.ID}
	}
	sess, ok := s.store.Get(params.ID)
	if !ok {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
	}
	sess.SetRole(params.Role)
	s.store.Persist(sess.ID)
	return Response{JSONRPC: "2.0", Result: toInfo(sess), ID: req.ID}
}

// session.setnumctx sets a per-session num_ctx override the daemon requests on
// each chat instead of the model-derived default; 0 clears it. returns the
// updated info so the client can refresh its context-window display.
func (s *Server) handleSessionSetNumCtx(req Request) Response {
	var params struct {
		ID     string `json:"id"`
		NumCtx int    `json:"num_ctx"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32600, Message: "invalid params"}, ID: req.ID}
	}
	sess, ok := s.store.Get(params.ID)
	if !ok {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
	}
	sess.SetNumCtx(params.NumCtx)
	s.store.Persist(sess.ID)
	return Response{JSONRPC: "2.0", Result: toInfo(sess), ID: req.ID}
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

// session.setcwd switches an existing session's working directory. it validates
// the target is an existing directory, updates the stored cwd, and rebuilds the
// filesystem-context system prompt (file tree + project context) for the new tree.
// tool calls read sess.CWD per-call, so the shell/file sandbox re-points to the new
// path automatically. returns the updated session info so inari refreshes its footer.
func (s *Server) handleSessionSetCwd(req Request) Response {
	var params struct {
		ID  string `json:"id"`
		CWD string `json:"cwd"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.CWD == "" {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "invalid params: cwd required"}, ID: req.ID}
	}
	sess, ok := s.store.Get(params.ID)
	if !ok {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "session not found"}, ID: req.ID}
	}
	cwd := expandUserPath(params.CWD)
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: "not a directory: " + params.CWD}, ID: req.ID}
	}
	sess.CWD = cwd
	sess.SetSystemPrompt(buildCWDSystemPrompt(cwd))
	// when a session already has conversation, its prior tool results (file listings,
	// file contents) describe the OLD cwd and are now stale; a model can regurgitate
	// them for the new directory instead of re-running tools. drop a marker into
	// history so it treats them as stale and re-reads. this must live in history near
	// the stale results (not in the system prompt) - recency is what makes the model
	// re-call the tool. the system role is not rendered by the TUI, so it stays
	// invisible to the user while still reaching the model. len>1 skips the marker on
	// a session that has only the system prompt (nothing stale yet).
	if len(sess.ChatHistory()) > 1 {
		sess.AppendMessage(provider.Message{Role: "system", Content: "working directory changed to " + cwd + "; previous file listings and file contents are stale, use the tools to read the new directory."})
	}
	s.store.Persist(sess.ID)
	return Response{JSONRPC: "2.0", Result: toInfo(sess), ID: req.ID}
}
