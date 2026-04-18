# ai-inari

```
    🦊         🦊
   🦊🦊🦊     🦊🦊🦊
  🦊🦊🦊🦊🦊🦊🦊🦊🦊🦊
  🦊🦊  🦊🦊🦊🦊  🦊🦊
   🦊🦊🦊🦊🦊🦊🦊🦊🦊

  "a herd behind every idea."
```

In Japanese mythology, Inari is the fox god — the kami of luck, prosperity, and
industry. Thousands of shrines across Japan are dedicated to Inari, each guarded
by kitsune, the foxes who serve as messengers between the spirit world and ours.
Inari doesn't shout. It works quietly, and good things follow.

That felt right.

Most AI tools pull your work into the cloud, scatter it across APIs you don't
control, and ask you to trust a dozen third parties with your data. We wanted
the opposite: intelligence that lives on your machine, answers to you alone, and
disappears when you close the lid.

**ai-inari** is a herd of local AI minions. You run them on your own hardware
via Ollama. You talk to them through `fox`, a keyboard-driven terminal UI
inspired by k9s. A persistent background daemon (`inarid`) keeps the herd
alive even when the UI is closed — reopen it and your sessions are right where
you left them, history intact.

No cloud. No telemetry. No secrets leaving the machine. Just a quiet herd
doing useful work in the background, waiting for your next word.

---

## What it does

- **Sessions first.** Create a named session, assign a model, start chatting.
  Your conversation history lives in `inarid` — close `fox` and reconnect
  without losing a word.
- **No cloud.** Every model runs locally through Ollama.
- **No noise.** One keyboard-driven screen, nothing you didn't ask for.
- **No secrets leaked.** Every tool-call is audit-logged locally.

---

## Architecture

```
  you (fox TUI)
      |
      |  JSON-RPC over Unix socket  (chmod 0600)
      |
  inarid (daemon)
    ├── session store   — persists sessions + history to ~/.local/share/inari/sessions/
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
The Thinker is Head Inari — the one you talk to directly.

---

## TUI keys

| Key     | Action                        |
|---------|-------------------------------|
| `l`     | Logs — tail selected session  |
| `d`     | Describe — session metadata   |
| `i`     | Chat — talk to Head Inari     |
| `esc`   | Back to herd view             |
| `q`     | Quit                          |

---

## Quick start

```sh
# build
make build

# start the daemon (keep it running)
./bin/inarid

# open the TUI in another terminal
./bin/fox
```

Configuration lives in `config.json`. See [SPEC.md](SPEC.md) for the full
architecture, security model, and build milestones.

---

## TODO

- [ ] MCP integration (deferred)
