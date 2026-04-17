# Agent Guidelines

## Context

- Always read `README.md` at the start of a session for project context.
- Keep `README.md` and `SPEC.md` up to date as development progresses — update milestone checkboxes, architecture notes, and open questions when they are resolved.

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

## Code Comments

- Prioritise comments that explain **why**, not what — hidden constraints, non-obvious invariants, workarounds, and design intent that cannot be read from the code alone.
- Treat existing comments as first-class project information: read them before changing logic, and update them when the behaviour they describe changes.
- Package-level doc comments (`// Package ...`) are encouraged — they give models and humans a fast orientation to each component's role.
- When a TODO or FIXME comment describes a known gap, reference it explicitly rather than silently working around it.

## Commits

- Do NOT add co-author lines (e.g. `Co-Authored-By: ...`) to commit messages.
- Keep commit messages concise and focused on the "why" of the change.
