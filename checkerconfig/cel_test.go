package checkerconfig_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/checkerconfig"
)

func TestBuildCreatesCheckerThatPassesAndFails(t *testing.T) {
	config := singleStepConfig(`output.revenue == "$4.2M" && output.period == "Q2"`)
	checkers, err := checkerconfig.Build(config)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	passed, err := checkers["reference"](attrib.Step{
		StepID: "reference",
		Output: json.RawMessage(`{"revenue":"$4.2M","period":"Q2"}`),
	})
	if err != nil {
		t.Fatalf("checker returned error: %v", err)
	}
	if !passed.Passed {
		t.Fatalf("expected checker to pass, got %q", passed.Reason)
	}

	failed, err := checkers["reference"](attrib.Step{
		StepID: "reference",
		Output: json.RawMessage(`{"revenue":"$4.2M","period":"Q1"}`),
	})
	if err != nil {
		t.Fatalf("checker returned error: %v", err)
	}
	if failed.Passed {
		t.Fatal("expected checker to fail")
	}
	if failed.Reason != "Reference misread the reporting period" {
		t.Fatalf("unexpected failure reason %q", failed.Reason)
	}
}

func TestBuildCheckerCanCompareActualInputAndOutput(t *testing.T) {
	config := singleStepConfig(`output.result == (input.extractor == input.reference ? "MATCH" : "MISMATCH")`)
	checkers, err := checkerconfig.Build(config)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	check, err := checkers["reference"](attrib.Step{
		StepID: "reference",
		Input: json.RawMessage(`{
			"extractor":{"revenue":"$4.2M","period":"Q1"},
			"reference":{"revenue":"$4.2M","period":"Q1"}
		}`),
		Output: json.RawMessage(`{"result":"MATCH"}`),
	})
	if err != nil {
		t.Fatalf("checker returned error: %v", err)
	}
	if !check.Passed {
		t.Fatalf("expected checker to pass, got %q", check.Reason)
	}
}

func TestBuildCheckerCanInspectGenericStepMetadata(t *testing.T) {
	config := singleStepConfig(`step.agent_name == "Reference" && step.status == "ok"`)
	checkers, err := checkerconfig.Build(config)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	check, err := checkers["reference"](attrib.Step{
		StepID:    "reference",
		AgentName: "Reference",
		Status:    "ok",
	})
	if err != nil {
		t.Fatalf("checker returned error: %v", err)
	}
	if !check.Passed {
		t.Fatalf("expected checker to pass, got %q", check.Reason)
	}
}

func TestBuildRejectsInvalidExpressions(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		wantMessage string
	}{
		{
			name:        "syntax error",
			expression:  "output.value ==",
			wantMessage: "compile checker",
		},
		{
			name:        "non Boolean result",
			expression:  "output.value",
			wantMessage: "expression must return bool",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := checkerconfig.Build(singleStepConfig(test.expression))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("expected error containing %q, got %q", test.wantMessage, err)
			}
		})
	}
}

