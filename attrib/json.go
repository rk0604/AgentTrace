package attrib

import (
	"encoding/json"
	"fmt"
	"io"
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

	return trace, nil
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

	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("encode attribution result JSON: %w", err)
	}

	return nil
}
