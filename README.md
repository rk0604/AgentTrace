# AgentTrace

**JSON trace in. Attribution result out.**

AgentTrace is a post run root cause attribution tool for AI agent pipelines. It
reads a completed pipeline trace, walks its dependency graph in topological
order, and evaluates developer supplied correctness rules. When rules fail,
AgentTrace identifies the earliest independent steps that introduced bad data,
separates later secondary divergences, and reports possible downstream
propagation.

Each step is judged against the input it actually received. A downstream agent
that correctly transforms incorrect upstream data is not blamed for the original
mistake.

## Scope

AgentTrace provides:

* A language neutral, versioned JSON trace contract
* Concurrency safe trace recorders for Go and Python
* Configurable CEL correctness rules and external expected data
* Trace, graph, checker, and coverage validation
* Target aware attribution with multiple root and secondary divergences
* Privacy safe reports, a bounded ASCII DAG, and versioned JSON output

The AgentTrace core and CLI do not orchestrate agents, call model providers,
store run history, manage users, or provide a dashboard. The repository includes
an optional live pipeline solely as an integration example.

LangGraph builds and runs workflows. Langfuse observes LLM applications over
time. AgentTrace analyzes one completed run and asks which step first introduced
the incorrect result.

## How It Works

```text
Agent pipeline
      |
      v
trace.json + checker rules + optional expected context
      |
      v
AgentTrace validate
      |
      v
AgentTrace run
      |
      v
Root causes + divergences + downstream candidates + propagation edges
```

AgentTrace:

1. Validates the trace schema and dependency graph.
2. Compiles the checker configuration.
3. Resolves explicit targets or the graph's single output sink.
4. Orders the selected ancestry by `depends_on`, not recorded call order.
5. Evaluates every relevant step against its actual input.
6. Reports independent root causes, secondary divergences, and possible
   downstream propagation.

AgentTrace does not infer correctness on its own. The developer supplies the
checker rules and any ground truth those rules require.

## Requirements

* Go 1.26.3 or newer
* Git
* Python 3.10 or newer for the optional Python recorder and incident pipeline

The Go demos are deterministic and require no model credentials or API keys.
The real incident pipeline has an offline replay mode that requires its
`requirements.txt` dependency. Live mode also requires an OpenAI API key, an
explicitly selected model, and `requirements-live.txt`.

## Setup

```powershell
git clone https://github.com/rk0604/AgentTrace.git
cd AgentTrace
go mod download
go test ./...
```

Install the optional real incident pipeline dependency:

```powershell
python -m pip install -r ./examples/incident_agent_pipeline/requirements.txt
```

Run from source:

```powershell
go run ./cmd/agenttrace help
```

Optionally build a Windows executable:

```powershell
go build -o agenttrace.exe ./cmd/agenttrace
.\agenttrace.exe help
```

On macOS or Linux, build as `agenttrace` and run it as `./agenttrace`.

## Quick Start

Run the six step document pipeline with an injected extraction failure:

```powershell
go run ./cmd/agenttrace demo document --failure extraction
```

The Information Extractor reads `$42,000` as `$420,000`. Later steps correctly
process the data they received, so AgentTrace identifies
`information_extractor` as the root cause and marks the affected path.

Run the healthy version:

```powershell
go run ./cmd/agenttrace demo document --failure none
```

Write the completed trace:

```powershell
go run ./cmd/agenttrace demo document --failure extraction --trace-output ./document-trace.json
```

Analyze that trace independently:

```powershell
go run ./cmd/agenttrace validate --input ./document-trace.json --checkers ./examples/document-checkers-v2.json --context ./examples/document-context.json
go run ./cmd/agenttrace run --input ./document-trace.json --checkers ./examples/document-checkers-v2.json --context ./examples/document-context.json
```

## Real Agent Pipeline

The repository includes a recorded 13-step incident investigation assistant. It
analyzes checkout alerts, logs, metrics, deployments, and runbook guidance to
produce an evidence-backed diagnosis and remediation plan. Analyzer branches run
concurrently, and no step can execute production changes.

Install its standards compliant JSON Schema validator once:

