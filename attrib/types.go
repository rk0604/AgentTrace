package attrib

import (
	"encoding/json"
	"time"
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
	RunID string `json:"run_id"`
	Steps []Step `json:"steps"`
}
