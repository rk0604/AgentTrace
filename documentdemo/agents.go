package documentdemo

import "fmt"

func loadDocuments(_ DocumentRequest) Documents {
	return Documents{
		ClaimDocument:  "Claim CLM-104 requests $42,000 for water damage.",
		PolicyDocument: "Policy covers water damage up to $50,000.",
	}
}

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

func mergeEvidence(input EvidenceInput) Evidence {
	return Evidence{
		Eligible: input.Reference.Covered && input.Claim.Amount <= input.Reference.CoverageLimit,
		Risk:     input.Risk.Risk,
	}
}

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
