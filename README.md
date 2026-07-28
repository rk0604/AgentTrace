# AgentTrace

AgentTrace is a Go tool for finding where incorrect data first entered an AI
agent pipeline.

It reads a completed JSON trace, orders steps by dependency, evaluates each step
with developer supplied CEL rules, and stops at the first incorrect output. It
then reports the root cause, failed expression, actual evidence, affected
downstream steps, and propagation edges.

This prevents a final agent from being blamed when it correctly processed bad
data produced earlier in the graph.

## Scope

AgentTrace performs post run attribution.

It does:

- Record generic pipeline traces from Go and Python
- Validate trace schemas and dependency graphs
- Evaluate configurable correctness rules
- Find the first divergence in dependency order
- Return human readable and machine readable evidence
- Render the pipeline as an ASCII DAG

It does not:

- Orchestrate agents
- Call model providers
- Store run history
- Provide accounts, persistence, or a dashboard

LangGraph builds and runs workflows. Langfuse observes LLM applications over
time. AgentTrace analyzes one completed run to identify the step that introduced
the bad result.

## Requirements

- Go 1.26.3 or newer
- Python 3.10 or newer for the optional Python recorder
- Git

## Setup

```powershell
git clone https://github.com/rk0604/AgentTrace.git
cd AgentTrace
go mod download
```

Run all Go tests:

```powershell
go test ./...
```

## Quick start

Run the recorder based document pipeline with an extraction failure:

```powershell
go run ./cmd/agenttrace demo document --failure extraction
```

This six step demo uses version 2 checks and external expected data. The
Information Extractor misreads `$42,000` as `$420,000`. Downstream risk,
eligibility, and report steps remain correct for the data they received, so
AgentTrace attributes the failure to `information_extractor`.

Run the healthy version:

```powershell
go run ./cmd/agenttrace demo document --failure none
```

Write the recorded trace while running the demo:

```powershell
go run ./cmd/agenttrace demo document `
  --failure extraction `
  --trace-output ./document-trace.json
```

Analyze that trace independently:

```powershell
go run ./cmd/agenttrace run `
  --input ./document-trace.json `
  --checkers ./examples/document-checkers-v2.json `
  --context ./examples/document-context.json
```

## Commands

Display command help:

```powershell
go run ./cmd/agenttrace help
```

### Run

Analyze a completed trace:

```powershell
go run ./cmd/agenttrace run `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json
```

Add `--context` when expressions use external expected data:

```powershell
go run ./cmd/agenttrace run `
  --input ./document-trace.json `
  --checkers ./examples/document-checkers-v2.json `
  --context ./examples/document-context.json
```

Return only JSON:

```powershell
go run ./cmd/agenttrace run `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json `
  --json
```

Write the JSON result while keeping the human report:

```powershell
go run ./cmd/agenttrace run `
  --input ./examples/trace-reference-failure.json `
  --checkers ./examples/toy-checkers.json `
  --output ./result.json
```

`--json` and `--output` cannot be combined.

### Validate

Compile expressions and validate the trace, graph, context, and checker
coverage without evaluating the steps:

```powershell
go run ./cmd/agenttrace validate `
  --input ./document-trace.json `
  --checkers ./examples/document-checkers-v2.json `
  --context ./examples/document-context.json
```

### Demo

Small four step pipeline:

```powershell
go run ./cmd/agenttrace demo toy
go run ./cmd/agenttrace demo toy --healthy
```

Thirteen step incident investigation:

```powershell
go run ./cmd/agenttrace demo incident --failure metrics
go run ./cmd/agenttrace demo incident --failure deployment
go run ./cmd/agenttrace demo incident --failure none
```

Recorder based document review:

```powershell
go run ./cmd/agenttrace demo document --failure extraction
go run ./cmd/agenttrace demo document --failure reference
go run ./cmd/agenttrace demo document --failure none
```

All demos are deterministic and require no API key.

## Record a Go pipeline

The `recorder` package captures arbitrary JSON compatible inputs and outputs.
Dependencies can be recorded from parallel pipeline branches.

```go
run, err := recorder.StartRun("claim-run-42", recorder.Options{})
if err != nil {
    return err
}

err = run.StartStep(recorder.StepStart{
    StepID:    "loader",
    AgentName: "Document Loader",
    Input:     map[string]any{"path": "claim.txt"},
    ModelUsed: "provider-model",
})
if err != nil {
    return err
}

output := map[string]any{"text": "loaded claim"}
if err := run.FinishStep("loader", output, nil); err != nil {
    return err
}

