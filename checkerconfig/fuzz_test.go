package checkerconfig_test

import (
	"strings"
	"testing"

	"github.com/rk0604/AgentTrace/checkerconfig"
)

func FuzzDecodeCheckerConfiguration(f *testing.F) {
	f.Add(`{
		"version":2,
		"steps":{
			"source":{
				"checks":[{
					"expression":"true",
					"failure_reason":"failed"
				}]
			}
		}
	}`)
	f.Add(`{"version":1,"steps":{}}`)
	f.Add(`{"broken"`)

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = checkerconfig.Decode(strings.NewReader(input))
	})
}
