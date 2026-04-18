# Inari (fox) — Project Spec

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

## 3. Architecture

```
┌─────────────────────────────┐
│         fox (TUI)           │  ← user-facing client
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

### 3.1 Session model

Sessions are the primary entity in ai-inari. A session is a named chat context
(e.g. "Arctic Fox") that exists independently of any model. The user creates a
session first, then optionally assigns a model to it. Chat history is stored
inside the session in inarid — fox is stateless and holds no history locally.

This means:
- Restarting fox reconnects to the existing herd without losing any conversation.
- A session with no model assigned is valid; the model can be swapped at any time.
- `session.chat` takes a session ID and a single new message; inarid appends it,
  sends the full history to Ollama, stores the reply, and returns only the text.
- Restarting inarid reloads all sessions from disk — history and model assignment
  are preserved across daemon restarts.

### 3.1.1 Session persistence

Sessions are persisted to disk as JSON files, one file per session (`<id>.json`),
stored in the session data directory (default: `~/.local/share/inari/sessions`,
overridable via `data_dir` in `config.json`).

Writes are atomic: inarid writes to a `.tmp` file then renames it, so readers
never observe a partial file. The file is written on every state change:
`session.create`, `session.assign`, `session.unassign`, and after each
`session.chat` turn (both the user message and the model reply are flushed).

This design is intentionally simple — a single JSON file per session is easy to
inspect, back up, and migrate. If query performance or concurrent access becomes
a requirement, the store can be replaced with SQLite without changing the session
model or IPC protocol.

### 3.2 IPC

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

### 3.3 Concurrency

- Each Ollama session runs in its own goroutine.
- A semaphore gates concurrent sessions based on configured memory budget.
- Slow/background tasks continue when the TUI is detached.

---

## 4. Components

### 4.1 `inarid` — Daemon

| Subsystem     | Responsibility                                              |
|---------------|-------------------------------------------------------------|
| UDS Server    | Accept and authenticate client connections                  |
| Session Store | Own named sessions with chat history; persists to JSON files on disk; survives daemon restart |
| MCP Host      | Spawn and manage MCP connectors (Filesystem, Search, SQL)   |
| Ollama Client | POST to `/api/chat`; stream tokens back to session          |
| Scheduler     | Semaphore-based concurrency throttle per resource tier      |
| Audit Logger  | Append-only log of all JSON-RPC tool-calls with timestamps  |

### 4.2 `fox` — TUI Client

| View    | Key | Description                              |
|---------|-----|------------------------------------------|
| Herd    | —   | Default view; table of all workers/pods  |
| Logs    | `l` | Tail output of selected session          |
| Describe| `d` | Full session metadata and config         |
| Chat    | `i` | Interactive chat with Head Inari (1GB)   |

Navigation is keyboard-driven, k9s-inspired.

---

## 5. Resource Tiers

| Size   | Tier     | Role                     | Example model | Required |
|--------|----------|--------------------------|---------------|----------|
| 100MB  | Sensors  | Routing / classification | Qwen3-Nano    | No       |
| 500MB  | Workers  | Parallel execution       | Bonsai 4B     | Yes      |
| 1GB    | Thinkers | Architect / chat         | Bonsai 8B     | Yes      |

Sensors are optional; Workers and Thinkers are the minimum viable deployment.

---

## 6. MCP Connectors

Connectors are spawned as child processes via stdio pipes.

| Connector  | Purpose                        |
|------------|--------------------------------|
| Filesystem | Read/write local files         |
| Search     | Web or local document search   |
| SQL        | Query local databases          |

Connector definitions loaded from `config.json` at daemon start.

---

## 7. Security Model

- All IPC local-only via UDS; no TCP exposure.
- Socket permissions restrict access to the owning user.
- All MCP tool-calls written to an append-only audit log.
- No credentials, tokens, or secrets stored by the daemon.
- MCP child processes run with inherited (restricted) environment.

---

## 8. Configuration

`config.json` (daemon reads on start):

```json
{
  "socket": "/tmp/inari.sock",
  "memory_budget_mb": 8192,
  "ollama_base_url": "http://localhost:11434",
  "data_dir": "~/.local/share/inari/sessions",
  "mcp_connectors": [
    { "name": "filesystem", "command": "mcp-filesystem", "args": [] },
    { "name": "search",     "command": "mcp-search",     "args": [] }
  ],
  "models": {
    "thinker": "bonsai:8b",
    "worker":  "bonsai:4b",
    "sensor":  "qwen3-nano"
  }
}
```

`data_dir` is optional. If omitted, inarid defaults to `~/.local/share/inari/sessions`.

---

## 9. Build Milestones

### M1 — UDS Bridge
- [ ] `inarid` starts and binds UDS socket.
- [ ] `fox` connects and performs handshake.
- [ ] Basic ping/pong JSON-RPC round-trip.

### M2 — Herd UI
- [ ] Bubble Tea table renders active sessions.
- [ ] Sessions update in real time from daemon events.
- [ ] Keyboard navigation (select, quit).

### M3 — Ollama Integration
- [ ] Daemon POSTs to Ollama `/api/chat` and streams tokens.
- [ ] Token stream forwarded to `fox` Logs view.
- [ ] Semaphore throttle enforces memory budget.

### M4 — MCP Loader
- [ ] `config.json` parsed at startup.
- [ ] Connectors spawned as child processes.
- [ ] Tool-calls routed and audit-logged.

### M5 — Chat View
- [ ] Interactive `i` view wires to Head Inari (Thinker tier).
- [ ] Message history scoped to session.
- [ ] Detach/reattach preserves session state.

---

## 10. Open Questions

- Audit log format: structured JSON lines vs. human-readable?
- Auth: is owner-only UDS sufficient, or add a local token?
- Session persistence: resolved — one JSON file per session, atomic write+rename,
  stored in `~/.local/share/inari/sessions`. SQLite is the natural next step if
  querying or concurrent access become requirements.
