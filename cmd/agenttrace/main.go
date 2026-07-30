package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/checkerconfig"
	"github.com/rk0604/AgentTrace/documentdemo"
	"github.com/rk0604/AgentTrace/incidentdemo"
	"github.com/rk0604/AgentTrace/toypipeline"
)

const (
	maxTraceFileBytes     int64 = 16 * 1024 * 1024
	maxCheckerFileBytes   int64 = 2 * 1024 * 1024
	maxContextFileBytes   int64 = 2 * 1024 * 1024
	maxEvidenceTextSize         = 1200
	maxGraphCanvasNodes         = 64
	maxGraphCanvasWidth         = 160
	maxGraphCanvasHeight        = 120
	maxGraphCanvasCells         = 160 * 120
	maxGraphCanvasEdges         = 96
	maxRenderedGraphNodes       = 100
	maxRenderedGraphEdges       = 256
)

const (
	exitCodeRuntime = 1
	exitCodeUsage   = 2
)

type usageError struct {
	err error
}

// Error returns the underlying usage message.
//
// Input
// usage usageError
// Error receiving the method call.
//
// Output
// string
// Human readable usage error.
func (usage usageError) Error() string {
	return usage.err.Error()
}

// Unwrap exposes the underlying error.
//
// Input
// usage usageError
// Error receiving the method call.
//
// Output
// error
// Wrapped error value.
func (usage usageError) Unwrap() error {
	return usage.err
}

// ExitCode returns the stable usage process code.
//
// Input
// usage usageError
// Error receiving the method call.
//
// Output
// int
// Process exit code for invalid command usage.
func (usage usageError) ExitCode() int {
	return exitCodeUsage
}

type outputOptions struct {
	JSON              bool
	OutputPath        string
	FailOnAttribution bool
	IncludeEvidence   bool
}

type stringListFlag []string

type stepView struct {
	Step     attrib.Step
	Checked  bool
	Failed   bool
	Root     bool
	Affected bool
	Reason   string
}

type graphLevel struct {
	Index int
	Views []stepView
}

type graphEdge struct {
	Source stepView
	Target stepView
	Cause  bool
}

type nodePlacement struct {
	View stepView
	X    int
	Y    int
}

func main() {
	if err := runCommandLine(os.Args[1:]); err != nil {
		exitWithError(err)
	}
}

// runCommandLine routes arguments to one AgentTrace command.
//
// Input
// args []string
// Command line arguments excluding the executable name.
//
// Output
// error
// Non nil when the command is missing, unknown, or unsuccessful.
func runCommandLine(args []string) error {
	if len(args) == 0 {
		return newUsageError("missing command: use run, validate, or demo")
	}

	switch args[0] {
	case "run":
		return runTraceCommand(args[1:])
	case "validate":
		return validateCommand(args[1:])
	case "demo":
		return demoCommand(args[1:])
	case "help", "--help", "-h":
		printUsage(os.Stdout)
		return nil
	default:
		return newUsageError("unknown command %q: use run, validate, or demo", args[0])
	}
}

// printUsage writes the supported command forms.
//
// Input
// writer io.Writer
// Destination for usage text.
//
// Output
// None
func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "AgentTrace")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "Commands")
	fmt.Fprintln(writer, "  run       Attribute a JSON trace")
	fmt.Fprintln(writer, "  validate  Validate a trace and checker configuration")
	fmt.Fprintln(writer, "  demo      Run toy, incident, or document")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "Run examples")
	fmt.Fprintln(writer, "  agenttrace run --input trace.json --checkers checkers.json [--context context.json] [--target step_id] [--include-evidence] [--fail-on-attribution]")
	fmt.Fprintln(writer, "  agenttrace validate --input trace.json --checkers checkers.json [--context context.json]")
	fmt.Fprintln(writer, "  agenttrace demo document --failure extraction")
}

// runTraceCommand attributes one JSON trace using CEL checker configuration.
//
// Input
// args []string
// Flags for trace input, checker configuration, and output selection.
//
// Output
// error
// Non nil when flags, files, checkers, or attribution are invalid.
func runTraceCommand(args []string) error {
	flags := newFlagSet("run")
	inputPath := flags.String("input", "", "read a trace from a JSON file")
	checkersPath := flags.String("checkers", "", "read CEL step checkers from a JSON configuration file")
	contextPath := flags.String("context", "", "read optional evaluation context from a JSON file")
	var targetStepIDs stringListFlag
	flags.Var(&targetStepIDs, "target", "analyze ancestry for this target step ID; repeat for multiple targets")
	output := addOutputFlags(flags)
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if *inputPath == "" {
		return newUsageError("run requires --input")
	}
	if *checkersPath == "" {
		return newUsageError("run requires --checkers")
	}

	trace, err := loadTrace(*inputPath)
	if err != nil {
		return err
	}
	checkers, err := loadCheckers(*checkersPath, trace, *contextPath)
	if err != nil {
		return err
	}

	return executeAttribution(
		trace,
		checkers,
		attrib.AnalysisOptions{TargetStepIDs: targetStepIDs},
		output,
	)
}

