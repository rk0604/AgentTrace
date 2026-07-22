package incidentdemo_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/incidentdemo"
)

func TestRunBuildsExpectedIncidentDAG(t *testing.T) {
	trace, err := incidentdemo.Run(incidentdemo.FailureNone)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(trace.Steps) != 13 {
		t.Fatalf("expected 13 steps, got %d", len(trace.Steps))
	}

	ordered, err := attrib.TopologicalSort(trace)
	if err != nil {
		t.Fatalf("TopologicalSort returned error: %v", err)
	}

	want := []string{
		incidentdemo.IncidentIntakeStepID,
		incidentdemo.InvestigationPlannerStepID,
		incidentdemo.LogAnalyzerStepID,
		incidentdemo.MetricsAnalyzerStepID,
		incidentdemo.DeploymentAnalyzerStepID,
		incidentdemo.RunbookLoaderStepID,
		incidentdemo.EvidenceMergerStepID,
		incidentdemo.TimelineBuilderStepID,
		incidentdemo.HypothesisGeneratorStepID,
		incidentdemo.ImpactAssessorStepID,
		incidentdemo.RunbookMatcherStepID,
		incidentdemo.RemediationPlannerStepID,
		incidentdemo.IncidentSummaryStepID,
	}
	assertStepIDs(t, ordered, want)

	steps := stepsByID(trace)
	assertDependencies(t, steps[incidentdemo.EvidenceMergerStepID].DependsOn, []string{
		incidentdemo.LogAnalyzerStepID,
		incidentdemo.MetricsAnalyzerStepID,
		incidentdemo.DeploymentAnalyzerStepID,
	})
	assertDependencies(t, steps[incidentdemo.RemediationPlannerStepID].DependsOn, []string{
		incidentdemo.HypothesisGeneratorStepID,
		incidentdemo.ImpactAssessorStepID,
		incidentdemo.RunbookMatcherStepID,
	})
}

func TestHealthyIncidentProducesExpectedConclusion(t *testing.T) {
	trace, err := incidentdemo.Run(incidentdemo.FailureNone)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	steps := stepsByID(trace)
	metrics := decodeOutput[incidentdemo.MetricFinding](t, steps[incidentdemo.MetricsAnalyzerStepID])
	if metrics.State != "critical" {
		t.Fatalf("expected critical metrics, got %q", metrics.State)
	}

	deployment := decodeOutput[incidentdemo.DeploymentFinding](t, steps[incidentdemo.DeploymentAnalyzerStepID])
	if deployment.DeploymentID != "deploy-42" || !deployment.Correlated {
		t.Fatalf("unexpected deployment finding %+v", deployment)
	}

	summary := decodeOutput[incidentdemo.IncidentSummary](t, steps[incidentdemo.IncidentSummaryStepID])
	if summary.RootCause != "timeout_regression" {
		t.Fatalf("expected timeout_regression, got %q", summary.RootCause)
	}
	if summary.Severity != "SEV1" {
		t.Fatalf("expected SEV1, got %q", summary.Severity)
	}
}

func TestMetricsFailureChangesDownstreamConclusion(t *testing.T) {
	trace, err := incidentdemo.Run(incidentdemo.FailureMetrics)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	steps := stepsByID(trace)
	metrics := decodeOutput[incidentdemo.MetricFinding](t, steps[incidentdemo.MetricsAnalyzerStepID])
	if metrics.State != "normal" {
		t.Fatalf("expected injected normal state, got %q", metrics.State)
	}

	summary := decodeOutput[incidentdemo.IncidentSummary](t, steps[incidentdemo.IncidentSummaryStepID])
	if summary.RootCause != "application_error" {
		t.Fatalf("expected propagated application_error, got %q", summary.RootCause)
	}
}

func TestDeploymentFailureChangesDownstreamConclusion(t *testing.T) {
	trace, err := incidentdemo.Run(incidentdemo.FailureDeployment)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	steps := stepsByID(trace)
	deployment := decodeOutput[incidentdemo.DeploymentFinding](t, steps[incidentdemo.DeploymentAnalyzerStepID])
	if deployment.DeploymentID != "deploy-41" || deployment.Correlated {
		t.Fatalf("unexpected injected deployment finding %+v", deployment)
	}

	summary := decodeOutput[incidentdemo.IncidentSummary](t, steps[incidentdemo.IncidentSummaryStepID])
	if summary.RootCause != "traffic_spike" {
		t.Fatalf("expected propagated traffic_spike, got %q", summary.RootCause)
	}
}

func TestRunRejectsUnknownFailureMode(t *testing.T) {
	_, err := incidentdemo.Run(incidentdemo.FailureMode("unknown"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "unsupported incident failure mode") {
		t.Fatalf("unexpected error %q", err)
	}
}

func stepsByID(trace attrib.Trace) map[string]attrib.Step {
	steps := make(map[string]attrib.Step, len(trace.Steps))
	for _, step := range trace.Steps {
		steps[step.StepID] = step
	}

	return steps
}

func decodeOutput[T any](t *testing.T, step attrib.Step) T {
	t.Helper()

	var output T
	if err := json.Unmarshal(step.Output, &output); err != nil {
		t.Fatalf("decode output for step %q: %v", step.StepID, err)
	}

	return output
}

func assertStepIDs(t *testing.T, steps []attrib.Step, want []string) {
	t.Helper()

	if len(steps) != len(want) {
		t.Fatalf("expected %d steps, got %d", len(want), len(steps))
	}
	for index, step := range steps {
		if step.StepID != want[index] {
			t.Fatalf("expected step %d to be %q, got %q", index, want[index], step.StepID)
		}
	}
}

func assertDependencies(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected dependencies %v, got %v", want, got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("expected dependencies %v, got %v", want, got)
		}
	}
}
