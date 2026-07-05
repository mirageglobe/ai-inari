package views

import "testing"

func TestDetectTier(t *testing.T) {
	const gb = uint64(1) << 30
	cases := []struct {
		name  string
		bytes uint64
		want  int
	}{
		{"exactly 32gb", 32 * gb, 32},
		{"just under 32gb", 32*gb - 1, 16},
		{"exactly 16gb", 16 * gb, 16},
		{"exactly 8gb", 8 * gb, 8},
		{"exactly 4gb", 4 * gb, 4},
		{"below smallest tier", 2 * gb, 4},
		{"well above top tier", 128 * gb, 32},
		{"zero (detection failed)", 0, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DetectTier(c.bytes); got != c.want {
				t.Errorf("DetectTier(%d) = %d, want %d", c.bytes, got, c.want)
			}
		})
	}
}

func TestRecommendedFor(t *testing.T) {
	got := RecommendedFor(8, nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 curated entries at the 8gb tier, got %d", len(got))
	}

	got = RecommendedFor(8, []string{"gemma4:e4b"})
	if len(got) != 1 || got[0].Model != "deepseek-r1:8b" {
		t.Fatalf("expected only deepseek-r1:8b left after gemma4:e4b is available, got %+v", got)
	}

	got = RecommendedFor(8, []string{"gemma4:e4b", "deepseek-r1:8b"})
	if len(got) != 0 {
		t.Fatalf("expected no recommendations once both 8gb-tier models are available, got %+v", got)
	}

	if got := RecommendedFor(999, nil); len(got) != 0 {
		t.Fatalf("expected no recommendations for an unknown tier, got %+v", got)
	}
}
