package tui

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestBrailleDotPositionsDistinct(t *testing.T) {
	seen := map[uint8]bool{}
	for _, row := range brailleDotBit {
		for _, bit := range row {
			if bit == 0 || seen[bit] || bit&(bit-1) != 0 {
				t.Fatalf("invalid Braille bit map: %v", brailleDotBit)
			}
			seen[bit] = true
		}
	}
	if len(seen) != 8 {
		t.Fatalf("Braille missing dots: %d", len(seen))
	}
}

func TestBrailleSamplesAreRightAnchoredWithoutInventingHistory(t *testing.T) {
	// A full-width chart with a single 5-second measurement may only occupy
	// the rightmost time slot. The empty preceding five-minute window is blank.
	cells := buildBrailleHistory([]float64{10}, 60, 5, 10)
	occupied := []int{}
	for col := 0; col < 60; col++ {
		for row := range cells {
			if cells[row][col].bits != 0 {
				occupied = append(occupied, col)
				break
			}
		}
	}
	if len(occupied) != 1 || occupied[0] != 59 {
		t.Fatalf("single sample should occupy last time slot only: %v", occupied)
	}
	if cells[0][0].bits != 0 {
		t.Fatal("future samples were invented on the left")
	}
}

func TestBrailleInvalidAndZeroValuesProduceNoActivity(t *testing.T) {
	for _, v := range []float64{0, -4, math.NaN(), math.Inf(1)} {
		if got := plottedValue(v, 10, 40); got != 0 {
			t.Fatalf("invalid value %v plotted: %d", v, got)
		}
	}
	for _, row := range buildBrailleHistory([]float64{0, -1, math.NaN()}, 60, 5, 10) {
		for _, cell := range row {
			if cell.bits != 0 {
				t.Fatal("zero/invalid measurements generated a visible wave")
			}
		}
	}
}

func TestBraillePlotKeepsUnitsAxesAndTerminalWidth(t *testing.T) {
	cases := []struct {
		width, height int
	}{
		{12, 1}, {21, 2}, {45, 7}, {110, 15},
	}
	for _, tc := range cases {
		rows := plotHistory([]float64{5, 20, 275}, tc.width, tc.height, 10, green)
		if len(rows) != tc.height {
			t.Fatalf("%+v rendered %d lines", tc, len(rows))
		}
		if !strings.Contains(rows[0], "500.0") {
			t.Fatalf("CPU >100%% plot clipped/mislabeled: %q", rows[0])
		}
		for _, row := range rows {
			if n := lipgloss.Width(row); n != tc.width {
				t.Fatalf("%+v row width %d: %q", tc, n, row)
			}
		}
	}
}

func TestBrailleTraceHasOutlineAndSparseInterior(t *testing.T) {
	cells := buildBrailleHistory([]float64{45, 45, 45}, 60, 8, 50)
	var outline, filled int
	for _, row := range cells {
		for _, cell := range row {
			if cell.bits != 0 { filled++ }
			if cell.outline { outline++ }
		}
	}
	if outline == 0 || filled <= outline {
		t.Fatalf("missing detail in Braille trace: outlined %d, filled %d", outline, filled)
	}
}

func TestBrailleGraphResponsiveOnAllDashboardSizes(t *testing.T) {
	for _, dims := range [][2]int{{172, 52}, {120, 35}, {90, 30}, {50, 16}, {30, 10}} {
		m := testModel()
		m.width, m.height = dims[0], dims[1]
		m.cpuHistory = []float64{0.2, 1.7, 3.0, 1.2}
		m.memHistory = []float64{42.5, 44.1, 43.8}
		for _, page := range []int{0, 1, 5} {
			m.page = page
			view := m.View()
			if lipgloss.Height(view) > m.height {
				t.Fatalf("%v page %d overflowed vertical size", dims, page)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > m.width {
					t.Fatalf("%v page %d overflowed horizontal size", dims, page)
				}
			}
		}
	}
}
