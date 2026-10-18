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
	resetBackgroundSGR  = "\x1b[49m"
)

func reassertBackground(s, background string) string {
	return strings.NewReplacer(
		"\x1b[0m", "\x1b[0m"+background,
		"\x1b[49m", "\x1b[49m"+background,
		"\x1b[m", "\x1b[m"+background,
	).Replace(s)
}

// An explicit background is applied per row, including padding spaces.
// A final default-background code prevents leaking the panel color into gaps.
func paintPanelRows(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = panelBackgroundSGR + reassertBackground(line, panelBackgroundSGR) + resetBackgroundSGR
	}
	return strings.Join(lines, "\n")
}

// paintCanvas covers blank terminal rows and columns in addition to visible
// widgets. It does not alter the widget text, size, or input behavior.
func paintCanvas(view string, width, height int) string {
	width = max(0, width)
	height = max(0, height)
	if width == 0 || height == 0 {
		return ""
	}
	// Do not rely on the emulator's configured background or opacity.
	// Lip Gloss pads to the terminal dimensions with explicitly styled cells.
	view = strings.TrimSuffix(view, "\n")
	return reassertBackground(canvasBackgroundSGR+canvasSurface.Width(width).Height(height).Render(view), canvasBackgroundSGR)
}
