"""Model clients for live, replay, and injected failure execution."""

from __future__ import annotations

import asyncio
import copy
import json
import os
import random
from pathlib import Path
from typing import Any, Awaitable, Callable, Protocol

from . import contracts, prompts


DEFAULT_REPLAY_DIRECTORY = Path(__file__).resolve().parent / "replays"
RETRYABLE_STATUS_CODES = frozenset({408, 409, 429})
RETRYABLE_ERROR_NAMES = frozenset(
    {
        "APIConnectionError",
        "APITimeoutError",
        "InternalServerError",
        "RateLimitError",
    }
)


class ModelRuntimeError(RuntimeError):
    """Reports a model request or replay failure."""


class ModelClient(Protocol):
    """Defines structured generation used by model backed pipeline steps."""

    @property
    def model_name(self) -> str:
        """Return the model label stored in the trace."""

    async def generate(
        self,
        step_id: str,
        input_data: Any,
    ) -> dict[str, Any]:
        """Generate one validated step output."""


class ReplayModelClient:
    """Returns deterministic responses loaded from replay files."""

    def __init__(
        self,
        responses: dict[str, Any],
        label: str,
    ) -> None:
        """Create a replay client.

        Input
        responses dict of str to Any
        Stored outputs keyed by model step ID.

        label str
        Replay label stored as the model name.

        Output
        None
        The replay responses are validated and stored.
        """

        if not label.strip():
            raise ValueError("replay label is empty")

        validated = {}
        for step_id, response in responses.items():
            if step_id not in contracts.MODEL_STEP_IDS:
                raise ValueError(f"replay contains unknown model step {step_id!r}")
            contracts.validate_output(step_id, response)
            validated[step_id] = copy.deepcopy(response)

        missing = sorted(contracts.MODEL_STEP_IDS.difference(validated))
        if missing:
            raise ValueError(f"replay is missing model step {missing[0]!r}")

        self._responses = validated
        self._model_name = f"replay/{label}"

    @property
    def model_name(self) -> str:
        """Return the replay label.

        Input
        None

        Output
        str
        Model label stored in recorded steps.
        """

        return self._model_name

    async def generate(
        self,
        step_id: str,
        input_data: Any,
    ) -> dict[str, Any]:
        """Return one stored response.

        Input
        step_id str
        Model backed step identifier.

        input_data Any
        Actual step input retained for interface compatibility.

        Output
        dict of str to Any
        Detached replay response.
        """

        del input_data
        await asyncio.sleep(0)
        if step_id not in self._responses:
            raise ModelRuntimeError(f"no replay response for step {step_id!r}")
        return copy.deepcopy(self._responses[step_id])

    @classmethod
    def from_directory(
        cls,
        failure: str,
        directory: Path = DEFAULT_REPLAY_DIRECTORY,
    ) -> "ReplayModelClient":
        """Load healthy responses and an optional failure overlay.

        Input
        failure str
        Failure mode named none, metrics, or deployment.

        directory Path
        Directory containing replay JSON files.

        Output
        ReplayModelClient
        Validated deterministic model client.
        """

        if failure not in {"none", "metrics", "deployment"}:
            raise ValueError(f"unsupported replay failure {failure!r}")

        responses = _load_replay_file(directory / "healthy.json")
        if failure != "none":
            overlay = _load_replay_file(directory / f"{failure}_failure.json")
            responses.update(overlay)
        return cls(responses, failure)


class FaultInjectingModelClient:
    """Overrides one live model step with a deterministic bad response."""

    def __init__(
        self,
        wrapped: ModelClient,
        step_id: str,
        output: dict[str, Any],
    ) -> None:
        """Create a fault injecting client.

        Input
        wrapped ModelClient
        Client used for every step except the selected target.

        step_id str
        Step that receives the injected output.

        output dict of str to Any
        Valid but semantically incorrect step output.

        Output
        None
        The validated override is stored.
        """

        if step_id not in contracts.MODEL_STEP_IDS:
            raise ValueError(f"cannot inject unknown model step {step_id!r}")
        contracts.validate_output(step_id, output)
        self._wrapped = wrapped
        self._step_id = step_id
        self._output = copy.deepcopy(output)

    @property
    def model_name(self) -> str:
        """Return the wrapped model label with fault metadata.

        Input
        None

        Output
        str
        Trace model label.
        """

        return f"{self._wrapped.model_name}+fault/{self._step_id}"

    async def generate(
        self,
        step_id: str,
        input_data: Any,
    ) -> dict[str, Any]:
        """Generate or inject one output.

        Input
        step_id str
        Current model step.

        input_data Any
        Actual model input.

        Output
        dict of str to Any
        Injected target output or wrapped client output.
        """

        if step_id == self._step_id:
            return copy.deepcopy(self._output)
        return await self._wrapped.generate(step_id, input_data)


