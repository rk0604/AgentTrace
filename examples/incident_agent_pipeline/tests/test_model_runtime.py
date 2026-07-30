"""Tests for replay, failure, and live model clients."""

from __future__ import annotations

import json
import sys
import time
import unittest
from pathlib import Path
from types import SimpleNamespace
from typing import Any


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(REPOSITORY_ROOT))

from examples.incident_agent_pipeline import contracts
from examples.incident_agent_pipeline.model_runtime import (
    FaultInjectingModelClient,
    ModelRuntimeError,
    OpenAIModelClient,
    ReplayModelClient,
    failure_override,
)


class FakeResponses:
    """Returns configured output text from responses.create."""

    def __init__(
        self,
        output: dict[str, Any],
        failures: int = 0,
        delay_seconds: float = 0,
    ) -> None:
        """Create a fake responses resource.

        Input
        output dict of str to Any
        Structured output returned after failures.

        failures int
        Number of calls that raise before success.

        delay_seconds float
        Blocking delay applied to every request.

        Output
        None
        The fake response state is stored.
        """

        self.output = output
        self.failures = failures
        self.delay_seconds = delay_seconds
        self.calls: list[dict[str, Any]] = []

    def create(self, **values: Any) -> SimpleNamespace:
        """Return one fake Responses API result.

        Input
        values Any
        Request arguments.

        Output
        SimpleNamespace
        Object containing output_text.
        """

        self.calls.append(values)
        if self.delay_seconds:
            time.sleep(self.delay_seconds)
        if len(self.calls) <= self.failures:
            raise RuntimeError("temporary provider failure")
        return SimpleNamespace(output_text=json.dumps(self.output))


class FakeOpenAI:
    """Exposes a fake responses resource."""

    def __init__(self, responses: FakeResponses) -> None:
        """Create a compatible fake client."""

        self.responses = responses


class ModelRuntimeTests(unittest.IsolatedAsyncioTestCase):
    """Validate model execution behavior."""

    async def test_replay_modes_cover_every_model_step(self) -> None:
        """Confirm that healthy and failure transcripts are complete."""

        for failure in ("none", "metrics", "deployment"):
            client = ReplayModelClient.from_directory(failure)
            for step_id in contracts.MODEL_STEP_IDS:
                output = await client.generate(step_id, {})
                contracts.validate_output(step_id, output)

    async def test_fault_client_only_overrides_selected_step(self) -> None:
        """Confirm that live fault injection leaves other calls unchanged."""

        base = ReplayModelClient.from_directory("none")
        target = failure_override("metrics")
        self.assertIsNotNone(target)
        step_id, output = target or ("", {})
        client = FaultInjectingModelClient(base, step_id, output)

        failed = await client.generate(contracts.METRICS_ANALYZER, {})
        healthy = await base.generate(contracts.METRICS_ANALYZER, {})
        logs = await client.generate(contracts.LOG_ANALYZER, {})

        self.assertEqual(failed["state"], "normal")
        self.assertEqual(healthy["state"], "critical")
        self.assertEqual(logs["primary_error_code"], "DB_CONNECTION_TIMEOUT")

    async def test_openai_client_uses_strict_structured_output(self) -> None:
        """Confirm that live requests carry the step JSON schema."""

        output = await ReplayModelClient.from_directory("none").generate(
            contracts.LOG_ANALYZER,
            {},
        )
        responses = FakeResponses(output)
        client = OpenAIModelClient(
            model="test-model",
            client=FakeOpenAI(responses),
            max_attempts=1,
        )

        result = await client.generate(contracts.LOG_ANALYZER, {"logs": []})

        self.assertEqual(result, output)
        request = responses.calls[0]
        self.assertEqual(request["model"], "test-model")
        self.assertTrue(request["text"]["format"]["strict"])
        self.assertEqual(
            request["text"]["format"]["schema"],
            contracts.OUTPUT_SCHEMAS[contracts.LOG_ANALYZER],
        )

    async def test_openai_client_retries_provider_failure(self) -> None:
        """Confirm that a transient provider error receives one retry."""

        output = await ReplayModelClient.from_directory("none").generate(
            contracts.INVESTIGATION_PLANNER,
            {},
        )
        responses = FakeResponses(output, failures=1)
        client = OpenAIModelClient(
            model="test-model",
            client=FakeOpenAI(responses),
            max_attempts=2,
            retry_delay_seconds=0,
        )

        result = await client.generate(contracts.INVESTIGATION_PLANNER, {})

        self.assertEqual(result, output)
        self.assertEqual(len(responses.calls), 2)

    async def test_openai_client_rejects_malformed_output(self) -> None:
        """Confirm that invalid provider JSON becomes a runtime error."""

        responses = FakeResponses({})

        def malformed_create(**values: Any) -> SimpleNamespace:
            responses.calls.append(values)
            return SimpleNamespace(output_text="{not-json")

        responses.create = malformed_create  # type: ignore[method-assign]
        client = OpenAIModelClient(
            model="test-model",
            client=FakeOpenAI(responses),
            max_attempts=1,
        )

        with self.assertRaisesRegex(ModelRuntimeError, "failed after 1 attempts"):
            await client.generate(contracts.LOG_ANALYZER, {})

    async def test_openai_client_times_out(self) -> None:
        """Confirm that a slow provider request respects its deadline."""

        output = await ReplayModelClient.from_directory("none").generate(
            contracts.LOG_ANALYZER,
            {},
        )
        responses = FakeResponses(output, delay_seconds=0.05)
        client = OpenAIModelClient(
            model="test-model",
            client=FakeOpenAI(responses),
            max_attempts=1,
            timeout_seconds=0.001,
        )

        with self.assertRaisesRegex(ModelRuntimeError, "failed after 1 attempts"):
            await client.generate(contracts.LOG_ANALYZER, {})


if __name__ == "__main__":
    unittest.main()
