package recorder

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/rk0604/AgentTrace/attrib"
)

// Options controls trace recording behavior.
type Options struct {
	Clock func() time.Time
}

// StepStart contains the data known when one pipeline step begins.
type StepStart struct {
	StepID    string
	AgentName string
	DependsOn []string
	Input     any
	ModelUsed string
}

// Recorder captures one pipeline run in memory.
type Recorder struct {
	mutex      sync.Mutex
	runID      string
	clock      func() time.Time
	nextOrder  int
	stepsByID  map[string]*recordedStep
	stepOrders []string
}

type recordedStep struct {
	order    int
	finished bool
	step     attrib.Step
}

// StartRun creates a recorder for one pipeline run.
//
// Input
// runID string
// Stable identifier shared by every recorded step.
//
// options Options
// Optional clock used to timestamp steps.
//
// Output
// pointer to Recorder
// Concurrency safe recorder for the run.
//
// error
// Non nil when the run ID is empty.
func StartRun(runID string, options Options) (*Recorder, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("start run: run ID is empty")
	}

	clock := options.Clock
	if clock == nil {
		clock = func() time.Time {
			return time.Now().UTC()
		}
	}

	return &Recorder{
		runID:      runID,
		clock:      clock,
		stepsByID:  make(map[string]*recordedStep),
		stepOrders: make([]string, 0),
	}, nil
}

// StartStep records the immutable data for one pipeline step.
//
// Input
// start StepStart
// Step identity, dependencies, input payload, and model name.
//
// Output
// error
// Non nil when the step data is invalid or the step ID already exists.
func (recorder *Recorder) StartStep(start StepStart) error {
	if strings.TrimSpace(start.StepID) == "" {
		return fmt.Errorf("start step: step ID is empty")
	}
	if strings.TrimSpace(start.AgentName) == "" {
		return fmt.Errorf("start step %q: agent name is empty", start.StepID)
	}

	input, err := marshalPayload(start.Input, "input")
	if err != nil {
		return fmt.Errorf("start step %q: %w", start.StepID, err)
	}

	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()

	if _, exists := recorder.stepsByID[start.StepID]; exists {
		return fmt.Errorf("start step %q: duplicate step ID", start.StepID)
	}

	dependencies := append([]string(nil), start.DependsOn...)
	recorder.stepsByID[start.StepID] = &recordedStep{
		order: recorder.nextOrder,
		step: attrib.Step{
			RunID:     recorder.runID,
			StepID:    start.StepID,
			AgentName: start.AgentName,
			DependsOn: dependencies,
			Input:     input,
			Output:    json.RawMessage("null"),
			ModelUsed: start.ModelUsed,
			Timestamp: recorder.clock().UTC(),
			Status:    "running",
		},
	}
	recorder.stepOrders = append(recorder.stepOrders, start.StepID)
	recorder.nextOrder++

	return nil
}

// FinishStep records a successful output for one started step.
//
// Input
// stepID string
// Identifier of the started step.
//
// output any
// Domain output encoded as JSON.
//
// confidence pointer to float64
// Optional confidence value from zero through one.
//
// Output
// error
// Non nil when the output is invalid or the step cannot be completed.
func (recorder *Recorder) FinishStep(stepID string, output any, confidence *float64) error {
	return recorder.completeStep(stepID, output, confidence, "ok")
}

// FailStep records an unsuccessful output for one started step.
//
// Input
// stepID string
// Identifier of the started step.
//
// output any
// Domain output or structured failure details encoded as JSON.
//
// status string
// Non empty failure status recorded on the step.
//
// Output
// error
// Non nil when the output or status is invalid or the step cannot be completed.
func (recorder *Recorder) FailStep(stepID string, output any, status string) error {
	if strings.TrimSpace(status) == "" || status == "ok" || status == "running" {
		return fmt.Errorf("fail step %q: status must describe a failure", stepID)
	}

	return recorder.completeStep(stepID, output, nil, status)
}

