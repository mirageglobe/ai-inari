package views

import "testing"

// TestRecommendedModel asserts the role+tier lookup picks the highest curated
// tier at or below the detected hardware, and returns "" for an unknown role.
func TestRecommendedModel(t *testing.T) {
	cases := []struct {
		role   string
		tierGB int
		want   string
	}{
		{"coding", 8, "deepseek-r1:8b"},
		{"coding", 12, "deepseek-r1:8b"}, // between 8 and 16 -> highest <= 12 is 8
		{"coding", 16, "deepseek-r1:14b"},
		{"general", 16, "phi-4:14b"},
		{"general", 4, "llama3.2:3b"},
		{"wizard", 32, ""}, // unknown role
	}
	for _, tc := range cases {
		if got := recommendedModel(tc.role, tc.tierGB); got != tc.want {
			t.Errorf("recommendedModel(%q, %d) = %q, want %q", tc.role, tc.tierGB, got, tc.want)
		}
	}
}
