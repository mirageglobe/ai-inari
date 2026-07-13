# changelog

all notable changes to ai-inari are documented here.
format follows [keep a changelog](https://keepachangelog.com/en/1.1.0/).

---

## [unreleased]

### added
- chat: after 60s idle the status line shows a rotating `hint:` usage tip (e.g. `try /compact to summarise a long chat`), cycling every 60s; any keypress or streamed token clears it and resets the timer. hints never override a recap, error, or live reply.

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
