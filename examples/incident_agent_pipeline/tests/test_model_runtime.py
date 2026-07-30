"""Tests for replay, failure, and live model clients."""

from __future__ import annotations

import asyncio
import json
import sys
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


def replay_exchange(
    failure: str,
    step_id: str,
) -> dict[str, Any]:
    """Load one stored replay exchange.

    Input
    failure str
    Replay failure mode.

    step_id str
    Model step identifier.

    Output
    dict of str to Any
    Stored input and output for the selected step.
    """

    file_name = (
        "healthy.json"
        if failure == "none"
        else f"{failure}_failure.json"
    )
    path = (
        REPOSITORY_ROOT
        / "examples"
        / "incident_agent_pipeline"
        / "replays"
        / file_name
    )
    document = json.loads(path.read_text(encoding="utf-8"))
    return document["steps"][step_id]


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
        Number of transient calls that raise before success.

        delay_seconds float
        Async delay applied to every request.

        Output
        None
        The fake response state is stored.
        """

        self.output = output
        self.failures = failures
        self.delay_seconds = delay_seconds
        self.calls: list[dict[str, Any]] = []
        self.active_requests = 0
        self.cancelled_requests = 0

    async def create(self, **values: Any) -> SimpleNamespace:
        """Return one fake Responses API result.

        Input
        values Any
        Request arguments.

        Output
        SimpleNamespace
        Object containing output_text.
        """

        self.calls.append(values)
        self.active_requests += 1
        try:
            if self.delay_seconds:
                await asyncio.sleep(self.delay_seconds)
            if len(self.calls) <= self.failures:
                raise FakeProviderError(503)
            return SimpleNamespace(output_text=json.dumps(self.output))
        except asyncio.CancelledError:
            self.cancelled_requests += 1
            raise
        finally:
            self.active_requests -= 1


class FakeProviderError(RuntimeError):
    """Represents one provider HTTP failure."""

    def __init__(
        self,
        status_code: int,
        message: str | None = None,
    ) -> None:
        """Create a provider failure.

        Input
        status_code int
        HTTP status code exposed by the provider.

        message str or None
        Optional private response text used by safety tests.

        Output
        None
        The provider error is initialized.
        """

        super().__init__(message or f"provider returned {status_code}")
        self.status_code = status_code


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
                exchange = replay_exchange(failure, step_id)
                output = await client.generate(step_id, exchange["input"])
                contracts.validate_output(step_id, output)

    async def test_replay_rejects_input_drift(self) -> None:
        """Confirm that stored outputs cannot hide changed pipeline wiring."""

        client = ReplayModelClient.from_directory("none")

        with self.assertRaisesRegex(ModelRuntimeError, "input mismatch"):
            await client.generate(
                contracts.INVESTIGATION_PLANNER,
                {
                    "incident_id": "changed",
                    "service": "checkout-api",
                    "environment": "production",
                    "started_at": "2026-07-20T10:00:00Z",
                    "symptom": "latency",
                    "accepted": True,
                    "evidence_sources": [
                        "logs",
                        "metrics",
                        "deployments",
                        "runbook",
                    ],
                },
            )

    async def test_fault_client_only_overrides_selected_step(self) -> None:
        """Confirm that live fault injection leaves other calls unchanged."""

        base = ReplayModelClient.from_directory("none")
        target = failure_override("metrics")
        self.assertIsNotNone(target)
        step_id, output = target or ("", {})
        client = FaultInjectingModelClient(base, step_id, output)
        metrics_input = replay_exchange(
            "none",
            contracts.METRICS_ANALYZER,
        )["input"]
        logs_input = replay_exchange(
            "none",
            contracts.LOG_ANALYZER,
        )["input"]

        failed = await client.generate(
            contracts.METRICS_ANALYZER,
            metrics_input,
        )
        healthy = await base.generate(
            contracts.METRICS_ANALYZER,
            metrics_input,
        )
        logs = await client.generate(contracts.LOG_ANALYZER, logs_input)

        self.assertEqual(failed["state"], "normal")
        self.assertEqual(healthy["state"], "critical")
        self.assertEqual(logs["primary_error_code"], "DB_CONNECTION_TIMEOUT")

    async def test_openai_client_uses_strict_structured_output(self) -> None:
        """Confirm that live requests carry the step JSON schema."""

        exchange = replay_exchange(
            "none",
            contracts.LOG_ANALYZER,
        )
        responses = FakeResponses(exchange["output"])
        client = OpenAIModelClient(
            model="test-model",
            client=FakeOpenAI(responses),
            max_attempts=1,
        )

        result = await client.generate(
            contracts.LOG_ANALYZER,
            exchange["input"],
        )

        self.assertEqual(result, exchange["output"])
        request = responses.calls[0]
        self.assertEqual(request["model"], "test-model")
        self.assertTrue(request["text"]["format"]["strict"])
        self.assertEqual(
            request["text"]["format"]["schema"],
            contracts.OUTPUT_SCHEMAS[contracts.LOG_ANALYZER],
        )

    async def test_openai_client_retries_provider_failure(self) -> None:
        """Confirm that a transient provider error receives one retry."""

        exchange = replay_exchange(
            "none",
            contracts.INVESTIGATION_PLANNER,
        )
        responses = FakeResponses(exchange["output"], failures=1)
        client = OpenAIModelClient(
            model="test-model",
            client=FakeOpenAI(responses),
            max_attempts=2,
            retry_delay_seconds=0,
        )

        result = await client.generate(
            contracts.INVESTIGATION_PLANNER,
            exchange["input"],
        )

        self.assertEqual(result, exchange["output"])
        self.assertEqual(len(responses.calls), 2)

    async def test_openai_client_does_not_retry_bad_request(self) -> None:
        """Confirm that a permanent provider error is returned immediately."""

        exchange = replay_exchange(
            "none",
            contracts.INVESTIGATION_PLANNER,
        )
        responses = FakeResponses(exchange["output"])

        async def bad_request(**values: Any) -> SimpleNamespace:
            responses.calls.append(values)
            raise FakeProviderError(400)

        responses.create = bad_request  # type: ignore[method-assign]
        client = OpenAIModelClient(
            model="test-model",
            client=FakeOpenAI(responses),
            max_attempts=3,
            retry_delay_seconds=0,
        )

        with self.assertRaisesRegex(ModelRuntimeError, "after 1 attempts"):
            await client.generate(
                contracts.INVESTIGATION_PLANNER,
                exchange["input"],
            )

        self.assertEqual(len(responses.calls), 1)

    async def test_openai_client_does_not_echo_provider_body(self) -> None:
        """Confirm that runtime errors exclude provider response body data."""

        exchange = replay_exchange(
            "none",
            contracts.INVESTIGATION_PLANNER,
        )
        responses = FakeResponses(exchange["output"])

        async def provider_failure(**values: Any) -> SimpleNamespace:
            responses.calls.append(values)
            raise FakeProviderError(503, "private-response-body")

        responses.create = provider_failure  # type: ignore[method-assign]
        client = OpenAIModelClient(
            model="test-model",
            client=FakeOpenAI(responses),
            max_attempts=1,
        )

        with self.assertRaises(ModelRuntimeError) as caught:
            await client.generate(
                contracts.INVESTIGATION_PLANNER,
                exchange["input"],
            )

        message = str(caught.exception)
        self.assertIn("FakeProviderError, status 503", message)
        self.assertNotIn("private-response-body", message)

    async def test_openai_client_uses_exponential_retry_delays(self) -> None:
        """Confirm that retry delays grow and include bounded jitter."""

        exchange = replay_exchange(
            "none",
            contracts.INVESTIGATION_PLANNER,
        )
        responses = FakeResponses(exchange["output"], failures=2)
        delays: list[float] = []

        async def record_delay(seconds: float) -> None:
            delays.append(seconds)

        client = OpenAIModelClient(
            model="test-model",
            client=FakeOpenAI(responses),
            max_attempts=3,
            retry_delay_seconds=0.5,
            sleep=record_delay,
            random_value=lambda: 1.0,
        )

        result = await client.generate(
            contracts.INVESTIGATION_PLANNER,
            exchange["input"],
        )

        self.assertEqual(result, exchange["output"])
        self.assertEqual(delays, [0.625, 1.25])

    async def test_openai_client_rejects_malformed_output(self) -> None:
        """Confirm that invalid provider JSON becomes a runtime error."""

        responses = FakeResponses({})

        async def malformed_create(**values: Any) -> SimpleNamespace:
            responses.calls.append(values)
            return SimpleNamespace(output_text="{not-json")

        responses.create = malformed_create  # type: ignore[method-assign]
        client = OpenAIModelClient(
            model="test-model",
            client=FakeOpenAI(responses),
            max_attempts=1,
        )
        input_data = replay_exchange(
            "none",
            contracts.LOG_ANALYZER,
        )["input"]

        with self.assertRaisesRegex(ModelRuntimeError, "failed after 1 attempts"):
            await client.generate(contracts.LOG_ANALYZER, input_data)
        self.assertEqual(len(responses.calls), 1)

    async def test_openai_client_times_out(self) -> None:
        """Confirm that a slow provider request respects its deadline."""

        exchange = replay_exchange(
            "none",
            contracts.LOG_ANALYZER,
        )
        responses = FakeResponses(
            exchange["output"],
            delay_seconds=0.05,
        )
        client = OpenAIModelClient(
            model="test-model",
            client=FakeOpenAI(responses),
            max_attempts=1,
            timeout_seconds=0.001,
        )

        with self.assertRaisesRegex(ModelRuntimeError, "failed after 1 attempts"):
            await client.generate(
                contracts.LOG_ANALYZER,
                exchange["input"],
            )

        self.assertEqual(responses.cancelled_requests, 1)
        self.assertEqual(responses.active_requests, 0)


if __name__ == "__main__":
    unittest.main()
