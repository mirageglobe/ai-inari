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

Runners are optional agents the thinker dispatches for background work.
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

`inari doctor` prints the exact `ollama pull` command if the base model is missing;
`inari doctor --models` goes further and runs each configured model through a real
tool-calling turn to confirm it works, not just that it is pulled.
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
    "runner":  ""
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

### per-project overlay (`.inari/config.json`)

drop a `.inari/config.json` in a project directory to tailor sessions opened there:

```json
{
  "context": { "system_prompt": "you are reviewing a payments service; be strict about money handling." },
  "exclude_dirs": ["testdata", "fixtures"]
}
```

- `context.system_prompt` replaces the global prompt for sessions in that directory (the more specific project prompt wins); the working-directory file tree and `AGENTS.md` context are kept.
- `exclude_dirs` adds extra directory names to prune from the injected file tree, on top of the built-in skips (`.git`, `node_modules`, ...).

only these two fields are honored. infra and security settings (socket, endpoints, provider, `shell.allowlist`, models, ...) are deliberately never read from a project file, so opening a session inside a cloned repo you do not fully trust cannot widen the shell allowlist or redirect the inference backend.

`shell.allowlist` lists the commands the assistant may run without asking you first. anything on the list runs straight away; anything else pauses for your `[y]`/`[n]` before it runs. leave it empty to use the built-in default set (common read and build commands; network tools like `curl` are deliberately left off so they always ask). add a command here only if you are happy for the assistant to run it unattended.

---

## usage

### commands

- `inari`: launch the daemon and open the TUI (the default action)
- `inari tui`: open the TUI; assumes the daemon is already running
- `inari chat --session <id> --message <text>`: send one message to an existing session and print the reply (headless, no TUI); `--message -` reads stdin, `--json` prints the reply as JSON
- `inari chat --new [--model <m>] [--cwd <p>] --message <text>`: create a fresh session and print the reply, a self-contained headless one-liner (no pre-existing session needed); the new session id is printed to stderr
- `inari try <tag>`: try out a candidate model you don't run yet; checks it resolves on the registry, pulls it, and runs a real tool-calling turn to confirm it works. `--check` only checks the tag resolves (no download)
- `inari daemon`: run the daemon; it backgrounds itself by default (so `inari stop` manages it), or pass `-f`/`--foreground` to keep it attached in the terminal (ctrl+c to quit)
- `inari doctor`: check dependencies and daemon status; add `--models` to also run each configured model through a real tool-calling turn and confirm it actually works, not just that it is pulled
- `inari stop`: stop the running daemon
- `inari version`: print the version
- `inari help`: show usage

### inari (TUI)

The TUI is inspired by `k9s` and is entirely keyboard-driven.

**sessions (main screen)**
- `s`: new session | `m`: assign model
- `c` / `enter`: open chat | `x`: delete session | `d`: describe
- `/`: filter sessions (type to narrow, `esc` clears)
- `l`: view logs | `r`: refresh | `q`: quit

**model selector** (opened with `m` or `ctrl+o`)
- `enter`: assign / pull | `u`: unload from RAM | `d`: delete from disk
- `q` / `esc`: cancel

**chat**
- `enter`: send message | `ctrl+o`: change model
- `!<cmd>`: run a shell command in the session directory, skipping the model (e.g. `!git status`, `!ls | wc -l`); the output shows in the chat and the model sees it too
- `ctrl+t`: tools panel | `ctrl+p`: command palette | `ctrl+g`: help
- `↑` / `↓`: input history | `esc`: stop response / clear slash input

**pop-up modals** (help, describe, logs, tools, model selector, theme)
- open centered over the current screen; `q` or `esc` closes and returns to it

---

## roadmap

See [SPEC.md](SPEC.md#3-roadmap--milestones) for the project roadmap and build milestones.
