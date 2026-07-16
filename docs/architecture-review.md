# architecture review (2026-07-16)

status: findings only. no behaviour changes land with this doc. the roadmap item
(`SPEC.md` near-term, `[inari]` `[hard]`) asks for a written findings pass first,
then incremental, separately-reviewable PRs. this is that pass; the ranked backlog
below is the PR queue.

## scope & method

five parallel read-only reviews across the areas the roadmap named:

- hot paths / allocation / concurrency (`internal/ipc/stream.go`, `loopguard.go`, tui streaming render, `internal/session`)
- rpc / daemon-server surface (`internal/ipc/*`, client/daemon symmetry)
- provider abstraction seam + config layering (`internal/provider`, `internal/ollama`, `internal/config`)
- package boundaries, file sizes, ownership hygiene
- tui structure (`tui/`, `tui/views/`)

**measure caveat (read this before acting on any perf item):** this was a static
code read, not a profiled run. no load test, no pprof, no flame graph. every
performance claim below is reasoned from code structure and labelled
`code-observed`; where a claim's payoff depends on real workload it is tagged
**needs-profile**. do not invest in a `[medium]`+ perf refactor before confirming
the bottleneck with a long-session profile. the correctness items do not need a
profile; they are strictly-cheaper or strictly-safer with identical behaviour.

tags: `[component]` per SPEC convention (`inarid` daemon, `inarit` tui, `inari` both);
`[easy|medium|hard]` effort; severity is `bug` (correctness) / `perf` / `structure`.

## 1. verified correctness issues (do first; small, high-confidence)

each was verified by reading the exact code, not just the reviewer's report.

| id | issue | file:line | sev | effort | verified |
| :--- | :--- | :--- | :--- | :--- | :--- |
| C1 | `stream.go` error path mutates `sess.Messages` directly, bypassing the `sess.mu` lock every other mutator holds; data race vs `ChatHistory`/`toInfo` readers | `internal/ipc/stream.go:177` | bug | easy | confirmed; rare (stream-error path only). fix: locked `RemoveLast()` on `Session` mirroring `AppendMessage` |
| C2 | `inari doctor` health-checks the legacy `OllamaBaseURL`, bypassing `cfg.ActiveEndpoint()`; reports green against the wrong backend once `endpoints`/`provider` are configured | `cmd/inari/doctor.go:40` | bug | easy | confirmed; latent until endpoints used. fix: use `cfg.ActiveEndpoint()` |
| C3 | `handleSessionAssign` calls `s.provider.ListModels()` with no `providerErr` guard, unlike every other provider-touching handler | `internal/ipc/dispatch_session.go:116` | consistency | easy | confirmed present, but **downgraded**: `daemon.go:46,91` always builds a non-nil provider, so this is not a live panic in the shipped daemon; add the guard for defensive consistency + test-path safety |
| C4 | `provider.Provider` doc claims "the concrete implementation is selected at startup via config.json 'provider' field" - false; the field selects an endpoint URL, the impl is hardcoded to Ollama | `internal/provider/provider.go:89` | doc-bug | easy | confirmed (see S6); fix the comment or the code, not both |

## 2. performance (ranked by expected impact; confirm needs-profile items first)

| id | issue | file:line | effort | confidence |
| :--- | :--- | :--- | :--- | :--- |
| P1 | `hasRepeatedTail` does `[]byte(s)` on the **whole** accumulated reply every token, then slices to the last 128 bytes; O(L) copy/token becomes O(L^2)/turn, pure waste | `internal/ipc/loopguard.go:22` | easy | **confirmed**, strictly cheaper; slice the string before converting. no profile needed |
| P2 | every streamed token re-`strings.Join`s the full scrollback and `ansi.Hardwrap`s the whole string, then re-splits; O(history x tokens)/turn though only `streamBuf` changed | `tui/views/chat_stream.go:43` to `chat_render.go:16-53` | medium | code-observed; **needs-profile** to confirm the session-length threshold. fix: cache wrapped history, wrap only `streamBuf` |
| P3 | `/api/show` (`ModelContextLength`) is a blocking HTTP round-trip **every turn** to derive a static `num_ctx`; adds to first-token latency | `internal/ipc/stream.go:95` to `internal/ollama/client.go:218` | easy | confirmed redundant call; memoize per model (invalidate on model change) |
| P4 | `streamBuf += token` reallocates the full buffer each token; O(L^2)/turn (lower weight than P2, same call site) | `tui/views/chat_stream.go:37` | medium | code-observed; do alongside P2 or accept. value-receiver model fights a builder |
| P5 | `modelNotResident` fires `/api/ps`, a blocking round-trip every turn for a loading-vs-thinking label | `internal/ipc/stream.go:83` | medium | needs-profile; lower priority, only worth it if P3 is done (issue concurrently or infer from time-to-first-chunk) |

