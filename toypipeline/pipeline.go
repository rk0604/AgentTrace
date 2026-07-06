package toypipeline

import (
	"encoding/json"
	"time"

	"github.com/rk0604/AgentTrace/attrib"
)

const (
	RunID             = "toy-reference-period-failure"
	ExtractorStepID   = "extractor"
	ReferenceStepID   = "reference"
	ComparatorStepID  = "comparator"
	SynthesizerStepID = "synthesizer"
)

type Documents struct {
	PressRelease string `json:"press_release"`
	Reference    string `json:"reference"`
}

type Claim struct {
	Revenue string `json:"revenue"`
	Period  string `json:"period"`
}

type ComparisonInput struct {
	Extractor Claim `json:"extractor"`
	Reference Claim `json:"reference"`
}

type Verdict struct {
	Result string `json:"result"`
}

type Synthesis struct {
	Answer string `json:"answer"`
}

func SyntheticDocuments() Documents {
	return Documents{
		PressRelease: "Acme reported revenue of $4.2M for Q1.",
		Reference:    "Acme reported revenue of $4.2M for the three months ended June 30.",
	}
}

func Run(injectReferenceFailure bool) attrib.Trace {
	docs := SyntheticDocuments()
	baseTime := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	extractorOutput := extractClaim(docs.PressRelease)
	referenceOutput := extractReferenceClaim(docs.Reference, injectReferenceFailure)
	comparisonInput := ComparisonInput{
		Extractor: extractorOutput,
		Reference: referenceOutput,
	}
	comparatorOutput := compareClaims(comparisonInput)
	synthesizerOutput := synthesize(comparatorOutput)

	return attrib.Trace{
		RunID: RunID,
		Steps: []attrib.Step{
			{
				RunID:     RunID,
				StepID:    ExtractorStepID,
				AgentName: "Extractor",
				DependsOn: nil,
				Input:     mustRaw(map[string]string{"document": docs.PressRelease}),
				Output:    mustRaw(extractorOutput),
				ModelUsed: "deterministic-stub",
				Timestamp: baseTime,
				Status:    "ok",
			},
			{
				RunID:     RunID,
				StepID:    ReferenceStepID,
				AgentName: "Reference",
				DependsOn: nil,
				Input:     mustRaw(map[string]string{"document": docs.Reference}),
				Output:    mustRaw(referenceOutput),
				ModelUsed: "deterministic-stub",
				Timestamp: baseTime.Add(time.Second),
				Status:    "ok",
			},
			{
				RunID:     RunID,
				StepID:    ComparatorStepID,
				AgentName: "Comparator",
				DependsOn: []string{ExtractorStepID, ReferenceStepID},
				Input:     mustRaw(comparisonInput),
				Output:    mustRaw(comparatorOutput),
				ModelUsed: "deterministic-stub",
				Timestamp: baseTime.Add(2 * time.Second),
				Status:    "ok",
			},
			{
				RunID:     RunID,
				StepID:    SynthesizerStepID,
				AgentName: "Synthesizer",
				DependsOn: []string{ComparatorStepID},
				Input:     mustRaw(comparatorOutput),
				Output:    mustRaw(synthesizerOutput),
				ModelUsed: "deterministic-stub",
				Timestamp: baseTime.Add(3 * time.Second),
				Status:    "ok",
			},
		},
	}
}

func extractClaim(_ string) Claim {
	return Claim{
		Revenue: "$4.2M",
		Period:  "Q1",
	}
}

func extractReferenceClaim(_ string, injectFailure bool) Claim {
	period := "Q2"
	if injectFailure {
		period = "Q1"
	}

	return Claim{
		Revenue: "$4.2M",
		Period:  period,
	}
}

func compareClaims(input ComparisonInput) Verdict {
	if input.Extractor == input.Reference {
		return Verdict{Result: "MATCH"}
	}

	return Verdict{Result: "MISMATCH"}
}

func synthesize(verdict Verdict) Synthesis {
	if verdict.Result == "MATCH" {
		return Synthesis{Answer: "The press release and reference agree."}
	}

	return Synthesis{Answer: "The press release and reference disagree."}
}

func mustRaw(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}

	return raw
}