// validateCommand validates JSON trace and checker files without running attribution.
//
// Input
// args []string
// Flags containing required trace and checker configuration paths.
//
// Output
// error
// Non nil when flags, files, the dependency graph, or checker coverage are invalid.
func validateCommand(args []string) error {
	flags := newFlagSet("validate")
	inputPath := flags.String("input", "", "read a trace from a JSON file")
	checkersPath := flags.String("checkers", "", "read CEL step checkers from a JSON configuration file")
	contextPath := flags.String("context", "", "read optional evaluation context from a JSON file")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if *inputPath == "" {
		return newUsageError("validate requires --input")
	}
	if *checkersPath == "" {
		return newUsageError("validate requires --checkers")
	}

	trace, err := loadTrace(*inputPath)
	if err != nil {
		return err
	}
	checkers, err := loadCheckers(*checkersPath, trace, *contextPath)
	if err != nil {
		return err
	}
	if err := attrib.Validate(trace, checkers); err != nil {
		return err
	}

	fmt.Println("Validation passed")
	fmt.Printf("Run ID: %s\n", trace.RunID)
	fmt.Printf("Steps: %d\n", len(trace.Steps))
	fmt.Printf("Checkers: %d\n", len(checkers))
	return nil
}

// demoCommand routes to one explicit built in demonstration.
//
// Input
// args []string
// Demo name followed by its flags.
//
// Output
// error
// Non nil when the demo name is missing, unknown, or unsuccessful.
func demoCommand(args []string) error {
	if len(args) == 0 {
		return newUsageError("demo requires a name: use toy, incident, or document")
	}

	switch args[0] {
	case "toy":
		return demoToyCommand(args[1:])
	case "incident":
		return demoIncidentCommand(args[1:])
	case "document":
		return demoDocumentCommand(args[1:])
	default:
		return newUsageError("unknown demo %q: use toy, incident, or document", args[0])
	}
}

// demoToyCommand runs the deterministic four step toy pipeline.
//
// Input
// args []string
// Flags selecting healthy mode and output behavior.
//
// Output
// error
// Non nil when flags or attribution are invalid.
func demoToyCommand(args []string) error {
	flags := newFlagSet("demo toy")
	healthy := flags.Bool("healthy", false, "run without the injected Reference failure")
	output := addOutputFlags(flags)
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	trace := toypipeline.Run(!*healthy)
	return executeAttribution(
		trace,
		toypipeline.Checkers(),
		attrib.AnalysisOptions{},
		output,
	)
}

// demoIncidentCommand runs the deterministic incident investigation pipeline.
//
// Input
// args []string
// Flags selecting the failure mode, checker file, and output behavior.
//
// Output
// error
// Non nil when flags, failure mode, checker configuration, or attribution are invalid.
func demoIncidentCommand(args []string) error {
	flags := newFlagSet("demo incident")
	failure := flags.String("failure", string(incidentdemo.FailureMetrics), "select none, metrics, or deployment")
	checkersPath := flags.String("checkers", "examples/incident-checkers.json", "read CEL step checkers from a JSON configuration file")
	contextPath := flags.String("context", "", "read optional evaluation context from a JSON file")
	output := addOutputFlags(flags)
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	trace, err := incidentdemo.Run(incidentdemo.FailureMode(*failure))
	if err != nil {
		return usageError{err: err}
	}
	checkers, err := loadCheckers(*checkersPath, trace, *contextPath)
	if err != nil {
		return err
	}

	return executeAttribution(
		trace,
		checkers,
		attrib.AnalysisOptions{},
		output,
	)
}

// demoDocumentCommand runs the recorder based document review pipeline.
//
// Input
// args slice of string
// Flags selecting failure mode, checker context, trace output, and result output.
//
// Output
// error
// Non nil when flags, recording, configuration, or attribution fail.
func demoDocumentCommand(args []string) error {
	flags := newFlagSet("demo document")
	failure := flags.String("failure", string(documentdemo.FailureExtraction), "select none, extraction, or reference")
	checkersPath := flags.String("checkers", "examples/document-checkers-v2.json", "read CEL step checkers from a JSON configuration file")
	contextPath := flags.String("context", "examples/document-context.json", "read evaluation context from a JSON file")
	traceOutputPath := flags.String("trace-output", "", "write the recorded JSON trace to a file")
	output := addOutputFlags(flags)
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	trace, err := documentdemo.Run(documentdemo.FailureMode(*failure))
	if err != nil {
		return usageError{err: err}
	}
	if *traceOutputPath != "" {
		if err := writeTraceFile(*traceOutputPath, trace); err != nil {
			return err
		}
	}

	checkers, err := loadCheckers(*checkersPath, trace, *contextPath)
	if err != nil {
		return err
	}

	return executeAttribution(
		trace,
		checkers,
		attrib.AnalysisOptions{},
		output,
	)
}

// newFlagSet creates a command flag parser that returns errors to the caller.
//
// Input
// name string
// Command name included in parsing errors.
//
// Output
// *flag.FlagSet
// Flag parser configured for command routing.
func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

// addOutputFlags registers common attribution output flags.
//
// Input
// flags *flag.FlagSet
// Flag parser that receives the output flags.
//
// Output
// *outputOptions
// Values populated when the parser reads command arguments.
func addOutputFlags(flags *flag.FlagSet) *outputOptions {
	options := &outputOptions{}
	flags.BoolVar(&options.JSON, "json", false, "write only the JSON attribution result to standard output")
	flags.StringVar(&options.OutputPath, "output", "", "write the JSON attribution result to a file")
	flags.BoolVar(&options.FailOnAttribution, "fail-on-attribution", false, "return exit code 1 when attribution status is failed")
	flags.BoolVar(&options.IncludeEvidence, "include-evidence", false, "include raw input, output, and context payloads in attribution output")
	return options
}

