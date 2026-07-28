package attrib

import (
	"container/heap"
	"fmt"
)

type readyStepHeap struct {
	stepIDs      []string
	positionByID map[string]int
}

// Len returns the number of ready step IDs.
//
// Input
// ready readyStepHeap
// Heap receiving the method call.
//
// Output
// int
// Number of queued step IDs.
func (ready readyStepHeap) Len() int {
	return len(ready.stepIDs)
}

// Less reports which ready step appeared first in the trace.
//
// Input
// left int
// Index of the first heap value.
//
// right int
// Index of the second heap value.
//
// Output
// bool
// True when the left step has the earlier original position.
func (ready readyStepHeap) Less(left int, right int) bool {
	return ready.positionByID[ready.stepIDs[left]] < ready.positionByID[ready.stepIDs[right]]
}

// Swap exchanges two ready step IDs.
//
// Input
// left int
// Index of the first heap value.
//
// right int
// Index of the second heap value.
//
// Output
// None
func (ready readyStepHeap) Swap(left int, right int) {
	ready.stepIDs[left], ready.stepIDs[right] = ready.stepIDs[right], ready.stepIDs[left]
}

// Push appends one step ID to the heap storage.
//
// Input
// value any
// String step ID supplied by container heap.
//
// Output
// None
func (ready *readyStepHeap) Push(value any) {
	ready.stepIDs = append(ready.stepIDs, value.(string))
}

// Pop removes the final step ID from the heap storage.
//
// Input
// ready pointer to readyStepHeap
// Heap receiving the method call.
//
// Output
// any
// Removed string step ID.
func (ready *readyStepHeap) Pop() any {
	lastIndex := len(ready.stepIDs) - 1
	value := ready.stepIDs[lastIndex]
	ready.stepIDs = ready.stepIDs[:lastIndex]
	return value
}

// TopologicalSort returns the trace steps in dependency order.
//
// Input:
// trace Trace
// A trace containing Step values. Each Step may list dependency step IDs in DependsOn.
//
// Output:
// []Step
// The same steps ordered so each dependency appears before the step that consumes it.
//
// error
// Non nil when the trace has invalid graph structure, such as an empty step ID,
// duplicate step ID, unknown dependency, or dependency cycle.
func TopologicalSort(trace Trace) ([]Step, error) {
	// stepsByID stores each step by its unique step ID.
	// This allows dependency IDs to be resolved into actual steps.
	stepsByID := make(map[string]Step, len(trace.Steps))

	// positionByID stores the original position of each step in the trace.
	// This keeps ordering deterministic when multiple steps are ready at once.
	positionByID := make(map[string]int, len(trace.Steps))

	// dependentsByID maps a step ID to the steps that depend on it.
	// Example: extractor maps to comparator.
	dependentsByID := make(map[string][]string, len(trace.Steps))

	// indegreeByID stores how many unresolved dependencies each step has.
	// A step with indegree 0 is ready to run.
	indegreeByID := make(map[string]int, len(trace.Steps))

	// First pass validates step IDs and initializes lookup maps.
	for position, step := range trace.Steps {
		if step.StepID == "" {
			return nil, fmt.Errorf("step at position %d has empty step_id", position)
		}

		if _, exists := stepsByID[step.StepID]; exists {
			return nil, fmt.Errorf("duplicate step_id %q", step.StepID)
		}

		stepsByID[step.StepID] = step
		positionByID[step.StepID] = position
		indegreeByID[step.StepID] = 0
	}

	// Second pass records graph edges and dependency counts.
	for _, step := range trace.Steps {
		seenDependencies := make(map[string]bool, len(step.DependsOn))
		for _, dependencyID := range step.DependsOn {
			if _, exists := stepsByID[dependencyID]; !exists {
				return nil, fmt.Errorf("step %q depends on unknown step %q", step.StepID, dependencyID)
			}
			if seenDependencies[dependencyID] {
				return nil, fmt.Errorf("step %q repeats dependency %q", step.StepID, dependencyID)
			}
			seenDependencies[dependencyID] = true

			// dependencyID must run before step.StepID.
			dependentsByID[dependencyID] = append(dependentsByID[dependencyID], step.StepID)

			// step.StepID has one more dependency that must be resolved.
			indegreeByID[step.StepID]++
		}
	}

	// ready contains steps that have no unresolved dependencies.
	ready := &readyStepHeap{
		stepIDs:      make([]string, 0, len(trace.Steps)),
		positionByID: positionByID,
	}
	for _, step := range trace.Steps {
		if indegreeByID[step.StepID] == 0 {
			heap.Push(ready, step.StepID)
		}
	}

	// ordered accumulates the final sorted step order.
	ordered := make([]Step, 0, len(trace.Steps))

	// Process ready steps until no ready steps remain.
	for ready.Len() > 0 {
		// Pick the ready step that appeared earliest in the original trace.
		nextID := heap.Pop(ready).(string)

		// Add the selected step to the sorted output.
		ordered = append(ordered, stepsByID[nextID])

		// Each dependent has one fewer unresolved dependency now.
		for _, dependentID := range dependentsByID[nextID] {
			indegreeByID[dependentID]--

			// Once a dependent has no unresolved dependencies, it is ready.
			if indegreeByID[dependentID] == 0 {
				heap.Push(ready, dependentID)
			}
		}
	}

	// If not every step was ordered, some steps were stuck behind a cycle.
	if len(ordered) != len(trace.Steps) {
		return nil, fmt.Errorf("trace contains a dependency cycle")
	}

	return ordered, nil
}
