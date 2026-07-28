package attrib_test

import (
	"encoding/json"
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

func TestFindRootCauseReturnsEvidenceAndDownstreamImpact(t *testing.T) {
	trace := attrib.Trace{
		RunID: "evidence-run",
		Steps: []attrib.Step{
			{
				StepID: "source",
				Input:  json.RawMessage(`{"document":"input"}`),
				Output: json.RawMessage(`{"value":"wrong"}`),
			},
			{StepID: "left", DependsOn: []string{"source"}},
			{StepID: "right", DependsOn: []string{"source"}},
			{StepID: "merge", DependsOn: []string{"left", "right"}},
			{StepID: "independent"},
		},
	}
	expected := json.RawMessage(`{"value":"correct"}`)
	checkers := map[string]attrib.StepChecker{
		"source": func(attrib.Step) (attrib.CheckResult, error) {
			return attrib.FailWithEvidence(
				"Source returned the wrong value",
				"output.value == expected.value",
				expected,
			), nil
		},
		"left":        func(attrib.Step) (attrib.CheckResult, error) { return attrib.Pass(), nil },
		"right":       func(attrib.Step) (attrib.CheckResult, error) { return attrib.Pass(), nil },
		"merge":       func(attrib.Step) (attrib.CheckResult, error) { return attrib.Pass(), nil },
		"independent": func(attrib.Step) (attrib.CheckResult, error) { return attrib.Pass(), nil },
	}

	result, err := attrib.FindRootCause(trace, checkers)
	if err != nil {
		t.Fatalf("FindRootCause returned error: %v", err)
	}
	if result.RootCause == nil {
		t.Fatal("expected a root cause")
	}
	if result.RootCause.Expression != "output.value == expected.value" {
		t.Fatalf("unexpected expression %q", result.RootCause.Expression)
	}
	if string(result.RootCause.Input) != `{"document":"input"}` {
		t.Fatalf("unexpected input evidence %s", result.RootCause.Input)
	}
	if string(result.RootCause.Output) != `{"value":"wrong"}` {
		t.Fatalf("unexpected output evidence %s", result.RootCause.Output)
	}
	if string(result.RootCause.Expected) != string(expected) {
		t.Fatalf("unexpected expected evidence %s", result.RootCause.Expected)
	}
	assertStrings(t, result.AffectedStepIDs, []string{"left", "right", "merge"})

	wantEdges := []attrib.CauseEdge{
		{FromStepID: "source", ToStepID: "left"},
		{FromStepID: "source", ToStepID: "right"},
		{FromStepID: "left", ToStepID: "merge"},
		{FromStepID: "right", ToStepID: "merge"},
	}
	if len(result.CauseEdges) != len(wantEdges) {
		t.Fatalf("expected cause edges %v, got %v", wantEdges, result.CauseEdges)
	}
	for index := range wantEdges {
		if result.CauseEdges[index] != wantEdges[index] {
			t.Fatalf("expected cause edges %v, got %v", wantEdges, result.CauseEdges)
		}
	}
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

func TestFindRootCauseValidatesAllCheckersBeforeEvaluation(t *testing.T) {
	trace := attrib.Trace{
		Steps: []attrib.Step{
			{StepID: "source"},
			{StepID: "final", DependsOn: []string{"source"}},
		},
	}
	called := false
	checkers := map[string]attrib.StepChecker{
		"source": func(attrib.Step) (attrib.CheckResult, error) {
			called = true
			return attrib.Fail("source failed"), nil
		},
	}

	_, err := attrib.FindRootCause(trace, checkers)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), `missing checker for step "final"`) {
		t.Fatalf("unexpected error %q", err)
	}
	if called {
		t.Fatal("expected checker validation before evaluation")
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

func TestValidateAcceptsValidGraphAndCheckerCoverage(t *testing.T) {
	trace := attrib.Trace{
		Steps: []attrib.Step{
			{StepID: "source"},
			{StepID: "final", DependsOn: []string{"source"}},
		},
	}
	checkers := map[string]attrib.StepChecker{
		"source": func(attrib.Step) (attrib.CheckResult, error) { return attrib.Pass(), nil },
		"final":  func(attrib.Step) (attrib.CheckResult, error) { return attrib.Pass(), nil },
	}

	if err := attrib.Validate(trace, checkers); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRejectsInvalidDependencyGraph(t *testing.T) {
	trace := attrib.Trace{
		Steps: []attrib.Step{
			{StepID: "source", DependsOn: []string{"missing"}},
		},
	}

	err := attrib.Validate(trace, map[string]attrib.StepChecker{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), `step "source" depends on unknown step "missing"`) {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestValidateRejectsMissingCheckerWithoutEvaluation(t *testing.T) {
	trace := attrib.Trace{
		Steps: []attrib.Step{
			{StepID: "source"},
			{StepID: "final", DependsOn: []string{"source"}},
		},
	}
	called := false
	checkers := map[string]attrib.StepChecker{
		"source": func(attrib.Step) (attrib.CheckResult, error) {
			called = true
			return attrib.Pass(), nil
		},
	}

	err := attrib.Validate(trace, checkers)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), `missing checker for step "final"`) {
		t.Fatalf("unexpected error %q", err)
	}
	if called {
		t.Fatal("expected validation not to evaluate checkers")
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
