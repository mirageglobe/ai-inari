# Haniwa (h9s) — Project Spec

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
│         h9s (TUI)           │  ← user-facing client
│   Bubble Tea / LipGloss     │
└────────────┬────────────────┘
             │ JSON-RPC over UDS
             │ /tmp/haniwa.sock (chmod 0600)
┌────────────▼────────────────┐
│       haniwad (daemon)      │  ← long-running engine
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

### 3.1 IPC

- Transport: Unix Domain Socket at `/tmp/haniwa.sock`.
- Permissions: `chmod 0600` — owner-only access.
- Protocol: JSON-RPC 2.0.
- Daemon persists sessions on client detach; client reconnects by session ID.

### 3.2 Concurrency

- Each Ollama session runs in its own goroutine.
- A semaphore gates concurrent sessions based on configured memory budget.
- Slow/background tasks continue when the TUI is detached.

---

## 4. Components

### 4.1 `haniwad` — Daemon

| Subsystem     | Responsibility                                              |
|---------------|-------------------------------------------------------------|
| UDS Server    | Accept and authenticate client connections                  |
| Session Store | Track active and background Ollama sessions                 |
| MCP Host      | Spawn and manage MCP connectors (Filesystem, Search, SQL)   |
| Ollama Client | POST to `/api/chat`; stream tokens back to session          |
| Scheduler     | Semaphore-based concurrency throttle per resource tier      |
| Audit Logger  | Append-only log of all JSON-RPC tool-calls with timestamps  |

### 4.2 `h9s` — TUI Client

| View    | Key | Description                              |
|---------|-----|------------------------------------------|
| Herd    | —   | Default view; table of all workers/pods  |
| Logs    | `l` | Tail output of selected session          |
| Describe| `d` | Full session metadata and config         |
| Chat    | `i` | Interactive chat with Head Haniwa (1GB)  |

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
  "socket": "/tmp/haniwa.sock",
  "memory_budget_mb": 8192,
  "ollama_base_url": "http://localhost:11434",
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

---

## 9. Build Milestones

### M1 — UDS Bridge
- [ ] `haniwad` starts and binds UDS socket.
- [ ] `h9s` connects and performs handshake.
- [ ] Basic ping/pong JSON-RPC round-trip.

### M2 — Herd UI
- [ ] Bubble Tea table renders active sessions.
- [ ] Sessions update in real time from daemon events.
- [ ] Keyboard navigation (select, quit).

### M3 — Ollama Integration
- [ ] Daemon POSTs to Ollama `/api/chat` and streams tokens.
- [ ] Token stream forwarded to `h9s` Logs view.
- [ ] Semaphore throttle enforces memory budget.

### M4 — MCP Loader
- [ ] `config.json` parsed at startup.
- [ ] Connectors spawned as child processes.
- [ ] Tool-calls routed and audit-logged.

### M5 — Chat View
- [ ] Interactive `i` view wires to Head Haniwa (Thinker tier).
- [ ] Message history scoped to session.
- [ ] Detach/reattach preserves session state.

---

## 10. Open Questions

- Audit log format: structured JSON lines vs. human-readable?
- Session persistence: in-memory only or serialised to disk?
- Auth: is owner-only UDS sufficient, or add a local token?
