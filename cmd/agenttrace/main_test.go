package main

import (
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
