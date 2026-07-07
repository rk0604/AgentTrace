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
		fmt.Printf("Root cause: none\n")
		return
	}

	fmt.Printf("Root cause step: %s\n", result.RootCause.StepID)
	fmt.Printf("Root cause agent: %s\n", result.RootCause.AgentName)
	fmt.Printf("Root cause reason: %s\n", result.RootCause.Reason)
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
