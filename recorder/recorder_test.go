package recorder_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/recorder"
)

func TestRecorderBuildsVersionedTrace(t *testing.T) {
	clock := sequenceClock(
		time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 28, 12, 0, 1, 0, time.UTC),
	)
	run, err := recorder.StartRun("recorded-run", recorder.Options{Clock: clock})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	if err := run.StartStep(recorder.StepStart{
		StepID:    "source",
		AgentName: "Source",
		Input:     map[string]any{"document": "input"},
		ModelUsed: "test-model",
	}); err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	confidence := 0.95
	if err := run.FinishStep("source", map[string]any{"value": "fact"}, &confidence); err != nil {
		t.Fatalf("FinishStep returned error: %v", err)
	}

	if err := run.StartStep(recorder.StepStart{
		StepID:    "consumer",
		AgentName: "Consumer",
		DependsOn: []string{"source"},
		Input:     map[string]any{"value": "fact"},
		ModelUsed: "test-model",
	}); err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := run.FinishStep("consumer", map[string]any{"answer": "done"}, nil); err != nil {
		t.Fatalf("FinishStep returned error: %v", err)
	}

	trace, err := run.Trace()
	if err != nil {
		t.Fatalf("Trace returned error: %v", err)
	}
	if trace.Version != attrib.CurrentTraceVersion {
		t.Fatalf("expected trace version %d, got %d", attrib.CurrentTraceVersion, trace.Version)
	}
	if len(trace.Steps) != 2 {
		t.Fatalf("expected two steps, got %d", len(trace.Steps))
	}
	if trace.Steps[1].DependsOn[0] != "source" {
		t.Fatalf("expected consumer dependency source, got %v", trace.Steps[1].DependsOn)
	}
	if trace.Steps[0].Confidence == nil || *trace.Steps[0].Confidence != confidence {
		t.Fatalf("expected confidence %v, got %v", confidence, trace.Steps[0].Confidence)
	}
}

func TestRecorderSupportsParallelSteps(t *testing.T) {
	run, err := recorder.StartRun("parallel-run", recorder.Options{
		Clock: func() time.Time {
			return time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	stepIDs := []string{"left", "right"}
	var wait sync.WaitGroup
	for _, stepID := range stepIDs {
		wait.Add(1)
		go func(id string) {
			defer wait.Done()

			if startErr := run.StartStep(recorder.StepStart{
				StepID:    id,
				AgentName: strings.ToUpper(id),
				Input:     map[string]string{"branch": id},
			}); startErr != nil {
				t.Errorf("StartStep returned error: %v", startErr)
				return
			}
			if finishErr := run.FinishStep(id, map[string]string{"result": id}, nil); finishErr != nil {
				t.Errorf("FinishStep returned error: %v", finishErr)
			}
		}(stepID)
	}
	wait.Wait()

	trace, err := run.Trace()
	if err != nil {
		t.Fatalf("Trace returned error: %v", err)
	}
	if len(trace.Steps) != 2 {
		t.Fatalf("expected two recorded steps, got %d", len(trace.Steps))
	}
}

func TestRecorderRejectsInvalidLifecycle(t *testing.T) {
	run, err := recorder.StartRun("invalid-run", recorder.Options{
		Clock: func() time.Time {
			return time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	if err := run.StartStep(recorder.StepStart{StepID: "source", AgentName: "Source", Input: nil}); err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}

	assertErrorContains(t, run.StartStep(recorder.StepStart{StepID: "source", AgentName: "Source", Input: nil}), "duplicate step ID")
	assertErrorContains(t, run.FinishStep("missing", nil, nil), "step was not started")
	if _, err := run.Trace(); err == nil || !strings.Contains(err.Error(), "unfinished") {
		t.Fatalf("expected unfinished step error, got %v", err)
	}

	if err := run.FailStep("source", map[string]string{"error": "timeout"}, "error"); err != nil {
		t.Fatalf("FailStep returned error: %v", err)
	}
	assertErrorContains(t, run.FinishStep("source", nil, nil), "already finished")
}

func TestRecorderWritesTraceAcceptedByDecoder(t *testing.T) {
	run, err := recorder.StartRun("json-run", recorder.Options{
		Clock: func() time.Time {
			return time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if err := run.StartStep(recorder.StepStart{
		StepID:    "source",
		AgentName: "Source",
		Input:     map[string]string{"text": "hello"},
	}); err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := run.FinishStep("source", map[string]string{"text": "world"}, nil); err != nil {
		t.Fatalf("FinishStep returned error: %v", err)
	}

	var output bytes.Buffer
	if err := run.WriteTrace(&output); err != nil {
		t.Fatalf("WriteTrace returned error: %v", err)
	}
	trace, err := attrib.DecodeTrace(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatalf("DecodeTrace returned error: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(trace.Steps[0].Output, &payload); err != nil {
		t.Fatalf("decode output payload: %v", err)
	}
	if payload["text"] != "world" {
		t.Fatalf("expected world output, got %q", payload["text"])
	}
}

func sequenceClock(times ...time.Time) func() time.Time {
	var mutex sync.Mutex
	index := 0

	return func() time.Time {
		mutex.Lock()
		defer mutex.Unlock()

		value := times[index]
		index++
		return value
	}
}

func assertErrorContains(t *testing.T, err error, expected string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected error containing %q, got %q", expected, err)
	}
}
