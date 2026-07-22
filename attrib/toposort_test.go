package attrib_test

import (
	"strings"
	"testing"

	"github.com/rk0604/AgentTrace/attrib"
)

func TestTopologicalSortOrdersDependenciesBeforeConsumers(t *testing.T) {
	trace := attrib.Trace{
		RunID: "run-1",
		Steps: []attrib.Step{
			{StepID: "merge", DependsOn: []string{"left", "right"}},
			{StepID: "right"},
			{StepID: "left"},
			{StepID: "final", DependsOn: []string{"merge"}},
		},
	}

	ordered, err := attrib.TopologicalSort(trace)
	if err != nil {
		t.Fatalf("TopologicalSort returned error: %v", err)
	}

	assertStepOrder(t, ordered, []string{"right", "left", "merge", "final"})
}

func TestTopologicalSortRejectsInvalidGraphs(t *testing.T) {
	tests := []struct {
		name        string
		trace       attrib.Trace
		wantMessage string
	}{
		{
			name: "empty step ID",
			trace: attrib.Trace{
				Steps: []attrib.Step{{StepID: ""}},
			},
			wantMessage: "empty step_id",
		},
		{
			name: "duplicate step ID",
			trace: attrib.Trace{
				Steps: []attrib.Step{{StepID: "same"}, {StepID: "same"}},
			},
			wantMessage: "duplicate step_id",
		},
		{
			name: "unknown dependency",
			trace: attrib.Trace{
				Steps: []attrib.Step{{StepID: "consumer", DependsOn: []string{"missing"}}},
			},
			wantMessage: "depends on unknown step",
		},
		{
			name: "dependency cycle",
			trace: attrib.Trace{
				Steps: []attrib.Step{
					{StepID: "first", DependsOn: []string{"second"}},
					{StepID: "second", DependsOn: []string{"first"}},
				},
			},
			wantMessage: "dependency cycle",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := attrib.TopologicalSort(test.trace)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("expected error containing %q, got %q", test.wantMessage, err)
			}
		})
	}
}

func assertStepOrder(t *testing.T, steps []attrib.Step, want []string) {
	t.Helper()

	if len(steps) != len(want) {
		t.Fatalf("expected %d steps, got %d", len(want), len(steps))
	}

	for index, step := range steps {
		if step.StepID != want[index] {
			t.Fatalf("expected step order %v, got %v", want, stepIDs(steps))
		}
	}
}

func stepIDs(steps []attrib.Step) []string {
	ids := make([]string, 0, len(steps))
	for _, step := range steps {
		ids = append(ids, step.StepID)
	}

	return ids
}
