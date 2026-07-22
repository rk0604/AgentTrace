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

## Run a JSON trace

The example files contain complete traces with real JSON objects inside each
step's `input` and `output` fields. The CLI decodes those objects into
`json.RawMessage`, then the toy pipeline checkers interpret their domain data.

Print a human readable report and graph from a JSON trace:

```powershell
go run ./cmd/agenttrace --input ./examples/trace-reference-failure.json
```

Print only the machine readable JSON attribution result:

```powershell
go run ./cmd/agenttrace `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json `
  --json
```

Write the JSON result to a file while still printing the human readable report:

```powershell
go run ./cmd/agenttrace --input ./examples/trace-reference-failure.json --output ./result.json
```

## Configure correctness checks

A checker configuration maps each trace step ID to a CEL expression. The
expression receives three generic variables:

- `input`: the step's JSON input.
- `output`: the step's JSON output.
- `step`: generic metadata such as `step_id`, `agent_name`, and `status`.

Each expression must return `true` when the step behaved correctly given its
actual input, or `false` when that step is the first source of bad data. The
configured failure reason is included in the attribution result.

Run the complete generic JSON flow:

```powershell
go run ./cmd/agenttrace `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json `
  --output ./result.json
```

When `--checkers` is omitted, the CLI keeps using `toypipeline.Checkers()` for
the original built in demonstration. The `attrib` package remains unaware of
CEL and domain specific payload fields.
