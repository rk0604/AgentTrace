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
	if config.Version != checkerconfig.CurrentVersion {
		t.Fatalf("expected version %d, got %d", checkerconfig.CurrentVersion, config.Version)
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
			input:       `{"version":2,"steps":{"source":{"expression":"true","failure_reason":"failed"}}}`,
			wantMessage: "unsupported checker configuration version 2",
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
