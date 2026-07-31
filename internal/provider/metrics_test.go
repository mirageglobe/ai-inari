package provider

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

// a verbatim-shaped ollama /api/chat final chunk. the point of decoding the real
// wire shape is to pin the json tags: a typo in any of the five counter names
// silently yields zero, which would look like "the backend reports nothing".
const doneChunkJSON = `{
  "model": "gemma4:e2b",
  "created_at": "2026-07-31T10:00:00.000Z",
  "message": {"role": "assistant", "content": ""},
  "done_reason": "stop",
  "done": true,
  "total_duration": 4883583458,
  "load_duration": 1334875,
  "prompt_eval_count": 26,
  "prompt_eval_duration": 342546000,
  "eval_count": 282,
  "eval_duration": 4535599000
}`

func TestChatResponseDecodesOllamaCounters(t *testing.T) {
	var got ChatResponse
	if err := json.Unmarshal([]byte(doneChunkJSON), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Done {
		t.Error("done: want true")
	}
	// each counter checked by name so a wrong tag fails loudly rather than as a zero.
	for _, c := range []struct {
		name string
		got  int64
		want int64
	}{
		{"total_duration", got.TotalDuration, 4883583458},
		{"load_duration", got.LoadDuration, 1334875},
		{"prompt_eval_count", int64(got.PromptEvalCount), 26},
		{"prompt_eval_duration", got.PromptEvalDuration, 342546000},
		{"eval_count", int64(got.EvalCount), 282},
		{"eval_duration", got.EvalDuration, 4535599000},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestMetricsFromMapsCountersAndTTFT(t *testing.T) {
	var chunk ChatResponse
	if err := json.Unmarshal([]byte(doneChunkJSON), &chunk); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	m := MetricsFrom(chunk, 380*time.Millisecond)

	if m.PromptTokens != 26 || m.EvalTokens != 282 {
		t.Errorf("tokens = %d/%d, want 26/282", m.PromptTokens, m.EvalTokens)
	}
	if m.Eval != 4535599000*time.Nanosecond {
		t.Errorf("eval = %v, want 4.535599s", m.Eval)
	}
	if m.PromptEval != 342546000*time.Nanosecond {
		t.Errorf("prompt eval = %v, want 342.546ms", m.PromptEval)
	}
	if m.Total != 4883583458*time.Nanosecond {
		t.Errorf("total = %v, want 4.883583458s", m.Total)
	}
	// ttft is wall-clock measured by the caller, not a field ollama returns.
	if m.TTFT != 380*time.Millisecond {
		t.Errorf("ttft = %v, want 380ms", m.TTFT)
	}
	if !m.Recorded() {
		t.Error("recorded: want true for a chunk carrying counters")
	}
}

func TestTokensPerSec(t *testing.T) {
	cases := []struct {
		name string
		m    Metrics
		want float64
	}{
		{"normal", Metrics{EvalTokens: 282, Eval: 4535599000 * time.Nanosecond}, 62.1749},
		{"one token per second", Metrics{EvalTokens: 1, Eval: time.Second}, 1},
		// a backend that reports no timing must not produce Inf or NaN; callers
		// format this straight into a log line.
		{"zero duration", Metrics{EvalTokens: 282, Eval: 0}, 0},
		{"zero tokens", Metrics{EvalTokens: 0, Eval: time.Second}, 0},
		{"empty", Metrics{}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.m.TokensPerSec()
			if math.IsInf(got, 0) || math.IsNaN(got) {
				t.Fatalf("tokens/sec = %v, want a finite number", got)
			}
			if math.Abs(got-c.want) > 0.001 {
				t.Errorf("tokens/sec = %.4f, want %.4f", got, c.want)
			}
		})
	}
}

// a provider that returns no counters at all (or a chunk that is not the final
// one) must be distinguishable from one reporting a genuine zero, so callers can
// skip logging instead of writing an all-zero record.
func TestRecordedFalseWithoutCounters(t *testing.T) {
	if MetricsFrom(ChatResponse{Done: true}, 0).Recorded() {
		t.Error("recorded: want false when the chunk carries no counters")
	}
	if MetricsFrom(ChatResponse{}, 250*time.Millisecond).Recorded() {
		t.Error("recorded: want false for a mid-stream chunk even with a ttft")
	}
}
