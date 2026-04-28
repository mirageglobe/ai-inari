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
- [x] `[inarid]` `config.json` parsed at startup.
- [x] `[inarid]` connectors spawned as child processes.
- [ ] `[inarid]` tool-calls routed and audit-logged. (`internal/mcp/host.go` `Call()` is a TODO stub — audit logging exists but actual JSON-RPC dispatch over stdio is not implemented)

#### M5 — Chat View
- [x] `[kitsune]` interactive `i` view wires to Head Inari (Thinker tier).
- [x] `[inarid]` message history scoped to session.
- [x] `[kitsune/inarid]` detach/reattach preserves session state.

### Near-term
- [ ] `[easy]` add `LICENSE` file — AGPLv3; copyright holder: Jimmy Lim
- [x] `[kitsune]` `[medium]` themes — a small set of built-in colour themes (e.g. default purple, amber, slate, rose); cycle through them with `[t]` from any view; theme is stored in config.json and applied at startup
- [x] `[kitsune]` `[easy]` help overlay — `[?]` opens a modal listing all hotkeys for the current view; `[esc]` or `[?]` dismisses it
- [ ] `[kitsune]` `[easy]` quick-start fox — if the herd view has no sessions, automatically create a default session so the user can start chatting immediately without a manual create step
- [ ] `[kitsune]` `[medium]` session search and filter in herd view
- [ ] `[kitsune]` `[easy]` export chat history to file
- [ ] `[kitsune/inarid]` `[hard]` main screen: allow token compression by summarising session content
- [ ] `[kitsune/inarid]` `[hard]` long-term task planning from high-level prompts
- [ ] `[kitsune/inarid]` `[medium]` interrupt in chat for messages
- [ ] `[inarid]` `[medium]` recap/summary when a chat session has been idle for 10+ mins
- [x] `[kitsune]` `[easy]` show current token count in chat
- [ ] `[kitsune]` `[easy]` allow download of context and copy of response as text
- [ ] `[inarid]` `[easy]` daemon: auto-shutdown after 30 mins idle
- [ ] `[inarid/kitsune]` `[medium]` **ollama context window detection and optimum setting** — on session creation (or model change), inarid queries the model's `num_ctx` parameter via the Ollama `/api/show` endpoint; the detected value is surfaced in the kitsune chat view alongside the token count. inarid also exposes a per-session override that sets `num_ctx` in each `/api/chat` request, defaulting to a sensible optimum (e.g. 8192 for worker-tier models, 4096 for sensor-tier) rather than Ollama's built-in default. the kitsune UI allows the user to view and adjust this value per session.
- [ ] `[kitsune]` `[easy]` chat navigation shortcuts — implement `ctrl+t` for tools (focus hint bar), `ctrl+p` for command palette/prompt, `ctrl+m` for menu, and `ctrl+g` for help; use `esc` to exit entry fields and return to navigation mode.
- [ ] `[kitsune]` `[easy]` slash commands — if chat input is empty, entering `/` opens a command selection menu (similar to google cli or slack).
- [ ] `[kitsune]` `[easy]` chat input mode indicators — update the chat entry prefix to show the active mode (e.g., `[chat] >`, `[tool] >`, `[/] >`) for better visual feedback.

### Ideas
- [ ] `[kitsune/inarid]` **context compression (ponder)** — manual `[p] ponder` command in chat triggers inarid
        to summarise the conversation history via the session's own model, replacing old turns
        with a compact summary. keeps the system behavior prompt intact. auto-compression
        variant triggers automatically when context exceeds a configurable threshold.
