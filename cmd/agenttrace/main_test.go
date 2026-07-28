package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCommandLineRequiresExplicitCommand(t *testing.T) {
	err := runCommandLine(nil)
	assertErrorContains(t, err, "missing command")
}

func TestRunCommandLineRejectsUnknownCommand(t *testing.T) {
	err := runCommandLine([]string{"unknown"})
	assertErrorContains(t, err, `unknown command "unknown"`)
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

func TestDemoCommandRequiresKnownDemo(t *testing.T) {
	assertErrorContains(t, demoCommand(nil), "demo requires a name")
	assertErrorContains(t, demoCommand([]string{"unknown"}), `unknown demo "unknown"`)
}

func TestIncidentDemoRejectsUnknownFailureMode(t *testing.T) {
	err := demoIncidentCommand([]string{"--failure", "unknown"})
	assertErrorContains(t, err, `unsupported incident failure mode "unknown"`)
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
