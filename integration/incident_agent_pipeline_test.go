package integration_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/checkerconfig"
)

// TestIncidentAgentPipelineAttribution verifies cross language root cause results.
//
// Input
// t pointer to testing.T
// Test state and failure reporting.
//
// Output
// None
// The test fails when one replay trace receives the wrong attribution.
func TestIncidentAgentPipelineAttribution(t *testing.T) {
	tests := []struct {
		name          string
		traceFile     string
		wantStatus    string
		wantRootCause string
	}{
		{
			name:       "healthy",
			traceFile:  "healthy.json",
			wantStatus: "passed",
		},
		{
			name:          "metrics failure",
			traceFile:     "metrics-failure.json",
			wantStatus:    "failed",
			wantRootCause: "metrics_analyzer",
		},
		{
			name:          "deployment failure",
			traceFile:     "deployment-failure.json",
			wantStatus:    "failed",
			wantRootCause: "deployment_analyzer",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trace, checkers := loadIncidentAgentInputs(t, test.traceFile)

			result, err := attrib.FindRootCause(trace, checkers)
			if err != nil {
				t.Fatalf("FindRootCause returned error: %v", err)
			}
			if result.Status != test.wantStatus {
				t.Fatalf(
					"expected status %q, got %q",
					test.wantStatus,
					result.Status,
				)
			}
			if test.wantRootCause == "" {
				if result.RootCause != nil {
					t.Fatalf("expected no root cause, got %+v", result.RootCause)
				}
				return
			}
			if result.RootCause == nil {
				t.Fatal("expected a root cause")
			}
			if result.RootCause.StepID != test.wantRootCause {
				t.Fatalf(
					"expected root cause %q, got %q",
					test.wantRootCause,
					result.RootCause.StepID,
				)
			}
		})
	}
}

// TestIncidentAgentPipelineDownstreamChecksUseActualInputs verifies checker semantics.
//
// Input
// t pointer to testing.T
// Test state and failure reporting.
//
// Output
// None
// The test fails when a downstream step is blamed for consistent bad input.
func TestIncidentAgentPipelineDownstreamChecksUseActualInputs(t *testing.T) {
	tests := []struct {
		name       string
		traceFile  string
		failedStep string
	}{
		{
			name:       "metrics failure",
			traceFile:  "metrics-failure.json",
			failedStep: "metrics_analyzer",
		},
		{
			name:       "deployment failure",
			traceFile:  "deployment-failure.json",
			failedStep: "deployment_analyzer",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trace, checkers := loadIncidentAgentInputs(t, test.traceFile)

			for _, step := range trace.Steps {
				check, err := checkers[step.StepID](step)
				if err != nil {
					t.Fatalf(
						"checker for step %q returned error: %v",
						step.StepID,
						err,
					)
				}
				if step.StepID == test.failedStep {
					if check.Passed {
						t.Fatalf(
							"expected step %q to fail",
							step.StepID,
						)
					}
					continue
				}
				if !check.Passed {
					t.Fatalf(
						"expected step %q to pass for actual inputs: %s",
						step.StepID,
						check.Reason,
					)
				}
			}
		})
	}
}

// loadIncidentAgentInputs loads one trace and the shared contextual checkers.
//
// Input
// t pointer to testing.T
// Test state and failure reporting.
//
// traceFile string
// File name inside the incident pipeline traces directory.
//
// Output
// attrib.Trace
// Decoded versioned Python trace.
//
// map of string to attrib.StepChecker
// Compiled version 2 checker functions.
func loadIncidentAgentInputs(
	t *testing.T,
	traceFile string,
) (attrib.Trace, map[string]attrib.StepChecker) {
	t.Helper()

	basePath := filepath.Join(
		"..",
		"examples",
		"incident_agent_pipeline",
	)
	traceData := readTestFile(
		t,
		filepath.Join(basePath, "traces", traceFile),
	)
	trace, err := attrib.DecodeTrace(bytes.NewReader(traceData))
	if err != nil {
		t.Fatalf("decode incident agent trace: %v", err)
	}

	checkerData := readTestFile(
		t,
		filepath.Join(basePath, "checkers-v2.json"),
	)
	config, err := checkerconfig.Decode(bytes.NewReader(checkerData))
	if err != nil {
		t.Fatalf("decode incident agent checkers: %v", err)
	}

	contextData := readTestFile(
		t,
		filepath.Join(basePath, "context.json"),
	)
	checkers, err := checkerconfig.BuildWithOptions(
		config,
		checkerconfig.BuildOptions{
			Trace:   &trace,
			Context: json.RawMessage(contextData),
		},
	)
	if err != nil {
		t.Fatalf("build incident agent checkers: %v", err)
	}

	return trace, checkers
}

// readTestFile reads one required integration fixture.
//
// Input
// t pointer to testing.T
// Test state and failure reporting.
//
// path string
// Fixture file path.
//
// Output
// slice of byte
// Complete fixture contents.
func readTestFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}
