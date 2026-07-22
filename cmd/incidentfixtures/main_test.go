package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
)

func TestWriteFixturesProducesValidTraceJSON(t *testing.T) {
	directory := t.TempDir()
	if err := writeFixtures(directory); err != nil {
		t.Fatalf("writeFixtures returned error: %v", err)
	}

	names := []string{
		"incident-healthy.json",
		"incident-metrics-failure.json",
		"incident-deployment-failure.json",
	}
	for _, name := range names {
		file, err := os.Open(filepath.Join(directory, name))
		if err != nil {
			t.Fatalf("open fixture %q: %v", name, err)
		}

		trace, decodeErr := attrib.DecodeTrace(file)
		closeErr := file.Close()
		if decodeErr != nil {
			t.Fatalf("decode fixture %q: %v", name, decodeErr)
		}
		if closeErr != nil {
			t.Fatalf("close fixture %q: %v", name, closeErr)
		}
		if len(trace.Steps) != 13 {
			t.Fatalf("expected fixture %q to contain 13 steps, got %d", name, len(trace.Steps))
		}
	}
}

func TestCheckedInFixturesMatchGenerator(t *testing.T) {
	directory := t.TempDir()
	if err := writeFixtures(directory); err != nil {
		t.Fatalf("writeFixtures returned error: %v", err)
	}

	names := []string{
		"incident-healthy.json",
		"incident-metrics-failure.json",
		"incident-deployment-failure.json",
	}
	for _, name := range names {
		generated, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatalf("read generated fixture %q: %v", name, err)
		}

		checkedIn, err := os.ReadFile(filepath.Join("..", "..", "examples", name))
		if err != nil {
			t.Fatalf("read checked in fixture %q: %v", name, err)
		}

		// Compact both documents so platform line endings do not affect the comparison.
		var generatedJSON bytes.Buffer
		if err := json.Compact(&generatedJSON, generated); err != nil {
			t.Fatalf("compact generated fixture %q: %v", name, err)
		}
		var checkedInJSON bytes.Buffer
		if err := json.Compact(&checkedInJSON, checkedIn); err != nil {
			t.Fatalf("compact checked in fixture %q: %v", name, err)
		}
		if !bytes.Equal(generatedJSON.Bytes(), checkedInJSON.Bytes()) {
			t.Fatalf("checked in fixture %q is stale", name)
		}
	}
}
