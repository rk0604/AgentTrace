package incidentdemo

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rk0604/AgentTrace/attrib"
)

// Run builds one deterministic incident investigation trace.
//
// Input
// failureMode FailureMode
// Semantic failure to inject into one analyzer, or FailureNone.
//
// Output
// attrib.Trace
// Complete incident investigation trace with generic JSON payloads.
//
// error
// Non nil when the requested failure mode is unsupported.
func Run(failureMode FailureMode) (attrib.Trace, error) {
	if err := validateFailureMode(failureMode); err != nil {
		return attrib.Trace{}, err
	}

	scenario := SyntheticScenario()
	baseTime := time.Date(2026, 7, 20, 10, 5, 0, 0, time.UTC)
	runID := runIDForMode(failureMode)

	incidentPacket := intakeIncident(scenario)
	plan := planInvestigation(incidentPacket)
	logInput := LogAnalyzerInput{IncidentID: scenario.IncidentID, Service: scenario.Service, Logs: scenario.Logs}
	metricsInput := MetricsAnalyzerInput{IncidentID: scenario.IncidentID, Service: scenario.Service, Metrics: scenario.Metrics}
	deploymentInput := DeploymentAnalyzerInput{IncidentID: scenario.IncidentID, Service: scenario.Service, StartedAt: scenario.StartedAt, Deployments: scenario.Deployments}
	runbookInput := RunbookLoaderInput{IncidentID: scenario.IncidentID, Runbook: scenario.Runbook}
	logFinding := analyzeLogs(logInput)
	metricFinding := analyzeMetrics(metricsInput, failureMode)
	deploymentFinding := analyzeDeployments(deploymentInput, failureMode)
	loadedRunbook := loadRunbook(runbookInput)
	evidenceInput := EvidenceInput{Logs: logFinding, Metrics: metricFinding, Deployment: deploymentFinding}
	evidence := mergeEvidence(evidenceInput)
	timeline := buildTimeline(evidence)
	hypothesisInput := HypothesisInput{Evidence: evidence, Timeline: timeline}
	hypothesis := generateHypothesis(hypothesisInput)
	impactInput := ImpactInput{Evidence: evidence, Hypothesis: hypothesis}
	impact := assessImpact(impactInput)
	runbookMatchInput := RunbookMatchInput{Hypothesis: hypothesis, Runbook: loadedRunbook}
	runbookMatch := matchRunbook(runbookMatchInput)
	remediationInput := RemediationInput{Hypothesis: hypothesis, Impact: impact, Runbook: runbookMatch}
	remediation := planRemediation(remediationInput)
	summaryInput := SummaryInput{Hypothesis: hypothesis, Remediation: remediation}
	summary := summarizeIncident(summaryInput)

	steps := []attrib.Step{
		newStep(runID, IncidentIntakeStepID, "Incident Intake", nil, scenario, incidentPacket, baseTime),
		newStep(runID, InvestigationPlannerStepID, "Investigation Planner", []string{IncidentIntakeStepID}, incidentPacket, plan, baseTime.Add(time.Second)),
		newStep(runID, LogAnalyzerStepID, "Log Analyzer", []string{IncidentIntakeStepID, InvestigationPlannerStepID}, logInput, logFinding, baseTime.Add(2*time.Second)),
		newStep(runID, MetricsAnalyzerStepID, "Metrics Analyzer", []string{IncidentIntakeStepID, InvestigationPlannerStepID}, metricsInput, metricFinding, baseTime.Add(2*time.Second)),
		newStep(runID, DeploymentAnalyzerStepID, "Deployment Analyzer", []string{IncidentIntakeStepID, InvestigationPlannerStepID}, deploymentInput, deploymentFinding, baseTime.Add(2*time.Second)),
		newStep(runID, RunbookLoaderStepID, "Runbook Loader", []string{IncidentIntakeStepID, InvestigationPlannerStepID}, runbookInput, loadedRunbook, baseTime.Add(2*time.Second)),
		newStep(runID, EvidenceMergerStepID, "Evidence Merger", []string{LogAnalyzerStepID, MetricsAnalyzerStepID, DeploymentAnalyzerStepID}, evidenceInput, evidence, baseTime.Add(3*time.Second)),
		newStep(runID, TimelineBuilderStepID, "Timeline Builder", []string{EvidenceMergerStepID}, evidence, timeline, baseTime.Add(4*time.Second)),
		newStep(runID, HypothesisGeneratorStepID, "Hypothesis Generator", []string{EvidenceMergerStepID, TimelineBuilderStepID}, hypothesisInput, hypothesis, baseTime.Add(5*time.Second)),
		newStep(runID, ImpactAssessorStepID, "Impact Assessor", []string{EvidenceMergerStepID, HypothesisGeneratorStepID}, impactInput, impact, baseTime.Add(6*time.Second)),
		newStep(runID, RunbookMatcherStepID, "Runbook Matcher", []string{RunbookLoaderStepID, HypothesisGeneratorStepID}, runbookMatchInput, runbookMatch, baseTime.Add(6*time.Second)),
		newStep(runID, RemediationPlannerStepID, "Remediation Planner", []string{HypothesisGeneratorStepID, ImpactAssessorStepID, RunbookMatcherStepID}, remediationInput, remediation, baseTime.Add(7*time.Second)),
		newStep(runID, IncidentSummaryStepID, "Incident Summary", []string{HypothesisGeneratorStepID, RemediationPlannerStepID}, summaryInput, summary, baseTime.Add(8*time.Second)),
	}

	return attrib.Trace{RunID: runID, Steps: steps}, nil
}

func validateFailureMode(failureMode FailureMode) error {
	switch failureMode {
	case FailureNone, FailureMetrics, FailureDeployment:
		return nil
	default:
		return fmt.Errorf("unsupported incident failure mode %q", failureMode)
	}
}

func runIDForMode(failureMode FailureMode) string {
	return "incident-investigation-" + string(failureMode)
}

func newStep(runID string, stepID string, agentName string, dependsOn []string, input any, output any, timestamp time.Time) attrib.Step {
	return attrib.Step{
		RunID:     runID,
		StepID:    stepID,
		AgentName: agentName,
		DependsOn: dependsOn,
		Input:     mustRaw(input),
		Output:    mustRaw(output),
		ModelUsed: "deterministic-stub",
		Timestamp: timestamp,
		Status:    "ok",
	}
}

func mustRaw(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}

	return raw
}
