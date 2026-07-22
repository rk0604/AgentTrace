package incidentdemo

// SyntheticScenario returns deterministic incident evidence for the demo.
//
// Input
// None
//
// Output
// Scenario
// Incident logs, metrics, deployments, and runbook data.
func SyntheticScenario() Scenario {
	return Scenario{
		IncidentID: "INC-2026-071",
		Service:    "checkout-api",
		StartedAt:  "2026-07-20T10:00:00Z",
		Symptom:    "checkout requests are timing out",
		Logs: []LogRecord{
			{Timestamp: "2026-07-20T10:00:03Z", Service: "checkout-api", Level: "ERROR", ErrorCode: "REQUEST_TIMEOUT", Message: "payment provider request exceeded configured timeout"},
			{Timestamp: "2026-07-20T10:00:04Z", Service: "checkout-api", Level: "ERROR", ErrorCode: "REQUEST_TIMEOUT", Message: "payment provider request exceeded configured timeout"},
			{Timestamp: "2026-07-20T10:00:05Z", Service: "checkout-api", Level: "INFO", Message: "retry queue depth increased"},
		},
		Metrics: MetricSnapshot{ObservedAt: "2026-07-20T10:01:00Z", P95LatencyMS: 2400, ThresholdMS: 1000, ErrorRate: 0.18},
		Deployments: []DeploymentRecord{
			{DeploymentID: "deploy-41", Service: "checkout-api", Commit: "a1b2c3d", DeployedAt: "2026-07-20T08:00:00Z", TimeoutBeforeSecond: 30, TimeoutAfterSecond: 30},
			{DeploymentID: "deploy-42", Service: "checkout-api", Commit: "d4e5f6a", DeployedAt: "2026-07-20T09:55:00Z", TimeoutBeforeSecond: 30, TimeoutAfterSecond: 3},
		},
		Runbook: RunbookRecord{
			RunbookID:         "RB-TIMEOUT-01",
			Signal:            "REQUEST_TIMEOUT",
			Cause:             "timeout_regression",
			RecommendedAction: "rollback deploy-42 and restore the timeout to 30 seconds",
		},
	}
}
