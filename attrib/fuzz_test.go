package attrib_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
)

func FuzzDecodeTrace(f *testing.F) {
	f.Add(`{
		"version":1,
		"run_id":"fuzz-run",
		"steps":[{
			"run_id":"fuzz-run",
			"step_id":"source",
			"agent_name":"Source",
			"depends_on":[],
			"input":{},
			"output":{},
			"model_used":"test",
			"timestamp":"2026-07-28T12:00:00Z",
			"status":"ok"
		}]
	}`)
	f.Add(`{"broken"`)
	f.Add(`null`)

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = attrib.DecodeTrace(strings.NewReader(input))
	})
}

func FuzzTopologicalSort(f *testing.F) {
	f.Add(uint8(4), uint64(0b000101))
	f.Add(uint8(1), uint64(0))
	f.Add(uint8(8), ^uint64(0))

	f.Fuzz(func(t *testing.T, rawCount uint8, edges uint64) {
		count := int(rawCount%16) + 1
		steps := make([]attrib.Step, count)
		bit := uint(0)
		for target := 0; target < count; target++ {
			steps[target].StepID = fmt.Sprintf("step-%d", target)
			for source := 0; source < target && bit < 64; source++ {
				if edges&(uint64(1)<<bit) != 0 {
					steps[target].DependsOn = append(
						steps[target].DependsOn,
						fmt.Sprintf("step-%d", source),
					)
				}
				bit++
			}
		}

		ordered, err := attrib.TopologicalSort(attrib.Trace{Steps: steps})
		if err != nil {
			t.Fatalf("generated acyclic graph returned error: %v", err)
		}
		if len(ordered) != count {
			t.Fatalf("expected %d ordered steps, got %d", count, len(ordered))
		}
	})
}
