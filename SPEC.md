# Inari - Project Spec

Security-first, minimalist local AI orchestrator.

---

## 1. Goals

**inari is a terminal coding assistant driven by local models.** every goal below
serves that. goals that did not are recorded as closed decisions in §12 rather
than left standing as aspirations nothing funds.

- Work on real projects from the terminal: a session opens in a directory, sees its layout and conventions, and reaches its files through typed, sandboxed tools.
- Run entirely on local models via Ollama: no cloud dependency, no network exposure, nothing leaving the machine.
- Keep the operator in the loop: every tool-call is audited, and anything outside the pre-consented set asks before it runs.
- Run several sessions concurrently without one blocking another.
- Stay fast on ordinary hardware: an interactive turn's cost (reasoning budget, prefill, render) is budgeted and measured against §6.3, never inherited from backend defaults.

## 2. Non-Goals

- Cloud or remote model backends.
- Multi-user or networked access.
- GUI / web interface.
- Model training or fine-tuning.

---

### 2.1 Development Strategy

**make it work → make it right.**

the guiding sequence for inari is: ship working features on concrete implementations first, then refactor toward open architecture once the right abstraction shape is known.

designing abstractions too early produces interfaces that fit the first implementation but break the moment a second is added. the right shape only becomes visible when writing real code against two concrete targets.

**practical sequence:**

1. **finish the basics** - prompt-based tool calling, session context, streaming stability. ship features that prove the design.
2. **add a second backend** - e.g. LM Studio (OpenAI-compatible). this is the moment the interface shape becomes obvious, not before.
3. **extract the abstraction** - **done.** `provider.Provider` (Ping, Chat, ChatStream, LoadModel, UnloadModel, ListModels, ListRunning) lives in `internal/provider`, `internal/ollama.Client` satisfies it under a compile-time check (`var _ provider.Provider = (*Client)(nil)`), and `internal/ipc.Server` holds only the interface. it was pulled from working code rather than invented upfront, which was the point. what it has **not** been tested against is a second concrete backend, so treat its shape as provisional until one lands.

**guard against premature abstraction:**

- the Ollama client is already isolated in `internal/ollama` - nothing outside imports Ollama-specific types directly. the boundary is there when needed.
- the `Provider` interface now exists; do **not** widen it speculatively. no plugin systems or backend registries until a second concrete backend forces the shape.
- when in doubt: duplicate once, abstract on the second duplication.

---

### 2.2 Terminology

- **session** - a persistent conversation container: cwd, model assignment, message history, tags. this is what the TUI's `Sessions` view lists and lets you create and select. a session does nothing on its own initiative, which is why the view is not called `Agents`.
- **model** - the local model assigned to a session. inari has exactly one model role: the one you talk to. there is no dispatcher, no background worker, and no tier hierarchy; §12 records why the earlier sensor/worker/thinker/runner scheme was removed rather than completed.
- **builtin** - one of the six typed, cwd-sandboxed tools inarid declares to the model (§4.6). deliberately distinct from `execute_shell_command`, which is neither typed nor path-sandboxed (§8.4).
- **hardware tier** - the `4gb`/`8gb`/`16gb`/`32gb` rows in §6.1. these size a model against available memory. they are not roles, and nothing dispatches between them.
- `models.thinker` in `config.json` names the model assigned to new sessions. `models.runner` is **deprecated and unread** by anything except `doctor`, which prints a warning saying so; it is scheduled for removal.

---

## 3. Roadmap

items carry a component tag (`[inarid]`/`[inarit]`/`[inari]`) and a difficulty tag
(`[easy]`/`[medium]`/`[hard]`). a work-type tag is optional: `[issue]` for a bug or
defect, `[feature]` for new capability (untagged reads as a feature). issues and
features live inline in Near-term/Ideas under these tags rather than in a separate
section.

### Milestones

#### M1 - UDS Bridge & Config
- [x] `[inarid]` starts and binds UDS socket.
- [x] `[inarit/inarid]` connects, performs handshake, and does a basic ping/pong JSON-RPC round-trip.
- [x] `[inarid]` `config.json` parsed at startup (created from defaults if absent); XDG path `~/.config/inari/config.json`.
- [x] `[inarid]` MCP connectors spawned as child processes.

#### M2 - Sessions UI
- [x] `[inarit]` Bubble Tea table renders active sessions.
- [x] `[inarit/inarid]` sessions update in real time from daemon events.
- [x] `[inarit]` keyboard navigation (select, quit).

#### M3 - Ollama Integration & Chat
- [x] `[inarid]` daemon POSTs to Ollama `/api/chat` and streams tokens. *(the memory-budget throttle originally scoped here was never wired; the `scheduler` package exists but is unused. tracked as an Ideas item below.)*
- [x] `[inarit/inarid]` token stream forwarded to the inarit chat view.
- [x] `[inarit]` interactive chat view wires to the session's assigned model.
- [x] `[inarid]` message history scoped to session; detach/reattach preserves session state.

### Near-term
- [ ] `[inari]` `[easy]` **widen the probe suite beyond one task per builtin** - `inari probe` currently aims exactly one prompt at each builtin, which measures "can the model reach this tool at all" but not selection under ambiguity (two plausible tools for one question, e.g. find_files vs grep_file for "where is the config"), multi-step chains, or a project layout unlike the fixture. add a second task class with a deliberately ambiguous target and score by preferred-vs-acceptable rather than a single `want`.
- [ ] `[inarit]` `[easy]` **surface turn metrics in the chat view** - the daemon now records tokens/sec, time-to-first-token and the prefill/decode split for every generation round (see **inference telemetry** in Done) and logs a `turn.metrics` line under `--verbose`, but none of it reaches the TUI. scope: decide where a per-turn cost line belongs in the chat view (footer, a dimmed line after the reply, or only behind a toggle) and forward the numbers over IPC. held back from the telemetry change deliberately: placement is a UI decision, not a mechanical one.
- [ ] `[inarid]` `[medium]` **capture and control reasoning tokens** - measured, so no longer a verify-first item (see §6.3.1). `gemma4:e2b`, the **default thinker**, returns its chain of thought in a separate `message.thinking` field rather than inline in `message.content`; `provider.Message` declares only `role`/`content`/`tool_calls`, so it is dropped at unmarshal, and inarid never sends the `think` parameter, so the model's own default (on) applies. inari therefore waits for tokens it then discards: medians of 3 on the reference machine put the cost at 244 vs 22 decode tokens and 4.87 s vs 0.77 s on an easy prompt, 985 vs 434 and 18.18 s vs 8.21 s on a hard one, for answers of comparable length. **correctness was not graded**, so the hard-prompt case may well be earning its keep; that is the reason to make it controllable rather than simply switch it off. scope: add a `Thinking` field to `provider.Message` (json tag `thinking`), thread the `think` request parameter through `ChatRequest`, add a per-session setting with a visible state (§6.4 invariant 3), and fold the captured text behind a toggle rather than dropping it. the r1 case this item originally described (inline `<think>` in `content`) still needs its own check against `deepseek-r1:14b`, since a second parse path may be required.
- [ ] `[inarit]` `[medium]` **coalesce streamed token frames on a display-rate budget** - one backend chunk currently produces one UDS JSON frame, one `ChatTokenMsg`, one `streamBuf` concatenation, one full viewport rebuild with hard-wrap, and one bubble tea `View()` (§6.3.3). at the measured 56 tok/s that is 56 full frame renders per second, and `c.streamBuf += msg.Token` is quadratic in reply length. scope: accumulate tokens in a `strings.Builder` and flush to the viewport on a ticker at a display-sensible rate (~30 fps) rather than per token, keeping the final flush on `ChatDoneMsg` so no tail is lost. the sharp edge to test: an interrupt or an error mid-stream must still flush whatever was buffered, or the visible reply silently truncates.
- [ ] `[inarid]` `[easy]` **doctor: report the memory knobs that multiply** - `inari doctor` surfaces `OLLAMA_MAX_LOADED_MODELS` and `OLLAMA_NUM_PARALLEL` but not `OLLAMA_FLASH_ATTENTION` or `OLLAMA_KV_CACHE_TYPE`, both present in `ollama serve --help` on 0.32.6 and both server-start only. it also does not say that ollama allocates `num_ctx` **per parallel slot**, so `num_parallel: 4` with an 8192-token window reserves 32768 tokens of kv cache (§6.3.4). scope: report both variables with their effective values, and compute and print the implied kv reservation from the session's window times `num_parallel`. **do not set them**: they are globals affecting every model on that server, and unsupported architectures fall back to `f16` without reporting it, so advice is honest where silent configuration would not be.
- [ ] `[inarid]` `[easy]` **stop presenting prefill rate as throughput** - the `turn.metrics` record derives its prefill numbers from `prompt_eval_count` over `prompt_eval_duration`, but that count reports tokens *submitted*, not tokens *computed*: on a prefix-cache hit it stays at the full history length while the duration collapses, so the derived rate reads ~10,000 tok/s and means nothing (§6.3.2). scope: drop or rename any prefill-rate field, and add a cache-hit signal derived from `prompt_eval_duration` instead, which is the only counter that can see it. this is the same defect class as the `make lint` staticcheck report: an instrument that answers confidently without measuring.
- [ ] `[inari]` `[medium]` **`make bench-turn`: make the §6.3 numbers reproducible** - every figure in the cost model was measured by hand against `/api/chat`. without a committed harness they rot into folklore the moment ollama or a model tag moves. scope: a small target that drives a fixed prompt set through the configured thinker with thinking on and off, reports medians of n runs for decode tokens, wall clock and prefill, and prints the reference-machine row alongside so drift is visible. it must **not** assert a threshold; hardware varies too much for that to be anything but a flaky test.
- [ ] `[inarid]` `[medium]` **move prompt-based tool calling to a JSON schema** - §4.6's fallback still specifies `format: "json"`, which only constrains the output to *some* valid JSON and leaves field names, types and required keys to chance. ollama now accepts a full JSON **schema** in `format`, enforced by constrained decoding, which makes an off-schema tool call unrepresentable rather than merely unlikely, and skips the tokens the model would have spent deciding on formatting. scope: emit a schema per declared tool and select it when the fallback path is active. the caveat to respect: small quantised models degrade on deeply nested schemas, so keep the tool schemas flat (one object, scalar fields) rather than modelling the whole tool union in one nested shape.
- [ ] `[inarid]` `[medium]` **validate shell argument paths before auto-approving** - implements the §8.4 decision. `shellAutoApproved` currently splits out the binary base name and looks it up in the allowlist; arguments are never inspected, so `cat` plus an absolute path runs with no prompt. scope: resolve each argument that looks like a path against the session `cwd` using the same `sandboxPath` the typed builtins use, and return false from the gate when any of them escapes, which drops the call into the existing approval prompt rather than blocking it. the sharp edge to test: arguments that are **not** paths (`-l`, `--porcelain`, a grep pattern, a make target) must not be mistaken for escaping paths, or every ordinary call starts prompting and the feature is worse than useless.
- [ ] `[inarid]` `[easy]` **delete the orchestration leftovers** - follows the §12 decision to close the herd direction. `internal/scheduler` (112 lines) has zero call sites; `memory_budget_mb` is a config field nothing reads; `models.runner` is read only by `doctor`, which prints a warning that the tier is unused. scope: remove the package, remove both config fields, and drop the doctor lines that report them. keeping dead code that implements a cancelled goal is how the goal creeps back.
- [ ] `[inari]` `[easy]` **`make lint` swallows every staticcheck failure as "not found"** - the recipe is `command -v staticcheck >/dev/null && staticcheck ./... || printf "staticcheck not found ..."`. in a shell `A && B || C` runs `C` whenever **B** fails, not only when `A` does, so any non-zero staticcheck exit prints the not-found message and `make lint` still exits 0. that hides two different things: a genuinely broken install (one was observed, the binary built against go1.24.1 while the module requires go1.24.2) and, more seriously, **every real lint finding**, which is discarded silently and can never fail the build. scope: test for the binary in its own conditional and let staticcheck's exit status propagate. the sharp edge: `make test` depends on this target, so the fix will surface findings that have been accumulating unseen; expect the first run to fail and budget for that rather than reverting the fix. same defect class as the prefill-rate item above and the §8 claims corrected in this branch: an instrument that answers confidently without measuring.
- [ ] `[inarit]` `[easy]` **`↑` recalls only what was sent to the model** - `inputHistory` is appended in exactly one place, `sendChat` (`tui/views/chat_send.go`), so both escape hatches bypass it: a `!` shell line is handled by `runShell`, which never records, and a `/` slash command by `handleSlashCommand`, which never records either. the effect is backwards: the two kinds of input most worth repeating, a shell command you are iterating on and a slash command carrying an argument, are the two that cannot be recalled, while ordinary prose you would not mind retyping is kept. more noticeable now that `!` is a sticky mode rather than a per-line prefix. scope: record both at their submit branches. **the decision to make deliberately** is whether they share one ring with chat messages or get their own: sharing is less code, but then `↑` in shell mode surfaces prose that would be nonsense as a command, and `↑` in chat surfaces shell lines.

