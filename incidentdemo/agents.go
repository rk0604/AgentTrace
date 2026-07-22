package incidentdemo

import "fmt"

// intakeIncident validates and normalizes submitted incident evidence.
//
// Input
// input Scenario
// Synthetic source evidence for one incident.
//
// Output
// IncidentPacket
// Normalized incident identity and symptom.
func intakeIncident(input Scenario) IncidentPacket {
	return IncidentPacket{
		IncidentID: input.IncidentID,
		Service:    input.Service,
		StartedAt:  input.StartedAt,
		Symptom:    input.Symptom,
		Accepted:   input.IncidentID != "" && input.Service != "",
	}
}

// planInvestigation creates the parallel investigation tasks.
//
// Input
// input IncidentPacket
// Normalized incident identity and symptom.
//
// Output
// InvestigationPlan
// Task list assigned to the analyzer branches.
func planInvestigation(input IncidentPacket) InvestigationPlan {
	return InvestigationPlan{
		IncidentID: input.IncidentID,
		Service:    input.Service,
		Tasks: []string{
			"analyze_logs",
			"analyze_metrics",
			"analyze_deployments",
			"load_runbook",
		},
	}
}

// analyzeLogs extracts error evidence from application logs.
//
// Input
// input LogAnalyzerInput
// Incident service and log records.
//
// Output
// LogFinding
// Error code, count, first occurrence, and summary.
func analyzeLogs(input LogAnalyzerInput) LogFinding {
	finding := LogFinding{Service: input.Service}

	for _, record := range input.Logs {
		if record.Service != input.Service || record.Level != "ERROR" {
			continue
		}
		if finding.ErrorCount == 0 {
			finding.ErrorCode = record.ErrorCode
			finding.FirstErrorAt = record.Timestamp
		}
		finding.ErrorCount++
	}

	finding.Summary = fmt.Sprintf("found %d %s errors", finding.ErrorCount, finding.ErrorCode)
	return finding
}

// analyzeMetrics classifies service metrics against their threshold.
//
// Input
// input MetricsAnalyzerInput
// Incident service and metric snapshot.
//
// failureMode FailureMode
// Optional semantic failure applied to the classification.
//
// Output
// MetricFinding
// Preserved metrics with interpreted state and summary.
func analyzeMetrics(input MetricsAnalyzerInput, failureMode FailureMode) MetricFinding {
	state := "normal"
	if input.Metrics.P95LatencyMS > input.Metrics.ThresholdMS {
		state = "critical"
	}
	if failureMode == FailureMetrics {
		state = "normal"
	}

	summary := "latency is within the configured threshold"
	if state == "critical" {
		summary = "latency exceeds the configured threshold"
	}

	return MetricFinding{
		ObservedAt:   input.Metrics.ObservedAt,
		P95LatencyMS: input.Metrics.P95LatencyMS,
		ThresholdMS:  input.Metrics.ThresholdMS,
		ErrorRate:    input.Metrics.ErrorRate,
		State:        state,
		Summary:      summary,
	}
}

// analyzeDeployments selects the deployment correlated with the incident.
//
// Input
// input DeploymentAnalyzerInput
// Incident start time and deployment history.
//
// failureMode FailureMode
// Optional semantic failure applied to deployment selection.
//
// Output
// DeploymentFinding
// Selected deployment, configuration change, and correlation decision.
func analyzeDeployments(input DeploymentAnalyzerInput, failureMode FailureMode) DeploymentFinding {
	selected := input.Deployments[len(input.Deployments)-1]
	if failureMode == FailureDeployment {
		selected = input.Deployments[0]
	}

	correlated := selected.DeployedAt < input.StartedAt && selected.TimeoutAfterSecond < selected.TimeoutBeforeSecond
	changedField := "none"
	if selected.TimeoutAfterSecond != selected.TimeoutBeforeSecond {
		changedField = "request_timeout_seconds"
	}

	summary := "deployment has no correlated timeout reduction"
	if correlated {
		summary = "deployment reduced the request timeout shortly before the incident"
	}

	return DeploymentFinding{
		DeploymentID: selected.DeploymentID,
		Commit:       selected.Commit,
		DeployedAt:   selected.DeployedAt,
		ChangedField: changedField,
		BeforeValue:  selected.TimeoutBeforeSecond,
		AfterValue:   selected.TimeoutAfterSecond,
		Correlated:   correlated,
		Summary:      summary,
	}
}

// loadRunbook normalizes the supplied operational runbook.
//
// Input
// input RunbookLoaderInput
// Incident identity and runbook record.
//
// Output
// LoadedRunbook
// Normalized runbook fields and loaded state.
func loadRunbook(input RunbookLoaderInput) LoadedRunbook {
	return LoadedRunbook{
		RunbookID:         input.Runbook.RunbookID,
		Signal:            input.Runbook.Signal,
		Cause:             input.Runbook.Cause,
		RecommendedAction: input.Runbook.RecommendedAction,
		Loaded:            input.Runbook.RunbookID != "",
	}
}

