// Package ipc implements the JSON-RPC 2.0 transport between fox and inarid over a Unix Domain Socket.
// Client is used by fox; Server is used by inarid. Both live here to keep the protocol in one place.
package ipc

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/mirageglobe/ai-inari/internal/ollama"
)

// SessionInfo is the wire representation of a session returned by session.list and session.create.
// it carries only the summary fields fox needs for display — full message history stays in inarid.
// ContextChars is the total character count of all messages (including system prompt),
// used by fox to estimate token usage without re-fetching history.
type SessionInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	CWD          string `json:"cwd,omitempty"`
	ContextChars int    `json:"context_chars,omitempty"`
}

// Client connects to inarid over a Unix Domain Socket.
// mu serialises calls — JSON-RPC over a single socket is inherently request/response,
// so concurrent callers must queue rather than interleave writes and reads.
type Client struct {
	socket string
	mu     sync.Mutex
	conn   net.Conn
	enc    *json.Encoder
	dec    *json.Decoder
	seq    int
}

func NewClient(socket string) *Client {
	return &Client{socket: socket}
}

// reconnect dials the socket and wires up a fresh encoder/decoder pair.
// Called lazily on first use and after any broken-pipe error.
func (c *Client) reconnect() error {
	conn, err := net.Dial("unix", c.socket)
	if err != nil {
		return err
	}
	c.conn = conn
	c.enc = json.NewEncoder(conn)
	c.dec = json.NewDecoder(conn)
	return nil
}

// Call serialises a JSON-RPC request, writes it, and reads the response.
// Lazy dial: we don't connect at construction time because inarid may not be running yet.
// On any I/O error we nil the connection so the next call triggers a fresh dial.
func (c *Client) Call(method string, params any) (*Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		if err := c.reconnect(); err != nil {
			return nil, fmt.Errorf("reconnect failed: %w", err)
		}
	}

	c.seq++
	req := Request{
		JSONRPC: "2.0",
		Method:  method,
		ID:      c.seq,
	}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		req.Params = json.RawMessage(b)
	}

	if err := c.enc.Encode(req); err != nil {
		c.conn = nil // force reconnect on next call
		return nil, err
	}

	var resp Response
	if err := c.dec.Decode(&resp); err != nil {
		c.conn = nil // force reconnect on next call
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Ping() error {
	_, err := c.Call("ping", nil)
	return err
}

func (c *Client) TryReconnect() error {
	return c.reconnect()
}

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
// The session and its chat history are preserved; a new model can be assigned later.
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
// Any existing chat history is retained and sent as context to the new model.
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

// ListModels returns models available in Ollama via inarid.
func (c *Client) ListModels() ([]ollama.Model, error) {
	resp, err := c.Call("ollama.models", nil)
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
	var models []ollama.Model
	if err := json.Unmarshal(b, &models); err != nil {
		return nil, err
	}
	return models, nil
}

// ListRunning returns models currently loaded in Ollama memory.
func (c *Client) ListRunning() ([]ollama.RunningModel, error) {
	resp, err := c.Call("ollama.running", nil)
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
	var models []ollama.RunningModel
	if err := json.Unmarshal(b, &models); err != nil {
		return nil, err
	}
	return models, nil
}

// LoadModel warms up the named model in Ollama memory via inarid.
func (c *Client) LoadModel(model string) error {
	resp, err := c.Call("ollama.load", map[string]string{"model": model})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s", resp.Error.Message)
	}
	return nil
}

// UnloadModel evicts the named model from Ollama memory via inarid.
func (c *Client) UnloadModel(model string) error {
	resp, err := c.Call("ollama.unload", map[string]string{"model": model})
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
func (c *Client) History(sessionID string) ([]ollama.Message, error) {
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
	var messages []ollama.Message
	if err := json.Unmarshal(b, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// ChatStream sends a user message and streams token chunks into tokens.
// it dials a fresh dedicated UDS connection so it never blocks the shared client
// connection — multiple sessions can stream concurrently without contention.
// the caller must drain tokens until it is closed; the goroutine closes it after
// the stream ends (success or error). the returned error reflects the final outcome.
func (c *Client) ChatStream(sessionID, text string, tokens chan<- string) error {
	conn, err := net.Dial("unix", c.socket)
	if err != nil {
		return fmt.Errorf("stream dial: %w", err)
	}
	defer conn.Close()

	req := Request{
		JSONRPC: "2.0",
		Method:  "session.stream",
		ID:      1,
	}
	b, _ := json.Marshal(map[string]string{"id": sessionID, "text": text})
	req.Params = json.RawMessage(b)

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return err
	}

	dec := json.NewDecoder(conn)
	for {
		var frame struct {
			Token string `json:"token"`
			Done  bool   `json:"done"`
			Error string `json:"error"`
		}
		if err := dec.Decode(&frame); err != nil {
			return err
		}
		if frame.Error != "" {
			return fmt.Errorf("%s", frame.Error)
		}
		if frame.Done {
			return nil
		}
		if frame.Token != "" {
			tokens <- frame.Token
		}
	}
}

// Chat sends a single user message to the session identified by sessionID.
// inarid owns the message history — it appends the message, sends the full
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

func (c *Client) Quit() error {
	_, err := c.Call("daemon.quit", nil)
	return err
}

func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}
