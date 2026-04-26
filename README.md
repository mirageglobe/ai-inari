# Inari

```
      🦊      🦊
    🦊🦊🦊  🦊🦊🦊
  🦊🦊🦊🦊🦊🦊🦊🦊🦊
  🦊🦊  🦊🦊🦊  🦊🦊
    🦊🦊🦊🦊🦊🦊🦊

  "a herd behind every idea."
```

![demo](demo.gif)

In Japanese mythology, Inari is the fox god — the kami of luck, prosperity, and
industry. Thousands of shrines across Japan are dedicated to Inari, each guarded
by kitsune, the foxes who serve as messengers between the spirit world and ours.
Inari doesn't shout. It works quietly, and good things follow.

**ai-inari** is a herd of local AI minions. Intelligence that lives on your
machine, answers to you alone, and disappears when you close the lid.

No cloud. No telemetry. No secrets leaving the machine. Just a quiet herd
doing useful work in the background, waiting for your next word.

---

## what it does

- **sessions first.** Create a named session, assign a model, start chatting.
  Conversation history lives in `inarid` (background daemon).
- **behavior context.** Each session has an editable system prompt (behavior).
- **context tracking.** Estimated token count visible in the chat header.
- **no cloud.** Every model runs locally through Ollama.
- **no noise.** One keyboard-driven screen (`kitsune` TUI), nothing you didn't ask for.
- **no secrets leaked.** Every tool-call is audit-logged locally.

---

## core concepts: the herd

The herd is organized into tiers based on resource usage and role:

| tier     | role                     | size   | example model | required |
|----------|--------------------------|--------|---------------|----------|
| sensors  | routing / classification | 100 MB | Qwen3-Nano    | no       |
| workers  | parallel execution       | 500 MB | Bonsai 4B     | yes      |
| thinkers | architect / chat         | 1 GB   | Bonsai 8B     | yes      |

Sensors are optional scouts. Workers do the heavy lifting in parallel.
The thinker is the "Head Inari" — the one you talk to directly.

---

## quick start

```sh
make start    # builds, starts ollama + inarid in background, launches kitsune TUI
make stop     # stop inarid (also runs automatically when kitsune exits)
```

---

## configuration

`ai-inari` reads from `config.json` in the project root.

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

---

## usage

### kitsune (TUI)

The TUI is inspired by `k9s` and is entirely keyboard-driven.

**herd (main screen)**
- `s`: new session | `m`: assign model | `u`: unload model
- `c` / `enter`: open chat | `x`: delete session | `d`: describe
- `l`: view logs | `r`: refresh | `q`: quit

**chat**
- `enter`: send message | `ctrl+o`: change model
- `↑` / `↓`: scroll | `esc`: back to herd

---

## roadmap

See [SPEC.md](SPEC.md#3-roadmap--milestones) for the project roadmap and build milestones.
