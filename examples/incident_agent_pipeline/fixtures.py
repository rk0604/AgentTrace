"""Load and validate the incident evidence bundle."""

from __future__ import annotations

import json
from datetime import datetime
from pathlib import Path
from typing import Any


DEFAULT_FIXTURE_DIRECTORY = Path(__file__).resolve().parent / "fixtures"


def load_fixture_bundle(
    directory: Path = DEFAULT_FIXTURE_DIRECTORY,
) -> dict[str, Any]:
    """Load one incident fixture bundle.

    Input
    directory Path
    Directory containing the alert, logs, metrics, deployments, and runbook.

    Output
    dict of str to Any
    Validated incident evidence grouped by source.
    """

    bundle = {
        "alert": _load_json(directory / "alert.json"),
        "logs": _load_ndjson(directory / "logs.ndjson"),
        "metrics": _load_json(directory / "metrics.json"),
        "deployments": _load_json(directory / "deployments.json"),
        "runbook": _load_json(directory / "runbook.json"),
    }
    validate_fixture_bundle(bundle)
    return bundle


def validate_fixture_bundle(bundle: dict[str, Any]) -> None:
    """Validate source evidence consistency.

    Input
    bundle dict of str to Any
    Candidate incident evidence bundle.

    Output
    None
    The function returns when the fixture is internally consistent.
    """

    required_sources = {"alert", "logs", "metrics", "deployments", "runbook"}
    missing = sorted(required_sources.difference(bundle))
    if missing:
        raise ValueError(f"fixture is missing source {missing[0]!r}")

    alert = _require_object(bundle["alert"], "alert")
    logs = _require_list(bundle["logs"], "logs")
    metrics = _require_object(bundle["metrics"], "metrics")
    deployments = _require_list(bundle["deployments"], "deployments")
    runbook = _require_object(bundle["runbook"], "runbook")

    service = _require_string(alert, "service", "alert")
    _parse_timestamp(_require_string(alert, "started_at", "alert"), "alert.started_at")
    if not logs:
        raise ValueError("fixture logs are empty")
    if not deployments:
        raise ValueError("fixture deployments are empty")

    evidence_ids = [
        _require_string(alert, "evidence_id", "alert"),
        _require_string(metrics, "evidence_id", "metrics"),
        _require_string(runbook, "evidence_id", "runbook"),
    ]

    for index, record_value in enumerate(logs):
        record = _require_object(record_value, f"logs[{index}]")
        if _require_string(record, "service", f"logs[{index}]") != service:
            raise ValueError(f"logs[{index}].service does not match alert service")
        _parse_timestamp(
            _require_string(record, "timestamp", f"logs[{index}]"),
            f"logs[{index}].timestamp",
        )
        evidence_ids.append(
            _require_string(record, "evidence_id", f"logs[{index}]")
        )

    for index, deployment_value in enumerate(deployments):
        deployment = _require_object(
            deployment_value,
            f"deployments[{index}]",
        )
        if (
            _require_string(
                deployment,
                "service",
                f"deployments[{index}]",
            )
            != service
        ):
            raise ValueError(
                f"deployments[{index}].service does not match alert service"
            )
        _parse_timestamp(
            _require_string(
                deployment,
                "deployed_at",
                f"deployments[{index}]",
            ),
            f"deployments[{index}].deployed_at",
        )
        evidence_ids.append(
            _require_string(
                deployment,
                "evidence_id",
                f"deployments[{index}]",
            )
        )

    if len(evidence_ids) != len(set(evidence_ids)):
        raise ValueError("fixture evidence IDs must be unique")

    _parse_timestamp(
        _require_string(metrics, "observed_at", "metrics"),
        "metrics.observed_at",
    )
    _validate_pool_regression(deployments)


def _validate_pool_regression(deployments: list[Any]) -> None:
    """Confirm that the fixture contains the intended configuration change.

    Input
    deployments list of Any
    Deployment records from the fixture.

    Output
    None
    The function returns when one deployment reduces the connection pool.
    """

    for deployment_value in deployments:
        deployment = _require_object(deployment_value, "deployment")
        changes = _require_list(deployment.get("changes"), "deployment.changes")
        for change_value in changes:
            change = _require_object(change_value, "deployment change")
            if (
                change.get("field") == "database.connection_pool_size"
                and isinstance(change.get("before"), int)
                and isinstance(change.get("after"), int)
                and change["after"] < change["before"]
            ):
                return

    raise ValueError("fixture does not contain a database pool regression")


def _load_json(path: Path) -> Any:
    """Read one UTF 8 JSON file.

    Input
    path Path
    Source file.

    Output
    Any
    Decoded JSON value.
    """

    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"load fixture {path}: {error}") from error


def _load_ndjson(path: Path) -> list[Any]:
    """Read one UTF 8 newline delimited JSON file.

    Input
    path Path
    Source file.

    Output
    list of Any
    Decoded JSON records.
    """

    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except OSError as error:
        raise ValueError(f"load fixture {path}: {error}") from error

    records = []
    for line_number, line in enumerate(lines, start=1):
        if not line.strip():
            continue
        try:
            records.append(json.loads(line))
        except json.JSONDecodeError as error:
            raise ValueError(
                f"load fixture {path} line {line_number}: {error}"
            ) from error
    return records


def _require_object(value: Any, path: str) -> dict[str, Any]:
    """Require a JSON object.

    Input
    value Any
    Candidate value.

    path str
    Error location.

    Output
    dict of str to Any
    Validated object.
    """

    if not isinstance(value, dict):
        raise ValueError(f"{path} must be an object")
    return value


def _require_list(value: Any, path: str) -> list[Any]:
    """Require a JSON array.

    Input
    value Any
    Candidate value.

    path str
    Error location.

    Output
    list of Any
    Validated array.
    """

    if not isinstance(value, list):
        raise ValueError(f"{path} must be an array")
    return value


def _require_string(
    value: dict[str, Any],
    field_name: str,
    path: str,
) -> str:
    """Read one required nonempty string field.

    Input
    value dict of str to Any
    Object containing the field.

    field_name str
    Required field name.

    path str
    Error location.

    Output
    str
    Validated string value.
    """

    field_value = value.get(field_name)
    if not isinstance(field_value, str) or not field_value.strip():
        raise ValueError(f"{path}.{field_name} must be a nonempty string")
    return field_value


def _parse_timestamp(value: str, path: str) -> datetime:
    """Parse one RFC 3339 timestamp.

    Input
    value str
    Timestamp text.

    path str
    Error location.

    Output
    datetime
    Parsed timezone aware timestamp.
    """

    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as error:
        raise ValueError(f"{path} must be an RFC 3339 timestamp") from error
    if parsed.tzinfo is None:
        raise ValueError(f"{path} must include a timezone")
    return parsed