// String formats configured repeatable flag values.
//
// Input
// values pointer to stringListFlag
// Flag values receiving the method call.
//
// Output
// string
// Comma separated values for flag diagnostics.
func (values *stringListFlag) String() string {
	if values == nil {
		return ""
	}

	return strings.Join(*values, ",")
}

// Set appends one nonempty repeatable flag value.
//
// Input
// value string
// Flag value supplied by the command line.
//
// Output
// error
// Non nil when the supplied value is empty.
func (values *stringListFlag) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("target step ID cannot be empty")
	}

	*values = append(*values, value)
	return nil
}

// parseFlags parses command flags and validates shared argument rules.
//
// Input
// flags *flag.FlagSet
// Configured command flag parser.
//
// args []string
// Arguments belonging to the command.
//
// Output
// error
// Non nil when parsing fails or positional arguments remain.
func parseFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return usageError{err: fmt.Errorf("parse %s flags: %w", flags.Name(), err)}
	}
	if flags.NArg() != 0 {
		return newUsageError("%s does not accept positional arguments", flags.Name())
	}

	return nil
}

// executeAttribution finds a root cause and writes the selected output format.
//
// Input
// trace attrib.Trace
// Trace to attribute.
//
// checkers map[string]attrib.StepChecker
// Step checker functions keyed by step ID.
//
// output *outputOptions
// Output format and optional destination file.
//
// Output
// error
// Non nil when options, graph order, attribution, or output writing fail.
func executeAttribution(
	trace attrib.Trace,
	checkers map[string]attrib.StepChecker,
	analysis attrib.AnalysisOptions,
	output *outputOptions,
) error {
	if output.JSON && output.OutputPath != "" {
		return newUsageError("json and output cannot be used together")
	}

	orderedSteps, err := attrib.TopologicalSort(trace)
	if err != nil {
		return err
	}
	result, err := attrib.Analyze(trace, checkers, analysis)
	if err != nil {
		return err
	}
	publishedResult := result
	if !output.IncludeEvidence {
		publishedResult = attrib.OmitEvidence(result)
	}
	if output.JSON {
		if err := attrib.EncodeResult(os.Stdout, publishedResult); err != nil {
			return err
		}
		return attributionFailure(result, output.FailOnAttribution)
	}
	if output.OutputPath != "" {
		if err := writeResultFile(output.OutputPath, publishedResult); err != nil {
			return err
		}
	}

	stepViews := buildStepViews(orderedSteps, result)
	printReport(trace, stepViews, publishedResult)
	if output.OutputPath != "" {
		fmt.Printf("\nJSON result written to %s\n", output.OutputPath)
	}

	return attributionFailure(result, output.FailOnAttribution)
}

// attributionFailure creates an optional CI gate error.
//
// Input
// result attrib.AttributionResult
// Completed attribution result.
//
// enabled bool
// Whether failed attribution should return a runtime error.
//
// Output
// error
// Non nil only when the gate is enabled and attribution failed.
func attributionFailure(result attrib.AttributionResult, enabled bool) error {
	if !enabled || result.Status != "failed" {
		return nil
	}
	if result.RootCause == nil {
		return fmt.Errorf("attribution status is failed")
	}

	return fmt.Errorf(
		"attribution failed at step %q: %s",
		result.RootCause.StepID,
		result.RootCause.Reason,
	)
}

// loadCheckers reads CEL checkers and optional context from JSON files.
//
// Input
// checkersPath string
// Checker configuration file path.
//
// trace attrib.Trace
// Trace metadata exposed to CEL through the run variable.
//
// contextPath string
// Optional JSON file exposed through context and expected variables.
//
// Output
// map[string]attrib.StepChecker
// Step checker functions keyed by step ID.
//
// error
// Non nil when a file cannot be opened, decoded, compiled, or closed.
func loadCheckers(checkersPath string, trace attrib.Trace, contextPath string) (map[string]attrib.StepChecker, error) {
	data, err := readLimitedFile(checkersPath, maxCheckerFileBytes, "checker configuration")
	if err != nil {
		return nil, err
	}

	config, err := checkerconfig.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	contextData, err := loadContext(contextPath)
	if err != nil {
		return nil, err
	}

	return checkerconfig.BuildWithOptions(config, checkerconfig.BuildOptions{
		Trace:   &trace,
		Context: contextData,
	})
}

// loadContext reads optional generic evaluation context.
//
// Input
// contextPath string
// Optional path to one JSON object.
//
// Output
// json.RawMessage
// Context bytes or nil when no path is supplied.
//
// error
// Non nil when the context file cannot be read.
func loadContext(contextPath string) (json.RawMessage, error) {
	if contextPath == "" {
		return nil, nil
	}

	data, err := readLimitedFile(contextPath, maxContextFileBytes, "evaluation context")
	if err != nil {
		return nil, err
	}

	return json.RawMessage(data), nil
}

// loadTrace reads one trace from a JSON file.
//
// Input
// inputPath string
// JSON file path.
//
// Output
// attrib.Trace
// The decoded trace.
//
// error
// Non nil when the JSON file cannot be opened, decoded, or closed.
func loadTrace(inputPath string) (attrib.Trace, error) {
	data, err := readLimitedFile(inputPath, maxTraceFileBytes, "trace file")
	if err != nil {
		return attrib.Trace{}, err
	}

	trace, err := attrib.DecodeTrace(bytes.NewReader(data))
	if err != nil {
		return attrib.Trace{}, err
	}

	return trace, nil
}

