package ipc

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// PullModel triggers a background `ollama pull` for model via inarid, streaming
// progress updates into progress until the download completes or fails.
// mirrors ChatStream: a dedicated UDS connection so it never blocks the shared
// client connection. the caller must drain progress until this returns.
func (c *Client) PullModel(model string, progress chan<- provider.PullProgress) error {
	conn, err := net.Dial("unix", c.socket)
	if err != nil {
		return fmt.Errorf("pull dial: %w", err)
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	req := Request{JSONRPC: "2.0", Method: "model.pull", ID: 1}
	b, _ := json.Marshal(map[string]string{"model": model})
	req.Params = json.RawMessage(b)

	if err := enc.Encode(req); err != nil {
		return err
	}

	for {
		var frame struct {
			Status    string `json:"status"`
			Completed int64  `json:"completed"`
			Total     int64  `json:"total"`
			Error     string `json:"error"`
		}
		if err := dec.Decode(&frame); err != nil {
			return err
		}
		if frame.Error != "" {
			return fmt.Errorf("%s", frame.Error)
		}
		progress <- provider.PullProgress{Status: frame.Status, Completed: frame.Completed, Total: frame.Total}
		if frame.Status == "success" {
			return nil
		}
	}
}
