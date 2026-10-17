package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

// dashboard is composed of fixed-height boxes so its contents never run below
// the footer. The only live data comes from the existing read-only snapshot.
func (m Model) dashboard(w, h int) string {
	if w < 90 || h < 23 {
		return m.compactDashboard(w, h)
	}
	topH := max(9, min(h-12, h*38/100))
	topLeft := max(50, w*68/100)
	topRight := w - topLeft - 1
	cpu := frame("CPU  /  DOCKER WORKLOADS", m.cpuPanel(topLeft-4, topH-2), topLeft, topH)
	engine := frame("ENGINE STATUS", m.enginePanel(topRight-4, topH-2), topRight, topH)
	top := lipgloss.JoinHorizontal(lipgloss.Top, cpu, " ", engine)
	bottomH := h - topH - 1
	leftW := max(36, w*43/100)
	rightW := w - leftW - 1
	memH := max(8, bottomH*58/100)
	ioH := bottomH - memH - 1
	mem := frame("MEMORY  /  LIVE HISTORY", m.memoryPanel(leftW-4, memH-2), leftW, memH)
	io := frame("NETWORK + DISK I/O", m.ioPanel(leftW-4, ioH-2), leftW, ioH)
	left := lipgloss.JoinVertical(lipgloss.Left, mem, "", io)
	listH := max(8, bottomH*58/100)
	detailH := bottomH - listH - 1
	containers := frame("CONTAINERS  /  "+fmt.Sprint(len(m.snapshot.Containers)), m.dashboardContainerRows(rightW-4, listH-2), rightW, listH)
	details := frame("SELECTED CONTAINER", m.compactContainerDetails(rightW-4, detailH-2), rightW, detailH)
	right := lipgloss.JoinVertical(lipgloss.Left, containers, "", details)
	return lipgloss.JoinVertical(lipgloss.Left, top, "", lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))
}

func (m Model) compactDashboard(w, h int) string {
	// Preserve a navigable container list on small terminals.
	if h < 12 {
		return frame("CONTAINERS", m.list(w-4, h-2), w, h)
	}
	summaryH := min(7, max(4, h/3))
	cpu, mem := m.snapshot.TotalUsage()
	summary := []string{cyan.Render(fmt.Sprintf("DOCKER  CPU %.1f%%  /  MEMORY %.1f MiB", cpu, mem/1048576))}
	running := 0
	for _, c := range m.snapshot.Containers {
		if c.State == "running" {
			running++
		}
	}
	summary = append(summary, fmt.Sprintf("Containers %d/%d  Images %d  Networks %d  Volumes %d", running, len(m.snapshot.Containers), len(m.snapshot.Images), len(m.snapshot.Networks), len(m.snapshot.Volumes)))
	summary = append(summary, green.Render(sparkline(m.cpuHistory, max(1, w-5), 10)))
	return lipgloss.JoinVertical(lipgloss.Left, frame("DOCKER METRICS", summary, w, summaryH), "", frame("CONTAINERS", m.list(w-4, max(1, h-summaryH-3)), w, h-summaryH-1))
}

func (m Model) cpuPanel(w, h int) []string {
	cpu, _ := m.snapshot.TotalUsage()
	peak := historyPeak(m.cpuHistory)
	caption := fmt.Sprintf("%s   %.1f%% total   •   peak %.1f%%   •   100%% = 1 CPU core", green.Render("● LIVE"), cpu, peak)
	lines := []string{caption}
	chartH := max(2, h-3)
	lines = append(lines, plotHistory(m.cpuHistory, max(12, w), chartH, 10, green)...)
	lines = append(lines, muted.Render("0% └  oldest"+strings.Repeat(" ", max(1, w-30))+"newest  /  5s per sample"))
	return lines
}
func (m Model) memoryPanel(w, h int) []string {
	_, mem := m.snapshot.TotalUsage()
	current := mem / 1048576
	lines := []string{fmt.Sprintf("%s  %.1f MiB  •  running containers only", cyan.Render("● LIVE"), current)}
	lines = append(lines, plotHistory(m.memHistory, max(12, w), max(1, h-3), 64, cyan)...)
	lines = append(lines, muted.Render("0 MiB  •  recent samples, not host RAM"))
	return lines
}

