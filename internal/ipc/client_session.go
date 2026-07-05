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
	resp, err := c.Call("session.create", map[string]string{"name": name, "cwd": cwd})
	if err != nil {
		return SessionInfo{}, err
	}
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
// fox calls this on chat open to restore the conversation display.
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
