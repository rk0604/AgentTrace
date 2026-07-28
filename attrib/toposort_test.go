package attrib_test

import (
	"fmt"
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
			name: "repeated dependency",
			trace: attrib.Trace{
				Steps: []attrib.Step{
					{StepID: "source"},
					{StepID: "consumer", DependsOn: []string{"source", "source"}},
				},
			},
			wantMessage: "repeats dependency",
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

func TestTopologicalSortHandlesWideGraphDeterministically(t *testing.T) {
	const sourceCount = 5000

	steps := make([]attrib.Step, 0, sourceCount+1)
	dependencies := make([]string, 0, sourceCount)
	for index := 0; index < sourceCount; index++ {
		stepID := fmt.Sprintf("source-%04d", index)
		steps = append(steps, attrib.Step{StepID: stepID})
		dependencies = append(dependencies, stepID)
	}
	steps = append(steps, attrib.Step{
		StepID:    "final",
		DependsOn: dependencies,
	})

	ordered, err := attrib.TopologicalSort(attrib.Trace{Steps: steps})
	if err != nil {
		t.Fatalf("TopologicalSort returned error: %v", err)
	}
	if len(ordered) != sourceCount+1 {
		t.Fatalf("expected %d steps, got %d", sourceCount+1, len(ordered))
	}
	for index := 0; index < sourceCount; index++ {
		want := fmt.Sprintf("source-%04d", index)
		if ordered[index].StepID != want {
			t.Fatalf("expected %q at position %d, got %q", want, index, ordered[index].StepID)
		}
	}
	if ordered[sourceCount].StepID != "final" {
		t.Fatalf("expected final step last, got %q", ordered[sourceCount].StepID)
	}
}

func BenchmarkTopologicalSortWideGraph(b *testing.B) {
	const sourceCount = 5000

	steps := make([]attrib.Step, 0, sourceCount+1)
	dependencies := make([]string, 0, sourceCount)
	for index := 0; index < sourceCount; index++ {
		stepID := fmt.Sprintf("source-%04d", index)
		steps = append(steps, attrib.Step{StepID: stepID})
		dependencies = append(dependencies, stepID)
	}
	steps = append(steps, attrib.Step{StepID: "final", DependsOn: dependencies})
	trace := attrib.Trace{Steps: steps}

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := attrib.TopologicalSort(trace); err != nil {
			b.Fatalf("TopologicalSort returned error: %v", err)
		}
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