// readLimitedFile reads a bounded external input file.
//
// Input
// path string
// Source file path.
//
// limit int64
// Maximum accepted byte count.
//
// label string
// Human readable file type used in errors.
//
// Output
// slice of byte
// Complete file contents within the configured limit.
//
// error
// Non nil when the file cannot be read or exceeds the limit.
func readLimitedFile(path string, limit int64, label string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", label, err)
	}

	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read %s: %w", label, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", label, closeErr)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds byte limit %d", label, limit)
	}

	return data, nil
}

// writeResultFile writes an attribution result as JSON.
//
// Input
// outputPath string
// Destination JSON file path.
//
// result attrib.AttributionResult
// Attribution result to write.
//
// Output
// error
// Non nil when the file cannot be created, encoded, or closed.
func writeResultFile(outputPath string, result attrib.AttributionResult) error {
	file, err := createPrivateFile(outputPath, "result file")
	if err != nil {
		return err
	}

	encodeErr := attrib.EncodeResult(file, result)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return fmt.Errorf("close result file: %w", closeErr)
	}

	return nil
}

// writeTraceFile writes a completed trace as versioned JSON.
//
// Input
// outputPath string
// Destination JSON file path.
//
// trace attrib.Trace
// Completed generic trace.
//
// Output
// error
// Non nil when the file cannot be created, encoded, or closed.
func writeTraceFile(outputPath string, trace attrib.Trace) error {
	file, err := createPrivateFile(outputPath, "trace file")
	if err != nil {
		return err
	}

	encodeErr := attrib.EncodeTrace(file, trace)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return fmt.Errorf("close trace file: %w", closeErr)
	}

	return nil
}

// createPrivateFile creates or truncates an owner only output file.
//
// Input
// outputPath string
// Destination file path.
//
// label string
// Human readable file type used in errors.
//
// Output
// pointer to os.File
// Writable output file with owner read and write permissions.
//
// error
// Non nil when the file cannot be created or secured.
func createPrivateFile(outputPath string, label string) (*os.File, error) {
	file, err := os.OpenFile(
		outputPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0600,
	)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", label, err)
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure %s: %w", label, err)
	}

	return file, nil
}

// buildStepViews creates display rows for the CLI graph report.
//
// Input
// orderedSteps []attrib.Step
// Steps sorted so each dependency appears before the step that consumes it.
//
// result attrib.AttributionResult
// The attribution result returned by attrib.FindRootCause.
//
// Output
// []stepView
// Display rows with check status and failure reason.
func buildStepViews(orderedSteps []attrib.Step, result attrib.AttributionResult) []stepView {
	checkedIDs := checkedStepSet(result.CheckedStepIDs)
	downstreamStepIDs := result.DownstreamCandidateStepIDs
	if len(downstreamStepIDs) == 0 {
		downstreamStepIDs = result.AffectedStepIDs
	}
	affectedIDs := checkedStepSet(downstreamStepIDs)
	rootIDs := make(map[string]bool, len(result.RootCauses))
	for _, rootCause := range result.RootCauses {
		rootIDs[rootCause.StepID] = true
	}
	if result.RootCause != nil {
		rootIDs[result.RootCause.StepID] = true
	}
	divergencesByID := make(map[string]attrib.Divergence, len(result.Divergences))
	for _, divergence := range result.Divergences {
		divergencesByID[divergence.StepID] = divergence
	}
	views := make([]stepView, 0, len(orderedSteps))

	for _, step := range orderedSteps {
		view := stepView{
			Step:     step,
			Checked:  checkedIDs[step.StepID],
			Root:     rootIDs[step.StepID],
			Affected: affectedIDs[step.StepID],
		}

		if divergence, exists := divergencesByID[step.StepID]; exists {
			view.Failed = true
			view.Reason = divergence.Reason
		} else if result.RootCause != nil &&
			result.RootCause.StepID == step.StepID {
			view.Failed = true
			view.Reason = result.RootCause.Reason
		}

		views = append(views, view)
	}

	return views
}

// checkedStepSet creates a lookup table for checked step IDs.
//
// Input
// stepIDs []string
// Step IDs that were checked before attribution stopped.
//
// Output
// map[string]bool
// Lookup table where true means the step was checked.
func checkedStepSet(stepIDs []string) map[string]bool {
	checkedIDs := make(map[string]bool, len(stepIDs))

	for _, stepID := range stepIDs {
		checkedIDs[stepID] = true
	}

	return checkedIDs
}

