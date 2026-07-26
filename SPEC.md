# Inari — Project Spec

Security-first, minimalist local AI orchestrator.

---

## 1. Goals

- Raise the bar on local LLM/SLM performance — through better context, tooling, and orchestration, make small models punch above their weight.
- Run and orchestrate local LLMs (via Ollama) from a single terminal UI.
- Keep the security surface minimal: no network exposure, no cloud dependencies.
- Support parallel model execution with explicit resource budgeting.
- Remain inspectable: all tool-calls are audited and visible to the operator.

## 2. Non-Goals

- Cloud or remote model backends.
- Multi-user or networked access.
- GUI / web interface.
- Model training or fine-tuning.

---

## 2.1 Development Strategy

**make it work → make it right.**

the guiding sequence for inari is: ship working features on concrete implementations first, then refactor toward open architecture once the right abstraction shape is known.

designing abstractions too early produces interfaces that fit the first implementation but break the moment a second is added. inari currently has one inference backend (Ollama) and one tool-calling mode — introducing a `Provider` interface now would be guessing. the right shape only becomes visible when writing real code against two concrete targets.

**practical sequence:**

1. **finish the basics** — prompt-based tool calling, session context, streaming stability. ship features that prove the design.
2. **add a second backend** — e.g. LM Studio (OpenAI-compatible). this is the moment the interface shape becomes obvious, not before.
3. **extract the abstraction** — the `Provider` interface is pulled from two working implementations, not invented upfront. it reflects reality.

**guard against premature abstraction:**

- the Ollama client is already isolated in `internal/ollama` — nothing outside imports Ollama-specific types directly. the boundary is there when needed.
- do not add `Provider` interfaces, plugin systems, or backend registries until a second concrete backend exists.
- when in doubt: duplicate once, abstract on the second duplication.

---

## 2.2 Terminology

- **session** - a persistent conversation container: cwd, model assignment, message history, tags. this is what the TUI's `Sessions` view lists and lets you create/select. a session on its own does nothing autonomous, so calling it an "agent" would overstate it - the view was renamed from `Agents` to `Sessions` for exactly this reason (see Done log).
- **agent** - an actor that does something on its own initiative. always qualified by tier, never used bare:
  - **thinker agent** - the model you converse with directly inside a session (the "Head Inari"). foreground, turn-by-turn, driven by the user.
  - **runner agent** - a model dispatched to do background work (intent classification, sub-tasks, tool calls) without blocking the thinker or waiting on the user each step.
- config's `models.thinker`/`models.worker`/`models.sensor` are now `models.thinker`/`models.runner`: worker and sensor were consolidated into one runner tier since neither had a real consumer yet (see §6) and keeping them split was guessing at a shape - splitting the tier back out is easy once actual routing logic needs a faster/cheaper classification pass than the runner's execution pass.

---

## 3. Roadmap

items carry a component tag (`[inarid]`/`[inarit]`/`[inari]`) and a difficulty tag
(`[easy]`/`[medium]`/`[hard]`). a work-type tag is optional: `[issue]` for a bug or
defect, `[feature]` for new capability (untagged reads as a feature). issues and
features live inline in Near-term/Ideas under these tags rather than in a separate
section.

### Milestones

#### M1 - UDS Bridge & Config
- [x] `[inarid]` starts and binds UDS socket.
- [x] `[inarit/inarid]` connects, performs handshake, and does a basic ping/pong JSON-RPC round-trip.
- [x] `[inarid]` `config.json` parsed at startup (created from defaults if absent); XDG path `~/.config/inari/config.json`.
- [x] `[inarid]` MCP connectors spawned as child processes.

#### M2 - Herd UI
- [x] `[inarit]` Bubble Tea table renders active sessions.
- [x] `[inarit/inarid]` sessions update in real time from daemon events.
- [x] `[inarit]` keyboard navigation (select, quit).

#### M3 - Ollama Integration & Chat
- [x] `[inarid]` daemon POSTs to Ollama `/api/chat` and streams tokens. *(the memory-budget throttle originally scoped here was never wired; the `scheduler` package exists but is unused. tracked as an Ideas item below.)*
- [x] `[inarit/inarid]` token stream forwarded to the inarit chat view.
- [x] `[inarit]` interactive `i` chat view wires to Head Inari (Thinker tier).
- [x] `[inarid]` message history scoped to session; detach/reattach preserves session state.

### Near-term
- [ ] `[inarid]` `[medium]` **review and improve the builtin tool surface** - the tool set (`internal/ipc/tools.go`: `read_file`, `list_dir`, `grep_file`, `stat_file`, `find_files`, `read_lines`, `execute_shell_command`, plus the `awk`/`sed`/`jq` shell allowlist) just grew in 86062ea; audit real session tool-call logs for what still falls through to `execute_shell_command`, which builtins go unused, and where schemas/descriptions confuse tool selection for small models, then improve based on findings.
- [ ] `[inarid]` `[medium]` **inference telemetry (capture what ollama already returns)** - ollama's `done` chunk carries `prompt_eval_count`, `prompt_eval_duration`, `eval_count`, `eval_duration` and `total_duration`; none of the five appear anywhere in the Go tree, and `probe`/`try` record no elapsed time either, so the project has no inference performance signal at all. scope: decode the fields into the provider's stream chunk, derive tokens/sec (`eval_count / eval_duration`) and time-to-first-token, expose them per turn and record them to the audit log. this is the prerequisite the two routing items are missing: **task difficulty/effort classification** and **multi-model routing** (both Ideas) need a measured cost signal to escalate or de-escalate against, and today tier choice rests on RAM bucket plus the prose adjectives in §6.1 rather than numbers from this machine. no new dependency and no benchmark harness; the data is already on the wire and discarded.
- [ ] `[inarid]` `[medium]` **reasoning-token handling (verify, then fold)** - `deepseek-r1` is curated at both the 8gb and 16gb coding tiers, and r1 is a reasoning model that emits chain-of-thought before its answer. the tree has no `<think>` parse and no read of ollama's separate `thinking` response field; the only matches are the `thinkingStyle`/`"thinking"` spinner strings in the TUI and the daemon's coarse phase signal. verify first by driving a real r1 turn through the chat view: if reasoning tokens arrive in `message.content`, they currently render as the reply. scope after that measurement: detect the reasoning segment, keep it out of the persisted answer, and fold it behind a toggle rather than dropping it silently.
- [ ] `[inari]` `[easy]` **rename the repo `ai-inari` to `inari`** - the `ai-` prefix survives into no user-facing surface: the binaries are `inari`/`inarid`/`inarit`, the tap formula is `Formula/inari.rb`, and the tap README lists the tool as `inari`. it is also not a namespace in use elsewhere; the sibling repos are `scout`, `monorepo` and `homebrew-tap`. measured blast radius: module path `github.com/mirageglobe/ai-inari` plus 88 files referencing `ai-inari` (mostly import paths), `.goreleaser.yaml`, the `Makefile`, and 9 references in dot-case (`workspace-ventures/prj-portfolio.md`, `SPEC.md`). github redirects renamed repos for both web and git, so the published v0.3.0 release download urls in the tap keep resolving; retarget them anyway for tidiness. open question to settle before committing: bare `inari` is a crowded search term (shinto deity, sushi), so check github for name collisions first; discoverability is the only argument for keeping the prefix. cheapest now, more expensive after every further release.

### Ideas
- [ ] `[inarid]` `[hard]` **scripting layer for agent execution (Yaegi vs Deno vs status quo)** - evaluated replacing or augmenting the model's `execute_shell_command` path with an embedded interpreter. **decision so far: not the in-process Go interpreter as pitched.** the deciding axis is the trust boundary (the *model* is the untrusted author), not execution latency (subprocess spawn ~30ms is immaterial against multi-second inference and the render hot path, both measured). options: (a) **Yaegi** (in-process Go) is fast and zero-external-dependency, but has no real sandbox: disabling `unsafe`/`syscall` still leaves `os`/`os/exec`/`net`, so model-authored Go is as dangerous as shell or worse and has no allowlist concept; only safe for *user*-authored plugins/macros (trusted, config-time), never model output. (b) **Deno** is a genuine deny-by-default permission sandbox (`--allow-read=<cwd>` only, no net/write/spawn), the safer runtime for model-authored code, but adds an external runtime and re-adds process spawn. (c) **status quo + more typed builtins** (chosen for now): small local models emit structured tool calls far more reliably than compilable Go/JS, so expanding the pure-Go builtin surface (shipped: `find_files`, `read_lines`; plus allowlisted `awk`/`sed`/`jq`) covers the need with no new runtime. revisit Yaegi only as a user-plugin extension mechanism, or Deno if model-authored scripting becomes a hard requirement.
- [ ] `[inarid]` `[medium]` **memory-budget throttle (wire the scheduler)** - the `internal/scheduler` semaphore (Acquire/Release, budget from `config.json` `memory_budget_mb`) exists as a library but is wired to nothing; the S10 cleanup (2026-07-17) removed the dead daemon plumbing that created it without using it. scope: Acquire before a stream's generation and Release after, keyed by model tier, so concurrent sessions respect the configured memory budget instead of streaming unthrottled.
- [ ] `[inarit/inarid]` `[hard]` **long-term task planning from high-level prompts** - decompose a high-level user goal into a tracked, multi-step plan that the session executes and checks off. exploratory; no concrete entry point yet, so parked here until the shape is clearer.
- [ ] `[inarit]` `[medium]` **pre-send prompt optimisation (autocorrect)** - the prompt-optimisation half of the pre-send intercept layer (its security/validation half is **pre-send message intercept**, near-term, which stays daemon-side as the authoritative gate). client-side because it is a UX affordance, not a security control: before the message leaves the TUI, lightly rewrite it to improve model accuracy - fix obvious typos, expand terse fragments, normalise formatting - without altering intent. the user must preview the rewrite in the input box and accept or reject it before send (or toggle the feature off); it never silently distorts what the user asked.
- [ ] `[inarit]` `[hard]` **chat viewport character selection** - build on line selection to add character-level precision: within the selected rows, re-parse ANSI sequences to locate byte ranges and inject highlight styles mid-sequence, so a drag can start and end partway through a line. builds on the shipped line-selection work (see Done). parked as exploratory: whole-row selection already covers the common copy case, so the mid-sequence ANSI re-parsing is not yet worth the complexity.
- [ ] `[inarid]` `[medium]` **MCP tool-call dispatch** - `internal/mcp/host.go` `Call()` is a TODO stub; audit logging exists but actual JSON-RPC dispatch over stdio is not implemented. prerequisite for the MCP integration work below (was near-term for M4; parked here until the wider MCP direction settles).
- [ ] `[inarid]` `[medium]` **MCP filesystem connector (layer 3)** — once the tool-call loop exists, replace built-in tools with `@modelcontextprotocol/server-filesystem` spawned via mcp-go. this is a natural extension of the MCP integration work below.
- [ ] `[inarid]` `[medium]` **destructive action prevention (§8.2)**: cwd enforcement (`sandboxPath` in `internal/ipc/tools.go`) and a tool-call loop cap (`maxToolRounds = 10` in `internal/ipc/stream.go`) are shipped, alongside per-call size caps; remaining scope is a true file-op-count cap and dry-run previews for caution-tier tool-calls. risk-tiered auto-approval is done (safe builtins auto-execute, allowlisted `execute_shell_command` binaries auto-execute, unlisted ones confirm)
- [ ] `[inarid]` `[hard]` multiple models per session — allow attaching different models to a single session for collaborative discussions and task execution
- [ ] `[inarid]` `[hard]` MCP integration — replace `internal/mcp` with `github.com/mark3labs/mcp-go`; connectors (Linear, Slack, Google Drive, etc.) configured via `config.json`
- [ ] `[inarid]` `[medium]` **prompt-based tool calling** — for models without native function-calling support, inject tool definitions as plain text into the system prompt and set `format: "json"`; inarid parses the JSON response to detect tool calls. select mode via session config or auto-detect from model name. makes layer 2 work on any instruction-following model (hermes-3-pro, qwen3-coder, etc.)
- [ ] `[inarid]` `[medium]` **provider abstraction** — the `Provider` interface already exists (`internal/provider/provider.go`: Chat, ChatStream, LoadModel, UnloadModel, ListModels, ListRunning, Ping) and inarid's core already talks only to it. remaining work is a second concrete provider (vLLM, LM Studio, llama.cpp server, or a cloud API) selected via `provider` in `config.json`; overlaps with the local endpoint profiles item above.
- [ ] `[inarid]` `[hard]` multi-model routing — a runner agent classifies intent and either handles the request itself or escalates to the thinker agent
- [ ] `[inarid]` `[hard]` **context caching / compression / optimisation** — investigate strategies to reduce prompt size and improve response speed: KV-cache reuse across turns, selective message eviction, rolling summary compression, and prefix caching at the provider level; goal is lower latency and higher effective context utilisation without degrading response quality
- [ ] `[inarid]` `[hard]` **vector store / RAG context** — replace or augment flat JSON session storage with a semantic retrieval layer. progression: (1) sqlite as structured store; (2) sqlite-vec (sqlite vector extension) for local embeddings — single file, no external service, fits the Go daemon cleanly; (3) full RAG pipeline with chunking, a local embedding model (~100MB), and ranked context injection. at query time, the user message is embedded and the top-k semantically similar chunks are injected into the prompt rather than the full history dump. benefit: small models see only relevant context, reducing token pressure and improving response quality. a global "master context" store (outside any cwd) could be maintained alongside per-session history, giving all sessions access to persistent personal or cross-project knowledge.
- [ ] `[inarid]` `[hard]` **task difficulty/effort classification** — investigate how to define and score task difficulty, complexity, and effort (e.g. token count, tool-call depth, reasoning hops) so inarid can automatically select the appropriate model tier (runner vs. thinker) rather than relying on manual session config
- [ ] `[inarid]` `[medium]` consider adding vLLM as an alternative backend to Ollama — vLLM is OpenAI-compatible and may offer better throughput on CUDA hardware; evaluate alongside the local endpoint profiles item as a concrete second backend candidate
- [ ] `[inarid]` `[medium]` consider exposing Ollama as an MCP server so other models — local or cloud — can be invoked as tools by the default model (`gemma4:e2b`); this lets the thinker agent delegate sub-tasks to specialised models (e.g. a coding runner) via the existing MCP tool-call loop rather than requiring a separate session

