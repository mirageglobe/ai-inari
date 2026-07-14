package ipc

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"strings"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// handleStream serves a session.stream request over a dedicated connection.
// if the session has a cwd set, filesystem tools are declared in the request.
// when the model responds with tool_calls, inarid executes them (sandboxed to cwd),
// appends the results, and re-sends, looping until the model returns a text reply.
// only tokens from the final text reply are forwarded to inari.
// the connection is closed by the caller (handle).
func (s *Server) handleStream(conn net.Conn, dec *json.Decoder, req Request) {
	enc := json.NewEncoder(conn)

	var params struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		enc.Encode(map[string]string{"error": "invalid params"})
		return
	}

	sess, ok := s.store.Get(params.ID)
	if !ok {
		enc.Encode(map[string]string{"error": "session not found"})
		return
	}
	model := s.modelFor(sess)
	if model == "" {
		enc.Encode(map[string]string{"error": "no model assigned to session"})
		return
	}

	sess.AppendMessage(provider.Message{Role: "user", Content: params.Text})

	// a cancellable context spans every tool round so a session.interrupt RPC can
	// abort the in-flight generation; registered under the session ID for lookup.
	ctx, cancel := context.WithCancel(context.Background())
	s.registerStream(params.ID, cancel)
	defer func() {
		cancel()
		s.unregisterStream(params.ID)
	}()

	var builtin []provider.Tool
	if sess.CWD != "" {
		builtin = filesystemTools()
	}

	// Ollama cold-loads a model into memory before it can generate; if the model
	// is not already resident, signal the load phase so inari can show a distinct
	// "loading <model>..." state for round 0 instead of "thinking...".
	loading := s.modelNotResident(model)

	// repeat_penalty discourages the token-level repetition that drives runaway
	// generation loops; the n-gram tail detector in the stream loop below is the
	// hard backstop when a penalty alone does not break the cycle.
	opts := map[string]any{"repeat_penalty": 1.3}
	// request a sensible num_ctx derived from the model's declared context window
	// so small models get a larger-than-default window, capped to avoid OOM. a 0
	// (unknown) result omits the option so Ollama falls back to its own default.
	if maxCtx, err := s.provider.ModelContextLength(model); err == nil {
		if nc := DefaultNumCtx(maxCtx); nc > 0 {
			opts["num_ctx"] = nc
			// cap a single reply to the window so a loop cannot generate past it.
			opts["num_predict"] = nc
		}
	}

	const maxToolRounds = 10
	for round := range maxToolRounds {
		if round == 0 && loading {
			enc.Encode(map[string]string{"status": "loading"})
		}

		chunks := make(chan provider.ChatResponse, 32)
		errCh := make(chan error, 1)
		go func() {
			errCh <- s.provider.ChatStream(ctx, provider.ChatRequest{
				Model:    model,
				Messages: sess.ChatHistory(),
				Stream:   true,
				Tools:    builtin,
				Options:  opts,
			}, chunks)
			close(chunks)
		}()

		// stream tokens to inari as they arrive; collect tool_calls from the done chunk.
		// tool-call rounds produce empty content so no tokens are forwarded during those rounds;
		// only the final text round produces visible output. the first chunk of round 0 also
		// marks the end of the load phase, since Ollama blocks the whole request until the
		// model is resident before it emits anything.
		var textBuf strings.Builder
		var toolCalls []provider.ToolCall
		var loopDetected bool
		for chunk := range chunks {
			if loading {
				enc.Encode(map[string]string{"status": "thinking"})
				loading = false
			}
			if len(chunk.Message.ToolCalls) > 0 {
				toolCalls = chunk.Message.ToolCalls
			}
			if chunk.Message.Content != "" {
				textBuf.WriteString(chunk.Message.Content)
				enc.Encode(map[string]string{"token": chunk.Message.Content})
				// a short sequence repeating at the tail means the model is stuck in a
				// generation loop; cancel so the ctx.Err() path below keeps the partial
				// reply and ends the turn cleanly instead of exhausting the context.
				if hasRepeatedTail(textBuf.String()) {
					loopDetected = true
					cancel()
					break
				}
			}
		}

		if err := <-errCh; err != nil {
			// user interrupt (ctx cancelled): keep whatever was generated so far and
			// end the turn cleanly rather than surfacing a scary error.
			if ctx.Err() != nil {
				if reply := textBuf.String(); reply != "" {
					sess.AppendMessage(provider.Message{Role: "assistant", Content: reply})
				}
				s.store.Persist(sess.ID)
				enc.Encode(map[string]bool{"done": true})
				if s.verbose {
					reason := "interrupted"
					if loopDetected {
						reason = "loop detected, stream cancelled"
					}
					log.Printf("[inarid->inariui] session.stream %s (%d chars)", reason, textBuf.Len())
				}
				return
			}
			sess.Messages = sess.Messages[:len(sess.Messages)-1]
			enc.Encode(map[string]string{"error": err.Error()})
			if s.verbose {
				log.Printf("[inarid->inariui] session.stream error: %v", err)
			}
			return
		}

		if len(toolCalls) == 0 {
			// text response: tokens already streamed above; persist and signal done.
			reply := textBuf.String()
			sess.AppendMessage(provider.Message{Role: "assistant", Content: reply})
			s.store.Persist(sess.ID)
			enc.Encode(map[string]bool{"done": true})
			if s.verbose {
				log.Printf("[inarid->inariui] session.stream ok (%d chars)", len(reply))
			}
			return
		}

		// tool-call round: append assistant message with calls, execute each, append results.
		sess.AppendMessage(provider.Message{Role: "assistant", ToolCalls: toolCalls})
		for _, tc := range toolCalls {
			// safe (read-only) tools and allowlisted shell commands execute immediately,
			// no round-trip to inari. any other tool call requires explicit user approval.
			if !safeTools[tc.Function.Name] && !shellAutoApproved(tc) {
				enc.Encode(map[string]any{
					"tool_request": map[string]any{
						"name": tc.Function.Name,
						"args": tc.Function.Arguments,
					},
				})
				var approval struct {
					Approved bool `json:"tool_approved"`
				}
				if decErr := dec.Decode(&approval); decErr != nil || !approval.Approved {
					sess.AppendMessage(provider.Message{Role: "tool", Content: "user denied tool execution"})
					if s.verbose {
						log.Printf("[inarid->builtin] %s denied by user", tc.Function.Name)
					}
					continue
				}
			}

			result, err := execTool(tc.Function.Name, tc.Function.Arguments, sess.CWD)
			if err != nil {
				result = "error: " + err.Error()
			}
			if s.verbose {
				log.Printf("[inarid->builtin] %s(%v) -> %d chars", tc.Function.Name, tc.Function.Arguments, len(result))
			}
			sess.AppendMessage(provider.Message{Role: "tool", Content: result})
		}
	}

	enc.Encode(map[string]string{"error": "tool call limit reached"})
}

// DefaultNumCtx returns the num_ctx to request for a model whose maximum context
// window is maxCtx: maxCtx capped at defaultNumCtxCap, or 0 when maxCtx is unknown
// (<= 0) so callers omit the option and fall back to the backend default. exported
// so the TUI shows the same effective window the daemon will actually request.
func DefaultNumCtx(maxCtx int) int {
	const defaultNumCtxCap = 8192
	if maxCtx <= 0 {
		return 0
	}
	if maxCtx < defaultNumCtxCap {
		return maxCtx
	}
	return defaultNumCtxCap
}

// modelNotResident reports whether model is absent from the backend's currently
// loaded models, meaning the next request will trigger a cold load. a ListRunning
// error is treated as "resident" (no load signal) so it never blocks the stream.
func (s *Server) modelNotResident(model string) bool {
	running, err := s.provider.ListRunning()
	if err != nil {
		return false
	}
	for _, r := range running {
		if r.Name == model {
			return false
		}
	}
	return true
}
