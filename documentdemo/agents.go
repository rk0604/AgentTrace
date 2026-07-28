package documentdemo

import "fmt"

// loadDocuments returns synthetic source documents.
//
// Input
// request DocumentRequest
// Document paths requested by the pipeline.
//
// Output
// Documents
// Claim and policy text.
func loadDocuments(_ DocumentRequest) Documents {
	return Documents{
		ClaimDocument:  "Claim CLM-104 requests $42,000 for water damage.",
		PolicyDocument: "Policy covers water damage up to $50,000.",
	}
}

// extractClaim converts claim text into structured facts.
//
// Input
// input ClaimInput
// Claim document text.
//
// failureMode FailureMode
// Optional extraction failure to inject.
//
// Output
// ClaimFacts
// Structured claim values.
func extractClaim(input ClaimInput, failureMode FailureMode) ClaimFacts {
	amount := 42000.0
	if failureMode == FailureExtraction {
		amount = 420000.0
	}

	return ClaimFacts{
		ClaimID:  "CLM-104",
		Amount:   amount,
		Incident: "water_damage",
	}
}

// checkPolicy converts policy text into coverage facts.
//
// Input
// input PolicyInput
// Policy document text.
//
// failureMode FailureMode
// Optional reference failure to inject.
//
// Output
// PolicyFacts
// Structured coverage values.
func checkPolicy(input PolicyInput, failureMode FailureMode) PolicyFacts {
	covered := true
	if failureMode == FailureReference {
		covered = false
	}

	return PolicyFacts{
		CoverageLimit: 50000,
		Covered:       covered,
	}
}

// analyzeRisk classifies risk from actual claim inputs.
//
// Input
// input RiskInput
// Claim amount and incident type.
//
// Output
// RiskFinding
// Risk label and rationale.
func analyzeRisk(input RiskInput) RiskFinding {
	if input.Amount > 100000 {
		return RiskFinding{
			Risk:      "high",
			Rationale: "claim amount exceeds the high risk threshold",
		}
	}

	return RiskFinding{
		Risk:      "moderate",
		Rationale: "claim amount is below the high risk threshold",
	}
}

// mergeEvidence calculates eligibility from actual branch outputs.
//
// Input
// input EvidenceInput
// Claim, policy, and risk findings.
//
// Output
// Evidence
// Merged eligibility and risk values.
func mergeEvidence(input EvidenceInput) Evidence {
	return Evidence{
		Eligible: input.Reference.Covered && input.Claim.Amount <= input.Reference.CoverageLimit,
		Risk:     input.Risk.Risk,
	}
}

// generateReport creates the final decision from merged evidence.
//
// Input
// input Evidence
// Eligibility and risk values.
//
// Output
// Report
// Decision and human readable summary.
func generateReport(input Evidence) Report {
	decision := "manual_review"
	if input.Eligible {
		decision = "approve"
	}

	return Report{
		Decision: decision,
		Summary: fmt.Sprintf(
			"Decision is %s with %s risk.",
			decision,
			input.Risk,
		),
	}
}
