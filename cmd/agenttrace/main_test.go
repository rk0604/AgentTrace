package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rk0604/AgentTrace/attrib"
)

func TestRunCommandLineRequiresExplicitCommand(t *testing.T) {
	err := runCommandLine(nil)
	assertErrorContains(t, err, "missing command")
}

func TestRunCommandLineRejectsUnknownCommand(t *testing.T) {
	err := runCommandLine([]string{"unknown"})
	assertErrorContains(t, err, `unknown command "unknown"`)
}

func TestRunCommandLineAcceptsHelp(t *testing.T) {
	if err := runCommandLine([]string{"help"}); err != nil {
		t.Fatalf("runCommandLine returned error: %v", err)
	}
}

// TestRenderFlowChartUsesBoxesWithinBudget verifies the normal graph view.
//
// Input
// t pointer to testing.T
// Test state and failure reporting.
//
// Output
// None
// The test fails when a small graph does not use the boxed layout.
func TestRenderFlowChartUsesBoxesWithinBudget(t *testing.T) {
	view := stepView{
		Step: attrib.Step{
			StepID:    "source",
			AgentName: "Source",
		},
		Checked: true,
	}
	lines := renderFlowChart(
		[]graphLevel{{Index: 0, Views: []stepView{view}}},
		nil,
		0,
		attrib.AttributionResult{},
	)
	output := strings.Join(lines, "\n")

	if !strings.Contains(output, boxTop()) {
		t.Fatalf("expected boxed graph, got:\n%s", output)
	}
	if strings.Contains(output, "layout omitted") {
		t.Fatalf("expected canvas layout, got:\n%s", output)
	}
}

// TestRenderFlowChartBoundsWideGraphs verifies the compact graph fallback.
//
// Input
// t pointer to testing.T
// Test state and failure reporting.
//
// Output
// None
// The test fails when a wide graph attempts an unbounded canvas.
func TestRenderFlowChartBoundsWideGraphs(t *testing.T) {
	views := make([]stepView, maxGraphCanvasNodes+1)
	for index := range views {
		views[index] = stepView{
			Step: attrib.Step{
				StepID:    fmt.Sprintf("step-%03d", index),
				AgentName: "Worker",
			},
			Checked: true,
		}
	}

	lines := renderFlowChart(
		[]graphLevel{{Index: 0, Views: views}},
		nil,
		0,
		attrib.AttributionResult{},
	)
	output := strings.Join(lines, "\n")

	if !strings.Contains(output, "layout omitted") {
		t.Fatalf("expected compact fallback, got:\n%s", output)
	}
	if len(lines) > maxRenderedGraphNodes+5 {
		t.Fatalf("compact graph returned too many lines: %d", len(lines))
	}
}

// TestGraphEdgesPrioritizesCauseEdges verifies bounded edge selection.
//
// Input
// t pointer to testing.T
// Test state and failure reporting.
//
// Output
// None
// The test fails when an omitted regular edge hides a cause edge.
func TestGraphEdgesPrioritizesCauseEdges(t *testing.T) {
	views := make([]stepView, maxRenderedGraphEdges+2)
	dependencies := make([]string, maxRenderedGraphEdges+1)
	for index := range dependencies {
		stepID := fmt.Sprintf("source-%03d", index)
		dependencies[index] = stepID
		views[index] = stepView{
			Step: attrib.Step{StepID: stepID},
		}
	}
	targetID := "target"
	views[len(views)-1] = stepView{
		Step: attrib.Step{
			StepID:    targetID,
			DependsOn: dependencies,
		},
	}
	causeSourceID := dependencies[len(dependencies)-1]
	result := attrib.AttributionResult{
		PotentialPropagationEdges: []attrib.CauseEdge{
			{
				FromStepID: causeSourceID,
				ToStepID:   targetID,
			},
		},
	}

	edges, totalEdges := graphEdges(
		views,
		stepViewByID(views),
		result,
	)

	if totalEdges != len(dependencies) {
		t.Fatalf(
			"expected %d total edges, got %d",
			len(dependencies),
			totalEdges,
		)
	}
	if len(edges) != maxRenderedGraphEdges {
		t.Fatalf(
			"expected %d rendered edges, got %d",
			maxRenderedGraphEdges,
			len(edges),
		)
	}
	foundCause := false
	for _, edge := range edges {
		if edge.Cause &&
			edge.Source.Step.StepID == causeSourceID &&
			edge.Target.Step.StepID == targetID {
			foundCause = true
		}
	}
	if !foundCause {
		t.Fatal("expected bounded edge list to retain cause edge")
	}
}

