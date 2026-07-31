// stream_metrics.go owns writing per-round inference cost to the audit log. it
// does NOT own measuring it (internal/provider derives the numbers, stream.go
// times the first token) or rendering it.

package ipc

import (
	"encoding/json"
	"log"
	"math"

	"github.com/mirageglobe/inari/internal/provider"
)

// auditTurnMetrics records what one generation round cost. durations are emitted
// in milliseconds because nanoseconds are unreadable in a log line and no
// consumer needs that resolution.
func (s *Server) auditTurnMetrics(sessionID, model string, round int, m provider.Metrics) {
	// a backend that reports no counters leaves no record at all; an all-zero line
	// would read as a real turn that generated nothing.
	if !m.Recorded() {
		return
	}
	b, err := json.Marshal(map[string]any{
		"session":        sessionID,
		"model":          model,
		"round":          round,
		"prompt_tokens":  m.PromptTokens,
		"eval_tokens":    m.EvalTokens,
		"tokens_per_sec": math.Round(m.TokensPerSec()*100) / 100,
		"ttft_ms":        m.TTFT.Milliseconds(),
		"prefill_ms":     m.PromptEval.Milliseconds(),
		"decode_ms":      m.Eval.Milliseconds(),
		"load_ms":        m.Load.Milliseconds(),
		"total_ms":       m.Total.Milliseconds(),
	})
	if err != nil {
		return
	}
	s.auditor.Log("turn.metrics", json.RawMessage(b))
	if s.verbose {
		log.Printf("[inarid] turn.metrics model=%s round=%d %d tok @ %.1f tok/s ttft=%dms",
			model, round, m.EvalTokens, m.TokensPerSec(), m.TTFT.Milliseconds())
	}
}
