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
via Ollama. You talk to them through `kitsune`, a keyboard-driven terminal UI
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
make start    # builds, starts ollama + inarid in background, launches kitsune TUI
make stop     # stop inarid (also runs automatically when kitsune exits)
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

terminal 3 — kitsune TUI:
```sh
./bin/kitsune
```

to stop inarid when running in the foreground: `ctrl+c`. it handles SIGINT
cleanly and shuts down the socket and session store.

to stop inarid when running in the background:
```sh
pkill inarid        # by name
# or
make stop           # uses saved pid at /tmp/inarid.pid
```

---

## fox CLI

`fox` is a lightweight CLI companion that talks to the same `inarid` daemon as `kitsune`.
use it for scripting, quick one-off prompts, or piping responses to other tools.

```sh
make build-cli            # build once

./bin/fox ping                              # check daemon is running
./bin/fox sessions                          # list all sessions
./bin/fox chat <session-id> <message>       # send a message to a session
```

run `fox sessions` to find a session ID.

---

## kitsune keys

### herd (main screen)

| key     | action                              |
|---------|-------------------------------------|
| `s`     | new kitsune session                 |
| `m`     | assign model to selected session    |
| `u`     | unload model from selected session  |
| `c` / `enter` | open chat for selected session |
| `x`     | delete selected session             |
| `d`     | describe — session metadata + behavior|
| `f`     | select default session for fox CLI use |
| `l`     | logs — view kitsune.log             |
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
- [ ] main screen: allow token compression by summarising session content
- [x] cli tool for local development - `kitsune` CLI for scriptable access to sessions
- [ ] long-term task planning from high-level prompts
- [ ] queue mode in chat for messages
- [ ] interrupt in chat for messages
- [ ] show current token count in chat
- [ ] allow download of context and copy of response as text

### ideas / deferred
- [ ] **context compression (ponder)** — manual `[p] ponder` command in chat triggers inarid
        to summarise the conversation history via the session's own model, replacing old turns
        with a compact summary. keeps the system behavior prompt intact. auto-compression
        variant triggers automatically when context exceeds a configurable threshold.
- [ ] multiple models per session — allow attaching different models to a single session for collaborative discussions and task execution
- [ ] MCP integration — filesystem, search, SQL connectors via child processes
- [ ] multi-model routing — sensor tier classifies intent, dispatches to worker or thinker
- [ ] session tagging and search
- [ ] show current ollama context length setting

### open issues
- [ ] track and manage known issues and bugs
- [ ] thinking spinner to be added in chat session when waiting for long running response
