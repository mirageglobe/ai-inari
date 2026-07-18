# changelog

all notable changes to ai-inari are documented here.
format follows [keep a changelog](https://keepachangelog.com/en/1.1.0/).

---

## [unreleased]

### fixed
- config: new sessions now use the configured `models.thinker` as their default model; `session.create` previously hardcoded `gemma4:e2b` and silently ignored a configured thinker tier.
- concurrency: the `session.stream` error path rolled back the failed turn's user message by truncating `sess.Messages` directly, bypassing the session lock every other mutator holds; a concurrent `session.history`/`session.list` read could race the slice header. now goes through a locked `Session.RemoveLast()`. (arch-review C1)
- doctor: `inari doctor` health-checked the legacy top-level `ollama_base_url` instead of the active endpoint, so it could report green against the wrong backend once a `provider`/`endpoints` profile is in use; it now checks `ActiveEndpoint()`. (arch-review C2)
- session.assign: added the missing provider guard so assigning a model with no configured provider returns a clean "provider not configured" error instead of risking a nil-deref on the model-validation call (defensive; the shipped daemon always has a provider). (arch-review C3)
- tool calling: `/cwd` now marks prior file listings stale. changing a session's working directory rebuilt the system prompt but left the previous cwd's tool results (file listings, file contents) in history; a model would regurgitate that stale listing for the new directory instead of re-running tools (e.g. reporting cwd `.../mirageglobe` but listing `.../ai-inari`'s files). `session.setcwd` now appends a `system`-role marker to history naming the new cwd and flagging earlier listings as stale, so the model re-reads. measured on gemma4:e2b: regurgitation ~3/5 without the marker, 0/5 with it. the marker is only added when the session already has conversation, and is not rendered in the TUI (system role), so it is invisible to the user while still reaching the model.
- tool calling: surface tool output when the model returns an empty final answer. some models (e.g. gemma4:e2b) reliably say nothing after a tool result, treating the result itself as the answer; `handleStream` accumulates each turn's tool output and, when the final round has empty content, streams and persists that output so the user sees the listing/file/matches they asked for instead of a blank reply. exposed by the tool-call guard + fallback now making tools actually fire, where the model previously narrated instead.
- tool calling: prompt-based fallback in `handleStream` - when a round returns text and no native `tool_calls`, the content is scanned for a written-out invocation (`list_dir{path: "."}`, `read_file(path='x')`, etc.); a match on a known tool is dispatched through the normal `execTool` gate (approval + cwd sandbox still apply). persisting the structured call rather than the raw text also heals the session history, so the model stops few-shotting off its own narration. this is the hard guarantee behind the system-prompt guard: it makes recovery deterministic regardless of the model's sampling temperature. fenced code blocks (` ```bash ls``` `) are deliberately not parsed (an illustrative command is ambiguous versus intent); prose that merely names a tool is ignored (a match requires an argument).
- tool calling: the cwd system prompt injected the file tree with a `name(args)` prose tool list, which let the model answer file/dir questions from the tree in text and modelled a text-shaped call; a small model (e.g. gemma4:e2b) then few-shot off its own text turns and stopped emitting native `tool_calls`, printing ` ```bash ls``` ` instead of running it. the tree is now framed as a stale orientation snapshot with an explicit "never answer from it, always call a tool" directive, and the prose signatures are dropped (the native tool schema is the real declaration). measured: on the exact failure sequence, native tool-call rate went from 0/3 (old prompt) to 3/3 (new prompt). note: gemma4:e2b runs at temperature 1.0, so this raises the odds strongly but is not a per-run guarantee; a prompt-based tool-call fallback is the hard guarantee (see SPEC roadmap).

### changed
- perf(tui): the chat view no longer re-wraps the entire scrollback on every streamed token. the wrapped display base is cached for the lifetime of a stream and only the in-progress line is re-wrapped per token, cutting per-turn render cost 3x-15x (72ms -> 4.8ms for a 200-token reply in a 1000-line session) and allocations up to 5.5x, with byte-identical output. measured via a new `BenchmarkStreamTurn`; `/api/show` (P3, ~7-29ms/turn) memoize is tracked as a follow-up.

### added
- cli: `inari chat --session <id> --message <text>` runs one turn headlessly and prints the assistant reply (no TUI); `--message -` reads stdin and `--json` wraps it as `{"reply": ...}`. it ensures a background daemon is up, then calls the non-streaming `session.chat` RPC (a plain model round-trip, no tool-call loop) so the path is deterministic and never blocks on an approval prompt - a scriptable entry point for automation and tests. `--session` is required and must already exist.
- config: per-project overlay `.inari/config.json` in a session's working directory; a restricted overlay honoring only `context.system_prompt` (replaces the global prompt for sessions in that dir) and `exclude_dirs` (extra file-tree skips). infra/security fields (socket, endpoints, provider, `shell.allowlist`, models, ...) are deliberately never read from a project file, so an untrusted cloned repo cannot widen the shell allowlist or redirect the backend.
- chat: `/cwd <path>` switches a session's working directory on the fly; inarid validates the path, rebuilds the file-tree + project-context system prompt for the new tree, and re-points the tool sandbox. the `[context]` line and builtin-tool availability update immediately.
- chat: after 60s idle the status line shows a rotating `hint:` usage tip (e.g. `try /compact to summarise a long chat`), cycling every 60s; any keypress or streamed token clears it and resets the timer. hints never override a recap, error, or live reply.
- chat: `esc` interrupts an in-flight response (while waiting or mid-stream); the daemon cancels the Ollama generation via a new `session.interrupt` RPC, keeps whatever text streamed so far, and ends the turn cleanly. frees the model immediately instead of waiting for the full reply.
- chat: `!<command>` runs a real shell command in the session's working directory, bypassing the model (e.g. `!git status`, `!ls | wc -l`). a real `sh -c` shell so pipes/globs/redirects work; output shows in the chat and is recorded in history so the model sees the result. safe because the command is user-authored, not model-authored: it keeps the cwd lock, 30s timeout, and 64KB output cap, and skips the allowlist prompt the model's `execute_shell_command` uses (the user typing the command is the approval). requires a cwd; runs even while offline.

### changed
- api: malformed-params RPC errors now consistently return JSON-RPC `-32602` (invalid params); a few handlers previously returned `-32600` (invalid request) for the same condition. error messages are unchanged and no client reads the code today, so this is a wire-correctness fix with no user-visible effect.
- refactor: removed the dead scheduler wiring from the daemon and `ipc` (it was created and passed to the server but never called; the memory-budget throttle it implied was never implemented). `config.json`'s `memory_budget_mb` is now documented as reserved for the planned throttle (SPEC roadmap) rather than implying an active limit. no runtime behaviour change: concurrent sessions were already unthrottled.
- tui: `/help`, `/describe`, `/logs`, and the `/tools` panel now open as centered pop-up modals over the current screen, instead of full-screen view swaps (`describe`/`logs`) or an inline footer hint (`tools`); the chat or agents view stays underneath and is revealed on close. every pop-up modal (also the model selector and theme picker) now closes on both `q` and `esc` and returns to the view it was opened from.
- perf: the runaway-loop detector (`hasRepeatedTail`) no longer copies the entire accumulated reply to `[]byte` on every streamed token; it slices the 128-byte tail first, turning an O(reply-length) per-token copy into a constant one. (arch-review P1)
- perf: the model context window (`/api/show`) is now memoised per model instead of refetched on every turn; it is a static model property, so this removes a blocking HTTP round-trip from first-token latency on each stream. (arch-review P3)

## [v0.3.0] - 2026-07-13

### added
- chat: reopening a session left idle 10+ min shows a one-line `[recap]` of where the conversation left off (generated on demand via `session.recap`, history untouched)
- context window: inarid detects a model's max context (`/api/show`) and requests a capped `num_ctx` (`min(max, 8192)`) on each chat, up from Ollama's default; the chat footer shows the effective window over the model max (e.g. `ctx 8192/40960`)
- agents view: `[/]` filters the session list live by name or model (case-insensitive); `[esc]` clears, `[enter]` keeps the filter; footer shows `[filter] <query> (N of M)`
- herd view: active chat session marked with `▶` indicator column
- chat view: `/describe` command opens session context editor without leaving chat
- chat view: `[copied] N lines` status after clipboard yank shows line count
- tool approval tiering: safe read-only builtins (`read_file`, `list_dir`, `grep_file`, `stat_file`) auto-execute; `execute_shell_command` auto-runs binaries on the `shell.allowlist` (config.json, defaults to common read/build commands) and prompts for `[y]`/`[n]` on anything else, which then runs on approval (`curl`/`wget` left off the default so network egress always asks)
- default system prompt instructs models to respond in plain text with no markdown formatting
- chat view: `ctrl+t` toggles tools panel, `ctrl+p` opens command palette, `ctrl+g` toggles help; `esc` exits tools panel or clears slash input
- chat view: input prefix shows the active mode (`[chat]`, `[tool]`, `[/]`)
- session context: `AGENTS.md` or `.inari/context.md` in a session's `cwd` is injected into the system prompt
- model selector: recommended-but-not-pulled models are shown inline marked `[pull]`; selecting one triggers `ollama pull` via inarid with live progress, then assigns it
- model selector: `[pull]` list now spans the full curated catalog (SPEC.md §6.1, all hardware tiers, tagged with tier/role), not just the detected tier
- model selector: `[d]` deletes a downloaded model from local disk (`DELETE /api/delete`) behind a `[y]`/cancel confirm, reclaiming space; distinct from `[u]` unload (memory only), and warns when the model is assigned to the target session
- model selector: `status` column (`loaded` / `downloaded` / `[pull]`) and a `notes` column (curated §6.1 notes, single-line truncated) alongside `model` and `est. vram`
- curated model table: add `gemma4:e2b` alongside `gemma4:e4b` at the 8gb general tier
- themes: `emerald`, `cyan`, and `mono` added alongside `purple`, `amber`, `slate`, `rose` (cycle with `[t]`)
- chat view: spinner shows `loading <model>...` while inarid cold-loads the assigned model into backend memory, switching to `thinking...` once generation begins

### changed
- modal overlays (model selector, agents) share one capped inner width; wide terminals no longer stretch popups past the 100-col budget
- model unload moved from the `/model unload` slash command to a `[u]` hotkey in the model selector modal (shown only when the session has a model assigned)
- session names are now `<adjective> <noun>` (e.g. `jade fox`) instead of `<adjective> agent`, giving N*M combinations before the numbered fallback

---

## [v0.1.0] — unreleased

### added
- herd view: session table with live VRAM and model expiry stats
- chat view: multi-turn conversation with configurable ollama models
- mouse drag text selection with clipboard yank in chat viewport
- tool approval gating: user prompt before executing agent tools
- builtin tools: `read_file`, `write_file`, `list_dir`, `grep_files`, `file_stat`, `run_command`
- `run_command` allowlist covering common read-only shell commands
- model capability tags (`[tool]`, `[vis]`) fetched via ollama `/api/show` shown in herd table
- model selector modal overlay on herd view
- `/clear` and `/compact` chat commands
- `/default chat` shortcut to open first session's chat
- command mode with autocomplete suggestions (`/` prefix) in both herd and chat views
- scrollbar indicator in chat viewport
- cwd sandbox line in footer (shows active working directory)
- footer labels (`[session]`, `[cwd]`, `[status]`, `[hint]`) always visible, uniform grey style
- theme cycling (`/theme`)
- config auto-creation at `~/.config/inari/config.json` when missing
- IPC over unix domain socket between inarit (TUI) and inarid (daemon)
- export chat history to file (`/agent export`)

### changed
- consolidated inarit + inarid into a single `inari` binary
- config moved to `~/.config/inari/config.json`
- `run_command` system prompt lists permitted commands dynamically

---

[v0.1.0]: https://github.com/mirageglobe/ai-inari/releases/tag/v0.1.0
