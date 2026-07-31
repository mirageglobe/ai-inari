package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mirageglobe/inari/internal/config"
	"github.com/mirageglobe/inari/internal/ipc"
	"github.com/mirageglobe/inari/internal/provider"
)

// runTry evaluates a candidate model you do not yet run. it (1) resolves the tag
// against the ollama registry (cheap HEAD, no download), (2) pulls it if absent,
// then (3) drives it through the same tool-calling smoke test as `doctor --models`
// (fixture cwd + streaming turn + audit-log tool.call check - not reply-text, a
// model can narrate a tool result without one running). unlike doctor (preflight
// over already-configured models), this is the model-shopping path, so it pulls.
// exits non-zero if the tag 404s, the pull fails, or it runs but invokes no tool.
// --check does only the registry resolution: the cheap signal the curated-model
// list rebuild uses to catch dead tags without downloading gigabytes.
func runTry(args []string) {
	fs := flag.NewFlagSet("try", flag.ExitOnError)
	checkOnly := fs.Bool("check", false, "only check the tag resolves on the registry; do not pull or run")
	cfgFlag := fs.String("config", "", "path to config.json")
	fs.Parse(args) //nolint:errcheck

	tag := strings.TrimSpace(fs.Arg(0))
	if tag == "" {
		fmt.Fprintln(os.Stderr, "error: usage: inari try [--check] <model-tag>")
		os.Exit(2)
	}
	fmt.Printf("inari try %s\n\n", tag)

	// 1. resolve: a registry manifest HEAD catches a 404 tag before any download.
	if code, ok := modelResolves(tag); ok {
		line("ok", "resolves", fmt.Sprintf("%s  (registry %d)", tag, code))
	} else {
		line("fail", "resolves", fmt.Sprintf("%s  (registry %d, no such tag)", tag, code))
		os.Exit(1)
	}
	if *checkOnly {
		fmt.Println("\n[ ok ] tag resolves (--check: skipped pull + run)")
		return
	}

	cfgPath := defaultConfigPath()
	if *cfgFlag != "" {
		cfgPath = *cfgFlag
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		line("fail", "config", err.Error())
		os.Exit(1)
	}
	ensureDaemon(cfgPath)
	client := ipc.NewClient(defaultSocket)
	defer client.Close()

	// 2. pull if not already local.
	if present(client, tag) {
		line("ok", "pull", tag+"  (already local)")
	} else if err := pullModel(client, tag); err != nil {
		line("fail", "pull", tag+"  "+err.Error())
		os.Exit(1)
	} else {
		line("ok", "pull", tag+"  (downloaded)")
	}

	// 3. smoke test: same streaming tool-call verification doctor --models uses.
	fixture, err := os.MkdirTemp("", "inari-try-*")
	if err != nil {
		line("fail", "fixture", err.Error())
		os.Exit(1)
	}
	defer os.RemoveAll(fixture)
	if err := os.WriteFile(filepath.Join(fixture, "marker.txt"), []byte("inari\n"), 0644); err != nil {
		line("fail", "fixture", err.Error())
		os.Exit(1)
	}
	if ok, detail := verifyOne(client, tag, fixture, auditLogPath(cfg.DataDir)); ok {
		line("ok", "tool-call", tag+"  ("+detail+")")
		fmt.Printf("\n[ ok ] %s resolves, pulls, and invokes tools\n", tag)
		return
	} else {
		line("fail", "tool-call", tag+"  ("+detail+")")
		os.Exit(1)
	}
}

// registryManifestURL maps a model tag to its ollama registry manifest URL.
// library models (no "/") are looked up under library/; a missing ":tag" defaults
// to latest. namespaced tags (user/model) are used as-is.
func registryManifestURL(tag string) string {
	name, ver := tag, "latest"
	if i := strings.LastIndex(tag, ":"); i >= 0 {
		name, ver = tag[:i], tag[i+1:]
	}
	if !strings.Contains(name, "/") {
		name = "library/" + name
	}
	return "https://registry.ollama.ai/v2/" + name + "/manifests/" + ver
}

// modelResolves reports the registry manifest status for tag and whether it is 200.
// a HEAD request: no auth or download, just proof the tag exists.
func modelResolves(tag string) (int, bool) {
	req, err := http.NewRequest(http.MethodHead, registryManifestURL(tag), nil)
	if err != nil {
		return 0, false
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return 0, false
	}
	resp.Body.Close()
	return resp.StatusCode, resp.StatusCode == http.StatusOK
}

// present reports whether tag is already pulled locally (via the daemon).
func present(client *ipc.Client, tag string) bool {
	models, err := client.ListModels()
	if err != nil {
		return false // cannot prove presence; fall through to a pull attempt
	}
	names := make([]string, len(models))
	for i, m := range models {
		names[i] = m.Name
	}
	return modelPresent(names, tag)
}

// pullModel runs a headless `ollama pull` via the daemon, draining progress until
// it finishes or errors. progress is informational here; the returned error is the
// verdict. the channel is closed after PullModel returns so the drain goroutine ends.
func pullModel(client *ipc.Client, tag string) error {
	progress := make(chan provider.PullProgress, 64)
	done := make(chan struct{})
	go func() { drain(progress); close(done) }()
	err := client.PullModel(tag, progress)
	close(progress)
	<-done
	return err
}
