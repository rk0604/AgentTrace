# Incident Agent Pipeline

This integration demonstrates AgentTrace against a real agent workflow rather
than a manually assembled trace. The Python pipeline can call a live model,
execute independent analyzers concurrently, record every actual input and
output, and hand the completed trace to the Go attribution engine.

Replay mode remains deterministic and requires no API key, network access, or
model cost.

## Business Goal

The pipeline assists an on-call engineer with one question:

> Why did checkout-api degrade, what evidence supports that conclusion, and
> what should happen next?

The included fixture describes a database connection pool regression:

* Checkout p95 latency increased to 2400 ms.
* Database connection timeout errors began at incident start.
* Deploy 42 reduced the connection pool from 50 to 5.
* The operational runbook recommends restoring the pool and verifying recovery.

The healthy pipeline returns
`database_connection_pool_regression` with `SEV1` impact. It proposes actions
but always sets `automatic_execution` to `false`.

## Pipeline

```text
Incident Intake
      |
Investigation Planner
      |
      +----------------+-------------------+----------------------+----------------+
      |                |                   |                      |
 Log Analyzer    Metrics Analyzer   Deployment Analyzer     Runbook Loader
      |                |                   |                      |
      +----------------+-------------------+                      |
                       |                                          |
                 Evidence Merger                                  |
                       |                                          |
                 Timeline Builder                                 |
                       |                                          |
               Hypothesis Generator-------------------------------+
                       |                                          |
                 Impact Assessor                           Runbook Matcher
                       |                                          |
                       +-------------------+----------------------+
                                           |
                                 Remediation Planner
                                           |
                                    Incident Summary
```

The pipeline contains 13 recorded steps. Nine use the model client. Incident
Intake, Runbook Loader, Timeline Builder, and Runbook Matcher are deterministic
business functions.

Log, Metrics, Deployment, and Runbook branches begin independently. The three
model analyzers execute concurrently.

## Execution Modes

| Mode | Behavior |
| --- | --- |
| `replay` | Uses checked-in structured model responses |
| `live` | Calls the OpenAI Responses API |

Both modes support:

| Failure | Behavior |
| --- | --- |
| `none` | Healthy investigation |
| `metrics` | Metrics Analyzer reports critical metrics as normal |
| `deployment` | Deployment Analyzer selects the wrong deployment |

Replay failure files also contain downstream responses that are correct for the
bad input they received. In live mode, only the selected source analyzer is
overridden and later model calls process that result normally.

## Run Replay Mode

Run commands from the repository root.

Healthy pipeline:

```powershell
python -m examples.incident_agent_pipeline `
  --mode replay `
  --failure none `
  --trace-output ./incident-agent-trace.json
```

Metrics failure:

```powershell
python -m examples.incident_agent_pipeline `
  --mode replay `
  --failure metrics `
  --trace-output ./incident-agent-trace.json
```

Deployment failure:

```powershell
python -m examples.incident_agent_pipeline `
  --mode replay `
  --failure deployment `
  --trace-output ./incident-agent-trace.json
```

Add `--summary-output ./incident-summary.json` to write the business result
separately from the AgentTrace file.

## Attribute the Trace

Validate the trace and all 13 checker entries:

```powershell
go run ./cmd/agenttrace validate `
  --input ./incident-agent-trace.json `
  --checkers ./examples/incident_agent_pipeline/checkers-v2.json `
  --context ./examples/incident_agent_pipeline/context.json
```

Print the root cause report and ASCII DAG:

```powershell
go run ./cmd/agenttrace run `
  --input ./incident-agent-trace.json `
  --checkers ./examples/incident_agent_pipeline/checkers-v2.json `
  --context ./examples/incident_agent_pipeline/context.json
```

Use the result as a CI gate:

```powershell
go run ./cmd/agenttrace run `
  --input ./incident-agent-trace.json `
  --checkers ./examples/incident_agent_pipeline/checkers-v2.json `
  --context ./examples/incident_agent_pipeline/context.json `
  --json `
  --fail-on-attribution
```