class OpenAIModelClient:
    """Calls the OpenAI Responses API with strict structured outputs."""

    def __init__(
        self,
        model: str,
        client: Any = None,
        max_attempts: int = 2,
        timeout_seconds: float = 45.0,
        retry_delay_seconds: float = 0.25,
        sleep: Callable[[float], Awaitable[None]] = asyncio.sleep,
        random_value: Callable[[], float] = random.random,
    ) -> None:
        """Create a live model client.

        Input
        model str
        OpenAI model identifier.

        client Any
        Optional compatible client used by tests.

        max_attempts int
        Total request attempts before failure.

        timeout_seconds float
        Deadline for each request.

        retry_delay_seconds float
        Initial delay between attempts.

        sleep Callable accepting float and returning Awaitable of None
        Async delay function used between attempts.

        random_value Callable returning float
        Random value provider used to add retry jitter.

        Output
        None
        The live client configuration is stored.
        """

        if not model.strip():
            raise ValueError("OpenAI model is empty")
        if max_attempts < 1:
            raise ValueError("max attempts must be at least one")
        if timeout_seconds <= 0:
            raise ValueError("timeout must be positive")
        if retry_delay_seconds < 0:
            raise ValueError("retry delay cannot be negative")

        self._model = model
        self._client = client
        self._max_attempts = max_attempts
        self._timeout_seconds = timeout_seconds
        self._retry_delay_seconds = retry_delay_seconds
        self._sleep = sleep
        self._random_value = random_value

    @property
    def model_name(self) -> str:
        """Return the live model label.

        Input
        None

        Output
        str
        Model label stored in the trace.
        """

        return f"openai/{self._model}"

    @classmethod
    def from_environment(cls) -> "OpenAIModelClient":
        """Create a client from environment configuration.

        Input
        None

        Output
        OpenAIModelClient
        Client using AGENTTRACE_OPENAI_MODEL.
        """

        model = os.environ.get("AGENTTRACE_OPENAI_MODEL", "").strip()
        if not model:
            raise ModelRuntimeError(
                "live mode requires AGENTTRACE_OPENAI_MODEL"
            )
        return cls(model=model)

    async def generate(
        self,
        step_id: str,
        input_data: Any,
    ) -> dict[str, Any]:
        """Generate one structured model response.

        Input
        step_id str
        Model backed step identifier.

        input_data Any
        Actual step input encoded into the user prompt.

        Output
        dict of str to Any
        Locally validated structured output.
        """

        if step_id not in contracts.MODEL_STEP_IDS:
            raise ModelRuntimeError(f"step {step_id!r} is not model backed")

        last_error: Exception | None = None
        for attempt in range(1, self._max_attempts + 1):
            try:
                output = await asyncio.wait_for(
                    self._request(step_id, input_data),
                    timeout=self._timeout_seconds,
                )
                contracts.validate_output(step_id, output)
                return output
            except asyncio.CancelledError:
                raise
            except Exception as error:
                last_error = error
                if (
                    attempt >= self._max_attempts
                    or not _is_retryable_provider_error(error)
                ):
                    break
                await self._sleep(self._retry_delay(attempt))

        raise ModelRuntimeError(
            f"model step {step_id!r} failed after "
            f"{attempt} attempts: {last_error}"
        ) from last_error

    async def _request(
        self,
        step_id: str,
        input_data: Any,
    ) -> dict[str, Any]:
        """Perform one asynchronous Responses API request.

        Input
        step_id str
        Model backed step identifier.

        input_data Any
        Actual step input.

        Output
        dict of str to Any
        Decoded JSON response.
        """

        client = self._get_client()
        response = await client.responses.create(
            model=self._model,
            input=[
                {
                    "role": "system",
                    "content": prompts.system_prompt(step_id),
                },
                {
                    "role": "user",
                    "content": prompts.user_prompt(input_data),
                },
            ],
            text={
                "format": {
                    "type": "json_schema",
                    "name": f"agenttrace_{step_id}",
                    "schema": contracts.OUTPUT_SCHEMAS[step_id],
                    "strict": True,
                }
            },
        )

        output_text = getattr(response, "output_text", "")
        if not isinstance(output_text, str) or not output_text.strip():
            raise ModelRuntimeError(
                f"model step {step_id!r} returned no output text"
            )
        try:
            output = json.loads(output_text)
        except json.JSONDecodeError as error:
            raise ModelRuntimeError(
                f"model step {step_id!r} returned invalid JSON"
            ) from error
        if not isinstance(output, dict):
            raise ModelRuntimeError(
                f"model step {step_id!r} output must be an object"
            )
        return output

    def _get_client(self) -> Any:
        """Return the shared asynchronous OpenAI client.

        Input
        None

        Output
        Any
        Existing or newly created client exposing responses.create.
        """

        if self._client is None:
            self._client = self._create_client()
        return self._client

    def _create_client(self) -> Any:
        """Create the optional asynchronous OpenAI SDK client.

        Input
        None

        Output
        Any
        AsyncOpenAI client exposing responses.create.
        """

        try:
            from openai import AsyncOpenAI
        except ImportError as error:
            raise ModelRuntimeError(
                "live mode requires: "
                "python -m pip install -r "
                "./examples/incident_agent_pipeline/requirements-live.txt"
            ) from error

        return AsyncOpenAI(
            max_retries=0,
            timeout=self._timeout_seconds,
        )

    def _retry_delay(self, attempt: int) -> float:
        """Calculate exponential retry delay with bounded jitter.

        Input
        attempt int
        Request attempt that just failed.

        Output
        float
        Delay in seconds before the next attempt.
        """

        base_delay = self._retry_delay_seconds * (2 ** (attempt - 1))
        jitter = base_delay * 0.25 * self._random_value()
        return base_delay + jitter


