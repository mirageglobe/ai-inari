# Inari — Project Spec

Security-first, minimalist local AI orchestrator.

---

## 1. Goals

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

## 3. Roadmap & Milestones

### 3.1 Build Milestones

#### M1 — UDS Bridge
- [ ] `inarid` starts and binds UDS socket.
- [ ] `kitsune` connects and performs handshake.
- [ ] Basic ping/pong JSON-RPC round-trip.

#### M2 — Herd UI
- [ ] Bubble Tea table renders active sessions.
- [ ] Sessions update in real time from daemon events.
- [ ] Keyboard navigation (select, quit).

#### M3 — Ollama Integration
- [ ] Daemon POSTs to Ollama `/api/chat` and streams tokens.
- [ ] Token stream forwarded to `kitsune` Logs view.
- [ ] Semaphore throttle enforces memory budget.

#### M4 — MCP Loader
- [ ] `config.json` parsed at startup.
- [ ] Connectors spawned as child processes.
- [ ] Tool-calls routed and audit-logged.

#### M5 — Chat View
- [ ] Interactive `i` view wires to Head Inari (Thinker tier).
- [ ] Message history scoped to session.
- [ ] Detach/reattach preserves session state.

### 3.2 Feature Roadmap

#### Near-term
- [ ] streaming chat — `session.stream` RPC over dedicated per-call UDS connections; kitsune renders tokens as they arrive
- [ ] filesystem context (layer 1) — inject working directory path and file tree into a session's system prompt at creation time; kitsune passes `cwd` to `session.create`; inarid prepends it as a system message so the model is aware of the project layout without reading any file content
- [ ] session search and filter in herd view
- [ ] export chat history to file
- [ ] main screen: allow token compression by summarising session content
- [ ] long-term task planning from high-level prompts
- [ ] interrupt in chat for messages
- [ ] recap/summary when a chat session has been idle for 10+ mins
- [ ] show current token count in chat
- [ ] allow download of context and copy of response as text
- [ ] daemon: auto-shutdown after 30 mins idle

#### Ideas
- [ ] **context compression (ponder)** — manual `[p] ponder` command in chat triggers inarid
        to summarise the conversation history via the session's own model, replacing old turns
        with a compact summary. keeps the system behavior prompt intact. auto-compression
        variant triggers automatically when context exceeds a configurable threshold.
- [ ] **filesystem tool-call loop (layer 2)** — inarid declares read-only tools (`read_file`, `list_dir`) in the Ollama API request for sessions that have a working directory set. when Ollama returns a tool-call instead of text, inarid executes the tool (sandboxed to the session's `cwd`), appends the result as a `tool` message, and re-sends to Ollama — looping until a final text response arrives. write operations are explicitly out of scope at this stage.
- [ ] **MCP filesystem connector (layer 3)** — once the tool-call loop exists, replace built-in tools with `@modelcontextprotocol/server-filesystem` spawned via mcp-go. this is a natural extension of the MCP integration work below.
- [ ] multiple models per session — allow attaching different models to a single session for collaborative discussions and task execution
- [ ] MCP integration — replace `internal/mcp` with `github.com/mark3labs/mcp-go`; connectors (Linear, Slack, Google Drive, etc.) configured via `config.json`
- [ ] multi-model routing — sensor tier classifies intent, dispatches to worker or thinker
- [ ] session tagging and search
- [ ] show current ollama context length setting

### 3.3 Status

#### Completed
- [x] `fox` CLI removed — functionality superseded by kitsune TUI
- [x] thinking spinner in chat session while waiting for a response
- [x] offline detection in chat — when inarid is unreachable, the hint line shows "inari is offline" and sends are blocked until connectivity is restored

#### Open Issues
- [ ] track and manage known issues and bugs

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

this requires ollama tool-call support — available for models that declare function-calling capability (qwen3, llama3.2, mistral-nemo, etc.).

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

## 10. Open Questions

- Audit log format: structured JSON lines vs. human-readable?
- Auth: is owner-only UDS sufficient, or add a local token?
- Session persistence: resolved — one JSON file per session, atomic write+rename,
  stored in `~/.local/share/inari/sessions`. SQLite is the natural next step if
  querying or concurrent access become requirements.