### Ideas
- [ ] `[inarid]` `[hard]` **scripting layer for agent execution (Yaegi vs Deno vs status quo)** - evaluated replacing or augmenting the model's `execute_shell_command` path with an embedded interpreter. **decision so far: not the in-process Go interpreter as pitched.** the deciding axis is the trust boundary (the *model* is the untrusted author), not execution latency (subprocess spawn ~30ms is immaterial against multi-second inference and the render hot path, both measured). options: (a) **Yaegi** (in-process Go) is fast and zero-external-dependency, but has no real sandbox: disabling `unsafe`/`syscall` still leaves `os`/`os/exec`/`net`, so model-authored Go is as dangerous as shell or worse and has no allowlist concept; only safe for *user*-authored plugins/macros (trusted, config-time), never model output. (b) **Deno** is a genuine deny-by-default permission sandbox (`--allow-read=<cwd>` only, no net/write/spawn), the safer runtime for model-authored code, but adds an external runtime and re-adds process spawn. (c) **status quo + more typed builtins** (chosen for now): small local models emit structured tool calls far more reliably than compilable Go/JS, so expanding the pure-Go builtin surface (shipped: `find_files`, `read_lines`; plus allowlisted `awk`/`sed`/`jq`) covers the need with no new runtime. revisit Yaegi only as a user-plugin extension mechanism, or Deno if model-authored scripting becomes a hard requirement.
- [ ] `[inarit/inarid]` `[hard]` **long-term task planning from high-level prompts** - decompose a high-level user goal into a tracked, multi-step plan that the session executes and checks off. exploratory; no concrete entry point yet, so parked here until the shape is clearer.
- [ ] `[inarit]` `[medium]` **pre-send prompt optimisation (autocorrect)** - the prompt-optimisation half of the pre-send intercept layer (its security/validation half is **pre-send message intercept**, near-term, which stays daemon-side as the authoritative gate). client-side because it is a UX affordance, not a security control: before the message leaves the TUI, lightly rewrite it to improve model accuracy - fix obvious typos, expand terse fragments, normalise formatting - without altering intent. the user must preview the rewrite in the input box and accept or reject it before send (or toggle the feature off); it never silently distorts what the user asked.
- [ ] `[inarit]` `[hard]` **chat viewport character selection** - build on line selection to add character-level precision: within the selected rows, re-parse ANSI sequences to locate byte ranges and inject highlight styles mid-sequence, so a drag can start and end partway through a line. builds on the shipped line-selection work (see Done). parked as exploratory: whole-row selection already covers the common copy case, so the mid-sequence ANSI re-parsing is not yet worth the complexity.
- [ ] `[inarid]` `[hard]` **MCP: one project, three steps** - previously three separate Ideas entries that could not be done independently; merged so the ordering is explicit. the library swap comes first, dispatch is what makes it useful, and the filesystem connector is the first consumer that proves the loop. sub-items below are the original entries, unchanged:
  - **MCP tool-call dispatch** - `internal/mcp/host.go` `Call()` is a TODO stub; audit logging exists but actual JSON-RPC dispatch over stdio is not implemented. prerequisite for the MCP integration work below (was near-term for M4; parked here until the wider MCP direction settles).
  - **MCP filesystem connector (layer 3)** - once the tool-call loop exists, replace built-in tools with `@modelcontextprotocol/server-filesystem` spawned via mcp-go. this is a natural extension of the MCP integration work below.
  - MCP integration - replace `internal/mcp` with `github.com/mark3labs/mcp-go`; connectors (Linear, Slack, Google Drive, etc.) configured via `config.json`
- [ ] `[inarid]` `[medium]` **destructive action prevention (§8.2)**: cwd enforcement (`sandboxPath` in `internal/ipc/tools.go`) and a tool-call loop cap (`maxToolRounds = 10` in `internal/ipc/stream.go`) are shipped, alongside per-call size caps; remaining scope is a true file-op-count cap and dry-run previews for caution-tier tool-calls. risk-tiered auto-approval is done (safe builtins auto-execute, allowlisted `execute_shell_command` binaries auto-execute, unlisted ones confirm)
- [ ] `[inarid]` `[medium]` **prompt-based tool calling** - for models without native function-calling support, inject tool definitions as plain text into the system prompt and set `format: "json"`; inarid parses the JSON response to detect tool calls. select mode via session config or auto-detect from model name. makes layer 2 work on any instruction-following model (hermes-3-pro, qwen3-coder, etc.)
- [ ] `[inarid]` `[medium]` **second provider backend** - merged: the abstraction item and the vLLM item were the same work seen from two ends, and the former already said it overlapped the endpoint-profiles item. the interface exists (§2.1); what is missing is one more concrete implementation to test its shape against. the candidate matters less than having a second one at all. original entries:
  - **provider abstraction** - the `Provider` interface already exists (`internal/provider/provider.go`: Chat, ChatStream, LoadModel, UnloadModel, ListModels, ListRunning, Ping) and inarid's core already talks only to it. remaining work is a second concrete provider (vLLM, LM Studio, llama.cpp server, or a cloud API) selected via `provider` in `config.json`; overlaps with the local endpoint profiles item above.
  - consider adding vLLM as an alternative backend to Ollama - vLLM is OpenAI-compatible and may offer better throughput on CUDA hardware; evaluate alongside the local endpoint profiles item as a concrete second backend candidate
