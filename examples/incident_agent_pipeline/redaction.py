"""Redact sensitive values before trace recording."""

from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Any


REDACTED = "[REDACTED]"
DEFAULT_SENSITIVE_KEYS = frozenset(
    {
        "api_key",
        "authorization",
        "password",
        "secret",
        "token",
    }
)


@dataclass(frozen=True)
class Redactor:
    """Creates detached trace values with sensitive data removed."""

    secret_values: tuple[str, ...] = ()
    sensitive_keys: frozenset[str] = DEFAULT_SENSITIVE_KEYS

    @classmethod
    def from_environment(cls) -> "Redactor":
        """Create a redactor from process configuration.

        Input
        None

        Output
        Redactor
        Redactor containing explicit values and the current OpenAI key.
        """

        configured = os.environ.get("AGENTTRACE_REDACT_VALUES", "")
        values = [
            value.strip()
            for value in configured.split(",")
            if value.strip()
        ]
        api_key = os.environ.get("OPENAI_API_KEY", "").strip()
        if api_key:
            values.append(api_key)
        return cls(secret_values=tuple(dict.fromkeys(values)))

    def redact(self, value: Any) -> Any:
        """Redact one JSON compatible value.

        Input
        value Any
        Pipeline input, output, or error data.

        Output
        Any
        Detached value with configured secrets removed.
        """

        return self._redact_value(value)

    def _redact_value(self, value: Any) -> Any:
        """Recursively redact one value.

        Input
        value Any
        Current nested value.

        Output
        Any
        Redacted copy.
        """

        if isinstance(value, dict):
            redacted = {}
            for key, nested in value.items():
                normalized_key = str(key).strip().lower()
                if normalized_key in self.sensitive_keys:
                    redacted[key] = REDACTED
                else:
                    redacted[key] = self._redact_value(nested)
            return redacted
        if isinstance(value, list):
            return [self._redact_value(item) for item in value]
        if isinstance(value, tuple):
            return [self._redact_value(item) for item in value]
        if isinstance(value, str):
            result = value
            for secret in self.secret_values:
                result = result.replace(secret, REDACTED)
            return result
        return value
