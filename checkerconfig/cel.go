package checkerconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/rk0604/AgentTrace/attrib"
)

const (
	evaluationCostLimit      = 100000
	defaultEvaluationTimeout = 250 * time.Millisecond
	interruptCheckFrequency  = 100
)

// BuildOptions supplies generic run and evaluation context to CEL checkers.
type BuildOptions struct {
	Trace             *attrib.Trace
	Context           json.RawMessage
	EvaluationTimeout time.Duration
}

type compiledCheck struct {
	expression    string
	failureReason string
	program       cel.Program
}

type baseActivation struct {
	run      map[string]any
	context  map[string]any
	expected any
}

// Build compiles configured CEL expressions without external context.
//
// Input
// config Config
// Validated checker configuration.
//
// Output
// map of string to attrib.StepChecker
// Compiled checkers keyed by step ID.
//
// error
// Non nil when the configuration or a CEL expression is invalid.
func Build(config Config) (map[string]attrib.StepChecker, error) {
	return BuildWithOptions(config, BuildOptions{})
}

// BuildWithOptions compiles CEL expressions with run and evaluation context.
//
// Input
// config Config
// Validated version 1 or version 2 checker configuration.
//
// options BuildOptions
// Optional trace metadata, context JSON, and evaluation timeout.
//
// Output
// map of string to attrib.StepChecker
// Compiled checkers keyed by step ID.
//
// error
// Non nil when configuration, context, or expression compilation fails.
func BuildWithOptions(config Config, options BuildOptions) (map[string]attrib.StepChecker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	timeout, err := evaluationTimeout(options.EvaluationTimeout)
	if err != nil {
		return nil, err
	}
	activation, err := buildBaseActivation(options)
	if err != nil {
		return nil, err
	}
	environment, err := newEnvironment()
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
		checkConfigs, err := config.ChecksForStep(stepID)
		if err != nil {
			return nil, err
		}

		compiled := make([]compiledCheck, 0, len(checkConfigs))
		for checkIndex, checkConfig := range checkConfigs {
			program, err := compileProgram(environment, stepID, checkIndex, checkConfig.Expression)
			if err != nil {
				return nil, err
			}
			compiled = append(compiled, compiledCheck{
				expression:    checkConfig.Expression,
				failureReason: checkConfig.FailureReason,
				program:       program,
			})
		}

		checkers[stepID] = configuredChecker(compiled, activation, timeout)
	}

	return checkers, nil
}

// newEnvironment creates the generic AgentTrace CEL environment.
//
// Input
// None
//
// Output
// pointer to cel.Env
// Environment containing generic variables and helper functions.
//
// error
// Non nil when CEL cannot construct the environment.
func newEnvironment() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("input", cel.DynType),
		cel.Variable("output", cel.DynType),
		cel.Variable("step", cel.DynType),
		cel.Variable("run", cel.DynType),
		cel.Variable("context", cel.DynType),
		cel.Variable("expected", cel.DynType),
		cel.Function(
			"hasField",
			cel.Overload(
				"agenttrace_has_field",
				[]*cel.Type{cel.DynType, cel.StringType},
				cel.BoolType,
				cel.BinaryBinding(hasField),
			),
		),
		cel.Function(
			"isString",
			cel.Overload(
				"agenttrace_is_string",
				[]*cel.Type{cel.DynType},
				cel.BoolType,
				cel.UnaryBinding(isString),
			),
		),
		cel.Function(
			"isNumber",
			cel.Overload(
				"agenttrace_is_number",
				[]*cel.Type{cel.DynType},
				cel.BoolType,
				cel.UnaryBinding(isNumber),
			),
		),
		cel.Function(
			"isNonEmpty",
			cel.Overload(
				"agenttrace_is_non_empty",
				[]*cel.Type{cel.DynType},
				cel.BoolType,
				cel.UnaryBinding(isNonEmpty),
			),
		),
		cel.Function(
			"withinRange",
			cel.Overload(
				"agenttrace_within_range",
				[]*cel.Type{cel.DynType, cel.DynType, cel.DynType},
				cel.BoolType,
				cel.FunctionBinding(withinRange),
			),
		),
		cel.Function(
			"equalsExpected",
			cel.Overload(
				"agenttrace_equals_expected",
				[]*cel.Type{cel.DynType, cel.DynType},
				cel.BoolType,
				cel.BinaryBinding(equalsExpected),
			),
		),
		cel.Function(
			"sameField",
			cel.Overload(
				"agenttrace_same_field",
				[]*cel.Type{cel.DynType, cel.DynType, cel.StringType},
				cel.BoolType,
				cel.FunctionBinding(sameField),
			),
		),
	)
}

