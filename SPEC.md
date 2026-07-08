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

## 3. Roadmap

### Milestones

#### M1 — UDS Bridge
- [x] `[inarid]` starts and binds UDS socket.
- [x] `[kitsune/inarid]` connects and performs handshake.
- [x] `[kitsune/inarid]` basic ping/pong JSON-RPC round-trip.

#### M2 — Herd UI
- [x] `[kitsune]` Bubble Tea table renders active sessions.
- [x] `[kitsune/inarid]` sessions update in real time from daemon events.
- [x] `[kitsune]` keyboard navigation (select, quit).

#### M3 — Ollama Integration
- [x] `[inarid]` daemon POSTs to Ollama `/api/chat` and streams tokens.
- [x] `[kitsune/inarid]` token stream forwarded to kitsune chat view.
- [x] `[inarid]` semaphore throttle enforces memory budget.

#### M4 — MCP Loader
- [x] `[inarid]` `config.json` parsed at startup; created from defaults if absent.
- [x] `[inarid]` connectors spawned as child processes.
- [x] `[inarid]` config path moved to `~/.config/inari/config.json` (XDG-compliant).

#### M5 — Chat View
- [x] `[kitsune]` interactive `i` view wires to Head Inari (Thinker tier).
- [x] `[inarid]` message history scoped to session.
- [x] `[kitsune/inarid]` detach/reattach preserves session state.

