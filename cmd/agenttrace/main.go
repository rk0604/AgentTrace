package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rk0604/AgentTrace/attrib"
	"github.com/rk0604/AgentTrace/checkerconfig"
	"github.com/rk0604/AgentTrace/toypipeline"
)

type stepView struct {
	Step    attrib.Step
	Checked bool
	Failed  bool
	Reason  string
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
	healthy := flag.Bool("healthy", false, "run the toy pipeline without the injected reference failure")
	inputPath := flag.String("input", "", "read a trace from a JSON file instead of running the toy pipeline")
	checkersPath := flag.String("checkers", "", "read CEL step checkers from a JSON configuration file")
	jsonOutput := flag.Bool("json", false, "write only the JSON attribution result to standard output")
	outputPath := flag.String("output", "", "write the JSON attribution result to a file")
	flag.Parse()

	if *inputPath != "" && *healthy {
		exitWithError(fmt.Errorf("healthy cannot be used with input"))
	}
	if *jsonOutput && *outputPath != "" {
		exitWithError(fmt.Errorf("json and output cannot be used together"))
	}

	// Build a deterministic trace or decode one from JSON.
	trace, err := loadTrace(*inputPath, !*healthy)
	if err != nil {
		exitWithError(err)
	}
	checkers, err := loadCheckers(*checkersPath)
	if err != nil {
		exitWithError(err)
	}

	// Compute dependency order from DependsOn.
	orderedSteps, err := attrib.TopologicalSort(trace)
	if err != nil {
		exitWithError(err)
	}

	// Run first divergence attribution.
	result, err := attrib.FindRootCause(trace, checkers)
	if err != nil {
		exitWithError(err)
	}
	if *jsonOutput {
		if err := attrib.EncodeResult(os.Stdout, result); err != nil {
			exitWithError(err)
		}
		return
	}
	if *outputPath != "" {
		if err := writeResultFile(*outputPath, result); err != nil {
			exitWithError(err)
		}
	}

	stepViews := buildStepViews(orderedSteps, result, checkers)
	printReport(trace, stepViews, result)
	if *outputPath != "" {
		fmt.Printf("\nJSON result written to %s\n", *outputPath)
	}
}

// loadCheckers creates toy checkers or reads CEL checkers from a JSON file.
//
// Input
// checkersPath string
// Checker configuration file path. An empty path selects the toy Go checkers.
//
// Output
// map[string]attrib.StepChecker
// Step checker functions keyed by step ID.
//
// error
// Non nil when the file cannot be opened, decoded, compiled, or closed.
func loadCheckers(checkersPath string) (map[string]attrib.StepChecker, error) {
	if checkersPath == "" {
		return toypipeline.Checkers(), nil
	}

	file, err := os.Open(checkersPath)
	if err != nil {
		return nil, fmt.Errorf("open checker configuration: %w", err)
	}

	config, decodeErr := checkerconfig.Decode(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return nil, decodeErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close checker configuration: %w", closeErr)
	}

	return checkerconfig.Build(config)
}

