"""Tests for the recorded incident investigation pipeline."""

from __future__ import annotations

import asyncio
import copy
import json
import os
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from io import StringIO
from pathlib import Path
from typing import Any


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(REPOSITORY_ROOT))

from examples.incident_agent_pipeline import contracts
from examples.incident_agent_pipeline.cli import main
from examples.incident_agent_pipeline.fixtures import load_fixture_bundle
from examples.incident_agent_pipeline.model_runtime import (
    ModelRuntimeError,
    ReplayModelClient,
)
from examples.incident_agent_pipeline.pipeline import (
    IncidentPipeline,
    PipelineExecutionError,
    SequenceClock,
)
from examples.incident_agent_pipeline.redaction import REDACTED, Redactor


class ConcurrencyTrackingClient:
    """Tracks simultaneous model calls while returning replay data."""

    def __init__(self) -> None:
        """Create a concurrency tracking replay client."""

        self._replay = ReplayModelClient.from_directory("none")
        self.active = 0
        self.maximum_active = 0

    @property
    def model_name(self) -> str:
        """Return the trace model label."""

        return "replay/concurrency"

    async def generate(
        self,
        step_id: str,
        input_data: Any,
    ) -> dict[str, Any]:
        """Return one response after an asynchronous scheduling point."""

        self.active += 1
        self.maximum_active = max(self.maximum_active, self.active)
        try:
            await asyncio.sleep(0.01)
            return await self._replay.generate(step_id, input_data)
        finally:
            self.active -= 1


class FailingModelClient:
    """Raises for one selected model step."""

    def __init__(self, failed_step_id: str) -> None:
        """Create a selectively failing replay client."""

        self._failed_step_id = failed_step_id
        self._replay = ReplayModelClient.from_directory("none")

    @property
    def model_name(self) -> str:
        """Return the trace model label."""

        return "replay/provider-failure"

    async def generate(
        self,
        step_id: str,
        input_data: Any,
    ) -> dict[str, Any]:
        """Raise for the selected step or return replay output."""

        await asyncio.sleep(0)
        if step_id == self._failed_step_id:
            raise ModelRuntimeError("provider timeout")
        return await self._replay.generate(step_id, input_data)


