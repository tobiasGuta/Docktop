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
)

var (
	canvasSurface  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText)).Background(lipgloss.Color(colorCanvas))
	panelSurface   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText)).Background(lipgloss.Color(colorPanel))
	graphLowerFill = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGraphLow))
)

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
	return canvasSurface.Width(width).Height(height).Render(view)
}