// printReport writes the CLI graph report.
//
// Input
// trace attrib.Trace
// The trace being displayed.
//
// views []stepView
// Display rows for dependency ordered steps.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// None
func printReport(trace attrib.Trace, views []stepView, result attrib.AttributionResult) {
	fmt.Printf("AgentTrace run\n")
	fmt.Printf("Run ID: %s\n\n", trace.RunID)

	fmt.Printf("Dependency ordered steps\n")
	for index, view := range views {
		fmt.Printf("%d. %s\n", index+1, view.Step.AgentName)
		fmt.Printf("   Step ID: %s\n", view.Step.StepID)
		fmt.Printf("   Depends on: %s\n", dependsOnText(view.Step.DependsOn))
		fmt.Printf("   Check status: %s\n", checkStatusText(view))
		if view.Failed && view.Reason != "" {
			fmt.Printf("   Failure reason: %s\n", view.Reason)
		}
		fmt.Printf("\n")
	}

	fmt.Printf("Attribution result\n")
	fmt.Printf("Status: %s\n", result.Status)
	if len(result.TargetStepIDs) > 0 {
		fmt.Printf("Targets: %s\n", strings.Join(result.TargetStepIDs, ", "))
	}
	if result.RootCause == nil {
		fmt.Printf("Root cause: none\n\n")
		printFlowChart(views, result)
		return
	}

	fmt.Printf("Root cause step: %s\n", result.RootCause.StepID)
	fmt.Printf("Root cause agent: %s\n", result.RootCause.AgentName)
	fmt.Printf("Root cause reason: %s\n", result.RootCause.Reason)
	if result.RootCause.Expression != "" {
		fmt.Printf("Failed expression: %s\n", result.RootCause.Expression)
	}
	if len(result.RootCause.Input) > 0 {
		fmt.Printf("Actual input: %s\n", compactJSON(result.RootCause.Input))
	}
	if len(result.RootCause.Output) > 0 {
		fmt.Printf("Actual output: %s\n", compactJSON(result.RootCause.Output))
	}
	if len(result.RootCause.Expected) > 0 {
		fmt.Printf("Expected context: %s\n", compactJSON(result.RootCause.Expected))
	}
	if len(result.RootCauses) > 1 {
		rootCauseIDs := make([]string, 0, len(result.RootCauses))
		for _, rootCause := range result.RootCauses {
			rootCauseIDs = append(rootCauseIDs, rootCause.StepID)
		}
		fmt.Printf("All root causes: %s\n", strings.Join(rootCauseIDs, ", "))
	}
	if len(result.SecondaryDivergences) > 0 {
		secondaryIDs := make([]string, 0, len(result.SecondaryDivergences))
		for _, divergence := range result.SecondaryDivergences {
			secondaryIDs = append(secondaryIDs, divergence.StepID)
		}
		fmt.Printf(
			"Secondary divergences: %s\n",
			strings.Join(secondaryIDs, ", "),
		)
	}
	downstreamStepIDs := result.DownstreamCandidateStepIDs
	if len(downstreamStepIDs) == 0 {
		downstreamStepIDs = result.AffectedStepIDs
	}
	fmt.Printf(
		"Downstream candidates: %s\n",
		affectedStepsText(downstreamStepIDs),
	)
	fmt.Printf("\n")

	printFlowChart(views, result)
}

// dependsOnText formats dependency IDs for display.
//
// Input
// dependsOn []string
// Step IDs consumed by the current step.
//
// Output
// string
// Human readable dependency text.
func dependsOnText(dependsOn []string) string {
	if len(dependsOn) == 0 {
		return "none"
	}

	return strings.Join(dependsOn, ", ")
}

// checkStatusText formats one step check state for display.
//
// Input
// view stepView
// Display row for one trace step.
//
// Output
// string
// Human readable check status.
func checkStatusText(view stepView) string {
	if !view.Checked {
		if view.Affected {
			return "affected by upstream failure"
		}
		return "not checked because attribution stopped earlier"
	}
	if view.Failed {
		if view.Root {
			return "failed root cause"
		}
		return "failed"
	}
	if view.Affected {
		return "passed given actual input; downstream candidate"
	}

	return "passed"
}

// compactJSON formats raw JSON on one line.
//
// Input
// data json.RawMessage
// JSON value to display.
//
// Output
// string
// Compact JSON or the original text when compaction fails.
func compactJSON(data json.RawMessage) string {
	var output bytes.Buffer
	if err := json.Compact(&output, data); err != nil {
		return fitEvidenceText(string(data))
	}

	return fitEvidenceText(output.String())
}

// fitEvidenceText bounds evidence printed in a human report.
//
// Input
// text string
// Compact evidence text.
//
// Output
// string
// Original text or a bounded prefix with a truncation marker.
func fitEvidenceText(text string) string {
	characters := []rune(text)
	if len(characters) <= maxEvidenceTextSize {
		return text
	}

	const marker = "... truncated"
	return string(
		characters[:maxEvidenceTextSize-len([]rune(marker))],
	) + marker
}

// affectedStepsText formats downstream impact for the report.
//
// Input
// stepIDs slice of string
// Affected step IDs in dependency order.
//
// Output
// string
// Comma separated IDs or none.
func affectedStepsText(stepIDs []string) string {
	if len(stepIDs) == 0 {
		return "none"
	}

	return strings.Join(stepIDs, ", ")
}

// printFlowChart writes a generic dependency graph flow chart.
//
// Input
// views []stepView
// Display rows for dependency ordered steps.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// None
func printFlowChart(views []stepView, result attrib.AttributionResult) {
	viewsByID := stepViewByID(views)
	levelsByID := graphLevelsByID(views, viewsByID)
	levels := graphLevels(views, levelsByID)
	edges, totalEdges := graphEdges(views, viewsByID, result)

	fmt.Printf("Flow chart\n")
	fmt.Printf("\n")
	for _, line := range renderFlowChart(
		levels,
		edges,
		totalEdges,
		result,
	) {
		fmt.Printf("%s\n", line)
	}

	if result.RootCause != nil {
		rootCauseIDs := make([]string, 0, len(result.RootCauses))
		for _, rootCause := range result.RootCauses {
			rootCauseIDs = append(rootCauseIDs, rootCause.StepID)
		}
		if len(rootCauseIDs) == 0 {
			rootCauseIDs = append(rootCauseIDs, result.RootCause.StepID)
		}
		fmt.Printf("\nMarked nodes: %s\n", strings.Join(rootCauseIDs, ", "))
		fmt.Printf(
			"Marked edges: # shows potential failure propagation\n",
		)
	}
}