- [ ] `[inarid]` `[hard]` **context compression / eviction** - narrowed by measurement (§6.3.2): KV-cache reuse across turns and prefix caching at the provider level are **already working** and need nothing built here; a follow-up turn that preserves its prefix re-prefills 16x faster than a cold one, and inari's frozen system prompt is what keeps that hit. what remains is the part the backend cannot decide for inari: *what* to drop. scope: selective message eviction and rolling summary compression, both of which rewrite history and therefore forfeit the prefix cache for one turn by construction (§6.4 invariant 2). they have to pay for themselves across the turns that follow, which argues for compacting rarely and deeply rather than on every threshold crossing.
- [ ] `[inarid]` `[hard]` **vector store / RAG context** - replace or augment flat JSON session storage with a semantic retrieval layer. progression: (1) sqlite as structured store; (2) sqlite-vec (sqlite vector extension) for local embeddings - single file, no external service, fits the Go daemon cleanly; (3) full RAG pipeline with chunking, a local embedding model (~100MB), and ranked context injection. at query time, the user message is embedded and the top-k semantically similar chunks are injected into the prompt rather than the full history dump. benefit: small models see only relevant context, reducing token pressure and improving response quality. a global "master context" store (outside any cwd) could be maintained alongside per-session history, giving all sessions access to persistent personal or cross-project knowledge.

### Done

the completed-work log lives in **[docs/done.md](docs/done.md)**. it was moved out
of this file when it reached 51.9% of the spec's bytes (79,758 of 153,599, with
single entries over 2,900 characters), which pushed the architecture, security and
roadmap sections a reader actually needs into the remaining half.

**single source of truth for change history:** `CHANGELOG.md` records *what*
shipped per release for users; `docs/done.md` records *why* it was done for
developers and agents; this roadmap records only what is *not* done. an entry
belongs in exactly one of the three.


## 4. Architecture

### 4.1 System Overview

```
  you (inari TUI)
      |
      |  JSON-RPC over Unix socket  (chmod 0600)
      |
  inarid (daemon)
    ├── session store - persists sessions + history to ~/.local/share/inari/sessions/
    ├── ollama client - sends full message history to local models
    ├── scheduler - semaphore memory-budget throttle (library only; not wired)
    └── audit logger - append-only record of tool calls, run and refused
```

**stack:** Go · Bubble Tea / LipGloss · Ollama

### 4.2 Component Topology

