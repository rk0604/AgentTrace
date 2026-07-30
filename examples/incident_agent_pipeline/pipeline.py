"""Execute and record the real incident investigation pipeline."""

from __future__ import annotations

import asyncio
import sys
import threading
from collections.abc import Callable
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

from . import contracts
from .model_runtime import ModelClient
from .redaction import Redactor


try:
    from agenttrace import TraceRecorder
except ModuleNotFoundError:
    REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
    PYTHON_SDK = REPOSITORY_ROOT / "sdk" / "python"
    sys.path.insert(0, str(PYTHON_SDK))
    from agenttrace import TraceRecorder


DETERMINISTIC_MODEL_NAME = "deterministic-python"


class PipelineExecutionError(RuntimeError):
    """Reports the pipeline step that could not complete."""

    def __init__(self, step_id: str, cause: Exception) -> None:
        """Create a step execution error.

        Input
        step_id str
        Step that failed.

        cause Exception
        Original execution error.

        Output
        None
        The contextual error is initialized.
        """

        super().__init__(f"step {step_id!r} failed: {cause}")
        self.step_id = step_id
        self.cause = cause


class SequenceClock:
    """Returns deterministic timestamps for replay traces."""

    def __init__(
        self,
        start: datetime | None = None,
    ) -> None:
        """Create a sequence clock.

        Input
        start datetime or None
        First timestamp, defaulting to the replay run time.

        Output
        None
        The sequence clock is initialized.
        """

        self._next = start or datetime(
            2026,
            7,
            20,
            10,
            5,
            tzinfo=timezone.utc,
        )
        self._lock = threading.Lock()

    def __call__(self) -> datetime:
        """Return the next timestamp.

        Input
        None

        Output
        datetime
        Current timestamp before advancing one second.
        """

        with self._lock:
            current = self._next
            self._next += timedelta(seconds=1)
            return current