func (m Model) enginePanel(w, h int) []string {
	running := 0
	for _, c := range m.snapshot.Containers {
		if c.State == "running" {
			running++
		}
	}
	cpu, mem := m.snapshot.TotalUsage()
	contextName := m.snapshot.Context
	if contextName == "" {
		contextName = "connecting"
	}
	status := "● CONNECTED"
	color := green
	if m.busy {
		status = "◌ SYNCING"
		color = amber
	}
	if m.snapshot.Context == "" {
		status = "○ WAITING"
		color = muted
	}
	lines := []string{color.Render(status), "", muted.Render("DOCKER CONTEXT"), cyan.Render(plain(contextName, w)), ""}
	lines = append(lines, fmt.Sprintf("%s  %d/%d", green.Render("RUNNING"), running, len(m.snapshot.Containers)))
	lines = append(lines, metricBar(float64(running), float64(max(1, len(m.snapshot.Containers))), max(4, w-2), green))
	lines = append(lines, fmt.Sprintf("CPU  %6.1f%%     MEM  %.1f MiB", cpu, mem/1048576))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("IMAGES  %-5d  NETWORKS  %d", len(m.snapshot.Images), len(m.snapshot.Networks)))
	lines = append(lines, fmt.Sprintf("VOLUMES %-5d  MCP  read-only", len(m.snapshot.Volumes)))
	if !m.snapshot.Updated.IsZero() {
		lines = append(lines, "", muted.Render("SCAN  "+m.snapshot.Updated.Format("15:04:05")))
	}
	lines = append(lines, muted.Render("No host or VM telemetry implied"))
	return trimLines(lines, h)
}

func (m Model) ioPanel(w, h int) []string {
	var rx, tx, read, write float64
	for _, c := range m.snapshot.Containers {
		if c.State != "running" {
			continue
		}
		u := m.snapshot.Stats[c.ID]
		if u.ID == "" {
			u = m.snapshot.Stats[c.Names]
		}
		in, out := ioPair(u.NetIO)
		rx += in
		tx += out
		in, out = ioPair(u.BlockIO)
		read += in
		write += out
	}
	lines := []string{muted.Render("Container counters (cumulative, not rates)"), ""}
	lines = append(lines, fmt.Sprintf("NET  RX  %s", niceBytes(rx)))
	lines = append(lines, fmt.Sprintf("     TX  %s", niceBytes(tx)))
	lines = append(lines, metricBar(rx, math.Max(1, math.Max(rx, tx)), max(4, w-3), cyan))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("DISK READ   %s", niceBytes(read)))
	lines = append(lines, fmt.Sprintf("     WRITE  %s", niceBytes(write)))
	lines = append(lines, metricBar(read, math.Max(1, math.Max(read, write)), max(4, w-3), green))
	return trimLines(lines, h)
}