class PipelineTests(unittest.IsolatedAsyncioTestCase):
    """Validate complete and partial pipeline execution."""

    async def test_replay_modes_produce_complete_consistent_traces(self) -> None:
        """Confirm that healthy and injected runs preserve actual data flow."""

        expected_causes = {
            "none": "database_connection_pool_regression",
            "metrics": "application_error",
            "deployment": "traffic_spike",
        }

        for failure, expected_cause in expected_causes.items():
            with self.subTest(failure=failure):
                pipeline = IncidentPipeline(
                    run_id=f"test-{failure}",
                    fixture_bundle=load_fixture_bundle(),
                    model_client=ReplayModelClient.from_directory(failure),
                    clock=SequenceClock(),
                )

                summary = await pipeline.run()
                trace = pipeline.trace()

                self.assertEqual(
                    [step["step_id"] for step in trace["steps"]],
                    list(contracts.STEP_IDS),
                )
                self.assertTrue(
                    all(step["status"] == "ok" for step in trace["steps"])
                )
                for step in trace["steps"]:
                    self.assertEqual(
                        step["depends_on"],
                        list(contracts.DEPENDENCIES[step["step_id"]]),
                    )

                evidence = pipeline.output(contracts.EVIDENCE_MERGER)
                metrics = pipeline.output(contracts.METRICS_ANALYZER)
                deployment = pipeline.output(contracts.DEPLOYMENT_ANALYZER)
                hypothesis = pipeline.output(contracts.HYPOTHESIS_GENERATOR)
                remediation = pipeline.output(contracts.REMEDIATION_PLANNER)

                self.assertEqual(evidence["metric_state"], metrics["state"])
                self.assertEqual(
                    evidence["deployment_id"],
                    deployment["deployment_id"],
                )
                self.assertEqual(hypothesis["cause"], expected_cause)
                self.assertEqual(remediation["cause"], hypothesis["cause"])
                self.assertEqual(summary["root_cause"], expected_cause)
                self.assertEqual(
                    summary["recommended_actions"],
                    remediation["actions"],
                )

    async def test_analysis_branches_execute_concurrently(self) -> None:
        """Confirm that independent model analyzers overlap execution."""

        client = ConcurrencyTrackingClient()
        pipeline = IncidentPipeline(
            run_id="test-concurrency",
            fixture_bundle=load_fixture_bundle(),
            model_client=client,
            clock=SequenceClock(),
        )

        await pipeline.run()

        self.assertGreaterEqual(client.maximum_active, 3)

    async def test_provider_failure_writes_valid_partial_trace(self) -> None:
        """Confirm that every started branch finishes before failure returns."""

        pipeline = IncidentPipeline(
            run_id="test-provider-failure",
            fixture_bundle=load_fixture_bundle(),
            model_client=FailingModelClient(contracts.METRICS_ANALYZER),
            clock=SequenceClock(),
        )

        with self.assertRaisesRegex(
            PipelineExecutionError,
            contracts.METRICS_ANALYZER,
        ):
            await pipeline.run()

        trace = pipeline.trace()
        steps = {step["step_id"]: step for step in trace["steps"]}
        self.assertEqual(len(steps), 6)
        self.assertEqual(steps[contracts.METRICS_ANALYZER]["status"], "error")
        self.assertEqual(steps[contracts.LOG_ANALYZER]["status"], "ok")
        self.assertEqual(steps[contracts.DEPLOYMENT_ANALYZER]["status"], "ok")
        self.assertEqual(steps[contracts.RUNBOOK_LOADER]["status"], "ok")

    async def test_trace_redacts_source_and_error_secrets(self) -> None:
        """Confirm that configured secrets never enter recorded JSON."""

        fixture = copy.deepcopy(load_fixture_bundle())
        fixture["alert"]["token"] = "private-value"
        fixture["alert"]["note"] = "uses private-value"
        pipeline = IncidentPipeline(
            run_id="test-redaction",
            fixture_bundle=fixture,
            model_client=ReplayModelClient.from_directory("none"),
            redactor=Redactor(secret_values=("private-value",)),
            clock=SequenceClock(),
        )

        await pipeline.run()
        encoded = json.dumps(pipeline.trace())

        self.assertNotIn("private-value", encoded)
        self.assertIn(REDACTED, encoded)


class CommandTests(unittest.TestCase):
    """Validate the repository level Python command."""

    def test_replay_command_writes_trace_and_summary(self) -> None:
        """Confirm that a developer can run the pipeline without credentials."""

        with tempfile.TemporaryDirectory() as directory:
            trace_path = Path(directory) / "trace.json"
            summary_path = Path(directory) / "summary.json"
            output = StringIO()
            errors = StringIO()

            with redirect_stdout(output), redirect_stderr(errors):
                status = main(
                    [
                        "--mode",
                        "replay",
                        "--failure",
                        "none",
                        "--trace-output",
                        str(trace_path),
                        "--summary-output",
                        str(summary_path),
                    ]
                )

            trace = json.loads(trace_path.read_text(encoding="utf-8"))
            summary = json.loads(summary_path.read_text(encoding="utf-8"))
            self.assertEqual(status, 0)
            self.assertEqual(errors.getvalue(), "")
            self.assertEqual(len(trace["steps"]), 13)
            self.assertEqual(
                summary["root_cause"],
                "database_connection_pool_regression",
            )
            if os.name != "nt":
                self.assertEqual(
                    trace_path.stat().st_mode & 0o777,
                    0o600,
                )
                self.assertEqual(
                    summary_path.stat().st_mode & 0o777,
                    0o600,
                )


if __name__ == "__main__":
    unittest.main()
