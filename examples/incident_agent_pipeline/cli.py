"""Command line entry point for the incident agent pipeline."""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
from pathlib import Path
from typing import Any

from .fixtures import DEFAULT_FIXTURE_DIRECTORY, load_fixture_bundle
from .model_runtime import (
    FaultInjectingModelClient,
    ModelClient,
    OpenAIModelClient,
    ReplayModelClient,
    failure_override,
)
from .pipeline import IncidentPipeline, SequenceClock
from .redaction import Redactor


def build_parser() -> argparse.ArgumentParser:
    """Create the pipeline argument parser.

    Input
    None

    Output
    argparse.ArgumentParser
    Parser for execution mode, failure mode, and output paths.
    """

    parser = argparse.ArgumentParser(
        description=(
            "Run a recorded 13 step incident investigation pipeline"
        )
    )
    parser.add_argument(
        "--mode",
        choices=("replay", "live"),
        default="replay",
        help="use stored responses or live model calls",
    )
    parser.add_argument(
        "--failure",
        choices=("none", "metrics", "deployment"),
        default="none",
        help="inject one semantic analyzer failure",
    )
    parser.add_argument(
        "--fixtures",
        type=Path,
        default=DEFAULT_FIXTURE_DIRECTORY,
        help="read source evidence from this directory",
    )
    parser.add_argument(
        "--trace-output",
        type=Path,
        default=Path("incident-agent-trace.json"),
        help="write the AgentTrace JSON trace to this path",
    )
    parser.add_argument(
        "--summary-output",
        type=Path,
        help="optionally write the final incident summary as JSON",
    )
    parser.add_argument(
        "--run-id",
        help="override the generated trace run ID",
    )
    return parser


def main(arguments: list[str] | None = None) -> int:
    """Run the incident pipeline command.

    Input
    arguments list of str or None
    Optional command arguments excluding the executable name.

    Output
    int
    Zero on success, one on runtime failure, or argparse usage status.
    """

    parser = build_parser()
    args = parser.parse_args(arguments)

    try:
        fixture = load_fixture_bundle(args.fixtures)
        client = _build_model_client(args.mode, args.failure)
        run_id = args.run_id or (
            f"incident-agent-{args.mode}-{args.failure}"
        )
        clock = SequenceClock() if args.mode == "replay" else None
        pipeline = IncidentPipeline(
            run_id=run_id,
            fixture_bundle=fixture,
            model_client=client,
            redactor=Redactor.from_environment(),
            clock=clock,
        )
    except Exception as error:
        print(f"configure incident pipeline: {error}", file=sys.stderr)
        return 1

    try:
        summary = asyncio.run(pipeline.run())
    except Exception as error:
        try:
            pipeline.write_trace(args.trace_output)
        except Exception as trace_error:
            print(
                f"write partial trace: {trace_error}",
                file=sys.stderr,
            )
        print(f"run incident pipeline: {error}", file=sys.stderr)
        return 1

    try:
        pipeline.write_trace(args.trace_output)
        if args.summary_output is not None:
            _write_json(args.summary_output, summary)
    except Exception as error:
        print(f"write incident output: {error}", file=sys.stderr)
        return 1

    print("Incident investigation completed")
    print(f"Run ID: {run_id}")
    print(f"Mode: {args.mode}")
    print(f"Failure: {args.failure}")
    print(f"Trace: {args.trace_output}")
    print("Summary:")
    print(json.dumps(summary, indent=2, ensure_ascii=False))
    return 0


def _build_model_client(mode: str, failure: str) -> ModelClient:
    """Create the selected model runtime.

    Input
    mode str
    Replay or live execution mode.

    failure str
    None, metrics, or deployment fault mode.

    Output
    ModelClient
    Configured structured generation client.
    """

    if mode == "replay":
        return ReplayModelClient.from_directory(failure)
    if mode != "live":
        raise ValueError(f"unsupported execution mode {mode!r}")

    client: ModelClient = OpenAIModelClient.from_environment()
    override = failure_override(failure)
    if override is not None:
        step_id, output = override
        client = FaultInjectingModelClient(client, step_id, output)
    return client


def _write_json(path: Path, value: Any) -> None:
    """Write one formatted UTF 8 JSON file.

    Input
    path Path
    Destination file.

    value Any
    JSON compatible value.

    Output
    None
    Parent directories and the JSON file are created.
    """

    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        json.dumps(value, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
        newline="\n",
    )


if __name__ == "__main__":
    raise SystemExit(main())