The final command exits with code `1` for metrics and deployment failure traces
after writing the complete JSON attribution result.

## Run Live Mode

Install the optional OpenAI Python dependency:

```powershell
python -m pip install -r ./examples/incident_agent_pipeline/requirements-live.txt
```

Configure credentials and an explicit model:

```powershell
$env:OPENAI_API_KEY = "replace-with-api-key"
$env:AGENTTRACE_OPENAI_MODEL = "replace-with-model-id"
```

Run the pipeline:

```powershell
python -m examples.incident_agent_pipeline `
  --mode live `
  --failure none `
  --trace-output ./incident-agent-trace.json
```

The selected model must support structured outputs. The adapter uses the
Responses API `text.format` JSON schema contract described in the
[OpenAI structured outputs documentation](https://developers.openai.com/api/docs/guides/structured-outputs).

Live mode makes nine model requests and can incur API cost. No default model is
selected because model availability, cost, and quality requirements vary.

## Correctness Rules

`checkers-v2.json` separates two kinds of correctness:

* Source analyzers are compared with trusted values in `context.json`.
* Downstream steps are compared with the actual inputs recorded in the trace.

This is why a metrics failure is attributed to `metrics_analyzer`. Evidence
Merger, Hypothesis Generator, Remediation Planner, and Incident Summary can all
pass when they faithfully process the incorrect metric finding.

The expected context is used only by AgentTrace after the pipeline run. It is
not included in model prompts.

## Failure Handling

Every started step is completed with either `ok` or `error` status. Parallel
branches are allowed to finish before a branch error returns.

When a provider request, timeout, malformed response, or contract validation
fails:

1. The failing step receives structured error output.
2. Other already started analyzer branches finish.
3. A valid partial trace is written.
4. The Python command exits with code `1`.

The model client performs two attempts by default and applies a separate timeout
to each attempt.

## Redaction

Trace data is redacted before it enters the recorder.

The following key names are redacted automatically:

* `api_key`
* `authorization`
* `password`
* `secret`
* `token`

The current `OPENAI_API_KEY` is also treated as a secret. Additional literal
values can be configured as a comma-separated list:

```powershell
$env:AGENTTRACE_REDACT_VALUES = "internal-secret,customer-identifier"
```

Redaction protects trace output. It does not change the unredacted values used
by in-process pipeline logic or model calls.

## Tests

Run the Python integration suite:

```powershell
python -m unittest discover `
  -s ./examples/incident_agent_pipeline/tests `
  -v
```

Run the Go cross-language attribution tests:

```powershell
go test ./integration -run IncidentAgentPipeline -v
```

The test matrix covers:

* Healthy, metrics failure, and deployment failure replays
* Exact 13-step dependencies
* Parallel analyzer execution
* Strict output contract validation
* Retry, malformed response, and timeout handling
* Valid partial traces
* Secret redaction
* Healthy attribution
* Metrics and deployment root cause attribution
* Downstream correctness against actual inputs

Live model calls are intentionally excluded from normal CI. Replay mode verifies
the same pipeline, prompts, schemas, recording, and attribution boundaries
without credentials or nondeterminism.

## Files

| Path | Purpose |
| --- | --- |
| `fixtures` | Alert, logs, metrics, deployments, and runbook |
| `replays` | Healthy responses and semantic failure overlays |
| `traces` | Checked-in complete traces used by Go integration tests |
| `contracts.py` | Step IDs, dependencies, and strict JSON schemas |
| `fixtures.py` | Fixture loading and consistency validation |
| `model_runtime.py` | Replay, live, retry, timeout, and fault clients |
| `pipeline.py` | 13-step orchestration and trace recording |
| `prompts.py` | Business prompts for model-backed steps |
| `redaction.py` | Trace redaction |
| `checkers-v2.json` | CEL correctness checks for every step |
| `context.json` | Trusted attribution context |
| `tests` | Python unit and integration tests |