func TestConfiguredCheckerReturnsEvaluationErrors(t *testing.T) {
	config := singleStepConfig(`output.value == "correct"`)
	checkers, err := checkerconfig.Build(config)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	_, err = checkers["reference"](attrib.Step{
		StepID: "reference",
		Output: json.RawMessage(`{"different":"field"}`),
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "evaluate CEL expression") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestConfiguredCheckerRejectsInvalidPayloadJSON(t *testing.T) {
	config := singleStepConfig(`true`)
	checkers, err := checkerconfig.Build(config)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	_, err = checkers["reference"](attrib.Step{
		StepID: "reference",
		Output: json.RawMessage(`{"broken"`),
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "decode step output") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestBuildWithOptionsExposesRunContextAndExpectedValues(t *testing.T) {
	config := checkerconfig.Config{
		Version: checkerconfig.CurrentVersion,
		Steps: map[string]checkerconfig.StepConfig{
			"source": {
				Checks: []checkerconfig.CheckConfig{
					{
						Expression:    `run.run_id == "context-run" && context.environment == "test"`,
						FailureReason: "Run context did not match",
					},
					{
						Expression:    `output.value == expected.value`,
						FailureReason: "Output did not match expected value",
					},
				},
			},
		},
	}
	trace := attrib.Trace{
		Version: attrib.CurrentTraceVersion,
		RunID:   "context-run",
		Steps:   []attrib.Step{{StepID: "source"}},
	}
	checkers, err := checkerconfig.BuildWithOptions(config, checkerconfig.BuildOptions{
		Trace:   &trace,
		Context: json.RawMessage(`{"environment":"test","expected":{"value":"correct"}}`),
	})
	if err != nil {
		t.Fatalf("BuildWithOptions returned error: %v", err)
	}

	check, err := checkers["source"](attrib.Step{
		StepID: "source",
		Output: json.RawMessage(`{"value":"correct"}`),
	})
	if err != nil {
		t.Fatalf("checker returned error: %v", err)
	}
	if !check.Passed {
		t.Fatalf("expected checker to pass, got %q", check.Reason)
	}
}

func TestVersionTwoStopsAtFirstFailedAssertion(t *testing.T) {
	config := checkerconfig.Config{
		Version: checkerconfig.CurrentVersion,
		Steps: map[string]checkerconfig.StepConfig{
			"source": {
				Checks: []checkerconfig.CheckConfig{
					{
						Expression:    `hasField(output, "value")`,
						FailureReason: "Output is missing value",
					},
					{
						Expression:    `output.value == "correct"`,
						FailureReason: "Output has the wrong value",
					},
				},
			},
		},
	}
	checkers, err := checkerconfig.Build(config)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	check, err := checkers["source"](attrib.Step{
		StepID: "source",
		Output: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("checker returned error: %v", err)
	}
	if check.Passed {
		t.Fatal("expected checker to fail")
	}
	if check.Reason != "Output is missing value" {
		t.Fatalf("unexpected failure reason %q", check.Reason)
	}
}

func TestGenericCELHelpers(t *testing.T) {
	expression := strings.Join([]string{
		`hasField(output, "name")`,
		`isString(output.name)`,
		`isNumber(output.score)`,
		`isNonEmpty(output.name)`,
		`withinRange(output.score, 0.8, 1.0)`,
		`equalsExpected(output.name, expected.name)`,
		`sameField(output, expected, "name")`,
	}, " && ")
	config := singleStepConfig(expression)
	checkers, err := checkerconfig.BuildWithOptions(config, checkerconfig.BuildOptions{
		Context: json.RawMessage(`{"expected":{"name":"AgentTrace"}}`),
	})
	if err != nil {
		t.Fatalf("BuildWithOptions returned error: %v", err)
	}

	check, err := checkers["reference"](attrib.Step{
		StepID: "reference",
		Output: json.RawMessage(`{"name":"AgentTrace","score":0.92}`),
	})
	if err != nil {
		t.Fatalf("checker returned error: %v", err)
	}
	if !check.Passed {
		t.Fatalf("expected helper expression to pass, got %q", check.Reason)
	}
}

func TestBuildWithOptionsRejectsInvalidContext(t *testing.T) {
	tests := []struct {
		name        string
		options     checkerconfig.BuildOptions
		wantMessage string
	}{
		{
			name:        "invalid JSON",
			options:     checkerconfig.BuildOptions{Context: json.RawMessage(`{"broken"`)},
			wantMessage: "decode step context",
		},
		{
			name:        "non object context",
			options:     checkerconfig.BuildOptions{Context: json.RawMessage(`[]`)},
			wantMessage: "top level value must be an object",
		},
		{
			name:        "negative timeout",
			options:     checkerconfig.BuildOptions{EvaluationTimeout: -time.Second},
			wantMessage: "timeout cannot be negative",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := checkerconfig.BuildWithOptions(singleStepConfig("true"), test.options)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("expected error containing %q, got %q", test.wantMessage, err)
			}
		})
	}
}

func singleStepConfig(expression string) checkerconfig.Config {
	return checkerconfig.Config{
		Version: checkerconfig.CurrentVersion,
		Steps: map[string]checkerconfig.StepConfig{
			"reference": {
				Expression:    expression,
				FailureReason: "Reference misread the reporting period",
			},
		},
	}
}
