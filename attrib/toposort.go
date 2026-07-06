package attrib

import "fmt"

func TopologicalSort(trace Trace) ([]Step, error) {
	stepsByID := make(map[string]Step, len(trace.Steps))
	positionByID := make(map[string]int, len(trace.Steps))
	dependentsByID := make(map[string][]string, len(trace.Steps))
	indegreeByID := make(map[string]int, len(trace.Steps))

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

	for _, step := range trace.Steps {
		for _, dependencyID := range step.DependsOn {
			if _, exists := stepsByID[dependencyID]; !exists {
				return nil, fmt.Errorf("step %q depends on unknown step %q", step.StepID, dependencyID)
			}

			dependentsByID[dependencyID] = append(dependentsByID[dependencyID], step.StepID)
			indegreeByID[step.StepID]++
		}
	}

	ready := make([]string, 0, len(trace.Steps))
	for _, step := range trace.Steps {
		if indegreeByID[step.StepID] == 0 {
			ready = append(ready, step.StepID)
		}
	}

	ordered := make([]Step, 0, len(trace.Steps))
	for len(ready) > 0 {
		nextID := popEarliestReady(ready, positionByID)
		ready = removeReady(ready, nextID)
		ordered = append(ordered, stepsByID[nextID])

		for _, dependentID := range dependentsByID[nextID] {
			indegreeByID[dependentID]--
			if indegreeByID[dependentID] == 0 {
				ready = append(ready, dependentID)
			}
		}
	}

	if len(ordered) != len(trace.Steps) {
		return nil, fmt.Errorf("trace contains a dependency cycle")
	}

	return ordered, nil
}

func popEarliestReady(ready []string, positionByID map[string]int) string {
	nextID := ready[0]
	for _, candidateID := range ready[1:] {
		if positionByID[candidateID] < positionByID[nextID] {
			nextID = candidateID
		}
	}

	return nextID
}

func removeReady(ready []string, stepID string) []string {
	for index, readyID := range ready {
		if readyID == stepID {
			return append(ready[:index], ready[index+1:]...)
		}
	}

	return ready
}
