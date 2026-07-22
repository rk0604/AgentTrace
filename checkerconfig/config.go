package checkerconfig

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// CurrentVersion identifies the supported checker configuration schema.
const CurrentVersion = 1

// Config contains versioned CEL checker definitions keyed by trace step ID.
type Config struct {
	Version int                   `json:"version"`
	Steps   map[string]StepConfig `json:"steps"`
}

// StepConfig defines one Boolean CEL expression and its failure reason.
type StepConfig struct {
	Expression    string `json:"expression"`
	FailureReason string `json:"failure_reason"`
}

// Decode reads and validates one checker configuration.
//
// Input
// reader io.Reader
// A stream containing one JSON checker configuration.
//
// Output
// Config
// The decoded and validated checker configuration.
//
// error
// Non nil when the JSON or configuration is invalid.
func Decode(reader io.Reader) (Config, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode checker configuration: %w", err)
	}

	// A valid configuration contains exactly one JSON value.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("decode checker configuration: multiple JSON values")
		}

		return Config{}, fmt.Errorf("decode checker configuration: %w", err)
	}

	if err := config.Validate(); err != nil {
		return Config{}, err
	}

	return config, nil
}

// Validate checks the checker configuration structure.
//
// Input
// config Config
// The checker configuration receiving the method call.
//
// Output
// error
// Non nil when the version, step IDs, expressions, or failure reasons are invalid.
func (config Config) Validate() error {
	if config.Version != CurrentVersion {
		return fmt.Errorf("unsupported checker configuration version %d", config.Version)
	}
	if len(config.Steps) == 0 {
		return fmt.Errorf("checker configuration has no steps")
	}

	for stepID, stepConfig := range config.Steps {
		if strings.TrimSpace(stepID) == "" {
			return fmt.Errorf("checker configuration has an empty step ID")
		}
		if strings.TrimSpace(stepConfig.Expression) == "" {
			return fmt.Errorf("checker for step %q has an empty expression", stepID)
		}
		if strings.TrimSpace(stepConfig.FailureReason) == "" {
			return fmt.Errorf("checker for step %q has an empty failure reason", stepID)
		}
	}

	return nil
}
