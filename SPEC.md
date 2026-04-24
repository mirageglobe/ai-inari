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
- [ ] `fox` connects and performs handshake.
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
- [ ] session search and filter in herd view
- [ ] export chat history to file
- [ ] main screen: allow token compression by summarising session content
- [ ] long-term task planning from high-level prompts
- [ ] queue mode in chat for messages
- [ ] interrupt in chat for messages
- [ ] show current token count in chat
- [ ] allow download of context and copy of response as text
- [ ] daemon: auto-shutdown after 30 mins idle
- [ ] fox CLI: provide a recap/summary when a discussion has been idle for 10+ mins

#### Ideas
- [ ] **context compression (ponder)** — manual `[p] ponder` command in chat triggers inarid
        to summarise the conversation history via the session's own model, replacing old turns
        with a compact summary. keeps the system behavior prompt intact. auto-compression
        variant triggers automatically when context exceeds a configurable threshold.
- [ ] multiple models per session — allow attaching different models to a single session for collaborative discussions and task execution
- [ ] MCP integration — filesystem, search, SQL connectors via child processes
- [ ] multi-model routing — sensor tier classifies intent, dispatches to worker or thinker
- [ ] session tagging and search
- [ ] show current ollama context length setting

### 3.3 Status

#### Completed
- [x] cli tool for local development - `fox` CLI for scriptable access to sessions

#### Open Issues
- [ ] track and manage known issues and bugs
- [ ] thinking spinner to be added in chat session when waiting for long running response

---

## 4. Architecture

### 4.1 System Overview

```
  you (kitsune TUI / fox CLI)
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
- Restarting kitsune or fox reconnects to the existing herd without losing any conversation.
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
- Protocol: JSON-RPC 2.0.
- Daemon persists sessions on client detach; client reconnects by session ID.

**Session RPCs:**

| Method           | Params               | Returns       | Description                        |
|------------------|----------------------|---------------|------------------------------------|
| `session.list`   | —                    | `SessionInfo[]` | Summary of all sessions (no history on wire) |
| `session.create` | `{name}`             | `SessionInfo` | Create a named session             |
| `session.delete` | `{id}`               | `"ok"`        | Remove session and its history     |
| `session.assign` | `{id, model}`        | `"ok"`        | Attach a model to a session        |
| `session.chat`   | `{id, text}`         | `string`      | Append message, get reply          |

### 4.5 Concurrency & Scheduling

- Each Ollama session runs in its own goroutine.
- A semaphore gates concurrent sessions based on configured memory budget.
- Slow/background tasks continue when the TUI is detached.

---

## 5. Components Deep-dive

### 5.1 `inarid` — Daemon Subsystems

| Subsystem     | Responsibility                                              |
|---------------|-------------------------------------------------------------|
| UDS Server    | Accept and authenticate client connections                  |
| Session Store | Own named sessions with chat history; persists to JSON files on disk; survives daemon restart |
| MCP Host      | Spawn and manage MCP connectors (Filesystem, Search, SQL)   |
| Ollama Client | POST to `/api/chat`; stream tokens back to session          |
| Scheduler     | Semaphore-based concurrency throttle per resource tier      |
| Audit Logger  | Append-only log of all JSON-RPC tool-calls with timestamps  |

### 5.2 `kitsune` / `fox` — Clients

| View    | Key | Description                              |
|---------|-----|------------------------------------------|
| Herd    | —   | Default view; table of all workers/pods  |
| Logs    | `l` | Tail output of selected session          |
| Describe| `d` | Full session metadata and config         |
| Chat    | `i` | Interactive chat with Head Inari (1GB)   |

---

## 6. Resource Tiers Logic

The herd uses a tiered scheduling system to manage local hardware resources:

- **Sensors (Routing):** Low-priority, small context window. Used for intent classification.
- **Workers (Execution):** Mid-priority, standard context. Used for parallel task execution.
- **Thinkers (Reasoning):** High-priority, large context. Used for interactive chat and complex architectural reasoning.

Memory budget is enforced via `memory_budget_mb` in `config.json`. The scheduler blocks model loading if the budget would be exceeded.

---

## 7. MCP Connectors

Connectors are spawned as child processes via stdio pipes.

| Connector  | Purpose                        |
|------------|--------------------------------|
| Filesystem | Read/write local files         |
| Search     | Web or local document search   |
| SQL        | Query local databases          |

Connector definitions loaded from `config.json` at daemon start.

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
