# Attribution Result Format

AgentTrace emits a versioned JSON attribution result. The current result version
is `2`.

## Example

```json
{
  "version": 2,
  "run_id": "incident-run-42",
  "status": "failed",
  "target_step_ids": [
    "incident_summary"
  ],
  "root_cause": {
    "step_id": "metrics_analyzer",
    "agent_name": "Metrics Analyzer",
    "reason": "Metrics Analyzer misclassified the threshold breach",
    "expression": "output.state == \"critical\""
  },
  "root_causes": [
    {
      "step_id": "metrics_analyzer",
      "agent_name": "Metrics Analyzer",
      "reason": "Metrics Analyzer misclassified the threshold breach",
      "expression": "output.state == \"critical\""
    }
  ],
  "divergences": [
    {
      "step_id": "metrics_analyzer",
      "agent_name": "Metrics Analyzer",
      "reason": "Metrics Analyzer misclassified the threshold breach",
      "expression": "output.state == \"critical\"",
      "classification": "root_cause"
    }
  ],
  "checked_step_ids": [
    "incident_intake",
    "metrics_analyzer",
    "incident_summary"
  ],
  "downstream_candidate_step_ids": [
    "incident_summary"
  ],
  "potential_propagation_edges": [
    {
      "from_step_id": "metrics_analyzer",
      "to_step_id": "incident_summary"
    }
  ]
}
```

## Top Level Fields

| Field | Type | Meaning |
| --- | --- | --- |
| `version` | integer | Attribution result contract version |
| `run_id` | string | Run ID copied from the trace |
| `status` | string | `passed` or `failed` |
| `target_step_ids` | array of strings | Output steps whose ancestry was evaluated |
| `root_cause` | object | First root cause in dependency order for compatibility |
| `root_causes` | array of objects | Every independent earliest divergence |
| `divergences` | array of objects | Every failed check and its classification |
| `secondary_divergences` | array of objects | Failed steps that have a failed ancestor |
| `checked_step_ids` | array of strings | Relevant steps evaluated in dependency order |
| `downstream_candidate_step_ids` | array of strings | Relevant descendants of root causes |
| `potential_propagation_edges` | array of objects | Dependency edges on possible downstream paths |
| `affected_step_ids` | array of strings | Compatibility alias for downstream candidates |
| `cause_edges` | array of objects | Compatibility alias for propagation edges |

Optional fields are omitted when empty.

## Target Selection

AgentTrace analyzes target ancestry rather than unrelated graph branches.

When the graph has exactly one sink and no `--target` flag is supplied, that
sink is selected automatically. A graph with multiple sinks requires at least
one explicit `--target`. Repeat the flag to analyze more than one output.

Every trace step still requires a checker. Exact checker coverage prevents stale
or misspelled checker entries from being silently ignored.

## Divergence Classification

A divergence is any relevant step whose checker fails.

A `root_cause` divergence has no failed ancestor in the selected subgraph. More
than one independent branch can therefore produce more than one root cause.

A `secondary_divergence` is a failed step with a failed ancestor. It identifies
an additional incorrect transformation without replacing the earlier upstream
cause.

A downstream candidate is not automatically incorrect. It is a descendant that
could have consumed data originating at a root cause. Its checker result remains
the source of truth.

## Root Cause Fields

| Field | Type | Meaning |
| --- | --- | --- |
| `step_id` | string | Stable trace step ID |
| `agent_name` | string | Human readable agent name |
| `reason` | string | Failure reason from checker configuration |
| `expression` | string | CEL expression that returned false |
| `input` | any JSON value | Actual step input |
| `output` | any JSON value | Actual step output |
| `expected` | any JSON value | Expected context supplied to the checker |

The CLI omits `input`, `output`, and `expected` by default. Use
`--include-evidence` only when raw payload disclosure is acceptable.

## Compatibility

Consumers should branch on `version` and ignore unknown fields. They should use
`root_causes`, `divergences`, `downstream_candidate_step_ids`, and
`potential_propagation_edges` for version 2 behavior.

`root_cause`, `affected_step_ids`, and `cause_edges` remain available so version
1 style consumers can migrate without losing the first root cause.

## Exit Codes

A valid result with `status` set to `failed` exits with code `0` by default.
This separates attribution findings from command execution failures.

Use `--fail-on-attribution` to return code `1` after a failed result has been
written. Invalid command usage returns code `2`.
