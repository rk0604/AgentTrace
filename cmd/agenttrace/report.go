package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rk0604/AgentTrace/attrib"
)

// buildStepViews creates display rows for the CLI graph report.
//
// Input
// orderedSteps []attrib.Step
// Steps sorted so each dependency appears before the step that consumes it.
//
// result attrib.AttributionResult
// The attribution result returned by attrib.FindRootCause.
//
// Output
// []stepView
// Display rows with check status and failure reason.
func buildStepViews(orderedSteps []attrib.Step, result attrib.AttributionResult) []stepView {
	checkedIDs := checkedStepSet(result.CheckedStepIDs)
	downstreamStepIDs := result.DownstreamCandidateStepIDs
	if len(downstreamStepIDs) == 0 {
		downstreamStepIDs = result.AffectedStepIDs
	}
	affectedIDs := checkedStepSet(downstreamStepIDs)
	rootIDs := make(map[string]bool, len(result.RootCauses))
	for _, rootCause := range result.RootCauses {
		rootIDs[rootCause.StepID] = true
	}
	if result.RootCause != nil {
		rootIDs[result.RootCause.StepID] = true
	}
	divergencesByID := make(map[string]attrib.Divergence, len(result.Divergences))
	for _, divergence := range result.Divergences {
		divergencesByID[divergence.StepID] = divergence
	}
	views := make([]stepView, 0, len(orderedSteps))

	for _, step := range orderedSteps {
		view := stepView{
			Step:     step,
			Checked:  checkedIDs[step.StepID],
			Root:     rootIDs[step.StepID],
			Affected: affectedIDs[step.StepID],
		}

		if divergence, exists := divergencesByID[step.StepID]; exists {
			view.Failed = true
			view.Reason = divergence.Reason
		} else if result.RootCause != nil &&
			result.RootCause.StepID == step.StepID {
			view.Failed = true
			view.Reason = result.RootCause.Reason
		}

		views = append(views, view)
	}

	return views
}

// checkedStepSet creates a lookup table for checked step IDs.
//
// Input
// stepIDs []string
// Step IDs that were checked before attribution stopped.
//
// Output
// map[string]bool
// Lookup table where true means the step was checked.
func checkedStepSet(stepIDs []string) map[string]bool {
	checkedIDs := make(map[string]bool, len(stepIDs))

	for _, stepID := range stepIDs {
		checkedIDs[stepID] = true
	}

	return checkedIDs
}