def _is_retryable_provider_error(error: Exception) -> bool:
    """Report whether a provider failure can be retried.

    Input
    error Exception
    Failure raised by the request path.

    Output
    bool
    True for connection, timeout, rate limit, and server failures.
    """

    if isinstance(error, (ConnectionError, TimeoutError)):
        return True

    status_code = getattr(error, "status_code", None)
    if isinstance(status_code, int):
        return (
            status_code in RETRYABLE_STATUS_CODES
            or status_code >= 500
        )

    return type(error).__name__ in RETRYABLE_ERROR_NAMES


def failure_override(
    failure: str,
    directory: Path = DEFAULT_REPLAY_DIRECTORY,
) -> tuple[str, dict[str, Any]] | None:
    """Load the source analyzer override for live failure mode.

    Input
    failure str
    Failure mode named none, metrics, or deployment.

    directory Path
    Replay directory.

    Output
    tuple of str and dict or None
    Target step and bad output, or None for healthy mode.
    """

    if failure == "none":
        return None
    if failure == "metrics":
        step_id = contracts.METRICS_ANALYZER
    elif failure == "deployment":
        step_id = contracts.DEPLOYMENT_ANALYZER
    else:
        raise ValueError(f"unsupported live failure {failure!r}")

    overlay = _load_replay_file(directory / f"{failure}_failure.json")
    if step_id not in overlay:
        raise ValueError(
            f"failure replay is missing target step {step_id!r}"
        )
    return step_id, overlay[step_id]


def _load_replay_file(path: Path) -> dict[str, Any]:
    """Read one replay JSON object.

    Input
    path Path
    Replay file path.

    Output
    dict of str to Any
    Stored outputs keyed by step ID.
    """

    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"load replay {path}: {error}") from error
    if not isinstance(value, dict):
        raise ValueError(f"replay {path} must contain an object")
    return value
