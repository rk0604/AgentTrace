# AgentTrace Python Recorder

This dependency free helper records Python pipeline steps in the versioned JSON
format accepted by the AgentTrace Go CLI. Attribution remains in the Go tool.

## Local installation

From the repository root:

```powershell
python -m pip install -e ./sdk/python
```

## Basic usage

```python
from agenttrace import TraceRecorder

recorder = TraceRecorder("claim-run-42")

document = recorder.run_step(
    "loader",
    "Document Loader",
    {"path": "claim.txt"},
    lambda value: {"text": "loaded claim"},
)

result = recorder.run_step(
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

For asynchronous agents, use `run_step_async`. Independent asynchronous steps
can be passed to `gather_recorded`.

When a wrapped function raises an exception, the recorder writes structured
failure details to that step and raises the original exception again.

The recorder validates the same trace contract as the Go engine, including
unknown fields, timestamps, finite confidence values, duplicate dependencies,
unknown dependencies, cycles, and graph size limits. JSON encoding rejects
nonfinite numbers.

`write_trace` creates files with owner read and write permissions on platforms
that support Unix permission bits. The recorder does not infer which domain
values are sensitive, so integrations must redact data before passing it to the
recorder.

## Test

```powershell
cd sdk/python
python -m unittest -v
```

See `examples/python_recorded_pipeline.py` for a complete pipeline.
