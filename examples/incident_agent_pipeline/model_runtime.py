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
CURRENT_REPLAY_VERSION = 1
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
        exchanges: dict[str, Any],
        label: str,
    ) -> None:
        """Create a replay client.

        Input
        exchanges dict of str to Any
        Stored inputs and outputs keyed by model step ID.

        label str
        Replay label stored as the model name.

        Output
        None
        The replay exchanges are validated and stored.
        """

        if not label.strip():
            raise ValueError("replay label is empty")

        validated = {}
        for step_id, exchange in exchanges.items():
            if step_id not in contracts.MODEL_STEP_IDS:
                raise ValueError(f"replay contains unknown model step {step_id!r}")
            if not isinstance(exchange, dict):
                raise ValueError(
                    f"replay step {step_id!r} must be an object"
                )
            fields = set(exchange)
            if fields != {"input", "output"}:
                raise ValueError(
                    f"replay step {step_id!r} must contain input and output"
                )
            contracts.validate_model_input(step_id, exchange["input"])
            contracts.validate_output(step_id, exchange["output"])
            validated[step_id] = copy.deepcopy(exchange)

        missing = sorted(contracts.MODEL_STEP_IDS.difference(validated))
        if missing:
            raise ValueError(f"replay is missing model step {missing[0]!r}")

        self._exchanges = validated
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
        Actual step input compared with the stored request.

        Output
        dict of str to Any
        Detached replay response.
        """

        await asyncio.sleep(0)
        if step_id not in self._exchanges:
            raise ModelRuntimeError(f"no replay response for step {step_id!r}")
        contracts.validate_model_input(step_id, input_data)
        exchange = self._exchanges[step_id]
        if input_data != exchange["input"]:
            raise ModelRuntimeError(
                f"replay input mismatch for step {step_id!r}"
            )
        return copy.deepcopy(exchange["output"])

    @classmethod
    def from_directory(
        cls,
        failure: str,
        directory: Path = DEFAULT_REPLAY_DIRECTORY,
    ) -> "ReplayModelClient":
        """Load one complete replay transcript.

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

        file_name = (
            "healthy.json"
            if failure == "none"
            else f"{failure}_failure.json"
        )
        exchanges = _load_replay_steps(directory / file_name)
        return cls(exchanges, failure)


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

        contracts.validate_model_input(step_id, input_data)
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
        contracts.validate_model_input(step_id, input_data)

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
            f"{attempt} attempts: {_safe_provider_error(last_error)}"
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
            output = _decode_json(output_text)
        except ValueError as error:
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


def _safe_provider_error(error: Exception | None) -> str:
    """Describe a provider failure without response body data.

    Input
    error Exception or None
    Last request or validation failure.

    Output
    str
    Error type with optional status code and request ID.
    """

    if error is None:
        return "unknown error"

    details = [type(error).__name__]
    status_code = getattr(error, "status_code", None)
    if isinstance(status_code, int):
        details.append(f"status {status_code}")

    request_id = getattr(error, "request_id", None)
    if isinstance(request_id, str) and request_id.strip():
        details.append(f"request {request_id.strip()}")

    return ", ".join(details)


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

    exchanges = _load_replay_steps(directory / f"{failure}_failure.json")
    if step_id not in exchanges:
        raise ValueError(
            f"failure replay is missing target step {step_id!r}"
        )
    return step_id, copy.deepcopy(exchanges[step_id]["output"])


def _load_replay_steps(path: Path) -> dict[str, Any]:
    """Read one versioned replay transcript.

    Input
    path Path
    Replay file path.

    Output
    dict of str to Any
    Stored input and output exchanges keyed by step ID.
    """

    try:
        value = _decode_json(path.read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        raise ValueError(f"load replay {path}: {error}") from error
    if not isinstance(value, dict):
        raise ValueError(f"replay {path} must contain an object")
    if set(value) != {"version", "steps"}:
        raise ValueError(
            f"replay {path} must contain version and steps"
        )
    if value["version"] != CURRENT_REPLAY_VERSION:
        raise ValueError(
            f"replay {path} uses unsupported version {value['version']!r}"
        )
    if not isinstance(value["steps"], dict):
        raise ValueError(f"replay {path} steps must be an object")
    return value["steps"]


def _decode_json(value: str) -> Any:
    """Decode strict JSON text.

    Input
    value str
    JSON document.

    Output
    Any
    Decoded JSON value with nonfinite constants rejected.
    """

    def reject_constant(constant: str) -> None:
        """Reject one nonstandard numeric constant.

        Input
        constant str
        Nonfinite JSON constant name.

        Output
        None
        The function always raises ValueError.
        """

        raise ValueError(f"invalid JSON constant {constant!r}")

    return json.loads(value, parse_constant=reject_constant)
