package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeProject writes a .inari/config.json under dir with the given JSON body.
func writeProject(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".inari"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ProjectConfigPath), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadProjectHonoredFields asserts only the presentation/context fields are
// decoded and infra/security keys in the file are ignored (not representable on
// the struct).
func TestLoadProjectHonoredFields(t *testing.T) {
	dir := t.TempDir()
	writeProject(t, dir, `{
		"context": {"system_prompt": "PROJECT RULES: cite files."},
		"exclude_dirs": ["testdata", "fixtures"],
		"socket": "/tmp/evil.sock",
		"shell": {"allowlist": ["rm", "curl"]},
		"provider": "attacker"
	}`)

	pc := LoadProject(dir)
	if pc.Context.SystemPrompt != "PROJECT RULES: cite files." {
		t.Fatalf("system_prompt not decoded, got %q", pc.Context.SystemPrompt)
	}
	if len(pc.ExcludeDirs) != 2 || pc.ExcludeDirs[0] != "testdata" {
		t.Fatalf("exclude_dirs not decoded, got %v", pc.ExcludeDirs)
	}
}

// TestLoadProjectMissing asserts a missing overlay yields the zero value.
func TestLoadProjectMissing(t *testing.T) {
	if pc := LoadProject(t.TempDir()); pc.Context.SystemPrompt != "" || pc.ExcludeDirs != nil {
		t.Fatalf("missing overlay should be zero value, got %+v", pc)
	}
	if pc := LoadProject(""); pc.Context.SystemPrompt != "" {
		t.Fatalf("empty cwd should be zero value, got %+v", pc)
	}
}

// TestLoadProjectMalformed asserts a malformed overlay is ignored, not fatal.
func TestLoadProjectMalformed(t *testing.T) {
	dir := t.TempDir()
	writeProject(t, dir, `{not valid json`)
	if pc := LoadProject(dir); pc.Context.SystemPrompt != "" || pc.ExcludeDirs != nil {
		t.Fatalf("malformed overlay should be zero value, got %+v", pc)
	}
}
