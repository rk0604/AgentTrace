# Checker Configuration

Checker files define correctness separately from pipeline execution. AgentTrace
supports legacy version 1 and current version 2 configurations.

## Version 2

Version 2 supports multiple ordered assertions for each step:

```json
{
  "version": 2,
  "steps": {
    "extractor": {
      "checks": [
        {
          "expression": "hasField(output, \"claim_id\")",
          "failure_reason": "Extractor omitted claim_id"
        },
        {
          "expression": "output.claim_id == expected.claim_id",
          "failure_reason": "Extractor returned the wrong claim ID"
        }
      ]
    }
  }
}
```

Assertions run in order. The first false expression becomes the reason attached
to the root cause.

Version 2 also accepts the version 1 single expression fields for incremental
migration. One step cannot combine single expression fields with `checks`.

## Evaluation variables

| Variable | Meaning |
| --- | --- |
| `input` | Actual step input JSON |
| `output` | Actual step output JSON |
| `step` | Generic step metadata |
| `run` | Trace version, run ID, and step count |
| `context` | Complete optional context JSON |
| `expected` | Value stored at `context.expected` |

Context is supplied separately:

```powershell
go run ./cmd/agenttrace run `
  --input ./trace.json `
  --checkers ./checkers.json `
  --context ./context.json
```

Example context:

```json
{
  "environment": "test",
  "expected": {
    "claim_id": "CLM-104"
  }
}
```

## Generic helpers

| Helper | Result |
| --- | --- |
| `hasField(value, field)` | True when a map contains the field |
| `isString(value)` | True for strings |
| `isNumber(value)` | True for integer, unsigned, or floating point numbers |
| `isNonEmpty(value)` | True for nonempty strings and collections |
| `withinRange(value, min, max)` | Inclusive numeric range check |
| `equalsExpected(actual, expected)` | Generic equality check |
| `sameField(left, right, field)` | Compares one field across two maps |

CEL built ins such as `has`, `size`, `matches`, `startsWith`, and `endsWith`
remain available.

Each expression has a cost limit and a 250 millisecond default timeout. Checker
and context files are limited to 2 MiB each.

Checker coverage is exact. Every trace step requires one checker entry, and a
checker entry that does not match a trace step is rejected. This catches stale
configuration and misspelled step IDs before evaluation.

## Correctness rule

A checker answers whether a step behaved correctly given the input it actually
received. A downstream step should pass when it faithfully transforms incorrect
upstream data. This rule allows attribution to identify the earliest independent
sources instead of the final visible symptoms.

AgentTrace evaluates all steps in the selected target ancestry. Independent
failed branches become separate root causes. A failed step with a failed
ancestor becomes a secondary divergence.
