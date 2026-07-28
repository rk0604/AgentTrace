package attrib

import (
	"encoding/json"
	"time"
)

// CurrentTraceVersion identifies the current external trace schema.
const CurrentTraceVersion = 1

const (
	// MaxTraceSteps bounds external trace graph size.
	MaxTraceSteps = 10000

	// MaxStepDependencies bounds incoming edges for one external step.
	MaxStepDependencies = 1000
)

type Step struct {
	RunID      string          `json:"run_id"`
	StepID     string          `json:"step_id"`
	AgentName  string          `json:"agent_name"`
	DependsOn  []string        `json:"depends_on"`
	Input      json.RawMessage `json:"input"`
	Output     json.RawMessage `json:"output"`
	ModelUsed  string          `json:"model_used"`
	Confidence *float64        `json:"confidence,omitempty"`
	Timestamp  time.Time       `json:"timestamp"`
	Status     string          `json:"status"`
}

type Trace struct {
	Version int    `json:"version,omitempty"`
	RunID   string `json:"run_id"`
	Steps   []Step `json:"steps"`
}