class IncidentPipeline:
    """Runs one 13 step incident investigation and records its trace."""

    def __init__(
        self,
        run_id: str,
        fixture_bundle: dict[str, Any],
        model_client: ModelClient,
        redactor: Redactor | None = None,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        """Create an incident pipeline.

        Input
        run_id str
        Stable identifier for the recorded pipeline run.

        fixture_bundle dict of str to Any
        Alert, logs, metrics, deployments, and runbook evidence.

        model_client ModelClient
        Live or replay structured generation client.

        redactor Redactor or None
        Optional trace data redactor.

        clock callable returning datetime or None
        Optional recorder clock.

        Output
        None
        Pipeline state and trace recorder are initialized.
        """

        self._fixture = fixture_bundle
        self._model_client = model_client
        self._redactor = redactor or Redactor()
        self._recorder = TraceRecorder(run_id, clock=clock)
        self._outputs: dict[str, dict[str, Any]] = {}

    async def run(self) -> dict[str, Any]:
        """Run the complete incident investigation.

        Input
        None

        Output
        dict of str to Any
        Final incident summary.
        """

        incident = self._run_sync_step(
            contracts.INCIDENT_INTAKE,
            self._fixture,
            _intake_incident,
        )
        plan = await self._run_model_step(
            contracts.INVESTIGATION_PLANNER,
            incident,
        )

        branch_inputs = _build_branch_inputs(
            self._fixture,
            incident,
            plan,
        )
        branch_results = await asyncio.gather(
            self._run_model_step(
                contracts.LOG_ANALYZER,
                branch_inputs[contracts.LOG_ANALYZER],
            ),
            self._run_model_step(
                contracts.METRICS_ANALYZER,
                branch_inputs[contracts.METRICS_ANALYZER],
            ),
            self._run_model_step(
                contracts.DEPLOYMENT_ANALYZER,
                branch_inputs[contracts.DEPLOYMENT_ANALYZER],
            ),
            self._run_sync_step_async(
                contracts.RUNBOOK_LOADER,
                branch_inputs[contracts.RUNBOOK_LOADER],
                _load_runbook,
            ),
            return_exceptions=True,
        )
        _raise_first_branch_error(branch_results)

        log_finding = _require_result(branch_results[0])
        metric_finding = _require_result(branch_results[1])
        deployment_finding = _require_result(branch_results[2])
        loaded_runbook = _require_result(branch_results[3])

        evidence_input = {
            "logs": log_finding,
            "metrics": metric_finding,
            "deployment": deployment_finding,
        }
        evidence = await self._run_model_step(
            contracts.EVIDENCE_MERGER,
            evidence_input,
        )
        timeline = self._run_sync_step(
            contracts.TIMELINE_BUILDER,
            evidence,
            _build_timeline,
        )
        hypothesis = await self._run_model_step(
            contracts.HYPOTHESIS_GENERATOR,
            {
                "evidence": evidence,
                "timeline": timeline,
            },
        )

        impact_result, runbook_result = await asyncio.gather(
            self._run_model_step(
                contracts.IMPACT_ASSESSOR,
                {
                    "evidence": evidence,
                    "hypothesis": hypothesis,
                },
            ),
            self._run_sync_step_async(
                contracts.RUNBOOK_MATCHER,
                {
                    "hypothesis": hypothesis,
                    "runbook": loaded_runbook,
                },
                _match_runbook,
            ),
        )

        remediation = await self._run_model_step(
            contracts.REMEDIATION_PLANNER,
            {
                "hypothesis": hypothesis,
                "impact": impact_result,
                "runbook": runbook_result,
            },
        )
        summary = await self._run_model_step(
            contracts.INCIDENT_SUMMARY,
            {
                "hypothesis": hypothesis,
                "impact": impact_result,
                "remediation": remediation,
            },
        )
        return summary

    def trace(self) -> dict[str, Any]:
        """Return the current completed trace.

        Input
        None

        Output
        dict of str to Any
        Versioned trace containing every started and completed step.
        """

        return self._recorder.trace()

    def write_trace(self, path: str | Path) -> None:
        """Write the current completed trace.

        Input
        path str or Path
        Destination trace file.

        Output
        None
        Versioned trace JSON is written.
        """

        self._recorder.write_trace(path)

    def output(self, step_id: str) -> dict[str, Any]:
        """Return one unredacted in-process step output.

        Input
        step_id str
        Completed step identifier.

        Output
        dict of str to Any
        Step output used by downstream pipeline logic.
        """

        if step_id not in self._outputs:
            raise KeyError(f"step {step_id!r} has no output")
        return self._outputs[step_id]

    def _run_sync_step(
        self,
        step_id: str,
        input_data: Any,
        function: Callable[[Any], dict[str, Any]],
    ) -> dict[str, Any]:
        """Execute and record one deterministic step.

        Input
        step_id str
        Step identifier.

        input_data Any
        Actual business input.

        function callable
        Deterministic business function.

        Output
        dict of str to Any
        Validated step output.
        """

        self._recorder.start_step(
            step_id,
            contracts.AGENT_NAMES[step_id],
            self._redactor.redact(input_data),
            contracts.DEPENDENCIES[step_id],
            DETERMINISTIC_MODEL_NAME,
        )
        try:
            output = function(input_data)
            contracts.validate_output(step_id, output)
        except Exception as error:
            self._record_failure(step_id, error)
            raise PipelineExecutionError(step_id, error) from error

        self._recorder.finish_step(
            step_id,
            self._redactor.redact(output),
        )
        self._outputs[step_id] = output
        return output

    async def _run_sync_step_async(
        self,
        step_id: str,
        input_data: Any,
        function: Callable[[Any], dict[str, Any]],
    ) -> dict[str, Any]:
        """Run one deterministic step through an asynchronous branch.

        Input
        step_id str
        Step identifier.

        input_data Any
        Actual business input.

        function callable
        Deterministic business function.

        Output
        dict of str to Any
        Validated step output.
        """

        await asyncio.sleep(0)
        return self._run_sync_step(step_id, input_data, function)

    async def _run_model_step(
        self,
        step_id: str,
        input_data: Any,
    ) -> dict[str, Any]:
        """Execute and record one model backed step.

        Input
        step_id str
        Step identifier.

        input_data Any
        Actual business input.

        Output
        dict of str to Any
        Validated structured model output.
        """

        self._recorder.start_step(
            step_id,
            contracts.AGENT_NAMES[step_id],
            self._redactor.redact(input_data),
            contracts.DEPENDENCIES[step_id],
            self._model_client.model_name,
        )
        try:
            output = await self._model_client.generate(step_id, input_data)
            contracts.validate_output(step_id, output)
        except Exception as error:
            self._record_failure(step_id, error)
            raise PipelineExecutionError(step_id, error) from error

        self._recorder.finish_step(
            step_id,
            self._redactor.redact(output),
        )
        self._outputs[step_id] = output
        return output

    def _record_failure(self, step_id: str, error: Exception) -> None:
        """Record one structured step failure.

        Input
        step_id str
        Started step identifier.

        error Exception
        Execution or validation error.

        Output
        None
        The step is completed with error status.
        """

        self._recorder.fail_step(
            step_id,
            self._redactor.redact(
                {
                    "step_id": step_id,
                    "error_type": type(error).__name__,
                    "message": str(error),
                }
            ),
        )


def _intake_incident(input_data: dict[str, Any]) -> dict[str, Any]:
    """Normalize submitted incident evidence.

    Input
    input_data dict of str to Any
    Complete fixture bundle.

    Output
    dict of str to Any
    Accepted incident packet.
    """

    alert = input_data["alert"]
    return {
        "incident_id": alert["incident_id"],
        "service": alert["service"],
        "environment": alert["environment"],
        "started_at": alert["started_at"],
        "symptom": alert["symptom"],
        "accepted": bool(alert["incident_id"] and alert["service"]),
        "evidence_sources": [
            "logs",
            "metrics",
            "deployments",
            "runbook",
        ],
    }


def _build_branch_inputs(
    fixture: dict[str, Any],
    incident: dict[str, Any],
    plan: dict[str, Any],
) -> dict[str, dict[str, Any]]:
    """Build inputs for parallel evidence branches.

    Input
    fixture dict of str to Any
    Complete source evidence.

    incident dict of str to Any
    Normalized incident packet.

    plan dict of str to Any
    Investigation plan.

    Output
    dict of str to dict
    Branch inputs keyed by step ID.
    """

    return {
        contracts.LOG_ANALYZER: {
            "incident": incident,
            "task": plan["tasks"][0],
            "logs": fixture["logs"],
        },
        contracts.METRICS_ANALYZER: {
            "incident": incident,
            "task": plan["tasks"][1],
            "metrics": fixture["metrics"],
        },
        contracts.DEPLOYMENT_ANALYZER: {
            "incident": incident,
            "task": plan["tasks"][2],
            "deployments": fixture["deployments"],
        },
        contracts.RUNBOOK_LOADER: {
            "incident": incident,
            "task": plan["tasks"][3],
            "runbook": fixture["runbook"],
        },
    }


def _load_runbook(input_data: dict[str, Any]) -> dict[str, Any]:
    """Normalize the supplied runbook.

    Input
    input_data dict of str to Any
    Incident and runbook source data.

    Output
    dict of str to Any
    Loaded operational guidance.
    """

    runbook = input_data["runbook"]
    return {
        "runbook_id": runbook["runbook_id"],
        "cause": runbook["cause"],
        "match_signals": list(runbook["match_signals"]),
        "recommended_actions": list(runbook["recommended_actions"]),
        "verification_steps": list(runbook["verification_steps"]),
        "automatic_execution": runbook["automatic_execution"],
        "loaded": bool(runbook["runbook_id"]),
        "evidence_ids": [runbook["evidence_id"]],
    }


def _build_timeline(input_data: dict[str, Any]) -> dict[str, Any]:
    """Order the deployment, first error, and metric events.

    Input
    input_data dict of str to Any
    Merged evidence bundle.

    Output
    dict of str to Any
    Timeline events and ordering decision.
    """

    events = [
        {
            "timestamp": input_data["deployment_at"],
            "event_type": "deployment",
            "evidence_id": _find_evidence_id(
                input_data["evidence_ids"],
                "deployment-",
            ),
        },
        {
            "timestamp": input_data["first_error_at"],
            "event_type": "error",
            "evidence_id": _find_evidence_id(
                input_data["evidence_ids"],
                "log-",
            ),
        },
        {
            "timestamp": input_data["metric_observed_at"],
            "event_type": "metric",
            "evidence_id": _find_evidence_id(
                input_data["evidence_ids"],
                "metric-",
            ),
        },
    ]
    return {
        "events": events,
        "ordered": (
            events[0]["timestamp"]
            < events[1]["timestamp"]
            < events[2]["timestamp"]
        ),
    }


def _find_evidence_id(evidence_ids: list[str], prefix: str) -> str:
    """Find one evidence ID by source prefix.

    Input
    evidence_ids list of str
    Available evidence identifiers.

    prefix str
    Required source prefix.

    Output
    str
    First matching evidence identifier.
    """

    for evidence_id in evidence_ids:
        if evidence_id.startswith(prefix):
            return evidence_id
    raise ValueError(f"missing evidence ID with prefix {prefix!r}")


def _match_runbook(input_data: dict[str, Any]) -> dict[str, Any]:
    """Match operational guidance to the actual hypothesis.

    Input
    input_data dict of str to Any
    Generated hypothesis and loaded runbook.

    Output
    dict of str to Any
    Applicable actions or a request for more evidence.
    """

    hypothesis = input_data["hypothesis"]
    runbook = input_data["runbook"]
    applicable = (
        runbook["loaded"]
        and runbook["cause"] == hypothesis["cause"]
    )
    if applicable:
        actions = list(runbook["recommended_actions"])
        verification_steps = list(runbook["verification_steps"])
    else:
        actions = ["collect additional evidence"]
        verification_steps = [
            "confirm a supported root cause before remediation"
        ]

    return {
        "runbook_id": runbook["runbook_id"],
        "applicable": applicable,
        "cause": hypothesis["cause"],
        "actions": actions,
        "verification_steps": verification_steps,
    }


def _raise_first_branch_error(results: list[Any]) -> None:
    """Raise the first parallel branch failure after all branches finish.

    Input
    results list of Any
    Values and exceptions returned by asyncio.gather.

    Output
    None
    The function returns when every branch succeeded.
    """

    for result in results:
        if isinstance(result, BaseException):
            raise result


def _require_result(value: Any) -> dict[str, Any]:
    """Require a successful branch result.

    Input
    value Any
    Branch result after error handling.

    Output
    dict of str to Any
    Successful structured output.
    """

    if not isinstance(value, dict):
        raise TypeError("parallel branch result must be an object")
    return value