### Near-term
- [ ] `[inari]` `[medium]` **agents view and model selector as chat-hosted popups**: generalise the existing `show*` overlay pattern already in `tui/model.go` (`showModelSelector`/`showThemePicker`/`showHelp` flags gate `tea.KeyMsg` routing to the modal's own `Update`, intercepting `esc` to close before any base-view hotkey sees the key) so chat becomes the single persistent base view. replace the `viewAgents` full-screen switch with a `showAgents bool` plus `Agents.RenderModal()` overlay rendered over chat, and change chat's `ctrl+o` to set `showModelSelector` while staying on `viewChat`, instead of switching `m.current` to `viewModels`. no new key-suppression mechanism is needed; the routing block already returns early after forwarding to the active popup.
- [x] `[inarid]` `[easy]` add `gemma4:e4b` as the default master local model always — set as the thinker-tier default in config and fallback when no model is assigned to a session
- [ ] `[inarid]` `[medium]` consider adding vLLM as an alternative backend to Ollama — vLLM is OpenAI-compatible and may offer better throughput on CUDA hardware; evaluate alongside the local endpoint profiles item as a concrete second backend candidate
- [ ] `[inarid]` `[medium]` consider exposing Ollama as an MCP server so other models — local or cloud — can be invoked as tools by the default master model (`gemma4:e4b`); this lets the thinker delegate sub-tasks to specialised models (e.g. a coding worker) via the existing MCP tool-call loop rather than requiring a separate session
- [ ] `[inari]` `[medium]` consolidate all commands to be in chat view. chat view is the main view
- [ ] `[inari]` `[medium]` session search and filter in agents view
- [ ] `[inari/inarid]` `[hard]` long-term task planning from high-level prompts
- [ ] `[inari/inarid]` `[medium]` interrupt in chat for messages
- [ ] `[inarid]` `[medium]` recap/summary when a chat session has been idle for 10+ mins
- [ ] `[inari]` `[hard]` **chat viewport text selection** — mouse drag within the chat viewport highlights a region and copies it to the system clipboard (`pbcopy`/`xclip`). requires mapping raw terminal cell coordinates to viewport content rows (accounting for scroll offset and `ansi.Hardwrap` line splits), then re-parsing ANSI sequences to locate byte ranges and inject highlight styles mid-sequence; character-level selection. line-granularity selection (whole lines only) is a viable medium-difficulty stepping stone.
- [ ] `[inarid/inari]` `[medium]` **ollama context window detection and optimum setting** — on session creation (or model change), inarid queries the model's `num_ctx` parameter via the Ollama `/api/show` endpoint; the detected value is surfaced in the inari chat view alongside the token count. inarid also exposes a per-session override that sets `num_ctx` in each `/api/chat` request, defaulting to a sensible optimum (e.g. 8192 for worker-tier models, 4096 for sensor-tier) rather than Ollama's built-in default. the inari UI allows the user to view and adjust this value per session.
- [ ] `[inarid]` `[medium]` **local server endpoint profiles** — support named endpoint profiles in `config.json` (e.g. `ollama`, `lmstudio`, `llamacpp`) each specifying a `base_url`, optional `api_key`, and any provider-specific headers or path overrides; inarid selects the active profile via a top-level `provider` field and routes all model requests through it. this is a prerequisite for the provider abstraction idea below and allows users to switch between local inference backends without rebuilding or patching inarid.
- [ ] `[inarid]` `[medium]` **ollama runtime env tuning** — investigate and expose three Ollama environment variables as first-class inarid config fields: `OLLAMA_MAX_LOADED_MODELS` (default `3`; caps how many models stay resident in VRAM/RAM simultaneously), `OLLAMA_NUM_PARALLEL` (default `1` for low-RAM setups, `4` for high-throughput; controls concurrent request slots per model), and `OLLAMA_KEEP_ALIVE` (default `5m`; how long an idle model stays loaded). inarid should read these from `config.json` under an `ollama` block and pass them as environment variables when spawning or communicating with the Ollama process, or document them as required host-level env vars if inarid does not manage the Ollama process lifecycle. the inari settings view (or a `--ollama-info` flag on inarid) should surface the active values so the user can tune memory vs. throughput trade-offs without needing to know the underlying env var names.
- [ ] `[inarid]` `[medium]` **role-based model assignment in session management**: curated model lists and system-tier recommendations (§6.1) already ship in the model selector (`tui/views/curated.go`); remaining scope is letting a session record an assigned role (coding, general) and default to the recommended model for that role.
- [ ] `[inarid/inari]` `[medium]` **split oversized files**: four files still exceed the ~150-line limit: `tui/views/chat.go` (557), `tui/model.go` (463), `tui/views/agents.go` (394), `cmd/inari/main.go` (306); `internal/ipc/server.go` (922 -> 204) and `internal/ipc/client.go` (433 -> 176) are already split down close to the limit. split by responsibility, starting with `chat.go` (now the largest).
- [ ] `[inarid/inari]` `[medium]` **test coverage for untested packages**: 6 of 12 packages still have zero tests: `mcp`, `ollama`, `provider`, `version`, `tui`, `cmd/inari`; `tui/views` gained `curated_test.go` but its render seams (chat, agents) remain untested. add unit coverage, prioritising `provider`/`ollama` (request shaping) and the `tui/views` render seams.
- [ ] `[inarid]` `[medium]` **global context configuration** - load global system prompts, default model settings, and context parameters from `~/.config/inari/config.json` for any Inari launch.
- [ ] `[inarid]` `[medium]` **local project-scoped configuration** - read project-scoped settings (e.g. custom prompts, file exclusions) from a local `.inari/config.json` in the session's working directory, overriding global settings where fields overlap.

- [ ] `[inari]` `[medium]` **unified command vocabulary** — footer hints and slash commands are currently view-specific (e.g. `/agent describe` in agents, `/describe` in chat) with no shared naming convention. standardise to a single command set where commands are contextually enabled/disabled across all views rather than defined per-view; the footer hint bar already dims unavailable commands, so the rendering model supports this. prerequisite: audit all slash commands across agents and chat views and agree on canonical names.

### Ideas
- [ ] `[inarid]` **MCP tool-call dispatch** — `internal/mcp/host.go` `Call()` is a TODO stub; audit logging exists but actual JSON-RPC dispatch over stdio is not implemented. complete to fulfil M4.
- [ ] `[inarid]` `[medium]` **model loop / EOF prevention** — some models enter repetitive generation loops (e.g. `for_for_for...`) that exhaust the context window and terminate the stream with an EOF error. three mitigations to investigate: (1) set `repeat_penalty` (1.3–1.5) and `num_predict` cap in the ollama request options to penalise and hard-limit runaway output; (2) add a stream-side n-gram detector in `handleStream` that buffers the last N tokens, identifies a repeating sequence appearing 3+ times consecutively, and cancels the stream with a graceful error before EOF is hit.
- [ ] `[inarid]` **MCP filesystem connector (layer 3)** — once the tool-call loop exists, replace built-in tools with `@modelcontextprotocol/server-filesystem` spawned via mcp-go. this is a natural extension of the MCP integration work below.
- [ ] `[inarid]` **destructive action prevention (§8.2)**: cwd enforcement (`sandboxPath` in `internal/ipc/tools.go`) and a tool-call loop cap (`maxToolRounds = 10` in `internal/ipc/stream.go`) are shipped, alongside per-call size caps; remaining scope is a true file-op-count cap and dry-run previews for caution-tier tool-calls. risk-tiered auto-approval is done (safe builtins auto-execute, `run` always confirms)
- [ ] `[inarid]` multiple models per session — allow attaching different models to a single session for collaborative discussions and task execution
- [ ] `[inarid]` MCP integration — replace `internal/mcp` with `github.com/mark3labs/mcp-go`; connectors (Linear, Slack, Google Drive, etc.) configured via `config.json`
- [ ] `[inarid]` **prompt-based tool calling** — for models without native function-calling support, inject tool definitions as plain text into the system prompt and set `format: "json"`; inarid parses the JSON response to detect tool calls. select mode via session config or auto-detect from model name. makes layer 2 work on any instruction-following model (hermes-3-pro, qwen3-coder, etc.)
- [ ] `[inarid]` **provider abstraction** — the `Provider` interface already exists (`internal/provider/provider.go`: Chat, ChatStream, LoadModel, UnloadModel, ListModels, ListRunning, Ping) and inarid's core already talks only to it. remaining work is a second concrete provider (vLLM, LM Studio, llama.cpp server, or a cloud API) selected via `provider` in `config.json`; overlaps with the local endpoint profiles item above.
- [ ] `[inarid]` multi-model routing — sensor tier classifies intent, dispatches to worker or thinker
- [ ] `[inarid]` `[medium]` **auto-compression threshold** — automatically trigger `/compact` when the session context exceeds a configurable token threshold (e.g. 80% of `num_ctx`); inarid monitors context size after each turn and fires the summarisation pipeline without user intervention.
- [ ] `[inarid]` **context caching / compression / optimisation** — investigate strategies to reduce prompt size and improve response speed: KV-cache reuse across turns, selective message eviction, rolling summary compression, and prefix caching at the provider level; goal is lower latency and higher effective context utilisation without degrading response quality
- [ ] `[inarid]` `[hard]` **vector store / RAG context** — replace or augment flat JSON session storage with a semantic retrieval layer. progression: (1) sqlite as structured store; (2) sqlite-vec (sqlite vector extension) for local embeddings — single file, no external service, fits the Go daemon cleanly; (3) full RAG pipeline with chunking, a local embedding model (~100MB sensor-tier), and ranked context injection. at query time, the user message is embedded and the top-k semantically similar chunks are injected into the prompt rather than the full history dump. benefit: small models see only relevant context, reducing token pressure and improving response quality. a global "master context" store (outside any cwd) could be maintained alongside per-session history, giving all sessions access to persistent personal or cross-project knowledge.
- [ ] `[inarid]` **task difficulty/effort classification** — investigate how to define and score task difficulty, complexity, and effort (e.g. token count, tool-call depth, reasoning hops) so inarid can automatically select the appropriate model tier (sensor → worker → thinker) rather than relying on manual session config
- [ ] `[inari/inarid]` session tagging - apply labels to sessions for grouping and quick filtering; plain search is already covered by the near-term agents search/filter item
- [ ] `[inari/inarid]` **rename session** — allow the user to rename an existing session from the agents view; inari sends a `session.rename` RPC to inarid which updates the stored session name and propagates the change back to all open views.

### Done
- [x] `[inari]` `[easy]` **pre-context line in chat** - the injected file-tree/project-context system prompt is summarised as a single `[context] cwd: <path> (+ project context)` line and rendered as the first line of the chat viewport (`buildContextLine` in `tui/views/chat_helpers.go`); persists across `/clear` and shows for a brand-new session with no history yet.
- [x] `[inarid/inari]` `[medium]` **pull models from the UI** - the model selector appends recommended-but-not-pulled models to the table marked `[pull]`; selecting one triggers `ollama pull` via a new `model.pull` RPC (dedicated connection, mirrors `session.stream`), streams download progress into the status line, then refreshes the list and assigns the model as usual.
- [x] `[kitsune]` `[easy]` **rename herd to agents** - the `Herd` view/type became `Agents` (files `herd.go`/`herd_view.go`/`herd_cmds.go`/`herd_slash.go` renamed to `agents*.go`); the `/herd` slash command used from chat to navigate back is now `/agents`; the `[herd]` session-line label is now `[agents]`. in-view sub-commands (`/agent add`, `/agent chat`, etc.) were already named `/agent` and are unchanged.
- [x] `[inarid]` `[easy]` **daemon idle auto-shutdown** - inarid exits on its own after `idle_shutdown_mins` (default 30) with no client activity; any RPC including ping heartbeats resets the watchdog, so the daemon stays up while a kitsune is open and exits after the TUI closes. `0` falls back to the default, a negative value disables it.
- [x] `[inarid/kitsune]` `[easy]` **package `doc.go` coverage** - every package now carries a `doc.go` with the canonical `it owns:` / `it does NOT own:` statement; pre-existing inline package comments were demoted to file-level notes so `go doc` shows one ownership block per package.
- [x] `[kitsune]` `[easy]` **download context and copy response** - chat slash commands `/copy` (copies the latest assistant response to the clipboard) and `/save` (writes full session history to `~/.local/share/inari/exports/`, reusing the herd export path); both report success or failure in the status line.
- [x] `[kitsune]` `[easy]` **surface swallowed errors** - clipboard-copy failures in the chat viewport and theme-save failures now show a `[warn]` status instead of failing silently; the theme save was also made synchronous, removing a config-write race.
- [x] `[kitsune]` `[easy]` **chat navigation shortcuts**: `ctrl+t` toggles the builtin tools panel, `ctrl+p` opens the slash command palette, `ctrl+g` toggles the help overlay; `esc` exits the tools panel or clears an in-progress slash command. ctrl-prefixed keys avoid the bare-key text-input clash (open issue). `ctrl+m` was dropped because terminals deliver it as carriage-return and would shadow `[enter]` send.
- [x] `[kitsune]` `[easy]` **chat input mode indicators**: the chat entry prefix reflects the active mode; `[/] ❯` while composing a slash command, `[tool] ❯` while the builtin panel is open, otherwise `[chat] ❯`.
- [x] `[inarid]` `[easy]` **agent context file**: on session creation with a `cwd`, inarid reads the first of `AGENTS.md` or `.inari/context.md` (sandboxed, capped at 8 KB) and appends it to the system prompt under a `project context:` heading. absent file is a graceful no-op.
- [x] `[fox]` CLI removed — functionality superseded by kitsune TUI
- [x] `[kitsune]` `[easy]` **chat-first startup with herd accessible separately** — on launch, kitsune opens directly into the chat view for the default (or most recent) session rather than the herd table; the herd view is reachable via `/herd` slash command from within chat.
- [x] `[kitsune]` `[easy]` **explicit hotkeys for view switching; remove esc-to-herd** — replaced implicit `esc` exit from chat to herd with `/herd` slash command; `esc` in chat dismisses overlays only.
- [x] `[inarid]` `[easy]` **daemon lifecycle commands** — `inarid start` and `inarid stop` subcommands; `start` forks inarid to the background and writes a PID file; `stop` reads the PID file, sends `SIGTERM`, and removes the file.
- [x] `[kitsune]` `[easy]` **mouse scroll for chat buffer** — mouse wheel scroll on the chat viewport.
- [x] `[kitsune]` `[easy]` **cpu and memory in top bar** — system-wide cpu and memory polled at ~2s intervals; displayed in top bar as `cpu N%  mem X / Y gb`.
- [x] `[kitsune/inarid]` **context compression (ponder)** — `/compact` in chat triggers inarid to summarise conversation history via the session's own model, replacing old turns with a compact summary; system prompt is preserved.
- [x] `[kitsune]` thinking spinner in chat session while waiting for a response
- [x] `[kitsune/inarid]` offline detection in chat — when inarid is unreachable, the hint line shows "inari is offline" and sends are blocked until connectivity is restored
- [x] `[kitsune/inarid]` streaming chat — `session.stream` RPC over dedicated per-call UDS connections; kitsune renders tokens as they arrive
- [x] `[kitsune]` title bar wave animation — per-character purple gradient drifts across the kitsune title at 200ms intervals
- [x] `[kitsune/inarid]` filesystem context (layer 1) — shallow file tree injected into system prompt at session creation; kitsune passes `cwd`, inarid walks up to 3 levels (skipping `.git`, `node_modules`, etc.)
- [x] `[easy]` add `LICENSE` file — bsl; copyright holder: Jimmy MG Lim
- [x] `[kitsune]` `[medium]` themes — a small set of built-in colour themes (e.g. default purple, amber, slate, rose); cycle through them with `[t]` from any view; theme is stored in config.json and applied at startup
- [x] `[kitsune]` `[easy]` help overlay — `[?]` opens a modal listing all hotkeys for the current view; `[esc]` or `[?]` dismisses it
- [x] `[kitsune]` `[easy]` quick-start fox — if the herd view has no sessions, automatically create a default session so the user can start chatting immediately without a manual create step
- [x] `[kitsune]` `[easy]` fox status line in herd view — render a `<session-name> > ` line directly above the hotkey hint bar, showing the name of the currently selected kitsune session as a prompt-style prefix; updates as the table cursor moves
- [x] `[kitsune]` `[easy]` export chat history to file — `[e]` in herd view fetches full message history via `session.history` RPC, formats as plain text (`role: content` per message, `---` separator), and writes to `~/.local/share/inari/exports/<session-name>-<timestamp>.txt` (XDG data dir); path is shown in the status bar on success
- [x] `[kitsune]` `[easy]` show current token count in chat
- [x] `[inarid]` **filesystem tool-call loop (layer 2)** — inarid declares read-only tools (`read_file`, `list_dir`) in the Ollama API request for sessions that have a working directory set. when Ollama returns a tool-call instead of text, inarid executes the tool (sandboxed to the session's `cwd`), appends the result as a `tool` message, and re-sends to Ollama — looping until a final text response arrives. write operations are explicitly out of scope at this stage.
- [x] `[inarid]` **extended layer-2 tools** — `grep_file` (regex search across files in cwd) and `stat_file` (size, mtime, type) added alongside `read_file`, `list_dir`, and `run`; all sandboxed to session `cwd`.
- [x] `[inarid]` **`run` builtin** — allowlisted bash execution: `go`, `make`, `git`, `date`, `echo`, `pwd`, `whoami`, `uname`, `wc`, `curl`, `wget`, `find`, `ps`, `ls`, `cat`, `df`, `uptime`, `which`; `exec.Command` (no shell expansion); 30 s timeout; 64 KB output cap. caution-tier per §8.3.
- [x] `[kitsune]` `[medium]` **tool approval gating** — when inarid needs to execute a tool during a stream, it sends a `tool.approval_request` message; the stream pauses and kitsune renders an approval prompt replacing the hint bar; the user presses `[y]`/`[n]` to approve or reject before execution resumes. all keys are absorbed while approval is pending.
- [x] `[inarid]` `[easy]` **auto-create config** — if `~/.config/inari/config.json` does not exist at startup, inarid creates it with defaults (socket, memory budget, ollama url, default model tiers, theme); the user gets a ready-to-edit file rather than a startup error.
- [x] `[kitsune]` `[easy]` **shared footer component** — `tui/views/footer.go` owns `RenderFoxLine`, `renderFooter`, and `renderCWDLine`; all views use it. the footer now shows `label | name | model | tokens | cwd` in one line, followed by a dedicated cwd sandbox line when a session directory is set.
- [x] `[kitsune]` `[easy]` show cwd in status bar — cwd is displayed in the footer of both chat and herd views; rendered via `renderCWDLine` as `[cwd] <path>`.
- [x] `[kitsune]` `[easy]` slash commands — `/` in the chat input opens a command suggestion list (`/clear`, `/compact`, `/model change`, `/tools`); selecting with tab or enter executes the command. `/clear` wipes session history; `/compact` summarises the conversation via the session's own model.
- [x] `[kitsune]` `[easy]` **input history navigation** — `↑`/`↓` in the chat input field cycles through previously sent messages; history is per-session and in-memory for the session lifetime.
- [x] `[kitsune]` `[easy]` **model capability tags in herd view** — after sessions load, inarid calls `ollama.show` (→ `POST /api/show`) per model; the response `capabilities` array is cached in the Herd. the model column renders `[tool]` and `[vis]` suffixes where applicable. fetches are lazy and per-model, so models without caps are unaffected.
- [x] `[inarid]` **`ollama.show` RPC** — new `ollama.show` handler in the server; `ModelCaps(model)` added to the `Provider` interface and implemented in the Ollama client. the IPC client exposes `ModelCaps(model string) ([]string, error)`.

### Open Issues
- [ ] `[inarid/inari]` track and manage known issues and bugs
- [ ] `[inari]` `[medium]` **no general focus-aware key suppression**: the concrete `?`/`t` clash was worked around (theme cycling moved off the bare `t` key, bare `?` scoped away from chat/agents, ctrl-prefixed shortcuts chosen instead; agents view already absorbs keys into its textinput while focused), but there is still no general mechanism to suppress global hotkeys when a text input is focused, so a future bare-key binding could reintroduce the clash.
- [ ] `[inari/inarid]` `[medium]` **model swap returns ollama error for some models** — swapping to certain models (e.g. deepseek) produces an ollama error; swapping between gemma4 and qwen works. likely a model-name format mismatch or missing pull — inarid should validate the model is available via `/api/tags` before sending the assign RPC and surface a clear error if not.

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
    ├── scheduler       — semaphore-based memory budget
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
   {"tool_approval_request":{"tool":"run","args":{"command":"go","args":["test","./..."]}}}
   {"token":"Tests passed."}
   {"done":true}
   ```
   when a `tool_approval_request` frame arrives, inari pauses rendering and waits for the user to press `[y]` or `[n]`. it then sends `{"tool_approved":true}` or `{"tool_approved":false}` back over the same connection. inarid blocks until it receives the response before executing or skipping the tool call.
4. on `done`, inarid has persisted the full reply to the session store; client closes the connection
5. on error, inarid sends `{"error":"<message>"}` and closes

inari opens one dedicated connection per active `session.stream` call. the shared `Client` connection remains exclusively for control RPCs and is never blocked by in-flight streams.

**multiple concurrent streams:**

within a single inari TUI, the user can spawn multiple named chat sessions (each displayed as a row in the agents view). each session is an independent agent — it can have a model assigned and an active generation in flight simultaneously. because each `session.stream` call uses its own dedicated UDS connection, all sessions can stream concurrently without blocking one another. inarid handles each stream in its own goroutine via the accept loop.

**message routing in inari:**

token messages (`ChatTokenMsg`, `ChatDoneMsg`) carry a `SessionID` field. the root model routes them directly to the correct `Chat` view in `m.chats[sessionID]` — regardless of which view is currently displayed. this allows background sessions to accumulate tokens invisibly; when the user switches back, the chat view already shows the partial or complete response.

### 4.5 Concurrency & Scheduling

- Each Ollama session runs in its own goroutine.
- A semaphore gates concurrent sessions based on configured memory budget.
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

| tool            | input                               | output                                   |
| :---            | :---                                | :---                                     |
| `read_file`     | `{path}`                            | file contents (text only)                |
| `list_dir`      | `{path}`                            | directory listing (names only)           |
| `grep_file`    | `{path, pattern}`                   | matching lines with filename and line no |
| `stat_file`     | `{path}`                            | size, mtime, type                        |
| `run`   | `{command, args[]}`                 | stdout+stderr, exit code as text         |

**naming convention:** tool names follow `verb_noun` (e.g. `read_file`, `list_dir`) — reads as an instruction and aligns with common tool-calling schemas (MCP, OpenAI). exception: `run` stands alone as it takes a command argument rather than a path.

all tools are sandboxed: paths are resolved relative to `cwd` and must not escape it (no `../` traversal). `run` is additionally gated by an allowlist (see §8.3). write operations are out of scope.

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
| Agents   | -   | default view; table of all agents with model and status        |
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

**chat slash commands:**

| command         | effect                                                            |
| :-------------- | :---------------------------------------------------------------- |
| `/clear`        | wipe session message history                                      |
| `/compact`      | summarise history via the session's own model; replaces old turns |
| `/model select` | open model selector for this session                              |
| `/tools`        | list active built-in tools for this session                       |

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

- **Sensors (Routing):** Low-priority, small context window. Used for intent classification.
- **Workers (Execution):** Mid-priority, standard context. Used for parallel task execution.
- **Thinkers (Reasoning):** High-priority, large context. Used for interactive chat and complex architectural reasoning.

Memory budget is enforced via `memory_budget_mb` in `config.json`. The scheduler blocks model loading if the budget would be exceeded.

### 6.1 Ollama Model Curation

curated picks by hardware tier and role. pull via `ollama pull <tag>`. prefer `q4_k_m` quant unless the tier has headroom for `q8_0`.

#### general

| tier | model       | size   | notes                                                   |
| :--- | :---------- | :----- | :------------------------------------------------------ |
| 32gb | gemma4:27b  | ~15gb  | google moe; near-frontier chat and review               |
| 16gb | phi-4:14b   | ~8gb   | microsoft; strong multi-file reasoning                  |
| 8gb  | gemma4:e4b  | ~2.7gb | 4.5b effective; fast routing and quick queries          |
| 8gb  | gemma4:e2b  | ~1.5gb | 2b effective; leaner and faster than e4b, lower quality |
| 4gb  | llama3.2:3b | ~2gb   | meta; best chat and reasoning within 4gb                |

#### coding

| tier | model                    | size   | notes                                        |
| :--- | :----------------------- | :----- | :------------------------------------------- |
| 32gb | qwen3.6:27b-coding-nvfp4 | ~18gb  | alibaba; near-frontier generation and review |
| 16gb | deepseek-r1:14b          | ~9gb   | r1-671b distil; strong coding and reasoning  |
| 8gb  | deepseek-r1:8b           | ~5gb   | r1-671b distil; fits 8gb; coding+reasoning   |
| 4gb  | llama3.2:3b              | ~2gb   | meta; best within 4gb budget                 |

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

| tier        | current tools                                          | inarid behaviour                                                |
| :---        | :---                                                   | :---                                                            |
| safe        | `read_file`, `list_dir`, `grep_file`, `stat_file`      | execute immediately, no approval round-trip, log result         |
| caution     | `run`                                                  | send `tool_request` to inari; block until `tool_approved`; rejection logged |
| destructive | (none yet — future write tools)                        | always require confirmation; shown in red in inari            |
| forbidden   | process spawn, network outside ollama/mcp, shell exec  | hard-rejected; never routable                                   |

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

### 8.3 run — allowlisted bash execution

`run` lets the model invoke a fixed set of development commands inside the session's `cwd`. it is the boundary between read-only filesystem tools and write/execute capability.

**implemented constraints**

| constraint        | detail                                                                                                           |
| :---              | :---                                                                                                             |
| allowlist         | `go`, `make`, `git`, `date`, `echo`, `pwd`, `whoami`, `uname`, `wc`, `curl`, `wget`, `find`, `ps`, `ls`, `cat`, `df`, `uptime`, `which` — binary base name only; all others hard-rejected |
| no shell expand   | `exec.Command(binary, args...)` — never `sh -c`; injection impossible |
| cwd lock          | `cmd.Dir = sess.CWD`; process starts inside the session directory     |
| timeout           | 30 s hard kill via `context.WithTimeout`                               |
| output cap        | stdout+stderr truncated to 64 KB before forwarding to the model        |
| exit errors       | non-zero exit is returned as text, not an error — model sees the output|

**adding a new allowed command**

edit `allowedCommands` in `internal/ipc/server.go`. the map key is the binary base name. no other changes are needed — the dispatch is generic.

**risk tier**

`run` is **caution-tier**. the allowlist keeps it out of the destructive tier, but:
- `git reset --hard`, `make clean`, `go generate` can delete or regenerate files.
- every `run` call sends a `tool_request` to inari and blocks until the user presses `[y]` or `[n]`. the call never executes unattended.
- read-only builtins (`read_file`, `list_dir`, `grep_file`, `stat_file`) are safe-tier and execute without prompting.

**rollout order for future expansion**

1. *(done)* allowlist-only `run` — `go`, `make`, `git`.
2. *(done)* per-call approval gating in inari (§8.2) for `run`; safe-tier builtins auto-execute.
3. arbitrary bash (destructive tier) only after write tools and blast-radius limits are in production.
4. replace with MCP process connector once the tool-call loop supports it.

**what stays forbidden**

- `ssh`, `scp` — remote shell and file-copy; network calls outside ollama/mcp.
- `rm`, `mv`, `chmod` — destructive filesystem ops.
- `sh`, `bash`, `zsh`, `python` — shell interpreters that bypass the allowlist.
- any binary not in `allowedCommands` — hard-rejected at dispatch.

note: `curl` and `wget` are in the allowlist for read-only http queries (e.g. querying local endpoints). they are caution-tier via `run`, so each call requires user confirmation. revisit whether they belong in the allowlist once write tools and blast-radius limits are in production.

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
