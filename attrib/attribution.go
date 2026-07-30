package attrib

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CurrentResultVersion identifies the current attribution result schema.
const CurrentResultVersion = 2

// StepChecker evaluates one step against its actual inputs.
type StepChecker func(Step) (CheckResult, error)

// AnalysisOptions selects the output steps whose ancestry should be analyzed.
type AnalysisOptions struct {
	TargetStepIDs []string
}

// CheckResult contains one evaluator decision and optional evidence.
type CheckResult struct {
	Passed     bool
	Reason     string
	Expression string
	Expected   json.RawMessage
}

// RootCause identifies the first failed step and its evidence.
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

// Divergence summarizes one failed correctness check.
type Divergence struct {
	StepID         string `json:"step_id"`
	AgentName      string `json:"agent_name"`
	Reason         string `json:"reason"`
	Expression     string `json:"expression,omitempty"`
	Classification string `json:"classification"`
}

// AttributionResult contains the final first divergence analysis.
type AttributionResult struct {
	Version                    int          `json:"version"`
	RunID                      string       `json:"run_id"`
	Status                     string       `json:"status"`
	TargetStepIDs              []string     `json:"target_step_ids,omitempty"`
	RootCause                  *RootCause   `json:"root_cause,omitempty"`
	RootCauses                 []RootCause  `json:"root_causes,omitempty"`
	Divergences                []Divergence `json:"divergences,omitempty"`
	SecondaryDivergences       []RootCause  `json:"secondary_divergences,omitempty"`
	CheckedStepIDs             []string     `json:"checked_step_ids"`
	DownstreamCandidateStepIDs []string     `json:"downstream_candidate_step_ids,omitempty"`
	PotentialPropagationEdges  []CauseEdge  `json:"potential_propagation_edges,omitempty"`
	AffectedStepIDs            []string     `json:"affected_step_ids,omitempty"`
	CauseEdges                 []CauseEdge  `json:"cause_edges,omitempty"`
}

// OmitEvidence returns a result without raw pipeline payloads.
//
// Input
// result AttributionResult
// Completed attribution result containing optional evidence.
//
// Output
// AttributionResult
// Detached summary with input, output, and expected payloads removed.
func OmitEvidence(result AttributionResult) AttributionResult {
	summary := result
	if result.RootCause != nil {
		rootCause := rootCauseWithoutEvidence(*result.RootCause)
		summary.RootCause = &rootCause
	}

	summary.RootCauses = make(
		[]RootCause,
		len(result.RootCauses),
	)
	for index, rootCause := range result.RootCauses {
		summary.RootCauses[index] = rootCauseWithoutEvidence(rootCause)
	}

	summary.SecondaryDivergences = make(
		[]RootCause,
		len(result.SecondaryDivergences),
	)
	for index, divergence := range result.SecondaryDivergences {
		summary.SecondaryDivergences[index] = rootCauseWithoutEvidence(
			divergence,
		)
	}

	return summary
}

// rootCauseWithoutEvidence copies root cause metadata without raw payloads.
//
// Input
// rootCause RootCause
// Root cause containing optional evidence.
//
// Output
// RootCause
// Root cause with input, output, and expected payloads removed.
func rootCauseWithoutEvidence(rootCause RootCause) RootCause {
	rootCause.Input = nil
	rootCause.Output = nil
	rootCause.Expected = nil
	return rootCause
}

// FindRootCause evaluates steps in dependency order.
//
// Input
// trace Trace
// Completed pipeline trace.
//
// checkers map of string to StepChecker
// Correctness checkers keyed by step ID.
//
// Output
// AttributionResult
// Passed result or first divergence with downstream impact.
//
// error
// Non nil when graph, checker coverage, or evaluation fails.
func FindRootCause(trace Trace, checkers map[string]StepChecker) (AttributionResult, error) {
	orderedSteps, err := validateAndOrderTrace(trace)
	if err != nil {
		return AttributionResult{}, err
	}

	if err := validateCheckerCoverage(orderedSteps, checkers); err != nil {
		return AttributionResult{}, err
	}

	result := AttributionResult{
		Version:        CurrentResultVersion,
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
			rootCause := rootCauseFromCheck(step, check)
			legacyRootCause := rootCause
			result.Status = "failed"
			result.RootCause = &legacyRootCause
			result.RootCauses = []RootCause{rootCause}
			result.Divergences = []Divergence{
				divergenceFromRootCause(rootCause, "root_cause"),
			}
			result.DownstreamCandidateStepIDs = append(
				[]string(nil),
				affectedStepIDs...,
			)
			result.PotentialPropagationEdges = append(
				[]CauseEdge(nil),
				causeEdges...,
			)
			result.AffectedStepIDs = affectedStepIDs
			result.CauseEdges = causeEdges
			return result, nil
		}
	}

	return result, nil
}

