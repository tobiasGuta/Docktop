package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

// dashboard uses bounded, responsive panels. Local host readings are
// explicitly separated from the Docker context workload snapshot.
func (m Model) dashboard(w, h int) string {
	if w < 90 || h < 30 {
		return m.compactDashboard(w, h)
	}
	topH := max(10, min(h-18, h*37/100))
	topLeft := max(50, w*68/100)
	topRight := w - topLeft - 1
	cpu := frame("CPU  /  DOCKER WORKLOADS", m.cpuPanel(topLeft-4, topH-2), topLeft, topH)
	engine := frame("LOCAL HOST  /  DOCKER", m.enginePanel(topRight-4, topH-2), topRight, topH)
	top := lipgloss.JoinHorizontal(lipgloss.Top, cpu, " ", engine)
	bottomH := h - topH - 1
	leftW := max(36, w*43/100)
	rightW := w - leftW - 1
	memH := max(8, bottomH*48/100)
	ioH := bottomH - memH - 1
	mem := frame("MEMORY  /  LIVE HISTORY", m.memoryPanel(leftW-4, memH-2), leftW, memH)
	io := frame("NETWORK + DISK I/O", m.ioPanel(leftW-4, ioH-2), leftW, ioH)
	left := lipgloss.JoinVertical(lipgloss.Left, mem, "", io)
	listH := max(8, bottomH*48/100)
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
	if m.host.MemoryTotal > 0 {
		summary = append(summary, fmt.Sprintf("HOST  CPU %.1f%%  RAM %.1f / %.1f GiB", m.host.CPUPercent, float64(m.host.MemoryUsed)/(1<<30), float64(m.host.MemoryTotal)/(1<<30)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, frame("DOCKER METRICS", summary, w, summaryH), "", frame("CONTAINERS", m.list(w-4, max(1, h-summaryH-3)), w, h-summaryH-1))
}

// historyFootnote distinguishes collected samples from empty chart space.
// Charts remain right-anchored rather than stretching short histories into
// fabricated data points.
func historyFootnote(samples int, unit string) string {
	count := min(max(0, samples), historyLimit)
	return fmt.Sprintf("◀ %dm window   •   %d/%d samples   •   latest ▶   •   %s",
		historyLimit*int(refreshInterval/time.Second)/60, count, historyLimit, unit)
}

func (m Model) cpuPanel(w, h int) []string {
	cpu, _ := m.snapshot.TotalUsage()
	peak := historyPeak(m.cpuHistory)
	caption := fmt.Sprintf("%s  %5.1f%%   %s peak %.1f%%   %s 100%% = one core",
		green.Render("● LIVE"), cpu, muted.Render("•"), peak, muted.Render("•"))
	lines := []string{caption}
	chartH := max(2, h-3)
	lines = append(lines, plotHistoryTones(m.cpuHistory, max(12, w), chartH, 10, green, graphCPULow, graphCPUMid)...)
	lines = append(lines, muted.Render(plain(historyFootnote(len(m.cpuHistory), "Docker CPU"), w)))
	return lines
}

func (m Model) memoryPanel(w, h int) []string {
	_, mem := m.snapshot.TotalUsage()
	current := mem / 1048576
	lines := []string{fmt.Sprintf("%s   %.1f MiB   %s running containers only",
		cyan.Render("● LIVE"), current, muted.Render("•"))}
	// An adaptive memory axis makes small workloads legible. Unlike a fixed
	// 64-MiB floor it can show 5–30 MiB workloads without flattening them.
	lines = append(lines, plotHistoryTones(m.memHistory, max(12, w), max(1, h-3), 8, cyan, graphLowerFill, graphMiddleFill)...)
	lines = append(lines, muted.Render(plain(historyFootnote(len(m.memHistory), "Docker memory"), w)))
	return lines
}

func (m Model) enginePanel(w, h int) []string {
	running := 0
	for _, c := range m.snapshot.Containers {
		if c.State == "running" {
			running++
		}
	}
	lines := []string{}
	if m.host.MemoryTotal > 0 {
		cpuText := "collecting"
		if m.host.CPUReady {
			cpuText = fmt.Sprintf("%.1f%%", m.host.CPUPercent)
		}
		lines = append(lines, cyan.Render("LOCAL LINUX HOST")+"  "+muted.Render("/proc"))
		lines = append(lines, fmt.Sprintf("CPU  %-9s%s", cpuText, muted.Render("all cores")))
		lines = append(lines, green.Render(sparkline(m.hostCPUHistory, max(6, w-3), 100)))
		lines = append(lines, fmt.Sprintf("RAM  %.1f / %.1f GiB", float64(m.host.MemoryUsed)/(1<<30), float64(m.host.MemoryTotal)/(1<<30)))
		lines = append(lines, metricBar(float64(m.host.MemoryUsed), float64(m.host.MemoryTotal), max(5, w-3), cyan))
		if m.host.GPU.Available {
			gpu := m.host.GPU
			label := fmt.Sprintf("GPU  %s %.0f%%", gpu.Name, gpu.Utilization)
			if gpu.MemoryTotal > 0 {
				label += fmt.Sprintf("  %.0f/%.0f MiB", float64(gpu.MemoryUsed)/(1<<20), float64(gpu.MemoryTotal)/(1<<20))
			}
			lines = append(lines, plain(label, w))
		} else {
			lines = append(lines, muted.Render("GPU  not reported (optional)"))
		}
	} else {
		lines = append(lines, amber.Render("LOCAL HOST  unavailable"))
		if m.hostError != "" {
			lines = append(lines, muted.Render(plain(m.hostError, w)))
		}
	}
	contextName := m.snapshot.Context
	if contextName == "" {
		contextName = "connecting"
	}
	status := green.Render("● READY")
	if m.busy {
		status = amber.Render("◌ SYNCING")
	}
	if m.snapshot.Context == "" {
		status = muted.Render("○ WAITING")
	}
	lines = append(lines, "", cyan.Render("DOCKER ENGINE")+"  "+status)
	lines = append(lines, muted.Render("CTX ")+plain(contextName, max(1, w-6)))
	lines = append(lines, fmt.Sprintf("RUNNING %d/%d   IMG %d   VOL %d", running, len(m.snapshot.Containers), len(m.snapshot.Images), len(m.snapshot.Volumes)))
	lines = append(lines, muted.Render("Local host ≠ Docker workload metrics"))
	return trimLines(lines, h)
}

func (m Model) ioPanel(w, h int) []string {
	lines := []string{muted.Render("Docker container I/O rates / second")}
	if !m.ioRates.Ready {
		return append(lines, "Collecting two matching samples…", muted.Render("No host network/disk telemetry"))
	}
	lines = append(lines, fmt.Sprintf("NET   ↓ %-13s ↑ %s", niceBytes(m.ioRates.RX)+"/s", niceBytes(m.ioRates.TX)+"/s"))
	if h >= 9 {
		lines = append(lines, plotHistoryTones(m.rxHistory, max(12, w), 2, 1, cyan, graphLowerFill, graphMiddleFill)...)
	} else if h >= 7 {
		lines = append(lines, cyan.Render(sparkline(m.rxHistory, max(1, w-3), max(1, historyPeak(m.rxHistory)))))
	}
	lines = append(lines, fmt.Sprintf("DISK  R %-13s W %s", niceBytes(m.ioRates.Read)+"/s", niceBytes(m.ioRates.Write)+"/s"))
	if h >= 9 {
		lines = append(lines, plotHistoryTones(m.diskHistory, max(12, w), 2, 1, green, graphCPULow, graphCPUMid)...)
	} else if h >= 8 {
		lines = append(lines, green.Render(sparkline(m.diskHistory, max(1, w-3), max(1, historyPeak(m.diskHistory)))))
	}
	lines = append(lines, muted.Render("~5s/sample  •  zero activity stays at baseline"))
	return trimLines(lines, h)
}

// cpuMeter is an instantaneous per-container CPU bar; it is not a history
// series. Values are capped at one core only for the bar, not the label.
func cpuMeter(percent float64, cells int) string {
	if cells <= 0 {
		return ""
	}
	count := int(math.Round(math.Min(100, math.Max(0, percent)) / 100 * float64(cells)))
	if percent > 0 && count == 0 {
		count = 1
	}
	return strings.Repeat("▰", count) + strings.Repeat("·", cells-count)
}

func (m Model) dashboardContainerRows(w, h int) []string {
	dense := w >= 65
	nameWidth := max(8, w-23)
	if dense {
		nameWidth = max(8, w-30)
	}
	header := padded("CONTAINER", nameWidth+2) + " CPU     MEMORY"
	if dense {
		header = padded("CONTAINER", nameWidth+2) + "ACTIVITY      CPU    MEMORY"
	}
	rows := []string{muted.Render(plain(header, w))}
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
		symbol := "○"
		if c.State == "running" {
			symbol = "●"
		}
		var label string
		if dense {
			meter := "        "
			if c.State == "running" && u.CPUPerc != "" {
				meter = cpuMeter(dockercli.Percent(u.CPUPerc), 8)
			}
			label = fmt.Sprintf("%s %s %8s %7s %9s", symbol,
				padded(c.Names, nameWidth), meter, plain(cpu, 7), plain(memory, 9))
		} else {
			label = fmt.Sprintf("%s %s %7s %9s", symbol,
				padded(c.Names, nameWidth-2), plain(cpu, 7), plain(memory, 9))
		}
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