if err := run.WriteTrace(writer); err != nil {
    return err
}
```

The recorder is concurrency safe. It rejects duplicate IDs, invalid confidence
values, unfinished steps, unknown dependencies, and cycles.

## Record a Python pipeline

Install the local Python helper:

```powershell
python -m pip install -e ./sdk/python
```

Generate the checked in Python example:

```powershell
python ./examples/python_recorded_pipeline.py ./python-trace.json
```

Validate it with the Go CLI:

```powershell
go run ./cmd/agenttrace validate `
  --input ./python-trace.json `
  --checkers ./examples/python-checkers.json
```

The helper supports manual lifecycle methods, synchronous `run_step`, and
asynchronous `run_step_async`. See
[sdk/python/README.md](sdk/python/README.md) for usage.

## Trace contract

The current external trace schema is version `1`:

```json
{
  "version": 1,
  "run_id": "example-run",
  "steps": [
    {
      "run_id": "example-run",
      "step_id": "source",
      "agent_name": "Source Agent",
      "depends_on": [],
      "input": {"document": "source text"},
      "output": {"value": "result"},
      "model_used": "provider-model",
      "confidence": 0.95,
      "timestamp": "2026-07-28T12:00:00Z",
      "status": "ok"
    }
  ]
}
```

`depends_on` represents data lineage, not call order. Domain specific fields
remain inside `input` and `output`; the generic schema has no knowledge of
claims, finance, incidents, or any other domain.

See [docs/trace-format.md](docs/trace-format.md) for every field and constraint.

## Checker contract

Version 2 supports ordered assertions:

```json
{
  "version": 2,
  "steps": {
    "source": {
      "checks": [
        {
          "expression": "hasField(output, \"value\")",
          "failure_reason": "Source omitted value"
        },
        {
          "expression": "output.value == expected.value",
          "failure_reason": "Source returned the wrong value"
        }
      ]
    }
  }
}
```

Expressions can inspect:

- `input`
- `output`
- `step`
- `run`
- `context`
- `expected`

Generic helpers include `hasField`, `isString`, `isNumber`, `isNonEmpty`,
`withinRange`, `equalsExpected`, and `sameField`.

Version 1 checker files remain supported. See
[docs/checkers.md](docs/checkers.md) for migration, helpers, and context details.

## Attribution output

A failed JSON result includes:

- Root cause step and agent
- Human failure reason
- Failed CEL expression
- Actual step input and output
- Expected context
- Checked step IDs
- Affected downstream step IDs
- Propagation edges

The human report truncates very large evidence for terminal readability. JSON
output retains the complete evidence.

## Operational behavior

External input limits:

- Trace file: 16 MiB
- Checker file: 2 MiB
- Context file: 2 MiB
- Trace steps: 10,000
- Dependencies per step: 1,000

CEL expressions have a cost limit and a 250 millisecond default timeout.
Topological sorting uses a deterministic heap and scales with the graph rather
than repeatedly scanning every ready node.

Process exit codes:

- `0`: command succeeded
- `1`: runtime, file, or evaluation failure
- `2`: invalid command usage

Return structured errors on standard error:

```powershell
$env:AGENTTRACE_LOG_FORMAT = "json"
go run ./cmd/agenttrace unknown
```

## Testing

Run all Go tests:

```powershell
go test ./...
```

Run static analysis:

```powershell
go vet ./...
```

Run Python recorder tests:

```powershell
cd ./sdk/python
python -m unittest -v
```

Run one root cause test:

```powershell
go test ./documentdemo -run TestDocumentConfigurationAttributesFailureModes -v
```

The suite includes unit tests, cross language integration tests, a 5,001 node
graph test, benchmarks, and fuzz seeds. GitHub Actions runs Go tests with race
detection, static analysis, Python tests, and both cross language demos.

## Project structure

```text
attrib               Generic schema, sorting, validation, and attribution
checkerconfig        Versioned CEL configuration and helper functions
recorder             Concurrency safe Go trace recorder
sdk/python           Dependency free Python trace recorder
documentdemo         Recorder based document review integration
incidentdemo         Thirteen step incident investigation demo
toypipeline          Small four step attribution demo
integration          Cross language integration tests
cmd/agenttrace       Main CLI
cmd/incidentfixtures Incident trace fixture generator
examples             Traces, contexts, checker files, and Python example
docs                 Trace and checker contracts
```

## Development workflow

`main` remains the stable branch. Codex work belongs on `agent/codex`, and
Claude Code work belongs on `agent/claude`. Each agent uses its own worktree,
runs the relevant verification, and merges reviewed changes through `main`.