// Analyze evaluates every relevant step for one or more target outputs.
//
// Input
// trace Trace
// Completed pipeline trace.
//
// checkers map of string to StepChecker
// Correctness checkers keyed by step ID.
//
// options AnalysisOptions
// Explicit targets or an empty selection for automatic single sink resolution.
//
// Output
// AttributionResult
// Target aware result containing root and secondary divergences.
//
// error
// Non nil when trace, target, checker coverage, or evaluation fails.
func Analyze(
	trace Trace,
	checkers map[string]StepChecker,
	options AnalysisOptions,
) (AttributionResult, error) {
	orderedSteps, err := validateAndOrderTrace(trace)
	if err != nil {
		return AttributionResult{}, err
	}
	if err := validateCheckerCoverage(orderedSteps, checkers); err != nil {
		return AttributionResult{}, err
	}

	targetStepIDs, err := resolveTargetStepIDs(orderedSteps, options.TargetStepIDs)
	if err != nil {
		return AttributionResult{}, err
	}
	relevantStepIDs := ancestorStepSet(orderedSteps, targetStepIDs)

	result := AttributionResult{
		Version:        CurrentResultVersion,
		RunID:          trace.RunID,
		Status:         "passed",
		TargetStepIDs:  append([]string(nil), targetStepIDs...),
		CheckedStepIDs: make([]string, 0, len(relevantStepIDs)),
	}
	failedByID := make(map[string]RootCause)

	for _, step := range orderedSteps {
		if !relevantStepIDs[step.StepID] {
			continue
		}

		check, checkErr := checkers[step.StepID](step)
		if checkErr != nil {
			return AttributionResult{}, fmt.Errorf(
				"check step %q: %w",
				step.StepID,
				checkErr,
			)
		}

		result.CheckedStepIDs = append(result.CheckedStepIDs, step.StepID)
		if !check.Passed {
			failedByID[step.StepID] = rootCauseFromCheck(step, check)
		}
	}

	if len(failedByID) == 0 {
		return result, nil
	}

	result.Status = "failed"
	failedUpstream := make(map[string]bool, len(relevantStepIDs))
	for _, step := range orderedSteps {
		if !relevantStepIDs[step.StepID] {
			continue
		}

		hasFailedAncestor := false
		for _, dependencyID := range step.DependsOn {
			if failedUpstream[dependencyID] {
				hasFailedAncestor = true
				break
			}
		}

		failure, failed := failedByID[step.StepID]
		if failed {
			classification := "root_cause"
			if hasFailedAncestor {
				classification = "secondary_divergence"
				result.SecondaryDivergences = append(
					result.SecondaryDivergences,
					failure,
				)
			} else {
				result.RootCauses = append(result.RootCauses, failure)
			}
			result.Divergences = append(
				result.Divergences,
				divergenceFromRootCause(failure, classification),
			)
		}

		failedUpstream[step.StepID] = failed || hasFailedAncestor
	}

	if len(result.RootCauses) > 0 {
		legacyRootCause := result.RootCauses[0]
		result.RootCause = &legacyRootCause
	}

	downstreamStepIDs, propagationEdges := downstreamImpactForRoots(
		orderedSteps,
		relevantStepIDs,
		result.RootCauses,
	)
	result.DownstreamCandidateStepIDs = downstreamStepIDs
	result.PotentialPropagationEdges = propagationEdges
	result.AffectedStepIDs = append([]string(nil), downstreamStepIDs...)
	result.CauseEdges = append([]CauseEdge(nil), propagationEdges...)

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
	orderedSteps, err := validateAndOrderTrace(trace)
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
	stepIDs := make(map[string]bool, len(orderedSteps))
	for _, step := range orderedSteps {
		stepIDs[step.StepID] = true
		checker, exists := checkers[step.StepID]
		if !exists || checker == nil {
			return fmt.Errorf("missing checker for step %q", step.StepID)
		}
	}
	for stepID, checker := range checkers {
		if !stepIDs[stepID] {
			return fmt.Errorf("checker configured for unknown step %q", stepID)
		}
		if checker == nil {
			return fmt.Errorf("missing checker for step %q", stepID)
		}
	}

	return nil
}

// resolveTargetStepIDs validates explicit targets or selects one sink step.
//
// Input
// orderedSteps slice of Step
// Validated trace steps in dependency order.
//
// configuredTargets slice of string
// Optional explicit target step IDs.
//
// Output
// slice of string
// Validated target IDs.
//
// error
// Non nil when a target is empty, repeated, unknown, or ambiguous.
func resolveTargetStepIDs(
	orderedSteps []Step,
	configuredTargets []string,
) ([]string, error) {
	stepIDs := make(map[string]bool, len(orderedSteps))
	hasDependent := make(map[string]bool, len(orderedSteps))
	for _, step := range orderedSteps {
		stepIDs[step.StepID] = true
		for _, dependencyID := range step.DependsOn {
			hasDependent[dependencyID] = true
		}
	}

	if len(configuredTargets) > 0 {
		targets := make([]string, 0, len(configuredTargets))
		seen := make(map[string]bool, len(configuredTargets))
		for _, targetID := range configuredTargets {
			if strings.TrimSpace(targetID) == "" {
				return nil, fmt.Errorf("target step ID is empty")
			}
			if !stepIDs[targetID] {
				return nil, fmt.Errorf("target step %q does not exist", targetID)
			}
			if seen[targetID] {
				return nil, fmt.Errorf("target step %q is repeated", targetID)
			}
			seen[targetID] = true
			targets = append(targets, targetID)
		}
		return targets, nil
	}

	sinks := make([]string, 0)
	for _, step := range orderedSteps {
		if !hasDependent[step.StepID] {
			sinks = append(sinks, step.StepID)
		}
	}
	if len(sinks) == 1 {
		return sinks, nil
	}

	return nil, fmt.Errorf(
		"trace has %d sink steps; specify at least one target step",
		len(sinks),
	)
}

