package main

import (
	"fmt"
	"strings"

	"github.com/rk0604/AgentTrace/attrib"
)

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
