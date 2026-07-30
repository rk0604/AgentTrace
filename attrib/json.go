package attrib

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
)

// DecodeTrace reads one JSON trace.
//
// Input
// reader io.Reader
// A stream containing one JSON object that matches the Trace structure.
//
// Output
// Trace
// The decoded generic trace. Input and Output payloads remain raw JSON.
//
// error
// Non nil when the JSON is invalid, contains an unknown schema field, or contains
// more than one JSON value.
func DecodeTrace(reader io.Reader) (Trace, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var trace Trace
	if err := decoder.Decode(&trace); err != nil {
		return Trace{}, fmt.Errorf("decode trace JSON: %w", err)
	}

	// A second decode must reach the end of the stream.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Trace{}, fmt.Errorf("decode trace JSON: multiple JSON values")
		}

		return Trace{}, fmt.Errorf("decode trace JSON: %w", err)
	}

	if trace.Version == 0 {
		// Traces created before explicit versioning use the first schema.
		trace.Version = CurrentTraceVersion
	}
	if err := ValidateTraceSchema(trace); err != nil {
		return Trace{}, err
	}

	return trace, nil
}

// ValidateTraceSchema checks the external trace data contract.
//
// Input
// trace Trace
// Trace containing generic run and step data.
//
// Output
// error
// Non nil when required fields, versions, run IDs, or payloads are invalid.
func ValidateTraceSchema(trace Trace) error {
	_, err := validateAndOrderTrace(trace)
	return err
}

// validateAndOrderTrace validates the complete trace and returns dependency order.
//
// Input
// trace Trace
// Trace containing generic run, step, payload, and dependency data.
//
// Output
// []Step
// Validated steps in dependency order.
//
// error
// Non nil when the trace fields or graph are invalid.
func validateAndOrderTrace(trace Trace) ([]Step, error) {
	if trace.Version != 0 && trace.Version != CurrentTraceVersion {
		return nil, fmt.Errorf("unsupported trace version %d", trace.Version)
	}
	if strings.TrimSpace(trace.RunID) == "" {
		return nil, fmt.Errorf("trace has an empty run_id")
	}
	if len(trace.Steps) == 0 {
		return nil, fmt.Errorf("trace has no steps")
	}
	if len(trace.Steps) > MaxTraceSteps {
		return nil, fmt.Errorf("trace has %d steps which exceeds limit %d", len(trace.Steps), MaxTraceSteps)
	}

	for position, step := range trace.Steps {
		if step.RunID != trace.RunID {
			return nil, fmt.Errorf("step at position %d has run_id %q instead of %q", position, step.RunID, trace.RunID)
		}
		if strings.TrimSpace(step.StepID) == "" {
			return nil, fmt.Errorf("step at position %d has an empty step_id", position)
		}
		if strings.TrimSpace(step.AgentName) == "" {
			return nil, fmt.Errorf("step %q has an empty agent_name", step.StepID)
		}
		if len(step.DependsOn) > MaxStepDependencies {
			return nil, fmt.Errorf(
				"step %q has %d dependencies which exceeds limit %d",
				step.StepID,
				len(step.DependsOn),
				MaxStepDependencies,
			)
		}
		if len(step.Input) == 0 || !json.Valid(step.Input) {
			return nil, fmt.Errorf("step %q has invalid input JSON", step.StepID)
		}
		if len(step.Output) == 0 || !json.Valid(step.Output) {
			return nil, fmt.Errorf("step %q has invalid output JSON", step.StepID)
		}
		if step.Timestamp.IsZero() {
			return nil, fmt.Errorf("step %q has an empty timestamp", step.StepID)
		}
		if strings.TrimSpace(step.Status) == "" {
			return nil, fmt.Errorf("step %q has an empty status", step.StepID)
		}
		if step.Confidence != nil &&
			(math.IsNaN(*step.Confidence) ||
				math.IsInf(*step.Confidence, 0) ||
				*step.Confidence < 0 ||
				*step.Confidence > 1) {
			return nil, fmt.Errorf("step %q has confidence outside zero through one", step.StepID)
		}
	}

	orderedSteps, err := TopologicalSort(trace)
	if err != nil {
		return nil, err
	}

	return orderedSteps, nil
}

// EncodeTrace writes one versioned JSON trace.
//
// Input
// writer io.Writer
// Destination for the encoded trace.
//
// trace Trace
// Generic trace to validate and encode.
//
// Output
// error
// Non nil when the trace is invalid or cannot be written.
func EncodeTrace(writer io.Writer, trace Trace) error {
	if trace.Version == 0 {
		trace.Version = CurrentTraceVersion
	}
	if err := ValidateTraceSchema(trace); err != nil {
		return err
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(trace); err != nil {
		return fmt.Errorf("encode trace JSON: %w", err)
	}

	return nil
}

// EncodeResult writes one JSON attribution result.
//
// Input
// writer io.Writer
// The destination for the JSON result.
//
// result AttributionResult
// The attribution result to encode.
//
// Output
// error
// Non nil when the result cannot be written.
func EncodeResult(writer io.Writer, result AttributionResult) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("encode attribution result JSON: %w", err)
	}

	return nil
}