reviewed and explicitly **not** a bottleneck (recorded so it is not re-flagged):
`session.ChatHistory()`'s per-turn copy is **shallow** (message headers, not bodies)
and per-round not per-token; the roadmap's own suspicion here is unfounded, leave it
(`internal/session/session.go:196`).

## 3. structure & maintainability (ranked by leverage)

### rpc / server surface (`inarid`)

- **S1 `[medium]` `NewServer` has 10 positional params**, including two adjacent same-typed strings (`defaultModel`, `globalSystemPrompt`) that transpose silently at the call site (`server.go:55`, called `daemon.go:91`). refactor to a `ServerConfig` struct (keyed literals, impossible transposition, non-breaking future knobs). this is the refactor the roadmap explicitly names.
- **S2 `[medium]` client discards JSON-RPC error codes**: every client method collapses `resp.Error` to a bare message string (`client_session.go`, `client_model.go`, ~18 sites), so callers cannot distinguish `session not found` (-32602) from `provider not configured` (-32603). this makes any server-side error-code cleanup cosmetic until the client preserves codes; do S2 before S3.
- **S3 `[easy]` bad-params error code is split -32600 vs -32602** for the identical "unmarshal failed" event across handlers (see `dispatch_session.go` create/delete/setrole/chat vs rename/tag/setcwd/interrupt/shell). pick one (JSON-RPC says -32602 for invalid params) and make it uniform.
- **S4 `[medium]` decode + "session not found" ritual repeated ~15x** across handlers; a generic `decodeParams[T]` + `getSession(req,id)` collapses each handler 6-8 lines and removes the soil S3 grew in.
- **S5 `[easy]` three client methods still inline the `SessionInfo` decode** (`CreateSession:41`, `SetCwd:186`, `Rename:208`) though a shared `sessionInfoCall`/`decodeSessionInfo` pair already exists two functions away (`client_session.go:243`). also `providerErr` prologue is copy-pasted ~10x (ties to C3).

### provider seam + config (`inarid`)

