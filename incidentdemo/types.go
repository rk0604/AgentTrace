package incidentdemo

// FailureMode identifies the semantic failure injected into an incident trace.
type FailureMode string

// Supported incident failure modes.
const (
	FailureNone       FailureMode = "none"
	FailureMetrics    FailureMode = "metrics"
	FailureDeployment FailureMode = "deployment"
)

// Incident investigation step IDs.
const (
	IncidentIntakeStepID       = "incident_intake"
	InvestigationPlannerStepID = "investigation_planner"
	LogAnalyzerStepID          = "log_analyzer"
	MetricsAnalyzerStepID      = "metrics_analyzer"
	DeploymentAnalyzerStepID   = "deployment_analyzer"
	RunbookLoaderStepID        = "runbook_loader"
	EvidenceMergerStepID       = "evidence_merger"
	TimelineBuilderStepID      = "timeline_builder"
	HypothesisGeneratorStepID  = "hypothesis_generator"
	ImpactAssessorStepID       = "impact_assessor"
	RunbookMatcherStepID       = "runbook_matcher"
	RemediationPlannerStepID   = "remediation_planner"
	IncidentSummaryStepID      = "incident_summary"
)

// LogRecord contains one application log event.
type LogRecord struct {
	Timestamp string `json:"timestamp"`
	Service   string `json:"service"`
	Level     string `json:"level"`
	ErrorCode string `json:"error_code,omitempty"`
	Message   string `json:"message"`
}

// MetricSnapshot contains one service health measurement.
type MetricSnapshot struct {
	ObservedAt   string  `json:"observed_at"`
	P95LatencyMS int     `json:"p95_latency_ms"`
	ThresholdMS  int     `json:"threshold_ms"`
	ErrorRate    float64 `json:"error_rate"`
}

// DeploymentRecord contains one service deployment and timeout change.
type DeploymentRecord struct {
	DeploymentID        string `json:"deployment_id"`
	Service             string `json:"service"`
	Commit              string `json:"commit"`
	DeployedAt          string `json:"deployed_at"`
	TimeoutBeforeSecond int    `json:"timeout_before_seconds"`
	TimeoutAfterSecond  int    `json:"timeout_after_seconds"`
}

// RunbookRecord contains one operational response rule.
type RunbookRecord struct {
	RunbookID         string `json:"runbook_id"`
	Signal            string `json:"signal"`
	Cause             string `json:"cause"`
	RecommendedAction string `json:"recommended_action"`
}

// Scenario contains all synthetic source evidence for one incident.
type Scenario struct {
	IncidentID  string             `json:"incident_id"`
	Service     string             `json:"service"`
	StartedAt   string             `json:"started_at"`
	Symptom     string             `json:"symptom"`
	Logs        []LogRecord        `json:"logs"`
	Metrics     MetricSnapshot     `json:"metrics"`
	Deployments []DeploymentRecord `json:"deployments"`
	Runbook     RunbookRecord      `json:"runbook"`
}

// IncidentPacket is the normalized output of Incident Intake.
type IncidentPacket struct {
	IncidentID string `json:"incident_id"`
	Service    string `json:"service"`
	StartedAt  string `json:"started_at"`
	Symptom    string `json:"symptom"`
	Accepted   bool   `json:"accepted"`
}

// InvestigationPlan lists the parallel investigation tasks.
type InvestigationPlan struct {
	IncidentID string   `json:"incident_id"`
	Service    string   `json:"service"`
	Tasks      []string `json:"tasks"`
}

// LogAnalyzerInput contains the logs assigned to Log Analyzer.
type LogAnalyzerInput struct {
	IncidentID string      `json:"incident_id"`
	Service    string      `json:"service"`
	Logs       []LogRecord `json:"logs"`
}

// LogFinding contains the evidence extracted from application logs.
type LogFinding struct {
	Service      string `json:"service"`
	ErrorCode    string `json:"error_code"`
	ErrorCount   int    `json:"error_count"`
	FirstErrorAt string `json:"first_error_at"`
	Summary      string `json:"summary"`
}

// MetricsAnalyzerInput contains the metrics assigned to Metrics Analyzer.
type MetricsAnalyzerInput struct {
	IncidentID string         `json:"incident_id"`
	Service    string         `json:"service"`
	Metrics    MetricSnapshot `json:"metrics"`
}

// MetricFinding contains the interpreted latency and error metrics.
type MetricFinding struct {
	ObservedAt   string  `json:"observed_at"`
	P95LatencyMS int     `json:"p95_latency_ms"`
	ThresholdMS  int     `json:"threshold_ms"`
	ErrorRate    float64 `json:"error_rate"`
	State        string  `json:"state"`
	Summary      string  `json:"summary"`
}