// TestFitTextPreservesUnicode verifies character based truncation.
//
// Input
// t pointer to testing.T
// Test state and failure reporting.
//
// Output
// None
// The test fails when truncation produces invalid UTF 8.
func TestFitTextPreservesUnicode(t *testing.T) {
	got := fitText("\u754c\u754c\u754c\u754c\u754c\u754c", 5)

	if !utf8.ValidString(got) {
		t.Fatalf("fitText returned invalid UTF 8: %q", got)
	}
	if utf8.RuneCountInString(got) != 5 {
		t.Fatalf("expected five characters, got %q", got)
	}
}

func TestRunCommandRequiresTraceAndCheckers(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing both", args: nil, want: "run requires --input"},
		{name: "missing checkers", args: []string{"--input", "trace.json"}, want: "run requires --checkers"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runTraceCommand(test.args)
			assertErrorContains(t, err, test.want)
		})
	}
}

func TestValidateCommandAcceptsExampleConfiguration(t *testing.T) {
	tracePath := filepath.Join("..", "..", "examples", "trace-reference-failure.json")
	checkersPath := filepath.Join("..", "..", "examples", "toy-checkers.json")

	err := validateCommand([]string{"--input", tracePath, "--checkers", checkersPath})
	if err != nil {
		t.Fatalf("validateCommand returned error: %v", err)
	}
}

func TestRunCommandAcceptsVersionTwoContext(t *testing.T) {
	directory := t.TempDir()
	tracePath := filepath.Join(directory, "trace.json")
	checkersPath := filepath.Join(directory, "checkers.json")
	contextPath := filepath.Join(directory, "context.json")

	writeTestFile(t, tracePath, `{
		"version":1,
		"run_id":"context-run",
		"steps":[{
			"run_id":"context-run",
			"step_id":"source",
			"agent_name":"Source",
			"depends_on":[],
			"input":{},
			"output":{"value":"correct"},
			"model_used":"test",
			"timestamp":"2026-07-28T12:00:00Z",
			"status":"ok"
		}]
	}`)
	writeTestFile(t, checkersPath, `{
		"version":2,
		"steps":{
			"source":{
				"checks":[{
					"expression":"output.value == expected.value",
					"failure_reason":"Source returned the wrong value"
				}]
			}
		}
	}`)
	writeTestFile(t, contextPath, `{"expected":{"value":"correct"}}`)

	err := runTraceCommand([]string{
		"--input", tracePath,
		"--checkers", checkersPath,
		"--context", contextPath,
		"--json",
	})
	if err != nil {
		t.Fatalf("runTraceCommand returned error: %v", err)
	}
}

func TestRunCommandRequiresTargetForMultipleSinks(t *testing.T) {
	directory := t.TempDir()
	tracePath := filepath.Join(directory, "trace.json")
	checkersPath := filepath.Join(directory, "checkers.json")

	writeTestFile(t, tracePath, `{
		"version":1,
		"run_id":"multiple-sinks",
		"steps":[
			{
				"run_id":"multiple-sinks",
				"step_id":"left",
				"agent_name":"Left",
				"depends_on":[],
				"input":{},
				"output":{},
				"model_used":"test",
				"timestamp":"2026-07-30T12:00:00Z",
				"status":"ok"
			},
			{
				"run_id":"multiple-sinks",
				"step_id":"right",
				"agent_name":"Right",
				"depends_on":[],
				"input":{},
				"output":{},
				"model_used":"test",
				"timestamp":"2026-07-30T12:00:01Z",
				"status":"ok"
			}
		]
	}`)
	writeTestFile(t, checkersPath, `{
		"version":2,
		"steps":{
			"left":{
				"checks":[{
					"expression":"true",
					"failure_reason":"left failed"
				}]
			},
			"right":{
				"checks":[{
					"expression":"true",
					"failure_reason":"right failed"
				}]
			}
		}
	}`)

	err := runTraceCommand([]string{
		"--input", tracePath,
		"--checkers", checkersPath,
		"--json",
	})
	assertErrorContains(t, err, "specify at least one target")

	err = runTraceCommand([]string{
		"--input", tracePath,
		"--checkers", checkersPath,
		"--target", "right",
		"--json",
	})
	if err != nil {
		t.Fatalf("runTraceCommand with target returned error: %v", err)
	}
}

// TestRunCommandCanFailOnAttribution verifies the optional CI gate.
//
// Input
// t pointer to testing.T
// Test state and failure reporting.
//
// Output
// None
// The test fails when a failed attribution still returns success.
func TestRunCommandCanFailOnAttribution(t *testing.T) {
	tracePath := filepath.Join(
		"..",
		"..",
		"examples",
		"trace-reference-failure.json",
	)
	checkersPath := filepath.Join(
		"..",
		"..",
		"examples",
		"toy-checkers.json",
	)

	err := runTraceCommand([]string{
		"--input", tracePath,
		"--checkers", checkersPath,
		"--json",
		"--fail-on-attribution",
	})

	assertErrorContains(t, err, `attribution failed at step "reference"`)
	if got := commandExitCode(err); got != exitCodeRuntime {
		t.Fatalf(
			"expected runtime exit code %d, got %d",
			exitCodeRuntime,
			got,
		)
	}
}

