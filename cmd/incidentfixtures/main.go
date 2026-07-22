package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rk0604/AgentTrace/incidentdemo"
)

type fixtureDefinition struct {
	Name        string
	FailureMode incidentdemo.FailureMode
}

func main() {
	outputDirectory := flag.String("output", "examples", "directory that receives incident trace fixtures")
	flag.Parse()

	if err := writeFixtures(*outputDirectory); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// writeFixtures generates every incident trace example.
//
// Input
// outputDirectory string
// Directory that receives generated JSON files.
//
// Output
// error
// Non nil when a trace cannot be generated, encoded, or written.
func writeFixtures(outputDirectory string) error {
	definitions := []fixtureDefinition{
		{Name: "incident-healthy.json", FailureMode: incidentdemo.FailureNone},
		{Name: "incident-metrics-failure.json", FailureMode: incidentdemo.FailureMetrics},
		{Name: "incident-deployment-failure.json", FailureMode: incidentdemo.FailureDeployment},
	}

	if err := os.MkdirAll(outputDirectory, 0755); err != nil {
		return fmt.Errorf("create fixture directory: %w", err)
	}

	for _, definition := range definitions {
		trace, err := incidentdemo.Run(definition.FailureMode)
		if err != nil {
			return fmt.Errorf("generate %s: %w", definition.Name, err)
		}

		data, err := json.MarshalIndent(trace, "", "  ")
		if err != nil {
			return fmt.Errorf("encode %s: %w", definition.Name, err)
		}
		data = append(data, '\n')

		path := filepath.Join(outputDirectory, definition.Name)
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", definition.Name, err)
		}
	}

	return nil
}