// ancestorStepSet returns every step that can reach a selected target.
//
// Input
// orderedSteps slice of Step
// Validated trace steps.
//
// targetStepIDs slice of string
// Selected output steps.
//
// Output
// map of string to bool
// Target and ancestor step IDs.
func ancestorStepSet(
	orderedSteps []Step,
	targetStepIDs []string,
) map[string]bool {
	stepsByID := make(map[string]Step, len(orderedSteps))
	for _, step := range orderedSteps {
		stepsByID[step.StepID] = step
	}

	relevant := make(map[string]bool)
	pending := append([]string(nil), targetStepIDs...)
	for len(pending) > 0 {
		lastIndex := len(pending) - 1
		stepID := pending[lastIndex]
		pending = pending[:lastIndex]
		if relevant[stepID] {
			continue
		}

		relevant[stepID] = true
		pending = append(pending, stepsByID[stepID].DependsOn...)
	}

	return relevant
}

// rootCauseFromCheck copies one failed checker decision into result evidence.
//
// Input
// step Step
// Failed trace step.
//
// check CheckResult
// Failed checker decision.
//
// Output
// RootCause
// Detached failure evidence.
func rootCauseFromCheck(step Step, check CheckResult) RootCause {
	return RootCause{
		StepID:     step.StepID,
		AgentName:  step.AgentName,
		Reason:     check.Reason,
		Expression: check.Expression,
		Input:      cloneRawMessage(step.Input),
		Output:     cloneRawMessage(step.Output),
		Expected:   cloneRawMessage(check.Expected),
	}
}

// divergenceFromRootCause creates one compact divergence summary.
//
// Input
// cause RootCause
// Full failed check evidence.
//
// classification string
// Root cause or secondary divergence label.
//
// Output
// Divergence
// Compact failed check summary.
func divergenceFromRootCause(
	cause RootCause,
	classification string,
) Divergence {
	return Divergence{
		StepID:         cause.StepID,
		AgentName:      cause.AgentName,
		Reason:         cause.Reason,
		Expression:     cause.Expression,
		Classification: classification,
	}
}

// Pass creates a successful check result.
//
// Input
// None
//
// Output
// CheckResult
// Successful evaluator result.
func Pass() CheckResult {
	return CheckResult{Passed: true}
}

// Fail creates a failed check result without structured evidence.
//
// Input
// reason string
// Human readable failure explanation.
//
// Output
// CheckResult
// Failed evaluator result.
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

// downstreamImpactForRoots finds target relevant descendants of root causes.
//
// Input
// orderedSteps slice of Step
// Trace steps in dependency order.
//
// relevantStepIDs map of string to bool
// Target and ancestor steps included in analysis.
//
// rootCauses slice of RootCause
// Primary upstream divergences.
//
// Output
// slice of string
// Downstream candidate IDs in dependency order.
//
// slice of CauseEdge
// Potential propagation edges inside the target ancestry.
func downstreamImpactForRoots(
	orderedSteps []Step,
	relevantStepIDs map[string]bool,
	rootCauses []RootCause,
) ([]string, []CauseEdge) {
	propagated := make(map[string]bool, len(relevantStepIDs))
	rootStepIDs := make(map[string]bool, len(rootCauses))
	for _, rootCause := range rootCauses {
		propagated[rootCause.StepID] = true
		rootStepIDs[rootCause.StepID] = true
	}

	downstreamStepIDs := make([]string, 0)
	propagationEdges := make([]CauseEdge, 0)
	for _, step := range orderedSteps {
		if !relevantStepIDs[step.StepID] || rootStepIDs[step.StepID] {
			continue
		}

		stepDownstream := false
		for _, dependencyID := range step.DependsOn {
			if !propagated[dependencyID] {
				continue
			}

			stepDownstream = true
			propagationEdges = append(propagationEdges, CauseEdge{
				FromStepID: dependencyID,
				ToStepID:   step.StepID,
			})
		}
		if stepDownstream {
			propagated[step.StepID] = true
			downstreamStepIDs = append(downstreamStepIDs, step.StepID)
		}
	}

	return downstreamStepIDs, propagationEdges
}

// cloneRawMessage creates an isolated copy of raw JSON.
//
// Input
// message json.RawMessage
// Raw JSON value to copy.
//
// Output
// json.RawMessage
// Detached bytes or nil.
func cloneRawMessage(message json.RawMessage) json.RawMessage {
	if len(message) == 0 {
		return nil
	}

	return append(json.RawMessage(nil), message...)
}
