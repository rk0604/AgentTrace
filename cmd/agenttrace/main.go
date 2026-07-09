package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/toypipeline"
)

type stepView struct {
	Step    attrib.Step
	Checked bool
	Failed  bool
	Reason  string
}

func main() {
	healthy := flag.Bool("healthy", false, "run the toy pipeline without the injected reference failure")
	flag.Parse()

	// Build the deterministic toy trace.
	trace := toypipeline.Run(!*healthy)
	checkers := toypipeline.Checkers()

	// Compute dependency order from DependsOn.
	orderedSteps, err := attrib.TopologicalSort(trace)
	if err != nil {
		exitWithError(err)
	}

	// Run first divergence attribution.
	result, err := attrib.FindRootCause(trace, checkers)
	if err != nil {
		exitWithError(err)
	}

	stepViews := buildStepViews(orderedSteps, result, checkers)
	printReport(trace, stepViews, result)
}

// buildStepViews creates display rows for the CLI graph report.
//
// Input
// orderedSteps []attrib.Step
// Steps sorted so each dependency appears before the step that consumes it.
//
// result attrib.AttributionResult
// The attribution result returned by attrib.FindRootCause.
//
// checkers map[string]attrib.StepChecker
// Step checker functions keyed by step ID.
//
// Output
// []stepView
// Display rows with check status and failure reason.
func buildStepViews(orderedSteps []attrib.Step, result attrib.AttributionResult, checkers map[string]attrib.StepChecker) []stepView {
	checkedIDs := checkedStepSet(result.CheckedStepIDs)
	views := make([]stepView, 0, len(orderedSteps))

	for _, step := range orderedSteps {
		view := stepView{
			Step:    step,
			Checked: checkedIDs[step.StepID],
		}

		if view.Checked {
			// Reuse the same checker functions so the report matches attribution.
			check, err := checkers[step.StepID](step)
			if err != nil {
				view.Failed = true
				view.Reason = err.Error()
			} else if !check.Passed {
				view.Failed = true
				view.Reason = check.Reason
			}
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
	fmt.Printf("AgentTrace toy run\n")
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
	if result.RootCause == nil {
		fmt.Printf("Root cause: none\n\n")
		printFlowChart(views, result)
		return
	}

	fmt.Printf("Root cause step: %s\n", result.RootCause.StepID)
	fmt.Printf("Root cause agent: %s\n", result.RootCause.AgentName)
	fmt.Printf("Root cause reason: %s\n\n", result.RootCause.Reason)

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
		return "not checked because attribution stopped earlier"
	}
	if view.Failed {
		return "failed"
	}

	return "passed"
}

// printFlowChart writes an edge based flow chart.
//
// Input
// views []stepView
// Display rows for dependency ordered steps.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// None
func printFlowChart(views []stepView, result attrib.AttributionResult) {
	viewsByID := stepViewByID(views)

	fmt.Printf("Flow chart\n")
	for _, target := range views {
		if len(target.Step.DependsOn) == 0 {
			continue
		}

		for _, sourceID := range target.Step.DependsOn {
			source, exists := viewsByID[sourceID]
			if !exists {
				continue
			}

			fmt.Printf("%s%s%s\n", nodeText(source, result), edgeText(source.Step.StepID, result), nodeText(target, result))
		}
	}

	if result.RootCause != nil {
		fmt.Printf("\nMarked node: %s\n", result.RootCause.StepID)
		fmt.Printf("Marked edge: bad output leaving %s\n", result.RootCause.StepID)
	}
}

// stepViewByID creates a lookup table for CLI step views.
//
// Input
// views []stepView
// Display rows for dependency ordered steps.
//
// Output
// map[string]stepView
// Lookup table keyed by step ID.
func stepViewByID(views []stepView) map[string]stepView {
	viewsByID := make(map[string]stepView, len(views))

	for _, view := range views {
		viewsByID[view.Step.StepID] = view
	}

	return viewsByID
}

// nodeText formats one node for the CLI flow chart.
//
// Input
// view stepView
// Display row for one trace step.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// string
// Human readable node text.
func nodeText(view stepView, result attrib.AttributionResult) string {
	label := fmt.Sprintf("[%s %s", view.Step.AgentName, chartStatusText(view))
	if result.RootCause != nil && view.Step.StepID == result.RootCause.StepID {
		label += " ROOT CAUSE"
	}

	return label + "]"
}

// edgeText formats one dependency edge for the CLI flow chart.
//
// Input
// sourceID string
// Step ID for the upstream edge source.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// string
// Human readable edge text.
func edgeText(sourceID string, result attrib.AttributionResult) string {
	if result.RootCause != nil && sourceID == result.RootCause.StepID {
		return " == CAUSE EDGE ==> "
	}

	return " -> "
}

// chartStatusText formats one compact node status for the CLI flow chart.
//
// Input
// view stepView
// Display row for one trace step.
//
// Output
// string
// Compact node status text.
func chartStatusText(view stepView) string {
	if !view.Checked {
		return "SKIPPED"
	}
	if view.Failed {
		return "FAIL"
	}

	return "PASS"
}

// exitWithError prints an error and exits the program.
//
// Input
// err error
// Error that stopped the command.
//
// Output
// None
func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