### Done
- [x] `[inarit/inarid]` `[easy]` **generate SPEC §6.1 from `curated.go` (killed the dual-maintenance drift)** - `CuratedModels` (`tui/views/curated.go`) is now the single source; the §6.1 tables are generated from it by `RenderCuratedTables` (`tui/views/curated_table.go`) between `<!-- BEGIN/END generated -->` markers. `make curated-sync` rewrites the region; `TestCuratedTablesInSync` (under `make test`) fails if it is left stale, comparing the marker region byte-for-byte against the rendered output. the renderer left-aligns and pads every column to its widest cell (repo table convention), so a hand-edit to the table alone can never silently diverge from the source. also refreshed the now-stale "update both together" comments in `curated.go` and the §6.2 findings note. verified: go build, go vet, `go test ./...`; `-update-curated` regenerates and the plain run then passes in-sync.
- [x] `[inarit/inarid]` `[medium]` **rebuild the curated model list for local LLMs** - rebuilt `CuratedModels` (`tui/views/curated.go`) and SPEC §6.1 against the live ollama registry (every tag HEAD-checked, 200 vs 404), which caught two dead tags the "live" list still shipped: `gemma4:27b` (404) -> `qwen3.6:27b` (verified; family already run locally as the coding variant) and `phi-4:14b` (404 hyphen typo) -> `phi4:14b`; added `gemma4:12b` (local, measured 7.6GB, was missing); set the empty `models.runner` config default to `gemma4:e4b` (its §6.1 note is "fast routing", the runner role). **automation investigated + delivered:** the cheap tag-resolution check (registry HEAD, no download) shipped as `inari try --check`, and the full run + tool-call test as `inari try <tag>` / `inari doctor --models`; SPEC §6.2 records the findings. the remaining drift-elimination piece (generate §6.1 from `curated.go` so the two cannot disagree) is split into its own near-term item. see also the `doctor --models` and `inari try` Done entries.
- [x] `[inari]` `[medium]` **`inari try <tag>`: discover + locally test new candidate models** - the model-shopping counterpart to `doctor --models` (which is a preflight over already-configured models). `inari try <tag>` (1) resolves the tag against the ollama registry with a cheap HEAD (`registryManifestURL` + `modelResolves` in `cmd/inari/try.go`, no auth/download; library models get the `library/` prefix, missing `:tag` defaults to latest, `user/model` used as-is), (2) pulls it if absent (headless `client.PullModel`), then (3) drives it through the exact same streaming tool-call smoke test `doctor --models` uses (`verifyOne`: fixture cwd + `client.ChatStream` + audit-log `tool.call` check, not reply-text). `--check` does only step 1 - the cheap dead-tag signal the curated-model-list rebuild wants, no gigabyte pull. exits non-zero if the tag 404s, the pull fails, or the model runs but invokes no tool. **shares the corrected mechanic from the doctor item:** `session.chat` has no tool loop, so verification drives the stream path. tests: `TestRegistryManifestURL` (tag->URL mapping). verified live: `try --check llama3.2:3b` -> resolves 200 exit 0; `try --check qwen3-nano` -> 404 exit 1; `try gemma4:e2b` -> resolves + already-local + `replied + called list_dir`, exit 0.
- [x] `[inarid]` `[easy]` **background the daemon by default; `-f`/`--foreground` to attach** - `inari daemon` now self-detaches into the background and returns (the common case, so `inari stop` manages it); `-f`/`--foreground` runs it attached with "ctrl+c to quit". the old `--background` flag was a misnomer (it never detached - it only wrote a pid file and skipped the ctrl+c line, the internal forked-worker marker); renamed to `--child`, set only by the parent when it forks. extracted `forkDaemon` + `refuseIfRunning` in `process.go` as the shared fork path (`cmdStart`, the default `daemon`, and `ensureDaemon` all use it, dropping the duplicated fork in `chat.go`). `runDaemon`'s `background` param became `attached` and it now **always** writes the pid file, so a foreground daemon is stoppable via `inari stop` too (it previously was not - the stated side benefit). also fixed a latent bug the change exposed: `forkDaemon` waited on the compiled-in `defaultSocket`, so a custom `socket` in config.json produced a false "did not come up within 5s" even though the daemon started; it now loads config and waits on the actual socket. **CLI-contract change:** `make run-daemon` moved from `go run ./cmd/inari daemon` to `daemon -f` (a supervisor expecting `daemon` to block must use `-f`); noted in help + README. verified via go vet, go build, `go test ./...`, and a live isolated-HOME lifecycle run: default detaches + returns (pid written, socket up), a second daemon is refused, `inari stop` stops it, and `-f`/`--foreground` block attached.
- [x] `[inari]` `[medium]` **`inari doctor --models`: verify pulled models actually work** - doctor previously only checked a configured tag was *present* (`modelPresent` name-match); presence is not function. `inari doctor --models` now drives each *configured, locally-present* model (thinker, and runner when set) through one real streaming turn against a temp fixture cwd and passes only if it (1) replies error-free and (2) actually invokes a tool, confirmed by scanning the audit log for a `tool.call` entry for that session - not by parsing the reply text (a model can narrate a tool result without one having run). **premise correction (the item as written could not work):** it specified reusing `inari chat --new ... --message`, but `session.chat` (`handleSessionChat`) is a plain `provider.Chat` round-trip with **no tools declared and no tool-call loop** - tool-calling lives only in `session.stream`/`handleStream` (and only when the session has a cwd). so verification drives `client.ChatStream` against the fixture cwd; a "list the files" prompt triggers `list_dir`, a safe builtin that auto-executes with no approval round-trip. safe-tool auto-exec means no `tool_request` is emitted; any tool that does ask is auto-denied so a model reaching for shell can neither hang the check nor run commands. gated behind `--models` so plain `inari doctor` stays a fast preflight (the run-test loads + runs each model). the fixture session is deleted after each turn; the audit scan reads only bytes appended during the turn (offset-based). **also:** extracted `resolveDataDir`/`auditLogPath` in `paths.go` as the single source both the daemon and doctor use, so they cannot disagree on where the audit log lives (daemon.go now calls them). exits non-zero on any configured+present model that fails. verified via go vet, go build, `go test ./...` (new `TestAuditLogPath`, `TestSessionToolCalled`), and a live `doctor --models` run: `verify thinker: gemma4:e2b (replied + called list_dir)`, exit 0.
- [x] `[inarit]` `[medium]` **rename `Agents` view/files to `Sessions`** - a session (cwd, model assignment, history, tags) is not an autonomous agent; it only acts when the user sends a message and waits for a reply (see §2.2). renamed the mislabeled UI concept throughout: 11 `tui/views` files (`agents.go`/`agents_data.go`/`agents_mutations.go`/`agents_input.go`/`agents_table.go`/`agents_view.go`/`agents_cmds.go`/`agents_msgs.go`/`agents_fmt.go` plus two `_test.go` files, via `git mv`) to their `sessions_*.go` equivalents; the `Agents` struct/`NewAgents` constructor to `Sessions`/`NewSessions`; message types `OpenAgentsMsg`/`CloseAgentsModalMsg`/`RefreshAgentsMsg`/`BackToAgentsMsg` to their `Sessions` equivalents; `tui/model.go` wiring (`viewAgents`, `overlayAgents`, `showAgents`, `agentsModalMarginW`/`H`, the `m.agents` field); `pickAgentName`/`agentsStyle`/`agentsHints`; and `internal/ipc`'s `defaultNewAgentModel` const to `defaultNewSessionModel`. **decision: no back-compat alias** - the `/agents` chat command was renamed straight to `/sessions` with no `/agent` shorthand kept, since this is a single-user personal tool and permanently maintaining two command names costs more than the one-time muscle-memory adjustment. updated README's hotkey heading and SPEC's living Architecture/command-surface docs (§5.2 view table, hotkeys, chat command table) to match; historical Done-log entries describing the earlier Herd->Agents rename were deliberately left as-is (point-in-time record, matching this file's existing convention of not retrofitting past entries when a name changes again later). verified via go vet, go build, `go test -race ./...` (all packages green, zero behaviour change).
- [x] `[inari]` `[medium]` **homebrew release for the inari binary** - a tag-triggered `.github/workflows/release.yml` runs goreleaser `release --clean` on a `v*` push to build the binaries (darwin amd64/arm64 + linux amd64), archives, checksums, and the GitHub release, using only the built-in `GITHUB_TOKEN` (mirrors `mirageglobe/scout`). **decision: the "scout way"** - no goreleaser tap-push automation; `Formula/inari.rb` is hand-maintained in `mirageglobe/homebrew-tap` alongside `Formula/scout.rb`, so no cross-repo PAT is needed (a goreleaser `brews`/cask block was prototyped then dropped in favour of matching scout). `brew install mirageglobe/tap/inari` installs from that hand-written formula. **also fixed a latent release-blocking bug:** `builds.main` was `./cmd/inari/main.go`, a single file, which stopped compiling once the arch-review split `main.go` into sibling files (chat/daemon/doctor/process/paths/tui) - goreleaser built only `main.go` and failed with `# command-line-arguments`; changed to the package path `./cmd/inari`. verified with `goreleaser release --snapshot --clean` (builds all targets, archives, checksums). **remaining to publish (operator steps, not code):** push a `v*` tag to cut the GitHub release, then update the tap's `Formula/inari.rb` with the new version + per-arch url + sha256 (from the release `checksums.txt`); no repo secret is needed (the "scout way"). the stale `v0.2.0`/`Apache-2.0` formula there needs this refresh.
- [x] `[inari]` `[easy]` **headless session-create (`inari chat --new`)** - `inari chat --new [--name N] [--model M] [--cwd P] --message ...` creates a persisted session, optionally overrides its default model (`AssignModel`), sends one turn via `session.chat`, and prints the reply: a self-contained headless one-liner needing no pre-existing session (completes the headless-mode thread). `--new` and `--session` are mutually exclusive and exactly one is required (`resolveTarget`); `--name` defaults to a generated `headless-*` (`session.create` requires a non-empty name); `--model` defaults to the daemon's configured model (`models.thinker`, which must be pulled). the new session id is echoed to stderr so stdout stays the reply only. **decision: persist** (no delete path) rather than ephemeral, matching the normal session lifecycle and keeping the change minimal; a `--rm` cleanup flag is a possible follow-up. verified via go vet, go build, `go test ./cmd/inari` (`TestResolveTarget`, `TestNewSessionName`) and a live end-to-end create+assign+chat against gemma4:e2b (plus the mutual-exclusion and neither-flag error paths).
- [x] `[inarit]` `[medium]` **streaming hot-path profiling + render cache (P2/P4)** - profiled the token-stream render path measure-first (per the audit decision record) via `BenchmarkStreamTurn` (`tui/views/chat_render_bench_test.go`), a synthetic streaming turn swept over session length, plus live `/api/ps` and `/api/show` round-trip timings. **P2 confirmed dominant:** the old `onToken` re-`strings.Join`ed the whole display scrollback and `ansi.Hardwrap`ped the entire string every token, so per-turn render cost scaled linearly with history - measured 0.89ms/turn at 0 lines up to 72ms/turn and 100MB allocated at 1000 lines for one 200-token reply, all on the single-threaded bubbletea loop. fix: cache the wrapped scrollback base for the lifetime of a stream (`streamBaseWrapped`, keyed on viewport width + `len(display)`, dropped in `onDone`) and re-wrap only the in-progress line per token; the hardwrap moved from `setViewportContent` into `viewportContent` so every render site shares it. result: 3.2x-15.1x faster (72ms -> 4.8ms at 1000 lines) and 2.7x-5.5x fewer bytes, byte-identical output (`TestStreamRenderCacheMatches`, `TestStreamRenderRefreshesOnResize`). **P4 accepted:** with the base cached, `streamBuf += token` (~1KB/reply) is negligible next to the residual base+partial concat; a `strings.Builder` fights the value-receiver `Chat` for no measured gain. **P5/P3 measured:** `/api/ps` ~2ms and `/api/show` ~7-29ms per *cold* call - immaterial vs multi-second inference. `/api/ps` (P5) is left as it drives the loading-vs-thinking indicator. `/api/show` (P3) needs no work: it was already memoised per model in the arch-review perf batch (531359a, `Server.ctxLen`/`modelContextLength`), so the streaming path fetches it once per model, not per turn. verified via go vet, go build, `go test ./tui/views` (race) and the before/after benchmark.
- [x] `[inari]` `[hard]` **architecture review + refactor** - whole-app performance + design pass with no behaviour regressions, delivered audit-first then landed as small separately-reviewable PRs rather than a big-bang change. the audit is the dated decision record [docs/architecture-audit.md](docs/architecture-audit.md) (2026-07-16); refactors shipped incrementally across PRs #76-#83 (2026-07-16 to 2026-07-18): `NewServer` config/options struct (S1), dispatch/client RPC dedup (S3-S5), config cleanups (S9), dead scheduler/mcpHost dep + plumbing removal (S10), tui modal-box helper + dead `View()` retirement (S12-S13), overlay-priority + broadcast centralisation (S14-S15), and the perf batch (P1, P3). two threads were deliberately carried forward as their own tracked items rather than forced into this umbrella: the second-provider seam (deferred; homed in the "provider abstraction" Idea) and streaming-render profiling (measure-first; the P2/P4/P5 Near-term item). closed as the umbrella scope is complete and its remaining work is homed elsewhere.
- [x] `[inari]` `[easy]` **headless mode (`inari chat`)** - a non-interactive `inari chat --session <id> --message <text>` subcommand drives one turn against an existing session and prints the assistant reply, no TUI. `--message -` reads from stdin; `--json` wraps the reply as `{"reply": ...}`. it parses its own flags ahead of the shared parser (`main.go` early switch), ensures a background daemon is up (`ensureDaemon`/`waitForSocket`, `chat.go`), and calls the existing `session.chat` RPC via `ipc.Client.Chat`. **decision:** routed through the non-streaming `session.chat` path (a plain `provider.Chat` round-trip with no tool-call loop), so headless is deterministic and can never block on a `[y/n]` approval - the intended shape for scripting and tests; a streaming/tool-enabled headless mode is a separate follow-up. `--session` is required and must already exist (no headless session-create yet). retagged `[medium]` -> `[easy]`: the client layer (`CreateSession`/`AssignModel`/`Chat`) was already fully wired, so the change is one new file plus one `main.go` case, zero daemon/protocol work. verified via go vet, go build, `go test ./cmd/inari` (`TestResolveMessage`, `TestPrintReply`) and a live end-to-end round-trip against gemma4:e2b (plain reply, stdin+`--json`, plus the missing-`--session` and unknown-session error paths).
- [x] `[inarit]` `[medium]` **modal overlays for read-only views** - `/help`, `/describe`, `/logs`, and the `/tools` panel now render as centred pop-up modals over the current view instead of full-screen view swaps (describe/logs) or an inline footer hint (tools). describe/logs were dropped from the `view` enum and are now opened via `showDescribe`/`showLogs` booleans (`model.go`), routed through `updateModal`, and rendered by new `RenderModal` methods (`describe_render.go`, `logs.go`) mirroring the existing model-selector/agents modal pattern; the tools panel became a centred box in the chat body (`chat.toolsModal`, `chat_view.go`) that replaces the transcript while open. the view underneath (chat or agents) stays as `current` and is revealed on close. verified via go vet, go build, test suite (`TestModalsCloseOnBothQAndEsc`, `TestOverlayReturnsToViewUnderneath`, `TestModalRendersNonEmpty`, `TestToolsModalClosesOnQAndEsc`).
- [x] `[inarit]` `[easy]` **uniform modal exit (q/esc -> underlying view)** - every pop-up modal (help, describe, logs, tools, model selector, theme picker) now closes on both `q` and `esc` and returns to the view it was opened from. previously the theme picker and help closed on `esc` only, and describe/logs were full-screen views whose `esc` forced a return to agents regardless of origin. close handling is centralised per modal in `updateModal`/`updateKeys` (root) and in the chat key handler for the tools modal, which captures q/esc early since it overlays the focused input; describe suppresses the close while its context editor is focused (esc exits edit mode first). verified via go vet, go build, test suite (`TestModalsCloseOnBothQAndEsc` exercises both keys across all four root overlays).
- [x] `[inari]` `[medium]` **`!` bash passthrough** - a `!`-prefixed line in the chat input runs the remainder as a real shell command in the session cwd, bypassing the model; its output streams into the transcript and is recorded in history so the next model turn sees it. path: TUI parses `!` on submit (`chat_keys.go`, user-input path only so the model can never author a `!`) -> client `Shell` -> daemon `session.shell` (`handleSessionShell`) -> `runUserShell`. **decision (sh -c vs word-split):** chose a real `sh -c` shell so pipes/globs/redirects work, matching the Claude-CLI `!` convention. this does NOT reopen the §8.3 injection surface: that rule's `exec.Command(binary, args...)` guard exists because the *model* authors `execute_shell_command`; a `!` line is *user-authored* at the user's own terminal (no privilege escalation, they already have a shell) and is parsed only on the user-submit path, never on model output. consequently `!` also bypasses the allowlist (the user typing the command is the approval) while keeping the cwd lock, 30s timeout, and 64KB cap; requires a cwd. verified via go vet, go build, race suite (`TestRunUserShellPipes`/`Cwd`/`Truncate`, `TestSessionShellRecordsHistory`/`RequiresCwd`, `TestOnShellRecordsOutputAndHistory`/`ErrorSurfacesStatus`).
- [x] `[inarid]` `[issue]` `[easy]` **mark prior listings stale on `/cwd`** - `session.setcwd` rebuilt the system prompt for the new cwd but left the previous cwd's tool results (file listings, file contents) in conversation history. a model would then regurgitate the stale listing for the new directory rather than re-running tools: real repro had cwd correctly `.../mirageglobe` while the model listed `.../ai-inari`'s files (its earlier list_dir result, still in history). the #69 guard sits in the system prompt (position 0) and did not stop this: measured, the stale listing at a later history position won ~3/5 at temperature 1.0. the fallback (#70) does not catch it either since a regurgitated listing is not `name(args)` syntax. fix (`handleSessionSetCwd`): after rebuilding the prompt, append a `system`-role marker to history naming the new cwd and flagging earlier listings stale; recency next to the stale results is what makes the model re-call the tool (marker in history 5/5 vs system role 5/5, tool role 3/5, user role would render as a fake turn). only added when the session already has conversation; the TUI's `rebuildDisplay` renders only user/assistant/error, so a system marker is invisible to the user while still reaching the model via `ChatHistory()`. verified via go vet, go build, race suite (`TestSetCwdInjectsStaleMarker`, `TestSetCwdNoMarkerWithoutHistory`).
- [x] `[inarid]` `[issue]` `[easy]` **surface tool output on empty final answer** - once the guard + fallback made tools actually fire, a latent gap surfaced: some models (measured: gemma4:e2b, deterministically over repeated runs) return an empty final answer after a tool result, treating the tool output as the answer, so inarid rendered a blank reply. fix (`handleStream`): a turn-level `turnToolOutput` accumulates each executed tool's result; when the final round has empty content and no tool call, that accumulated output is streamed to the UI (tool rounds are otherwise silent) and persisted as the assistant reply. diagnosed by replaying inarid's full multi-round loop against gemma4:e2b via `/api/chat`: round 0 emitted a native `list_dir` call, round 1 returned empty content 3/3 times; a manual "present the result" nudge produced the listing, confirming the model just will not self-verbalise after a tool. chose harness-surfacing over an injected nudge: deterministic, no extra model round, model-agnostic. verified via go vet, go build, race suite (`TestStreamSurfacesToolOutputOnEmptyFinal`).
- [x] `[inarid]` `[hard]` **prompt-based tool-call fallback** - `handleStream` only dispatched native `chunk.Message.ToolCalls`; when a model instead emitted a call as text (`list_dir{path:"."}`, `read_file(path='x')`) inarid treated it as a plain reply and printed it. small models at temperature 1.0 (e.g. gemma4:e2b) do this stochastically, and one text turn few-shots the rest of the session into narration, so the system-prompt guard alone was not a hard guarantee. fix (`tools_textfallback.go`, `parseTextToolCall`): when a round returns content and no native tool_calls, scan the content for a `name(...)`/`name{...}` invocation; if the name is one of the tools offered this stream (`knownTools`, empty when no cwd) and it carries at least one arg, dispatch it through the existing tool-call path so `execTool`'s approval + cwd sandbox still apply. persisting the structured call rather than the raw text also heals the history, so the model stops few-shotting its own narration. scoped decisions: fenced code blocks (` ```bash ls``` `) are NOT parsed (an illustrative command is ambiguous versus intent); a bare prose mention with no args is ignored (filters "use list_dir to browse"). known limitation: the raw text was already streamed to the UI this turn, so the junk invocation is visible before the real answer arrives in the next round; the stored history stays clean. verified via go vet, go build, race suite (`TestParseTextToolCall` table incl. negative cases, `TestParseTextToolCallEmptyKnown`).
- [x] `[inarid]` `[issue]` `[medium]` **tool-call system prompt guard** - `buildCWDSystemPrompt` (`context.go`) previously injected the file tree followed by a `name(args): ...` prose tool list. two failure modes, both diagnosed by replaying a real poisoned session against gemma4:e2b via `/api/chat`: (1) the model answered file/dir questions straight from the injected tree in text (never calling a tool), and (2) the call-shaped prose modelled a text invocation the model reproduced; once one assistant turn was text, the model few-shot off its own history and stopped emitting native `tool_calls` for the rest of the session (printed ` ```bash ls``` ` instead of running it). fix: keep the tree but frame it as a stale orientation snapshot with an explicit "never answer from it or memory, always call a tool" directive, and list tool names plainly without the `(args)` signatures (the native `filesystemTools` schema is the real declaration). measured on the exact failure sequence: native tool-call rate 0/3 (old prompt) -> 3/3 (new prompt). limitation: gemma4:e2b runs at temperature 1.0, so this shifts probability strongly but is not a per-run guarantee; the hard guarantee is the prompt-based tool-call fallback (see near-term). verified via go vet, go build, race suite (`TestBuildCWDSystemPromptGuard`).
- [x] `[inarid]` `[medium]` **local project-scoped configuration** - a per-project overlay read from `.inari/config.json` in the session's working directory (`config.LoadProject`, `internal/config/project.go`), applied at session creation. **restricted overlay by decision:** only two fields are honored - `context.system_prompt` and `exclude_dirs` - and the `ProjectConfig` struct deliberately omits every infra/security field, so a project file cannot set socket, endpoints, provider, `shell.allowlist`, models, data_dir, or ollama tuning. rationale (merge-precedence decision that had blocked the item): a cwd can be an untrusted cloned repo, so letting `.inari/config.json` widen the shell allowlist or redirect the inference backend is an unacceptable blast radius; the overlay is limited to presentation/context, matching the roadmap's own "custom prompts, file exclusions" scope. precedence: the project `system_prompt` **replaces** the global `context.system_prompt` in the prepend slot (more-specific wins), applied in `handleSessionCreate` (`dispatch_session.go`); the base cwd file-tree/AGENTS.md context is always retained. `exclude_dirs` are merged into the built-in file-tree skip set for that session only (`buildFileTree` takes an `extraSkip`, read via `buildCWDSystemPrompt` so both `session.create` and `session.setcwd` honor it). a missing or malformed overlay yields the zero value (no overlay), never an error. verified via go vet, go build, race suite (`TestLoadProjectHonoredFields`/`Missing`/`Malformed`; `TestSessionCreateProjectPromptOverridesGlobal`, `TestSessionCreateProjectExcludeDirs`, `TestSessionCreateGlobalPromptWithoutOverlay`).
- [x] `[inarit]` `[issue]` `[medium]` **focus-aware key suppression** - a general guard in the root key router (`updateKeys`, `model_input.go`): when the active view is capturing text input (`activeViewInputFocused`: chat message box, agents filter, describe editor) and no global overlay is open, unmodified keys fall through to the view instead of matching a global bare-key hotkey; modifier chords (ctrl/alt) still work (`isBareKey`). replaces the ad-hoc `?`/`t` workaround so a future bare-key binding cannot shadow typing, and fixes two latent cases: typing `?` while editing describe context, and `esc` clearing the agents filter rather than leaving the view. `Agents.Filtering()` added as the focus accessor. verified via go vet, go build, race suite (`TestIsBareKey`, `TestFocusAwareKeySuppression`, the first tests in the `tui` package).
- [x] `[inarit/inarid]` `[issue]` `[medium]` **model swap returns ollama error for some models** - swapping to certain models (e.g. deepseek) produced an ollama error while gemma4/qwen worked; a model-name mismatch or missing pull surfaced as an opaque error. fixed: `session.assign` (`internal/ipc/dispatch.go`) validates the model against the provider's `ListModels` and rejects an unmatched name with a clear error instead of persisting silently; covered by `TestSessionAssign` (`internal/ipc/ipc_test.go`).
- [x] `[inarid]` `[medium]` **global context configuration** - a `config.json` `context.system_prompt` field is prepended to every new session's system prompt at creation (`handleSessionCreate`, `dispatch_session.go`), composing with the session's base prompt (the default concise-response prompt, or the cwd file-tree/AGENTS.md context) rather than replacing it. threaded from config through `NewServer` (`globalSystemPrompt`). default model settings and context parameters already ship via the `models`/`ollama` blocks, so the global system prompt is the new addition here. the per-project `.inari/config.json` override layer shipped separately (see Done: local project-scoped configuration). verified via go vet, go build, race suite (`TestSessionCreateGlobalPrompt`: global prompt prepended, base prompt retained).
- [x] `[inarid/inarit]` `[medium]` **pre-send message intercept (short-circuit + warn)** - two low-risk halves. daemon-side (`handleStream`, `stream.go`): empty/low-effort input with no alphanumeric content (e.g. `?`, whitespace) is short-circuited with a canned local reply, skipping the model round-trip. client-side (`chat_secrets.go`, non-blocking): an outgoing message that looks like it carries a secret (known token prefixes, `key=value` assignments) shows a soft `[warn]` in the status line but is still sent. by design this does NOT hard-block or content-filter natural-language messages (that would false-positive and duplicate the tool-call safety tiers, which remain the real execution gate); the destructive-shell-text filter from the original item was intentionally dropped. verified via go vet, go build, race suite (`TestStreamShortCircuitsLowEffort`, `TestLooksLikeSecret`).
- [x] `[inarid/inarit]` `[medium]` **test coverage for untested packages** - added the `version` package test and the first `tui/views` render-seam tests (`TestChatViewRenders`, `TestAgentsViewRenders`: View renders non-empty without panicking after a window-size), plus the feature tests landed alongside this batch. `mcp` and `cmd/inari` remain uncovered (low value: `mcp.Call` is a TODO stub, `cmd/inari` is thin wiring) and are left for a future pass. verified via go vet, go build, `go test -race`.
- [x] `[inarid]` `[medium]` **ollama runtime env tuning** - a `config.json` `ollama` block exposes `keep_alive`, `max_loaded_models`, and `num_parallel`. `keep_alive` is actionable by inarid: the ollama client (`SetKeepAlive`, `client.go`) attaches it to every chat request via a `chatBody` wrapper so the provider type stays backend-agnostic, and the daemon wires it from config. `max_loaded_models`/`num_parallel` are server-start settings inarid cannot set on an external `ollama serve`, so `inari doctor` (`reportOllamaTuning`, `doctor.go`) surfaces them as the host env vars OLLAMA_MAX_LOADED_MODELS / OLLAMA_NUM_PARALLEL to export (the roadmap's own fallback for the no-lifecycle case). verified via go vet, go build, test suite (`TestKeepAliveInChatBody`: keep_alive present when set, omitted when unset).
- [x] `[inarid/inarit]` `[medium]` **role-based model assignment** - a session records a task role via `session.setrole` (`dispatch_session.go`, validates general/coding, empty clears; `SessionInfo.Role`). a `/role <general|coding>` chat command sets the role and defaults the session to the recommended curated model for that role at the detected hardware tier (`recommendedModel`, `curated.go`), assigning it when already pulled and otherwise naming it to pull. verified via go vet, go build, race suite (`TestSessionSetRole`: valid/invalid/clear/unknown; `TestRecommendedModel`: role+tier lookup; `TestEveryCommandDispatches` confirms `/role` dispatches).
- [x] `[inarit]` `[medium]` **split remaining oversized files** - the three residual `chat*` files were split by responsibility, no behaviour change: `chat.go` 262 -> 132 (Update dispatcher to `chat_update.go`, styles + message types to `chat_types.go`), `chat_render.go` 189 -> 111 (`View` + `inputPrompt` to `chat_view.go`), `chat_keys.go` 171 -> 138 (message-send path to `chat_send.go`). `agents.go`/`tools_exec.go` remain unsplit (marginal, ~6-8 lines over, low value). verified via go vet, go build, `go test -race` (all `tui/views` tests unchanged and green).
- [x] `[inarit/inarid]` `[medium]` **session tagging** - free-form labels for grouping and filtering. sessions carry a `Tags []string` field (`session.go`, `ToggleTag` keeps it sorted/deduped); a `/tag <label>` chat command toggles a label via the `session.tag` RPC (`dispatch_session.go`), which returns the updated `SessionInfo`. the agents view shows tags after the session name (`jade fox [work urgent]`) and includes them in the plain-text filter (`applyFilter`, `agents_data.go`), building on the shipped agents search. verified via go vet, go build, race suite (`TestSessionTag`: toggle on/off, empty-tag rejected; `TestEveryCommandDispatches` confirms `/tag` dispatches).
- [x] `[inarid/inarit]` `[medium]` **per-session context window override (num_ctx UI)** - a `/numctx [tokens|auto]` chat command lets the user view and adjust `num_ctx` per session from the chat view: no arg shows the effective window, a number sets it, `0`/`auto` clears it. the session stores `NumCtxOverride` (`session.go`, `SetNumCtx`); `session.setnumctx` RPC (`dispatch_session.go`) persists it; `handleStream` (`stream.go`) prefers the override over the model-derived default when building the request options, and the footer shows the effective window (`effectiveNumCtx`, threaded through `SelectModelMsg` -> `NewChat` so it is correct on open). the remaining per-tier defaults (higher for thinker, lower for sensor) are deferred until a session-to-tier mapping exists. verified via go vet, go build, `go test -race` (`TestSessionSetNumCtx`, `TestStreamUsesNumCtxOverride`, `TestEffectiveNumCtx`, `TestShouldAutoCompact`).
- [x] `[inarid]` `[medium]` **local server endpoint profiles** - named backend profiles in `config.json` under `endpoints` (each with `base_url`, optional `api_key`, optional `headers`), selected via a top-level `provider` field; empty `provider` falls back to the legacy single `ollama_base_url`, and a partial profile inherits `ollama_base_url`. `Config.ActiveEndpoint` (`config.go`) resolves the active profile and reports whether a named one matched, so the daemon (`cmd/inari/daemon.go`) can warn on a dangling `provider`. the ollama client gained `NewClientWithAuth` (`client.go`) which injects an `Authorization: Bearer` token and any static headers via a `RoundTripper`, so an authenticated or non-default local backend (LM Studio, llama.cpp, a cloud proxy) works without touching each request site. prerequisite for the provider-abstraction idea; a second concrete provider is still deferred. verified via go vet, go build, race suite (`TestActiveEndpoint`, `TestNewClientWithAuth`).
- [x] `[inarit/inarid]` `[medium]` **rename session** - a `/rename <name>` chat command renames an existing session in place. inarit's `handleSlashCommand` (`chat_dispatch.go`) prefix-matches `/rename` (mirroring `/cwd`), calls `client.Rename`, and on the `renameResultMsg` (`onRename`, `chat_msgs.go`) adopts the new name, refreshes the input placeholder, and rebuilds the display so assistant lines re-render under the new name. inarid's `session.rename` RPC (`dispatch_session.go`) validates a non-empty name, updates the stored name + `UpdatedAt`, persists, and returns the updated `SessionInfo`; model, history, and cwd are preserved. mirrors the `session.setcwd`/`session.setcontext` handler pattern. the agents view picks up the new name on its next list refresh. verified via go vet, go build, race suite (`TestSessionRename`: valid rename preserves model, empty name rejected, unknown session rejected; `TestEveryCommandDispatches` confirms `/rename` dispatches).
- [x] `[inarit]` `[medium]` **auto-compression threshold** - after a streamed turn, the chat view auto-fires the existing `/compact` summarisation pipeline once the running token estimate reaches `autoCompactFraction` (80%) of the effective context window, without the user asking. `shouldAutoCompact(ctxChars, maxCtx)` (`chat_stream.go`) reuses the footer's estimate (`ctxChars/4`) and effective window (`ipc.DefaultNumCtx(maxCtx)`), returning false when the window is unknown; `onDone` checks it after committing the reply and, when tripped, shows `auto-compacting…` and dispatches `CompactHistory`, whose result flows through the shipped `onCompact` handler. **decision:** implemented TUI-side (retagged from `[inarid]`) rather than daemon-side. the daemon path would need a new post-turn stream frame plus a channel threaded through 5 TUI files to keep the local message list in sync; TUI-side reuses the fully-wired `/compact` path with zero protocol change and is visible to the user, and inarit is the only frontend today. the fraction is a package const; daemon-authoritative, config-driven wiring (so future frontends inherit it) is a follow-up. verified via go vet, go build, test suite (`TestShouldAutoCompact`: unknown window, below, above, far-above threshold).
- [x] `[inarid]` `[medium]` **model loop / EOF prevention** - guards against models stuck in repetitive generation (e.g. `for_for_for...`) that would exhaust the context window and end the stream on an EOF error. two mitigations in `handleStream` (`stream.go`): (1) the ollama request options always carry `repeat_penalty: 1.3`, plus a `num_predict` cap set to the effective `num_ctx` so a single reply cannot generate past the window; (2) a stream-side n-gram tail detector (`hasRepeatedTail`, `loopguard.go`) checks the recent token tail per chunk and, on a short sequence repeating 3+ times, cancels the stream ctx so the existing interrupt path keeps the partial reply and ends the turn cleanly (logged as "loop detected" in verbose mode). the detector uses a minimum period of 3 and requires a letter/digit in the repeating block, so punctuation runs (`-----`) and zero-padded numbers do not trip it. verified via go vet, go build, `go test -race`, and full suite (`TestHasRepeatedTail` covers trips + non-trips; `TestStreamLoopDetection` drives a looping fake provider and asserts clean cancel + partial-reply persistence).
- [x] `[inarit/inarid]` `[medium]` **change session cwd** - a `/cwd <path>` chat command switches an existing session's working directory without recreating it. inarit's `handleSlashCommand` (`chat_dispatch.go`) prefix-matches `/cwd`, calls `client.SetCwd`, and on the `setCwdResultMsg` (`onSetCwd`, `chat_msgs.go`) adopts the new cwd, rebuilds the `[context]` line, and re-renders (builtin tools become available once cwd is set). inarid's `session.setcwd` RPC (`dispatch_session.go`) validates the target is an existing directory (`expandUserPath` handles `~`), updates the stored cwd, and rebuilds the filesystem-context system prompt via the extracted `buildCWDSystemPrompt` helper (`context.go`, now shared with `session.create`) so the new file tree + `AGENTS.md`/`.inari/context.md` project context are injected. because tool calls read `sess.CWD` per-call, the shell/file sandbox re-points to the new path automatically; the RPC returns the updated `SessionInfo` so the footer refreshes. verified via go vet, go build, full test suite (`TestSessionSetCwd`: valid dir updates cwd+prompt, non-directory rejected, unknown session rejected).
- [x] `[inarit]` `[medium]` **split oversized files (agents_view, selector_update)** - two `tui/views` monoliths broken into responsibility-scoped files with no behaviour change: `agents_view.go` 206->131 (table construction + session accessors `rebuildTable`/`SelectedSession`/`DefaultSession`/`usedNames` extracted to `agents_table.go`), `selector_update.go` 187->124 (the `u`/`d`/`enter`/`l` key handling extracted to `selector_keys.go` as a `handleKey` returning a `handled` bool, mirroring the chat view's dispatcher pattern; `Update`'s `KeyMsg` case now delegates and falls through to the table nav on `handled == false`). unused imports trimmed from both. verified via go vet, go build, full test suite (`tui/views` unchanged). remaining residuals tracked in the near-term follow-up.
- [x] `[inarit]` `[medium]` **rolling usage hints on idle** - after `idleHintDelay` (60s) with no activity, the chat status line shows a rotating usage hint prefixed `hint:` (e.g. `try /compact to summarise a long chat`), advancing one entry per further 60s of idleness; every hint names a real binding verified against `chat_keys.go`/`chat_commands.go`. a single root-owned poll (`IdleHintTick`, 20s cadence, started once in `model.Init` and rescheduled in `model_router.updateSystem`) fans out `IdleHintTickMsg` to every chat, so no per-chat `Init` can double-start the loop; each `Chat` computes its own hint from `lastActivity` in `onIdleHintTick` (`tui/views/chat_idle.go`). any keypress (`chat.go` `Update`) or streamed token (`chat_stream.go` `onToken`) resets `lastActivity` and clears the hint. render is the lowest-priority status-line branch (`chat_render.go`), so a pending tool, running tool, recap, or error always wins. verified via go vet, go build, full test suite (`TestOnIdleHintTick` covers pre-delay, first hint, elapsed-advance, wrap-around, and status/waiting suppression).
- [x] `[inarit/inarid]` `[medium]` **interrupt in chat for messages** - `esc` in the chat view aborts an in-flight response while waiting or mid-stream (`chat_keys.go`); `interruptStream` (`chat_helpers.go`) fires a fire-and-forget `session.interrupt` RPC over the shared client connection, decoupled from the dedicated stream conn so there is no concurrent-decoder race. inarid keys a `context.CancelFunc` per session (`Server.streams`, guarded by `streamsMu`; `registerStream`/`unregisterStream`/`interruptStream` in `server.go`); `handleStream` registers a cancellable ctx spanning all tool rounds and passes it to `provider.ChatStream(ctx, ...)`. the `Provider` interface gained a leading `context.Context` on `ChatStream`; the Ollama impl uses `http.NewRequestWithContext` so cancelling ctx aborts the HTTP request mid-generation, and selects on `ctx.Done()` when forwarding chunks so a torn-down consumer never leaks the goroutine. on cancellation `handleStream` keeps the partial reply, persists it, and signals a clean `done` (not an error). `session.interrupt` returns `{"interrupted": bool}`. verified via go vet, go build, `go test -race`, and full suite (`TestStreamInterruptKeepsPartialReply`, `TestInterruptNoActiveStream`); four provider fakes + ollama tests updated for the new signature.
- [x] `[inarid]` `[medium]` **recap/summary when a chat session has been idle for 10+ mins** - a non-destructive `session.recap` RPC returns a one-sentence "where you left off" summary generated with the session's assigned model via `provider.Chat`, gated to fire only when the session has been idle at least `recapIdleThreshold` (10 min, no new messages) and has a real user/assistant exchange; otherwise it returns an empty string. unlike `session.compact` it never touches the stored history. inari's chat view fetches it on open (`fetchRecap`) alongside history/model-context and shows it in the status line as `[recap] ...` (newlines collapsed), skipped when empty or a stream is already underway. verified via go vet, go build, full test suite (`TestSessionRecap`: idle-with-conversation, fresh-session, idle-without-conversation).
- [x] `[inarid/inarit]` `[medium]` **ollama context window detection + display** - inarid reads a model's maximum context window from the `<arch>.context_length` field of Ollama's `/api/show` (new `provider.ModelContextLength`, `ollama.context` RPC) and requests a capped `num_ctx` on each `/api/chat` call via a new `ChatRequest.Options` map: `DefaultNumCtx(max)` returns `min(max, 8192)`, or 0 (omit the option, use Ollama's own default) when the max is unknown. the chat footer surfaces the effective window over the model max, e.g. `ctx 8192/40960`, fetched once on chat open; `DefaultNumCtx` is exported so the TUI shows exactly the window inarid will request. blast-radius note: raising `num_ctx` to 8192 costs more memory than Ollama's default on small setups, so the value is a safe cap, not the model max. the interactive per-session adjust UI and per-tier defaults are a follow-up (see near term). verified via go vet, go build, full test suite (`TestModelContextLength`, `TestModelContextLengthUnknown`, `TestDefaultNumCtx`).
- [x] `[inarit]` `[medium]` **session search and filter in agents view** - `[/]` opens a filter input in the agents view footer; typing narrows the session table live (case-insensitive substring match against session name and model), `[esc]` clears and exits, `[enter]` keeps the filter and returns to hotkey navigation over the narrowed list. implemented by splitting the view's data model into `allSessions` (full backing set) and `sessions` (the filtered list the table and all cursor-indexed actions consume); `applyFilter` recomputes the latter and every optimistic mutation (create/delete/assign/unassign) now updates `allSessions` via a shared `setSessionModel` helper so an active filter stays consistent. `usedNames`/`DefaultSession` read the full set so name generation and the `/chat` default ignore the filter. footer shows `[filter] <query> (N of M)`. verified via go vet, go build, full test suite (`TestAgentsApplyFilter`, `TestAgentsFilterKeys`) and a render smoke check.
- [x] `[inarid]` `[medium]` **per-command shell auto-approve** - the caution-tier gate for `execute_shell_command` now branches per command: a binary on the auto-approve allowlist runs immediately (no `tool_request` round-trip), while any command not on the list still sends a `tool_request` and blocks for the user's `[y]`/`[n]`. the allowlist is a default read/build/inspect set (`go`, `make`, `git`, `ls`, `cat`, `find`, ...) in `internal/ipc/tools.go`, overridable via `config.json` `shell.allowlist` (empty falls back to the default); `curl`/`wget` were dropped from the default so network egress still prompts. `execTool`'s former hard-reject of non-allowlisted commands is gone: a non-listed command now runs *after* explicit approval rather than being refused, gated by `shellAutoApproved` at the stream dispatch. blast-radius note: this widens what a user can approve (previously non-listed commands were unrunnable), and `execute_shell_command` args are not path-sandboxed (only `cmd.Dir` is set), so an approved `rm`/`ssh`/`curl` can reach outside `cwd`; a future destructive tier should re-introduce hard-blocks for interpreters and remote-shell binaries. verified via go vet, go build, full test suite (`TestShellAutoApproved`, `TestSetShellAllowlist`, `TestLoadShellAllowlist`).
- [x] `[inarit/inarid]` `[medium]` **delete model from disk** - a `[d]` hotkey in the model selector removes a downloaded model from local disk storage (Ollama `DELETE /api/delete`), reclaiming space. distinct from `[u]` unload (memory eviction, `UnloadModel`); this frees disk, not RAM, and needs a re-pull afterwards. added `DeleteModel` to `provider.Provider` (implemented by `*ollama.Client` via `http.MethodDelete`), an `ollama.delete` one-shot RPC in the dispatch switch (placed alongside `ollama.load`/`ollama.unload`, not the streaming `model.pull` special-case), and an `ipc.Client.DeleteModel` proxy. destructive and irreversible, so `[d]` arms an in-selector confirm (`[y]` confirms, any other key cancels) per §8.2; when the target row is the session's assigned model the prompt carries an extra `(assigned to <session>)` warning. on success the list refreshes so the row flips back to `[pull]`. known limitation: deleting a model still assigned to a session leaves a dangling reference (next chat fails at Ollama until reassigned); auto-unassign-on-delete is out of scope here. verified via go vet, go build, full test suite (`TestDeleteModel`, `TestDeleteModelError`, `TestSelectorDeleteHotkey`).
- [x] `[inarit/inarid]` `[medium]` **model-load vs thinking indicator** - inarid checks `ListRunning` before starting a stream's first round; if the assigned model is not yet resident it emits a `status: "loading"` frame, then a `status: "thinking"` frame once the first chunk arrives from the provider (Ollama blocks the whole request until the model is loaded, so the first chunk is a reliable load-complete signal). state-accurate, not timeout-based; subsequent tool-call rounds within the same turn never re-signal since the model is already resident by then. inari's `ipc.Client.ChatStream` gained a `statuses` channel alongside `tokens`; the chat spinner renders `loading <model>...` while `loadingModel` is set, falling back to `thinking...`, with a running tool still taking priority over both. verified via go vet, go build, full test suite (`TestStreamSignalsLoadingWhenModelNotResident`, `TestStreamSkipsLoadingWhenModelResident`, `TestOnStatusTracksLoadingModel`, `TestViewportContentLoadingLabel`).
- [x] `[inarit]` `[easy]` **rename TUI references to `inarit`** - swept the docs (SPEC.md, CHANGELOG.md) so the terminal-UI client is called `inarit` everywhere, matching the `inarid` daemon convention: `inari` = product/umbrella, `inarid` = daemon, `inarit` = terminal ui, with `inarig`/`inariw` reserved for future frontends. resolved the overloaded roadmap component tags: `[kitsune]` -> `[inarit]`, `[kitsune/inarid]` -> `[inarit/inarid]`, and each `[inari]`/`[inari/inarid]` reclassified to `[inarit]`/`[inarit/inarid]` for TUI or client+daemon work; only genuinely product-level items keep `[inari]` (e.g. the cli command surface + doctor). mythological uses of "kitsune" (the fox messengers, in README and a code comment) were left intact. docs/tags only; the `cmd/inari` binary and `tui` package are unchanged by design.
- [x] `[inarit]` `[easy]` **fold `/model unload` into a hotkey** - `/model unload` removed from `chatCommandTable` and `handleSlashCommand`; unload is now a `[u]` hotkey in the model selector modal, shown only when the target session has a model assigned. the session's current model flows into the selector via `OpenModelSelectorMsg.Model` / `ForSession`; `[u]` emits a new `UnassignModelMsg` that the root routes to the agents optimistic-update + unassign RPC (mirrors the assign path). verified via go vet, go build, full test suite (`TestSelectorUnloadHotkey`, updated command-table sync tests) and a render check of the selector hint.
- [x] `[inarit]` `[easy]` **more built-in themes** - `emerald`, `cyan`, and `mono` (greyscale) added to `Themes` in `tui/views/theme.go` alongside purple/amber/slate/rose; each defines the same palette slots (Primary/Secondary/User/Ray), so no render changes were needed. cycled with `[t]` and stored in `config.json` as before. verified via go vet, go build, full test suite (`TestThemesWellFormed`).
- [x] `[inarit]` `[easy]` **larger agent-name pool** - session names are now `<adjective> <noun>` (e.g. `jade fox`) via a new `sessionNouns` pool (fox/woodland flavour) paired with the existing 24 adjectives, giving 24*26 = 624 combinations before the `agent #N` numeric fallback. `pickAgentName`'s in-use dedup and fallback are unchanged. verified via go vet, go build, full test suite (`TestPickAgentName`).
- [x] `[inarit]` `[easy]` **consistent modal width** - shared `ModalInnerW` constant (`UIWidth - 4`) and `modalInnerWidth(termWidth)` helper in `tui/views/hints.go`; both centred table modals (model selector, agents) size their columns and hint bar through it, capped to the 100-col budget and clamped down on narrow terminals. previously the selector pinned a hardcoded `modalInnerW = 64` while the agents modal followed raw `h.width` (so wide terminals stretched it past budget). the `help` overlay (content-sized centred box) and `describe` (full-screen view, not a modal) were assessed and intentionally left as-is; pinning short content to a fixed width only adds empty padding. verified via go vet, go build, full test suite, and a render-width assertion (`TestSelectorRenderModalWidth`).
- [x] `[inarit]` `[easy]` **status column in model popup** - `status` column added to the selector table showing `loaded` for models resident in memory (via `ListRunning`, fetched alongside the model list in `Init`), `downloaded` for other pulled models, and `[pull]` for recommended-but-not-pulled; the `[pull]` marker moved from an inline `est. vram` suffix to its own column. verified via go vet, go build, full test suite (`TestBuildSelectorRows`).
- [x] `[inarit]` `[easy]` **model notes column in popup** - `notes` column sourced from `CuratedModel.Notes` (§6.1), looked up by model name for any row (empty for non-curated local models), single-line truncated on rune boundaries with a trailing `...` measured via `lipgloss.Width` (`truncateCell`). column widths for `model`/`status`/`notes`/`est. vram` are computed within the shared modal budget (`selectorColumns`). row building was extracted to a pure `buildSelectorRows` for testability. verified via go vet, go build, full test suite (`TestSelectorColumns`, `TestBuildSelectorRows`, `TestTruncateCell`, `TestCuratedNotes`).
- [x] `[inari]` `[easy]` **cli command surface + `doctor`** - bare `inari` now launches daemon + TUI (previously printed help); `start` is kept as a hidden alias so `make start` and existing muscle memory still work. `status` is folded into a new `inari doctor` that checks ollama reachability, config presence, the configured base (thinker) model plus worker/sensor tiers, and daemon/socket state, exiting non-zero on any required-check failure so it works as a preflight gate. one binary, first arg selects mode; surface documented in §4.2.1. verified via go vet, go build, and the non-network test suite (ipc/ollama httptest failures are the pre-existing sandbox port-bind limitation).
- [x] `[inarid]` `[medium]` **split oversized files (ipc, session, ollama)** - five daemon-side files broken into responsibility-scoped files with no behaviour change: `internal/ipc/dispatch.go` 341->71 (dispatch_session/dispatch_chat/dispatch_ollama), `internal/ipc/tools.go` 276->135 (tools_exec), `internal/session/session.go` 271->148 (store), `internal/ollama/client.go` 227->139 (chat), `internal/ipc/server.go` 226->146 (conn/types). verified via go vet, go build, and the full test suite; ipc/session/ollama all have test coverage that exercises the moved code.
- [x] `[inarid/inarit]` `[medium]` **split oversized files (chat, model, agents, main)** - the four flagged monoliths are broken into responsibility-scoped files with no behaviour change: `tui/views/chat.go` 575->204 (chat_keys/chat_mouse/chat_stream/chat_msgs/chat_dispatch), `tui/model.go` 509->133 (model_router/model_nav/model_input/model_view), `tui/views/agents.go` 372->152 (agents_data/agents_mutations/agents_input), `cmd/inari/main.go` 316->70 (daemon/tui/process/paths). each view's `Update` is now a thin dispatcher delegating to per-topic handler methods; verified via go vet, go build, and the full test suite. remaining files over ~150 are tracked in the follow-up near-term item.
- [x] `[inarit]` `[medium]` **split oversized files (tui/views: selector, describe, agents_cmds)** - the three remaining flagged files are broken into responsibility-scoped files with no behaviour change: `tui/views/selector.go` 312->115 (selector_update, selector_render), `tui/views/describe.go` 287->119 (describe_update, describe_render), `tui/views/agents_cmds.go` 240->113 (agents_msgs for message types, agents_fmt for naming/formatting helpers). matches the existing `agents_data`/`agents_mutations`/`agents_input` split pattern; verified via go vet, go build, and the full test suite (`tui/views` package tests pass; unrelated pre-existing failures in `internal/ipc`/`internal/ollama` are sandbox network restrictions, not a regression from this change).
- [x] `[inarit]` `[medium]` **unified command vocabulary** - slash commands only exist in chat now (agents/describe/logs/models are hotkey-only), so the earlier `/agent describe` split no longer applies. remaining scope was killing the three parallel hand-synced command lists and adding contextual dimming. a single canonical table `chatCommandTable` (`tui/views/chat_commands.go`) is now the source of truth from which the command palette, tab-completion, and the help overlay all derive; `helpByView["chat"]` is populated by `init()` from the table. the palette (`renderChatSuggestions`) dims commands not currently actionable (`/model unload` with no model, `/copy` with no reply, `/tools` with no cwd), mirroring the validity guards already in `handleSlashCommand` and matching how the agents hint bar already dims. `chat_commands_test.go` pins the predicates, prefix filtering, and guards against help/dispatch drift.
- [x] `[inarit]` `[medium]` **consolidate commands into chat view** - the agents popup no longer has a text input or slash commands; `/model`, `/model unload`, export, `/logs`, and describe all now live in chat (`tui/views/chat.go`), matching how export and describe were already reachable there. `agents_slash.go` was removed and the agents popup trimmed to just the session table plus add/enter/delete/back hotkeys.
- [x] `[inarid]` `[easy]` **default model on agent creation** - `session.create` (`internal/ipc/dispatch.go`) now attaches `defaultNewAgentModel` (`gemma4:e2b`) to every new session, so `[a]` add in the agents popup and the first-run "default agent" auto-create both produce a session ready to chat without a manual `/model select`.
- [x] `[inarit]` `[medium]` **agents view and model selector as chat-hosted popups** - chat is now the single persistent base view. `showAgents`/`showModelSelector` overlay flags (`tui/model.go`) render `Agents.RenderModal()`/`ModelSelector.RenderModal()` over chat instead of switching `m.current`; the `viewModels` view and `returnView` field were removed since every entry point (agents' `/model select`, chat's `ctrl+o` and `/model select`) now opens the modal in place. the agents popup itself was trimmed to just the session table plus a `[q/esc] back to chat` hint, dropping the session/cwd/status footer; `[q]` and `[esc]` close both the agents and model-selector popups consistently.
- [x] `[inarit]` `[easy]` **pre-context line in chat** - the injected file-tree/project-context system prompt is summarised as a single `[context] cwd: <path> (+ project context)` line and rendered as the first line of the chat viewport (`buildContextLine` in `tui/views/chat_helpers.go`); persists across `/clear` and shows for a brand-new session with no history yet.
- [x] `[inarid/inarit]` `[medium]` **pull models from the UI** - the model selector appends recommended-but-not-pulled models to the table marked `[pull]`; selecting one triggers `ollama pull` via a new `model.pull` RPC (dedicated connection, mirrors `session.stream`), streams download progress into the status line, then refreshes the list and assigns the model as usual.
- [x] `[inarit]` `[easy]` **rename herd to agents** - the `Herd` view/type became `Agents` (files `herd.go`/`herd_view.go`/`herd_cmds.go`/`herd_slash.go` renamed to `agents*.go`); the `/herd` slash command used from chat to navigate back is now `/agents`; the `[herd]` session-line label is now `[agents]`. in-view sub-commands (`/agent add`, `/agent chat`, etc.) were already named `/agent` and are unchanged.
- [x] `[inarid]` `[easy]` **daemon idle auto-shutdown** - inarid exits on its own after `idle_shutdown_mins` (default 30) with no client activity; any RPC including ping heartbeats resets the watchdog, so the daemon stays up while a inarit is open and exits after the TUI closes. `0` falls back to the default, a negative value disables it.
- [x] `[inarid/inarit]` `[easy]` **package `doc.go` coverage** - every package now carries a `doc.go` with the canonical `it owns:` / `it does NOT own:` statement; pre-existing inline package comments were demoted to file-level notes so `go doc` shows one ownership block per package.
- [x] `[inarit]` `[easy]` **download context and copy response** - chat slash commands `/copy` (copies the latest assistant response to the clipboard) and `/save` (writes full session history to `~/.local/share/inari/exports/`, reusing the herd export path); both report success or failure in the status line.
- [x] `[inarit]` `[easy]` **surface swallowed errors** - clipboard-copy failures in the chat viewport and theme-save failures now show a `[warn]` status instead of failing silently; the theme save was also made synchronous, removing a config-write race.
- [x] `[inarit]` `[easy]` **chat navigation shortcuts**: `ctrl+t` toggles the builtin tools panel, `ctrl+p` opens the slash command palette, `ctrl+g` toggles the help overlay; `esc` exits the tools panel or clears an in-progress slash command. ctrl-prefixed keys avoid the bare-key text-input clash (open issue). `ctrl+m` was dropped because terminals deliver it as carriage-return and would shadow `[enter]` send.
- [x] `[inarit]` `[easy]` **chat input mode indicators**: the chat entry prefix reflects the active mode; `[/] ❯` while composing a slash command, `[tool] ❯` while the builtin panel is open, otherwise `[chat] ❯`.
- [x] `[inarid]` `[easy]` **agent context file**: on session creation with a `cwd`, inarid reads the first of `AGENTS.md` or `.inari/context.md` (sandboxed, capped at 8 KB) and appends it to the system prompt under a `project context:` heading. absent file is a graceful no-op.
- [x] `[fox]` CLI removed — functionality superseded by inarit TUI
- [x] `[inarit]` `[easy]` **chat-first startup with herd accessible separately** — on launch, inarit opens directly into the chat view for the default (or most recent) session rather than the herd table; the herd view is reachable via `/herd` slash command from within chat.
- [x] `[inarit]` `[easy]` **explicit hotkeys for view switching; remove esc-to-herd** — replaced implicit `esc` exit from chat to herd with `/herd` slash command; `esc` in chat dismisses overlays only.
- [x] `[inarid]` `[easy]` **daemon lifecycle commands** — `inarid start` and `inarid stop` subcommands; `start` forks inarid to the background and writes a PID file; `stop` reads the PID file, sends `SIGTERM`, and removes the file.
- [x] `[inarit]` `[easy]` **mouse scroll for chat buffer** — mouse wheel scroll on the chat viewport.
- [x] `[inarit]` `[easy]` **cpu and memory in top bar** — system-wide cpu and memory polled at ~2s intervals; displayed in top bar as `cpu N%  mem X / Y gb`.
- [x] `[inarit/inarid]` **context compression (ponder)** — `/compact` in chat triggers inarid to summarise conversation history via the session's own model, replacing old turns with a compact summary; system prompt is preserved.
- [x] `[inarit]` thinking spinner in chat session while waiting for a response
- [x] `[inarit/inarid]` offline detection in chat — when inarid is unreachable, the hint line shows "inari is offline" and sends are blocked until connectivity is restored
- [x] `[inarit/inarid]` streaming chat — `session.stream` RPC over dedicated per-call UDS connections; inarit renders tokens as they arrive
- [x] `[inarit]` title bar wave animation — per-character purple gradient drifts across the inarit title at 200ms intervals
- [x] `[inarit/inarid]` filesystem context (layer 1) — shallow file tree injected into system prompt at session creation; inarit passes `cwd`, inarid walks up to 3 levels (skipping `.git`, `node_modules`, etc.)
- [x] `[easy]` add `LICENSE` file — bsl; copyright holder: Jimmy MG Lim
- [x] `[inarit]` `[medium]` themes — a small set of built-in colour themes (e.g. default purple, amber, slate, rose); cycle through them with `[t]` from any view; theme is stored in config.json and applied at startup
- [x] `[inarit]` `[easy]` help overlay — `[?]` opens a modal listing all hotkeys for the current view; `[esc]` or `[?]` dismisses it
- [x] `[inarit]` `[easy]` quick-start fox — if the herd view has no sessions, automatically create a default session so the user can start chatting immediately without a manual create step
- [x] `[inarit]` `[easy]` fox status line in herd view — render a `<session-name> > ` line directly above the hotkey hint bar, showing the name of the currently selected inarit session as a prompt-style prefix; updates as the table cursor moves
- [x] `[inarit]` `[easy]` export chat history to file — `[e]` in herd view fetches full message history via `session.history` RPC, formats as plain text (`role: content` per message, `---` separator), and writes to `~/.local/share/inari/exports/<session-name>-<timestamp>.txt` (XDG data dir); path is shown in the status bar on success
- [x] `[inarit]` `[easy]` show current token count in chat
- [x] `[inarid]` **filesystem tool-call loop (layer 2)** — inarid declares read-only tools (`read_file`, `list_dir`) in the Ollama API request for sessions that have a working directory set. when Ollama returns a tool-call instead of text, inarid executes the tool (sandboxed to the session's `cwd`), appends the result as a `tool` message, and re-sends to Ollama — looping until a final text response arrives. write operations are explicitly out of scope at this stage.
- [x] `[inarid]` **extended layer-2 tools** — `grep_file` (regex search across files in cwd) and `stat_file` (size, mtime, type) added alongside `read_file`, `list_dir`, and `execute_shell_command`; all sandboxed to session `cwd`.
- [x] `[inarid]` **`execute_shell_command` builtin** — allowlisted bash execution: `go`, `make`, `git`, `date`, `echo`, `pwd`, `whoami`, `uname`, `wc`, `curl`, `wget`, `find`, `ps`, `ls`, `cat`, `df`, `uptime`, `which`; `exec.Command` (no shell expansion); 30 s timeout; 64 KB output cap. caution-tier per §8.3.
- [x] `[inarit]` `[medium]` **tool approval gating** — when inarid needs to execute a tool during a stream, it sends a `tool.approval_request` message; the stream pauses and inarit renders an approval prompt replacing the hint bar; the user presses `[y]`/`[n]` to approve or reject before execution resumes. all keys are absorbed while approval is pending.
- [x] `[inarid]` `[easy]` **auto-create config** — if `~/.config/inari/config.json` does not exist at startup, inarid creates it with defaults (socket, memory budget, ollama url, default model tiers, theme); the user gets a ready-to-edit file rather than a startup error.
- [x] `[inarit]` `[easy]` **shared footer component** — `tui/views/footer.go` owns `RenderFoxLine`, `renderFooter`, and `renderCWDLine`; all views use it. the footer now shows `label | name | model | tokens | cwd` in one line, followed by a dedicated cwd sandbox line when a session directory is set.
- [x] `[inarit]` `[easy]` show cwd in status bar — cwd is displayed in the footer of both chat and herd views; rendered via `renderCWDLine` as `[cwd] <path>`.
- [x] `[inarit]` `[easy]` slash commands — `/` in the chat input opens a command suggestion list (`/clear`, `/compact`, `/model change`, `/tools`); selecting with tab or enter executes the command. `/clear` wipes session history; `/compact` summarises the conversation via the session's own model.
- [x] `[inarit]` `[easy]` **input history navigation** — `↑`/`↓` in the chat input field cycles through previously sent messages; history is per-session and in-memory for the session lifetime.
- [x] `[inarit]` `[easy]` **model capability tags in herd view** — after sessions load, inarid calls `ollama.show` (→ `POST /api/show`) per model; the response `capabilities` array is cached in the Herd. the model column renders `[tool]` and `[vis]` suffixes where applicable. fetches are lazy and per-model, so models without caps are unaffected.
- [x] `[inarid]` **`ollama.show` RPC** — new `ollama.show` handler in the server; `ModelCaps(model)` added to the `Provider` interface and implemented in the Ollama client. the IPC client exposes `ModelCaps(model string) ([]string, error)`.
- [x] `[inarid]` `[easy]` add `gemma4:e2b` as the default master local model always — set as the thinker-tier default in config and fallback when no model is assigned to a session
- [x] `[inarit]` `[medium]` **chat viewport line selection** - left-button drag inside the chat viewport selects whole content rows and copies them to the system clipboard on release. `chat_mouse.go` tracks `selStartLine`/`selEndLine` as post-hardwrap content-row indices (`viewport.YOffset + (msg.Y - viewportTopY)`), so selection follows scroll offset and `ansi.Hardwrap` splits; `setViewportContentWithSel` (`chat_render.go`) paints a highlight over the selected rows and `selectedText` joins + `ansi.Strip`s them. `copyToClipboard` (`clipboard.go`) shells to `pbcopy` (darwin) / `xclip` (linux); the footer shows `[copied] N lines`, or `[warn] copy failed` when the clipboard tool is absent. whole-row granularity only; character-level precision is the separate `[hard]` item in near term. no dedicated mouse/selection test; verified by inspection of the press/motion/release path.

---

## 4. Architecture

### 4.1 System Overview

```
  you (inari TUI)
      |
      |  JSON-RPC over Unix socket  (chmod 0600)
      |
  inarid (daemon)
    ├── session store   — persists sessions + history to ~/.local/share/inari/sessions/
    ├── ollama client   — sends full message history to local models
    ├── scheduler       — semaphore memory-budget throttle (library only; not wired)
    └── audit logger    — append-only record of all tool calls
```

**stack:** Go · Bubble Tea / LipGloss · Ollama

### 4.2 Component Topology

```
┌─────────────────────────────┐
│         inari (TUI)         │  ← user-facing client
│   Bubble Tea / LipGloss     │
└────────────┬────────────────┘
             │ JSON-RPC over UDS
             │ /tmp/inari.sock (chmod 0600)
┌────────────▼────────────────┐
│       inarid (daemon)       │  ← long-running engine
│                             │
│  ┌──────────┐ ┌──────────┐  │
│  │ MCP Host │ │ Ollama   │  │
│  │ (stdio)  │ │ Sessions │  │
│  └──────────┘ └──────────┘  │
│  ┌──────────────────────┐   │
│  │    Audit Logger      │   │
│  └──────────────────────┘   │
└─────────────────────────────┘
```

### 4.2.1 Binary & Command Surface

inari ships as a single binary; the first argument selects a mode, and there is no
separate daemon executable. this keeps distribution to one artifact (a single
`brew install`) and guarantees the client and daemon are always the same build, so
the private UDS protocol never has to negotiate versions across two binaries.

| command         | mode                                  |
| :-------------- | :------------------------------------ |
| `inari`         | default: fork daemon, then run TUI    |
| `inari tui`     | TUI only; daemon must be running      |
| `inari chat`    | headless one-shot; print reply        |
| `inari daemon`  | run daemon in foreground              |
| `inari doctor`  | preflight: ollama, config, base model |
| `inari stop`    | signal daemon to exit                 |
| `inari version` | print version                         |
| `inari help`    | usage                                 |

bare `inari` (and its `start` alias, kept so `make start` keeps working) re-execs
the binary as `inari daemon --background` via `os.Executable()`
(`cmd/inari/process.go`), writes a PID file, then runs the TUI in the foreground.
the daemon self-terminates after `idle_shutdown_mins` of no client activity, so the
common path needs no explicit `stop`. `doctor` exits non-zero when a required check
fails (ollama unreachable, or the base/thinker model not pulled), so it doubles as a
setup or CI preflight gate.

### 4.3 Session Model

Sessions are the primary entity in ai-inari. A session is a named chat context
(e.g. "Arctic Fox") that exists independently of any model. The user creates a
session first, then optionally assigns a model to it. Chat history is stored
inside the session in inarid — clients are stateless and hold no history locally.

This means:
- Restarting inari reconnects to the existing sessions without losing any conversation.
- A session with no model assigned is valid; the model can be swapped at any time.
- `session.chat` takes a session ID and a single new message; inarid appends it,
  sends the full history to Ollama, stores the reply, and returns only the text.
- Restarting inarid reloads all sessions from disk — history and model assignment
  are preserved across daemon restarts.

#### 4.3.1 Session Persistence

Sessions are persisted to disk as JSON files, one file per session (`<id>.json`),
stored in the session data directory (default: `~/.local/share/inari/sessions`,
overridable via `data_dir` in `config.json`).

Writes are atomic: inarid writes to a `.tmp` file then renames it, so readers
never observe a partial file. The file is written on every state change:
`session.create`, `session.assign`, `session.unassign`, and after each
`session.chat` turn (both the user message and the model reply are flushed).

### 4.4 IPC

- Transport: Unix Domain Socket at `/tmp/inari.sock`.
- Permissions: `chmod 0600` — owner-only access.
- Protocol: JSON-RPC 2.0 for all control RPCs; newline-delimited JSON frames for streaming chat.
- Daemon persists sessions on client detach; client reconnects by session ID.

**Session RPCs:**

| Method              | Params               | Returns         | Description                                      |
| :---                | :---                 | :---            | :---                                             |
| `session.list`      | —                    | `SessionInfo[]` | summary of all sessions (no history on wire)     |
| `session.create`    | `{name, cwd?}`       | `SessionInfo`   | create a named session; optional `cwd` enables filesystem context |
| `session.delete`    | `{id}`               | `"ok"`          | remove session and its history                   |
| `session.assign`    | `{id, model}`        | `"ok"`          | attach a model to a session                      |
| `session.unassign`  | `{id}`               | `"ok"`          | detach the model from a session                  |
| `session.chat`      | `{id, text}`         | `string`        | blocking: append message, return full reply      |
| `session.stream`    | `{id, text}`         | *(see below)*   | streaming: append message, stream token chunks   |
| `session.history`   | `{id}`               | `Message[]`     | full message history for a session               |
| `session.compact`   | `{id}`               | `"ok"`          | summarise history via the session's own model and replace old turns |
| `ollama.show`       | `{model}`            | `string[]`      | capability tags for a model (e.g. `["completion","tools"]`); empty array if unknown |

**Streaming chat (`session.stream`):**

`session.stream` uses a **dedicated per-call UDS connection** rather than the shared RPC connection. This allows multiple simultaneous streams (one per open chat view) without head-of-line blocking.

Protocol over the dedicated connection:

1. client dials a new `unix` connection to `/tmp/inari.sock`
2. client sends a normal JSON-RPC 2.0 request: `{"method":"session.stream","params":{"id":"...","text":"..."}}`
3. inarid responds with a stream of newline-delimited JSON frames:
   ```json
   {"token":"Hello"}
   {"token":" world"}
   {"tool_approval_request":{"tool":"execute_shell_command","args":{"command":"go","args":["test","./..."]}}}
   {"token":"Tests passed."}
   {"done":true}
   ```
   when a `tool_approval_request` frame arrives, inari pauses rendering and waits for the user to press `[y]` or `[n]`. it then sends `{"tool_approved":true}` or `{"tool_approved":false}` back over the same connection. inarid blocks until it receives the response before executing or skipping the tool call.
4. on `done`, inarid has persisted the full reply to the session store; client closes the connection
5. on error, inarid sends `{"error":"<message>"}` and closes

inari opens one dedicated connection per active `session.stream` call. the shared `Client` connection remains exclusively for control RPCs and is never blocked by in-flight streams.

**multiple concurrent streams:**

within a single inari TUI, the user can spawn multiple named chat sessions (each displayed as a row in the sessions view). each session runs independently — it can have a model assigned and an active generation in flight simultaneously. because each `session.stream` call uses its own dedicated UDS connection, all sessions can stream concurrently without blocking one another. inarid handles each stream in its own goroutine via the accept loop.

**message routing in inari:**

token messages (`ChatTokenMsg`, `ChatDoneMsg`) carry a `SessionID` field. the root model routes them directly to the correct `Chat` view in `m.chats[sessionID]` — regardless of which view is currently displayed. this allows background sessions to accumulate tokens invisibly; when the user switches back, the chat view already shows the partial or complete response.

### 4.5 Concurrency & Scheduling

- Each Ollama session runs in its own goroutine.
- A memory-budget semaphore to gate concurrent sessions is planned but not yet wired (see Ideas); today each session streams unthrottled.
- Multiple simultaneous chat streams are supported — each uses its own UDS connection.
- Slow/background tasks continue when the TUI is detached.

### 4.6 Filesystem Awareness — Three-Layer Model

sessions can be given awareness of the local filesystem in three progressively richer layers. each layer is a prerequisite for the next.

**layer 1 — directory context (system prompt injection)**

inari passes the current working directory when creating a session. inarid resolves a shallow file tree (`find . -maxdepth 3`, filtered by `.gitignore`) and prepends it as a system message:

```
system: working directory: /path/to/project
<file tree>
```

the model can reason about the project layout and refer to files by path, but cannot read their content. this requires no changes to the ollama request format and works with every model.

**layer 2 — read-only file access (agentic tool-call loop)**

inarid declares five built-in tools in the ollama `/api/chat` request for sessions that have `cwd` set:

| tool                   | input               | output                                    |
| :--------------------- | :------------------ | :---------------------------------------- |
| `read_file`            | `{path}`             | file contents (text only)                 |
| `list_dir`             | `{path}`             | directory listing (names only)            |
| `grep_file`            | `{path, pattern}`    | matching lines with filename and line no  |
| `stat_file`            | `{path}`             | size, mtime, type                         |
| `execute_shell_command` | `{command, args[]}`  | stdout+stderr, exit code as text          |

**naming convention:** tool names follow `verb_noun` (e.g. `read_file`, `list_dir`, `execute_shell_command`); reads as an instruction and aligns with common tool-calling schemas (MCP, OpenAI).

all tools are sandboxed: paths are resolved relative to `cwd` and must not escape it (no `../` traversal). `execute_shell_command` is additionally gated by an allowlist (see §8.3). write operations are out of scope.

when ollama returns a `tool_calls` response, inarid's `handleStream` loop:

1. executes each tool call inside the sandbox
2. appends a `tool` role message with the result
3. re-sends the full message history to ollama
4. repeats until ollama returns a `message` (text) response
5. streams the final text back to inari as normal token frames

this requires ollama tool-call support — only models that explicitly declare function-calling capability in their model card will use the tools. others silently ignore them and respond with plain text.

**models with tool-call support (layer 2 compatible):**

| model | notes |
|-------|-------|
| qwen3 (any size) | recommended; strong tool use across all sizes |
| llama3.1 / llama3.2 | instruct variants only |
| mistral-nemo | solid tool support |
| mistral 7b instruct | function-calling variants |
| command-r | designed for agentic use |

**models without tool-call support (layer 1 only):**

| model | behaviour |
|-------|-----------|
| phi3 / phi4 | ignores tools, responds with text |
| gemma2 | ignores tools, responds with text |
| deepseek-r1 | most variants do not support tool calls |
| older / chat-only models | silent no-op — tools declared but never invoked |

assigning a non-tool-capable model to a session with `cwd` set is safe — tools are declared in the request but the model will not invoke them. layer 1 (file tree in system prompt) still applies and provides value regardless of model capability.

**prompt-based tool calling (fallback for non-native models):**

the native `tools` API parameter solves the "silent ignore" problem only for models that natively support it. for everything else — including strong local models like `hermes-3-pro-8b` or `qwen3-coder` — a more reliable approach is:

1. **do not use the `tools` parameter.** inject tool definitions as plain text into the system prompt instead:
   ```
   you have access to the following tools. when you need to use one, respond only with valid JSON in this format:
   {"tool": "read_file", "path": "relative/path"}
   {"tool": "list_dir", "path": "."}
   ```
2. **set `format: "json"` in the ollama request.** this forces the model to treat tool use as a structured text instruction rather than an API feature, making it reliable across any instruction-following model.
3. **inarid parses the response.** if the JSON response contains a `tool` key, it is treated as a tool call; otherwise it is a plain text reply.

this approach trades API cleanliness for broad model compatibility. it is the recommended strategy for local SLMs where native function-calling is patchy or absent.

**roadmap:** inarid should detect model capability at session creation (or via config) and automatically select native vs. prompt-based tool calling. the `handleStream` loop is the same either way — only the request format and response parser differ.

**layer 3 — MCP filesystem connector**

once the tool-call loop exists, built-in tools can be replaced by `@modelcontextprotocol/server-filesystem` spawned via mcp-go. the loop delegates tool execution to the MCP host instead of running it inline. this unlocks the full MCP tool surface (search, write when permitted, etc.) and the same loop handles all future connectors uniformly.

**`session.create` RPC extension (layers 1 + 2):**

```json
{"name": "my session", "cwd": "/path/to/project"}
```

`cwd` is optional. when absent, the session behaves as today — no filesystem context, no tools declared.

### 4.7 Configuration Hierarchy

when inarid loads, it merges configuration files using the following precedence (highest to lowest):
1. local project-scoped configuration (`.inari/config.json` in the session's working directory)
2. global configuration (`~/.config/inari/config.json`)
3. built-in defaults

only configuration fields explicitly defined in the local file override the global settings; other fields are inherited.

---

## 5. Components Deep-dive

### 5.1 `inarid` — Daemon Subsystems

| Subsystem     | Responsibility                                              |
|---------------|-------------------------------------------------------------|
| UDS Server    | Accept and authenticate client connections                  |
| Session Store | Own named sessions with chat history; persists to JSON files on disk; survives daemon restart |
| MCP Host      | Spawn and manage MCP connectors via `mcp-go` (Linear, Slack, Google Drive, etc.); current `internal/mcp` is a hand-rolled fallback — low migration risk as the protocol is stable JSON-RPC 2.0 over stdio |
| Ollama Client | POST to `/api/chat`; stream tokens back to session          |
| Scheduler     | Semaphore-based concurrency throttle per resource tier      |
| Audit Logger  | Append-only log of all JSON-RPC tool-calls with timestamps  |

### 5.2 `inari` — Client

| view     | key | description                                                    |
| :------- | :-- | :------------------------------------------------------------- |
| Sessions | -   | default view; table of all sessions with model and status      |
| Logs     | `l` | tail output of selected session                                |
| Describe | `d` | full session metadata and config                               |
| Chat     | `i` | interactive chat; slash commands, tool approval, input history |

**footer layout (all views):**

```
label | name | model | tokens | cwd
[cwd] <path>              ← omitted when no cwd is set
<status message>          ← transient; cleared on next keypress
<input widget>
<hint bar>
```

the footer is assembled by `renderFooter` in `tui/views/footer.go` and shared across all views.

**sessions hotkeys:** the sessions popup is hotkey-only, no text input, no slash commands. model selection, export, logs, and describe all moved to chat (`/model`, `/model unload`, `/export`, `/logs`, `/describe`) since chat is the main view and owns every shared command.

| key         | effect                                                         |
| :---------- | :-------------------------------------------------------------- |
| `a`         | create session                                                 |
| `enter`     | open chat for the session under the cursor                     |
| `x`         | delete session under the cursor                                |
| `q` / `esc` | close popup and return to chat (no-op in the full-screen view) |

**chat slash commands:**

| command         | effect                                                        |
| :-------------- | :------------------------------------------------------------ |
| `/clear`        | wipe session message history                                  |
| `/compact`      | summarise history via session's own model; replaces old turns |
| `/copy`         | copy last assistant response to clipboard                     |
| `/export`       | save full session history to a text file                      |
| `/model`        | open model selector for this session                          |
| `/model unload` | unload the assigned model                                     |
| `/tools`        | toggle built-in tools panel for this session                  |
| `/describe`     | open session metadata/config view                             |
| `/logs`         | open the inarid log viewer                                    |
| `/sessions`     | open sessions as a popup over chat; [q]/[esc] closes          |
| `/chat`         | jump to default session's chat                                |
| `/refresh`      | reload the session list in background                         |
| `/theme`        | cycle theme                                                   |
| `/help`         | toggle help overlay                                           |
| `/quit`         | quit                                                          |

#### 5.2.1 Offline resilience

the root model polls inarid via `ConnStatusMsg` on a regular tick. when the daemon is unreachable, every `Chat` view is updated with `WithOffline(true)`. in that state:

- the `[enter] send` key binding is suppressed — pressing Enter does nothing.
- the hint line replaces the key-binding row with `inari is offline` (rendered in red).
- the input textarea remains editable so the user can compose a message while waiting.
- when connectivity is restored (`ConnStatusMsg{OK: true}`), all chats are updated with `WithOffline(false)` and normal behaviour resumes immediately — no queued messages are replayed.

queuing was explicitly not chosen: a silently queued message submitted minutes later (possibly to a cold model) is more surprising than a clear offline signal.

#### 5.2.2 Viewport quirks (`bubbles v0.18.0`)

**`GotoBottom` undershoots when content overflows the pane.**

`viewport.SetContent` in bubbles v0.18.0 splits content on `\n` only — it does not perform terminal line-wrapping. `GotoBottom` computes its offset from `len(lines) - height`, where `lines` is the raw newline count. Long styled lines (e.g. a multi-sentence assistant reply with no embedded newlines) count as 1 line but visually wrap across multiple terminal rows. Once accumulated wrapping exceeds the pane height, `GotoBottom` undershoots and new streaming tokens appear below the visible area.

**fix:** call `ansi.Hardwrap(content, vp.Width, true)` before `SetContent`. This inserts real `\n` characters at the terminal width (ANSI-aware, so escape codes don't inflate the count), making the stored line count match the visual row count. See `setViewportContent` in `tui/views/chat.go`.

---

## 6. Resource Tiers Logic

The herd uses a tiered scheduling system to manage local hardware resources:

- **Runners (Background execution):** Low/mid-priority, small-to-standard context. Dispatched by the thinker agent for intent classification and background/parallel task execution; consolidates the earlier separate sensor and worker tiers (see §2.2) since neither had a real consumer of its own.
- **Thinkers (Reasoning):** High-priority, large context. Used for interactive chat and complex architectural reasoning; the agent the user talks to directly.

Memory budget is enforced via `memory_budget_mb` in `config.json`. The scheduler blocks model loading if the budget would be exceeded.

### 6.1 Ollama Model Curation

curated picks by hardware tier and role. pull via `ollama pull <tag>`. prefer `q4_k_m` quant unless the tier has headroom for `q8_0`.

<!-- BEGIN generated: §6.1 tables from tui/views/curated.go CuratedModels; run `make curated-sync` -->

#### general

| tier | model       | size   | notes                                                   |
| :--- | :---------- | :----- | :------------------------------------------------------ |
| 32gb | qwen3.6:27b | ~16gb  | alibaba; near-frontier chat and review                  |
| 16gb | phi4:14b    | ~8gb   | microsoft; strong multi-file reasoning                  |
| 16gb | gemma4:12b  | ~7.6gb | google; 12b dense; strong general chat                  |
| 8gb  | gemma4:e4b  | ~2.7gb | 4.5b effective; fast routing and quick queries          |
| 8gb  | gemma4:e2b  | ~1.5gb | 2b effective; leaner and faster than e4b, lower quality |
| 4gb  | llama3.2:3b | ~2gb   | meta; best chat and reasoning within 4gb                |

#### coding

| tier | model                    | size  | notes                                        |
| :--- | :----------------------- | :---- | :------------------------------------------- |
| 32gb | qwen3.6:27b-coding-nvfp4 | ~18gb | alibaba; near-frontier generation and review |
| 16gb | deepseek-r1:14b          | ~9gb  | r1-671b distil; strong coding and reasoning  |
| 8gb  | deepseek-r1:8b           | ~5gb  | r1-671b distil; fits 8gb; coding+reasoning   |
| 4gb  | llama3.2:3b              | ~2gb  | meta; best within 4gb budget                 |

<!-- END generated: §6.1 tables -->

### 6.2 Keeping the curation current (findings)

the table is dual-maintained (`tui/views/curated.go` `CuratedModels` + §6.1 above) and drifts; the last rebuild found two dead tags shipping in a "live" list (`gemma4:27b` and `phi-4:14b`, both 404 - the latter a `phi-4`/`phi4` typo). two cheap mechanisms, split by what each can actually prove:

- **tag resolves (does it 404):** an HTTP HEAD against the ollama registry is enough and needs no pull - `curl -s -o /dev/null -w "%{http_code}" https://registry.ollama.ai/v2/library/<model>/manifests/<tag>` returns 200 vs 404. verified against the current list. fold into a `make check-models` target or `inari doctor` to catch dead tags without downloading gigabytes.
- **model actually runs + invokes tools:** resolution is not function; that check is the headless tool-calling smoke test (roadmap items "`inari doctor`: verify pulled models actually work" and "discover + locally test new candidate models"), which drives a real turn and checks the audit log for a `tool.call` entry.

drift itself is killed at the source: `CuratedModels` (`tui/views/curated.go`) is the single source and the §6.1 tables above are generated from it (`RenderCuratedTables`, between the marker comments). edit `CuratedModels`, run `make curated-sync`, and `make test` fails if the table is left stale (`TestCuratedTablesInSync`).

---

## 7. MCP Connectors

Connectors are spawned as child processes via stdio pipes and speak JSON-RPC 2.0 (the MCP protocol). Any MCP-compliant server works — connectors are independent of inarid.

**library: `github.com/mark3labs/mcp-go`**

inarid uses `mcp-go` as the MCP client library rather than hand-rolling the protocol. it handles stdio transport, capability negotiation, and message framing. the current `internal/mcp` package is a hand-rolled precursor and will be replaced. migration risk is low — if `mcp-go` is ever unavailable, `internal/mcp` serves as a known-working fallback since the protocol is stable.

**planned connectors:**

| Connector    | Purpose                          | Server package              |
|--------------|----------------------------------|-----------------------------|
| Linear       | issue tracking, project management | `@linear/mcp-server`      |
| Slack        | messaging, channel search        | community Node.js server    |
| Google Drive | file read/write                  | community Node.js server    |
| Filesystem   | read/write local files           | `@modelcontextprotocol/server-filesystem` |
| Search       | web or local document search     | community server            |
| SQL          | query local databases            | community server            |

connector definitions loaded from `config.json` at daemon start. each entry specifies the command to spawn and its arguments — inarid is agnostic to the connector's implementation language.

---

## 8. Security Model

- All IPC local-only via UDS; no TCP exposure.
- Socket permissions restrict access to the owning user.
- All MCP tool-calls written to an append-only audit log.
- No credentials, tokens, or secrets stored by the daemon.
- MCP child processes run with inherited (restricted) environment.

### 8.1 Least-Privilege Principle

**default posture: deny.** every capability a model or connector can exercise must be explicitly granted. nothing is open by default.

this applies at every layer where the model can touch the host system:

| layer | default | must be explicitly granted |
|-------|---------|---------------------------|
| filesystem context (layer 1) | read file tree (names only, no content) | — (always safe) |
| filesystem tools (layer 2) | no tools declared | `read_file`, `list_dir` per session, sandboxed to `cwd` |
| MCP connectors | none spawned | each connector named in `config.json`; scope defined per connector |
| write operations | never | no write tools at any layer without explicit future design decision |

**sandbox invariants (layer 2+):**
- all paths are resolved relative to the session's `cwd` and validated before execution.
- `../` traversal and absolute paths outside `cwd` are rejected.
- write, delete, and execute operations are out of scope until a deliberate security review approves them.

**MCP connector hygiene:**
- each connector is spawned as a child process with a minimal, scrubbed environment — only variables it explicitly needs.
- connectors declare their own tool surface; inarid does not grant capabilities beyond what the connector exposes.
- adding a new connector to `config.json` is a conscious operator decision, not an automatic one.

**audit log as enforcement:**
- every tool-call routed through inarid is appended to the audit log before execution, not after. if logging fails, the call is rejected.
- the log is append-only and owned by the daemon process; connectors cannot write to it directly.

### 8.2 Destructive Action Prevention

the goal is to make the worst-case outcome bounded regardless of user behaviour — confirmation gates alone are insufficient because users start approving blindly under repeated prompts.

**three layers working together:**

**layer A — risk-tiered auto-approval**

every tool-call is classified at dispatch time by a static risk tier. the tier is defined per tool, not inferred from the model's intent or phrasing.

| tier        | current tools                                                                 | inarid behaviour                                                                                                       |
| :---------- | :---------------------------------------------------------------------------- | :--------------------------------------------------------------------------------------------------------------------- |
| safe        | `read_file`, `read_lines`, `list_dir`, `find_files`, `grep_file`, `stat_file` | execute immediately, no approval round-trip, log result                                                                |
| caution     | `execute_shell_command`                                                       | allowlisted command: execute immediately; otherwise send `tool_request`, block until `tool_approved`, rejection logged |
| destructive | (none yet, future write tools)                                                | always require confirmation; shown in red in inari                                                                     |
| forbidden   | process spawn, network outside ollama/mcp, shell exec                         | hard-rejected; never routable                                                                                          |

classification is conservative: if a tool's tier is ambiguous, it is assigned the higher-risk tier. adding a new tool requires an explicit tier assignment — unclassified tools are rejected.

**layer B — blast-radius limits**

hard limits enforced by inarid regardless of tier or confirmation:

- all file operations capped at 1 MB per call.
- no operations outside the session's `cwd` (path traversal rejected at validation, not policy).
- no more than 10 tool-calls per model turn (prevents runaway loops).
- no spawning processes or making network calls from within a tool handler.

**layer C — dry-run for caution-tier actions**

before executing a caution-tier tool-call, inarid computes a dry-run result and sends a `tool.preview` message to inari:

```
[preview] write_file: path/to/file.go
--- current
+++ proposed
@@ -1,3 +1,5 @@
 ...
```

inari renders the preview and waits for `[y] approve` or `[n] reject`. only on approval does inarid execute. rejection is logged. if inari is detached, caution-tier calls are automatically rejected — they never execute unattended.

**non-goal:** this design does not attempt to detect malicious intent from model outputs. it bounds damage structurally so that even a model producing harmful tool-calls cannot exceed the permitted blast radius.

### 8.3 execute_shell_command - allowlisted bash execution

`execute_shell_command` lets the model invoke a fixed set of development commands inside the session's `cwd`. it is the boundary between read-only filesystem tools and write/execute capability.

**implemented constraints**

| constraint        | detail                                                                                                           |
| :---              | :---                                                                                                             |
| auto-approve list | default `go`, `make`, `git`, `ls`, `cat`, `find`, `pwd`, `whoami`, `uname`, `wc`, `date`, `echo`, `which`, `df`, `du`, `uptime`, `ps` (binary base name only), overridable via `config.json` `shell.allowlist`. listed = run without a prompt; unlisted = prompt for approval, then run |
| no shell expand   | `exec.Command(binary, args...)` — never `sh -c`; injection impossible |
| cwd lock          | `cmd.Dir = sess.CWD`; process starts inside the session directory     |
| timeout           | 30 s hard kill via `context.WithTimeout`                               |
| output cap        | stdout+stderr truncated to 64 KB before forwarding to the model        |
| exit errors       | non-zero exit is returned as text, not an error — model sees the output|

**adding a new allowed command**

add it to `shell.allowlist` in `config.json` (auto-approve, no rebuild), or edit `defaultShellAllowlist` in `internal/ipc/tools.go` to change the built-in default. the entry is the binary base name; the dispatch is generic. a command left off the list still runs, but prompts the user for approval first rather than being rejected.

**risk tier**

`execute_shell_command` is **caution-tier**, but the per-command allowlist splits it:
- a command on the auto-approve list executes immediately, like a safe-tier builtin, so keep that list to genuinely low-risk read/build/inspect commands.
- any other command sends a `tool_request` to inari and blocks until the user presses `[y]` or `[n]`.
- allowlisted commands still carry risk: `git reset --hard`, `make clean`, `go generate` can delete or regenerate files, and run without a prompt.
- read-only builtins (`read_file`, `read_lines`, `list_dir`, `find_files`, `grep_file`, `stat_file`) are safe-tier and always execute without prompting.

**rollout order for future expansion**

1. *(done)* allowlist-only `execute_shell_command` - `go`, `make`, `git`.
2. *(done)* per-call approval gating in inari (§8.2) for `execute_shell_command`; safe-tier builtins auto-execute.
3. *(done)* per-command auto-approve: allowlisted binaries run without a prompt, unlisted binaries prompt; the list is config-overridable via `shell.allowlist`.
4. arbitrary bash (destructive tier) only after write tools and blast-radius limits are in production.
4. replace with MCP process connector once the tool-call loop supports it.

**what prompts before running** (anything not on the auto-approve list)

these are no longer hard-blocked at the shell layer; each runs only after the user approves that specific call, so review them when prompted:

- `ssh`, `scp`: remote shell and file-copy reaching outside the host.
- `rm`, `mv`, `chmod`: destructive filesystem ops.
- `curl`, `wget`: network egress.
- `sh`, `bash`, `zsh`, `python`: interpreters that would run arbitrary code.

`execute_shell_command` args are NOT path-validated (only `cmd.Dir` is set to `cwd`), so an approved command can touch paths outside `cwd`; the sandbox confines the file tools, not shell arguments. structural limits that always hold: layer B caps (1 MB/op, 10 calls/turn). a future destructive tier should re-introduce hard-blocks for the interpreter and remote-shell binaries above.

---

## 9. Development & Debugging

For active development, it is often useful to run the components in the foreground across multiple terminals.

### 9.1 Independent Execution

**Terminal 1 — Ollama:**
```sh
ollama serve
```

**Terminal 2 — inarid (foreground):**
```sh
make build
./bin/inari daemon
```

**Terminal 3 — inari TUI:**
```sh
./bin/inari tui
```

### 9.2 Signal Handling
`inarid` handles `SIGINT` (Ctrl+C) and `SIGTERM` cleanly, flushing all session state to disk and closing the Unix socket before exit.

### 9.3 Git Worktrees

> **for AI agents:** development here normally happens inside a git worktree, one at a time. a worktree is a full working checkout, so `make start`/`make build`/`go run` behave identically from within it; live-test by `cd`-ing into the worktree's path (or telling the user that path so they can) rather than testing from the original checkout. keep only one worktree open per feature: close (`ExitWorktree` or `git worktree remove`) the current one before starting the next, so there is never ambiguity about which directory holds the live branch. note: `.claude/` is gitignored, so files under it (e.g. `.claude/settings.json`) are never present in a worktree; edit those directly in the main checkout, not inside a worktree.

---

## 9.4 Release Process

> **for AI agents:** always ask the user which release method to use before proceeding (default: CI). present every command as a manual step for the user to run — do NOT execute `make bump-*`, `make push-tags`, `make release`, or `make update` autonomously. these commands affect shared git history and remote state. guide one phase at a time and wait for confirmation before continuing.

two release methods are available. **CI goreleaser is the default and preferred method.**

| method          | when to use                                                       |
| :---            | :---                                                              |
| CI goreleaser   | default; clean environment; audit trail in GitHub Actions         |
| local goreleaser | CI is broken; no internet on runner; faster iteration            |

**prerequisites:**
- homebrew tap repo checked out locally alongside this repo: `../homebrew-tap`
- CI method: GitHub Actions workflow at `.github/workflows/release.yml` triggers on tag push
- local method: `goreleaser` installed locally; `GITHUB_TOKEN` exported in shell

### phase 1 — prepare

```sh
# 1. ensure you are on a feature branch and all changes are committed
# 2. update internal/version/version.go — set Version to the new vX.Y.Z
# 3. update CHANGELOG.md:
#    - move all unreleased items under a new heading: ## [vX.Y.Z] — YYYY-MM-DD
#    - add a fresh empty unreleased section at the top
# 4. commit the version + changelog update
git add internal/version/version.go CHANGELOG.md
git commit -m "chore: release vX.Y.Z"
# 5. open a PR and merge to main
gh pr create --title "chore: release vX.Y.Z"
# 6. tag the release on main after merge (choose one):
make bump-patch   # vX.Y.Z -> vX.Y.(Z+1)
make bump-minor   # vX.Y.Z -> vX.(Y+1).0
make bump-major   # vX.Y.Z -> v(X+1).0.0
```

### phase 2a — publish via CI goreleaser (default)

```sh
# 7. push the tag — triggers GitHub Actions goreleaser
make push-tags
```

### phase 2b — publish via local goreleaser (alternative)

```sh
# 7. publish directly using local goreleaser (requires GITHUB_TOKEN exported)
make release
```

### phase 3 — update homebrew tap (after goreleaser completes)

shared by both methods. the tap lives at `../homebrew-tap`. run manually — do not automate.

```sh
cd ../homebrew-tap
gmake update FORMULA=inari VERSION=X.Y.Z REPO=ai-inari   # VERSION without the v prefix, e.g. 0.2.0
# note: gmake required — macOS ships with GNU make 3.81 which lacks .ONESHELL support
# note: REPO=ai-inari because the GitHub repo name differs from the formula name
```

this target:
- fetches `inari_X.Y.Z_checksums.txt` from the GitHub release
- patches `Formula/inari.rb` — version string, download urls, and all sha256 values
- commits with `feat: update inari formula to vX.Y.Z`

verify the release:
```sh
brew upgrade mirageglobe/tap/inari
inari version
```

### troubleshooting

**release fails with `422 Validation Failed — tag_name already_exists`**

goreleaser cannot overwrite an existing release. delete the partial release and retrigger:

```sh
make release-reset   # deletes any existing GitHub release for the current tag
make push-tags       # CI method: retriggers goreleaser via tag push
# make release       # local method: run goreleaser directly instead
```

**dry run (no publish)**
```sh
make release-dry   # builds binaries and archives locally, no publish
```

---

## 10. Complexity Score

> only update this table using a large/strong model after significant architectural changes.

| dimension            | score | notes                                                                                            |
| :------------------- | :---- | :----------------------------------------------------------------------------------------------- |
| overall              | 3 / 5 | moderate; single binary with streaming IPC, tool-call loop, approval gating, and multi-view TUI  |
| `internal/ipc`       | 4 / 5 | custom JSON-RPC over UDS, per-stream connections, tool approval protocol, concurrent goroutines  |
| `tui` (inari)        | 3 / 5 | multi-view Bubble Tea; message routing, offline resilience, tool approval prompt, slash commands |
| `internal/session`   | 2 / 5 | session lifecycle and atomic disk persistence; well-bounded                                      |
| `internal/ollama`    | 2 / 5 | HTTP streaming client; straightforward                                                           |
| `internal/provider`  | 2 / 5 | five sandboxed built-in tools (layer 2) with path validation and run allowlist                   |
| `internal/config`    | 1 / 5 | load/save with auto-create; minimal                                                              |
| `internal/mcp`       | 1 / 5 | stub only — `Call()` is a TODO; will rise to 3+ when JSON-RPC dispatch is implemented            |
| `internal/scheduler` | 1 / 5 | semaphore wrapper; minimal                                                                       |
| `internal/audit`     | 1 / 5 | append-only log; minimal                                                                         |

---

## 11. Open Questions

- Audit log format: structured JSON lines vs. human-readable?
- Auth: is owner-only UDS sufficient, or add a local token?
- Session persistence: resolved — one JSON file per session, atomic write+rename,
  stored in `~/.local/share/inari/sessions`. SQLite is the natural next step if
  querying or concurrent access become requirements.
