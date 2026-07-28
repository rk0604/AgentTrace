"""Tests for the Python AgentTrace recorder."""

from __future__ import annotations

import asyncio
import json
import tempfile
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

from agenttrace import TraceRecorder, gather_recorded


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

    def test_writes_utf8_json(self) -> None:
        """Confirm that a trace can be read from a UTF 8 JSON file."""

        recorder = TraceRecorder("file-run", clock=SequenceClock())
        recorder.start_step("source", "Source", {"text": "café"})
        recorder.finish_step("source", {"text": "résumé"})

        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "trace.json"
            recorder.write_trace(path)
            decoded = json.loads(path.read_text(encoding="utf-8"))

        self.assertEqual(decoded["steps"][0]["output"]["text"], "résumé")


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
