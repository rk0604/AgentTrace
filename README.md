# AgentTrace

AgentTrace is set up for parallel AI-assisted development with a stable `main`
branch and separate agent workstreams.

## Branch model

- `main`: production-ready baseline. Merge here only after review and tests.
- `agent/codex`: Codex working branch.
- `agent/claude`: Claude Code working branch.
- `feature/<name>`: optional short-lived branches for focused work.

## Recommended workflow

1. Keep `main` clean and deployable.
2. Do Codex work in the Codex worktree/branch.
3. Do Claude Code work in the Claude worktree/branch.
4. Merge finished work back through pull requests or reviewed local merges.
5. Rebase or merge `main` into each agent branch regularly to reduce drift.

## Run the toy CLI

From the Codex worktree:

```powershell
cd C:\Users\Risha\Desktop\AgentTrace-codex
go run ./cmd/agenttrace
```

The default command runs the toy pipeline with the injected Reference failure.
It prints the dependency ordered steps, each step dependency, each check status,
and the root cause attribution.

To run the same toy pipeline without the injected failure:

```powershell
go run ./cmd/agenttrace --healthy
```

To run all tests:

```powershell
go test ./...
```