```powershell
python -m pip install -r ./examples/incident_agent_pipeline/requirements.txt
```

Run the healthy pipeline without credentials:

```powershell
python -m examples.incident_agent_pipeline --mode replay --failure none --trace-output ./incident-agent-trace.json
```

Analyze its trace:

```powershell
go run ./cmd/agenttrace run --input ./incident-agent-trace.json --checkers ./examples/incident_agent_pipeline/checkers-v2.json --context ./examples/incident_agent_pipeline/context.json
```

Demonstrate upstream attribution:

```powershell
python -m examples.incident_agent_pipeline --mode replay --failure metrics --trace-output ./incident-agent-trace.json
go run ./cmd/agenttrace run --input ./incident-agent-trace.json --checkers ./examples/incident_agent_pipeline/checkers-v2.json --context ./examples/incident_agent_pipeline/context.json
```

The final summary follows the bad metric finding and reports an application
error. AgentTrace still attributes the earliest divergence on that branch to
`metrics_analyzer`.

For live model calls, installation, environment variables, architecture, and
failure modes, see
[examples/incident_agent_pipeline/README.md](examples/incident_agent_pipeline/README.md).

## Developer Integration

### 1. Choose a Trace Source

| Pipeline | Integration |
| --- | --- |
| Go | Import `github.com/rk0604/AgentTrace/recorder` |
| Python | Install the helper in `sdk/python` |
| Another language | Emit the documented JSON trace |
| Existing trace exporter | Convert its output to the AgentTrace contract |

Go and Python are convenience recorders. The attribution engine accepts traces
from any language.

### 2. Record the Pipeline

Every trace contains one run ID and a list of completed steps:

```json
{
  "version": 1,
  "run_id": "claim-run-42",
  "steps": [
    {
      "run_id": "claim-run-42",
      "step_id": "extractor",
      "agent_name": "Information Extractor",
      "depends_on": [],
      "input": {
        "document": "Claim CLM-104 requests $42,000."
      },
      "output": {
        "claim_id": "CLM-104",
        "amount": 42000
      },
      "model_used": "provider-model",
      "confidence": 0.94,
      "timestamp": "2026-07-28T12:00:00Z",
      "status": "ok"
    }
  ]
}
```

Use stable, unique step IDs. `depends_on` names the steps whose outputs produced
the current input. It represents data lineage, so independent parallel steps do
not depend on each other.

Keep domain data inside `input` and `output`. The generic schema never gains
fields such as claim amount, revenue, quarter, or incident cause.

See [docs/trace-format.md](docs/trace-format.md) for all fields and validation
rules.

### 3. Define Correctness

Create a version 2 checker file with checks for every trace step:

```json
{
  "version": 2,
  "steps": {
    "extractor": {
      "checks": [
        {
          "expression": "hasField(output, \"claim_id\") && hasField(output, \"amount\")",
          "failure_reason": "Extractor omitted a required field"
        },
        {
          "expression": "output.claim_id == expected.claim_id && output.amount == expected.amount",
          "failure_reason": "Extractor returned facts that differ from the source"
        }
      ]
    }
  }
}
```

Checks run in order. The first expression that returns `false` supplies the root
cause reason.

Expressions can inspect:

| Variable | Value |
| --- | --- |
| `input` | Actual input received by the step |
| `output` | Actual output produced by the step |
| `step` | Generic step metadata |
| `run` | Trace version, run ID, and step count |
| `context` | Complete optional context JSON |
| `expected` | Value stored at `context.expected` |

Generic helpers include `hasField`, `isString`, `isNumber`, `isNonEmpty`,
`withinRange`, `equalsExpected`, and `sameField`.

Use trusted expected data for source and extraction steps. For transformations
and merges, compare the output to the actual input. Check `step.status`
explicitly when execution status should affect correctness.

See [docs/checkers.md](docs/checkers.md) for CEL helpers, limits, and version 1
migration.

### 4. Supply Expected Data

Expected values remain separate from the trace:

```json
{
  "environment": "test",
  "expected": {
    "claim_id": "CLM-104",
    "amount": 42000
  }
}
```

