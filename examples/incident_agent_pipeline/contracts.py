"""Step identifiers, JSON schemas, and local output validation."""

from __future__ import annotations

from typing import Any

from jsonschema import Draft202012Validator
from jsonschema.exceptions import ValidationError


INCIDENT_INTAKE = "incident_intake"
INVESTIGATION_PLANNER = "investigation_planner"
LOG_ANALYZER = "log_analyzer"
METRICS_ANALYZER = "metrics_analyzer"
DEPLOYMENT_ANALYZER = "deployment_analyzer"
RUNBOOK_LOADER = "runbook_loader"
EVIDENCE_MERGER = "evidence_merger"
TIMELINE_BUILDER = "timeline_builder"
HYPOTHESIS_GENERATOR = "hypothesis_generator"
IMPACT_ASSESSOR = "impact_assessor"
RUNBOOK_MATCHER = "runbook_matcher"
REMEDIATION_PLANNER = "remediation_planner"
INCIDENT_SUMMARY = "incident_summary"

STEP_IDS = (
    INCIDENT_INTAKE,
    INVESTIGATION_PLANNER,
    LOG_ANALYZER,
    METRICS_ANALYZER,
    DEPLOYMENT_ANALYZER,
    RUNBOOK_LOADER,
    EVIDENCE_MERGER,
    TIMELINE_BUILDER,
    HYPOTHESIS_GENERATOR,
    IMPACT_ASSESSOR,
    RUNBOOK_MATCHER,
    REMEDIATION_PLANNER,
    INCIDENT_SUMMARY,
)

AGENT_NAMES = {
    INCIDENT_INTAKE: "Incident Intake",
    INVESTIGATION_PLANNER: "Investigation Planner",
    LOG_ANALYZER: "Log Analyzer",
    METRICS_ANALYZER: "Metrics Analyzer",
    DEPLOYMENT_ANALYZER: "Deployment Analyzer",
    RUNBOOK_LOADER: "Runbook Loader",
    EVIDENCE_MERGER: "Evidence Merger",
    TIMELINE_BUILDER: "Timeline Builder",
    HYPOTHESIS_GENERATOR: "Hypothesis Generator",
    IMPACT_ASSESSOR: "Impact Assessor",
    RUNBOOK_MATCHER: "Runbook Matcher",
    REMEDIATION_PLANNER: "Remediation Planner",
    INCIDENT_SUMMARY: "Incident Summary",
}

DEPENDENCIES = {
    INCIDENT_INTAKE: (),
    INVESTIGATION_PLANNER: (INCIDENT_INTAKE,),
    LOG_ANALYZER: (INCIDENT_INTAKE, INVESTIGATION_PLANNER),
    METRICS_ANALYZER: (INCIDENT_INTAKE, INVESTIGATION_PLANNER),
    DEPLOYMENT_ANALYZER: (INCIDENT_INTAKE, INVESTIGATION_PLANNER),
    RUNBOOK_LOADER: (INCIDENT_INTAKE, INVESTIGATION_PLANNER),
    EVIDENCE_MERGER: (
        LOG_ANALYZER,
        METRICS_ANALYZER,
        DEPLOYMENT_ANALYZER,
    ),
    TIMELINE_BUILDER: (EVIDENCE_MERGER,),
    HYPOTHESIS_GENERATOR: (EVIDENCE_MERGER, TIMELINE_BUILDER),
    IMPACT_ASSESSOR: (EVIDENCE_MERGER, HYPOTHESIS_GENERATOR),
    RUNBOOK_MATCHER: (RUNBOOK_LOADER, HYPOTHESIS_GENERATOR),
    REMEDIATION_PLANNER: (
        HYPOTHESIS_GENERATOR,
        IMPACT_ASSESSOR,
        RUNBOOK_MATCHER,
    ),
    INCIDENT_SUMMARY: (HYPOTHESIS_GENERATOR, REMEDIATION_PLANNER),
}

MODEL_STEP_IDS = frozenset(
    {
        INVESTIGATION_PLANNER,
        LOG_ANALYZER,
        METRICS_ANALYZER,
        DEPLOYMENT_ANALYZER,
        EVIDENCE_MERGER,
        HYPOTHESIS_GENERATOR,
        IMPACT_ASSESSOR,
        REMEDIATION_PLANNER,
        INCIDENT_SUMMARY,
    }
)

PLANNER_TASK_IDS = (
    LOG_ANALYZER,
    METRICS_ANALYZER,
    DEPLOYMENT_ANALYZER,
    RUNBOOK_LOADER,
)


class ContractError(ValueError):
    """Reports a value that does not satisfy a step output contract."""


def _object(properties: dict[str, dict[str, Any]]) -> dict[str, Any]:
    """Create a strict object schema.

    Input
    properties dict of str to dict
    Property names and their JSON schemas.

    Output
    dict of str to Any
    JSON Schema object with every property required.
    """

    return {
        "type": "object",
        "properties": properties,
        "required": list(properties),
        "additionalProperties": False,
    }


