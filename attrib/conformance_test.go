package attrib_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
)

type traceContractCase struct {
	Name  string          `json:"name"`
	Valid bool            `json:"valid"`
	Trace json.RawMessage `json:"trace"`
}

// TestTraceContractConformance runs the language neutral contract fixtures.
//
// Input
// t pointer to testing.T
// Test state and failure reporting.
//
// Output
// None
// The test fails when the Go decoder disagrees with fixture validity.
func TestTraceContractConformance(t *testing.T) {
	path := filepath.Join("..", "testdata", "trace-contract", "cases.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace contract cases: %v", err)
	}

	var cases []traceContractCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("decode trace contract cases: %v", err)
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			_, decodeErr := attrib.DecodeTrace(bytes.NewReader(test.Trace))
			if test.Valid && decodeErr != nil {
				t.Fatalf("expected valid trace, got %v", decodeErr)
			}
			if !test.Valid && decodeErr == nil {
				t.Fatal("expected invalid trace")
			}
		})
	}
}
