// dispatch_ollama.go owns the ollama.* JSON-RPC handlers (load, unload, delete,
// running, models, show) and daemon.quit. it does NOT own the session.*
// handlers (dispatch_session.go) or the method switch (dispatch.go).

package ipc

func (s *Server) handleOllamaLoad(req Request) Response {
	if r, ok := s.providerErr(req); !ok {
		return r
	}
	var params struct {
		Model string `json:"model"`
	}
	if r := decodeParams(req, &params); r != nil {
		return *r
	}
	if err := s.provider.LoadModel(params.Model); err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
	}
	return Response{JSONRPC: "2.0", Result: "loaded", ID: req.ID}
}

func (s *Server) handleOllamaUnload(req Request) Response {
	if r, ok := s.providerErr(req); !ok {
		return r
	}
	var params struct {
		Model string `json:"model"`
	}
	if r := decodeParams(req, &params); r != nil {
		return *r
	}
	if err := s.provider.UnloadModel(params.Model); err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
	}
	return Response{JSONRPC: "2.0", Result: "unloaded", ID: req.ID}
}

// handleOllamaDelete removes a model from local disk storage. destructive and
// irreversible; the TUI gates it behind a confirm prompt before this is called.
func (s *Server) handleOllamaDelete(req Request) Response {
	if r, ok := s.providerErr(req); !ok {
		return r
	}
	var params struct {
		Model string `json:"model"`
	}
	if r := decodeParams(req, &params); r != nil {
		return *r
	}
	if err := s.provider.DeleteModel(params.Model); err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
	}
	return Response{JSONRPC: "2.0", Result: "deleted", ID: req.ID}
}

func (s *Server) handleOllamaRunning(req Request) Response {
	if r, ok := s.providerErr(req); !ok {
		return r
	}
	running, err := s.provider.ListRunning()
	if err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
	}
	return Response{JSONRPC: "2.0", Result: running, ID: req.ID}
}

func (s *Server) handleOllamaModels(req Request) Response {
	if r, ok := s.providerErr(req); !ok {
		return r
	}
	models, err := s.provider.ListModels()
	if err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
	}
	return Response{JSONRPC: "2.0", Result: models, ID: req.ID}
}

func (s *Server) handleOllamaShow(req Request) Response {
	if r, ok := s.providerErr(req); !ok {
		return r
	}
	var params struct {
		Model string `json:"model"`
	}
	if r := decodeParams(req, &params); r != nil {
		return *r
	}
	caps, err := s.provider.ModelCaps(params.Model)
	if err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
	}
	return Response{JSONRPC: "2.0", Result: caps, ID: req.ID}
}

// handleOllamaContext returns the model's maximum context window in tokens (0 if unknown).
func (s *Server) handleOllamaContext(req Request) Response {
	if r, ok := s.providerErr(req); !ok {
		return r
	}
	var params struct {
		Model string `json:"model"`
	}
	if r := decodeParams(req, &params); r != nil {
		return *r
	}
	n, err := s.provider.ModelContextLength(params.Model)
	if err != nil {
		return Response{JSONRPC: "2.0", Error: &Error{Code: -32603, Message: err.Error()}, ID: req.ID}
	}
	return Response{JSONRPC: "2.0", Result: n, ID: req.ID}
}

func (s *Server) handleDaemonQuit(req Request) Response {
	s.closeQuit() // idempotent; also used by the idle watchdog
	return Response{JSONRPC: "2.0", Result: "shutting down", ID: req.ID}
}
