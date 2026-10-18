package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Unicode Braille packs 2 horizontal by 4 vertical dots in a single terminal
// cell. We use exact sampled values, held across their five-second time slot,
// rather than smoothing, interpolation, or invented measurements.
var brailleDotBit = [4][2]uint8{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

type graphCell struct {
	bits    uint8
	outline bool
}

func plottedValue(v, ceiling float64, dots int) int {
	if math.IsNaN(v) || v <= 0 || math.IsInf(v, 0) || ceiling <= 0 {
		return 0
	}
	if v >= ceiling {
		return dots
	}
	return max(1, min(dots, int(math.Round(v/ceiling*float64(dots)))))
}

// buildBrailleHistory maps the bounded 60-slot history onto terminal dot
// columns. Empty preceding time slots stay empty; they are not zero samples.
func buildBrailleHistory(samples []float64, columns, rows int, ceiling float64) [][]graphCell {
	if len(samples) > historyLimit {
		samples = samples[len(samples)-historyLimit:]
	}
	result := make([][]graphCell, rows)
	for row := range result {
		result[row] = make([]graphCell, columns)
	}
	if rows <= 0 || columns <= 0 {
		return result
	}
	dotCols, dotRows := columns*2, rows*4
	for x := 0; x < dotCols; x++ {
		slot := x * historyLimit / dotCols
		sampleIdx := slot - (historyLimit - len(samples))
		if sampleIdx < 0 || sampleIdx >= len(samples) {
			continue
		}
		filled := plottedValue(samples[sampleIdx], ceiling, dotRows)
		if filled == 0 {
			continue
		}
		top := dotRows - filled
		for y := top; y < dotRows; y++ {
			// A bright thin contour and sparse under-fill make a stable series
			// look like a trace, not an opaque rectangular slab.
			outline := y == top || y == top+1
			if !outline && ((y-top)%4 != 0 || x%2 != 0) {
				continue
			}
			row, col := y/4, x/2
			cell := &result[row][col]
			cell.bits |= brailleDotBit[y%4][x%2]
			cell.outline = cell.outline || outline
		}
	}
	return result
}

// plotHistory keeps the historical API for existing graph callers and tests.
func plotHistory(samples []float64, width, height int, minimum float64, accent lipgloss.Style) []string {
	return plotHistoryTones(samples, width, height, minimum, accent, graphLowerFill, graphMiddleFill)
}

// plotHistoryTones renders readable axes and a time-accurate, right-anchored
// Braille activity trace. The ceiling remains tied to real sample peaks.
func plotHistoryTones(samples []float64, width, height int, minimum float64, accent, low, middle lipgloss.Style) []string {
	width = max(12, width)
	height = max(1, height)
	const axis = 9
	columns := width - axis
	if len(samples) > historyLimit {
		samples = samples[len(samples)-historyLimit:]
	}
	ceiling := niceScale(historyPeak(samples), minimum)
	cells := buildBrailleHistory(samples, columns, height, ceiling)
	styles := [4]lipgloss.Style{muted, low, middle, accent}
	out := make([]string, 0, height)
	for row := 0; row < height; row++ {
		axisText := "         "
		switch {
		case row == 0:
			axisText = fmt.Sprintf("%8.1f ", ceiling)
		case row == height-1:
			axisText = "    0.0  "
		case row == height/2:
			axisText = fmt.Sprintf("%8.1f ", ceiling/2)
		}
		var rowText strings.Builder
		// Batch adjacent cells of the same color to minimize escape codes
		// and rendering allocations during each TUI refresh.
		var span strings.Builder
		current := -1
		flush := func() {
			if current >= 0 && span.Len() > 0 {
				rowText.WriteString(styles[current].Render(span.String()))
				span.Reset()
			}
		}
		for col, cell := range cells[row] {
			mode := 0
			char := ' '
			if cell.bits == 0 {
				// Faint reference lines are not activity samples.
				if (row == 0 || row == height/2 || row == height-1) && col%4 == 0 {
					char = '·'
				}
			} else {
				char = rune(0x2800) + rune(cell.bits)
				mode = 1
				if row < height*2/3 {
					mode = 2
				}
				if cell.outline {
					mode = 3
				}
			}
			if mode != current {
				flush()
				current = mode
			}
			span.WriteRune(char)
		}
		flush()
		out = append(out, muted.Render(axisText)+rowText.String())
	}
	return out
}
