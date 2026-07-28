package documentdemo_test

import (
	"encoding/json"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/documentdemo"
)

func TestRunRecordsExpectedDocumentDAG(t *testing.T) {
	trace, err := documentdemo.Run(documentdemo.FailureNone)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if trace.Version != attrib.CurrentTraceVersion {
		t.Fatalf("expected trace version %d, got %d", attrib.CurrentTraceVersion, trace.Version)
	}
	if len(trace.Steps) != 6 {
		t.Fatalf("expected six steps, got %d", len(trace.Steps))
	}

	ordered, err := attrib.TopologicalSort(trace)
	if err != nil {
		t.Fatalf("TopologicalSort returned error: %v", err)
	}
	want := []string{
		documentdemo.DocumentLoaderStepID,
		documentdemo.InformationExtractorStepID,
		documentdemo.ReferenceCheckerStepID,
		documentdemo.RiskAnalyzerStepID,
		documentdemo.EvidenceMergerStepID,
		documentdemo.ReportGeneratorStepID,
	}
	for index, stepID := range want {
		if ordered[index].StepID != stepID {
			t.Fatalf("expected step %q at position %d, got %q", stepID, index, ordered[index].StepID)
		}
	}
}

func TestHealthyDocumentRunApprovesClaim(t *testing.T) {
	trace, err := documentdemo.Run(documentdemo.FailureNone)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	report := decodeReport(t, trace)
	if report.Decision != "approve" {
		t.Fatalf("expected approve decision, got %q", report.Decision)
	}
}

func TestExtractionFailurePropagatesThroughActualInputs(t *testing.T) {
	trace, err := documentdemo.Run(documentdemo.FailureExtraction)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	report := decodeReport(t, trace)
	if report.Decision != "manual_review" {
		t.Fatalf("expected manual review decision, got %q", report.Decision)
	}
	if report.Summary != "Decision is manual_review with high risk." {
		t.Fatalf("unexpected propagated report %q", report.Summary)
	}
}

func TestRunRejectsUnknownFailureMode(t *testing.T) {
	_, err := documentdemo.Run(documentdemo.FailureMode("unknown"))
	if err == nil {
		t.Fatal("expected an error")
	}
}

func decodeReport(t *testing.T, trace attrib.Trace) documentdemo.Report {
	t.Helper()

	for _, step := range trace.Steps {
		if step.StepID != documentdemo.ReportGeneratorStepID {
			continue
		}

		var report documentdemo.Report
		if err := json.Unmarshal(step.Output, &report); err != nil {
			t.Fatalf("decode report: %v", err)
		}
		return report
	}

	t.Fatal("missing report generator step")
	return documentdemo.Report{}
}
