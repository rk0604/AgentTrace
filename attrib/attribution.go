package attrib

import "fmt"

type StepChecker func(Step) (CheckResult, error)

type CheckResult struct {
	Passed bool
	Reason string
}

type RootCause struct {
	StepID    string `json:"step_id"`
	AgentName string `json:"agent_name"`
	Reason    string `json:"reason"`
}

type AttributionResult struct {
	RunID          string     `json:"run_id"`
	Status         string     `json:"status"`
	RootCause      *RootCause `json:"root_cause,omitempty"`
	CheckedStepIDs []string   `json:"checked_step_ids"`
}

func FindRootCause(trace Trace, checkers map[string]StepChecker) (AttributionResult, error) {
	orderedSteps, err := TopologicalSort(trace)
	if err != nil {
		return AttributionResult{}, err
	}

	// Validate complete checker coverage before treating any result as attribution.
	for _, step := range orderedSteps {
		checker, exists := checkers[step.StepID]
		if !exists || checker == nil {
			return AttributionResult{}, fmt.Errorf("missing checker for step %q", step.StepID)
		}
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
			result.Status = "failed"
			result.RootCause = &RootCause{
				StepID:    step.StepID,
				AgentName: step.AgentName,
				Reason:    check.Reason,
			}
			return result, nil
		}
	}

	return result, nil
}

func Pass() CheckResult {
	return CheckResult{Passed: true}
}

func Fail(reason string) CheckResult {
	return CheckResult{Passed: false, Reason: reason}
}