Save this as `context.json` and pass it with `--context`. The flag is optional
when rules only use the recorded input, output, or metadata.

### 5. Validate the Integration

```powershell
go run ./cmd/agenttrace validate --input ./trace.json --checkers ./checkers.json --context ./context.json
```

Validation checks JSON schemas, versions, step IDs, run IDs, dependencies,
cycles, CEL syntax, context data, and checker coverage. It compiles the rules but
does not evaluate step correctness.

### 6. Run Attribution

Print the human report and ASCII graph:

```powershell
go run ./cmd/agenttrace run --input ./trace.json --checkers ./checkers.json --context ./context.json
```

Return only JSON:

```powershell
go run ./cmd/agenttrace run --input ./trace.json --checkers ./checkers.json --context ./context.json --json
```

Write the privacy safe JSON result while retaining the human report:

```powershell
go run ./cmd/agenttrace run --input ./trace.json --checkers ./checkers.json --context ./context.json --output ./attribution-result.json
```

`--json` and `--output` cannot be combined. Raw step input, output, and expected
context are omitted by default. Include them only in an approved environment:

```powershell
go run ./cmd/agenttrace run --input ./trace.json --checkers ./checkers.json --context ./context.json --json --include-evidence
```

A failed version 2 result contains targets, independent root causes, all
divergences, checked steps, downstream candidates, and possible propagation
edges. `root_cause`, `affected_step_ids`, and `cause_edges` remain compatibility
aliases. A healthy result has status `passed` and no root cause.

When a graph has multiple output sinks, repeat `--target` to select the outputs
whose ancestry should be analyzed:

```powershell
go run ./cmd/agenttrace run --input ./trace.json --checkers ./checkers.json --target report --target alert
```

By default, the process exit code reports whether analysis completed, so a valid
result with status `failed` exits with code `0`. Add `--fail-on-attribution` to
return code `1` after writing a failed result:

```powershell
go run ./cmd/agenttrace run --input ./trace.json --checkers ./checkers.json --context ./context.json --json --fail-on-attribution
```

See [docs/result-format.md](docs/result-format.md) for the complete result
contract and classification rules.

## Go Recorder

The Go recorder accepts JSON compatible inputs and outputs and supports parallel
step recording.

```go
// recordTrace records one completed pipeline run.
//
// Input
// traceFile pointer to os.File
// Destination for the versioned trace.
//
// Output
// error
// Non nil when recording or writing fails.
func recordTrace(traceFile *os.File) error {
	run, err := recorder.StartRun("claim-run-42", recorder.Options{})
	if err != nil {
		return err
	}

	if err := run.StartStep(recorder.StepStart{
		StepID:    "extractor",
		AgentName: "Information Extractor",
		Input:     map[string]any{"document": "Claim CLM-104 requests $42,000."},
		ModelUsed: "provider-model",
	}); err != nil {
		return err
	}

	confidence := 0.94
	if err := run.FinishStep(
		"extractor",
		map[string]any{"claim_id": "CLM-104", "amount": 42000},
		&confidence,
	); err != nil {
		return err
	}

	return run.WriteTrace(traceFile)
}
```

Use `StartStep` before executing a step, then call `FinishStep` or `FailStep`.
`Trace` returns an in memory snapshot and `WriteTrace` writes versioned JSON.
The recorder rejects invalid lifecycle state and invalid graphs.

See [documentdemo](documentdemo) for a complete recorder based pipeline.

## Python Recorder

Install the dependency free helper:

```powershell
python -m pip install -e ./sdk/python
```

Record synchronous functions:

```python
from agenttrace import TraceRecorder

recorder = TraceRecorder("claim-run-42")

document = recorder.run_step(
    "loader",
    "Document Loader",
    {"path": "claim.txt"},
    lambda value: {"text": "loaded claim"},
)

recorder.run_step(
    "extractor",
    "Information Extractor",
    document,
    lambda value: {"claim_id": "CLM-104"},
    depends_on=["loader"],
    model_used="provider-model",
    confidence=0.94,
)

recorder.write_trace("trace.json")
```

