package ipc

import (
	"encoding/json"
	"net"

	"github.com/mirageglobe/inari/internal/provider"
)

// handlePull serves a model.pull request over a dedicated connection, forwarding
// download progress frames as they stream from the backend. the connection is
// closed by the caller (handle), matching handleStream's contract.
func (s *Server) handlePull(conn net.Conn, req Request) {
	enc := json.NewEncoder(conn)

	var params struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Model == "" {
		enc.Encode(map[string]string{"error": "invalid params"})
		return
	}
	if s.provider == nil {
		enc.Encode(map[string]string{"error": "provider not configured"})
		return
	}

	progress := make(chan provider.PullProgress, 8)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.provider.PullModel(params.Model, progress)
		close(progress)
	}()

	for p := range progress {
		enc.Encode(p)
	}
	if err := <-errCh; err != nil {
		enc.Encode(map[string]string{"error": err.Error()})
	}
}