// renderFlowChart builds a connected ASCII graph diagram.
//
// Input
// levels []graphLevel
// Nodes grouped by visual level.
//
// edges []graphEdge
// Dependency edges in the graph.
//
// totalEdges int
// Total dependency edge count before display limits.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// []string
// Lines that form the rendered graph diagram.
func renderFlowChart(
	levels []graphLevel,
	edges []graphEdge,
	totalEdges int,
	result attrib.AttributionResult,
) []string {
	if totalEdges > maxGraphCanvasEdges {
		return renderCompactFlowChart(levels, edges, totalEdges)
	}

	placements, width, height, fits := graphPlacements(levels)
	if !fits {
		return renderCompactFlowChart(levels, edges, totalEdges)
	}

	canvas := newCanvas(width, height)

	for _, edge := range edges {
		source := placements[edge.Source.Step.StepID]
		target := placements[edge.Target.Step.StepID]
		drawConnector(canvas, source, target, edge.Cause)
	}

	for _, placement := range placements {
		drawNodeBox(canvas, placement, result)
	}

	return canvasLines(canvas)
}

// renderCompactFlowChart builds a bounded text view for a large graph.
//
// Input
// levels []graphLevel
// Nodes grouped by visual level.
//
// edges []graphEdge
// Dependency edges retained for display.
//
// totalEdges int
// Total dependency edge count before display limits.
//
// Output
// []string
// Bounded node and edge lines.
func renderCompactFlowChart(
	levels []graphLevel,
	edges []graphEdge,
	totalEdges int,
) []string {
	lines := []string{
		"Box layout omitted because the graph exceeds the display budget",
		"Nodes",
	}

	totalNodes := 0
	renderedNodes := 0
	for _, level := range levels {
		totalNodes += len(level.Views)
		for _, view := range level.Views {
			if renderedNodes >= maxRenderedGraphNodes {
				continue
			}

			lines = append(
				lines,
				fmt.Sprintf(
					"  L%d [%s] %s (%s)",
					level.Index,
					chartStatusText(view),
					fitText(view.Step.AgentName, 40),
					fitText(view.Step.StepID, 40),
				),
			)
			renderedNodes++
		}
	}
	if renderedNodes < totalNodes {
		lines = append(
			lines,
			fmt.Sprintf(
				"  %d additional nodes omitted",
				totalNodes-renderedNodes,
			),
		)
	}

	lines = append(lines, "Edges")
	for _, edge := range edges {
		marker := " -> "
		if edge.Cause {
			marker = " == CAUSE ==> "
		}
		lines = append(
			lines,
			"  "+
				fitText(edge.Source.Step.StepID, 40)+
				marker+
				fitText(edge.Target.Step.StepID, 40),
		)
	}
	if len(edges) < totalEdges {
		lines = append(
			lines,
			fmt.Sprintf(
				"  %d additional edges omitted",
				totalEdges-len(edges),
			),
		)
	}

	return lines
}

// graphPlacements computes node positions for the ASCII canvas.
//
// Input
// levels []graphLevel
// Nodes grouped by visual level.
//
// Output
// map[string]nodePlacement
// Node placement keyed by step ID.
//
// int
// Canvas width.
//
// int
// Canvas height.
//
// bool
// True when the graph fits within the canvas budget.
func graphPlacements(
	levels []graphLevel,
) (map[string]nodePlacement, int, int, bool) {
	const boxWidth = 24
	const boxHeight = 4
	const horizontalGap = 8
	const verticalGap = 5

	nodeCount := 0
	maxLevelWidth := 0
	for _, level := range levels {
		nodeCount += len(level.Views)
		if nodeCount > maxGraphCanvasNodes {
			return nil, 0, 0, false
		}

		levelWidth := len(level.Views)*boxWidth + maxInt(0, len(level.Views)-1)*horizontalGap
		if levelWidth > maxGraphCanvasWidth {
			return nil, 0, 0, false
		}
		if levelWidth > maxLevelWidth {
			maxLevelWidth = levelWidth
		}
	}

	placements := make(map[string]nodePlacement)
	for _, level := range levels {
		levelWidth := len(level.Views)*boxWidth + maxInt(0, len(level.Views)-1)*horizontalGap
		x := (maxLevelWidth - levelWidth) / 2
		y := level.Index * (boxHeight + verticalGap)

		for _, view := range level.Views {
			placements[view.Step.StepID] = nodePlacement{
				View: view,
				X:    x,
				Y:    y,
			}
			x += boxWidth + horizontalGap
		}
	}

	height := 0
	if len(levels) > 0 {
		height = (len(levels)-1)*(boxHeight+verticalGap) + boxHeight
	}
	if height > maxGraphCanvasHeight {
		return nil, 0, 0, false
	}
	if maxLevelWidth*height > maxGraphCanvasCells {
		return nil, 0, 0, false
	}

	return placements, maxLevelWidth, height, true
}

// newCanvas creates a blank ASCII canvas.
//
// Input
// width int
// Number of columns.
//
// height int
// Number of rows.
//
// Output
// [][]rune
// Blank canvas filled with spaces.
func newCanvas(width int, height int) [][]rune {
	canvas := make([][]rune, height)
	for row := range canvas {
		canvas[row] = make([]rune, width)
		for column := range canvas[row] {
			canvas[row][column] = ' '
		}
	}

	return canvas
}