// mergeEvidence combines the analyzer outputs without reinterpreting them.
//
// Input
// input EvidenceInput
// Log, metric, and deployment findings.
//
// Output
// EvidenceBundle
// Normalized evidence copied from the actual branch outputs.
func mergeEvidence(input EvidenceInput) EvidenceBundle {
	return EvidenceBundle{
		Service:        input.Logs.Service,
		ErrorCode:      input.Logs.ErrorCode,
		ErrorCount:     input.Logs.ErrorCount,
		FirstErrorAt:   input.Logs.FirstErrorAt,
		MetricState:    input.Metrics.State,
		P95LatencyMS:   input.Metrics.P95LatencyMS,
		ThresholdMS:    input.Metrics.ThresholdMS,
		MetricObserved: input.Metrics.ObservedAt,
		DeploymentID:   input.Deployment.DeploymentID,
		DeploymentAt:   input.Deployment.DeployedAt,
		DeploymentLink: input.Deployment.Correlated,
	}
}

// buildTimeline orders the deployment, error, and metric observations.
//
// Input
// input EvidenceBundle
// Merged analyzer evidence.
//
// Output
// Timeline
// Incident event timestamps and ordering result.
func buildTimeline(input EvidenceBundle) Timeline {
	return Timeline{
		DeploymentID:   input.DeploymentID,
		DeploymentAt:   input.DeploymentAt,
		FirstErrorAt:   input.FirstErrorAt,
		MetricObserved: input.MetricObserved,
		Ordered:        input.DeploymentAt < input.FirstErrorAt && input.FirstErrorAt < input.MetricObserved,
	}
}

// generateHypothesis derives a diagnosis from actual merged evidence.
//
// Input
// input HypothesisInput
// Merged evidence and incident timeline.
//
// Output
// Hypothesis
// Proposed cause, confidence, and supporting evidence labels.
func generateHypothesis(input HypothesisInput) Hypothesis {
	if input.Evidence.MetricState == "critical" && input.Evidence.DeploymentLink && input.Evidence.ErrorCode == "REQUEST_TIMEOUT" {
		return Hypothesis{
			Cause:      "timeout_regression",
			Confidence: "high",
			Evidence:   []string{"latency_threshold_breach", "recent_timeout_change", "request_timeout_errors"},
		}
	}

	if input.Evidence.MetricState != "critical" {
		return Hypothesis{
			Cause:      "application_error",
			Confidence: "medium",
			Evidence:   []string{"request_timeout_errors", "metrics_reported_normal"},
		}
	}

	return Hypothesis{
		Cause:      "traffic_spike",
		Confidence: "medium",
		Evidence:   []string{"latency_threshold_breach", "no_correlated_deployment"},
	}
}

// assessImpact assigns severity from the diagnosis and error evidence.
//
// Input
// input ImpactInput
// Merged evidence and generated hypothesis.
//
// Output
// ImpactAssessment
// Severity, affected service, and customer impact.
func assessImpact(input ImpactInput) ImpactAssessment {
	severity := "SEV2"
	impact := "degraded checkout performance"
	if input.Hypothesis.Confidence == "high" && input.Evidence.ErrorCount >= 2 {
		severity = "SEV1"
		impact = "checkout requests are failing for customers"
	}

	return ImpactAssessment{
		Severity:        severity,
		AffectedService: input.Evidence.Service,
		CustomerImpact:  impact,
	}
}

// matchRunbook selects an action for the generated hypothesis.
//
// Input
// input RunbookMatchInput
// Generated hypothesis and loaded runbook.
//
// Output
// RunbookMatch
// Applicability decision and recommended action.
func matchRunbook(input RunbookMatchInput) RunbookMatch {
	applicable := input.Runbook.Loaded && input.Runbook.Cause == input.Hypothesis.Cause
	action := "collect additional evidence"
	if applicable {
		action = input.Runbook.RecommendedAction
	}

	return RunbookMatch{
		RunbookID:  input.Runbook.RunbookID,
		Applicable: applicable,
		Action:     action,
	}
}

// planRemediation combines diagnosis, impact, and runbook guidance.
//
// Input
// input RemediationInput
// Hypothesis, impact assessment, and runbook match.
//
// Output
// RemediationPlan
// Prioritized action and owning team.
func planRemediation(input RemediationInput) RemediationPlan {
	return RemediationPlan{
		Cause:    input.Hypothesis.Cause,
		Priority: input.Impact.Severity,
		Action:   input.Runbook.Action,
		Owner:    "checkout-on-call",
	}
}

// summarizeIncident creates the final incident conclusion.
//
// Input
// input SummaryInput
// Generated hypothesis and remediation plan.
//
// Output
// IncidentSummary
// Title, root cause, severity, and resolution.
func summarizeIncident(input SummaryInput) IncidentSummary {
	return IncidentSummary{
		Title:      "Checkout API timeout incident",
		RootCause:  input.Hypothesis.Cause,
		Severity:   input.Remediation.Priority,
		Resolution: input.Remediation.Action,
	}
}
