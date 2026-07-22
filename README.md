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

## CLI commands

From the Codex worktree:

```powershell
cd C:\Users\Risha\Desktop\AgentTrace-codex
```

AgentTrace has three explicit commands:

- `run` attributes a JSON trace using a JSON CEL checker configuration.
- `validate` validates both files without evaluating the checkers.
- `demo` runs a built in toy or incident scenario.

Running the CLI without a command returns an error instead of silently selecting
a demonstration.

## Run a JSON trace

The normal run mode always requires both `--input` and `--checkers`:

```powershell
go run ./cmd/agenttrace run `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json
```

The example trace contains real JSON objects inside each step's `input` and
`output` fields. The CLI decodes those objects into `json.RawMessage`, compiles
the CEL expressions, runs first divergence attribution, and prints the graph.

Print only the machine readable JSON attribution result:

```powershell
go run ./cmd/agenttrace run `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json `
  --json
```

Write the JSON result to a file while still printing the human readable report:

```powershell
go run ./cmd/agenttrace run `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json `
  --output ./result.json
```

## Validate configuration

The `validate` command decodes the trace, compiles every CEL expression, checks
the dependency graph, and confirms that every step has a checker. It does not
evaluate the checkers or produce an attribution result.

```powershell
go run ./cmd/agenttrace validate `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json
```

## Run the toy demo

Run the four step pipeline with the injected Reference failure:

```powershell
go run ./cmd/agenttrace demo toy
```

Run the same pipeline without the failure:

```powershell
go run ./cmd/agenttrace demo toy --healthy
```

To run all tests:

```powershell
go test ./...
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

The `attrib` package remains unaware of CEL and domain specific payload fields.
Only the checker configuration interprets the JSON payloads.

## Run the incident investigation demo

The incident demo models a checkout outage investigation with thirteen steps.
Log, metrics, deployment, and runbook agents branch in parallel before their
evidence is merged into a timeline, hypothesis, impact assessment, remediation
plan, and final incident summary.

Run the healthy investigation demo:

```powershell
go run ./cmd/agenttrace demo incident --failure none
```

Inject a Metrics Analyzer failure. This is the default incident mode:

```powershell
go run ./cmd/agenttrace demo incident --failure metrics
```

Inject a Deployment Analyzer failure and print only JSON:

```powershell
go run ./cmd/agenttrace demo incident `
  --failure deployment `
  --json
```

The incident demo loads `./examples/incident-checkers.json` by default. A
different configuration can be selected with `--checkers`.

The downstream agents deliberately continue from the evidence they actually
receive. Their outputs can therefore be wrong in the real world while remaining
correct transformations of bad upstream input. AgentTrace attributes the first
divergence to the analyzer that introduced the bad evidence.

Regenerate the three incident trace fixtures after changing the demo agents:

```powershell
go run ./cmd/incidentfixtures --output ./examples
```
