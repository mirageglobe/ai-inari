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

func TestNotLocal(t *testing.T) {
	got := NotLocal(nil)
	if len(got) != len(CuratedModels) {
		t.Fatalf("expected all %d curated entries with nothing local, got %d", len(CuratedModels), len(got))
	}
	if got[0].TierGB != 32 {
		t.Fatalf("expected largest tier first, got tier %d", got[0].TierGB)
	}

	got = NotLocal([]string{"gemma4:e4b"})
	if len(got) != len(CuratedModels)-1 {
		t.Fatalf("expected one fewer entry once gemma4:e4b is local, got %d", len(got))
	}
	for _, c := range got {
		if c.Model == "gemma4:e4b" {
			t.Fatalf("gemma4:e4b should be excluded once local, got %+v", got)
		}
	}

	allNames := make([]string, len(CuratedModels))
	for i, c := range CuratedModels {
		allNames[i] = c.Model
	}
	if got := NotLocal(allNames); len(got) != 0 {
		t.Fatalf("expected no entries left once every curated model is local, got %+v", got)
	}
}
