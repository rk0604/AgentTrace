"""Tests for incident step output contracts."""

from __future__ import annotations

import copy
import json
import sys
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(REPOSITORY_ROOT))

from examples.incident_agent_pipeline import contracts


class ContractTests(unittest.TestCase):
    """Validate strict local output checking."""

    def test_accepts_every_healthy_replay_output(self) -> None:
        """Confirm that stored healthy model outputs satisfy their contracts."""

        path = (
            REPOSITORY_ROOT
            / "examples"
            / "incident_agent_pipeline"
            / "replays"
            / "healthy.json"
        )
        document = json.loads(path.read_text(encoding="utf-8"))

        for step_id, exchange in document["steps"].items():
            with self.subTest(step_id=step_id):
                contracts.validate_model_input(step_id, exchange["input"])
                contracts.validate_output(step_id, exchange["output"])

    def test_rejects_missing_and_unknown_fields(self) -> None:
        """Confirm that strict contracts reject incomplete or expanded output."""

        valid = {
            "incident_id": "INC-1",
            "service": "checkout-api",
            "tasks": {
                contracts.LOG_ANALYZER: "analyze_logs",
                contracts.METRICS_ANALYZER: "analyze_metrics",
                contracts.DEPLOYMENT_ANALYZER: "analyze_deployments",
                contracts.RUNBOOK_LOADER: "load_runbook",
            },
            "investigation_goal": "find cause",
        }

        missing = copy.deepcopy(valid)
        del missing["service"]
        with self.assertRaisesRegex(contracts.ContractError, "required property"):
            contracts.validate_output(contracts.INVESTIGATION_PLANNER, missing)

        expanded = copy.deepcopy(valid)
        expanded["unexpected"] = True
        with self.assertRaisesRegex(
            contracts.ContractError,
            "Additional properties",
        ):
            contracts.validate_output(contracts.INVESTIGATION_PLANNER, expanded)

    def test_rejects_boolean_as_number(self) -> None:
        """Confirm that Python Boolean values cannot satisfy numeric fields."""

        output = {
            "observed_at": "2026-07-20T10:01:00Z",
            "p95_latency_ms": True,
            "latency_threshold_ms": 1000,
            "error_rate": 0.1,
            "connection_pool_utilization": 1.0,
            "state": "critical",
            "evidence_ids": ["metric-001"],
            "summary": "critical",
        }

        with self.assertRaisesRegex(contracts.ContractError, "integer"):
            contracts.validate_output(contracts.METRICS_ANALYZER, output)

    def test_rejects_model_input_with_wrong_dependency_shape(self) -> None:
        """Confirm that model inputs must match their dependency contracts."""

        input_data = {
            "logs": {},
            "metrics": {},
            "deployment": {},
        }

        with self.assertRaisesRegex(
            contracts.ContractError,
            "required property",
        ):
            contracts.validate_model_input(
                contracts.EVIDENCE_MERGER,
                input_data,
            )


if __name__ == "__main__":
    unittest.main()