```
┌─────────────────────────────┐
│         inari (TUI)         │  ← user-facing client
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

### 4.2.1 Binary & Command Surface

inari ships as a single binary; the first argument selects a mode, and there is no
separate daemon executable. this keeps distribution to one artifact (a single
`brew install`) and guarantees the client and daemon are always the same build, so
the private UDS protocol never has to negotiate versions across two binaries.

| command         | mode                                  |
| :-------------- | :------------------------------------ |
| `inari`         | default: fork daemon, then run TUI    |
| `inari tui`     | TUI only; daemon must be running      |
| `inari chat`    | headless one-shot; print reply        |
| `inari try`     | resolve, pull, tool-call test a model |
| `inari probe`   | audit which builtin the model selects |
| `inari daemon`  | run daemon in foreground              |
| `inari doctor`  | preflight: ollama, config, base model |
| `inari stop`    | signal daemon to exit                 |
| `inari version` | print version                         |
| `inari help`    | usage                                 |

bare `inari` (and its `start` alias, kept so `make start` keeps working) re-execs
the binary as `inari daemon --background` via `os.Executable()`
(`cmd/inari/process.go`), writes a PID file, then runs the TUI in the foreground.
the daemon self-terminates after `idle_shutdown_mins` of no client activity, so the
common path needs no explicit `stop`. `doctor` exits non-zero when a required check
fails (ollama unreachable, or the base/thinker model not pulled), so it doubles as a
setup or CI preflight gate.

### 4.3 Session Model

Sessions are the primary entity in inari. A session is a named chat context
(e.g. "Arctic Fox") that exists independently of any model. The user creates a
session first, then optionally assigns a model to it. Chat history is stored
inside the session in inarid - clients are stateless and hold no history locally.

This means:
- Restarting inari reconnects to the existing sessions without losing any conversation.
- A session with no model assigned is valid; the model can be swapped at any time.
- `session.chat` takes a session ID and a single new message; inarid appends it,
  sends the full history to Ollama, stores the reply, and returns only the text.
- Restarting inarid reloads all sessions from disk - history and model assignment
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
- Permissions: `chmod 0600` - owner-only access.
- Protocol: JSON-RPC 2.0 for all control RPCs; newline-delimited JSON frames for streaming chat.
- Daemon persists sessions on client detach; client reconnects by session ID.

**Session RPCs:**

| Method             | Params         | Returns         | Description                                                                         |
| :----------------- | :------------- | :-------------- | :---------------------------------------------------------------------------------- |
| `session.list`     | -              | `SessionInfo[]` | summary of all sessions (no history on wire)                                        |
| `session.create`   | `{name, cwd?}` | `SessionInfo`   | create a named session; optional `cwd` enables filesystem context                   |
| `session.delete`   | `{id}`         | `"ok"`          | remove session and its history                                                      |
| `session.assign`   | `{id, model}`  | `"ok"`          | attach a model to a session                                                         |
| `session.unassign` | `{id}`         | `"ok"`          | detach the model from a session                                                     |
| `session.chat`     | `{id, text}`   | `string`        | blocking: append message, return full reply                                         |
| `session.stream`   | `{id, text}`   | *(see below)*   | streaming: append message, stream token chunks                                      |
| `session.history`  | `{id}`         | `Message[]`     | full message history for a session                                                  |
| `session.compact`  | `{id}`         | `"ok"`          | summarise history via the session's own model and replace old turns                 |
| `ollama.show`      | `{model}`      | `string[]`      | capability tags for a model (e.g. `["completion","tools"]`); empty array if unknown |

**Streaming chat (`session.stream`):**

`session.stream` uses a **dedicated per-call UDS connection** rather than the shared RPC connection. This allows multiple simultaneous streams (one per open chat view) without head-of-line blocking.

Protocol over the dedicated connection:

1. client dials a new `unix` connection to `/tmp/inari.sock`
2. client sends a normal JSON-RPC 2.0 request: `{"method":"session.stream","params":{"id":"...","text":"..."}}`
3. inarid responds with a stream of newline-delimited JSON frames:
   ```json
   {"token":"Hello"}
   {"token":" world"}
   {"tool_approval_request":{"tool":"execute_shell_command","args":{"command":"go","args":["test","./..."]}}}
   {"token":"Tests passed."}
   {"done":true}
   ```
   when a `tool_approval_request` frame arrives, inari pauses rendering and waits for the user to press `[y]` or `[n]`. it then sends `{"tool_approved":true}` or `{"tool_approved":false}` back over the same connection. inarid blocks until it receives the response before executing or skipping the tool call.
4. on `done`, inarid has persisted the full reply to the session store; client closes the connection
5. on error, inarid sends `{"error":"<message>"}` and closes

inari opens one dedicated connection per active `session.stream` call. the shared `Client` connection remains exclusively for control RPCs and is never blocked by in-flight streams.

**multiple concurrent streams:**

within a single inari TUI, the user can spawn multiple named chat sessions (each displayed as a row in the sessions view). each session runs independently - it can have a model assigned and an active generation in flight simultaneously. because each `session.stream` call uses its own dedicated UDS connection, all sessions can stream concurrently without blocking one another. inarid handles each stream in its own goroutine via the accept loop.

**message routing in inari:**

token messages (`ChatTokenMsg`, `ChatDoneMsg`) carry a `SessionID` field. the root model routes them directly to the correct `Chat` view in `m.chats[sessionID]` - regardless of which view is currently displayed. this allows background sessions to accumulate tokens invisibly; when the user switches back, the chat view already shows the partial or complete response.

### 4.5 Concurrency & Scheduling

- Each Ollama session runs in its own goroutine.
- A memory-budget semaphore to gate concurrent sessions is planned but not yet wired (see Ideas); today each session streams unthrottled.
- Multiple simultaneous chat streams are supported - each uses its own UDS connection.
- Slow/background tasks continue when the TUI is detached.

### 4.6 Filesystem Awareness - Three-Layer Model

sessions can be given awareness of the local filesystem in three progressively richer layers. each layer is a prerequisite for the next.

**layer 1 - directory context (system prompt injection)**

inari passes the current working directory when creating a session. inarid resolves a shallow file tree (`find . -maxdepth 3`, filtered by `.gitignore`) and prepends it as a system message:

```
system: working directory: /path/to/project
<file tree>
```

the model can reason about the project layout and refer to files by path, but cannot read their content. this requires no changes to the ollama request format and works with every model.

**layer 2 - read-only file access (agentic tool-call loop)**

inarid declares five built-in tools in the ollama `/api/chat` request for sessions that have `cwd` set:

| tool                    | input               | output                                   |
| :---------------------- | :------------------ | :--------------------------------------- |
| `read_file`             | `{path}`            | file contents (text only)                |
| `list_dir`              | `{path}`            | directory listing (names only)           |
| `grep_file`             | `{path, pattern}`   | matching lines with filename and line no |
| `stat_file`             | `{path}`            | size, mtime, type                        |
| `execute_shell_command` | `{command, args[]}` | stdout+stderr, exit code as text         |

**naming convention:** tool names follow `verb_noun` (e.g. `read_file`, `list_dir`, `execute_shell_command`); reads as an instruction and aligns with common tool-calling schemas (MCP, OpenAI).

the six typed builtins are sandboxed: their paths are resolved relative to `cwd` and cannot escape it (no `../` traversal). **`execute_shell_command` is not.** it takes `command` and `args` as opaque strings, sets `cmd.Dir` and validates no paths, so an allowlisted binary given an absolute path reads outside the session; see §8.4. write operations are out of scope for the typed builtins only.

when ollama returns a `tool_calls` response, inarid's `handleStream` loop:

1. executes each tool call inside the sandbox
2. appends a `tool` role message with the result
3. re-sends the full message history to ollama
4. repeats until ollama returns a `message` (text) response
5. streams the final text back to inari as normal token frames

this requires ollama tool-call support - only models that explicitly declare function-calling capability in their model card will use the tools. others silently ignore them and respond with plain text.

**models with tool-call support (layer 2 compatible):**

| model               | notes                                         |
| :------------------ | :-------------------------------------------- |
| qwen3 (any size)    | recommended; strong tool use across all sizes |
| llama3.1 / llama3.2 | instruct variants only                        |
| mistral-nemo        | solid tool support                            |
| mistral 7b instruct | function-calling variants                     |
| command-r           | designed for agentic use                      |

**models without tool-call support (layer 1 only):**

| model                    | behaviour                                       |
| :----------------------- | :---------------------------------------------- |
| phi3 / phi4              | ignores tools, responds with text               |
| gemma2                   | ignores tools, responds with text               |
| deepseek-r1              | most variants do not support tool calls         |
| older / chat-only models | silent no-op - tools declared but never invoked |

assigning a non-tool-capable model to a session with `cwd` set is safe - tools are declared in the request but the model will not invoke them. layer 1 (file tree in system prompt) still applies and provides value regardless of model capability.

**prompt-based tool calling (fallback for non-native models):**

the native `tools` API parameter solves the "silent ignore" problem only for models that natively support it. for everything else - including strong local models like `hermes-3-pro-8b` or `qwen3-coder` - a more reliable approach is:

1. **do not use the `tools` parameter.** inject tool definitions as plain text into the system prompt instead:
   ```
   you have access to the following tools. when you need to use one, respond only with valid JSON in this format:
   {"tool": "read_file", "path": "relative/path"}
   {"tool": "list_dir", "path": "."}
   ```
2. **constrain the output with a JSON schema.** ollama's `format` parameter accepts a full JSON schema, not just the string `"json"`, and enforces it by constrained decoding: the sampler masks tokens that cannot continue a valid document, so an off-schema reply is unrepresentable rather than merely unlikely, and no tokens are spent deciding on formatting. plain `format: "json"` (JSON mode) is the weaker fallback: it guarantees *some* valid JSON and nothing about field names, types or required keys. keep schemas flat (one object, scalar fields); small quantised models degrade noticeably on deeply nested shapes.
3. **inarid parses the response.** if the JSON response contains a `tool` key, it is treated as a tool call; otherwise it is a plain text reply.

this approach trades API cleanliness for broad model compatibility. it is the recommended strategy for local SLMs where native function-calling is patchy or absent.

**roadmap:** inarid should detect model capability at session creation (or via config) and automatically select native vs. prompt-based tool calling. the `handleStream` loop is the same either way - only the request format and response parser differ.

**layer 3 - MCP filesystem connector**

once the tool-call loop exists, built-in tools can be replaced by `@modelcontextprotocol/server-filesystem` spawned via mcp-go. the loop delegates tool execution to the MCP host instead of running it inline. this unlocks the full MCP tool surface (search, write when permitted, etc.) and the same loop handles all future connectors uniformly.

**`session.create` RPC extension (layers 1 + 2):**

```json
{"name": "my session", "cwd": "/path/to/project"}
```

`cwd` is optional. when absent, the session behaves as today - no filesystem context, no tools declared.

### 4.7 Configuration Hierarchy

when inarid loads, it merges configuration files using the following precedence (highest to lowest):
1. local project-scoped configuration (`.inari/config.json` in the session's working directory)
2. global configuration (`~/.config/inari/config.json`)
3. built-in defaults

only configuration fields explicitly defined in the local file override the global settings; other fields are inherited.

---

## 5. Components Deep-dive

### 5.1 `inarid` - Daemon Subsystems

| Subsystem     | Responsibility                                                                                                                                                                                            |
| :------------ | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| UDS Server    | Accept and authenticate client connections                                                                                                                                                                |
| Session Store | Own named sessions with chat history; persists to JSON files on disk; survives daemon restart                                                                                                             |
| MCP Host      | Spawn and manage MCP connectors via `mcp-go` (Linear, Slack, Google Drive, etc.); current `internal/mcp` is a hand-rolled fallback - low migration risk as the protocol is stable JSON-RPC 2.0 over stdio |
| Ollama Client | POST to `/api/chat`; stream tokens back to session                                                                                                                                                        |
| Scheduler     | Semaphore-based concurrency throttle per resource tier                                                                                                                                                    |
| Audit Logger  | Append-only log of all JSON-RPC tool-calls with timestamps                                                                                                                                                |

### 5.2 `inari` - Client

| view     | opened by            | description                                                    |
| :------- | :------------------- | :------------------------------------------------------------- |
| Sessions | default view         | table of all sessions with model and status                    |
| Chat     | `enter` on a session | interactive chat; slash commands, tool approval, input history |
| Logs     | `/logs`              | tail output of selected session                                |
| Describe | `/describe`          | full session metadata and config                               |
| Model    | `ctrl+o`             | model selector; assign, pull, unload, delete                   |

**footer layout (all views):**

```
label | name | model | tokens | cwd
[cwd] <path>              ← omitted when no cwd is set
<status message>          ← transient; cleared on next keypress
<input widget>
<hint bar>
```

the footer is assembled by `renderFooter` in `tui/views/footer.go` and shared across all views.

**sessions hotkeys:** the sessions popup is hotkey-only, no text input, no slash commands. model selection, export, logs, and describe all moved to chat (`/model`, `/model unload`, `/export`, `/logs`, `/describe`) since chat is the main view and owns every shared command.

| key         | effect                                                         |
| :---------- | :------------------------------------------------------------- |
| `a`         | create session                                                 |
| `enter`     | open chat for the session under the cursor                     |
| `x`         | delete session under the cursor                                |
| `q` / `esc` | close popup and return to chat (no-op in the full-screen view) |

**chat slash commands:**

| command         | effect                                                        |
| :-------------- | :------------------------------------------------------------ |
| `/clear`        | wipe session message history                                  |
| `/compact`      | summarise history via session's own model; replaces old turns |
| `/copy`         | copy last assistant response to clipboard                     |
| `/export`       | save full session history to a text file                      |
| `/model`        | open model selector for this session                          |
| `/model unload` | unload the assigned model                                     |
| `/tools`        | toggle built-in tools panel for this session                  |
| `/describe`     | open session metadata/config view                             |
| `/logs`         | open the inarid log viewer                                    |
| `/sessions`     | open sessions as a popup over chat; [q]/[esc] closes          |
| `/chat`         | jump to default session's chat                                |
| `/refresh`      | reload the session list in background                         |
| `/theme`        | cycle theme                                                   |
| `/help`         | toggle help overlay                                           |
| `/quit`         | quit                                                          |

#### 5.2.1 Offline resilience

the root model polls inarid via `ConnStatusMsg` on a regular tick. when the daemon is unreachable, every `Chat` view is updated with `WithOffline(true)`. in that state:

- the `[enter] send` key binding is suppressed - pressing Enter does nothing.
- the hint line replaces the key-binding row with `inari is offline` (rendered in red).
- the input textarea remains editable so the user can compose a message while waiting.
- when connectivity is restored (`ConnStatusMsg{OK: true}`), all chats are updated with `WithOffline(false)` and normal behaviour resumes immediately - no queued messages are replayed.

queuing was explicitly not chosen: a silently queued message submitted minutes later (possibly to a cold model) is more surprising than a clear offline signal.

#### 5.2.2 Viewport quirks (`bubbles v0.18.0`)

**`GotoBottom` undershoots when content overflows the pane.**

`viewport.SetContent` in bubbles v0.18.0 splits content on `\n` only - it does not perform terminal line-wrapping. `GotoBottom` computes its offset from `len(lines) - height`, where `lines` is the raw newline count. Long styled lines (e.g. a multi-sentence assistant reply with no embedded newlines) count as 1 line but visually wrap across multiple terminal rows. Once accumulated wrapping exceeds the pane height, `GotoBottom` undershoots and new streaming tokens appear below the visible area.

**fix:** call `ansi.Hardwrap(content, vp.Width, true)` before `SetContent`. This inserts real `\n` characters at the terminal width (ANSI-aware, so escape codes don't inflate the count), making the stored line count match the visual row count. See `setViewportContent` in `tui/views/chat.go`.

---

## 6. Resource & Performance Model

§6.1 curates models by the hardware they fit; §6.3 measures what a turn actually
costs on that hardware. there is no scheduler and no tier hierarchy: inari runs
the one model assigned to the session, and concurrency is bounded by how many
sessions the operator opens.

**`memory_budget_mb` is not enforced.** the `internal/scheduler` semaphore that
would have enforced it has no call sites anywhere in the tree, so the budget is a
config field that does nothing. previous wording here claimed the scheduler
"blocks model loading if the budget would be exceeded", which was never true.
both the field and the package are scheduled for removal (§12).

### 6.1 Ollama Model Curation

curated picks by hardware tier and role. pull via `ollama pull <tag>`. prefer `q4_k_m` quant unless the tier has headroom for `q8_0`.

`size` is the **resident** footprint (what `ollama ps` reports once loaded), which is what decides whether a model fits a tier. it is not the download: `gemma4:e2b` measures 1.7 GB resident against 7.2 GB on disk, a 4.2x gap (§6.3.4). check `ollama list` before promising a tier's users a small download.

<!-- BEGIN generated: §6.1 tables from tui/views/curated.go CuratedModels; run `make curated-sync` -->

#### general

| tier | model       | size   | notes                                                   |
| :--- | :---------- | :----- | :------------------------------------------------------ |
| 32gb | qwen3.6:27b | ~16gb  | alibaba; near-frontier chat and review                  |
| 16gb | phi4:14b    | ~8gb   | microsoft; strong multi-file reasoning                  |
| 16gb | gemma4:12b  | ~7.6gb | google; 12b dense; strong general chat                  |
| 8gb  | gemma4:e4b  | ~2.7gb | 4.5b effective; fast routing and quick queries          |
| 8gb  | gemma4:e2b  | ~1.5gb | 2b effective; leaner and faster than e4b, lower quality |
| 4gb  | llama3.2:3b | ~2gb   | meta; best chat and reasoning within 4gb                |

#### coding

| tier | model                    | size  | notes                                        |
| :--- | :----------------------- | :---- | :------------------------------------------- |
| 32gb | qwen3.6:27b-coding-nvfp4 | ~18gb | alibaba; near-frontier generation and review |
| 16gb | deepseek-r1:14b          | ~9gb  | r1-671b distil; strong coding and reasoning  |
| 8gb  | deepseek-r1:8b           | ~5gb  | r1-671b distil; fits 8gb; coding+reasoning   |
| 4gb  | llama3.2:3b              | ~2gb  | meta; best within 4gb budget                 |

<!-- END generated: §6.1 tables -->

### 6.2 Keeping the curation current (findings)

the table is dual-maintained (`tui/views/curated.go` `CuratedModels` + §6.1 above) and drifts; the last rebuild found two dead tags shipping in a "live" list (`gemma4:27b` and `phi-4:14b`, both 404 - the latter a `phi-4`/`phi4` typo). two cheap mechanisms, split by what each can actually prove:

- **tag resolves (does it 404):** an HTTP HEAD against the ollama registry is enough and needs no pull - `curl -s -o /dev/null -w "%{http_code}" https://registry.ollama.ai/v2/library/<model>/manifests/<tag>` returns 200 vs 404. verified against the current list. fold into a `make check-models` target or `inari doctor` to catch dead tags without downloading gigabytes.
- **model actually runs + invokes tools:** resolution is not function; that check is the headless tool-calling smoke test (roadmap items "`inari doctor`: verify pulled models actually work" and "discover + locally test new candidate models"), which drives a real turn and checks the audit log for a `tool.call` entry.

