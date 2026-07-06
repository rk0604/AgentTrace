package toypipeline

import (
	"encoding/json"
	"fmt"

	"github.com/rk0604/AgentTrace/attrib"
)

func Checkers() map[string]attrib.StepChecker {
	return map[string]attrib.StepChecker{
		ExtractorStepID:   checkExtractor,
		ReferenceStepID:   checkReference,
		ComparatorStepID:  checkComparator,
		SynthesizerStepID: checkSynthesizer,
	}
}

func checkExtractor(step attrib.Step) (attrib.CheckResult, error) {
	var output Claim
	if err := json.Unmarshal(step.Output, &output); err != nil {
		return attrib.CheckResult{}, err
	}

	expected := Claim{Revenue: "$4.2M", Period: "Q1"}
	if output != expected {
		return attrib.Fail(fmt.Sprintf("expected %+v, got %+v", expected, output)), nil
	}

	return attrib.Pass(), nil
}

func checkReference(step attrib.Step) (attrib.CheckResult, error) {
	var output Claim
	if err := json.Unmarshal(step.Output, &output); err != nil {
		return attrib.CheckResult{}, err
	}

	expected := Claim{Revenue: "$4.2M", Period: "Q2"}
	if output != expected {
		return attrib.Fail(fmt.Sprintf("expected %+v, got %+v", expected, output)), nil
	}

	return attrib.Pass(), nil
}

func checkComparator(step attrib.Step) (attrib.CheckResult, error) {
	var input ComparisonInput
	if err := json.Unmarshal(step.Input, &input); err != nil {
		return attrib.CheckResult{}, err
	}

	var output Verdict
	if err := json.Unmarshal(step.Output, &output); err != nil {
		return attrib.CheckResult{}, err
	}

	expected := compareClaims(input)
	if output != expected {
		return attrib.Fail(fmt.Sprintf("expected %+v, got %+v", expected, output)), nil
	}

	return attrib.Pass(), nil
}

func checkSynthesizer(step attrib.Step) (attrib.CheckResult, error) {
	var input Verdict
	if err := json.Unmarshal(step.Input, &input); err != nil {
		return attrib.CheckResult{}, err
	}

	var output Synthesis
	if err := json.Unmarshal(step.Output, &output); err != nil {
		return attrib.CheckResult{}, err
	}

	expected := synthesize(input)
	if output != expected {
		return attrib.Fail(fmt.Sprintf("expected %+v, got %+v", expected, output)), nil
	}

	return attrib.Pass(), nil
}