- **S6 `[hard]` no provider factory + no endpoint discriminator**: `daemon.go:46` unconditionally builds an Ollama client; `config.Endpoint` (`config.go:25`) has no `type`/`kind` field, so config cannot even express "this endpoint speaks OpenAI". this is the real blocker to any non-Ollama-wire backend (the roadmap's "second-backend readiness"). the types are ready; the wiring is not.
- **S7 `[medium]` Ollama option keys written in daemon core**: `stream.go:88,100-102` puts `repeat_penalty`/`num_ctx`/`num_predict` into the neutral `ChatRequest.Options` bag; an OpenAI-style backend wants `temperature`/`max_tokens`. the provider should translate neutral fields, not have the core populate backend-specific keys. entangled with S6 and the `num_ctx` sizing dance (`stream.go:93-103`).
- **S8 `[medium]` `ollama.*` RPC namespace leaks the backend**: generic verbs (load/unload/models/show/running) are wire-named `ollama.*` (`dispatch.go:75-88`, `client_model.go`). rename to `model.*`/`backend.*`; touches the wire contract so do it before more clients exist.
- **S9 `[easy]` config cleanups**: `defaultNewAgentModel` const (`dispatch.go:9`) duplicates `config.Models.Thinker` so the config default is silently ignored for new sessions (the server already carries `defaultModel`); dual URL source of truth `OllamaBaseURL` vs `Endpoints[].BaseURL` with precedence living only in `ActiveEndpoint` prose (`config.go:130`); idle-default 0-to-30min logic split between `daemon.go:80` and a config comment; `defaultNumCtxCap = 8192` buried as a code const (`stream.go:267`); project overlay read twice per create (`context.go:75` + `dispatch_session.go:56`).

### package boundaries (`inari`)

- **S10 `[medium]` `ipc` holds its deps as concrete pointers** (`store`/`sched`/`mcpHost`/`auditor`, `server.go:24-30`) with zero interface seams, against the project's consumer-defined-interface convention; this is why the RPC layer is hard to unit-test in isolation. the sole existing interface (`provider.Provider`) is defined in the producer package, inverting where the convention wants it. highest-value structural fix; pairs with S1.
- **S11 `[easy]` oversized files** (convention: source under ~150 lines; 23 files exceed it). worst structural offender is `handleStream`, a single ~247-line function (`stream.go:19-265`) doing context-assembly + streaming + tool-dispatch in one body (`[hard]` to split well). the rest are large-but-cohesive mechanical splits: `client_session.go` (309), `dispatch_session.go` (308), `ollama/client.go` (245, `[medium]`), `chat_msgs.go` (215). note: `sched`/`mcpHost` are stored on `Server` but appear unused by any `ipc` handler; confirm and drop if so.
- positive, recorded: **doc.go ownership hygiene is fully compliant** (every package has one with explicit "does NOT own" scoping) and there are **no import cycles** (clean layering, `provider` at the bottom). do not disturb.

### tui (`inarit`)

the dispatch chain (`updateBroadcast` -> `updateSystem` -> `updateNav` -> `updateModal`
-> `updateKeys` -> `updateActiveView`) is a good design; findings are accreted
duplication around it, not its shape. **three of these are consequences of the modal
refactor just landed in PR #74 and are called out honestly:**

- **S12 `[easy]` modal-box construction copy-pasted across 5 renderers** (`logs.go:97`, `describe_render.go:84`, `selector_render.go:50`, `agents_view.go:72`, `chat_view.go:50`): the same rounded-border + padding + join + center-`Place` block. PR #74 added two of these copies. extract `renderModalBox(lines, w, h)` next to `modalInnerWidth`; highest leverage-to-effort in the tui. **(introduced/worsened by #74)**
- **S13 `[easy]` dead full-screen `View()` paths** on `Logs` (`logs.go:105`) and `Describe` (`describe_render.go:92`): after #74 both render only as overlays; ~50 unreachable lines plus stale `[esc] back` help text (`help.go:35-38`). delete or stub (the selector already stubs `View()`; pick one convention). **(introduced by #74)**
- **S14 `[medium]` six `showX` bools encode one mutually-exclusive overlay** (`model.go:44-49`); exclusivity is enforced only by matching check-order across `model_view.go:17` (render) and `model_input.go:44` (capture), plus a hand-maintained `overlayOpen` OR-chain. replace with a single `activeOverlay` enum: exclusivity becomes structural, render/capture become one `switch`, and it subsumes the "leave updateModal alone" note below. PR #74 added two of the six bools. **(worsened by #74)**
- **S15 `[medium]` `updateBroadcast` fan-out duplicated**: two near-identical 22-line blocks (WindowSize, ThemeChanged) manually threading 5 views, plus a third divergent list in the offline fan-out (`model_router.go:99` covers only 3). extract `broadcast(msg)`; the value-typed fields still force the type-asserted write-backs inside it, so this dedupes the message blocks, not the assignments.
- reviewed and **leave as-is**: the four per-modal blocks in `updateModal` look extractable but each has a real quirk (describe guards on `!IsEditing()`, agents closes via message not key); a table-driven registry would trade four readable blocks for one opaque one. if S14 lands they become `case` arms anyway.

## 4. suggested PR sequence

small, separately-reviewable, ordered so each de-risks the next:

1. **correctness batch** (C1-C4): one small PR; locked `RemoveLast`, doctor endpoint, assign guard, doc fix. tests per fix.
2. **P1 + P3**: trivial perf wins (loopguard slice-before-convert; memoize `/api/show`); no profile needed.
3. **S12 + S13**: tui modal-box helper + delete dead `View()` paths; cleans up after #74.
4. **S1 + S10**: `ServerConfig` struct then `ipc` consumer-side interface seams (do together; both touch the constructor).
5. **S5 + S3 + S2 + S4**: client/handler dedup and error-code consistency, in that dependency order (S2 before S3 or S3 is cosmetic).
6. **S14 + S15**: tui `activeOverlay` enum then broadcast helper (after the state settles).
7. **profile P2/P4/P5** before touching them; then the streaming-render cache if the profile justifies it.
8. **S6 (+S7/S8/S9)**: the second-provider factory; largest scope, best done last as its own milestone once the surface above is clean.

items 1-3 are safe quick wins; 4-6 are the core structural work; 7-8 are milestones.
