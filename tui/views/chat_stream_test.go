package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
)

func newSpinnerForTest() spinner.Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return sp
}

// a "loading" status sets loadingModel to the session's assigned model; a
// subsequent "thinking" status clears it, mirroring the load-then-generate
// transition inarid signals once the cold-load phase ends.
func TestOnStatusTracksLoadingModel(t *testing.T) {
	c := Chat{sessionID: "s1", model: "gemma4:e2b", spinner: newSpinnerForTest()}

	got, _ := c.onStatus(ChatStatusMsg{SessionID: "s1", Status: "loading"})
	c = got.(Chat)
	if c.loadingModel != "gemma4:e2b" {
		t.Fatalf("loadingModel = %q, want gemma4:e2b", c.loadingModel)
	}

	got, _ = c.onStatus(ChatStatusMsg{SessionID: "s1", Status: "thinking"})
	c = got.(Chat)
	if c.loadingModel != "" {
		t.Fatalf("loadingModel = %q, want empty after thinking status", c.loadingModel)
	}
}

// a status for a different session is ignored, matching onToken/onDone's
// session-routing guard.
func TestOnStatusIgnoresOtherSession(t *testing.T) {
	c := Chat{sessionID: "s1", model: "gemma4:e2b", spinner: newSpinnerForTest()}
	got, _ := c.onStatus(ChatStatusMsg{SessionID: "other", Status: "loading"})
	c = got.(Chat)
	if c.loadingModel != "" {
		t.Fatalf("loadingModel = %q, want empty for a status routed to another session", c.loadingModel)
	}
}

// the waiting spinner shows "loading <model>..." while a load is in progress,
// falling back to "thinking..." once cleared; a running tool always takes
// priority over both.
func TestViewportContentLoadingLabel(t *testing.T) {
	c := Chat{waiting: true, loadingModel: "gemma4:e2b", spinner: newSpinnerForTest()}
	if got := c.viewportContent(); !strings.Contains(got, "loading gemma4:e2b...") {
		t.Fatalf("viewportContent() = %q, want it to contain the loading label", got)
	}

	c.loadingModel = ""
	if got := c.viewportContent(); !strings.Contains(got, "thinking...") {
		t.Fatalf("viewportContent() = %q, want it to fall back to thinking", got)
	}

	c.loadingModel = "gemma4:e2b"
	c.runningTool = "read_file"
	if got := c.viewportContent(); !strings.Contains(got, "running: read_file...") {
		t.Fatalf("viewportContent() = %q, want a running tool to take priority over loading", got)
	}
}
