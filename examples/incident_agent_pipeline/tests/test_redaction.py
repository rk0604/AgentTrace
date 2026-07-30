"""Tests for trace redaction."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(REPOSITORY_ROOT))

from examples.incident_agent_pipeline.redaction import REDACTED, Redactor


class RedactionTests(unittest.TestCase):
    """Validate sensitive trace value removal."""

    def test_redacts_sensitive_keys_and_literal_values(self) -> None:
        """Confirm that nested secrets are removed from detached trace data."""

        source = {
            "authorization": "Bearer private-value",
            "nested": {
                "message": "request used private-value",
                "safe": "visible",
            },
        }
        redacted = Redactor(secret_values=("private-value",)).redact(source)

        self.assertEqual(redacted["authorization"], REDACTED)
        self.assertEqual(redacted["nested"]["message"], f"request used {REDACTED}")
        self.assertEqual(redacted["nested"]["safe"], "visible")
        self.assertEqual(source["authorization"], "Bearer private-value")


if __name__ == "__main__":
    unittest.main()
