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

	// pre-send intercept: short-circuit empty/low-effort input (no alphanumeric
	// content, e.g. "?" or whitespace) with a local reply instead of a full model
	// round-trip. this is a latency/cost optimisation only; the tool-call safety
	// tiers still gate any actual execution for real messages.
	if !hasAlnum([]byte(params.Text)) {
		reply := "i didn't catch a question there; could you rephrase?"
		sess.AppendMessage(provider.Message{Role: "assistant", Content: reply})
		s.store.Persist(sess.ID)
		enc.Encode(map[string]string{"token": reply})
		enc.Encode(map[string]bool{"done": true})
		if s.verbose {
			log.Printf("[inarid->inariui] session.stream short-circuited low-effort input")
		}
		return
	}

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
	// names of the tools actually offered this stream, for the text-based fallback
	// below. empty when no cwd is set, so the fallback dispatches nothing.
	knownTools := make(map[string]bool, len(builtin))
	for _, t := range builtin {
		knownTools[t.Function.Name] = true
	}

	// Ollama cold-loads a model into memory before it can generate; if the model
	// is not already resident, signal the load phase so inari can show a distinct
	// "loading <model>..." state for round 0 instead of "thinking...".
	loading := s.modelNotResident(model)

	// repeat_penalty discourages the token-level repetition that drives runaway
	// generation loops; the n-gram tail detector in the stream loop below is the
	// hard backstop when a penalty alone does not break the cycle.
	opts := map[string]any{"repeat_penalty": 1.3}
	// prefer a per-session num_ctx override; otherwise derive a sensible window
	// from the model's declared context length (capped) so small models get a
	// larger-than-default window without risking OOM. a 0/unknown result omits the
	// option so Ollama falls back to its own default.
	nc := sess.NumCtxOverride
	if nc <= 0 {
		if maxCtx, err := s.modelContextLength(model); err == nil {
			nc = DefaultNumCtx(maxCtx)
		}
	}
	if nc > 0 {
		opts["num_ctx"] = nc
		// cap a single reply to the window so a loop cannot generate past it.
		opts["num_predict"] = nc
	}

	const maxToolRounds = 10
	// tool output accumulated across this turn's rounds. some models (e.g.
	// gemma4:e2b) reliably return an empty final answer after a tool result; when
	// that happens we surface this so the user sees the data they asked for rather
	// than a blank reply.
	var turnToolOutput strings.Builder
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
			sess.RemoveLast()
			enc.Encode(map[string]string{"error": err.Error()})
			if s.verbose {
				log.Printf("[inarid->inariui] session.stream error: %v", err)
			}
			return
		}

		if len(toolCalls) == 0 {
			// prompt-based tool-call fallback: a model (esp. a small one at high
			// temperature) sometimes writes a tool call as text instead of emitting a
			// native tool_call. if the text carries a recognised invocation, dispatch it
			// through the normal path below so approval + sandbox still apply. persisting
			// the structured call (not the raw text) also heals the history, so the model
			// stops few-shotting off its own narration. the raw text was already streamed
			// to the UI this turn; the real answer follows in the next round.
			if tc, ok := parseTextToolCall(textBuf.String(), knownTools); ok {
				toolCalls = []provider.ToolCall{tc}
				if s.verbose {
					log.Printf("[inarid] text tool-call fallback: %s(%v)", tc.Function.Name, tc.Function.Arguments)
				}
				// fall through to the tool-call dispatch below.
			} else {
				// text response: tokens already streamed above; persist and signal done.
				reply := textBuf.String()
				// some models return an empty final answer after a tool ran (they treat
				// the tool result as the answer). surface the accumulated tool output so
				// the user sees the data they asked for instead of a blank reply; the
				// tokens were never streamed (tool rounds are silent), so stream them now.
				if reply == "" && turnToolOutput.Len() > 0 {
					reply = turnToolOutput.String()
					enc.Encode(map[string]string{"token": reply})
				}
				sess.AppendMessage(provider.Message{Role: "assistant", Content: reply})
				s.store.Persist(sess.ID)
				enc.Encode(map[string]bool{"done": true})
				if s.verbose {
					log.Printf("[inarid->inariui] session.stream ok (%d chars)", len(reply))
				}
				return
			}
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
			// audit every model-invoked tool call (name + args), not just the outer
			// session.stream request that started the turn; this is the data the
			// curated-tool-surface review depends on.
			if b, mErr := json.Marshal(map[string]any{
				"session": sess.ID,
				"tool":    tc.Function.Name,
				"args":    tc.Function.Arguments,
				"failed":  err != nil,
			}); mErr == nil {
				s.auditor.Log("tool.call", json.RawMessage(b))
			}
			if turnToolOutput.Len() > 0 {
				turnToolOutput.WriteString("\n")
			}
			turnToolOutput.WriteString(result)
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

// modelContextLength returns the model's declared context window, memoised for the
// process lifetime (see Server.ctxLen). errors are not cached, so a transient
// backend failure is retried next turn rather than pinned to 0.
func (s *Server) modelContextLength(model string) (int, error) {
	if v, ok := s.ctxLen.Load(model); ok {
		return v.(int), nil
	}
	n, err := s.provider.ModelContextLength(model)
	if err != nil {
		return 0, err
	}
	s.ctxLen.Store(model, n)
	return n, nil
}
