package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/inari/internal/provider"
)

// modalInnerWidth caps at ModalInnerW on wide terminals and clamps to the
// terminal width (minus chrome) on narrow ones, with a hard floor.
func TestModalInnerWidth(t *testing.T) {
	cases := []struct {
		name      string
		termWidth int
		want      int
	}{
		{"unset falls back to cap", 0, ModalInnerW},
		{"wide terminal capped", 200, ModalInnerW},
		{"exactly at budget", UIWidth, ModalInnerW},
		{"narrow clamps to term minus chrome", 80, 76},
		{"very narrow hits floor", 10, 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := modalInnerWidth(c.termWidth); got != c.want {
				t.Errorf("modalInnerWidth(%d) = %d, want %d", c.termWidth, got, c.want)
			}
		})
	}
}

// selectorColumns must produce the four ordered columns, keep status/vram
// fixed, and never let the content overflow the inner width (content + the 8
// cells of table padding == inner, so the modal box width stays consistent).
func TestSelectorColumns(t *testing.T) {
	for _, inner := range []int{ModalInnerW, 64, 40, 20} {
		cols := selectorColumns(inner)
		if len(cols) != 4 {
			t.Fatalf("inner=%d: expected 4 columns, got %d", inner, len(cols))
		}
		titles := []string{cols[0].Title, cols[1].Title, cols[2].Title, cols[3].Title}
		want := []string{"model", "status", "notes", "est. vram"}
		for i := range want {
			if titles[i] != want[i] {
				t.Errorf("inner=%d: column %d title = %q, want %q", inner, i, titles[i], want[i])
			}
		}
		sum := cols[0].Width + cols[1].Width + cols[2].Width + cols[3].Width
		// content + 8 padding cols fills the inner width exactly once the modal
		// has real room (rest >= 24, i.e. inner >= 53); below that the minimum
		// column floor intentionally overflows the clamped narrow modal.
		if inner >= 53 && sum+8 != inner {
			t.Errorf("inner=%d: columns+padding = %d, want == inner width", inner, sum+8)
		}
	}
}

// buildSelectorRows lays out downloaded models first, then [pull] candidates,
// pulls notes from the curated table, and keeps rowLocal/rowModel aligned.
func TestBuildSelectorRows(t *testing.T) {
	local := []provider.Model{{Name: "gemma4:e2b", Size: 1500000000}}
	recommended := []CuratedModel{{TierGB: 16, Role: "coding", Model: "deepseek-r1:14b", Size: "~9gb", Notes: "r1-671b distil; strong coding and reasoning"}}

	rows, rowLocal, rowModel := buildSelectorRows(local, recommended, nil, 20)

	if len(rows) != 2 || len(rowLocal) != 2 || len(rowModel) != 2 {
		t.Fatalf("expected 2 aligned rows, got rows=%d local=%d model=%d", len(rows), len(rowLocal), len(rowModel))
	}

	// row 0: downloaded local model (not running), notes looked up from the curated table.
	if rows[0][0] != "gemma4:e2b" || rows[0][1] != "downloaded" || !rowLocal[0] {
		t.Errorf("local row = %v (local=%v), want downloaded gemma4:e2b", rows[0], rowLocal[0])
	}
	if !strings.Contains(rows[0][2], "2b effective") {
		t.Errorf("local notes = %q, want curated notes for gemma4:e2b", rows[0][2])
	}

	// row 1: [pull] candidate, its own curated notes, marked non-local.
	if rows[1][0] != "deepseek-r1:14b" || rows[1][1] != "[pull]" || rowLocal[1] {
		t.Errorf("pull row = %v (local=%v), want [pull] deepseek-r1:14b", rows[1], rowLocal[1])
	}
	if rowModel[0] != "gemma4:e2b" || rowModel[1] != "deepseek-r1:14b" {
		t.Errorf("rowModel = %v, want [gemma4:e2b deepseek-r1:14b]", rowModel)
	}

	// every notes cell must fit the truncation width.
	for i, r := range rows {
		if w := lipgloss.Width(r[2]); w > 20 {
			t.Errorf("row %d notes width %d exceeds 20: %q", i, w, r[2])
		}
	}

	// a local model resident in memory is statused "loaded", not "downloaded".
	loadedRows, _, _ := buildSelectorRows(local, nil, map[string]bool{"gemma4:e2b": true}, 20)
	if loadedRows[0][1] != "loaded" {
		t.Errorf("running local model status = %q, want loaded", loadedRows[0][1])
	}
}

// non-curated local models get an empty notes cell rather than a bogus lookup.
func TestCuratedNotes(t *testing.T) {
	if got := curatedNotes("gemma4:e2b"); !strings.Contains(got, "2b effective") {
		t.Errorf("curatedNotes(gemma4:e2b) = %q, want the §6.1 notes", got)
	}
	if got := curatedNotes("some-local-only:latest"); got != "" {
		t.Errorf("curatedNotes(non-curated) = %q, want empty", got)
	}
}