func (m Model) dashboardContainerRows(w, h int) []string {
	nameWidth := max(8, w-23)
	rows := []string{muted.Render(plain(padded("CONTAINER", nameWidth)+" CPU      MEMORY", w))}
	if len(m.snapshot.Containers) == 0 {
		return append(rows, muted.Render("No containers in this context"))
	}
	visible := max(1, h-1)
	start := max(0, m.selected-visible+1)
	for i := start; i < min(len(m.snapshot.Containers), start+visible); i++ {
		c := m.snapshot.Containers[i]
		u := m.snapshot.Stats[c.ID]
		if u.ID == "" {
			u = m.snapshot.Stats[c.Names]
		}
		cpu := u.CPUPerc
		if cpu == "" {
			cpu = "—"
		}
		memory := u.MemUsage
		if memory == "" {
			memory = "—"
		}
		if before, _, ok := strings.Cut(memory, " /"); ok {
			memory = before
		}
		nameWidth := max(8, w-23)
		symbol := "○"
		if c.State == "running" {
			symbol = "●"
		}
		label := fmt.Sprintf("%s %s %7s %9s", symbol, padded(c.Names, nameWidth-2), padded(cpu, 7), plain(memory, 9))
		if i == m.selected {
			rows = append(rows, selectedStyle.Render(padded(label, w)))
		} else if c.State == "running" {
			rows = append(rows, plain(label, w))
		} else {
			rows = append(rows, muted.Render(plain(label, w)))
		}
	}
	return rows
}
func (m Model) compactContainerDetails(w, h int) []string {
	if len(m.snapshot.Containers) == 0 || m.selected >= len(m.snapshot.Containers) {
		return []string{"Select a container"}
	}
	c := m.snapshot.Containers[m.selected]
	u := m.snapshot.Stats[c.ID]
	if u.ID == "" {
		u = m.snapshot.Stats[c.Names]
	}
	state := red
	if c.State == "running" {
		state = green
	}
	lines := []string{cyan.Render(plain(c.Names, w)), state.Render(strings.ToUpper(c.State)) + "  " + muted.Render(plain(c.Status, max(2, w-14)))}
	lines = append(lines, fmt.Sprintf("IMAGE  %s", plain(c.Image, max(2, w-8))))
	if c.Ports != "" {
		lines = append(lines, "PORTS  "+plain(c.Ports, max(2, w-8)))
	}
	cpu := dockercli.Percent(u.CPUPerc)
	lines = append(lines, fmt.Sprintf("CPU    %5.1f%% (1 core = 100%%)", cpu))
	lines = append(lines, metricBar(cpu, 100, max(3, w-2), green))
	lines = append(lines, "MEM    "+plain(u.MemUsage, max(1, w-7)))
	lines = append(lines, muted.Render("[2] manage  [l] logs  [Enter] inspect"))
	return trimLines(lines, h)
}
func ioPair(raw string) (float64, float64) {
	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		return 0, 0
	}
	return dockercli.ByteSize(strings.TrimSpace(parts[0])), dockercli.ByteSize(strings.TrimSpace(parts[1]))
}
func niceBytes(v float64) string {
	if v < 1024 {
		return fmt.Sprintf("%.0f B", v)
	}
	if v < 1048576 {
		return fmt.Sprintf("%.1f KiB", v/1024)
	}
	if v < 1073741824 {
		return fmt.Sprintf("%.1f MiB", v/1048576)
	}
	return fmt.Sprintf("%.2f GiB", v/1073741824)
}
func trimLines(lines []string, h int) []string {
	if h < 0 {
		return nil
	}
	if len(lines) > h {
		return lines[:h]
	}
	return lines
}
func metricBar(value, maximum float64, width int, color lipgloss.Style) string {
	width = max(0, width)
	if width == 0 {
		return ""
	}
	if maximum <= 0 {
		maximum = 1
	}
	count := int(math.Round(math.Max(0, math.Min(1, value/maximum)) * float64(width)))
	return color.Render(strings.Repeat("━", count)) + muted.Render(strings.Repeat("─", width-count))
}
func historyPeak(v []float64) float64 {
	peak := 0.0
	for _, s := range v {
		if s > peak && !math.IsNaN(s) && !math.IsInf(s, 0) {
			peak = s
		}
	}
	return peak
}
func niceScale(peak, minimum float64) float64 {
	peak = math.Max(minimum, peak)
	if peak <= 0 {
		return 1
	}
	p := math.Pow(10, math.Floor(math.Log10(peak)))
	for _, factor := range []float64{1, 2, 5, 10} {
		if p*factor >= peak {
			return p * factor
		}
	}
	return peak
}

// plotHistory draws accurate, autoscaled vertical bars. The axis shows the
// actual plotted ceiling (CPU may exceed 100% on multicore workloads).
func plotHistory(samples []float64, width, height int, minimum float64, color lipgloss.Style) []string {
	width = max(10, width)
	height = max(1, height)
	axis := 9
	plotWidth := max(1, width-axis)
	if len(samples) > plotWidth {
		samples = samples[len(samples)-plotWidth:]
	}
	ceiling := niceScale(historyPeak(samples), minimum)
	out := make([]string, 0, height)
	for row := 0; row < height; row++ {
		level := float64(height - row)
		label := "        "
		if row == 0 {
			label = fmt.Sprintf("%7.1f ", ceiling)
		}
		if row == height-1 {
			label = "   0.0  "
		}
		var bars strings.Builder
		bars.WriteString(strings.Repeat(" ", max(0, plotWidth-len(samples))))
		for _, raw := range samples {
			v := math.Max(0, math.Min(ceiling, raw))
			fraction := v/ceiling*float64(height) - (level - 1)
			symbols := []rune(" ▁▂▃▄▅▆▇█")
			index := max(0, min(8, int(math.Ceil(fraction*8))))
			bars.WriteRune(symbols[index])
		}
		out = append(out, muted.Render(label)+color.Render(bars.String()))
	}
	return out
}
