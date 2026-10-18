package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// A single terminal-native palette for the first Docktop visual design pass.
// No external assets, extra services, or rendering dependencies are required.
const (
	colorCanvas    = "#101522"
	colorPanel     = "#1A2232"
	colorText      = "#E4EDF7"
	colorMuted     = "#90A1B7"
	colorBorder    = "#52627E"
	colorSelection = "#26516B"
	colorCyan      = "#69D4F5"
	colorGreen     = "#56DCA1"
	colorAmber     = "#F1C878"
	colorRed       = "#FF8292"
	colorViolet    = "#B39AF5"
	colorGraphLow  = "#326984"
	colorGraphMid  = "#3CA8B3"
)

var (
	canvasSurface  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText)).Background(lipgloss.Color(colorCanvas))
	panelSurface   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText)).Background(lipgloss.Color(colorPanel))
	graphLowerFill  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGraphLow))
	graphMiddleFill = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGraphMid))
	graphCPULow     = lipgloss.NewStyle().Foreground(lipgloss.Color("#28564F"))
	graphCPUMid     = lipgloss.NewStyle().Foreground(lipgloss.Color("#349C83"))
)

// Lip Gloss applies the desired background around a rendered block, but color
// spans inside that block can reset the terminal background to its transparent
// default. This happened in Ptyxis: borders and outer gutters were opaque
// while the panel interiors showed desktop wallpaper.
//
// Reassert the current surface after ANSI SGR resets, without replacing
// foreground colors or intentionally selected-row backgrounds.
const (
	canvasBackgroundSGR = "\x1b[48;2;16;21;34m" // colorCanvas
	panelBackgroundSGR  = "\x1b[48;2;26;34;50m" // colorPanel
	panelForegroundSGR  = "\x1b[38;2;228;237;247m" // colorText
	resetBackgroundSGR  = "\x1b[49m"
)

// Reuse the SGR replacers: a full dashboard redraw contains dozens of
// colored spans, and allocating a fresh replacer per panel row is unnecessary.
var (
	canvasSGRRestorer = strings.NewReplacer(
		"\x1b[0m", "\x1b[0m"+canvasBackgroundSGR,
		"\x1b[49m", "\x1b[49m"+canvasBackgroundSGR,
		"\x1b[m", "\x1b[m"+canvasBackgroundSGR,
	)
	panelSGRRestorer = strings.NewReplacer(
		"\x1b[0m", "\x1b[0m"+panelBackgroundSGR+panelForegroundSGR,
		"\x1b[49m", "\x1b[49m"+panelBackgroundSGR,
		"\x1b[39m", "\x1b[39m"+panelForegroundSGR,
		"\x1b[m", "\x1b[m"+panelBackgroundSGR+panelForegroundSGR,
	)
)

func reassertBackground(s, background string) string {
	switch background {
	case canvasBackgroundSGR:
		return canvasSGRRestorer.Replace(s)
	case panelBackgroundSGR:
		return panelSGRRestorer.Replace(s)
	default:
		return strings.NewReplacer(
			"\x1b[0m", "\x1b[0m"+background,
			"\x1b[49m", "\x1b[49m"+background,
			"\x1b[m", "\x1b[m"+background,
		).Replace(s)
	}
}

// Direct row painting avoids running an already fully sized/styled ANSI frame
// through Lip Gloss's general layout renderer again. Reassert the exact
// foreground/background after nested SGR resets to preserve the dark palette.
func paintPanelRows(s string) string {
	lines := strings.Split(s, "\n")
	var b strings.Builder
	b.Grow(len(s) + len(lines)*64)
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(panelBackgroundSGR)
		b.WriteString(panelForegroundSGR)
		b.WriteString(reassertBackground(line, panelBackgroundSGR))
		b.WriteString("\x1b[39m")
		b.WriteString(resetBackgroundSGR)
	}
	return b.String()
}

// paintCanvas writes only visible rows, padding to the requested terminal
// dimensions with explicit dark-background cells. This is much cheaper than
// laying out the entire, already sized dashboard with a second styling pass.
func paintCanvas(view string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
	var b strings.Builder
	b.Grow(len(view) + width*height/4)
	for row := 0; row < height; row++ {
		if row > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(canvasBackgroundSGR)
		if row < len(lines) {
			line := lines[row]
			b.WriteString(reassertBackground(line, canvasBackgroundSGR))
			padding := width - lipgloss.Width(line)
			if padding > 0 {
				b.WriteString(strings.Repeat(" ", padding))
			}
		} else {
			b.WriteString(strings.Repeat(" ", width))
		}
	}
	return b.String()
}
