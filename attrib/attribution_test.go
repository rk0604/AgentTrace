package attrib_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
)

func TestFindRootCauseStopsAtFirstFailureInDependencyOrder(t *testing.T) {
	trace := attrib.Trace{
		RunID: "run-failed",
		Steps: []attrib.Step{
			{StepID: "final", AgentName: "Final", DependsOn: []string{"middle"}},
			{StepID: "middle", AgentName: "Middle", DependsOn: []string{"source"}},
			{StepID: "source", AgentName: "Source"},
		},
	}
	called := make([]string, 0)
	checkers := map[string]attrib.StepChecker{
		"source": checkerThatRecords(&called, true, ""),
		"middle": checkerThatRecords(&called, false, "middle produced incorrect data"),
		"final":  checkerThatRecords(&called, false, "final also looks incorrect"),
	}

	result, err := attrib.FindRootCause(trace, checkers)
	if err != nil {
		t.Fatalf("FindRootCause returned error: %v", err)
	}

	if result.Status != "failed" {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if result.RootCause == nil {
		t.Fatal("expected a root cause")
	}
	if result.RootCause.StepID != "middle" {
		t.Fatalf("expected middle as root cause, got %q", result.RootCause.StepID)
	}
	if result.RootCause.AgentName != "Middle" {
		t.Fatalf("expected agent name Middle, got %q", result.RootCause.AgentName)
	}
	if result.RootCause.Reason != "middle produced incorrect data" {
		t.Fatalf("unexpected root cause reason %q", result.RootCause.Reason)
	}
	assertStrings(t, result.CheckedStepIDs, []string{"source", "middle"})
	assertStrings(t, called, []string{"source", "middle"})
}

func TestFindRootCauseReturnsPassedWhenEveryStepPasses(t *testing.T) {
	trace := attrib.Trace{
		RunID: "run-passed",
		Steps: []attrib.Step{
			{StepID: "source"},
			{StepID: "final", DependsOn: []string{"source"}},
		},
	}
	checkers := map[string]attrib.StepChecker{
		"source": func(attrib.Step) (attrib.CheckResult, error) { return attrib.Pass(), nil },
		"final":  func(attrib.Step) (attrib.CheckResult, error) { return attrib.Pass(), nil },
	}

	result, err := attrib.FindRootCause(trace, checkers)
	if err != nil {
		t.Fatalf("FindRootCause returned error: %v", err)
	}

	if result.Status != "passed" {
		t.Fatalf("expected passed status, got %q", result.Status)
	}
	if result.RootCause != nil {
		t.Fatalf("expected no root cause, got %+v", result.RootCause)
	}
	assertStrings(t, result.CheckedStepIDs, []string{"source", "final"})
}

func TestFindRootCauseReturnsErrorForMissingChecker(t *testing.T) {
	trace := attrib.Trace{Steps: []attrib.Step{{StepID: "source"}}}

	_, err := attrib.FindRootCause(trace, map[string]attrib.StepChecker{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), `missing checker for step "source"`) {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestFindRootCauseReturnsCheckerErrorWithStepID(t *testing.T) {
	trace := attrib.Trace{Steps: []attrib.Step{{StepID: "source"}}}
	checkers := map[string]attrib.StepChecker{
		"source": func(attrib.Step) (attrib.CheckResult, error) {
			return attrib.CheckResult{}, errors.New("evaluator unavailable")
		},
	}

	_, err := attrib.FindRootCause(trace, checkers)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), `check step "source": evaluator unavailable`) {
		t.Fatalf("unexpected error %q", err)
	}
}

func checkerThatRecords(called *[]string, passed bool, reason string) attrib.StepChecker {
	return func(step attrib.Step) (attrib.CheckResult, error) {
		*called = append(*called, step.StepID)
		if passed {
			return attrib.Pass(), nil
		}

		return attrib.Fail(reason), nil
	}
}

func assertStrings(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
