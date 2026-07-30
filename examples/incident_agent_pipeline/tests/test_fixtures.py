"""Tests for incident fixture loading."""

from __future__ import annotations

import copy
import sys
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(REPOSITORY_ROOT))

from examples.incident_agent_pipeline.fixtures import (
    load_fixture_bundle,
    validate_fixture_bundle,
)


class FixtureTests(unittest.TestCase):
    """Validate incident source evidence."""

    def test_loads_consistent_fixture_bundle(self) -> None:
        """Confirm that every required evidence source is present."""

        bundle = load_fixture_bundle()

        self.assertEqual(bundle["alert"]["incident_id"], "INC-2026-072")
        self.assertEqual(len(bundle["logs"]), 4)
        self.assertEqual(len(bundle["deployments"]), 2)
        self.assertNotIn("expected", bundle)

    def test_rejects_duplicate_evidence_identifiers(self) -> None:
        """Confirm that source evidence IDs remain unambiguous."""

        bundle = copy.deepcopy(load_fixture_bundle())
        bundle["metrics"]["evidence_id"] = bundle["logs"][0]["evidence_id"]

        with self.assertRaisesRegex(ValueError, "evidence IDs must be unique"):
            validate_fixture_bundle(bundle)

    def test_rejects_fixture_without_pool_regression(self) -> None:
        """Confirm that the business scenario contains its intended change."""

        bundle = copy.deepcopy(load_fixture_bundle())
        for deployment in bundle["deployments"]:
            for change in deployment["changes"]:
                change["after"] = change["before"]

        with self.assertRaisesRegex(ValueError, "pool regression"):
            validate_fixture_bundle(bundle)


if __name__ == "__main__":
    unittest.main()
