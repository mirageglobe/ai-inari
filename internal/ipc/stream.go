package ipc

import (
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

	var builtin []provider.Tool
	if sess.CWD != "" {
		builtin = filesystemTools()
	}

	const maxToolRounds = 10
	for range maxToolRounds {
		chunks := make(chan provider.ChatResponse, 32)
		errCh := make(chan error, 1)
		go func() {
			errCh <- s.provider.ChatStream(provider.ChatRequest{
				Model:    model,
				Messages: sess.ChatHistory(),
				Stream:   true,
				Tools:    builtin,
			}, chunks)
			close(chunks)
		}()

		// stream tokens to inari as they arrive; collect tool_calls from the done chunk.
		// tool-call rounds produce empty content so no tokens are forwarded during those rounds;
		// only the final text round produces visible output.
		var textBuf strings.Builder
		var toolCalls []provider.ToolCall
		for chunk := range chunks {
			if len(chunk.Message.ToolCalls) > 0 {
				toolCalls = chunk.Message.ToolCalls
			}
			if chunk.Message.Content != "" {
				textBuf.WriteString(chunk.Message.Content)
				enc.Encode(map[string]string{"token": chunk.Message.Content})
			}
		}

		if err := <-errCh; err != nil {
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
			// safe (read-only) tools execute immediately, no round-trip to inari.
			// all other tools (currently only "run") require explicit user approval.
			if !safeTools[tc.Function.Name] {
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
