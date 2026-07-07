package attrib

import "fmt"

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
		for _, dependencyID := range step.DependsOn {
			if _, exists := stepsByID[dependencyID]; !exists {
				return nil, fmt.Errorf("step %q depends on unknown step %q", step.StepID, dependencyID)
			}

			// dependencyID must run before step.StepID.
			dependentsByID[dependencyID] = append(dependentsByID[dependencyID], step.StepID)

			// step.StepID has one more dependency that must be resolved.
			indegreeByID[step.StepID]++
		}
	}

	// ready contains steps that have no unresolved dependencies.
	ready := make([]string, 0, len(trace.Steps))
	for _, step := range trace.Steps {
		if indegreeByID[step.StepID] == 0 {
			ready = append(ready, step.StepID)
		}
	}

	// ordered accumulates the final sorted step order.
	ordered := make([]Step, 0, len(trace.Steps))

	// Process ready steps until no ready steps remain.
	for len(ready) > 0 {
		// Pick the ready step that appeared earliest in the original trace.
		nextID := popEarliestReady(ready, positionByID)

		// Remove the selected step from the ready queue.
		ready = removeReady(ready, nextID)

		// Add the selected step to the sorted output.
		ordered = append(ordered, stepsByID[nextID])

		// Each dependent has one fewer unresolved dependency now.
		for _, dependentID := range dependentsByID[nextID] {
			indegreeByID[dependentID]--

			// Once a dependent has no unresolved dependencies, it is ready.
			if indegreeByID[dependentID] == 0 {
				ready = append(ready, dependentID)
			}
		}
	}

	// If not every step was ordered, some steps were stuck behind a cycle.
	if len(ordered) != len(trace.Steps) {
		return nil, fmt.Errorf("trace contains a dependency cycle")
	}

	return ordered, nil
}

// popEarliestReady selects the ready step that appeared earliest in the original trace.
//
// Input:
// ready []string
// Step IDs that currently have no unresolved dependencies.
//
// positionByID map[string]int
// Map from step ID to original trace position.
//
// Output:
// string
// The ready step ID with the smallest original position.
func popEarliestReady(ready []string, positionByID map[string]int) string {
	nextID := ready[0]

	for _, candidateID := range ready[1:] {
		if positionByID[candidateID] < positionByID[nextID] {
			nextID = candidateID
		}
	}

	return nextID
}

// removeReady removes one step ID from the ready list.
//
// Input:
// ready []string
// Current list of ready step IDs.
//
// stepID string
// Step ID to remove.
//
// Output:
// []string
// Ready list without the selected step ID.
func removeReady(ready []string, stepID string) []string {
	for index, readyID := range ready {
		if readyID == stepID {
			return append(ready[:index], ready[index+1:]...)
		}
	}

	return ready
}