drift itself is killed at the source: `CuratedModels` (`tui/views/curated.go`) is the single source and the §6.1 tables above are generated from it (`RenderCuratedTables`, between the marker comments). edit `CuratedModels`, run `make curated-sync`, and `make test` fails if the table is left stale (`TestCuratedTablesInSync`).

### 6.3 Inference cost model (measured)

"fast and resource-efficient" is only meaningful against a cost model, so this
section carries measured numbers rather than adjectives.

**reference machine:** MacBook Pro 18,3 (Apple M1 Pro), 32 GB unified memory,
ollama 0.32.6, model `gemma4:e2b` unless stated. **method:** direct `/api/chat`
calls reading the backend's own counters (`prompt_eval_count`,
`prompt_eval_duration`, `eval_count`, `eval_duration`, `total_duration`); medians
of 3 runs. re-measure before trusting any of it on different hardware.

a turn costs four things, and they answer to completely different levers:

| cost    | what it is                          | grows with          | lever                  |
| :------ | :---------------------------------- | :------------------ | :--------------------- |
| load    | cold-loading weights into memory    | model size          | keep_alive; residency  |
| prefill | turning the prompt into kv cache    | prompt tokens       | prefix-cache stability |
| decode  | generating the reply token by token | reply tokens        | reasoning budget       |
| render  | drawing the reply in the terminal   | tokens x frame cost | frame coalescing       |

the ranking that matters for an interactive TUI: on short turns **decode
dominates**, and the largest term in decode is often not the answer.

#### 6.3.1 reasoning tokens are the largest controllable cost

`gemma4:e2b`, the default thinker, is a reasoning model: it returns its chain of
thought in a **separate `message.thinking` field**, not inline in
`message.content`. same prompt, thinking on vs off, medians of 3:

| prompt | thinking | decode tokens | wall clock | answer length |
| :----- | :------- | :------------ | :--------- | :------------ |
| easy   | on       | 244           | 4.87 s     | 113 chars     |
| easy   | off      | 22            | 0.77 s     | 112 chars     |
| hard   | on       | 985           | 18.18 s    | 2146 chars    |
| hard   | off      | 434           | 8.21 s     | 2084 chars    |

on the easy prompt, thinking costs **11x the decode tokens and 6.3x the wall
clock** for an answer of the same length; on the hard prompt, 2.3x and 2.2x.
answer *length* is comparable in both cases and correctness was **not** graded, so
this measures cost, not value: the hard-prompt case is exactly where the extra
tokens may be earning their keep.

**inari currently pays this cost and discards the result.** `provider.Message`
declares only `role`, `content` and `tool_calls`, so `thinking` is dropped at
unmarshal; inarid never sends the `think` request parameter, so the model's own
default (on, for this model) applies. the user waits for tokens that are then
thrown away.

this reframes the near-term "reasoning-token handling" item, which assumed the
r1-style inline `<think>` case. the measured behaviour is a **separate field**,
and it affects the **default general model**, not only the curated `deepseek-r1`
coding picks.

#### 6.3.2 prefix caching is real, and one edited byte forfeits it

inari resends the full history every turn. that is only affordable because the
backend prefix-caches. measured against a 1236-token first turn:

| turn                               | prompt_eval_count | prefill |
| :--------------------------------- | :---------------- | :------ |
| 1; cold                            | 1236              | 1.97 s  |
| 2; prefix preserved                | 1257              | 0.12 s  |
| 2; one word changed near the front | 1257              | 1.99 s  |

preserving the prefix makes the follow-up turn's prefill **16x cheaper**; breaking
it costs as much as a cold start.

**instrument warning:** `prompt_eval_count` is identical (1257) across both turn-2
rows. it counts tokens *submitted*, not tokens *computed*, so it cannot see a
cache hit; only `prompt_eval_duration` can. a "prefill tokens/sec" derived from
count over duration is therefore meaningless on a cache hit (it would report
~10,000 tok/s above), and the `turn.metrics` audit record must not present such a
rate as throughput.

#### 6.3.3 the render path fans out once per token

one backend chunk currently produces one of everything, all the way to the glass:

- `internal/ipc/stream.go` encodes one `{"token":...}` JSON frame per token onto the UDS connection
- `readNextToken` (`tui/views/chat_helpers.go`) turns each frame into one `ChatTokenMsg`
- `onToken` (`tui/views/chat_stream.go`) appends via `c.streamBuf += msg.Token`, then rebuilds and hard-wraps the entire viewport content
- bubble tea then runs a full `View()` per message

at the measured 56 tok/s that is 56 full frame renders per second, and the string
concatenation is quadratic in reply length. the terminal cannot display faster
than it refreshes, so beyond roughly 30 fps every additional render is pure cost.

#### 6.3.4 memory: the knobs multiply

`gemma4:e2b` measures **1.7 GB resident** at a 32768-token window (`ollama ps`)
against **7.2 GB on disk** (`ollama list`). the two differ by 4.2x and answer
different questions: resident decides whether it runs, on-disk decides whether the
user can face the download. §6.1's `size` column reports the resident figure.

the backend-side knobs are multiplicative, which the config surface currently
hides:

- ollama allocates `num_ctx` **per parallel slot**, so `OLLAMA_NUM_PARALLEL=4` with an 8192-token window reserves 32768 tokens of kv cache, not 8192.
- `OLLAMA_KV_CACHE_TYPE` defaults to `f16`; `q8_0` roughly halves kv memory at a perplexity cost small enough to go unnoticed in practice.
- `OLLAMA_FLASH_ATTENTION` cuts attention memory; combined with a quantised cache it roughly doubles the context that fits.

