# Trace Format

AgentTrace uses a language neutral JSON trace. The current schema version is
`1`. A missing version is accepted as legacy version 1 input.

## Trace

```json
{
  "version": 1,
  "run_id": "claim-run-42",
  "steps": []
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `version` | integer | Trace schema version |
| `run_id` | string | Stable identifier for the complete pipeline run |
| `steps` | array | Recorded pipeline steps |

## Step

```json
{
  "run_id": "claim-run-42",
  "step_id": "extractor",
  "agent_name": "Information Extractor",
  "depends_on": ["loader"],
  "input": {"text": "claim text"},
  "output": {"claim_id": "CLM-104"},
  "model_used": "provider-model",
  "confidence": 0.94,
  "timestamp": "2026-07-28T12:00:00Z",
  "status": "ok"
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `run_id` | string | Must match the trace run ID |
| `step_id` | string | Unique step identifier |
| `agent_name` | string | Human readable agent name |
| `depends_on` | string array | Steps that produced this step input |
| `input` | any JSON | Actual input received by the step |
| `output` | any JSON | Actual output produced by the step |
| `model_used` | string | Model or implementation identifier |
| `confidence` | number | Optional value from zero through one |
| `timestamp` | RFC 3339 string | Time at which the step started |
| `status` | string | Final execution status |

`depends_on` describes data lineage, not call order. Parallel steps can have the
same dependency or no dependency between each other.

Domain fields belong only inside `input` and `output`. Generic trace structs
must not gain fields such as revenue, claim amount, quarter, or incident cause.

## Validation rules

- Trace and step run IDs must be present and match.
- Step IDs must be unique.
- Dependencies must exist and cannot be repeated.
- The dependency graph must be acyclic.
- Input and output values must contain valid JSON.
- Confidence must be between zero and one.
- A trace can contain at most 10,000 steps.
- One step can declare at most 1,000 dependencies.

The CLI accepts trace files up to 16 MiB.
