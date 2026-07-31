// metrics.go owns inference timing for a single completed turn: the counters the
// backend returns on its final chunk, plus the caller-measured time to first
// token. it does NOT own where metrics are recorded (internal/ipc audit log) or
// how they are rendered (tui).

package provider

import "time"

// Metrics reports what one turn cost. every duration comes off the backend's
// final chunk except TTFT, which only the caller can observe.
type Metrics struct {
	PromptTokens int
	EvalTokens   int
	PromptEval   time.Duration // prefill: turning the prompt into kv cache
	Eval         time.Duration // decode: generating the reply
	Load         time.Duration // cold-loading the model into memory
	Total        time.Duration
	TTFT         time.Duration // wall clock from request sent to first content chunk
}

// MetricsFrom lifts the counters off a final chunk. ttft is passed in because the
// backend never reports it; only the code draining the stream can time it.
func MetricsFrom(r ChatResponse, ttft time.Duration) Metrics {
	return Metrics{
		PromptTokens: r.PromptEvalCount,
		EvalTokens:   r.EvalCount,
		PromptEval:   time.Duration(r.PromptEvalDuration),
		Eval:         time.Duration(r.EvalDuration),
		Load:         time.Duration(r.LoadDuration),
		Total:        time.Duration(r.TotalDuration),
		TTFT:         ttft,
	}
}

// TokensPerSec is the decode rate, the headline number when comparing models on
// one machine. zero when either side is missing, so callers formatting this into
// a log line never emit Inf or NaN.
func (m Metrics) TokensPerSec() float64 {
	if m.Eval <= 0 || m.EvalTokens <= 0 {
		return 0
	}
	return float64(m.EvalTokens) / m.Eval.Seconds()
}

// Recorded reports whether the backend actually returned counters. this separates
// a backend that reports nothing from one reporting a genuine zero, so callers can
// skip writing an all-zero record.
func (m Metrics) Recorded() bool {
	return m.EvalTokens > 0 || m.PromptTokens > 0 || m.Total > 0
}
