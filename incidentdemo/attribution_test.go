package incidentdemo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/checkerconfig"
	"github.com/rk0604/AgentTrace/incidentdemo"
)

func TestIncidentCheckerConfigurationAttributesFailureModes(t *testing.T) {
	checkers := loadIncidentCheckers(t)
	tests := []struct {
		name          string
		failureMode   incidentdemo.FailureMode
		wantStatus    string
		wantRootCause string
		wantChecked   int
	}{
		{name: "healthy", failureMode: incidentdemo.FailureNone, wantStatus: "passed", wantChecked: 13},
		{name: "metrics failure", failureMode: incidentdemo.FailureMetrics, wantStatus: "failed", wantRootCause: incidentdemo.MetricsAnalyzerStepID, wantChecked: 4},
		{name: "deployment failure", failureMode: incidentdemo.FailureDeployment, wantStatus: "failed", wantRootCause: incidentdemo.DeploymentAnalyzerStepID, wantChecked: 5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trace, err := incidentdemo.Run(test.failureMode)
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}

			result, err := attrib.FindRootCause(trace, checkers)
			if err != nil {
				t.Fatalf("FindRootCause returned error: %v", err)
			}
			if result.Status != test.wantStatus {
				t.Fatalf("expected status %q, got %q", test.wantStatus, result.Status)
			}
			if len(result.CheckedStepIDs) != test.wantChecked {
				t.Fatalf("expected %d checked steps, got %d", test.wantChecked, len(result.CheckedStepIDs))
			}

			if test.wantRootCause == "" {
				if result.RootCause != nil {
					t.Fatalf("expected no root cause, got %+v", result.RootCause)
				}
				return
			}
			if result.RootCause == nil || result.RootCause.StepID != test.wantRootCause {
				t.Fatalf("expected root cause %q, got %+v", test.wantRootCause, result.RootCause)
			}
		})
	}
}

func TestIncidentDownstreamStepsRemainCorrectGivenFailedInputs(t *testing.T) {
	checkers := loadIncidentCheckers(t)
	tests := []struct {
		name        string
		failureMode incidentdemo.FailureMode
		failedStep  string
	}{
		{name: "metrics failure", failureMode: incidentdemo.FailureMetrics, failedStep: incidentdemo.MetricsAnalyzerStepID},
		{name: "deployment failure", failureMode: incidentdemo.FailureDeployment, failedStep: incidentdemo.DeploymentAnalyzerStepID},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trace, err := incidentdemo.Run(test.failureMode)
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}

			for _, step := range trace.Steps {
				check, err := checkers[step.StepID](step)
				if err != nil {
					t.Fatalf("checker for step %q returned error: %v", step.StepID, err)
				}
				if step.StepID == test.failedStep {
					if check.Passed {
						t.Fatalf("expected injected step %q to fail", step.StepID)
					}
					continue
				}
				if !check.Passed {
					t.Fatalf("expected step %q to remain correct given actual inputs: %s", step.StepID, check.Reason)
				}
			}
		})
	}
}

func TestIncidentJSONFixturesProduceExpectedAttribution(t *testing.T) {
	checkers := loadIncidentCheckers(t)
	tests := []struct {
		name          string
		fixture       string
		wantStatus    string
		wantRootCause string
	}{
		{name: "healthy", fixture: "incident-healthy.json", wantStatus: "passed"},
		{name: "metrics failure", fixture: "incident-metrics-failure.json", wantStatus: "failed", wantRootCause: incidentdemo.MetricsAnalyzerStepID},
		{name: "deployment failure", fixture: "incident-deployment-failure.json", wantStatus: "failed", wantRootCause: incidentdemo.DeploymentAnalyzerStepID},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trace := loadIncidentTrace(t, test.fixture)
			result, err := attrib.FindRootCause(trace, checkers)
			if err != nil {
				t.Fatalf("FindRootCause returned error: %v", err)
			}
			if result.Status != test.wantStatus {
				t.Fatalf("expected status %q, got %q", test.wantStatus, result.Status)
			}
			if test.wantRootCause == "" {
				if result.RootCause != nil {
					t.Fatalf("expected no root cause, got %+v", result.RootCause)
				}
				return
			}
			if result.RootCause == nil || result.RootCause.StepID != test.wantRootCause {
				t.Fatalf("expected root cause %q, got %+v", test.wantRootCause, result.RootCause)
			}
		})
	}
}

func loadIncidentCheckers(t *testing.T) map[string]attrib.StepChecker {
	t.Helper()

	file, err := os.Open(filepath.Join("..", "examples", "incident-checkers.json"))
	if err != nil {
		t.Fatalf("open incident checker configuration: %v", err)
	}
	defer file.Close()

	config, err := checkerconfig.Decode(file)
	if err != nil {
		t.Fatalf("decode incident checker configuration: %v", err)
	}

	checkers, err := checkerconfig.Build(config)
	if err != nil {
		t.Fatalf("build incident checkers: %v", err)
	}

	return checkers
}

func loadIncidentTrace(t *testing.T, name string) attrib.Trace {
	t.Helper()

	file, err := os.Open(filepath.Join("..", "examples", name))
	if err != nil {
		t.Fatalf("open incident trace %q: %v", name, err)
	}
	defer file.Close()

	trace, err := attrib.DecodeTrace(file)
	if err != nil {
		t.Fatalf("decode incident trace %q: %v", name, err)
	}

	return trace
}
