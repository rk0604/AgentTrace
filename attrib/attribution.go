package attrib

import (
	"encoding/json"
	"fmt"
)

type StepChecker func(Step) (CheckResult, error)

type CheckResult struct {
	Passed     bool
	Reason     string
	Expression string
	Expected   json.RawMessage
}

type RootCause struct {
	StepID     string          `json:"step_id"`
	AgentName  string          `json:"agent_name"`
	Reason     string          `json:"reason"`
	Expression string          `json:"expression,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	Expected   json.RawMessage `json:"expected,omitempty"`
}

// CauseEdge identifies one downstream propagation edge.
type CauseEdge struct {
	FromStepID string `json:"from_step_id"`
	ToStepID   string `json:"to_step_id"`
}

type AttributionResult struct {
	RunID           string      `json:"run_id"`
	Status          string      `json:"status"`
	RootCause       *RootCause  `json:"root_cause,omitempty"`
	CheckedStepIDs  []string    `json:"checked_step_ids"`
	AffectedStepIDs []string    `json:"affected_step_ids,omitempty"`
	CauseEdges      []CauseEdge `json:"cause_edges,omitempty"`
}

func FindRootCause(trace Trace, checkers map[string]StepChecker) (AttributionResult, error) {
	orderedSteps, err := TopologicalSort(trace)
	if err != nil {
		return AttributionResult{}, err
	}

	if err := validateCheckerCoverage(orderedSteps, checkers); err != nil {
		return AttributionResult{}, err
	}

	result := AttributionResult{
		RunID:          trace.RunID,
		Status:         "passed",
		CheckedStepIDs: make([]string, 0, len(orderedSteps)),
	}

	for _, step := range orderedSteps {
		checker := checkers[step.StepID]

		check, err := checker(step)
		if err != nil {
			return AttributionResult{}, fmt.Errorf("check step %q: %w", step.StepID, err)
		}

		result.CheckedStepIDs = append(result.CheckedStepIDs, step.StepID)
		if !check.Passed {
			affectedStepIDs, causeEdges := downstreamImpact(orderedSteps, step.StepID)
			result.Status = "failed"
			result.RootCause = &RootCause{
				StepID:     step.StepID,
				AgentName:  step.AgentName,
				Reason:     check.Reason,
				Expression: check.Expression,
				Input:      cloneRawMessage(step.Input),
				Output:     cloneRawMessage(step.Output),
				Expected:   cloneRawMessage(check.Expected),
			}
			result.AffectedStepIDs = affectedStepIDs
			result.CauseEdges = causeEdges
			return result, nil
		}
	}

	return result, nil
}

// Validate checks a trace dependency graph and its checker coverage without evaluating checkers.
//
// Input
// trace Trace
// Trace containing the dependency graph to validate.
//
// checkers map[string]StepChecker
// Step checker functions keyed by step ID.
//
// Output
// error
// Non nil when the graph is invalid or a step has no checker.
func Validate(trace Trace, checkers map[string]StepChecker) error {
	orderedSteps, err := TopologicalSort(trace)
	if err != nil {
		return err
	}

	return validateCheckerCoverage(orderedSteps, checkers)
}

// validateCheckerCoverage confirms that every ordered step has a checker function.
//
// Input
// orderedSteps []Step
// Steps in dependency order.
//
// checkers map[string]StepChecker
// Step checker functions keyed by step ID.
//
// Output
// error
// Non nil when a step has no checker function.
func validateCheckerCoverage(orderedSteps []Step, checkers map[string]StepChecker) error {
	for _, step := range orderedSteps {
		checker, exists := checkers[step.StepID]
		if !exists || checker == nil {
			return fmt.Errorf("missing checker for step %q", step.StepID)
		}
	}

	return nil
}

func Pass() CheckResult {
	return CheckResult{Passed: true}
}

func Fail(reason string) CheckResult {
	return CheckResult{Passed: false, Reason: reason}
}

// FailWithEvidence creates a failed check with evaluator evidence.
//
// Input
// reason string
// Human readable failure explanation.
//
// expression string
// Correctness expression that returned false.
//
// expected json.RawMessage
// Optional expected evaluation context.
//
// Output
// CheckResult
// Failed check result containing copied evidence.
func FailWithEvidence(reason string, expression string, expected json.RawMessage) CheckResult {
	return CheckResult{
		Passed:     false,
		Reason:     reason,
		Expression: expression,
		Expected:   cloneRawMessage(expected),
	}
}

// downstreamImpact finds every descendant of the root cause.
//
// Input
// orderedSteps slice of Step
// Trace steps in dependency order.
//
// rootCauseStepID string
// Step that introduced the failed output.
//
// Output
// slice of string
// Descendant step IDs in dependency order.
//
// slice of CauseEdge
// Dependency edges that carry data from the failed branch.
func downstreamImpact(orderedSteps []Step, rootCauseStepID string) ([]string, []CauseEdge) {
	affected := map[string]bool{rootCauseStepID: true}
	affectedStepIDs := make([]string, 0)
	causeEdges := make([]CauseEdge, 0)

	for _, step := range orderedSteps {
		if step.StepID == rootCauseStepID {
			continue
		}

		stepAffected := false
		for _, dependencyID := range step.DependsOn {
			if !affected[dependencyID] {
				continue
			}

			stepAffected = true
			causeEdges = append(causeEdges, CauseEdge{
				FromStepID: dependencyID,
				ToStepID:   step.StepID,
			})
		}
		if stepAffected {
			affected[step.StepID] = true
			affectedStepIDs = append(affectedStepIDs, step.StepID)
		}
	}

	return affectedStepIDs, causeEdges
}

func cloneRawMessage(message json.RawMessage) json.RawMessage {
	if len(message) == 0 {
		return nil
	}

	return append(json.RawMessage(nil), message...)
}
