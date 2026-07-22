package checkerconfig

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/rk0604/AgentTrace/attrib"
)

// evaluationCostLimit bounds the work performed by one CEL expression.
const evaluationCostLimit = 100000

// Build compiles configured CEL expressions into AgentTrace step checkers.
//
// Input
// config Config
// A validated checker configuration containing one CEL expression per step.
//
// Output
// map[string]attrib.StepChecker
// Compiled checkers keyed by step ID.
//
// error
// Non nil when the configuration or any CEL expression is invalid.
func Build(config Config) (map[string]attrib.StepChecker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Dynamic variables allow expressions to inspect arbitrary JSON payloads.
	environment, err := cel.NewEnv(
		cel.Variable("input", cel.DynType),
		cel.Variable("output", cel.DynType),
		cel.Variable("step", cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("create CEL environment: %w", err)
	}

	stepIDs := make([]string, 0, len(config.Steps))
	for stepID := range config.Steps {
		stepIDs = append(stepIDs, stepID)
	}
	sort.Strings(stepIDs)

	checkers := make(map[string]attrib.StepChecker, len(stepIDs))
	for _, stepID := range stepIDs {
		stepConfig := config.Steps[stepID]
		program, err := compileProgram(environment, stepID, stepConfig.Expression)
		if err != nil {
			return nil, err
		}

		checkers[stepID] = configuredChecker(program, stepConfig.FailureReason)
	}

	return checkers, nil
}

// compileProgram compiles one Boolean CEL expression.
//
// Input
// environment *cel.Env
// CEL environment declaring the variables available to expressions.
//
// stepID string
// Step ID used to identify configuration errors.
//
// expression string
// CEL expression to compile.
//
// Output
// cel.Program
// A reusable and thread safe compiled CEL program.
//
// error
// Non nil when parsing, type checking, or program construction fails.
func compileProgram(environment *cel.Env, stepID string, expression string) (cel.Program, error) {
	ast, issues := environment.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile checker for step %q: %w", stepID, issues.Err())
	}
	if !cel.BoolType.IsAssignableType(ast.OutputType()) {
		return nil, fmt.Errorf("compile checker for step %q: expression must return bool, got %s", stepID, ast.OutputType())
	}

	program, err := environment.Program(ast, cel.CostLimit(evaluationCostLimit))
	if err != nil {
		return nil, fmt.Errorf("build checker for step %q: %w", stepID, err)
	}

	return program, nil
}

// configuredChecker creates a StepChecker backed by a compiled CEL program.
//
// Input
// program cel.Program
// Compiled CEL program that evaluates one step.
//
// failureReason string
// Reason returned when the expression evaluates to false.
//
// Output
// attrib.StepChecker
// Checker that evaluates generic step JSON through CEL.
func configuredChecker(program cel.Program, failureReason string) attrib.StepChecker {
	return func(step attrib.Step) (attrib.CheckResult, error) {
		activation, err := stepActivation(step)
		if err != nil {
			return attrib.CheckResult{}, err
		}

		value, _, err := program.Eval(activation)
		if err != nil {
			return attrib.CheckResult{}, fmt.Errorf("evaluate CEL expression: %w", err)
		}

		passed, ok := value.Value().(bool)
		if !ok {
			return attrib.CheckResult{}, fmt.Errorf("evaluate CEL expression: expected bool result, got %T", value.Value())
		}
		if !passed {
			return attrib.Fail(failureReason), nil
		}

		return attrib.Pass(), nil
	}
}

// stepActivation converts a generic trace step into CEL variables.
//
// Input
// step attrib.Step
// Trace step being checked.
//
// Output
// map[string]any
// CEL activation containing input, output, and generic step metadata.
//
// error
// Non nil when an Input or Output payload is not valid JSON.
func stepActivation(step attrib.Step) (map[string]any, error) {
	input, err := decodePayload(step.Input, "input")
	if err != nil {
		return nil, err
	}

	output, err := decodePayload(step.Output, "output")
	if err != nil {
		return nil, err
	}

	metadata := map[string]any{
		"run_id":     step.RunID,
		"step_id":    step.StepID,
		"agent_name": step.AgentName,
		"depends_on": step.DependsOn,
		"model_used": step.ModelUsed,
		"confidence": confidenceValue(step.Confidence),
		"timestamp":  step.Timestamp.Format(time.RFC3339Nano),
		"status":     step.Status,
	}

	return map[string]any{
		"input":  input,
		"output": output,
		"step":   metadata,
	}, nil
}

// decodePayload converts raw JSON into CEL compatible native values.
//
// Input
// payload json.RawMessage
// Raw JSON payload from a trace step.
//
// fieldName string
// Field name used in an error message.
//
// Output
// any
// Native map, list, scalar, or nil value for CEL evaluation.
//
// error
// Non nil when the payload is invalid JSON.
func decodePayload(payload json.RawMessage, fieldName string) (any, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, fmt.Errorf("decode step %s: %w", fieldName, err)
	}

	return value, nil
}

// confidenceValue converts an optional confidence pointer into a JSON like value.
//
// Input
// confidence *float64
// Optional step confidence value.
//
// Output
// any
// Confidence number or nil when confidence is absent.
func confidenceValue(confidence *float64) any {
	if confidence == nil {
		return nil
	}

	return *confidence
}