the latter two are **server-start** variables, exactly like the
`OLLAMA_MAX_LOADED_MODELS` and `OLLAMA_NUM_PARALLEL` that `inari doctor` already
surfaces, and neither is reported today. they are also not universally honoured:
unsupported architectures fall back to `f16` silently, so setting the variable is
not proof that it applied. only a measured memory figure is.

**measured absence:** `ollama serve --help` on 0.32.6 lists no
`OLLAMA_SPECULATIVE_DECODE`. whatever secondary sources claim, speculative
decoding is not a lever this backend exposes today; treat it as llama.cpp-only
until `ollama serve --help` says otherwise.

#### 6.3.5 cold start costs more than a whole warm turn

`gemma4:e2b` (1.7 GB resident), measured via `load_duration`:

| state                                   | load  | total |
| :-------------------------------------- | :---- | :---- |
| cold; model evicted, OS page cache cold | 6.33s | 6.48s |
| cold; model evicted, page cache warm    | 3.03s | 3.18s |
| warm; model resident                    | 0.32s | 0.43s |

a cold start therefore adds roughly 2.7s to 6s before the first token, against an
entire warm easy turn of 0.77s (§6.3.1). the spread between the two cold rows is
the OS page cache, not ollama, so the honest figure is a range rather than a
constant.

two consequences. first, `keep_alive` is the highest-leverage single setting for
perceived speed on an interactive TUI, and ollama's 5-minute default is short
enough that a user returning from a meeting pays full cold start; `ollama.keep_alive`
in `config.json` is the knob and inari already applies it per request. second,
this is what earns the existing `loading <model>...` status signal
(`modelNotResident` in `internal/ipc/stream.go`): a multi-second silent gap before
the first token reads as a hang, and distinguishing it from `thinking...` is the
difference between a stall and progress.


### 6.4 Efficiency invariants

rules that follow from the measurements above. each is a constraint on future
changes, not a preference.

1. **the prompt prefix is append-only.** system prompt, project context and file
   tree are resolved **once** at session creation (`buildCWDSystemPrompt`, called
   from `session.create` and `/setcwd` only) and then frozen. never regenerate them
   per turn, never reorder them, never inject anything ahead of them. inari happens
   to do this correctly today; §6.3.2 is why it must stay that way, and why this is
   written down rather than left to be rediscovered.
2. **anything that rewrites history declares its cost.** `session.compact` replaces
   old turns with a summary, so the following turn pays a full cold prefill by
   construction. that is a fair trade for a smaller window, but it is not free and
   must not fire automatically mid-conversation without saying so.
3. **tokens the user never sees are opted into, never defaulted into.** a reasoning
   budget is a per-session setting with visible state, not an inherited model
   default.
4. **render at the display's rate, not the model's.** token frames coalesce on a
   frame budget; the terminal is the consumer and it refreshes at a fixed rate.
5. **a cap is not a budget.** `num_predict` is currently pinned to the full context
   window, which bounds a runaway loop but does nothing for latency: at 56 tok/s an
   8192-token reply runs over two minutes. the n-gram tail detector stays the real
   backstop.
6. **measure, do not infer.** every performance claim here cites a measured number
   and the instrument that produced it. §6.3.2 is the cautionary case: a
   plausible-looking counter that cannot see the thing you want to know.

### 6.5 Decisions

| decision              | chosen                                                                             | why                                                                                                                                   |
| :-------------------- | :--------------------------------------------------------------------------------- | :------------------------------------------------------------------------------------------------------------------------------------ |
| reasoning tokens      | decode them; capture into a separate field; render behind a toggle, off by default | measured 2.2x to 6.3x wall clock; dropping them silently is the current bug, but suppressing them outright would regress hard prompts |
| prompt prefix         | frozen at session creation; append-only thereafter                                 | one changed byte near the front costs a full cold prefill (16x)                                                                       |
| history transport     | keep resending full history; do not build an incremental protocol                  | the backend prefix-cache already makes it cheap; a delta protocol would add state to both sides for no measured gain                  |
| token rendering       | coalesce frames on a display-rate budget                                           | the terminal cannot show more than its refresh rate; extra renders are pure cost                                                      |
| speculative decoding  | not pursued                                                                        | absent from ollama 0.32.6's own env-var list; not a lever this backend exposes                                                        |
| kv-cache quantisation | surfaced as advice in doctor; never set silently                                   | it is a server-start global affecting every model, and unsupported architectures fall back without saying so                          |


---

## 7. MCP Connectors

Connectors are spawned as child processes via stdio pipes and speak JSON-RPC 2.0 (the MCP protocol). Any MCP-compliant server works - connectors are independent of inarid.

**library: `github.com/mark3labs/mcp-go`**

inarid uses `mcp-go` as the MCP client library rather than hand-rolling the protocol. it handles stdio transport, capability negotiation, and message framing. the current `internal/mcp` package is a hand-rolled precursor and will be replaced. migration risk is low - if `mcp-go` is ever unavailable, `internal/mcp` serves as a known-working fallback since the protocol is stable.

**planned connectors:**

| Connector    | Purpose                            | Server package                            |
| :----------- | :--------------------------------- | :---------------------------------------- |
| Linear       | issue tracking, project management | `@linear/mcp-server`                      |
| Slack        | messaging, channel search          | community Node.js server                  |
| Google Drive | file read/write                    | community Node.js server                  |
| Filesystem   | read/write local files             | `@modelcontextprotocol/server-filesystem` |
| Search       | web or local document search       | community server                          |
| SQL          | query local databases              | community server                          |

connector definitions loaded from `config.json` at daemon start. each entry specifies the command to spawn and its arguments - inarid is agnostic to the connector's implementation language.

---

## 8. Security Model

- All IPC local-only via UDS; no TCP exposure.
- Socket permissions restrict access to the owning user.
- All MCP tool-calls written to an append-only audit log.
- No credentials, tokens, or secrets stored by the daemon.
- MCP child processes run with inherited (restricted) environment.

### 8.1 Least-Privilege Principle

**default posture: deny.** every capability a model or connector can exercise must be explicitly granted. nothing is open by default.

this applies at every layer where the model can touch the host system:

| layer                        | default                                 | must be explicitly granted                                                                                                         |
| :--------------------------- | :-------------------------------------- | :--------------------------------------------------------------------------------------------------------------------------------- |
| filesystem context (layer 1) | read file tree (names only, no content) | - (always safe)                                                                                                                    |
| filesystem tools (layer 2)   | no tools declared                       | six typed read-only builtins per session, sandboxed to `cwd`; plus `execute_shell_command`, which is **not** path-sandboxed (§8.4) |
| MCP connectors               | none spawned                            | each connector named in `config.json`; scope defined per connector                                                                 |
| write operations             | never                                   | no write tools at any layer without explicit future design decision                                                                |

**sandbox invariants (layer 2+):**
- all paths are resolved relative to the session's `cwd` and validated before execution.
- `../` traversal and absolute paths outside `cwd` are rejected **by the typed builtins**. `execute_shell_command` performs no such validation (§8.4).
- write and delete operations are out of scope. **execute is not**: `execute_shell_command` shipped, and `go`, `make` and `git` on its default auto-approve list are each capable of writing, executing repo-controlled code, and reaching the network (§8.4).

**MCP connector hygiene:**
- each connector is spawned as a child process with a minimal, scrubbed environment - only variables it explicitly needs.
- connectors declare their own tool surface; inarid does not grant capabilities beyond what the connector exposes.
- adding a new connector to `config.json` is a conscious operator decision, not an automatic one.

**audit log as enforcement:**
- every tool-call routed through inarid is appended to the audit log before execution, not after. if logging fails, the call is rejected.
- the log is append-only and owned by the daemon process; connectors cannot write to it directly.

### 8.2 Destructive Action Prevention

the goal is to make the worst-case outcome bounded regardless of user behaviour - confirmation gates alone are insufficient because users start approving blindly under repeated prompts.

**three layers working together:**

**layer A - risk-tiered auto-approval**

every tool-call is classified at dispatch time by a static risk tier. the tier is defined per tool, not inferred from the model's intent or phrasing.

| tier        | current tools                                                                 | inarid behaviour                                                                                                       |
| :---------- | :---------------------------------------------------------------------------- | :--------------------------------------------------------------------------------------------------------------------- |
| safe        | `read_file`, `read_lines`, `list_dir`, `find_files`, `grep_file`, `stat_file` | execute immediately, no approval round-trip, log result                                                                |
| caution     | `execute_shell_command`                                                       | allowlisted command: execute immediately; otherwise send `tool_request`, block until `tool_approved`, rejection logged |
| destructive | (none yet, future write tools)                                                | always require confirmation; shown in red in inari                                                                     |
| forbidden   | process spawn, network outside ollama/mcp, shell exec                         | hard-rejected; never routable                                                                                          |

classification is conservative: if a tool's tier is ambiguous, it is assigned the higher-risk tier. adding a new tool requires an explicit tier assignment - unclassified tools are rejected.

**layer B - blast-radius limits**