- [x] `[inarid]` **filesystem tool-call loop (layer 2)** — inarid declares read-only tools (`read_file`, `list_dir`) in the Ollama API request for sessions that have a working directory set. when Ollama returns a tool-call instead of text, inarid executes the tool (sandboxed to the session's `cwd`), appends the result as a `tool` message, and re-sends to Ollama — looping until a final text response arrives. write operations are explicitly out of scope at this stage.
- [ ] `[inarid]` **MCP filesystem connector (layer 3)** — once the tool-call loop exists, replace built-in tools with `@modelcontextprotocol/server-filesystem` spawned via mcp-go. this is a natural extension of the MCP integration work below.
- [ ] `[inarid]` **destructive action prevention (§8.2)** — risk-tiered auto-approval, blast-radius limits, and dry-run previews for caution-tier tool-calls; prerequisite for any layer 2+ tool execution
- [ ] `[inarid]` multiple models per session — allow attaching different models to a single session for collaborative discussions and task execution
- [ ] `[inarid]` MCP integration — replace `internal/mcp` with `github.com/mark3labs/mcp-go`; connectors (Linear, Slack, Google Drive, etc.) configured via `config.json`
- [ ] `[inarid]` **prompt-based tool calling** — for models without native function-calling support, inject tool definitions as plain text into the system prompt and set `format: "json"`; inarid parses the JSON response to detect tool calls. select mode via session config or auto-detect from model name. makes layer 2 work on any instruction-following model (hermes-3-pro, qwen3-coder, etc.)
- [ ] `[inarid]` **provider abstraction** — open up the hard-coded Ollama dependency by introducing a `Provider` interface (`Chat`, `ChatStream`, `ListModels`, `ListRunning`). inarid's core talks only to the interface; the concrete provider is selected via `provider` in `config.json`. ollama is the default. this allows swapping to vLLM, LM Studio, llama.cpp server, or even a cloud API (Claude, OpenAI) with a single config change and no core changes.
- [ ] `[inarid]` multi-model routing — sensor tier classifies intent, dispatches to worker or thinker
- [ ] `[inarid]` **context caching / compression / optimisation** — investigate strategies to reduce prompt size and improve response speed: KV-cache reuse across turns, selective message eviction, rolling summary compression, and prefix caching at the provider level; goal is lower latency and higher effective context utilisation without degrading response quality
- [ ] `[inarid]` **task difficulty/effort classification** — investigate how to define and score task difficulty, complexity, and effort (e.g. token count, tool-call depth, reasoning hops) so inarid can automatically select the appropriate model tier (sensor → worker → thinker) rather than relying on manual session config
- [ ] `[kitsune/inarid]` session tagging and search
- [ ] `[kitsune/inarid]` **rename session** — allow the user to rename an existing session from the herd view; kitsune sends a `session.rename` RPC to inarid which updates the stored session name and propagates the change back to all open views.

### Completed
- [x] `[fox]` CLI removed — functionality superseded by kitsune TUI
- [x] `[kitsune]` thinking spinner in chat session while waiting for a response
- [x] `[kitsune/inarid]` offline detection in chat — when inarid is unreachable, the hint line shows "inari is offline" and sends are blocked until connectivity is restored
- [x] `[kitsune/inarid]` streaming chat — `session.stream` RPC over dedicated per-call UDS connections; kitsune renders tokens as they arrive
- [x] `[kitsune]` title bar wave animation — per-character purple gradient drifts across the kitsune title at 200ms intervals
- [x] `[kitsune/inarid]` filesystem context (layer 1) — shallow file tree injected into system prompt at session creation; kitsune passes `cwd`, inarid walks up to 3 levels (skipping `.git`, `node_modules`, etc.)

### Open Issues
- [ ] `[inarid/kitsune]` track and manage known issues and bugs
- [ ] `[kitsune]` hotkeys (e.g. `?`, `t`) clash with text input in chat mode — global key bindings must be suppressed when a text input widget is focused  [medium]

---

## 4. Architecture

### 4.1 System Overview

```
  you (kitsune TUI)
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
│       kitsune (TUI)         │  ← user-facing client
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
- Restarting kitsune reconnects to the existing herd without losing any conversation.
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

| Method             | Params               | Returns         | Description                                      |
|--------------------|----------------------|-----------------|--------------------------------------------------|
| `session.list`     | —                    | `SessionInfo[]` | summary of all sessions (no history on wire)     |
| `session.create`   | `{name, cwd?}`       | `SessionInfo`   | create a named session; optional `cwd` enables filesystem context |
| `session.delete`   | `{id}`               | `"ok"`          | remove session and its history                   |
| `session.assign`   | `{id, model}`        | `"ok"`          | attach a model to a session                      |
| `session.chat`     | `{id, text}`         | `string`        | blocking: append message, return full reply      |
| `session.stream`   | `{id, text}`         | *(see below)*   | streaming: append message, stream token chunks   |

**Streaming chat (`session.stream`):**

`session.stream` uses a **dedicated per-call UDS connection** rather than the shared RPC connection. This allows multiple simultaneous streams (one per open chat view) without head-of-line blocking.

Protocol over the dedicated connection:

1. client dials a new `unix` connection to `/tmp/inari.sock`
2. client sends a normal JSON-RPC 2.0 request: `{"method":"session.stream","params":{"id":"...","text":"..."}}`
3. inarid responds with a stream of newline-delimited JSON frames, one per token chunk:
   ```json
   {"token":"Hello"}
   {"token":" world"}
   {"done":true}
   ```
4. on `done`, inarid has persisted the full reply to the session store; client closes the connection
5. on error, inarid sends `{"error":"<message>"}` and closes

kitsune opens one dedicated connection per active `session.stream` call. the shared `Client` connection remains exclusively for control RPCs and is never blocked by in-flight streams.

**multiple concurrent streams:**

within a single kitsune TUI, the user can spawn multiple named chat sessions (each displayed as a row in the herd view). each session is an independent kitsune — it can have a model assigned and an active generation in flight simultaneously. because each `session.stream` call uses its own dedicated UDS connection, all sessions can stream concurrently without blocking one another. inarid handles each stream in its own goroutine via the accept loop.

**message routing in kitsune:**

token messages (`ChatTokenMsg`, `ChatDoneMsg`) carry a `SessionID` field. the root model routes them directly to the correct `Chat` view in `m.chats[sessionID]` — regardless of which view is currently displayed. this allows background sessions to accumulate tokens invisibly; when the user switches back, the chat view already shows the partial or complete response.

### 4.5 Concurrency & Scheduling

- Each Ollama session runs in its own goroutine.
- A semaphore gates concurrent sessions based on configured memory budget.
- Multiple simultaneous chat streams are supported — each uses its own UDS connection.
- Slow/background tasks continue when the TUI is detached.

### 4.6 Filesystem Awareness — Three-Layer Model

sessions can be given awareness of the local filesystem in three progressively richer layers. each layer is a prerequisite for the next.

**layer 1 — directory context (system prompt injection)**

kitsune passes the current working directory when creating a session. inarid resolves a shallow file tree (`find . -maxdepth 3`, filtered by `.gitignore`) and prepends it as a system message:

```
system: working directory: /path/to/project
<file tree>
```

the model can reason about the project layout and refer to files by path, but cannot read their content. this requires no changes to the ollama request format and works with every model.

**layer 2 — read-only file access (agentic tool-call loop)**

inarid declares two built-in tools in the ollama `/api/chat` request for sessions that have `cwd` set:

| Tool        | Input              | Output                          |
|-------------|--------------------|---------------------------------|
| `read_file` | `{path: string}`   | file contents (text only)       |
| `list_dir`  | `{path: string}`   | directory listing (names only)  |

both tools are sandboxed: paths are resolved relative to `cwd` and must not escape it (no `../` traversal). write operations are out of scope.

when ollama returns a `tool_calls` response, inarid's `handleStream` loop:

1. executes each tool call inside the sandbox
2. appends a `tool` role message with the result
3. re-sends the full message history to ollama
4. repeats until ollama returns a `message` (text) response
5. streams the final text back to kitsune as normal token frames

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

### 5.2 `kitsune` — Client

| View    | Key | Description                              |
|---------|-----|------------------------------------------|
| Herd    | —   | Default view; table of all workers/pods  |
| Logs    | `l` | Tail output of selected session          |
| Describe| `d` | Full session metadata and config         |
| Chat    | `i` | Interactive chat with Head Inari (1GB)   |

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

| tier | examples | inarid behaviour |
|------|----------|-----------------|
| safe | `read_file`, `list_dir`, `session.list` | execute immediately, log |
| caution | `write_file`, `create_issue`, `send_message` | dry-run first, then require confirmation |
| destructive | `delete_file`, `close_issue`, `delete_*` | always require confirmation; shown in red in kitsune |
| forbidden | process spawn, network calls outside ollama/mcp, shell exec | hard-rejected; never routable |

classification is conservative: if a tool's tier is ambiguous, it is assigned the higher-risk tier. adding a new tool requires an explicit tier assignment — unclassified tools are rejected.

**layer B — blast-radius limits**

hard limits enforced by inarid regardless of tier or confirmation:

- all file operations capped at 1 MB per call.
- no operations outside the session's `cwd` (path traversal rejected at validation, not policy).
- no more than 10 tool-calls per model turn (prevents runaway loops).
- no spawning processes or making network calls from within a tool handler.

**layer C — dry-run for caution-tier actions**

before executing a caution-tier tool-call, inarid computes a dry-run result and sends a `tool.preview` message to kitsune:

```
[preview] write_file: path/to/file.go
--- current
+++ proposed
@@ -1,3 +1,5 @@
 ...
```

kitsune renders the preview and waits for `[y] approve` or `[n] reject`. only on approval does inarid execute. rejection is logged. if kitsune is detached, caution-tier calls are automatically rejected — they never execute unattended.

**non-goal:** this design does not attempt to detect malicious intent from model outputs. it bounds damage structurally so that even a model producing harmful tool-calls cannot exceed the permitted blast radius.

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
./bin/inarid
```

**Terminal 3 — kitsune TUI:**
```sh
./bin/kitsune
```

### 9.2 Signal Handling
`inarid` handles `SIGINT` (Ctrl+C) and `SIGTERM` cleanly, flushing all session state to disk and closing the Unix socket before exit.

---

## 10. Complexity Score

> only update this table using a large/strong model after significant architectural changes.

| dimension | score | notes |
| :--- | :--- | :--- |
| overall | 3 / 5 | moderate; dual-binary with custom streaming IPC and multi-view TUI |
| `internal/ipc` | 4 / 5 | highest complexity — custom JSON-RPC over UDS, dedicated per-stream connections, concurrent goroutines |
| `tui` (kitsune) | 3 / 5 | multi-view Bubble Tea app with message routing, offline resilience, and live token rendering |
| `internal/session` | 2 / 5 | session lifecycle and atomic disk persistence; well-bounded |
| `internal/ollama` | 2 / 5 | HTTP streaming client; straightforward |
| `internal/provider` | 2 / 5 | filesystem tool-call loop (layer 2) with sandbox path validation |
| `internal/mcp` | 1 / 5 | stub only — `Call()` is a TODO; will rise to 3+ when JSON-RPC dispatch is implemented |
| `internal/scheduler` | 1 / 5 | semaphore wrapper; minimal |
| `internal/audit` | 1 / 5 | append-only log; minimal |

---

## 11. Open Questions

- Audit log format: structured JSON lines vs. human-readable?
- Auth: is owner-only UDS sufficient, or add a local token?
- Session persistence: resolved — one JSON file per session, atomic write+rename,
  stored in `~/.local/share/inari/sessions`. SQLite is the natural next step if
  querying or concurrent access become requirements.
