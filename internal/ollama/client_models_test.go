package ollama

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPingOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestPingUnreachable(t *testing.T) {
	if err := NewClient("http://127.0.0.1:0").Ping(); err == nil {
		t.Fatal("expected error for unreachable host, got nil")
	}
}

func TestPingNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).Ping(); err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"models":[{"name":"gemma4:e4b","size":123}]}`)
	}))
	defer srv.Close()

	models, err := NewClient(srv.URL).ListModels()
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 || models[0].Name != "gemma4:e4b" || models[0].Size != 123 {
		t.Errorf("got %+v, want one model gemma4:e4b size 123", models)
	}
}

func TestListRunning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ps" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"models":[{"name":"gemma4:e4b","size_vram":456,"expires_at":"2026-01-01T00:00:00Z"}]}`)
	}))
	defer srv.Close()

	models, err := NewClient(srv.URL).ListRunning()
	if err != nil {
		t.Fatalf("ListRunning: %v", err)
	}
	if len(models) != 1 || models[0].SizeVRAM != 456 {
		t.Errorf("got %+v, want one running model with size_vram 456", models)
	}
}

func TestLoadModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).LoadModel("gemma4:e4b"); err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
}

func TestLoadModelError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"model not found"}`)
	}))
	defer srv.Close()

	err := NewClient(srv.URL).LoadModel("missing")
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestUnloadModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).UnloadModel("gemma4:e4b"); err != nil {
		t.Fatalf("UnloadModel: %v", err)
	}
}

func TestDeleteModel(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).DeleteModel("gemma4:e4b"); err != nil {
		t.Fatalf("DeleteModel: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/delete" {
		t.Errorf("got %s %s, want DELETE /api/delete", gotMethod, gotPath)
	}
}

func TestDeleteModelError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"model not found"}`)
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).DeleteModel("missing"); err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestModelCapsReturnsTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/show" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"capabilities":["tools","vision"]}`)
	}))
	defer srv.Close()

	caps, err := NewClient(srv.URL).ModelCaps("gemma4:e4b")
	if err != nil {
		t.Fatalf("ModelCaps: %v", err)
	}
	if len(caps) != 2 || caps[0] != "tools" || caps[1] != "vision" {
		t.Errorf("got %v, want [tools vision]", caps)
	}
}

func TestModelCapsNotFoundReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	caps, err := NewClient(srv.URL).ModelCaps("missing")
	if err != nil {
		t.Fatalf("ModelCaps: unexpected error %v", err)
	}
	if len(caps) != 0 {
		t.Errorf("got %v, want empty slice", caps)
	}
}

func TestModelContextLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/show" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// model_info uses an architecture-prefixed context_length key.
		fmt.Fprint(w, `{"model_info":{"general.architecture":"llama","llama.context_length":40960,"llama.block_count":32}}`)
	}))
	defer srv.Close()

	n, err := NewClient(srv.URL).ModelContextLength("gemma4:e4b")
	if err != nil {
		t.Fatalf("ModelContextLength: %v", err)
	}
	if n != 40960 {
		t.Errorf("got %d, want 40960", n)
	}
}

func TestModelContextLengthUnknown(t *testing.T) {
	// 404, and a 200 with no context_length key, both yield 0 and no error.
	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer notFound.Close()
	if n, err := NewClient(notFound.URL).ModelContextLength("missing"); err != nil || n != 0 {
		t.Errorf("404: got (%d, %v), want (0, nil)", n, err)
	}

	noKey := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"model_info":{"general.architecture":"llama"}}`)
	}))
	defer noKey.Close()
	if n, err := NewClient(noKey.URL).ModelContextLength("nokey"); err != nil || n != 0 {
		t.Errorf("missing key: got (%d, %v), want (0, nil)", n, err)
	}
}