// drawNodeBox draws one node box on the canvas.
//
// Input
// canvas [][]rune
// Canvas that receives the box.
//
// placement nodePlacement
// Node position and step data.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// None
func drawNodeBox(canvas [][]rune, placement nodePlacement, result attrib.AttributionResult) {
	putString(canvas, placement.X, placement.Y, boxTop())
	putString(canvas, placement.X, placement.Y+1, boxLine(placement.View.Step.AgentName))
	putString(canvas, placement.X, placement.Y+2, boxLine(chartNodeStatus(placement.View, result)))
	putString(canvas, placement.X, placement.Y+3, boxBottom())
}

// drawConnector draws one dependency connector between two boxes.
//
// Input
// canvas [][]rune
// Canvas that receives the connector.
//
// source nodePlacement
// Upstream node placement.
//
// target nodePlacement
// Downstream node placement.
//
// cause bool
// True when the connector leaves the root cause node.
//
// Output
// None
func drawConnector(canvas [][]rune, source nodePlacement, target nodePlacement, cause bool) {
	const boxWidth = 24
	const boxHeight = 4

	startX := source.X + boxWidth/2
	startY := source.Y + boxHeight
	endX := target.X + boxWidth/2
	endY := target.Y - 1
	steps := maxInt(1, endY-startY+1)

	for step := 0; step < steps; step++ {
		y := startY + step
		x := startX + ((endX - startX) * step / steps)
		mark := connectorRune(startX, endX)
		if step == steps-1 {
			mark = 'v'
		}
		if cause {
			mark = '#'
			if step == steps-1 {
				mark = 'V'
			}
		}
		putRune(canvas, x, y, mark)
	}
}

// connectorRune returns the connector character for one edge.
//
// Input
// startX int
// Connector start column.
//
// endX int
// Connector end column.
//
// Output
// rune
// Connector character.
func connectorRune(startX int, endX int) rune {
	if startX < endX {
		return '\\'
	}
	if startX > endX {
		return '/'
	}

	return '|'
}

// putString writes text on the canvas.
//
// Input
// canvas [][]rune
// Canvas that receives the text.
//
// x int
// Starting column.
//
// y int
// Row.
//
// text string
// Text to write.
//
// Output
// None
func putString(canvas [][]rune, x int, y int, text string) {
	for offset, char := range text {
		putRune(canvas, x+offset, y, char)
	}
}

// putRune writes one character on the canvas.
//
// Input
// canvas [][]rune
// Canvas that receives the character.
//
// x int
// Column.
//
// y int
// Row.
//
// char rune
// Character to write.
//
// Output
// None
func putRune(canvas [][]rune, x int, y int, char rune) {
	if y < 0 || y >= len(canvas) {
		return
	}
	if x < 0 || x >= len(canvas[y]) {
		return
	}

	canvas[y][x] = char
}

// canvasLines converts a canvas into printable lines.
//
// Input
// canvas [][]rune
// Canvas to convert.
//
// Output
// []string
// Printable lines with trailing spaces removed.
func canvasLines(canvas [][]rune) []string {
	lines := make([]string, 0, len(canvas))

	for _, row := range canvas {
		lines = append(lines, strings.TrimRight(string(row), " "))
	}

	return lines
}

// graphLevelsByID computes the visual level for each node.
//
// Input
// views []stepView
// Display rows in dependency order.
//
// viewsByID map[string]stepView
// Lookup table keyed by step ID.
//
// Output
// map[string]int
// Visual level keyed by step ID.
func graphLevelsByID(views []stepView, viewsByID map[string]stepView) map[string]int {
	levelsByID := make(map[string]int, len(views))

	for _, view := range views {
		level := 0

		for _, dependencyID := range view.Step.DependsOn {
			if _, exists := viewsByID[dependencyID]; !exists {
				continue
			}

			dependencyLevel := levelsByID[dependencyID] + 1
			if dependencyLevel > level {
				level = dependencyLevel
			}
		}

		levelsByID[view.Step.StepID] = level
	}

	return levelsByID
}

// graphLevels groups nodes by visual level.
//
// Input
// views []stepView
// Display rows in dependency order.
//
// levelsByID map[string]int
// Visual level keyed by step ID.
//
// Output
// []graphLevel
// Levels containing nodes to render together.
func graphLevels(views []stepView, levelsByID map[string]int) []graphLevel {
	maxLevel := 0
	for _, level := range levelsByID {
		if level > maxLevel {
			maxLevel = level
		}
	}

	levels := make([]graphLevel, maxLevel+1)
	for index := range levels {
		levels[index] = graphLevel{Index: index}
	}

	for _, view := range views {
		level := levelsByID[view.Step.StepID]
		levels[level].Views = append(levels[level].Views, view)
	}

	return levels
}

