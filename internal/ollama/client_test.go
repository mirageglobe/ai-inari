package ollama

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mirageglobe/inari/internal/provider"
)

func TestPullModelStreamsProgressToSuccess(t *testing.T) {
	lines := []string{
		`{"status":"pulling manifest"}`,
		`{"status":"downloading","completed":50,"total":100}`,
		`{"status":"success"}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pull" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		for _, l := range lines {
			fmt.Fprintln(w, l)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	out := make(chan provider.PullProgress, 8)
	if err := c.PullModel("gemma4:e4b", out); err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	close(out)

	var got []provider.PullProgress
	for p := range out {
		got = append(got, p)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 progress updates, got %d: %+v", len(got), got)
	}
	if got[1].Completed != 50 || got[1].Total != 100 {
		t.Errorf("downloading update = %+v, want completed=50 total=100", got[1])
	}
	if got[2].Status != "success" {
		t.Errorf("final status = %q, want success", got[2].Status)
	}
}

func TestPullModelPropagatesErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"model not found"}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	out := make(chan provider.PullProgress, 8)
	err := c.PullModel("does-not-exist", out)
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}