hard limits enforced by inarid regardless of tier or confirmation:

- all file operations capped at 1 MB per call.
- no operations outside the session's `cwd` for the typed builtins (traversal rejected at validation, not policy). **not enforced for `execute_shell_command`** (§8.4).
- no more than 10 tool-calls per model turn (prevents runaway loops).
- no spawning processes or making network calls from within a **typed** tool handler. `execute_shell_command` spawns a process by definition, and `git`, `go` and `make` on the default allowlist can each reach the network (§8.4).

**layer C - dry-run for caution-tier actions**

before executing a caution-tier tool-call, inarid computes a dry-run result and sends a `tool.preview` message to inari:

```
[preview] write_file: path/to/file.go
--- current
+++ proposed
@@ -1,3 +1,5 @@
 ...
```

inari renders the preview and waits for `[y] approve` or `[n] reject`. only on approval does inarid execute. rejection is logged to the audit trail as a `tool.denied` entry carrying session, tool, and args, so a refused call is as visible as an executed one. if inari is detached, caution-tier calls are automatically rejected - they never execute unattended.

**non-goal:** this design does not attempt to detect malicious intent from model outputs. it bounds damage structurally so that even a model producing harmful tool-calls cannot exceed the permitted blast radius.

### 8.3 execute_shell_command - allowlisted bash execution

`execute_shell_command` lets the model invoke a fixed set of development commands inside the session's `cwd`. it is the boundary between read-only filesystem tools and write/execute capability.

**implemented constraints**

| constraint        | detail                                                                                                                                                                                                                                                                                                                                                                                                 |
| :---------------- | :----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| auto-approve list | default `go`, `make`, `git`, `ls`, `cat`, `find`, `pwd`, `whoami`, `uname`, `wc`, `date`, `echo`, `which`, `df`, `du`, `uptime`, `ps`, `awk`, `sed`, `jq` (binary base name only), overridable via `config.json` `shell.allowlist`. listed = run without a prompt; unlisted = prompt for approval, then run. **the gate reads the binary name and nothing else**; arguments are never inspected (§8.4) |
| no shell expand   | `exec.Command(binary, args...)`, never `sh -c`, so shell metacharacter injection is impossible. this does **not** make the call safe: the arguments are model-authored, and `find -exec`, `awk` `system()` and `make <target>` each reach arbitrary execution without a metacharacter                                                                                                                  |
| cwd start         | `cmd.Dir = sess.CWD` sets where the process **starts**; it does not confine what the process may reach. an absolute path argument leaves the session entirely (§8.4)                                                                                                                                                                                                                                   |
| timeout           | 30 s hard kill via `context.WithTimeout`                                                                                                                                                                                                                                                                                                                                                               |
| output cap        | stdout+stderr truncated to 64 KB before forwarding to the model                                                                                                                                                                                                                                                                                                                                        |
| exit errors       | non-zero exit is returned as text, not an error - model sees the output                                                                                                                                                                                                                                                                                                                                |

**adding a new allowed command**

add it to `shell.allowlist` in `config.json` (auto-approve, no rebuild), or edit `defaultShellAllowlist` in `internal/ipc/tools.go` to change the built-in default. the entry is the binary base name; the dispatch is generic. a command left off the list still runs, but prompts the user for approval first rather than being rejected.

**risk tier**

`execute_shell_command` is **caution-tier**, but the per-command allowlist splits it:
- a command on the auto-approve list executes immediately, like a safe-tier builtin, so keep that list to genuinely low-risk read/build/inspect commands.
- any other command sends a `tool_request` to inari and blocks until the user presses `[y]` or `[n]`.
- allowlisted commands still carry risk: `git reset --hard`, `make clean`, `go generate` can delete or regenerate files, and run without a prompt.
- read-only builtins (`read_file`, `read_lines`, `list_dir`, `find_files`, `grep_file`, `stat_file`) are safe-tier and always execute without prompting.

**rollout order for future expansion**

1. *(done)* allowlist-only `execute_shell_command` - `go`, `make`, `git`.
2. *(done)* per-call approval gating in inari (§8.2) for `execute_shell_command`; safe-tier builtins auto-execute.
3. *(done)* per-command auto-approve: allowlisted binaries run without a prompt, unlisted binaries prompt; the list is config-overridable via `shell.allowlist`.
4. arbitrary bash (destructive tier) only after write tools and blast-radius limits are in production.
4. replace with MCP process connector once the tool-call loop supports it.

**what prompts before running** (anything not on the auto-approve list)

these are no longer hard-blocked at the shell layer; each runs only after the user approves that specific call, so review them when prompted:

- `ssh`, `scp`: remote shell and file-copy reaching outside the host.
- `rm`, `mv`, `chmod`: destructive filesystem ops.
- `curl`, `wget`: network egress.
- `sh`, `bash`, `zsh`, `python`: interpreters that would run arbitrary code.

`execute_shell_command` args are NOT path-validated (only `cmd.Dir` is set to `cwd`), so an approved command can touch paths outside `cwd`; the sandbox confines the file tools, not shell arguments. structural limits that always hold: layer B caps (1 MB/op, 10 calls/turn). a future destructive tier should re-introduce hard-blocks for the interpreter and remote-shell binaries above.

### 8.4 Known gap: the shell tool is outside the sandbox

this section exists because §8.1 to §8.3 previously described a containment
guarantee the code does not implement. the gap is recorded here rather than
quietly corrected, so the difference between the intended and actual posture stays
visible until it is closed.

**what is actually enforced.** the six typed builtins (`read_file`, `list_dir`,
`grep_file`, `stat_file`, `find_files`, `read_lines`) resolve every path through
`sandboxPath` and refuse to leave `cwd`. that half of the model holds.

**what is not.** `execute_shell_command` (`internal/ipc/tools_exec.go`) reads
`command` and `args` as opaque strings, sets `cmd.Dir = cwd`, and validates no
paths at all. the auto-approve gate, `shellAutoApproved`
(`internal/ipc/tools.go`), splits out the binary base name and looks it up in the
allowlist; **arguments are never inspected**. the two facts compose badly:

- `cat` is on the default allowlist, so a model-authored call naming an absolute path outside the session runs **with no approval prompt** and returns the contents into the conversation.
- `cmd.Dir` is a starting directory, not a jail. nothing in the process is confined to it.

**the allowlist also breaks its own stated rule.** its comment says "read/build/
inspect commands only; network commands (curl, wget) are intentionally absent so
they still prompt", but three entries defeat that:

| entry  | why it is not read-only                                                             |
| :----- | :---------------------------------------------------------------------------------- |
| `git`  | `push` and `clone <url>` reach the network; `reset --hard` and `clean -fdx` destroy |
| `make` | runs whatever the **session directory's** Makefile defines; arbitrary execution     |
| `go`   | `run`, `generate` and `test` all execute repo-controlled code                       |
| `find` | `-exec` runs an arbitrary command                                                   |
| `awk`  | `system()` runs an arbitrary command                                                |

`make` and `go` matter most because they read their instructions from the session
directory. `internal/config/project.go` deliberately refuses to honour
`shell.allowlist` from a project-local config, precisely so that opening a session
inside an untrusted clone cannot widen privileges. a hostile repository does not
need to: it ships a `Makefile`.

**why this is a design question and not just a patch.** the honest options differ
in what they cost the user, so this is recorded as a decision to be taken rather
than an obvious fix:

1. **validate argument paths** against the sandbox before auto-approving, and fall through to a prompt otherwise. cheap, and closes the `cat /path/outside` case, but it cannot see through `make` or `go`.
2. **shrink the default allowlist** to commands that cannot execute or egress, moving `go`, `make`, `git`, `find` and `awk` to prompt-on-use. safest, and noticeably more annoying in exactly the workflow inari is for.
3. **treat the allowlist as a convenience, not a boundary**, and say so plainly: auto-approve means "the operator pre-consented to this class of command in this directory", with the real boundary being the operator's choice to open a session there at all.

these are not exclusive; 1 and 3 compose well. what should **not** happen is
leaving the spec claiming a sandbox while the shell tool sits outside it.



**decision taken.** options 1 and 3 above, together:

- **validate argument paths before auto-approving.** any argument that resolves outside the session `cwd`, whether absolute or via `../`, drops the call out of the auto-approve path and into the normal user prompt. this closes the `cat /absolute/outside` case, which is the cheap and silent one.
- **state plainly that the allowlist is not a boundary.** it is operator pre-consent for a class of command in a directory the operator chose to open. it cannot contain `make` or `go`, and it is not claimed to.

option 2 (shrinking the allowlist) was **not** taken: moving `go`, `make` and
`git` to prompt-on-use would put a confirmation in the middle of the build-and-test
loop inari exists to serve, and a prompt that fires constantly is one the operator
learns to dismiss without reading, which buys nothing.

the residual risk is therefore explicit and accepted: a session opened in a
directory you do not trust can run that directory's `Makefile`. the boundary is
the operator's choice of directory, not the allowlist.

---

## 9. Development & Debugging

For active development, it is often useful to run the components in the foreground across multiple terminals.

### 9.1 Independent Execution

**Terminal 1 - Ollama:**
```sh
ollama serve
```

