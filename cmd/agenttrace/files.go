package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/checkerconfig"
)

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
