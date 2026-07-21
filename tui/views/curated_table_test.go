package views

import (
	"flag"
	"os"
	"strings"
	"testing"
)

// updateCurated rewrites the generated §6.1 region in SPEC.md instead of asserting;
// `make curated-sync` sets it. mirrors the golden-file `-update` convention.
var updateCurated = flag.Bool("update-curated", false, "rewrite the generated §6.1 region in ../../SPEC.md from CuratedModels")

const specPath = "../../SPEC.md"

// TestCuratedTablesInSync fails when SPEC.md §6.1 has drifted from CuratedModels,
// the single source. it runs under `make test`; `make curated-sync` regenerates.
func TestCuratedTablesInSync(t *testing.T) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	b := strings.Index(content, CuratedMarkerBegin)
	e := strings.Index(content, CuratedMarkerEnd)
	if b < 0 || e < 0 || e < b {
		t.Fatalf("curated markers not found in %s (begin=%d end=%d)", specPath, b, e)
	}
	innerStart := b + len(CuratedMarkerBegin)
	want := "\n\n" + RenderCuratedTables() + "\n"
	got := content[innerStart:e]

	if *updateCurated {
		if got == want {
			t.Logf("%s already in sync", specPath)
			return
		}
		updated := content[:innerStart] + want + content[e:]
		if err := os.WriteFile(specPath, []byte(updated), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote §6.1 region in %s", specPath)
		return
	}
	if got != want {
		t.Errorf("SPEC §6.1 is out of sync with CuratedModels; run `make curated-sync`\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}
