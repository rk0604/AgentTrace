"""Language neutral trace recording helpers for Python agent pipelines."""

from __future__ import annotations

import asyncio
import copy
import inspect
import json
import threading
from collections.abc import Awaitable, Callable, Iterable
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


TRACE_VERSION = 1


class TraceRecorder:
    """Records one agent pipeline run as AgentTrace JSON."""

    def __init__(
        self,
        run_id: str,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        """Create a recorder.

        Input
        run_id str
        Stable identifier shared by every recorded step.

        clock callable returning datetime or None
        Optional timestamp source used for deterministic tests.

        Output
        None
        The initialized recorder is stored in the new instance.
        """

        if not run_id or not run_id.strip():
            raise ValueError("start run: run ID is empty")

        self._run_id = run_id
        self._clock = clock or (lambda: datetime.now(timezone.utc))
        self._lock = threading.Lock()
        self._steps: dict[str, dict[str, Any]] = {}
        self._step_order: list[str] = []
        self._finished: set[str] = set()

    def start_step(
        self,
        step_id: str,
        agent_name: str,
        input_data: Any,
        depends_on: Iterable[str] = (),
        model_used: str = "",
    ) -> None:
        """Record the immutable data for one step.

        Input
        step_id str
        Unique identifier for the step.

        agent_name str
        Human readable agent name.

        input_data Any
        JSON serializable step input.

        depends_on iterable of str
        Step IDs that produced the input data.

        model_used str
        Optional model identifier.

        Output
        None
        The started step is added to the recorder.
        """

        if not step_id or not step_id.strip():
            raise ValueError("start step: step ID is empty")
        if not agent_name or not agent_name.strip():
            raise ValueError(f"start step {step_id!r}: agent name is empty")

        encoded_input = _json_clone(input_data, "input")
        dependencies = list(depends_on)
        if any(not dependency or not dependency.strip() for dependency in dependencies):
            raise ValueError(f"start step {step_id!r}: dependency ID is empty")

        with self._lock:
            if step_id in self._steps:
                raise ValueError(f"start step {step_id!r}: duplicate step ID")

            self._steps[step_id] = {
                "run_id": self._run_id,
                "step_id": step_id,
                "agent_name": agent_name,
                "depends_on": dependencies,
                "input": encoded_input,
                "output": None,
                "model_used": model_used,
                "timestamp": _format_timestamp(self._clock()),
                "status": "running",
            }
            self._step_order.append(step_id)

    def finish_step(
        self,
        step_id: str,
        output: Any,
        confidence: float | None = None,
    ) -> None:
        """Record a successful step output.

        Input
        step_id str
        Identifier of a started step.

        output Any
        JSON serializable step output.

        confidence float or None
        Optional confidence from zero through one.

        Output
        None
        The step is marked complete with status ok.
        """

        self._complete_step(step_id, output, confidence, "ok")

    def fail_step(
        self,
        step_id: str,
        output: Any,
        status: str = "error",
    ) -> None:
        """Record an unsuccessful step output.

        Input
        step_id str
        Identifier of a started step.

        output Any
        JSON serializable failure details or partial output.

        status str
        Non empty failure status.

        Output
        None
        The step is marked complete with the supplied status.
        """

        if not status or not status.strip() or status in {"ok", "running"}:
            raise ValueError(f"fail step {step_id!r}: status must describe a failure")

        self._complete_step(step_id, output, None, status)

    def run_step(
        self,
        step_id: str,
        agent_name: str,
        input_data: Any,
        function: Callable[[Any], Any],
        depends_on: Iterable[str] = (),
        model_used: str = "",
        confidence: float | None = None,
    ) -> Any:
        """Execute and record one synchronous pipeline function.

        Input
        step_id str
        Unique identifier for the step.

        agent_name str
        Human readable agent name.

        input_data Any
        JSON serializable function input.

        function callable
        Synchronous function receiving input_data and returning JSON data.

        depends_on iterable of str
        Step IDs that produced the input data.

        model_used str
        Optional model identifier.

        confidence float or None
        Optional confidence recorded on success.

        Output
        Any
        Function result after it has been recorded.
        """

        self.start_step(step_id, agent_name, input_data, depends_on, model_used)
        try:
            result = function(input_data)
            if inspect.isawaitable(result):
                raise TypeError("run_step received an asynchronous function")
        except Exception as error:
            self.fail_step(
                step_id,
                {"error_type": type(error).__name__, "message": str(error)},
            )
            raise

        self.finish_step(step_id, result, confidence)
        return result

    async def run_step_async(
        self,
        step_id: str,
        agent_name: str,
        input_data: Any,
        function: Callable[[Any], Awaitable[Any]],
        depends_on: Iterable[str] = (),
        model_used: str = "",
        confidence: float | None = None,
    ) -> Any:
        """Execute and record one asynchronous pipeline function.

        Input
        step_id str
        Unique identifier for the step.

        agent_name str
        Human readable agent name.

        input_data Any
        JSON serializable function input.

        function callable returning Awaitable
        Asynchronous function receiving input_data.

        depends_on iterable of str
        Step IDs that produced the input data.

        model_used str
        Optional model identifier.

        confidence float or None
        Optional confidence recorded on success.

        Output
        Any
        Awaited function result after it has been recorded.
        """

        self.start_step(step_id, agent_name, input_data, depends_on, model_used)
        try:
            result = await function(input_data)
        except Exception as error:
            self.fail_step(
                step_id,
                {"error_type": type(error).__name__, "message": str(error)},
            )
            raise

        self.finish_step(step_id, result, confidence)
        return result

    def trace(self) -> dict[str, Any]:
        """Return a validated copy of the completed trace.

        Input
        None

        Output
        dict of str to Any
        Versioned AgentTrace document ordered by step start order.
        """

        with self._lock:
            unfinished = [
                step_id
                for step_id in self._step_order
                if step_id not in self._finished
            ]
            if unfinished:
                raise ValueError(f"build trace: step {unfinished[0]!r} is unfinished")

            steps = [
                copy.deepcopy(self._steps[step_id])
                for step_id in self._step_order
            ]

        _validate_graph(steps)
        return {
            "version": TRACE_VERSION,
            "run_id": self._run_id,
            "steps": steps,
        }

    def write_trace(self, path: str | Path) -> None:
        """Write the completed trace to a JSON file.

        Input
        path str or Path
        Destination path for the trace.

        Output
        None
        A formatted UTF 8 JSON file is created.
        """

        destination = Path(path)
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(
            json.dumps(self.trace(), indent=2, ensure_ascii=False) + "\n",
            encoding="utf-8",
        )

    def _complete_step(
        self,
        step_id: str,
        output: Any,
        confidence: float | None,
        status: str,
    ) -> None:
        """Store the final state for one recorded step.

        Input
        step_id str
        Identifier of the started step.

        output Any
        JSON serializable final output.

        confidence float or None
        Optional confidence from zero through one.

        status str
        Final step status.

        Output
        None
        The matching step is completed.
        """

        encoded_output = _json_clone(output, "output")
        if confidence is not None and not 0 <= confidence <= 1:
            raise ValueError(
                f"complete step {step_id!r}: confidence must be between zero and one"
            )

        with self._lock:
            if step_id not in self._steps:
                raise ValueError(f"complete step {step_id!r}: step was not started")
            if step_id in self._finished:
                raise ValueError(f"complete step {step_id!r}: step is already finished")

            step = self._steps[step_id]
            step["output"] = encoded_output
            if confidence is not None:
                step["confidence"] = confidence
            step["status"] = status
            self._finished.add(step_id)


def _json_clone(value: Any, field_name: str) -> Any:
    """Validate and clone a JSON value.

    Input
    value Any
    Candidate JSON value.

    field_name str
    Field name used in validation errors.

    Output
    Any
    Detached value containing only JSON compatible types.
    """

    try:
        return json.loads(json.dumps(value, ensure_ascii=False))
    except (TypeError, ValueError) as error:
        raise ValueError(f"encode {field_name} JSON: {error}") from error


def _format_timestamp(value: datetime) -> str:
    """Format a timestamp for the Go trace decoder.

    Input
    value datetime
    Timestamp from the configured clock.

    Output
    str
    UTC timestamp in RFC 3339 compatible form.
    """

    if value.tzinfo is None:
        value = value.replace(tzinfo=timezone.utc)
    value = value.astimezone(timezone.utc)
    return value.isoformat().replace("+00:00", "Z")


def _validate_graph(steps: list[dict[str, Any]]) -> None:
    """Validate dependencies and detect cycles.

    Input
    steps list of dict
    Completed step documents.

    Output
    None
    The function returns after a valid graph is confirmed.
    """

    step_ids = {step["step_id"] for step in steps}
    indegree = {step_id: 0 for step_id in step_ids}
    dependents: dict[str, list[str]] = {step_id: [] for step_id in step_ids}

    for step in steps:
        for dependency in step["depends_on"]:
            if dependency not in step_ids:
                raise ValueError(
                    f"step {step['step_id']!r} depends on unknown step {dependency!r}"
                )
            indegree[step["step_id"]] += 1
            dependents[dependency].append(step["step_id"])

    ready = [
        step["step_id"]
        for step in steps
        if indegree[step["step_id"]] == 0
    ]
    checked = 0
    while ready:
        step_id = ready.pop(0)
        checked += 1
        for dependent in dependents[step_id]:
            indegree[dependent] -= 1
            if indegree[dependent] == 0:
                ready.append(dependent)

    if checked != len(steps):
        raise ValueError("trace contains a dependency cycle")


async def gather_recorded(*awaitables: Awaitable[Any]) -> list[Any]:
    """Run independent recorded operations concurrently.

    Input
    awaitables Awaitable values
    Independent recorded pipeline operations.

    Output
    list of Any
    Results in the same order as the supplied awaitables.
    """

    return list(await asyncio.gather(*awaitables))
