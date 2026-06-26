# changelog

all notable changes to ai-inari are documented here.
format follows [keep a changelog](https://keepachangelog.com/en/1.1.0/).

---

## [v0.2.0] — unreleased

### added
- herd view: active chat session marked with `▶` indicator column
- chat view: `/describe` command opens session context editor without leaving chat
- chat view: `[copied] N lines` status after clipboard yank shows line count
- tool approval tiering: safe read-only builtins (`read_file`, `list_dir`, `grep_file`, `stat_file`) auto-execute; `run` always requires confirmation
- default system prompt instructs models to respond in plain text with no markdown formatting
- chat view: `ctrl+t` toggles tools panel, `ctrl+p` opens command palette, `ctrl+g` toggles help; `esc` exits tools panel or clears slash input
- chat view: input prefix shows the active mode (`[chat]`, `[tool]`, `[/]`)
- session context: `AGENTS.md` or `.inari/context.md` in a session's `cwd` is injected into the system prompt

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
- IPC over unix domain socket between kitsune (TUI) and inarid (daemon)
- export chat history to file (`/agent export`)

### changed
- consolidated kitsune + inarid into a single `inari` binary
- config moved to `~/.config/inari/config.json`
- `run_command` system prompt lists permitted commands dynamically

---

[v0.1.0]: https://github.com/mirageglobe/ai-inari/releases/tag/v0.1.0