// RenderModal shows the new status/notes columns and caps the box width to the
// shared modal budget even on a very wide terminal.
func TestSelectorRenderModalWidth(t *testing.T) {
	m := NewModelSelector(nil).WithModalDimensions()
	out := m.RenderModal(200, 40)

	for _, col := range []string{"model", "status", "notes", "est. vram"} {
		if !strings.Contains(out, col) {
			t.Errorf("rendered modal missing %q column header", col)
		}
	}

	// lipgloss.Place centres the box and space-pads to the full terminal width,
	// so trim both sides to measure the box itself (its border chars survive trim).
	widest := 0
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(strings.TrimSpace(line)); w > widest {
			widest = w
		}
	}
	// box = inner + rounded border (2) + padding (2); centred on a 200-col
	// terminal it must stay at the capped width, not stretch to fill.
	if want := ModalInnerW + 4; widest != want {
		t.Errorf("modal box width = %d, want %d (capped)", widest, want)
	}
}

// [u] emits UnassignModelMsg for the target session when a model is assigned,
// and is a no-op when the session has none.
func TestSelectorUnloadHotkey(t *testing.T) {
	keyU := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}}

	// model assigned: [u] emits UnassignModelMsg for the target session.
	m := NewModelSelector(nil).ForSession("s1", "sess-one", "gemma4:e2b")
	_, cmd := m.Update(keyU)
	if cmd == nil {
		t.Fatal("[u] with an assigned model produced no command")
	}
	un, ok := cmd().(UnassignModelMsg)
	if !ok {
		t.Fatalf("[u] produced %T, want UnassignModelMsg", cmd())
	}
	if un.SessionID != "s1" || un.SessionName != "sess-one" {
		t.Errorf("UnassignModelMsg = %+v, want session s1/sess-one", un)
	}

	// no model assigned: [u] must not emit an unassign.
	m2 := NewModelSelector(nil).ForSession("s2", "sess-two", "")
	if _, cmd2 := m2.Update(keyU); cmd2 != nil {
		if _, isUnassign := cmd2().(UnassignModelMsg); isUnassign {
			t.Error("[u] with no assigned model should not emit UnassignModelMsg")
		}
	}
}

// [d] arms a disk-delete confirm on a downloaded row (no immediate delete);
// [y] confirms and fires a command; any other key cancels in place.
func TestSelectorDeleteHotkey(t *testing.T) {
	keyD := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}

	load := func() ModelSelector {
		m := NewModelSelector(nil).ForSession("s1", "sess-one", "")
		updated, _ := m.Update(modelsMsg{models: []provider.Model{{Name: "gemma4:e2b"}}})
		return updated.(ModelSelector)
	}

	// [d] on a downloaded row arms pendingDelete without firing a command.
	m := load()
	updated, cmd := m.Update(keyD)
	m = updated.(ModelSelector)
	if m.pendingDelete != "gemma4:e2b" {
		t.Fatalf("[d] pendingDelete = %q, want gemma4:e2b", m.pendingDelete)
	}
	if cmd != nil {
		t.Error("[d] should arm a confirm, not fire a delete command")
	}

	// any non-y key cancels the pending delete in place, no command.
	keyN := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
	updated, cmd = m.Update(keyN)
	m = updated.(ModelSelector)
	if m.pendingDelete != "" {
		t.Errorf("[n] should clear pendingDelete, got %q", m.pendingDelete)
	}
	if cmd != nil {
		t.Error("[n] cancel should not fire a command")
	}

	// re-arm then [y] confirms: clears pendingDelete, enters loading, returns a command.
	updated, _ = m.Update(keyD)
	m = updated.(ModelSelector)
	keyY := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
	updated, cmd = m.Update(keyY)
	m = updated.(ModelSelector)
	if m.pendingDelete != "" || !m.loading {
		t.Errorf("[y] want cleared pendingDelete + loading, got pending=%q loading=%v", m.pendingDelete, m.loading)
	}
	if cmd == nil {
		t.Error("[y] confirm should fire a delete command")
	}
}

// truncateCell cuts on rune boundaries with an ellipsis and never overflows.
func TestTruncateCell(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want string
	}{
		{"short", 20, "short"},
		{"exactly ten!", 12, "exactly ten!"},
		{"this is a long note about a model", 10, "this is..."},
		{"tiny", 2, "ti"},
	}
	for _, c := range cases {
		got := truncateCell(c.in, c.w)
		if got != c.want {
			t.Errorf("truncateCell(%q, %d) = %q, want %q", c.in, c.w, got, c.want)
		}
		if lipgloss.Width(got) > c.w {
			t.Errorf("truncateCell(%q, %d) width %d exceeds %d", c.in, c.w, lipgloss.Width(got), c.w)
		}
	}
}
