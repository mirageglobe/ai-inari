package views

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
)

// benchLine is a representative wrapped chat row (~74 visible chars, close to the
// common 80-col terminal width) used to build synthetic scrollback.
const benchLine = "blue spirit: the quick brown fox jumps over the lazy dog and then more"

// benchDisplay builds an n-line scrollback of realistic chat rows.
func benchDisplay(n int) []string {
	d := make([]string, n)
	for i := range d {
		d[i] = benchLine
	}
	return d
}

// BenchmarkStreamTurn measures the per-turn cost of the current onToken render
// path: each streamed token re-joins the full display scrollback and hardwraps
// the whole string (P2), while streamBuf grows by string concatenation (P4). the
// sweep over history length shows how per-token cost scales with session length;
// if the cost climbs with history, P2 is confirmed and the wrapped-base cache is
// justified. ns/op is one full reply; divide by tokens for per-token cost.
func BenchmarkStreamTurn(b *testing.B) {
	const tokens = 200    // tokens in one assistant reply (a medium answer)
	const token = "word " // ~5 chars/token, typical
	for _, h := range []int{0, 50, 200, 1000} {
		b.Run(fmt.Sprintf("history=%d", h), func(b *testing.B) {
			display := benchDisplay(h)
			vp := viewport.New(80, 24)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c := Chat{sessionName: "blue spirit", display: display}
				for t := 0; t < tokens; t++ {
					// mirrors onToken's hot path exactly (chat_stream.go:37,43).
					c.streamBuf += token
					setViewportContent(&vp, c.viewportContent())
				}
			}
		})
	}
}
