# AgentTrace

AgentTrace is a Go CLI for finding the earliest source of failure in an AI agent
pipeline. It reads a completed pipeline trace, orders the steps by dependency,
evaluates each step with developer supplied correctness rules, and reports the
first step that produced an incorrect output.

This matters when a bad value travels through several agents before it becomes
visible. A downstream agent may behave correctly given its inputs even when the
final answer is wrong. AgentTrace follows data lineage so the original upstream
failure receives the attribution.

## What AgentTrace does

1. Reads a pipeline trace from JSON.
2. Validates step IDs and dependencies.
3. Topologically sorts the dependency graph.
4. Evaluates each step with a CEL correctness expression.
5. Stops at the first failing step.
6. Prints a report and an ASCII dependency graph with the cause marked.

AgentTrace is intentionally focused. It does not orchestrate agents, call
models, store run history, or provide a dashboard. LangGraph builds workflows
and Langfuse observes LLM applications. AgentTrace analyzes an already completed
trace to answer one question: which step first introduced the bad result?

## Requirements

- Go 1.26.3 or newer
- Git

Check the installed Go version:

```powershell
go version
```

## Setup

Clone the repository and download its Go dependencies:

```powershell
git clone https://github.com/rk0604/AgentTrace.git
cd AgentTrace
go mod download
```

## Quick start

Run the small four step demo with an injected Reference failure:

```powershell
go run ./cmd/agenttrace demo toy
```

Run the same pipeline in healthy mode:

```powershell
go run ./cmd/agenttrace demo toy --healthy
```

Run the larger incident investigation demo. The available failure modes are
`none`, `metrics`, and `deployment`:

```powershell
go run ./cmd/agenttrace demo incident --failure metrics
```

Each demo prints the dependency ordered steps, the attribution result, and a
flow chart that marks the root cause node and its outgoing cause edge.

## Analyze a JSON trace

The `run` command requires a trace file and a checker configuration:

```powershell
go run ./cmd/agenttrace run `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json
```

Return only the machine readable JSON result:

```powershell
go run ./cmd/agenttrace run `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json `
  --json
```

Write the JSON result to a file while keeping the human readable report:

```powershell
go run ./cmd/agenttrace run `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json `
  --output ./result.json
```

The `--json` and `--output` options cannot be used together.

## Validate inputs

Validate the trace structure, dependency graph, CEL expressions, and checker
coverage without running attribution:

```powershell
go run ./cmd/agenttrace validate `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json
```

A valid configuration prints `Validation passed` with its run, step, and checker
counts.

## Trace format

A trace contains a run ID and a list of steps. `depends_on` describes data
lineage, not the order in which agents happened to run. Domain specific values
belong inside `input` and `output` JSON objects.

```json
{
  "run_id": "example-run",
  "steps": [
    {
      "run_id": "example-run",
      "step_id": "extractor",
      "agent_name": "Extractor",
      "depends_on": [],
      "input": {"document": "Example source text"},
      "output": {"value": "example"},
      "model_used": "model-name",
      "confidence": 0.95,
      "timestamp": "2026-07-21T12:00:00Z",
      "status": "ok"
    }
  ]
}
```

`confidence` is optional. Step IDs must be unique, every dependency must exist,
and the graph must not contain a cycle.

## Checker format

Correctness rules are stored separately from the trace as versioned CEL
expressions keyed by step ID:

```json
{
  "version": 1,
  "steps": {
    "extractor": {
      "expression": "output.value == \"example\"",
      "failure_reason": "Extractor returned the wrong value"
    }
  }
}
```

Each expression must return a Boolean value. It can inspect:

- `input`, the step input JSON object
- `output`, the step output JSON object
- `step`, generic metadata such as `step_id`, `agent_name`, and `status`

The checker should return `true` when the step behaved correctly given its
actual inputs. This distinction prevents a downstream step from being blamed
for faithfully processing incorrect upstream data. Every trace step must have a
matching checker.

See [examples/toy-checkers.json](examples/toy-checkers.json) and
[examples/incident-checkers.json](examples/incident-checkers.json) for complete
configurations.

## Testing

Run the complete test suite:

```powershell
go test ./...
```

Run static analysis:

```powershell
go vet ./...
```

Run tests for one package:

```powershell
go test ./attrib
go test ./checkerconfig
go test ./toypipeline
go test ./incidentdemo
```

Run one named test with verbose output:

```powershell
go test ./toypipeline -run TestReferenceFailureIsRootCause -v
```

## Project structure

```text
attrib              Generic trace schema, DAG sorting, validation, and attribution
checkerconfig       JSON checker configuration and CEL evaluation
cmd/agenttrace      Main command line application
cmd/incidentfixtures Incident fixture generator
examples            Example traces and checker configurations
incidentdemo        Complex incident investigation pipeline
toypipeline         Small deterministic demonstration pipeline
```

The generic `attrib` package has no finance or incident specific fields. Domain
meaning remains inside JSON payloads and external checker rules.

## Regenerate incident fixtures

After changing the incident demo agents, regenerate its example traces:

```powershell
go run ./cmd/incidentfixtures --output ./examples
```

## Development workflow

`main` is the stable branch. Codex work is developed on `agent/codex`, and Claude
Code work is developed on `agent/claude`. Keep each agent in its own worktree,
run the relevant tests, and merge reviewed changes into `main`.