// compileProgram compiles one Boolean CEL expression.
//
// Input
// environment pointer to cel.Env
// CEL environment declaring available variables and helpers.
//
// stepID string
// Step identifier used in configuration errors.
//
// checkIndex int
// Position of the assertion inside its step.
//
// expression string
// CEL expression to compile.
//
// Output
// cel.Program
// Reusable compiled CEL program.
//
// error
// Non nil when parsing, type checking, or program construction fails.
func compileProgram(environment *cel.Env, stepID string, checkIndex int, expression string) (cel.Program, error) {
	ast, issues := environment.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile checker for step %q check %d: %w", stepID, checkIndex, issues.Err())
	}
	if !cel.BoolType.IsAssignableType(ast.OutputType()) {
		return nil, fmt.Errorf(
			"compile checker for step %q check %d: expression must return bool, got %s",
			stepID,
			checkIndex,
			ast.OutputType(),
		)
	}

	program, err := environment.Program(
		ast,
		cel.CostLimit(evaluationCostLimit),
		cel.InterruptCheckFrequency(interruptCheckFrequency),
	)
	if err != nil {
		return nil, fmt.Errorf("build checker for step %q check %d: %w", stepID, checkIndex, err)
	}

	return program, nil
}

// configuredChecker creates a checker backed by ordered CEL assertions.
//
// Input
// checks slice of compiledCheck
// Ordered assertions for one step.
//
// base baseActivation
// Shared run, context, and expected values.
//
// timeout time.Duration
// Maximum duration for each assertion.
//
// Output
// attrib.StepChecker
// Checker that stops at the first failed assertion.
func configuredChecker(checks []compiledCheck, base baseActivation, timeout time.Duration) attrib.StepChecker {
	return func(step attrib.Step) (attrib.CheckResult, error) {
		activation, err := stepActivation(step, base)
		if err != nil {
			return attrib.CheckResult{}, err
		}

		for _, check := range checks {
			evaluationContext, cancel := context.WithTimeout(context.Background(), timeout)
			value, _, evaluationErr := check.program.ContextEval(evaluationContext, activation)
			cancel()
			if evaluationErr != nil {
				return attrib.CheckResult{}, fmt.Errorf("evaluate CEL expression %q: %w", check.expression, evaluationErr)
			}

			passed, ok := value.Value().(bool)
			if !ok {
				return attrib.CheckResult{}, fmt.Errorf(
					"evaluate CEL expression %q: expected bool result, got %T",
					check.expression,
					value.Value(),
				)
			}
			if !passed {
				return attrib.Fail(check.failureReason), nil
			}
		}

		return attrib.Pass(), nil
	}
}

// buildBaseActivation converts build options into shared CEL values.
//
// Input
// options BuildOptions
// Optional trace and external context data.
//
// Output
// baseActivation
// Generic values shared by every step checker.
//
// error
// Non nil when context is invalid or is not a JSON object.
func buildBaseActivation(options BuildOptions) (baseActivation, error) {
	contextValue := map[string]any{}
	if len(options.Context) > 0 {
		decoded, err := decodePayload(options.Context, "context")
		if err != nil {
			return baseActivation{}, err
		}

		contextMap, ok := decoded.(map[string]any)
		if !ok {
			return baseActivation{}, fmt.Errorf("decode context: top level value must be an object")
		}
		contextValue = contextMap
	}

	expected := any(map[string]any{})
	if configuredExpected, exists := contextValue["expected"]; exists {
		expected = configuredExpected
	}

	run := map[string]any{}
	if options.Trace != nil {
		version := options.Trace.Version
		if version == 0 {
			version = attrib.CurrentTraceVersion
		}
		run = map[string]any{
			"version":    version,
			"run_id":     options.Trace.RunID,
			"step_count": len(options.Trace.Steps),
		}
	}

	return baseActivation{
		run:      run,
		context:  contextValue,
		expected: expected,
	}, nil
}

