# Agent Development Rules

This repository is designed for multiple AI coding agents working in parallel.

## Shared rules

- Treat `main` as production-ready.
- Do not commit directly to `main` except for explicit release/admin work.
- Keep changes focused and scoped to the requested task.
- Run the relevant tests before merging.
- Do not overwrite another agent's branch or worktree.
- Prefer small commits with clear messages.

## Branch ownership

- Codex owns `agent/codex` by default.
- Claude Code owns `agent/claude` by default.
- Cross-agent changes should be merged through `main` or a reviewed integration
  branch, not by editing the other agent's branch directly.

## Handoff notes

When finishing a task, include:

- What changed.
- How it was tested.
- Any remaining risks or follow-up work.

