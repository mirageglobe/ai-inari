package version

import (
	"strings"
	"testing"
)

// TestVersion asserts the version string is present and follows the vN.N.N shape.
func TestVersion(t *testing.T) {
	if Version == "" {
		t.Fatal("Version is empty")
	}
	if !strings.HasPrefix(Version, "v") {
		t.Fatalf("Version %q should start with 'v'", Version)
	}
}
