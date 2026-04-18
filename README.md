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

## what it does

- **sessions first.** Create a named session, assign a model, start chatting.
  Conversation history lives in `inarid` — close `fox` and reconnect without
  losing a word.
- **behavior context.** Each session has an editable system prompt (behavior) shown
  in the describe view. New sessions default to "keep all responses concise and short."
- **context tracking.** The chat header shows an estimated token count for the
  session so you can see how much context the model is carrying.
- **no cloud.** Every model runs locally through Ollama.
- **no noise.** One keyboard-driven screen, nothing you didn't ask for.
- **no secrets leaked.** Every tool-call is audit-logged locally.

---

## architecture

```
  you (fox TUI)
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

---

## the herd

| size   | tier     | role                     | example model | required |
|--------|----------|--------------------------|---------------|----------|
| 100 MB | sensors  | routing / classification | Qwen3-Nano    | no       |
| 500 MB | workers  | parallel execution       | Bonsai 4B     | yes      |
| 1 GB   | thinkers | architect / chat         | Bonsai 8B     | yes      |

sensors are optional scouts. workers do the heavy lifting in parallel.
the thinker is head inari — the one you talk to directly.

---

## quick start

**together (recommended)**

```sh
make start    # builds, starts ollama + inarid in background, launches fox
make stop     # stop inarid (also runs automatically when fox exits)
```

**independently (for debugging)**

terminal 1 — ollama:
```sh
ollama serve
```

terminal 2 — inarid (foreground, so you can see all daemon logs):
```sh
make build
./bin/inarid
```

terminal 3 — fox:
```sh
./bin/fox
```

to stop inarid when running in the foreground: `ctrl+c`. it handles SIGINT
cleanly and shuts down the socket and session store.

to stop inarid when running in the background:
```sh
pkill inarid        # by name
# or
make stop           # uses saved pid at /tmp/inarid.pid
```

**logs**

fox writes its own log to `fox.log` in the working directory, viewable
inside the TUI with `[l]`. inarid logs go to stdout (or wherever you redirect
them). the audit log of all tool calls is written to `inari-audit.log`.

configuration lives in `config.json`. see [SPEC.md](SPEC.md) for the full
architecture, security model, and build milestones.

---

## TUI keys

### herd (main screen)

| key     | action                              |
|---------|-------------------------------------|
| `s`     | new kitsune session                 |
| `m`     | assign model to selected session    |
| `u`     | unload model from selected session  |
| `c` / `enter` | open chat for selected session |
| `x`     | delete selected session             |
| `d`     | describe — session metadata + behavior|
| `l`     | logs — view fox.log                 |
| `r`     | refresh session and model list      |
| `q`     | quit fox                            |

### chat

| key       | action                        |
|-----------|-------------------------------|
| `enter`   | send message                  |
| `ctrl+o`  | change model for this session |
| `↑` / `↓` | scroll chat history          |
| `esc`     | back to herd                  |

### describe

| key      | action                        |
|----------|-------------------------------|
| `e`      | edit session behavior (context) |
| `ctrl+s` | save behavior                   |
| `esc`    | cancel edit / back to herd    |

### logs

| key   | action        |
|-------|---------------|
| `r`   | refresh log   |
| `esc` | back to herd  |

### model selector

| key     | action                        |
|---------|-------------------------------|
| `enter` | assign model to session       |
| `esc`   | back                          |

---

## session lifecycle

sessions are owned by `inarid`, not `fox`. closing fox does not delete sessions.
when fox reconnects, the herd view shows all live sessions with their current
model assignment and status:

| status    | meaning                                      |
|-----------|----------------------------------------------|
| `in Xs`   | model loaded, keep-alive expires in X seconds|
| `sleeping`| model assigned but not currently in memory   |
| `waking`  | keep-alive expired, model will reload on chat|
| `—`       | no model assigned                            |

---

## roadmap

### in progress / near-term
- [ ] session search and filter in herd view
- [ ] export chat history to file

### ideas / deferred
- [ ] **context compression (ponder)** — manual `[p] ponder` command in chat triggers inarid
      to summarise the conversation history via the session's own model, replacing old turns
      with a compact summary. keeps the system behavior prompt intact. auto-compression
      variant triggers automatically when context exceeds a configurable threshold.
- [ ] MCP integration — filesystem, search, SQL connectors via child processes
- [ ] multi-model routing — sensor tier classifies intent, dispatches to worker or thinker
- [ ] session tagging and search