// Trace returns a validated snapshot of the completed run.
//
// Input
// recorder pointer to Recorder
// Recorder receiving the method call.
//
// Output
// attrib.Trace
// Versioned trace ordered by step start order.
//
// error
// Non nil when a step remains unfinished or the final trace is invalid.
func (recorder *Recorder) Trace() (attrib.Trace, error) {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()

	steps := make([]attrib.Step, 0, len(recorder.stepOrders))
	for _, stepID := range recorder.stepOrders {
		recorded := recorder.stepsByID[stepID]
		if !recorded.finished {
			return attrib.Trace{}, fmt.Errorf("build trace: step %q is unfinished", stepID)
		}

		steps = append(steps, cloneStep(recorded.step))
	}

	trace := attrib.Trace{
		Version: attrib.CurrentTraceVersion,
		RunID:   recorder.runID,
		Steps:   steps,
	}
	if err := attrib.ValidateTraceSchema(trace); err != nil {
		return attrib.Trace{}, fmt.Errorf("build trace: %w", err)
	}

	return trace, nil
}

// WriteTrace writes the completed run as versioned JSON.
//
// Input
// writer io.Writer
// Destination for the trace JSON.
//
// Output
// error
// Non nil when the trace is incomplete, invalid, or cannot be written.
func (recorder *Recorder) WriteTrace(writer io.Writer) error {
	trace, err := recorder.Trace()
	if err != nil {
		return err
	}

	return attrib.EncodeTrace(writer, trace)
}

// completeStep stores the final state for one recorded step.
//
// Input
// stepID string
// Identifier of the started step.
//
// output any
// JSON serializable final output.
//
// confidence pointer to float64
// Optional confidence from zero through one.
//
// status string
// Final step status.
//
// Output
// error
// Non nil when output or lifecycle state is invalid.
func (recorder *Recorder) completeStep(stepID string, output any, confidence *float64, status string) error {
	encodedOutput, err := marshalPayload(output, "output")
	if err != nil {
		return fmt.Errorf("complete step %q: %w", stepID, err)
	}
	if confidence != nil &&
		(math.IsNaN(*confidence) ||
			math.IsInf(*confidence, 0) ||
			*confidence < 0 ||
			*confidence > 1) {
		return fmt.Errorf("complete step %q: confidence must be between zero and one", stepID)
	}

	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()

	recorded, exists := recorder.stepsByID[stepID]
	if !exists {
		return fmt.Errorf("complete step %q: step was not started", stepID)
	}
	if recorded.finished {
		return fmt.Errorf("complete step %q: step is already finished", stepID)
	}

	recorded.step.Output = encodedOutput
	recorded.step.Confidence = cloneConfidence(confidence)
	recorded.step.Status = status
	recorded.finished = true

	return nil
}

// marshalPayload encodes one domain value as generic JSON.
//
// Input
// value any
// Domain value to encode.
//
// fieldName string
// Field name used in errors.
//
// Output
// json.RawMessage
// Encoded JSON value.
//
// error
// Non nil when the value cannot be encoded.
func marshalPayload(value any, fieldName string) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode %s JSON: %w", fieldName, err)
	}

	return json.RawMessage(encoded), nil
}

// cloneStep creates an isolated copy of one recorded step.
//
// Input
// step attrib.Step
// Step to copy.
//
// Output
// attrib.Step
// Step with copied slices and payloads.
func cloneStep(step attrib.Step) attrib.Step {
	cloned := step
	cloned.DependsOn = append([]string(nil), step.DependsOn...)
	cloned.Input = append(json.RawMessage(nil), step.Input...)
	cloned.Output = append(json.RawMessage(nil), step.Output...)
	cloned.Confidence = cloneConfidence(step.Confidence)

	return cloned
}

// cloneConfidence copies an optional confidence value.
//
// Input
// confidence pointer to float64
// Optional value to copy.
//
// Output
// pointer to float64
// Detached value or nil.
func cloneConfidence(confidence *float64) *float64 {
	if confidence == nil {
		return nil
	}

	cloned := *confidence
	return &cloned
}
