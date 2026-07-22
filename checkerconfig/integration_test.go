package checkerconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/checkerconfig"
)

func TestExampleConfigurationAttributesJSONTrace(t *testing.T) {
	checkers := loadExampleCheckers(t)
	trace := loadExampleTrace(t, "trace-reference-failure.json")

	result, err := attrib.FindRootCause(trace, checkers)
	if err != nil {
		t.Fatalf("FindRootCause returned error: %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if result.RootCause == nil || result.RootCause.StepID != "reference" {
		t.Fatalf("expected reference root cause, got %+v", result.RootCause)
	}
	if result.RootCause.Reason != "Reference misread the reporting period" {
		t.Fatalf("unexpected root cause reason %q", result.RootCause.Reason)
	}
}

func TestExampleConfigurationPassesHealthyJSONTrace(t *testing.T) {
	checkers := loadExampleCheckers(t)
	trace := loadExampleTrace(t, "trace-healthy.json")

	result, err := attrib.FindRootCause(trace, checkers)
	if err != nil {
		t.Fatalf("FindRootCause returned error: %v", err)
	}
	if result.Status != "passed" {
		t.Fatalf("expected passed status, got %q", result.Status)
	}
	if result.RootCause != nil {
		t.Fatalf("expected no root cause, got %+v", result.RootCause)
	}
}

func loadExampleCheckers(t *testing.T) map[string]attrib.StepChecker {
	t.Helper()

	file, err := os.Open(filepath.Join("..", "examples", "toy-checkers.json"))
	if err != nil {
		t.Fatalf("open example checker configuration: %v", err)
	}
	defer file.Close()

	config, err := checkerconfig.Decode(file)
	if err != nil {
		t.Fatalf("decode example checker configuration: %v", err)
	}

	checkers, err := checkerconfig.Build(config)
	if err != nil {
		t.Fatalf("build example checkers: %v", err)
	}

	return checkers
}

func loadExampleTrace(t *testing.T, name string) attrib.Trace {
	t.Helper()

	file, err := os.Open(filepath.Join("..", "examples", name))
	if err != nil {
		t.Fatalf("open example trace: %v", err)
	}
	defer file.Close()

	trace, err := attrib.DecodeTrace(file)
	if err != nil {
		t.Fatalf("decode example trace: %v", err)
	}

	return trace
}
