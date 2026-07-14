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
- **project context.** A session opened in a directory picks up its `AGENTS.md`
  (or `.inari/context.md`) automatically, so the herd knows your conventions.
- **context tracking.** Estimated token count visible in the chat header.
- **no cloud.** Every model runs locally through Ollama.
- **no noise.** One keyboard-driven screen (`inari` TUI), nothing you didn't ask for.
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

## install

```sh
brew tap mirageglobe/tap
brew install mirageglobe/tap/inari
```

## upgrade

```sh
brew upgrade mirageglobe/tap/inari
```

## quick start

inari drives local models through [Ollama](https://ollama.com), so make sure Ollama
is running first. then:

```sh
inari doctor   # check ollama, config, and that the base model is pulled
inari          # launch the daemon + TUI (the default action)
inari stop     # stop the background daemon
```

`inari doctor` prints the exact `ollama pull` command if the base model is missing.
the background daemon also shuts itself down after an idle period, so `inari stop` is
only needed to end it immediately.

---

## configuration

`ai-inari` reads from `config.json` in the project root.

```json
{
  "socket": "/tmp/inari.sock",
  "memory_budget_mb": 8192,
  "ollama_base_url": "http://localhost:11434",
  "provider": "",
  "endpoints": {
    "ollama":   { "base_url": "http://localhost:11434" },
    "lmstudio": { "base_url": "http://localhost:1234/v1", "api_key": "" }
  },
  "data_dir": "~/.local/share/inari/sessions",
  "idle_shutdown_mins": 30,
  "mcp_connectors": [
    { "name": "filesystem", "command": "mcp-filesystem", "args": [] },
    { "name": "search",     "command": "mcp-search",     "args": [] }
  ],
  "models": {
    "thinker": "gemma4:e2b",
    "worker":  "bonsai:4b",
    "sensor":  "qwen3-nano"
  },
  "shell": {
    "allowlist": ["go", "make", "git", "ls", "cat", "find"]
  },
  "ollama": {
    "keep_alive": "5m",
    "max_loaded_models": 3,
    "num_parallel": 1
  },
  "context": {
    "system_prompt": "you are a terse, senior engineer."
  }
}
```

`idle_shutdown_mins` is how long the daemon may sit with no client activity before it shuts itself down (default `30`). set it to `0` to use the default, or a negative value to keep the daemon running indefinitely.

`endpoints` lets you name inference backends (an ollama-compatible server each) with a `base_url` and optional `api_key`. set `provider` to the name of the profile you want active; leave it empty to use `ollama_base_url`. this lets you switch between local backends (ollama, lm studio, llama.cpp) without editing the base url each time.

`ollama` tunes the ollama backend. `keep_alive` (e.g. `5m`) is how long an idle model stays loaded; inari applies it on every request. `max_loaded_models` and `num_parallel` are server-start settings inari cannot set on a running `ollama serve`; `inari doctor` surfaces them as the `OLLAMA_MAX_LOADED_MODELS` / `OLLAMA_NUM_PARALLEL` env vars to export before starting ollama.

`context.system_prompt` is a global instruction prepended to every new session, on top of its own context (working-directory file tree, `AGENTS.md`). leave it empty for none.

`shell.allowlist` lists the commands the assistant may run without asking you first. anything on the list runs straight away; anything else pauses for your `[y]`/`[n]` before it runs. leave it empty to use the built-in default set (common read and build commands; network tools like `curl` are deliberately left off so they always ask). add a command here only if you are happy for the assistant to run it unattended.

---

## usage

### commands

- `inari`: launch the daemon and open the TUI (the default action)
- `inari tui`: open the TUI; assumes the daemon is already running
- `inari daemon`: run the daemon in the foreground
- `inari doctor`: check dependencies and daemon status
- `inari stop`: stop the running daemon
- `inari version`: print the version
- `inari help`: show usage

### inari (TUI)

The TUI is inspired by `k9s` and is entirely keyboard-driven.

**agents (main screen)**
- `s`: new session | `m`: assign model
- `c` / `enter`: open chat | `x`: delete session | `d`: describe
- `/`: filter sessions (type to narrow, `esc` clears)
- `l`: view logs | `r`: refresh | `q`: quit

**model selector** (opened with `m` or `ctrl+o`)
- `enter`: assign / pull | `u`: unload from RAM | `d`: delete from disk
- `q` / `esc`: cancel

**chat**
- `enter`: send message | `ctrl+o`: change model
- `ctrl+t`: tools panel | `ctrl+p`: command palette | `ctrl+g`: help
- `↑` / `↓`: input history | `esc`: stop response / exit tools panel / clear slash input

---

## roadmap

See [SPEC.md](SPEC.md#3-roadmap--milestones) for the project roadmap and build milestones.
