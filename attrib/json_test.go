package attrib_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

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
	if trace.Version != attrib.CurrentTraceVersion {
		t.Fatalf("expected legacy trace to normalize to version %d, got %d", attrib.CurrentTraceVersion, trace.Version)
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

func TestDecodeTraceRejectsInvalidSchema(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMessage string
	}{
		{
			name:        "unsupported version",
			input:       `{"version":2,"run_id":"run-json","steps":[]}`,
			wantMessage: "unsupported trace version 2",
		},
		{
			name:        "empty run ID",
			input:       `{"version":1,"run_id":"","steps":[]}`,
			wantMessage: "empty run_id",
		},
		{
			name:        "no steps",
			input:       `{"version":1,"run_id":"run-json","steps":[]}`,
			wantMessage: "trace has no steps",
		},
		{
			name: "mismatched step run ID",
			input: `{
				"version":1,
				"run_id":"run-json",
				"steps":[{
					"run_id":"different",
					"step_id":"source",
					"agent_name":"Source",
					"depends_on":[],
					"input":{},
					"output":{},
					"model_used":"model",
					"timestamp":"2026-07-28T12:00:00Z",
					"status":"ok"
				}]
			}`,
			wantMessage: "instead of",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := attrib.DecodeTrace(strings.NewReader(test.input))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("expected error containing %q, got %q", test.wantMessage, err)
			}
		})
	}
}

func TestEncodeTraceWritesCurrentVersion(t *testing.T) {
	trace := attrib.Trace{
		RunID: "run-json",
		Steps: []attrib.Step{
			{
				RunID:     "run-json",
				StepID:    "source",
				AgentName: "Source",
				Input:     json.RawMessage(`{"text":"input"}`),
				Output:    json.RawMessage(`{"text":"output"}`),
				Timestamp: time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC),
				Status:    "ok",
			},
		},
	}
	var output bytes.Buffer

	if err := attrib.EncodeTrace(&output, trace); err != nil {
		t.Fatalf("EncodeTrace returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("decode encoded trace: %v", err)
	}
	if decoded["version"] != float64(attrib.CurrentTraceVersion) {
		t.Fatalf("expected version %d, got %v", attrib.CurrentTraceVersion, decoded["version"])
	}
}

func TestValidateTraceSchemaEnforcesResourceLimits(t *testing.T) {
	tooManySteps := attrib.Trace{
		Version: attrib.CurrentTraceVersion,
		RunID:   "large-run",
		Steps:   make([]attrib.Step, attrib.MaxTraceSteps+1),
	}
	err := attrib.ValidateTraceSchema(tooManySteps)
	if err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("expected step limit error, got %v", err)
	}

	dependencies := make([]string, attrib.MaxStepDependencies+1)
	trace := attrib.Trace{
		Version: attrib.CurrentTraceVersion,
		RunID:   "dependency-run",
		Steps: []attrib.Step{
			{
				RunID:     "dependency-run",
				StepID:    "consumer",
				AgentName: "Consumer",
				DependsOn: dependencies,
				Input:     json.RawMessage(`{}`),
				Output:    json.RawMessage(`{}`),
				Timestamp: time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC),
				Status:    "ok",
			},
		},
	}
	err = attrib.ValidateTraceSchema(trace)
	if err == nil || !strings.Contains(err.Error(), "dependencies which exceeds limit") {
		t.Fatalf("expected dependency limit error, got %v", err)
	}
}

func TestValidateTraceSchemaRejectsInvalidConfidence(t *testing.T) {
	confidence := 1.1
	trace := attrib.Trace{
		Version: attrib.CurrentTraceVersion,
		RunID:   "confidence-run",
		Steps: []attrib.Step{
			{
				RunID:      "confidence-run",
				StepID:     "source",
				AgentName:  "Source",
				Input:      json.RawMessage(`{}`),
				Output:     json.RawMessage(`{}`),
				Confidence: &confidence,
				Timestamp:  time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC),
				Status:     "ok",
			},
		},
	}

	err := attrib.ValidateTraceSchema(trace)

	if err == nil || !strings.Contains(err.Error(), "confidence outside") {
		t.Fatalf("expected confidence error, got %v", err)
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