// printReport writes the CLI graph report.
//
// Input
// trace attrib.Trace
// The trace being displayed.
//
// views []stepView
// Display rows for dependency ordered steps.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// None
func printReport(trace attrib.Trace, views []stepView, result attrib.AttributionResult) {
	fmt.Printf("AgentTrace run\n")
	fmt.Printf("Run ID: %s\n\n", trace.RunID)

	fmt.Printf("Dependency ordered steps\n")
	for index, view := range views {
		fmt.Printf("%d. %s\n", index+1, view.Step.AgentName)
		fmt.Printf("   Step ID: %s\n", view.Step.StepID)
		fmt.Printf("   Depends on: %s\n", dependsOnText(view.Step.DependsOn))
		fmt.Printf("   Check status: %s\n", checkStatusText(view))
		if view.Failed && view.Reason != "" {
			fmt.Printf("   Failure reason: %s\n", view.Reason)
		}
		fmt.Printf("\n")
	}

	fmt.Printf("Attribution result\n")
	fmt.Printf("Status: %s\n", result.Status)
	if len(result.TargetStepIDs) > 0 {
		fmt.Printf("Targets: %s\n", strings.Join(result.TargetStepIDs, ", "))
	}
	if result.RootCause == nil {
		fmt.Printf("Root cause: none\n\n")
		printFlowChart(views, result)
		return
	}

	fmt.Printf("Root cause step: %s\n", result.RootCause.StepID)
	fmt.Printf("Root cause agent: %s\n", result.RootCause.AgentName)
	fmt.Printf("Root cause reason: %s\n", result.RootCause.Reason)
	if result.RootCause.Expression != "" {
		fmt.Printf("Failed expression: %s\n", result.RootCause.Expression)
	}
	if len(result.RootCause.Input) > 0 {
		fmt.Printf("Actual input: %s\n", compactJSON(result.RootCause.Input))
	}
	if len(result.RootCause.Output) > 0 {
		fmt.Printf("Actual output: %s\n", compactJSON(result.RootCause.Output))
	}
	if len(result.RootCause.Expected) > 0 {
		fmt.Printf("Expected context: %s\n", compactJSON(result.RootCause.Expected))
	}
	if len(result.RootCauses) > 1 {
		rootCauseIDs := make([]string, 0, len(result.RootCauses))
		for _, rootCause := range result.RootCauses {
			rootCauseIDs = append(rootCauseIDs, rootCause.StepID)
		}
		fmt.Printf("All root causes: %s\n", strings.Join(rootCauseIDs, ", "))
	}
	if len(result.SecondaryDivergences) > 0 {
		secondaryIDs := make([]string, 0, len(result.SecondaryDivergences))
		for _, divergence := range result.SecondaryDivergences {
			secondaryIDs = append(secondaryIDs, divergence.StepID)
		}
		fmt.Printf(
			"Secondary divergences: %s\n",
			strings.Join(secondaryIDs, ", "),
		)
	}
	downstreamStepIDs := result.DownstreamCandidateStepIDs
	if len(downstreamStepIDs) == 0 {
		downstreamStepIDs = result.AffectedStepIDs
	}
	fmt.Printf(
		"Downstream candidates: %s\n",
		affectedStepsText(downstreamStepIDs),
	)
	fmt.Printf("\n")

	printFlowChart(views, result)
}

// dependsOnText formats dependency IDs for display.
//
// Input
// dependsOn []string
// Step IDs consumed by the current step.
//
// Output
// string
// Human readable dependency text.
func dependsOnText(dependsOn []string) string {
	if len(dependsOn) == 0 {
		return "none"
	}

	return strings.Join(dependsOn, ", ")
}

// checkStatusText formats one step check state for display.
//
// Input
// view stepView
// Display row for one trace step.
//
// Output
// string
// Human readable check status.
func checkStatusText(view stepView) string {
	if !view.Checked {
		if view.Affected {
			return "affected by upstream failure"
		}
		return "not checked because attribution stopped earlier"
	}
	if view.Failed {
		if view.Root {
			return "failed root cause"
		}
		return "failed"
	}
	if view.Affected {
		return "passed given actual input; downstream candidate"
	}

	return "passed"
}

// compactJSON formats raw JSON on one line.
//
// Input
// data json.RawMessage
// JSON value to display.
//
// Output
// string
// Compact JSON or the original text when compaction fails.
func compactJSON(data json.RawMessage) string {
	var output bytes.Buffer
	if err := json.Compact(&output, data); err != nil {
		return fitEvidenceText(string(data))
	}

	return fitEvidenceText(output.String())
}

// fitEvidenceText bounds evidence printed in a human report.
//
// Input
// text string
// Compact evidence text.
//
// Output
// string
// Original text or a bounded prefix with a truncation marker.
func fitEvidenceText(text string) string {
	characters := []rune(text)
	if len(characters) <= maxEvidenceTextSize {
		return text
	}

	const marker = "... truncated"
	return string(
		characters[:maxEvidenceTextSize-len([]rune(marker))],
	) + marker
}

// affectedStepsText formats downstream impact for the report.
//
// Input
// stepIDs slice of string
// Affected step IDs in dependency order.
//
// Output
// string
// Comma separated IDs or none.
func affectedStepsText(stepIDs []string) string {
	if len(stepIDs) == 0 {
		return "none"
	}

	return strings.Join(stepIDs, ", ")
}
