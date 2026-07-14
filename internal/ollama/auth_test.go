package ollama

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewClientWithAuth asserts the auth transport injects the bearer token and
// any static headers on outbound requests, and that empty auth behaves like a
// plain client (no Authorization header).
func TestNewClientWithAuth(t *testing.T) {
	var gotAuth, gotCustom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCustom = r.Header.Get("X-Test")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	c := NewClientWithAuth(srv.URL, "secret", map[string]string{"X-Test": "v"})
	if err := c.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer secret")
	}
	if gotCustom != "v" {
		t.Fatalf("X-Test = %q, want %q", gotCustom, "v")
	}

	// no auth + no headers behaves like NewClient: no Authorization sent.
	gotAuth = ""
	c2 := NewClientWithAuth(srv.URL, "", nil)
	if err := c2.Ping(); err != nil {
		t.Fatalf("Ping (no auth): %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("expected no Authorization header, got %q", gotAuth)
	}
}
