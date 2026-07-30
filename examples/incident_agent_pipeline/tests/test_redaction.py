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

    def test_redacts_common_secret_key_variants(self) -> None:
        """Confirm that separators and camel case cannot bypass redaction."""

        source = {
            "x-api-key": "first",
            "clientSecret": "second",
            "database_password": "third",
            "sessionToken": "fourth",
            "safe_key": "visible",
        }

        redacted = Redactor().redact(source)

        self.assertEqual(redacted["x-api-key"], REDACTED)
        self.assertEqual(redacted["clientSecret"], REDACTED)
        self.assertEqual(redacted["database_password"], REDACTED)
        self.assertEqual(redacted["sessionToken"], REDACTED)
        self.assertEqual(redacted["safe_key"], "visible")

    def test_redacts_longest_literal_secret_first(self) -> None:
        """Confirm that overlapping secrets do not expose a suffix."""

        redactor = Redactor(secret_values=("private", "private-value"))

        redacted = redactor.redact("token private-value")

        self.assertEqual(redacted, f"token {REDACTED}")


if __name__ == "__main__":
    unittest.main()
