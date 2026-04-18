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
// It carries only the summary fields fox needs for display — full message history stays in inarid.
type SessionInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
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
		c.conn = nil
		return nil, err
	}

	var resp Response
	if err := c.dec.Decode(&resp); err != nil {
		c.conn = nil
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
func (c *Client) CreateSession(name string) (SessionInfo, error) {
	resp, err := c.Call("session.create", map[string]string{"name": name})
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

// AssignModel assigns a model to a session in inarid.
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

func (c *Client) Quit() error {
	_, err := c.Call("daemon.quit", nil)
	return err
}

func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}
