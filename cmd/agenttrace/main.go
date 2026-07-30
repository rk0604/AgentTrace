package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rk0604/AgentTrace/attrib"
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
