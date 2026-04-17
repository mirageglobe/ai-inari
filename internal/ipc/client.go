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

// Client connects to inarid over a Unix Domain Socket.
// mu serialises calls — JSON-RPC over a single socket is inherently request/response,
// so concurrent callers must queue rather than interleave writes and reads.
type Client struct {
	mu   sync.Mutex
	conn net.Conn
	enc  *json.Encoder
	dec  *json.Decoder
	seq  int
}

func NewClient(socket string) (*Client, error) {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn: conn,
		enc:  json.NewEncoder(conn),
		dec:  json.NewDecoder(conn),
	}, nil
}

func (c *Client) Call(method string, params any) (*Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
		return nil, err
	}

	var resp Response
	if err := c.dec.Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Ping() error {
	_, err := c.Call("ping", nil)
	return err
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

// Chat sends the full message history to inarid and returns the model's reply.
func (c *Client) Chat(model string, messages []ollama.Message) (string, error) {
	resp, err := c.Call("session.chat", map[string]any{"model": model, "messages": messages})
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
	c.conn.Close()
}
