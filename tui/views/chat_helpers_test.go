package views

import "testing"

func TestBuildContextLine(t *testing.T) {
	cases := []struct {
		name         string
		cwd          string
		systemPrompt string
		want         string
	}{
		{"no cwd", "", "anything", ""},
		{"cwd without project context", "/tmp/proj", "working directory: /tmp/proj\ntree", "[context] cwd: /tmp/proj"},
		{"cwd with project context", "/tmp/proj", "working directory: /tmp/proj\n\nproject context:\nsome rules", "[context] cwd: /tmp/proj (+ project context)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildContextLine(c.cwd, c.systemPrompt)
			want := c.want
			if want != "" {
				want = thinkingStyle.Render(want)
			}
			if got != want {
				t.Errorf("buildContextLine(%q, ...) = %q, want %q", c.cwd, got, want)
			}
		})
	}
}
