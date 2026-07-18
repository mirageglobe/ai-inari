package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/x/ansi"
)

// ref computes the expected wrapped viewport content the naive way: wrap the
// scrollback and the in-progress line independently. hardwrap is line-local, so
// this equals wrapping the whole concatenation, which is what the cache must match.
func ref(display []string, name, buf string, w int) string {
	base := ansi.Hardwrap(strings.Join(display, "\n"), w, true)
	partial := ansi.Hardwrap(assistantStyle.Render(name+": ")+buf, w, true)
	return base + "\n" + partial
}

// TestStreamRenderCacheMatches: the per-stream wrapped-base cache (P2) must produce
// byte-identical output to a naive full re-wrap, both cold and on a warm cache hit.
func TestStreamRenderCacheMatches(t *testing.T) {
	display := benchDisplay(10)
	buf := "hello world this is a streamed reply that is long enough to wrap around"
	mk := func() Chat {
		c := Chat{sessionName: "blue spirit", display: display, streamBuf: buf}
		c.viewport = viewport.New(40, 24)
		return c
	}
	c1 := mk()
	cold := c1.viewportContent() // cache miss then populate
	c2 := mk()
	_ = c2.viewportContent()     // seed cache
	warm := c2.viewportContent() // cache hit
	if cold != warm {
		t.Fatalf("warm-cache render diverged from cold render")
	}
	if want := ref(display, "blue spirit", buf, 40); cold != want {
		t.Fatalf("render mismatch:\n got %q\nwant %q", cold, want)
	}
}

// TestStreamRenderRefreshesOnResize: a mid-stream width change must refresh the
// cached base (keyed on viewport width), not serve the stale wrap.
func TestStreamRenderRefreshesOnResize(t *testing.T) {
	display := benchDisplay(5)
	c := Chat{sessionName: "s", display: display, streamBuf: "tok"}
	c.viewport = viewport.New(80, 24)
	_ = c.viewportContent() // seed cache at width 80
	c.viewport = viewport.New(20, 24)
	if got, want := c.viewportContent(), ref(display, "s", "tok", 20); got != want {
		t.Fatalf("resize not reflected:\n got %q\nwant %q", got, want)
	}
}