The helper also provides manual lifecycle methods, `run_step_async`, and
`gather_recorded` for independent asynchronous branches. Wrapped exceptions are
recorded as structured failures and raised again.

See [sdk/python/README.md](sdk/python/README.md) and
[examples/python_recorded_pipeline.py](examples/python_recorded_pipeline.py).

## Demos

```powershell
go run ./cmd/agenttrace demo toy
go run ./cmd/agenttrace demo toy --healthy

go run ./cmd/agenttrace demo incident --failure metrics
go run ./cmd/agenttrace demo incident --failure deployment
go run ./cmd/agenttrace demo incident --failure none

go run ./cmd/agenttrace demo document --failure extraction
go run ./cmd/agenttrace demo document --failure reference
go run ./cmd/agenttrace demo document --failure none
```

All demos support human output, `--json`, `--output`, and
`--include-evidence`.
The separate real incident pipeline is documented under
`examples/incident_agent_pipeline`.

## Operational Behavior

| Input | Limit |
| --- | --- |
| Trace file | 16 MiB |
| Checker file | 2 MiB |
| Context file | 2 MiB |
| Steps per trace | 10,000 |
| Dependencies per step | 1,000 |

Each CEL expression has a cost limit and a 250 millisecond default timeout.
Topological sorting is deterministic and uses a heap backed ready queue.
Boxed graph rendering is bounded. Large, deep, wide, or edge dense graphs use a
compact node and edge view with explicit omission counts.

Result and trace files created by the CLI use owner read and write permissions
on platforms that support Unix permission bits. Treat traces as sensitive even
when output evidence is omitted from attribution results.

Process exit codes:

| Code | Meaning |
| --- | --- |
| `0` | Command completed |
| `1` | Runtime, file, graph, evaluation, or enabled attribution gate failure |
| `2` | Invalid command usage |

Structured command errors can be enabled on standard error:

```powershell
$env:AGENTTRACE_LOG_FORMAT = "json"
go run ./cmd/agenttrace unknown
```

## Testing

Run Go tests and static analysis:

```powershell
go test ./...
go vet ./...
```

Run race detection in an environment with CGO support:

```powershell
go test -race ./...
```

Run Python recorder tests:

```powershell
cd ./sdk/python
python -m unittest -v
cd ../..
```

Run real incident pipeline tests:

```powershell
python -m unittest discover -s ./examples/incident_agent_pipeline/tests -v
```

Run the cross language example:

```powershell
python ./examples/python_recorded_pipeline.py ./python-trace.json
go run ./cmd/agenttrace validate --input ./python-trace.json --checkers ./examples/python-checkers.json
```

Run one focused attribution test:

```powershell
go test ./documentdemo -run TestDocumentConfigurationAttributesFailureModes -v
```

GitHub Actions verifies Go modules and Python dependencies, checks formatting,
runs race detection and static analysis, enforces a 75 percent Go coverage
floor, compiles Python, runs both Python suites, regenerates all incident replay
traces, validates a generated trace, and verifies the attribution failure exit
gate.

## Project Structure

| Path | Purpose |
| --- | --- |
| `attrib` | Generic schema, graph validation, sorting, and attribution |
| `checkerconfig` | Versioned CEL rules, context, and helpers |
| `recorder` | Concurrency safe Go trace recorder |
| `sdk/python` | Dependency free Python trace recorder |
| `documentdemo` | Six step recorder based document pipeline |
| `incidentdemo` | Thirteen step incident investigation pipeline |
| `toypipeline` | Four step introductory pipeline |
| `integration` | Cross language integration tests |
| `cmd/agenttrace` | CLI commands, file boundaries, reports, and bounded DAG renderer |
| `examples/incident_agent_pipeline` | Live and replay 13-step Python pipeline |
| `examples` | Traces, checkers, contexts, and Python examples |
| `docs` | Detailed trace and checker contracts |

## Repository Workflow

Treat `main` as the stable baseline. Run relevant tests before merging and keep
changes focused. Multi agent branch ownership and handoff rules are in
[AGENTS.md](AGENTS.md).

Contribution and security guidance are in
[CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md).