// stepActivation converts one trace step into CEL variables.
//
// Input
// step attrib.Step
// Trace step being checked.
//
// base baseActivation
// Shared run and external context values.
//
// Output
// map of string to any
// CEL activation containing all generic variables.
//
// error
// Non nil when an input or output payload is invalid JSON.
func stepActivation(step attrib.Step, base baseActivation) (map[string]any, error) {
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
		"input":    input,
		"output":   output,
		"step":     metadata,
		"run":      base.run,
		"context":  base.context,
		"expected": base.expected,
	}, nil
}

// decodePayload converts raw JSON into CEL compatible native values.
//
// Input
// payload json.RawMessage
// Raw JSON payload.
//
// fieldName string
// Field name used in errors.
//
// Output
// any
// Native JSON value for CEL evaluation.
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

// confidenceValue converts optional confidence into a CEL value.
//
// Input
// confidence pointer to float64
// Optional confidence number.
//
// Output
// any
// Confidence number or nil.
func confidenceValue(confidence *float64) any {
	if confidence == nil {
		return nil
	}

	return *confidence
}

func evaluationTimeout(configured time.Duration) (time.Duration, error) {
	if configured < 0 {
		return 0, fmt.Errorf("evaluation timeout cannot be negative")
	}
	if configured == 0 {
		return defaultEvaluationTimeout, nil
	}

	return configured, nil
}

func hasField(value ref.Val, field ref.Val) ref.Val {
	mapper, ok := value.(traits.Mapper)
	if !ok {
		return types.False
	}

	_, found := mapper.Find(field)
	return types.Bool(found)
}

func isString(value ref.Val) ref.Val {
	return types.Bool(value.Type() == types.StringType)
}

func isNumber(value ref.Val) ref.Val {
	_, ok := numberValue(value)
	return types.Bool(ok)
}

func isNonEmpty(value ref.Val) ref.Val {
	sizer, ok := value.(traits.Sizer)
	if !ok {
		return types.False
	}

	size, ok := sizer.Size().(types.Int)
	return types.Bool(ok && size > 0)
}

func withinRange(values ...ref.Val) ref.Val {
	if len(values) != 3 {
		return types.NewErr("withinRange requires three arguments")
	}

	value, valueOK := numberValue(values[0])
	minimum, minimumOK := numberValue(values[1])
	maximum, maximumOK := numberValue(values[2])
	if !valueOK || !minimumOK || !maximumOK {
		return types.False
	}

	return types.Bool(value >= minimum && value <= maximum)
}

func equalsExpected(actual ref.Val, expected ref.Val) ref.Val {
	return actual.Equal(expected)
}

func sameField(values ...ref.Val) ref.Val {
	if len(values) != 3 {
		return types.NewErr("sameField requires three arguments")
	}

	left, leftOK := values[0].(traits.Mapper)
	right, rightOK := values[1].(traits.Mapper)
	if !leftOK || !rightOK {
		return types.False
	}

	leftValue, leftFound := left.Find(values[2])
	rightValue, rightFound := right.Find(values[2])
	if !leftFound || !rightFound {
		return types.False
	}

	return leftValue.Equal(rightValue)
}

func numberValue(value ref.Val) (float64, bool) {
	switch number := value.(type) {
	case types.Double:
		return float64(number), true
	case types.Int:
		return float64(number), true
	case types.Uint:
		return float64(number), true
	default:
		return 0, false
	}
}
