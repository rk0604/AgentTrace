package toypipeline_test

import (
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/toypipeline"
)

func TestReferenceFailureIsRootCause(t *testing.T) {
	trace := toypipeline.Run(true)

	result, err := attrib.FindRootCause(trace, toypipeline.Checkers())
	if err != nil {
		t.Fatalf("FindRootCause returned error: %v", err)
	}

	if result.Status != "failed" {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if result.RootCause == nil {
		t.Fatal("expected a root cause")
	}
	if result.RootCause.StepID != toypipeline.ReferenceStepID {
		t.Fatalf("expected root cause %q, got %q", toypipeline.ReferenceStepID, result.RootCause.StepID)
	}

	expectedChecked := []string{toypipeline.ExtractorStepID, toypipeline.ReferenceStepID}
	assertStepIDs(t, result.CheckedStepIDs, expectedChecked)
}

func TestHealthyToyRunPasses(t *testing.T) {
	trace := toypipeline.Run(false)

	result, err := attrib.FindRootCause(trace, toypipeline.Checkers())
	if err != nil {
		t.Fatalf("FindRootCause returned error: %v", err)
	}

	if result.Status != "passed" {
		t.Fatalf("expected passed status, got %q", result.Status)
	}
	if result.RootCause != nil {
		t.Fatalf("expected no root cause, got %+v", result.RootCause)
	}

	expectedChecked := []string{
		toypipeline.ExtractorStepID,
		toypipeline.ReferenceStepID,
		toypipeline.ComparatorStepID,
		toypipeline.SynthesizerStepID,
	}
	assertStepIDs(t, result.CheckedStepIDs, expectedChecked)
}

func TestComparatorPassesGivenConsistentBadInputs(t *testing.T) {
	trace := toypipeline.Run(true)
	step := findStep(t, trace, toypipeline.ComparatorStepID)

	check, err := toypipeline.Checkers()[toypipeline.ComparatorStepID](step)
	if err != nil {
		t.Fatalf("comparator checker returned error: %v", err)
	}
	if !check.Passed {
		t.Fatalf("expected comparator to pass given actual inputs, got failure: %s", check.Reason)
	}
}

func findStep(t *testing.T, trace attrib.Trace, stepID string) attrib.Step {
	t.Helper()

	for _, step := range trace.Steps {
		if step.StepID == stepID {
			return step
		}
	}

	t.Fatalf("missing step %q", stepID)
	return attrib.Step{}
}

func assertStepIDs(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected checked steps %v, got %v", want, got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("expected checked steps %v, got %v", want, got)
		}
	}
}