// DeploymentAnalyzerInput contains deployment history for correlation.
type DeploymentAnalyzerInput struct {
	IncidentID  string             `json:"incident_id"`
	Service     string             `json:"service"`
	StartedAt   string             `json:"started_at"`
	Deployments []DeploymentRecord `json:"deployments"`
}

// DeploymentFinding contains the deployment selected as incident evidence.
type DeploymentFinding struct {
	DeploymentID string `json:"deployment_id"`
	Commit       string `json:"commit"`
	DeployedAt   string `json:"deployed_at"`
	ChangedField string `json:"changed_field"`
	BeforeValue  int    `json:"before_value"`
	AfterValue   int    `json:"after_value"`
	Correlated   bool   `json:"correlated"`
	Summary      string `json:"summary"`
}

// RunbookLoaderInput contains the runbook assigned for loading.
type RunbookLoaderInput struct {
	IncidentID string        `json:"incident_id"`
	Runbook    RunbookRecord `json:"runbook"`
}

// LoadedRunbook contains the normalized runbook data.
type LoadedRunbook struct {
	RunbookID         string `json:"runbook_id"`
	Signal            string `json:"signal"`
	Cause             string `json:"cause"`
	RecommendedAction string `json:"recommended_action"`
	Loaded            bool   `json:"loaded"`
}

// EvidenceInput contains the three analyzer findings being merged.
type EvidenceInput struct {
	Logs       LogFinding        `json:"logs"`
	Metrics    MetricFinding     `json:"metrics"`
	Deployment DeploymentFinding `json:"deployment"`
}

// EvidenceBundle contains normalized evidence from every analyzer branch.
type EvidenceBundle struct {
	Service        string `json:"service"`
	ErrorCode      string `json:"error_code"`
	ErrorCount     int    `json:"error_count"`
	FirstErrorAt   string `json:"first_error_at"`
	MetricState    string `json:"metric_state"`
	P95LatencyMS   int    `json:"p95_latency_ms"`
	ThresholdMS    int    `json:"threshold_ms"`
	MetricObserved string `json:"metric_observed_at"`
	DeploymentID   string `json:"deployment_id"`
	DeploymentAt   string `json:"deployment_at"`
	DeploymentLink bool   `json:"deployment_correlated"`
}

// Timeline contains the ordered deployment, error, and metric events.
type Timeline struct {
	DeploymentID   string `json:"deployment_id"`
	DeploymentAt   string `json:"deployment_at"`
	FirstErrorAt   string `json:"first_error_at"`
	MetricObserved string `json:"metric_observed_at"`
	Ordered        bool   `json:"ordered"`
}

// HypothesisInput contains the evidence and timeline used for diagnosis.
type HypothesisInput struct {
	Evidence EvidenceBundle `json:"evidence"`
	Timeline Timeline       `json:"timeline"`
}

// Hypothesis contains a proposed incident cause and supporting evidence.
type Hypothesis struct {
	Cause      string   `json:"cause"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence"`
}

// ImpactInput contains the evidence and hypothesis used for severity assessment.
type ImpactInput struct {
	Evidence   EvidenceBundle `json:"evidence"`
	Hypothesis Hypothesis     `json:"hypothesis"`
}

// ImpactAssessment contains the incident severity and customer effect.
type ImpactAssessment struct {
	Severity        string `json:"severity"`
	AffectedService string `json:"affected_service"`
	CustomerImpact  string `json:"customer_impact"`
}

// RunbookMatchInput contains the hypothesis and loaded runbook.
type RunbookMatchInput struct {
	Hypothesis Hypothesis    `json:"hypothesis"`
	Runbook    LoadedRunbook `json:"runbook"`
}

// RunbookMatch contains the selected operational action.
type RunbookMatch struct {
	RunbookID  string `json:"runbook_id"`
	Applicable bool   `json:"applicable"`
	Action     string `json:"action"`
}

// RemediationInput contains the diagnosis, impact, and runbook decision.
type RemediationInput struct {
	Hypothesis Hypothesis       `json:"hypothesis"`
	Impact     ImpactAssessment `json:"impact"`
	Runbook    RunbookMatch     `json:"runbook"`
}

// RemediationPlan contains the final operational response.
type RemediationPlan struct {
	Cause    string `json:"cause"`
	Priority string `json:"priority"`
	Action   string `json:"action"`
	Owner    string `json:"owner"`
}

// SummaryInput contains the diagnosis and remediation being summarized.
type SummaryInput struct {
	Hypothesis  Hypothesis      `json:"hypothesis"`
	Remediation RemediationPlan `json:"remediation"`
}

// IncidentSummary contains the final human readable incident conclusion.
type IncidentSummary struct {
	Title      string `json:"title"`
	RootCause  string `json:"root_cause"`
	Severity   string `json:"severity"`
	Resolution string `json:"resolution"`
}
