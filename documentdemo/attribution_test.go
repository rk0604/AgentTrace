package documentdemo_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/checkerconfig"
	"github.com/rk0604/AgentTrace/documentdemo"
)

func TestDocumentConfigurationAttributesFailureModes(t *testing.T) {
	tests := []struct {
		name          string
		failureMode   documentdemo.FailureMode
		wantStatus    string
		wantRootCause string
		wantChecked   int
	}{
		{
			name:        "healthy",
			failureMode: documentdemo.FailureNone,
			wantStatus:  "passed",
			wantChecked: 6,
		},
		{
			name:          "extraction failure",
			failureMode:   documentdemo.FailureExtraction,
			wantStatus:    "failed",
			wantRootCause: documentdemo.InformationExtractorStepID,
			wantChecked:   2,
		},
		{
			name:          "reference failure",
			failureMode:   documentdemo.FailureReference,
			wantStatus:    "failed",
			wantRootCause: documentdemo.ReferenceCheckerStepID,
			wantChecked:   3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trace, err := documentdemo.Run(test.failureMode)
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
			checkers := loadDocumentCheckers(t, trace)

			result, err := attrib.FindRootCause(trace, checkers)
			if err != nil {
				t.Fatalf("FindRootCause returned error: %v", err)
			}
			if result.Status != test.wantStatus {
				t.Fatalf("expected status %q, got %q", test.wantStatus, result.Status)
			}
			if len(result.CheckedStepIDs) != test.wantChecked {
				t.Fatalf("expected %d checked steps, got %d", test.wantChecked, len(result.CheckedStepIDs))
			}
			if test.wantRootCause == "" {
				if result.RootCause != nil {
					t.Fatalf("expected no root cause, got %+v", result.RootCause)
				}
				return
			}
			if result.RootCause == nil || result.RootCause.StepID != test.wantRootCause {
				t.Fatalf("expected root cause %q, got %+v", test.wantRootCause, result.RootCause)
			}
		})
	}
}

func TestDocumentDownstreamStepsPassGivenFailedInputs(t *testing.T) {
	trace, err := documentdemo.Run(documentdemo.FailureExtraction)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	checkers := loadDocumentCheckers(t, trace)

	for _, stepID := range []string{
		documentdemo.RiskAnalyzerStepID,
		documentdemo.EvidenceMergerStepID,
		documentdemo.ReportGeneratorStepID,
	} {
		step := findDocumentStep(t, trace, stepID)
		check, err := checkers[stepID](step)
		if err != nil {
			t.Fatalf("checker for %q returned error: %v", stepID, err)
		}
		if !check.Passed {
			t.Fatalf("expected downstream step %q to pass, got %q", stepID, check.Reason)
		}
	}
}

func loadDocumentCheckers(t *testing.T, trace attrib.Trace) map[string]attrib.StepChecker {
	t.Helper()

	configFile, err := os.Open(filepath.Join("..", "examples", "document-checkers-v2.json"))
	if err != nil {
		t.Fatalf("open checker configuration: %v", err)
	}
	config, decodeErr := checkerconfig.Decode(configFile)
	closeErr := configFile.Close()
	if decodeErr != nil {
		t.Fatalf("decode checker configuration: %v", decodeErr)
	}
	if closeErr != nil {
		t.Fatalf("close checker configuration: %v", closeErr)
	}

	contextData, err := os.ReadFile(filepath.Join("..", "examples", "document-context.json"))
	if err != nil {
		t.Fatalf("read evaluation context: %v", err)
	}
	checkers, err := checkerconfig.BuildWithOptions(config, checkerconfig.BuildOptions{
		Trace:   &trace,
		Context: json.RawMessage(contextData),
	})
	if err != nil {
		t.Fatalf("build checkers: %v", err)
	}

	return checkers
}

func findDocumentStep(t *testing.T, trace attrib.Trace, stepID string) attrib.Step {
	t.Helper()

	for _, step := range trace.Steps {
		if step.StepID == stepID {
			return step
		}
	}

	t.Fatalf("missing step %q", stepID)
	return attrib.Step{}
}