def _array(items: dict[str, Any]) -> dict[str, Any]:
    """Create an array schema.

    Input
    items dict of str to Any
    JSON schema applied to every array item.

    Output
    dict of str to Any
    JSON Schema array.
    """

    return {"type": "array", "items": items}


STRING = {"type": "string"}
INTEGER = {"type": "integer"}
NUMBER = {"type": "number"}
BOOLEAN = {"type": "boolean"}
STRING_ARRAY = _array(STRING)

TIMELINE_EVENT_SCHEMA = _object(
    {
        "timestamp": STRING,
        "event_type": {
            "type": "string",
            "enum": ["deployment", "error", "metric"],
        },
        "evidence_id": STRING,
    }
)

OUTPUT_SCHEMAS: dict[str, dict[str, Any]] = {
    INCIDENT_INTAKE: _object(
        {
            "incident_id": STRING,
            "service": STRING,
            "environment": STRING,
            "started_at": STRING,
            "symptom": STRING,
            "accepted": BOOLEAN,
            "evidence_sources": STRING_ARRAY,
        }
    ),
    INVESTIGATION_PLANNER: _object(
        {
            "incident_id": STRING,
            "service": STRING,
            "tasks": _object(
                {
                    LOG_ANALYZER: STRING,
                    METRICS_ANALYZER: STRING,
                    DEPLOYMENT_ANALYZER: STRING,
                    RUNBOOK_LOADER: STRING,
                }
            ),
            "investigation_goal": STRING,
        }
    ),
    LOG_ANALYZER: _object(
        {
            "service": STRING,
            "primary_error_code": STRING,
            "error_count": INTEGER,
            "first_error_at": STRING,
            "evidence_ids": STRING_ARRAY,
            "summary": STRING,
        }
    ),
    METRICS_ANALYZER: _object(
        {
            "observed_at": STRING,
            "p95_latency_ms": INTEGER,
            "latency_threshold_ms": INTEGER,
            "error_rate": NUMBER,
            "connection_pool_utilization": NUMBER,
            "state": {"type": "string", "enum": ["normal", "critical"]},
            "evidence_ids": STRING_ARRAY,
            "summary": STRING,
        }
    ),
    DEPLOYMENT_ANALYZER: _object(
        {
            "deployment_id": STRING,
            "deployed_at": STRING,
            "changed_field": STRING,
            "before_value": INTEGER,
            "after_value": INTEGER,
            "correlated": BOOLEAN,
            "evidence_ids": STRING_ARRAY,
            "summary": STRING,
        }
    ),
    RUNBOOK_LOADER: _object(
        {
            "runbook_id": STRING,
            "cause": STRING,
            "match_signals": STRING_ARRAY,
            "recommended_actions": STRING_ARRAY,
            "verification_steps": STRING_ARRAY,
            "automatic_execution": BOOLEAN,
            "loaded": BOOLEAN,
            "evidence_ids": STRING_ARRAY,
        }
    ),
    EVIDENCE_MERGER: _object(
        {
            "service": STRING,
            "error_code": STRING,
            "error_count": INTEGER,
            "first_error_at": STRING,
            "metric_state": {"type": "string", "enum": ["normal", "critical"]},
            "p95_latency_ms": INTEGER,
            "latency_threshold_ms": INTEGER,
            "metric_observed_at": STRING,
            "connection_pool_utilization": NUMBER,
            "deployment_id": STRING,
            "deployment_at": STRING,
            "deployment_correlated": BOOLEAN,
            "changed_field": STRING,
            "before_value": INTEGER,
            "after_value": INTEGER,
            "evidence_ids": STRING_ARRAY,
            "signals": STRING_ARRAY,
        }
    ),
    TIMELINE_BUILDER: _object(
        {
            "events": _array(TIMELINE_EVENT_SCHEMA),
            "ordered": BOOLEAN,
        }
    ),
    HYPOTHESIS_GENERATOR: _object(
        {
            "cause": STRING,
            "confidence": {
                "type": "string",
                "enum": ["low", "medium", "high"],
            },
            "evidence_ids": STRING_ARRAY,
            "rationale": STRING,
        }
    ),
    IMPACT_ASSESSOR: _object(
        {
            "severity": {"type": "string", "enum": ["SEV1", "SEV2", "SEV3"]},
            "affected_service": STRING,
            "customer_impact": STRING,
            "evidence_ids": STRING_ARRAY,
        }
    ),
    RUNBOOK_MATCHER: _object(
        {
            "runbook_id": STRING,
            "applicable": BOOLEAN,
            "cause": STRING,
            "actions": STRING_ARRAY,
            "verification_steps": STRING_ARRAY,
        }
    ),
    REMEDIATION_PLANNER: _object(
        {
            "cause": STRING,
            "priority": {"type": "string", "enum": ["SEV1", "SEV2", "SEV3"]},
            "actions": STRING_ARRAY,
            "verification_steps": STRING_ARRAY,
            "owner": STRING,
            "automatic_execution": BOOLEAN,
        }
    ),
    INCIDENT_SUMMARY: _object(
        {
            "title": STRING,
            "root_cause": STRING,
            "severity": {"type": "string", "enum": ["SEV1", "SEV2", "SEV3"]},
            "customer_impact": STRING,
            "recommended_actions": STRING_ARRAY,
            "evidence_ids": STRING_ARRAY,
            "automatic_execution": BOOLEAN,
        }
    ),
}