**Terminal 2 - inarid (foreground):**
```sh
make build
./bin/inari daemon
```

**Terminal 3 - inari TUI:**
```sh
./bin/inari tui
```

### 9.2 Signal Handling
`inarid` handles `SIGINT` (Ctrl+C) and `SIGTERM` cleanly, flushing all session state to disk and closing the Unix socket before exit.

### 9.3 Git Worktrees

> **for AI agents:** development here normally happens inside a git worktree, one at a time. a worktree is a full working checkout, so `make start`/`make build`/`go run` behave identically from within it; live-test by `cd`-ing into the worktree's path (or telling the user that path so they can) rather than testing from the original checkout. keep only one worktree open per feature: close (`ExitWorktree` or `git worktree remove`) the current one before starting the next, so there is never ambiguity about which directory holds the live branch. note: `.claude/` is gitignored, so files under it (e.g. `.claude/settings.json`) are never present in a worktree; edit those directly in the main checkout, not inside a worktree.

---

### 9.4 Release Process

> **for AI agents:** always ask the user which release method to use before proceeding (default: CI). present every command as a manual step for the user to run - do NOT execute `make bump-*`, `make push-tags`, `make release`, or `make update` autonomously. these commands affect shared git history and remote state. guide one phase at a time and wait for confirmation before continuing.

two release methods are available. **CI goreleaser is the default and preferred method.**

| method           | when to use                                               |
| :--------------- | :-------------------------------------------------------- |
| CI goreleaser    | default; clean environment; audit trail in GitHub Actions |
| local goreleaser | CI is broken; no internet on runner; faster iteration     |

**prerequisites:**
- homebrew tap repo checked out locally alongside this repo: `../homebrew-tap`
- CI method: GitHub Actions workflow at `.github/workflows/release.yml` triggers on tag push
- local method: `goreleaser` installed locally; `GITHUB_TOKEN` exported in shell

### phase 1 - prepare

```sh
# 1. ensure you are on a feature branch and all changes are committed
# 2. update internal/version/version.go - set Version to the new vX.Y.Z
# 3. update CHANGELOG.md:
#    - move all unreleased items under a new heading: ## [vX.Y.Z] - YYYY-MM-DD
#    - add a fresh empty unreleased section at the top
# 4. commit the version + changelog update
git add internal/version/version.go CHANGELOG.md
git commit -m "chore: release vX.Y.Z"
# 5. open a PR and merge to main
gh pr create --title "chore: release vX.Y.Z"
# 6. tag the release on main after merge (choose one):
make bump-patch   # vX.Y.Z -> vX.Y.(Z+1)
make bump-minor   # vX.Y.Z -> vX.(Y+1).0
make bump-major   # vX.Y.Z -> v(X+1).0.0
```

### phase 2a - publish via CI goreleaser (default)

```sh
# 7. push the tag - triggers GitHub Actions goreleaser
make push-tags
```

### phase 2b - publish via local goreleaser (alternative)

```sh
# 7. publish directly using local goreleaser (requires GITHUB_TOKEN exported)
make release
```

### phase 3 - update homebrew tap (after goreleaser completes)

shared by both methods. the tap lives at `../homebrew-tap`. run manually - do not automate.

```sh
cd ../homebrew-tap
gmake update FORMULA=inari VERSION=X.Y.Z   # VERSION without the v prefix, e.g. 0.2.0
# note: gmake required - macOS ships with GNU make 3.81 which lacks .ONESHELL support
# note: no REPO arg needed; the tap defaults REPO to FORMULA and since the repo
# rename both are `inari`
```

this target:
- fetches `inari_X.Y.Z_checksums.txt` from the GitHub release
- patches `Formula/inari.rb` - version string, download urls, and all sha256 values
- commits with `feat: update inari formula to vX.Y.Z`

verify the release:
```sh
brew upgrade mirageglobe/tap/inari
inari version
```

### troubleshooting

**release fails with `422 Validation Failed - tag_name already_exists`**

goreleaser cannot overwrite an existing release. delete the partial release and retrigger:

```sh
make release-reset   # deletes any existing GitHub release for the current tag
make push-tags       # CI method: retriggers goreleaser via tag push
# make release       # local method: run goreleaser directly instead
```

**dry run (no publish)**
```sh
make release-dry   # builds binaries and archives locally, no publish
```

---

## 10. Complexity Score

> only update this table using a large/strong model after significant architectural changes.

| dimension            | score | notes                                                                                            |
| :------------------- | :---- | :----------------------------------------------------------------------------------------------- |
| overall              | 3 / 5 | moderate; single binary with streaming IPC, tool-call loop, approval gating, and multi-view TUI  |
| `internal/ipc`       | 4 / 5 | custom JSON-RPC over UDS, per-stream connections, tool approval protocol, concurrent goroutines  |
| `tui` (inari)        | 3 / 5 | multi-view Bubble Tea; message routing, offline resilience, tool approval prompt, slash commands |
| `internal/session`   | 2 / 5 | session lifecycle and atomic disk persistence; well-bounded                                      |
| `internal/ollama`    | 2 / 5 | HTTP streaming client; straightforward                                                           |
| `internal/provider`  | 2 / 5 | backend-agnostic types and the `Provider` interface; the builtin tools live in `internal/ipc`    |
| `internal/config`    | 1 / 5 | load/save with auto-create; minimal                                                              |
| `internal/mcp`       | 1 / 5 | stub only - `Call()` is a TODO; will rise to 3+ when JSON-RPC dispatch is implemented            |
| `internal/scheduler` | 1 / 5 | semaphore wrapper; minimal                                                                       |
| `internal/audit`     | 1 / 5 | append-only log; minimal                                                                         |

---

## 11. Open Questions

- Audit log format: resolved - structured JSON lines (`json.NewEncoder` over an append-only file, `internal/audit/audit.go`).
- Auth: is owner-only UDS sufficient, or add a local token?
- Session persistence: resolved - one JSON file per session, atomic write+rename,
  stored in `~/.local/share/inari/sessions`. SQLite is the natural next step if
  querying or concurrent access become requirements.
---

## 12. Decisions

architectural choices and the reason behind each. performance decisions have their
own table in §6.5; this one covers everything else. an entry here is settled: to
reverse one, replace the row rather than arguing against it in a new section.

| decision                   | chosen                                                                                | why                                                                                                                                                                                                               |
| :------------------------- | :------------------------------------------------------------------------------------ | :---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| binary layout              | one binary; first argument selects the mode (§4.2.1)                                  | one artifact to install, and client and daemon are always the same build, so the private protocol never negotiates versions                                                                                       |
| transport                  | JSON-RPC 2.0 over a Unix socket at 0600 (§4.4)                                        | local-only by construction; no TCP surface to defend                                                                                                                                                              |
| streaming                  | a dedicated UDS connection per `session.stream` call (§4.4)                           | several chat views stream at once without head-of-line blocking on the shared control connection                                                                                                                  |
| history ownership          | the daemon owns history; clients are stateless (§4.3)                                 | restarting the TUI or the daemon loses nothing, and a client never has to be trusted with the record                                                                                                              |
| session persistence        | one JSON file per session, written to `.tmp` then renamed (§4.3.1)                    | atomic by rename, readable by hand, no database dependency; sqlite stays available if querying is ever needed                                                                                                     |
| tier model                 | `thinker` and `runner` only; `sensor` and `worker` merged (§2.2)                      | neither of the split tiers had a consumer, and splitting them again is easy once routing logic needs it                                                                                                           |
| backend abstraction        | `Provider` interface extracted from the working ollama client (§2.1)                  | pulled from real code rather than invented upfront; still untested against a second backend                                                                                                                       |
| offline behaviour          | block sending and say so; never queue (§5.2.1)                                        | a message silently delivered minutes later, possibly to a cold model, surprises more than a clear refusal                                                                                                         |
| project config trust       | project files may set only prompt and excludes, never infra (§4.7)                    | opening a session in an untrusted clone must not widen the shell allowlist or redirect the backend                                                                                                                |
| shell allowlist status     | **open question**; today it is a convenience, not a boundary (§8.4)                   | the gate reads the binary name only, and `make`/`go` execute code the session directory controls                                                                                                                  |
| product scope              | inari is a terminal coding assistant; the herd/orchestration direction is closed (§1) | the tool surface built (typed file reads, grep, shell, cwd context, AGENTS.md) is a coding toolkit; nothing in it served general chat specifically, and holding both meant neither could break a tie              |
| the herd                   | removed from goals, terminology and README; not deferred                              | `runner` had no consumer beyond a doctor warning that said so, and `internal/scheduler` had zero call sites. a goal every planned item ignores is decoration; it returns as a goal only when something dispatches |
| "punch above their weight" | dropped as a goal; kept only as ethos                                                 | unfalsifiable as written: no threshold, no benchmark. the concrete goals (fast, secure, inspectable, in-project) carry the spec and can each be failed                                                            |
| shell allowlist            | validate argument paths; declare the list operator pre-consent, not a boundary (§8.4) | closes the silent out-of-sandbox read without putting a prompt inside the build-and-test loop; a prompt that fires constantly gets dismissed unread                                                               |

---
