"""Generate a small trace through the Python AgentTrace recorder."""

from __future__ import annotations

import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any


REPOSITORY_ROOT = Path(__file__).resolve().parents[1]
PYTHON_SDK = REPOSITORY_ROOT / "sdk" / "python"
sys.path.insert(0, str(PYTHON_SDK))

from agenttrace import TraceRecorder


class ExampleClock:
    """Returns deterministic timestamps for the checked in example."""

    def __init__(self) -> None:
        """Create a clock beginning at the example run time."""

        self._next = datetime(2026, 7, 28, 14, 0, tzinfo=timezone.utc)

    def __call__(self) -> datetime:
        """Return the next deterministic timestamp.

        Input
        None

        Output
        datetime
        Current timestamp before advancing one second.
        """

        current = self._next
        self._next += timedelta(seconds=1)
        return current


def load_document(_: dict[str, str]) -> dict[str, str]:
    """Return the source document.

    Input
    value dict of str to str
    Document location metadata.

    Output
    dict of str to str
    Loaded document text.
    """

    return {
        "document": "Café North reported revenue of $4.2M for Q2.",
    }


def extract_facts(value: dict[str, str]) -> dict[str, Any]:
    """Extract structured facts from the source document.

    Input
    value dict of str to str
    Loaded source document.

    Output
    dict of str to Any
    Revenue and reporting period.
    """

    return {
        "revenue": "$4.2M",
        "period": "Q2",
    }


def classify_risk(value: dict[str, Any]) -> dict[str, str]:
    """Classify risk from extracted facts.

    Input
    value dict of str to Any
    Extracted revenue facts.

    Output
    dict of str to str
    Risk classification.
    """

    return {
        "risk": "low" if value["period"] == "Q2" else "high",
    }


def write_report(value: dict[str, Any]) -> dict[str, str]:
    """Build a final report from branch outputs.

    Input
    value dict of str to Any
    Extracted facts and risk classification.

    Output
    dict of str to str
    Human readable report.
    """

    facts = value["facts"]
    risk = value["risk"]
    return {
        "report": (
            f"Revenue was {facts['revenue']} in {facts['period']} "
            f"with {risk['risk']} risk."
        ),
    }


def build_trace(output_path: Path) -> None:
    """Run the example pipeline and write its trace.

    Input
    output_path Path
    Destination for the generated trace.

    Output
    None
    A versioned AgentTrace JSON file is written.
    """

    recorder = TraceRecorder("python-recorded-pipeline", clock=ExampleClock())
    document = recorder.run_step(
        "document_loader",
        "Document Loader",
        {"path": "cafe-north-release.txt"},
        load_document,
        model_used="python-function",
    )
    facts = recorder.run_step(
        "information_extractor",
        "Information Extractor",
        document,
        extract_facts,
        depends_on=["document_loader"],
        model_used="python-function",
        confidence=0.96,
    )
    risk = recorder.run_step(
        "risk_analyzer",
        "Risk Analyzer",
        facts,
        classify_risk,
        depends_on=["information_extractor"],
        model_used="python-function",
        confidence=0.91,
    )
    recorder.run_step(
        "report_generator",
        "Report Generator",
        {"facts": facts, "risk": risk},
        write_report,
        depends_on=["information_extractor", "risk_analyzer"],
        model_used="python-function",
    )
    recorder.write_trace(output_path)


if __name__ == "__main__":
    destination = (
        Path(sys.argv[1])
        if len(sys.argv) > 1
        else REPOSITORY_ROOT / "examples" / "python-recorded-trace.json"
    )
    build_trace(destination)