// loadTrace creates the toy trace or reads one from a JSON file.
//
// Input
// inputPath string
// JSON file path. An empty path selects the generated toy trace.
//
// injectReferenceFailure bool
// True when the generated toy trace should contain the Reference failure.
//
// Output
// attrib.Trace
// The generated or decoded trace.
//
// error
// Non nil when the JSON file cannot be opened, decoded, or closed.
func loadTrace(inputPath string, injectReferenceFailure bool) (attrib.Trace, error) {
	if inputPath == "" {
		return toypipeline.Run(injectReferenceFailure), nil
	}

	file, err := os.Open(inputPath)
	if err != nil {
		return attrib.Trace{}, fmt.Errorf("open trace file: %w", err)
	}

	trace, decodeErr := attrib.DecodeTrace(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return attrib.Trace{}, decodeErr
	}
	if closeErr != nil {
		return attrib.Trace{}, fmt.Errorf("close trace file: %w", closeErr)
	}

	return trace, nil
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
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create result file: %w", err)
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

// buildStepViews creates display rows for the CLI graph report.
//
// Input
// orderedSteps []attrib.Step
// Steps sorted so each dependency appears before the step that consumes it.
//
// result attrib.AttributionResult
// The attribution result returned by attrib.FindRootCause.
//
// checkers map[string]attrib.StepChecker
// Step checker functions keyed by step ID.
//
// Output
// []stepView
// Display rows with check status and failure reason.
func buildStepViews(orderedSteps []attrib.Step, result attrib.AttributionResult, checkers map[string]attrib.StepChecker) []stepView {
	checkedIDs := checkedStepSet(result.CheckedStepIDs)
	views := make([]stepView, 0, len(orderedSteps))

	for _, step := range orderedSteps {
		view := stepView{
			Step:    step,
			Checked: checkedIDs[step.StepID],
		}

		if view.Checked {
			// Reuse the same checker functions so the report matches attribution.
			check, err := checkers[step.StepID](step)
			if err != nil {
				view.Failed = true
				view.Reason = err.Error()
			} else if !check.Passed {
				view.Failed = true
				view.Reason = check.Reason
			}
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
	if result.RootCause == nil {
		fmt.Printf("Root cause: none\n\n")
		printFlowChart(views, result)
		return
	}

	fmt.Printf("Root cause step: %s\n", result.RootCause.StepID)
	fmt.Printf("Root cause agent: %s\n", result.RootCause.AgentName)
	fmt.Printf("Root cause reason: %s\n\n", result.RootCause.Reason)

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
		return "not checked because attribution stopped earlier"
	}
	if view.Failed {
		return "failed"
	}

	return "passed"
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
	edges := graphEdges(views, viewsByID, result)

	fmt.Printf("Flow chart\n")
	fmt.Printf("\n")
	for _, line := range renderFlowChart(levels, edges, result) {
		fmt.Printf("%s\n", line)
	}

	if result.RootCause != nil {
		fmt.Printf("\nMarked node: %s\n", result.RootCause.StepID)
		fmt.Printf("Marked edge: bad output leaving %s\n", result.RootCause.StepID)
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
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// []string
// Lines that form the rendered graph diagram.
func renderFlowChart(levels []graphLevel, edges []graphEdge, result attrib.AttributionResult) []string {
	placements, width, height := graphPlacements(levels)
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
func graphPlacements(levels []graphLevel) (map[string]nodePlacement, int, int) {
	const boxWidth = 24
	const boxHeight = 4
	const horizontalGap = 8
	const verticalGap = 5

	maxLevelWidth := 0
	for _, level := range levels {
		levelWidth := len(level.Views)*boxWidth + maxInt(0, len(level.Views)-1)*horizontalGap
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

	return placements, maxLevelWidth, height
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
		if cause && mark == '|' {
			putString(canvas, x, y, "||")
		} else {
			putRune(canvas, x, y, mark)
		}
	}

	if cause {
		labelY := startY + steps/2
		labelX := minInt(startX, endX) + 3
		putString(canvas, labelX, labelY, "CAUSE EDGE")
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
// Dependency edges with cause markers when applicable.
func graphEdges(views []stepView, viewsByID map[string]stepView, result attrib.AttributionResult) []graphEdge {
	edges := make([]graphEdge, 0)

	for _, target := range views {
		for _, sourceID := range target.Step.DependsOn {
			source, exists := viewsByID[sourceID]
			if !exists {
				continue
			}

			edges = append(edges, graphEdge{
				Source: source,
				Target: target,
				Cause:  isCauseEdge(sourceID, result),
			})
		}
	}

	return edges
}

// isCauseEdge reports whether an edge carries the failed output.
//
// Input
// sourceID string
// Step ID for the upstream edge source.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// bool
// True when the edge leaves the root cause step.
func isCauseEdge(sourceID string, result attrib.AttributionResult) bool {
	return result.RootCause != nil && sourceID == result.RootCause.StepID
}

// printLevelBoxes writes boxes for all nodes in one graph level.
//
// Input
// views []stepView
// Display rows for one visual level.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// None
func printLevelBoxes(views []stepView, result attrib.AttributionResult) {
	const maxBoxesPerRow = 3

	for start := 0; start < len(views); start += maxBoxesPerRow {
		end := start + maxBoxesPerRow
		if end > len(views) {
			end = len(views)
		}

		printBoxRow(views[start:end], result)
	}
}

// printBoxRow writes one horizontal row of node boxes.
//
// Input
// views []stepView
// Display rows to render as boxes.
//
// result attrib.AttributionResult
// Root cause attribution result.
//
// Output
// None
func printBoxRow(views []stepView, result attrib.AttributionResult) {
	printRepeatedBoxLine(views, boxTop)

	for _, view := range views {
		fmt.Printf("%s  ", boxLine(view.Step.AgentName))
	}
	fmt.Printf("\n")

	for _, view := range views {
		fmt.Printf("%s  ", boxLine(chartNodeStatus(view, result)))
	}
	fmt.Printf("\n")

	for _, view := range views {
		fmt.Printf("%s  ", boxLine("id "+view.Step.StepID))
	}
	fmt.Printf("\n")

	printRepeatedBoxLine(views, boxBottom)
}

// printRepeatedBoxLine writes one border row for each box in a row.
//
// Input
// views []stepView
// Display rows to render as boxes.
//
// line func() string
// Function that returns a box border line.
//
// Output
// None
func printRepeatedBoxLine(views []stepView, line func() string) {
	for range views {
		fmt.Printf("%s  ", line())
	}
	fmt.Printf("\n")
}

// printOutgoingEdges writes edges leaving one graph level.
//
// Input
// level int
// Visual level whose outgoing edges should be displayed.
//
// edges []graphEdge
// Dependency edges in the graph.
//
// levelsByID map[string]int
// Visual level keyed by step ID.
//
// Output
// None
func printOutgoingEdges(level int, edges []graphEdge, levelsByID map[string]int) {
	levelEdges := make([]graphEdge, 0)

	for _, edge := range edges {
		if levelsByID[edge.Source.Step.StepID] == level {
			levelEdges = append(levelEdges, edge)
		}
	}

	if len(levelEdges) == 0 {
		return
	}

	fmt.Printf("Edges\n")
	for _, edge := range levelEdges {
		fmt.Printf("  %s\n", edgeText(edge))
	}
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

// edgeText formats one dependency edge for the CLI flow chart.
//
// Input
// edge graphEdge
// Dependency edge to display.
//
// Output
// string
// Human readable edge text.
func edgeText(edge graphEdge) string {
	source := edge.Source.Step.StepID
	target := edge.Target.Step.StepID

	if edge.Cause {
		return fmt.Sprintf("%s == CAUSE EDGE ==> %s", source, target)
	}

	return fmt.Sprintf("%s -> %s", source, target)
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
		return "SKIPPED"
	}
	if view.Failed {
		return "FAIL"
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
	status := chartStatusText(view)
	if result.RootCause != nil && view.Step.StepID == result.RootCause.StepID {
		return status + " ROOT CAUSE"
	}

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
	if len(text) <= width {
		return text
	}

	if width <= 3 {
		return text[:width]
	}

	return text[:width-3] + "..."
}

// minInt returns the smaller integer.
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
// Smaller value.
func minInt(left int, right int) int {
	if left < right {
		return left
	}

	return right
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

// exitWithError prints an error and exits the program.
//
// Input
// err error
// Error that stopped the command.
//
// Output
// None
func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
