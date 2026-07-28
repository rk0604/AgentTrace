package checkerconfig_test

import (
	"strings"
	"testing"

	"github.com/rk0604/AgentTrace/checkerconfig"
)

func TestDecodeReturnsValidConfiguration(t *testing.T) {
	input := `{
		"version": 1,
		"steps": {
			"source": {
				"expression": "output.value == 'correct'",
				"failure_reason": "Source returned the wrong value"
			}
		}
	}`

	config, err := checkerconfig.Decode(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if config.Version != checkerconfig.LegacyVersion {
		t.Fatalf("expected version %d, got %d", checkerconfig.LegacyVersion, config.Version)
	}
	if config.Steps["source"].Expression != "output.value == 'correct'" {
		t.Fatalf("unexpected expression %q", config.Steps["source"].Expression)
	}
}

func TestDecodeRejectsInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMessage string
	}{
		{
			name:        "unknown field",
			input:       `{"version":1,"steps":{"source":{"expression":"true","failure_reason":"failed","mode":"unknown"}}}`,
			wantMessage: `unknown field "mode"`,
		},
		{
			name:        "unsupported version",
			input:       `{"version":3,"steps":{"source":{"expression":"true","failure_reason":"failed"}}}`,
			wantMessage: "unsupported checker configuration version 3",
		},
		{
			name:        "no steps",
			input:       `{"version":1,"steps":{}}`,
			wantMessage: "checker configuration has no steps",
		},
		{
			name:        "empty expression",
			input:       `{"version":1,"steps":{"source":{"expression":" ","failure_reason":"failed"}}}`,
			wantMessage: "empty expression",
		},
		{
			name:        "empty failure reason",
			input:       `{"version":1,"steps":{"source":{"expression":"true","failure_reason":" "}}}`,
			wantMessage: "empty failure reason",
		},
		{
			name:        "multiple JSON values",
			input:       `{"version":1,"steps":{"source":{"expression":"true","failure_reason":"failed"}}} {}`,
			wantMessage: "multiple JSON values",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := checkerconfig.Decode(strings.NewReader(test.input))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("expected error containing %q, got %q", test.wantMessage, err)
			}
		})
	}
}

func TestDecodeReturnsVersionTwoChecks(t *testing.T) {
	input := `{
		"version": 2,
		"steps": {
			"source": {
				"checks": [
					{
						"expression": "hasField(output, 'value')",
						"failure_reason": "Source output is missing value"
					},
					{
						"expression": "output.value == expected.value",
						"failure_reason": "Source returned the wrong value"
					}
				]
			}
		}
	}`

	config, err := checkerconfig.Decode(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	checks, err := config.ChecksForStep("source")
	if err != nil {
		t.Fatalf("ChecksForStep returned error: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("expected two checks, got %d", len(checks))
	}
	if checks[1].FailureReason != "Source returned the wrong value" {
		t.Fatalf("unexpected failure reason %q", checks[1].FailureReason)
	}
}

func TestVersionTwoRejectsConflictingCheckForms(t *testing.T) {
	input := `{
		"version": 2,
		"steps": {
			"source": {
				"expression": "true",
				"failure_reason": "single",
				"checks": [{"expression":"true","failure_reason":"list"}]
			}
		}
	}`

	_, err := checkerconfig.Decode(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "cannot combine") {
		t.Fatalf("unexpected error %q", err)
	}
}
