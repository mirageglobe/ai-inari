package ipc

import "testing"

// countingCtxLenProvider embeds the shared fake and counts ModelContextLength
// calls so the memo can be observed.
type countingCtxLenProvider struct {
	fakeAssignProvider
	calls int
}

func (p *countingCtxLenProvider) ModelContextLength(string) (int, error) {
	p.calls++
	return 4096, nil
}

// TestModelContextLengthMemoised asserts repeated lookups for the same model hit
// the backend once, while a different model is a fresh key.
func TestModelContextLengthMemoised(t *testing.T) {
	p := &countingCtxLenProvider{}
	srv := &Server{provider: p}

	for i := 0; i < 3; i++ {
		n, err := srv.modelContextLength("m")
		if err != nil || n != 4096 {
			t.Fatalf("lookup %d: got (%d, %v), want (4096, nil)", i, n, err)
		}
	}
	if p.calls != 1 {
		t.Fatalf("same model should hit the backend once (memoised), got %d calls", p.calls)
	}

	if _, err := srv.modelContextLength("other"); err != nil {
		t.Fatalf("second model lookup: %v", err)
	}
	if p.calls != 2 {
		t.Fatalf("a different model should miss the cache, got %d calls total", p.calls)
	}
}
