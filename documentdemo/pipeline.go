package documentdemo

import (
	"fmt"
	"time"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/recorder"
)

const (
	DocumentLoaderStepID       = "document_loader"
	InformationExtractorStepID = "information_extractor"
	ReferenceCheckerStepID     = "reference_checker"
	RiskAnalyzerStepID         = "risk_analyzer"
	EvidenceMergerStepID       = "evidence_merger"
	ReportGeneratorStepID      = "report_generator"
)

// FailureMode identifies the incorrect behavior injected into the demo.
type FailureMode string

const (
	FailureNone       FailureMode = "none"
	FailureExtraction FailureMode = "extraction"
	FailureReference  FailureMode = "reference"
)

// Run executes and records the document review pipeline.
//
// Input
// failureMode FailureMode
// Failure injected into an upstream agent or FailureNone.
//
// Output
// attrib.Trace
// Completed versioned trace captured through the generic recorder.
//
// error
// Non nil when the failure mode or recording lifecycle is invalid.
func Run(failureMode FailureMode) (attrib.Trace, error) {
	if err := validateFailureMode(failureMode); err != nil {
		return attrib.Trace{}, err
	}

	clock := sequenceClock(time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC))
	run, err := recorder.StartRun("document-review-"+string(failureMode), recorder.Options{Clock: clock})
	if err != nil {
		return attrib.Trace{}, err
	}

	request := DocumentRequest{
		ClaimPath:  "claim-CLM-104.txt",
		PolicyPath: "policy-CLM-104.txt",
	}
	if err := run.StartStep(recorder.StepStart{
		StepID:    DocumentLoaderStepID,
		AgentName: "Document Loader",
		Input:     request,
		ModelUsed: "deterministic-function",
	}); err != nil {
		return attrib.Trace{}, err
	}
	documents := loadDocuments(request)
	if err := run.FinishStep(DocumentLoaderStepID, documents, nil); err != nil {
		return attrib.Trace{}, err
	}

	claimInput := ClaimInput{Document: documents.ClaimDocument}
	if err := run.StartStep(recorder.StepStart{
		StepID:    InformationExtractorStepID,
		AgentName: "Information Extractor",
		DependsOn: []string{DocumentLoaderStepID},
		Input:     claimInput,
		ModelUsed: "deterministic-function",
	}); err != nil {
		return attrib.Trace{}, err
	}
	claim := extractClaim(claimInput, failureMode)
	extractionConfidence := 0.93
	if err := run.FinishStep(InformationExtractorStepID, claim, &extractionConfidence); err != nil {
		return attrib.Trace{}, err
	}

	policyInput := PolicyInput{Document: documents.PolicyDocument}
	if err := run.StartStep(recorder.StepStart{
		StepID:    ReferenceCheckerStepID,
		AgentName: "Reference Checker",
		DependsOn: []string{DocumentLoaderStepID},
		Input:     policyInput,
		ModelUsed: "deterministic-function",
	}); err != nil {
		return attrib.Trace{}, err
	}
	policy := checkPolicy(policyInput, failureMode)
	referenceConfidence := 0.97
	if err := run.FinishStep(ReferenceCheckerStepID, policy, &referenceConfidence); err != nil {
		return attrib.Trace{}, err
	}

	riskInput := RiskInput{Amount: claim.Amount, Incident: claim.Incident}
	if err := run.StartStep(recorder.StepStart{
		StepID:    RiskAnalyzerStepID,
		AgentName: "Risk Analyzer",
		DependsOn: []string{InformationExtractorStepID},
		Input:     riskInput,
		ModelUsed: "deterministic-function",
	}); err != nil {
		return attrib.Trace{}, err
	}
	risk := analyzeRisk(riskInput)
	if err := run.FinishStep(RiskAnalyzerStepID, risk, nil); err != nil {
		return attrib.Trace{}, err
	}

	evidenceInput := EvidenceInput{
		Claim:     claim,
		Reference: policy,
		Risk:      risk,
	}
	if err := run.StartStep(recorder.StepStart{
		StepID:    EvidenceMergerStepID,
		AgentName: "Evidence Merger",
		DependsOn: []string{
			InformationExtractorStepID,
			ReferenceCheckerStepID,
			RiskAnalyzerStepID,
		},
		Input:     evidenceInput,
		ModelUsed: "deterministic-function",
	}); err != nil {
		return attrib.Trace{}, err
	}
	evidence := mergeEvidence(evidenceInput)
	if err := run.FinishStep(EvidenceMergerStepID, evidence, nil); err != nil {
		return attrib.Trace{}, err
	}

	if err := run.StartStep(recorder.StepStart{
		StepID:    ReportGeneratorStepID,
		AgentName: "Report Generator",
		DependsOn: []string{EvidenceMergerStepID},
		Input:     evidence,
		ModelUsed: "deterministic-function",
	}); err != nil {
		return attrib.Trace{}, err
	}
	report := generateReport(evidence)
	if err := run.FinishStep(ReportGeneratorStepID, report, nil); err != nil {
		return attrib.Trace{}, err
	}

	return run.Trace()
}

// validateFailureMode checks the requested document failure.
//
// Input
// failureMode FailureMode
// Requested injected behavior.
//
// Output
// error
// Non nil when the mode is unsupported.
func validateFailureMode(failureMode FailureMode) error {
	switch failureMode {
	case FailureNone, FailureExtraction, FailureReference:
		return nil
	default:
		return fmt.Errorf("unsupported document failure mode %q", failureMode)
	}
}

// sequenceClock creates deterministic one second timestamps.
//
// Input
// start time.Time
// First timestamp returned by the clock.
//
// Output
// function returning time.Time
// Clock that advances one second after each call.
func sequenceClock(start time.Time) func() time.Time {
	next := start

	return func() time.Time {
		current := next
		next = next.Add(time.Second)
		return current
	}
}
