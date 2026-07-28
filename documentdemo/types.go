package documentdemo

// Documents contains synthetic claim and policy text.
type Documents struct {
	ClaimDocument  string `json:"claim_document"`
	PolicyDocument string `json:"policy_document"`
}

// DocumentRequest identifies the documents requested by the pipeline.
type DocumentRequest struct {
	ClaimPath  string `json:"claim_path"`
	PolicyPath string `json:"policy_path"`
}

// ClaimInput contains claim text for extraction.
type ClaimInput struct {
	Document string `json:"document"`
}

// ClaimFacts contains structured claim facts.
type ClaimFacts struct {
	ClaimID  string  `json:"claim_id"`
	Amount   float64 `json:"amount"`
	Incident string  `json:"incident"`
}

// PolicyInput contains policy text for reference analysis.
type PolicyInput struct {
	Document string `json:"document"`
}

// PolicyFacts contains structured coverage facts.
type PolicyFacts struct {
	CoverageLimit float64 `json:"coverage_limit"`
	Covered       bool    `json:"covered"`
}

// RiskInput contains claim facts used for classification.
type RiskInput struct {
	Amount   float64 `json:"amount"`
	Incident string  `json:"incident"`
}

// RiskFinding contains a risk classification and rationale.
type RiskFinding struct {
	Risk      string `json:"risk"`
	Rationale string `json:"rationale"`
}

// EvidenceInput contains branch outputs consumed by the merger.
type EvidenceInput struct {
	Claim     ClaimFacts  `json:"claim"`
	Reference PolicyFacts `json:"reference"`
	Risk      RiskFinding `json:"risk"`
}

// Evidence contains the merged claim decision facts.
type Evidence struct {
	Eligible bool   `json:"eligible"`
	Risk     string `json:"risk"`
}

// Report contains the final decision and summary.
type Report struct {
	Decision string `json:"decision"`
	Summary  string `json:"summary"`
}
