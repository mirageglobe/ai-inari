# ai-haniwa (h9s)

```
  ∧ ∧
 (･ω･)  "leave it to us!"
  |つ⊂|
```

Haniwa are small forest spirits — curious, tireless, and happy to help.
In Japanese folklore they drift through mountain paths doing quiet, useful work.
Here they live inside your machine: a herd of tiny local AI minions that run
models, crunch tasks, and report back — all without phoning home.

You are the Head Haniwa. You give the word; the herd does the work.

---

## What it does

`h9s` is a terminal UI for orchestrating local LLMs via Ollama. A persistent
daemon (`haniwad`) keeps your haniwa herd running in the background even when
you close the TUI. Reconnect and they are still there, mid-task.

- **No cloud.** Everything runs on your machine.
- **No noise.** One keyboard-driven screen, k9s-style.
- **No secrets leaked.** Every tool-call is audit-logged locally.

---

## Architecture

```
  you (h9s TUI)
      |
      |  JSON-RPC over Unix socket  (chmod 0600)
      |
  haniwad (daemon)
    ├── session store   — tracks each haniwa's state
    ├── ollama client   — streams tokens from local models
    ├── scheduler       — semaphore-based memory budget
    └── audit logger    — append-only record of all tool calls
```

**Stack:** Go · Bubble Tea / LipGloss · Ollama

---

## The herd

| Size   | Tier     | Role                     | Example model | Required |
|--------|----------|--------------------------|---------------|----------|
| 100 MB | Sensors  | Routing / classification | Qwen3-Nano    | No       |
| 500 MB | Workers  | Parallel execution       | Bonsai 4B     | Yes      |
| 1 GB   | Thinkers | Architect / chat         | Bonsai 8B     | Yes      |

Sensors are optional scouts. Workers do the heavy lifting in parallel.
The Thinker is Head Haniwa — the one you talk to directly.

---

## TUI keys

| Key     | Action                        |
|---------|-------------------------------|
| `l`     | Logs — tail selected session  |
| `d`     | Describe — session metadata   |
| `i`     | Chat — talk to Head Haniwa    |
| `esc`   | Back to herd view             |
| `q`     | Quit                          |

---

## Quick start

```sh
# build
make build

# start the daemon (keep it running)
./bin/haniwad

# open the TUI in another terminal
./bin/h9s
```

Configuration lives in `config.json`. See [SPEC.md](SPEC.md) for the full
architecture, security model, and build milestones.

---

## TODO

- [ ] MCP integration (deferred)
