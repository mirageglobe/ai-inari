package ipc

import (
	"encoding/json"
	"net"
)

// Client connects to sudamad over a Unix Domain Socket.
type Client struct {
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

func (c *Client) Close() {
	c.conn.Close()
}
