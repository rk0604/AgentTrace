"""Tests for the Python AgentTrace recorder."""

from __future__ import annotations

import asyncio
import json
import math
import os
import tempfile
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

from agenttrace import (
    MAX_TRACE_STEPS,
    TraceRecorder,
    gather_recorded,
    validate_trace,
)


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]


class SequenceClock:
    """Returns deterministic timestamps in sequence."""

    def __init__(self) -> None:
        """Create a clock beginning at a fixed UTC timestamp."""

        self._next = datetime(2026, 7, 28, 12, 0, tzinfo=timezone.utc)

    def __call__(self) -> datetime:
        """Return the next timestamp.

        Input
        None

        Output
        datetime
        Current deterministic timestamp before advancing one second.
        """

        current = self._next
        self._next += timedelta(seconds=1)
        return current


class TraceRecorderTests(unittest.TestCase):
    """Covers synchronous recording and validation."""

    def test_records_dependency_graph(self) -> None:
        """Confirm that completed steps produce a versioned trace."""

        recorder = TraceRecorder("python-run", clock=SequenceClock())
        source = recorder.run_step(
            "source",
            "Source",
            {"document": "café input"},
            lambda value: {"text": value["document"]},
            model_used="python-function",
            confidence=0.9,
        )
        recorder.run_step(
            "consumer",
            "Consumer",
            source,
            lambda value: {"length": len(value["text"])},
            depends_on=["source"],
            model_used="python-function",
        )

        trace = recorder.trace()

        self.assertEqual(trace["version"], 1)
        self.assertEqual(trace["run_id"], "python-run")
        self.assertEqual(trace["steps"][1]["depends_on"], ["source"])
        self.assertEqual(trace["steps"][0]["input"]["document"], "café input")

    def test_records_function_failure(self) -> None:
        """Confirm that an exception becomes structured step output."""

        recorder = TraceRecorder("failed-run", clock=SequenceClock())

        def fail(_: Any) -> Any:
            raise RuntimeError("provider unavailable")

        with self.assertRaisesRegex(RuntimeError, "provider unavailable"):
            recorder.run_step("agent", "Agent", {}, fail)

        trace = recorder.trace()
        self.assertEqual(trace["steps"][0]["status"], "error")
        self.assertEqual(trace["steps"][0]["output"]["error_type"], "RuntimeError")

    def test_rejects_unknown_dependency(self) -> None:
        """Confirm that graph validation rejects missing steps."""

        recorder = TraceRecorder("invalid-run", clock=SequenceClock())
        recorder.start_step("consumer", "Consumer", {}, depends_on=["missing"])
        recorder.finish_step("consumer", {})

        with self.assertRaisesRegex(ValueError, "depends on unknown step"):
            recorder.trace()

    def test_rejects_empty_trace(self) -> None:
        """Confirm that a completed trace must contain one step."""

        recorder = TraceRecorder("empty-run", clock=SequenceClock())

        with self.assertRaisesRegex(ValueError, "trace has no steps"):
            recorder.trace()

    def test_rejects_repeated_dependency(self) -> None:
        """Confirm that one dependency cannot be declared twice."""

        recorder = TraceRecorder("repeated-run", clock=SequenceClock())
        recorder.start_step("source", "Source", {})
        recorder.finish_step("source", {})

        with self.assertRaisesRegex(ValueError, "dependency ID is repeated"):
            recorder.start_step(
                "consumer",
                "Consumer",
                {},
                depends_on=["source", "source"],
            )

    def test_rejects_nonfinite_json_and_confidence(self) -> None:
        """Confirm that recorder output remains strict JSON."""

        recorder = TraceRecorder("nonfinite-run", clock=SequenceClock())
        with self.assertRaisesRegex(ValueError, "Out of range float"):
            recorder.start_step("source", "Source", {"value": math.nan})

        recorder.start_step("source", "Source", {})
        with self.assertRaisesRegex(ValueError, "confidence must be between"):
            recorder.finish_step("source", {}, confidence=math.nan)

    def test_rejects_trace_over_step_limit(self) -> None:
        """Confirm that Python enforces the shared graph size limit."""

        trace = {
            "version": 1,
            "run_id": "oversized-run",
            "steps": [{}] * (MAX_TRACE_STEPS + 1),
        }

        with self.assertRaisesRegex(ValueError, "exceeds limit"):
            validate_trace(trace)

    def test_writes_utf8_json(self) -> None:
        """Confirm that a trace can be read from a UTF 8 JSON file."""

        recorder = TraceRecorder("file-run", clock=SequenceClock())
        recorder.start_step("source", "Source", {"text": "café"})
        recorder.finish_step("source", {"text": "résumé"})

        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "trace.json"
            recorder.write_trace(path)
            raw = path.read_bytes()
            decoded = json.loads(path.read_text(encoding="utf-8"))
            if os.name != "nt":
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)

        self.assertEqual(decoded["steps"][0]["output"]["text"], "résumé")
        self.assertNotIn(b"\r\n", raw)


class TraceContractTests(unittest.TestCase):
    """Runs shared language neutral trace contract fixtures."""

    def test_shared_trace_contract_cases(self) -> None:
        """Confirm that Python agrees with the canonical fixture outcomes."""

        path = (
            REPOSITORY_ROOT
            / "testdata"
            / "trace-contract"
            / "cases.json"
        )
        cases = json.loads(path.read_text(encoding="utf-8"))

        for case in cases:
            with self.subTest(case=case["name"]):
                if case["valid"]:
                    validate_trace(case["trace"])
                    continue

                with self.assertRaises(ValueError):
                    validate_trace(case["trace"])


class AsyncTraceRecorderTests(unittest.IsolatedAsyncioTestCase):
    """Covers asynchronous and parallel recording."""

    async def test_records_parallel_async_steps(self) -> None:
        """Confirm that independent asynchronous branches are captured."""

        recorder = TraceRecorder("async-run", clock=SequenceClock())

        async def branch(value: dict[str, str]) -> dict[str, str]:
            await asyncio.sleep(0)
            return {"branch": value["branch"]}

        results = await gather_recorded(
            recorder.run_step_async(
                "left",
                "Left",
                {"branch": "left"},
                branch,
            ),
            recorder.run_step_async(
                "right",
                "Right",
                {"branch": "right"},
                branch,
            ),
        )

        self.assertEqual(results, [{"branch": "left"}, {"branch": "right"}])
        self.assertEqual(len(recorder.trace()["steps"]), 2)


if __name__ == "__main__":
    unittest.main()
