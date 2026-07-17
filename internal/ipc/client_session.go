package ipc

import (
	"encoding/json"
	"fmt"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// ListSessions returns all sessions from inarid.
func (c *Client) ListSessions() ([]SessionInfo, error) {
	resp, err := c.Call("session.list", nil)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("%s", resp.Error.Message)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}
	var sessions []SessionInfo
	if err := json.Unmarshal(b, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// CreateSession creates a new named session in inarid and returns its summary.
// cwd is optional; when non-empty inarid injects a shallow file tree into the session's
// system prompt so the model is aware of the project layout from the first message.
func (c *Client) CreateSession(name, cwd string) (SessionInfo, error) {
	return c.sessionInfoCall("session.create", map[string]string{"name": name, "cwd": cwd})
}

// DeleteSession removes a session from inarid by ID.
func (c *Client) DeleteSession(id string) error {
	resp, err := c.Call("session.delete", map[string]string{"id": id})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s", resp.Error.Message)
	}
	return nil
}

// UnassignModel detaches the model from a session in inarid.
// the session and its chat history are preserved; a new model can be assigned later.
func (c *Client) UnassignModel(sessionID string) error {
	resp, err := c.Call("session.unassign", map[string]string{"id": sessionID})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s", resp.Error.Message)
	}
	return nil
}

// AssignModel assigns a model to a session in inarid.
// any existing chat history is retained and sent as context to the new model.
func (c *Client) AssignModel(sessionID, model string) error {
	resp, err := c.Call("session.assign", map[string]string{"id": sessionID, "model": model})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s", resp.Error.Message)
	}
	return nil
}

// History fetches the full message history for a session.
// inari calls this on chat open to restore the conversation display.
func (c *Client) History(sessionID string) ([]provider.Message, error) {
	resp, err := c.Call("session.history", map[string]string{"id": sessionID})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("%s", resp.Error.Message)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}
	var messages []provider.Message
	if err := json.Unmarshal(b, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// ClearHistory removes all user/assistant messages from the session, retaining the system prompt.
func (c *Client) ClearHistory(sessionID string) error {
	resp, err := c.Call("session.clear", map[string]string{"id": sessionID})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s", resp.Error.Message)
	}
	return nil
}

// CompactHistory summarises the session history with the assigned model and replaces all
// user/assistant messages with the summary. returns the summary text on success.
func (c *Client) CompactHistory(sessionID string) (string, error) {
	resp, err := c.Call("session.compact", map[string]string{"id": sessionID})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("%s", resp.Error.Message)
	}
	summary, ok := resp.Result.(string)
	if !ok {
		b, err := json.Marshal(resp.Result)
		if err != nil {
			return "", fmt.Errorf("compact: unexpected result type")
		}
		if err := json.Unmarshal(b, &summary); err != nil {
			return "", fmt.Errorf("compact: unexpected result type")
		}
	}
	return summary, nil
}

// Recap returns a one-sentence "where you left off" summary for an idle session,
// or "" when the session is not idle long enough or has nothing to recap. never
// mutates the session history.
func (c *Client) Recap(sessionID string) (string, error) {
	resp, err := c.Call("session.recap", map[string]string{"id": sessionID})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("%s", resp.Error.Message)
	}
	recap, _ := resp.Result.(string)
	return recap, nil
}

// SetContext sets the system prompt for a session.
// the prompt is prepended as a system message on every subsequent chat request.
// pass an empty string to clear the context.
func (c *Client) SetContext(sessionID, prompt string) error {
	resp, err := c.Call("session.setcontext", map[string]string{"id": sessionID, "prompt": prompt})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s", resp.Error.Message)
	}
	return nil
}

// SetCwd switches a session's working directory via session.setcwd and returns
// the updated session info (new cwd + rebuilt system prompt) so the caller can
// refresh its view. errors when the path is not an existing directory.
func (c *Client) SetCwd(sessionID, cwd string) (SessionInfo, error) {
	return c.sessionInfoCall("session.setcwd", map[string]string{"id": sessionID, "cwd": cwd})
}

// Rename changes a session's display name via session.rename and returns the
// updated session info so the caller can refresh its label. errors on an empty
// name or an unknown session.
func (c *Client) Rename(sessionID, name string) (SessionInfo, error) {
	return c.sessionInfoCall("session.rename", map[string]string{"id": sessionID, "name": name})
}

// Tag toggles a label on a session via session.tag (adds if absent, removes if
// present) and returns the updated session info so the caller can refresh.
func (c *Client) Tag(sessionID, tag string) (SessionInfo, error) {
	return c.sessionInfoCall("session.tag", map[string]string{"id": sessionID, "tag": tag})
}

// SetRole records the session's task role ("general"/"coding", or "" to clear)
// via session.setrole and returns the updated session info.
func (c *Client) SetRole(sessionID, role string) (SessionInfo, error) {
	return c.sessionInfoCall("session.setrole", map[string]string{"id": sessionID, "role": role})
}

// SetNumCtx sets a per-session num_ctx override via session.setnumctx (0 clears
// it) and returns the updated session info so the caller can refresh its display.
func (c *Client) SetNumCtx(sessionID string, numCtx int) (SessionInfo, error) {
	resp, err := c.Call("session.setnumctx", map[string]any{"id": sessionID, "num_ctx": numCtx})
	if err != nil {
		return SessionInfo{}, err
	}
	return decodeSessionInfo(resp)
}

// sessionInfoCall issues an RPC with string params and decodes a SessionInfo
// result, the shared shape of the rename/tag/setcwd handlers.
func (c *Client) sessionInfoCall(method string, params map[string]string) (SessionInfo, error) {
	resp, err := c.Call(method, params)
	if err != nil {
		return SessionInfo{}, err
	}
	return decodeSessionInfo(resp)
}

// decodeSessionInfo turns a Response carrying a SessionInfo result into the typed
// value, surfacing any JSON-RPC error as a Go error.
func decodeSessionInfo(resp *Response) (SessionInfo, error) {
	if resp.Error != nil {
		return SessionInfo{}, fmt.Errorf("%s", resp.Error.Message)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		return SessionInfo{}, err
	}
	var sess SessionInfo
	if err := json.Unmarshal(b, &sess); err != nil {
		return SessionInfo{}, err
	}
	return sess, nil
}

// Chat sends a single user message to the session identified by sessionID.
// inarid owns the message history; it appends the message, sends the full
// history to Ollama, stores the reply, and returns the assistant's text.
func (c *Client) Chat(sessionID, text string) (string, error) {
	resp, err := c.Call("session.chat", map[string]string{"id": sessionID, "text": text})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("%s", resp.Error.Message)
	}
	reply, ok := resp.Result.(string)
	if !ok {
		return "", fmt.Errorf("unexpected response type")
	}
	return reply, nil
}

// Shell runs a user-authored `!` command in the session cwd via session.shell and
// returns its combined stdout+stderr. inarid runs it through a real shell (sh -c)
// and records the command + output in history so the model sees the result. errors
// on an empty command, an unknown session, or a session with no cwd set.
func (c *Client) Shell(sessionID, command string) (string, error) {
	resp, err := c.Call("session.shell", map[string]string{"id": sessionID, "command": command})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("%s", resp.Error.Message)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		return "", err
	}
	var out struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	return out.Output, nil
}
