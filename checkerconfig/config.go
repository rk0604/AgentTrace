package checkerconfig

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	// LegacyVersion identifies the original single expression schema.
	LegacyVersion = 1

	// CurrentVersion identifies the current checker configuration schema.
	CurrentVersion = 2
)

// Config contains versioned CEL checker definitions keyed by trace step ID.
type Config struct {
	Version int                   `json:"version"`
	Steps   map[string]StepConfig `json:"steps"`
}

// StepConfig defines one Boolean CEL expression and its failure reason.
type StepConfig struct {
	Expression    string        `json:"expression,omitempty"`
	FailureReason string        `json:"failure_reason,omitempty"`
	Checks        []CheckConfig `json:"checks,omitempty"`
}

// CheckConfig defines one ordered Boolean assertion.
type CheckConfig struct {
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
	if config.Version != LegacyVersion && config.Version != CurrentVersion {
		return fmt.Errorf("unsupported checker configuration version %d", config.Version)
	}
	if len(config.Steps) == 0 {
		return fmt.Errorf("checker configuration has no steps")
	}

	for stepID, stepConfig := range config.Steps {
		if strings.TrimSpace(stepID) == "" {
			return fmt.Errorf("checker configuration has an empty step ID")
		}

		checks, err := stepConfig.checks(config.Version, stepID)
		if err != nil {
			return err
		}
		for checkIndex, check := range checks {
			if strings.TrimSpace(check.Expression) == "" {
				return fmt.Errorf("checker for step %q check %d has an empty expression", stepID, checkIndex)
			}
			if strings.TrimSpace(check.FailureReason) == "" {
				return fmt.Errorf("checker for step %q check %d has an empty failure reason", stepID, checkIndex)
			}
		}
	}

	return nil
}

// checks returns the ordered assertions for one step.
//
// Input
// version int
// Checker configuration schema version.
//
// stepID string
// Step identifier used in validation errors.
//
// Output
// slice of CheckConfig
// Ordered assertions represented by the step configuration.
//
// error
// Non nil when fields conflict with the selected schema version.
func (config StepConfig) checks(version int, stepID string) ([]CheckConfig, error) {
	hasSingleCheck := strings.TrimSpace(config.Expression) != "" || strings.TrimSpace(config.FailureReason) != ""
	hasCheckList := len(config.Checks) > 0

	if version == LegacyVersion {
		if hasCheckList {
			return nil, fmt.Errorf("checker for step %q uses checks with legacy version %d", stepID, LegacyVersion)
		}

		return []CheckConfig{{
			Expression:    config.Expression,
			FailureReason: config.FailureReason,
		}}, nil
	}

	if hasSingleCheck && hasCheckList {
		return nil, fmt.Errorf("checker for step %q cannot combine expression fields with checks", stepID)
	}
	if hasCheckList {
		return append([]CheckConfig(nil), config.Checks...), nil
	}
	if hasSingleCheck {
		return []CheckConfig{{
			Expression:    config.Expression,
			FailureReason: config.FailureReason,
		}}, nil
	}

	return nil, fmt.Errorf("checker for step %q has no checks", stepID)
}

// ChecksForStep returns the normalized assertions for one configured step.
//
// Input
// config Config
// Valid checker configuration.
//
// stepID string
// Step identifier to resolve.
//
// Output
// slice of CheckConfig
// Ordered assertions for the step.
//
// error
// Non nil when the step is missing or invalid.
func (config Config) ChecksForStep(stepID string) ([]CheckConfig, error) {
	stepConfig, exists := config.Steps[stepID]
	if !exists {
		return nil, fmt.Errorf("checker configuration has no step %q", stepID)
	}

	return stepConfig.checks(config.Version, stepID)
}
