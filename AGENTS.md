# Agent Guidelines

## Documentation Philosophy

Adhere to a strict "Manual vs. Blueprint" separation:

- **README.md (The Manual):** Dedicated to **Users**. Focus on the project ethos, feature lists, high-level core concepts (e.g., The Herd), quick-start commands, and user-facing configuration (`config.json`). Keep it free of technical implementation details or manual debugging steps.
- **SPEC.md (The Blueprint):** Dedicated to **Developers and Agents**. This is the single source of truth for architecture diagrams, IPC protocols, internal data models, security specs, and build/debugging instructions.
- **Roadmap:** The Roadmap and Build Milestones live exclusively in `SPEC.md`. The `README.md` should only provide a link to this section.

## Context

- Always read `README.md` and `AGENTS.md` at the start of a session for project context.
- Keep `README.md` and `SPEC.md` up to date as development progresses — update milestone checkboxes, architecture notes, and open questions when they are resolved.
- When a significant design decision is made or implemented, update `SPEC.md` with the rationale and design details. Update `README.md` only if the user-facing interface or high-level concept changes.
- When a proposed change would violate architectural boundaries or established best practices (e.g. a client taking on process-manager responsibilities, mixing concerns across layers), flag it as an antipattern before implementing. Explain why it conflicts with the design, and offer a clean alternative that achieves the user's intent within the existing architecture.

**Resource tiers (2026 1-bit models):**

| Size   | Tier     | Role                   | Example model | Required |
|--------|----------|------------------------|---------------|----------|
| 100MB  | Sensors  | Routing / classification | Qwen3-Nano  | No       |
| 500MB  | Workers  | Parallel execution     | Bonsai 4B     | Yes      |
| 1GB    | Thinkers | Architect / chat       | Bonsai 8B     | Yes      |

## Project Reviews

When reviewing the scaffold or making recommendations:
- Check `.gitignore` is current for the project's stack and build outputs.
- Verify it covers: OS artefacts, editor files, build directories (`bin/`), test binaries (`*.test`), coverage files, logs, and secrets — removing patterns irrelevant to the stack.

## TUI Layout

- Target a maximum UI width of **100 characters**. Hardcoded widths for table columns, viewports, and hint strings should sum to 100 or fewer.
- Prefer `tea.WindowSizeMsg` for dynamic sizing rather than hardcoded dimensions where possible.

## Code Comments

- Prioritise comments that explain **why**, not what — hidden constraints, non-obvious invariants, workarounds, and design intent that cannot be read from the code alone.
- Treat existing comments as first-class project information: read them before changing logic, and update them when the behaviour they describe changes.
- Package-level doc comments (`// Package ...`) are encouraged — they give models and humans a fast orientation to each component's role.
- When a TODO or FIXME comment describes a known gap, reference it explicitly rather than silently working around it.

## Style: lowercase-first

- All comments and UI text strings start with a **lowercase** letter, even at the start of a sentence.
- Exception: when a Go doc comment starts with the exported identifier itself (e.g. `// RenderTopBar renders…`), the identifier stays capitalised — everything else follows lowercase.
- Exception: proper nouns that are always capitalised (Ollama, Bubble Tea, etc.) stay as-is.
- Apply this rule to new code and when editing existing files.

## Code Quality

- Always run `go vet ./...` after making code changes and before committing. It catches real bugs (wrong argument counts, misused format strings, unreachable code) — not just style issues.
- Run `go build ./...` to confirm the project compiles cleanly after every change.
- If `go vet` or `go build` fail, fix the errors before proceeding.

## Commits

- Do NOT add co-author lines (e.g. `Co-Authored-By: ...`) to commit messages.
- Keep commit messages concise and focused on the "why" of the change.
- Before creating a commit, verify the current branch is **not** `main`. If it is, prompt the user to create or switch to a feature branch first.
