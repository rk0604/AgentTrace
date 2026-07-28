package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/checkerconfig"
)

func TestPythonRecordedTracePassesGoAttribution(t *testing.T) {
	traceFile, err := os.Open(filepath.Join("..", "examples", "python-recorded-trace.json"))
	if err != nil {
		t.Fatalf("open Python trace: %v", err)
	}
	trace, decodeTraceErr := attrib.DecodeTrace(traceFile)
	closeTraceErr := traceFile.Close()
	if decodeTraceErr != nil {
		t.Fatalf("decode Python trace: %v", decodeTraceErr)
	}
	if closeTraceErr != nil {
		t.Fatalf("close Python trace: %v", closeTraceErr)
	}

	checkerFile, err := os.Open(filepath.Join("..", "examples", "python-checkers.json"))
	if err != nil {
		t.Fatalf("open Python checker configuration: %v", err)
	}
	config, decodeConfigErr := checkerconfig.Decode(checkerFile)
	closeConfigErr := checkerFile.Close()
	if decodeConfigErr != nil {
		t.Fatalf("decode Python checker configuration: %v", decodeConfigErr)
	}
	if closeConfigErr != nil {
		t.Fatalf("close Python checker configuration: %v", closeConfigErr)
	}

	checkers, err := checkerconfig.Build(config)
	if err != nil {
		t.Fatalf("build Python checkers: %v", err)
	}
	result, err := attrib.FindRootCause(trace, checkers)
	if err != nil {
		t.Fatalf("attribute Python trace: %v", err)
	}
	if result.Status != "passed" {
		t.Fatalf("expected Python trace to pass, got %q", result.Status)
	}
}