// graphEdges builds dependency edges for the flow chart.
//
// Input
// views []stepView
// Display rows in dependency order.
//
// viewsByID map[string]stepView
// Lookup table keyed by step ID.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// []graphEdge
// Bounded dependency edges with cause markers when applicable.
//
// int
// Total dependency edge count before display limits.
func graphEdges(
	views []stepView,
	viewsByID map[string]stepView,
	result attrib.AttributionResult,
) ([]graphEdge, int) {
	edges := make([]graphEdge, 0, maxRenderedGraphEdges)
	totalEdges := 0

	for _, target := range views {
		for _, sourceID := range target.Step.DependsOn {
			source, exists := viewsByID[sourceID]
			if !exists {
				continue
			}
			totalEdges++

			if len(edges) < maxRenderedGraphEdges &&
				isCauseEdge(sourceID, target.Step.StepID, result) {
				edges = append(edges, graphEdge{
					Source: source,
					Target: target,
					Cause:  true,
				})
			}
		}
	}

	for _, target := range views {
		for _, sourceID := range target.Step.DependsOn {
			if len(edges) >= maxRenderedGraphEdges {
				return edges, totalEdges
			}

			source, exists := viewsByID[sourceID]
			if !exists || isCauseEdge(
				sourceID,
				target.Step.StepID,
				result,
			) {
				continue
			}

			edges = append(edges, graphEdge{
				Source: source,
				Target: target,
			})
		}
	}

	return edges, totalEdges
}

// isCauseEdge reports whether an edge carries the failed output.
//
// Input
// sourceID string
// Step ID for the edge source.
//
// targetID string
// Step ID for the edge target.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// bool
// True when the edge leaves the root cause step.
func isCauseEdge(
	sourceID string,
	targetID string,
	result attrib.AttributionResult,
) bool {
	edges := result.PotentialPropagationEdges
	if len(edges) == 0 {
		edges = result.CauseEdges
	}
	for _, edge := range edges {
		if edge.FromStepID == sourceID && edge.ToStepID == targetID {
			return true
		}
	}

	return false
}

// stepViewByID creates a lookup table for CLI step views.
//
// Input
// views []stepView
// Display rows for dependency ordered steps.
//
// Output
// map[string]stepView
// Lookup table keyed by step ID.
func stepViewByID(views []stepView) map[string]stepView {
	viewsByID := make(map[string]stepView, len(views))

	for _, view := range views {
		viewsByID[view.Step.StepID] = view
	}

	return viewsByID
}

// chartStatusText formats one compact node status for the CLI flow chart.
//
// Input
// view stepView
// Display row for one trace step.
//
// Output
// string
// Compact node status text.
func chartStatusText(view stepView) string {
	if !view.Checked {
		if view.Affected {
			return "AFFECTED"
		}
		return "SKIPPED"
	}
	if view.Failed {
		if view.Root {
			return "FAIL ROOT CAUSE"
		}
		return "FAIL"
	}
	if view.Affected {
		return "DOWNSTREAM"
	}

	return "PASS"
}

// chartNodeStatus formats one boxed node status for the toy flow chart.
//
// Input
// view stepView
// Display row for one trace step.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// string
// Compact node status with root cause marker when applicable.
func chartNodeStatus(view stepView, result attrib.AttributionResult) string {
	_ = result
	status := chartStatusText(view)
	return status
}

// boxTop returns the top border for a fixed width CLI node box.
//
// Input
// None
//
// Output
// string
// Top border text.
func boxTop() string {
	return "+----------------------+"
}

// boxBottom returns the bottom border for a fixed width CLI node box.
//
// Input
// None
//
// Output
// string
// Bottom border text.
func boxBottom() string {
	return "+----------------------+"
}

// boxLine formats one content row for a fixed width CLI node box.
//
// Input
// text string
// Content to display inside the box.
//
// Output
// string
// Box row containing the text.
func boxLine(text string) string {
	return fmt.Sprintf("| %-20s |", fitText(text, 20))
}

// fitText trims text to a fixed display width.
//
// Input
// text string
// Text to fit.
//
// width int
// Maximum allowed width.
//
// Output
// string
// Text that fits within the requested width.
func fitText(text string, width int) string {
	if width <= 0 {
		return ""
	}

	characters := []rune(text)
	if len(characters) <= width {
		return text
	}

	if width <= 3 {
		return string(characters[:width])
	}

	return string(characters[:width-3]) + "..."
}

// maxInt returns the larger integer.
//
// Input
// left int
// First value.
//
// right int
// Second value.
//
// Output
// int
// Larger value.
func maxInt(left int, right int) int {
	if left > right {
		return left
	}

	return right
}

// newUsageError creates an error for invalid command usage.
//
// Input
// format string
// Error message format.
//
// values variadic any
// Values interpolated into the format.
//
// Output
// error
// Error that maps to the usage exit code.
func newUsageError(format string, values ...any) error {
	return usageError{err: fmt.Errorf(format, values...)}
}

// commandExitCode maps a command error to a process exit code.
//
// Input
// err error
// Command failure.
//
// Output
// int
// Stable usage or runtime exit code.
func commandExitCode(err error) int {
	var coded interface {
		ExitCode() int
	}
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}

	return exitCodeRuntime
}

// writeCommandError writes a human or JSON error record.
//
// Input
// writer io.Writer
// Error output destination.
//
// err error
// Command failure to report.
//
// format string
// Output format selected by AGENTTRACE_LOG_FORMAT.
//
// Output
// None
func writeCommandError(writer io.Writer, err error, format string) {
	if format == "json" {
		record := map[string]any{
			"level":     "error",
			"message":   err.Error(),
			"exit_code": commandExitCode(err),
		}
		encoder := json.NewEncoder(writer)
		_ = encoder.Encode(record)
		return
	}

	fmt.Fprintf(writer, "error: %v\n", err)
}

// exitWithError prints an error and exits the program.
//
// Input
// err error
// Error that stopped the command.
//
// Output
// None
func exitWithError(err error) {
	writeCommandError(os.Stderr, err, os.Getenv("AGENTTRACE_LOG_FORMAT"))
	os.Exit(commandExitCode(err))
}
