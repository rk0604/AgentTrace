package attrib_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
)

func TestDecodeTracePreservesDomainPayloadsAsRawJSON(t *testing.T) {
	input := `{
		"run_id": "run-json",
		"steps": [{
			"run_id": "run-json",
			"step_id": "source",
			"agent_name": "Source",
			"depends_on": [],
			"input": {"document": "example"},
			"output": {"score": 0.9},
			"model_used": "test-model",
			"timestamp": "2026-07-21T12:00:00Z",
			"status": "ok"
		}]
	}`

	trace, err := attrib.DecodeTrace(strings.NewReader(input))
	if err != nil {
		t.Fatalf("DecodeTrace returned error: %v", err)
	}

	if trace.RunID != "run-json" {
		t.Fatalf("expected run ID run-json, got %q", trace.RunID)
	}
	if len(trace.Steps) != 1 {
		t.Fatalf("expected one step, got %d", len(trace.Steps))
	}

	var output map[string]float64
	if err := json.Unmarshal(trace.Steps[0].Output, &output); err != nil {
		t.Fatalf("decode raw output: %v", err)
	}
	if output["score"] != 0.9 {
		t.Fatalf("expected output score 0.9, got %v", output["score"])
	}
}

func TestDecodeTraceRejectsUnknownSchemaField(t *testing.T) {
	input := `{"run_id":"run-json","steps":[],"created_at":"not allowed"}`

	_, err := attrib.DecodeTrace(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), `unknown field "created_at"`) {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestDecodeTraceRejectsMultipleJSONValues(t *testing.T) {
	input := `{"run_id":"first","steps":[]} {"run_id":"second","steps":[]}`

	_, err := attrib.DecodeTrace(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestEncodeResultWritesJSON(t *testing.T) {
	result := attrib.AttributionResult{
		RunID:  "run-json",
		Status: "failed",
		RootCause: &attrib.RootCause{
			StepID:    "source",
			AgentName: "Source",
			Reason:    "incorrect output",
		},
		CheckedStepIDs: []string{"source"},
	}
	var output bytes.Buffer

	if err := attrib.EncodeResult(&output, result); err != nil {
		t.Fatalf("EncodeResult returned error: %v", err)
	}

	var decoded attrib.AttributionResult
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("decode encoded result: %v", err)
	}
	if decoded.RootCause == nil || decoded.RootCause.StepID != "source" {
		t.Fatalf("unexpected encoded root cause %+v", decoded.RootCause)
	}
}
