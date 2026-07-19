// conn.go owns the connection layer: the accept loop and the per-connection
// JSON-RPC read loop, including the special-cased streaming and pull calls that
// take over a connection. it does NOT own the Server lifecycle, the idle
// watchdog, or method dispatch (server.go, dispatch.go).

package ipc

import (
	"encoding/json"
	"log"
	"net"
)

// accept runs until the listener is closed (on shutdown). each connection gets its own goroutine
// so a slow inari call (e.g. a long Ollama reply) doesn't block other clients.
func (s *Server) accept() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

// handle reads JSON-RPC requests from conn in a loop. the connection stays open across multiple
// calls so inari can reuse it without re-dialing for every operation.
// session.stream is handled specially: it takes over the connection for the duration of the
// stream and closes it when done, so the loop exits after one streaming call.
func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	for {
		var req Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		s.touch() // any RPC (including ping heartbeats) resets the idle watchdog
		if req.Method != "ping" {
			s.auditor.Log(req.Method, req.Params) // ping fires every heartbeat and adds no signal
		}

		if req.Method == "session.stream" {
			if s.verbose {
				log.Printf("[inariui->inarid] session.stream %s", req.Params)
			}
			s.handleStream(conn, dec, req)
			return
		}

		if req.Method == "model.pull" {
			if s.verbose {
				log.Printf("[inariui->inarid] model.pull %s", req.Params)
			}
			s.handlePull(conn, req)
			return
		}

		// suppress ping; fires on every heartbeat and adds no signal.
		if req.Method != "ping" && s.verbose {
			log.Printf("[inariui->inarid] %s %s", req.Method, req.Params)
		}
		resp := s.dispatch(req)
		if req.Method != "ping" && s.verbose {
			if resp.Error != nil {
				log.Printf("[inarid->inariui] %s error: %s", req.Method, resp.Error.Message)
			} else {
				log.Printf("[inarid->inariui] %s ok", req.Method)
			}
		}
		enc.Encode(resp)
	}
}