ANY_OBJECT = {"type": "object"}
ANY_OBJECT_ARRAY = _array(ANY_OBJECT)

MODEL_INPUT_SCHEMAS: dict[str, dict[str, Any]] = {
    INVESTIGATION_PLANNER: OUTPUT_SCHEMAS[INCIDENT_INTAKE],
    LOG_ANALYZER: _object(
        {
            "incident": OUTPUT_SCHEMAS[INCIDENT_INTAKE],
            "task": STRING,
            "logs": ANY_OBJECT_ARRAY,
        }
    ),
    METRICS_ANALYZER: _object(
        {
            "incident": OUTPUT_SCHEMAS[INCIDENT_INTAKE],
            "task": STRING,
            "metrics": ANY_OBJECT,
        }
    ),
    DEPLOYMENT_ANALYZER: _object(
        {
            "incident": OUTPUT_SCHEMAS[INCIDENT_INTAKE],
            "task": STRING,
            "deployments": ANY_OBJECT_ARRAY,
        }
    ),
    EVIDENCE_MERGER: _object(
        {
            "logs": OUTPUT_SCHEMAS[LOG_ANALYZER],
            "metrics": OUTPUT_SCHEMAS[METRICS_ANALYZER],
            "deployment": OUTPUT_SCHEMAS[DEPLOYMENT_ANALYZER],
        }
    ),
    HYPOTHESIS_GENERATOR: _object(
        {
            "evidence": OUTPUT_SCHEMAS[EVIDENCE_MERGER],
            "timeline": OUTPUT_SCHEMAS[TIMELINE_BUILDER],
        }
    ),
    IMPACT_ASSESSOR: _object(
        {
            "evidence": OUTPUT_SCHEMAS[EVIDENCE_MERGER],
            "hypothesis": OUTPUT_SCHEMAS[HYPOTHESIS_GENERATOR],
        }
    ),
    REMEDIATION_PLANNER: _object(
        {
            "hypothesis": OUTPUT_SCHEMAS[HYPOTHESIS_GENERATOR],
            "impact": OUTPUT_SCHEMAS[IMPACT_ASSESSOR],
            "runbook": OUTPUT_SCHEMAS[RUNBOOK_MATCHER],
        }
    ),
    INCIDENT_SUMMARY: _object(
        {
            "hypothesis": OUTPUT_SCHEMAS[HYPOTHESIS_GENERATOR],
            "impact": OUTPUT_SCHEMAS[IMPACT_ASSESSOR],
            "remediation": OUTPUT_SCHEMAS[REMEDIATION_PLANNER],
        }
    ),
}

OUTPUT_VALIDATORS = {
    step_id: Draft202012Validator(schema)
    for step_id, schema in OUTPUT_SCHEMAS.items()
}
MODEL_INPUT_VALIDATORS = {
    step_id: Draft202012Validator(schema)
    for step_id, schema in MODEL_INPUT_SCHEMAS.items()
}


def validate_output(step_id: str, value: Any) -> None:
    """Validate one step output.

    Input
    step_id str
    Step whose output contract should be used.

    value Any
    Candidate JSON compatible output.

    Output
    None
    The function returns after successful validation.
    """

    if step_id not in OUTPUT_SCHEMAS:
        raise ContractError(f"unknown step output contract {step_id!r}")

    _validate(
        OUTPUT_VALIDATORS[step_id],
        value,
        f"output for {step_id}",
    )


def validate_model_input(step_id: str, value: Any) -> None:
    """Validate one model step input.

    Input
    step_id str
    Model backed step whose input contract should be used.

    value Any
    Candidate JSON compatible input.

    Output
    None
    The function returns after successful validation.
    """

    if step_id not in MODEL_INPUT_SCHEMAS:
        raise ContractError(f"unknown model input contract {step_id!r}")

    _validate(
        MODEL_INPUT_VALIDATORS[step_id],
        value,
        f"input for {step_id}",
    )


def _validate(
    validator: Draft202012Validator,
    value: Any,
    root_path: str,
) -> None:
    """Validate a value with one compiled JSON Schema validator.

    Input
    validator Draft202012Validator
    Validator for the required contract.

    value Any
    Candidate JSON compatible value.

    root_path str
    Human readable root included in errors.

    Output
    None
    The function returns after successful validation.
    """

    errors = sorted(
        validator.iter_errors(value),
        key=lambda error: tuple(str(part) for part in error.absolute_path),
    )
    if not errors:
        return

    error = errors[0]
    path = _error_path(root_path, error)
    raise ContractError(f"{path}: {error.message}")


def _error_path(root_path: str, error: ValidationError) -> str:
    """Build a readable path for one schema failure.

    Input
    root_path str
    Human readable validation root.

    error ValidationError
    JSON Schema validation failure.

    Output
    str
    Root with object fields and array indexes appended.
    """

    path = root_path
    for part in error.absolute_path:
        if isinstance(part, int):
            path += f"[{part}]"
        else:
            path += f".{part}"
    return path