// TestRunCommandFailGateAllowsHealthyAttribution verifies successful CI behavior.
//
// Input
// t pointer to testing.T
// Test state and failure reporting.
//
// Output
// None
// The test fails when a healthy attribution returns a gate error.
func TestRunCommandFailGateAllowsHealthyAttribution(t *testing.T) {
	tracePath := filepath.Join(
		"..",
		"..",
		"examples",
		"trace-healthy.json",
	)
	checkersPath := filepath.Join(
		"..",
		"..",
		"examples",
		"toy-checkers.json",
	)

	err := runTraceCommand([]string{
		"--input", tracePath,
		"--checkers", checkersPath,
		"--json",
		"--fail-on-attribution",
	})
	if err != nil {
		t.Fatalf("runTraceCommand returned error: %v", err)
	}
}

func TestDemoCommandRequiresKnownDemo(t *testing.T) {
	assertErrorContains(t, demoCommand(nil), "demo requires a name")
	assertErrorContains(t, demoCommand([]string{"unknown"}), `unknown demo "unknown"`)
}

func TestIncidentDemoRejectsUnknownFailureMode(t *testing.T) {
	err := demoIncidentCommand([]string{"--failure", "unknown"})
	assertErrorContains(t, err, `unsupported incident failure mode "unknown"`)
}

func TestDocumentDemoRejectsUnknownFailureMode(t *testing.T) {
	err := demoDocumentCommand([]string{"--failure", "unknown"})
	assertErrorContains(t, err, `unsupported document failure mode "unknown"`)
}

func TestDocumentDemoWritesRecordedTrace(t *testing.T) {
	tracePath := filepath.Join(t.TempDir(), "document-trace.json")

	err := demoDocumentCommand([]string{
		"--failure", "none",
		"--trace-output", tracePath,
		"--checkers", filepath.Join("..", "..", "examples", "document-checkers-v2.json"),
		"--context", filepath.Join("..", "..", "examples", "document-context.json"),
		"--json",
	})
	if err != nil {
		t.Fatalf("demoDocumentCommand returned error: %v", err)
	}

	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read recorded trace: %v", err)
	}
	if !strings.Contains(string(data), `"version": 1`) {
		t.Fatalf("expected versioned trace, got %s", data)
	}
}

func TestReadLimitedFileRejectsOversizedInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.json")
	writeTestFile(t, path, strings.Repeat("x", 11))

	_, err := readLimitedFile(path, 10, "test input")
	assertErrorContains(t, err, "exceeds byte limit 10")
}

func TestFitEvidenceTextBoundsHumanOutput(t *testing.T) {
	text := strings.Repeat("x", maxEvidenceTextSize+10)

	got := fitEvidenceText(text)

	if len(got) >= len(text) {
		t.Fatalf("expected bounded evidence, got length %d", len(got))
	}
	if !strings.HasSuffix(got, "... truncated") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
}

func TestCommandExitCodeDistinguishesUsageAndRuntimeErrors(t *testing.T) {
	if got := commandExitCode(runCommandLine(nil)); got != exitCodeUsage {
		t.Fatalf("expected usage exit code %d, got %d", exitCodeUsage, got)
	}
	if got := commandExitCode(errors.New("runtime failure")); got != exitCodeRuntime {
		t.Fatalf("expected runtime exit code %d, got %d", exitCodeRuntime, got)
	}
}

func TestWriteCommandErrorSupportsJSON(t *testing.T) {
	var output bytes.Buffer
	err := newUsageError("missing required input")

	writeCommandError(&output, err, "json")

	var record map[string]any
	if decodeErr := json.Unmarshal(output.Bytes(), &record); decodeErr != nil {
		t.Fatalf("decode JSON error record: %v", decodeErr)
	}
	if record["level"] != "error" {
		t.Fatalf("expected error level, got %v", record["level"])
	}
	if record["message"] != "missing required input" {
		t.Fatalf("unexpected message %v", record["message"])
	}
	if record["exit_code"] != float64(exitCodeUsage) {
		t.Fatalf("expected exit code %d, got %v", exitCodeUsage, record["exit_code"])
	}
}

// assertErrorContains verifies that an error contains the expected text.
//
// Input
// t *testing.T
// Active test state.
//
// err error
// Error returned by the function under test.
//
// expected string
// Text expected inside the error message.
//
// Output
// None
func assertErrorContains(t *testing.T, err error, expected string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected error containing %q, got %q", expected, err)
	}
}

// writeTestFile writes one temporary CLI input.
//
// Input
// t pointer to testing.T
// Active test state.
//
// path string
// Destination file path.
//
// contents string
// File contents.
//
// Output
// None
func writeTestFile(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
