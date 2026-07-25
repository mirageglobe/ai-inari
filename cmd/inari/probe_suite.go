// probe_suite.go owns the tool-surface probe's fixture repo and task list: the
// stimulus. it does NOT own running the tasks (probe.go) or scoring them
// (probe_report.go).

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// probeTask is one natural-language prompt plus the builtin a competent tool
// selection would reach for. prompts deliberately never name the tool: naming it
// measures instruction-following, not selection, which is what the audit is about.
type probeTask struct {
	name   string
	prompt string
	want   string
}

// probeTasks aims exactly one task at each declared builtin (enforced by
// TestProbeTasksCoverEveryBuiltin), so a builtin the model never picks shows up
// as an unused tool rather than a gap in the suite.
var probeTasks = []probeTask{
	{"list", "what files and directories are in this project's root?", "list_dir"},
	{"read", "what does the README say this project is for?", "read_file"},
	{"grep", "which files mention ParseConfig?", "grep_file"},
	{"stat", "how many bytes is logs/app.log, and when was it last modified?", "stat_file"},
	{"find", "list every go source file anywhere in this project", "find_files"},
	{"lines", "show me lines 100 to 110 of logs/app.log", "read_lines"},
	{"shell", "run the project's tests and tell me if they pass", "execute_shell_command"},
}

// writeProbeFixture builds a small but realistic go project under dir. realism
// matters: on an empty temp dir every question collapses to list_dir, which would
// score a perfect run while measuring nothing.
func writeProbeFixture(dir string) error {
	files := map[string]string{
		"README.md":   "# widget\n\nwidget is a small command line tool that reads a config file\nand prints a report. run `make test` to check it.\n",
		"Makefile":    "test:\n\tgo test ./...\n\nbuild:\n\tgo build ./...\n",
		"go.mod":      "module example.com/widget\n\ngo 1.22\n",
		"main.go":     "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"widget\")\n}\n",
		"config.json": "{\n  \"name\": \"widget\",\n  \"verbose\": false\n}\n",
		// ParseConfig is the grep target: one definition, one call site, so a
		// content search has something to find that a name search cannot.
		filepath.Join("internal", "parse", "parse.go"):      "package parse\n\n// ParseConfig reads the widget config file.\nfunc ParseConfig(path string) error {\n\treturn nil\n}\n",
		filepath.Join("internal", "parse", "parse_test.go"): "package parse\n\nimport \"testing\"\n\nfunc TestParseConfig(t *testing.T) {\n\tif err := ParseConfig(\"config.json\"); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n",
		filepath.Join("logs", "app.log"):                    buildProbeLog(300),
	}
	for rel, body := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			return err
		}
	}
	return nil
}

// buildProbeLog returns n numbered log lines: long enough that reading the whole
// file to answer "show me lines 100 to 110" is the wrong move, which is what makes
// read_lines the right answer rather than read_file.
func buildProbeLog(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "2026-07-25T10:00:%02dZ level=info seq=%d msg=\"widget tick\"\n", i%60, i)
	}
	return b.String()
}
