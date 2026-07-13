package ipc

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

// SessionInfo is the wire representation of a session returned by session.list and session.create.
// it carries only the summary fields inari needs for display; full message history stays in inarid.
// ContextChars is the total character count of all messages (including system prompt),
// used by inari to estimate token usage without re-fetching history.
type SessionInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	CWD          string `json:"cwd,omitempty"`
	ContextChars int    `json:"context_chars,omitempty"`
}

// Client connects to inarid over a Unix Domain Socket.
// mu serialises calls; JSON-RPC over a single socket is inherently request/response,
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
// called lazily on first use and after any broken-pipe error.
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
// lazy dial: we don't connect at construction time because inarid may not be running yet.
// on any I/O error we nil the connection so the next call triggers a fresh dial.
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

// ToolRequestMsg is sent from the server when a tool call needs user approval before executing.
type ToolRequestMsg struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// ChatStream sends a user message and streams token chunks into tokens.
// it dials a fresh dedicated UDS connection so it never blocks the shared client
// connection; multiple sessions can stream concurrently without contention.
// when the server needs to run a tool it sends a ToolRequestMsg on toolReqs and
// blocks until the caller sends true/false on approvals. statuses carries
// coarse phase signals ("loading", "thinking") the server emits when the model
// is not yet resident in backend memory; ChatTokenMsg/onToken handle the actual
// content stream regardless of phase. the caller must drain tokens until it is
// closed; the goroutine closes it after the stream ends. the returned error
// reflects the final outcome.
func (c *Client) ChatStream(sessionID, text string, tokens chan<- string, statuses chan<- string, toolReqs chan<- ToolRequestMsg, approvals <-chan bool) error {
	conn, err := net.Dial("unix", c.socket)
	if err != nil {
		return fmt.Errorf("stream dial: %w", err)
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	req := Request{
		JSONRPC: "2.0",
		Method:  "session.stream",
		ID:      1,
	}
	b, _ := json.Marshal(map[string]string{"id": sessionID, "text": text})
	req.Params = json.RawMessage(b)

	if err := enc.Encode(req); err != nil {
		return err
	}

	for {
		var frame struct {
			Token       string          `json:"token"`
			Status      string          `json:"status"`
			Done        bool            `json:"done"`
			Error       string          `json:"error"`
			ToolRequest *ToolRequestMsg `json:"tool_request,omitempty"`
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
		if frame.Status != "" {
			statuses <- frame.Status
			continue
		}
		if frame.ToolRequest != nil {
			// forward to TUI, wait for user decision, send back to server.
			toolReqs <- *frame.ToolRequest
			approved := <-approvals
			if err := enc.Encode(map[string]bool{"tool_approved": approved}); err != nil {
				return err
			}
			continue
		}
		if frame.Token != "" {
			tokens <- frame.Token
		}
	}
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
