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
		{"general", 32, "qwen3.6:27b"}, // 32gb general: near-frontier chat pick
		{"general", 16, "phi4:14b"},    // first 16gb entry wins over the gemma4:12b alternate
		{"general", 4, "llama3.2:3b"},
		{"wizard", 32, ""}, // unknown role
	}
	for _, tc := range cases {
		if got := recommendedModel(tc.role, tc.tierGB); got != tc.want {
			t.Errorf("recommendedModel(%q, %d) = %q, want %q", tc.role, tc.tierGB, got, tc.want)
		}
	}
}
