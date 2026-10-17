package tui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
	"github.com/tobiasGuta/Docktop/internal/mcpcatalog"
)

const refreshInterval = 5 * time.Second
const catalog = "mcp/docker-mcp-catalog:latest"
const historyLimit = 60

var (
	cyan          = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	muted         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	green         = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	amber         = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	red           = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24"))
	edge          = lipgloss.NewStyle().Foreground(lipgloss.Color("61"))
)
var tabs = []string{"Dashboard", "Containers", "Images", "Networks", "Volumes", "MCP Catalog"}
var shortTabs = []string{"Home", "Ctrs", "Img", "Net", "Vol", "MCP"}

type Model struct {
	client                                   dockercli.Client
	snapshot                                 dockercli.Snapshot
	width, height                            int
	page, selected, scroll                   int
	busy, mcpBusy, mcpLoaded                 bool
	message                                  string
	mcpText, mcpFilter                       string
	servers                                  []mcpcatalog.Server
	editingFilter                            bool
	confirmOperation, confirmID, confirmName string
	detailTitle, detailText                  string
	showHelp, showItem                       bool
	cpuHistory, memHistory                   []float64
}

type snapshotMsg struct {
	snapshot dockercli.Snapshot
	err      error
}
type mcpMsg struct {
	value string
	err   error
}
type logsMsg struct {
	name, value string
	err         error
}
type changedMsg struct {
	operation, name string
	err             error
}
type tickMsg time.Time

func New() Model { return Model{client: dockercli.New(), width: 100, height: 30, busy: true} }
func poll(client dockercli.Client) tea.Cmd {
	return func() tea.Msg { s, e := client.Snapshot(context.Background()); return snapshotMsg{s, e} }
}
func loadMCP(client dockercli.Client) tea.Cmd {
	return func() tea.Msg { s, e := client.MCP(context.Background(), ""); return mcpMsg{s, e} }
}
func loadLogs(client dockercli.Client, id, name string) tea.Cmd {
	return func() tea.Msg { s, e := client.Logs(context.Background(), id); return logsMsg{name, s, e} }
}
func change(client dockercli.Client, op, id, name string) tea.Cmd {
	return func() tea.Msg { e := client.Change(context.Background(), op, id); return changedMsg{op, name, e} }
}
func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}
func (m Model) Init() tea.Cmd { return tea.Batch(poll(m.client), tick()) }

func appendHistory(history []float64, sample float64) []float64 {
	history = append(history, sample)
	if len(history) > historyLimit {
		history = history[len(history)-historyLimit:]
	}
	return history
}
func (m *Model) filteredServers() []mcpcatalog.Server {
	return mcpcatalog.Filter(m.servers, m.mcpFilter)
}
func (m *Model) clampSelection() {
	if m.selected >= m.rowCount() {
		m.selected = max(0, m.rowCount()-1)
	}
	if m.selected < 0 {
		m.selected = 0
	}
}
func (m Model) rowCount() int {
	switch m.page {
	case 0, 1:
		return len(m.snapshot.Containers)
	case 2:
		return len(m.snapshot.Images)
	case 3:
		return len(m.snapshot.Networks)
	case 4:
		return len(m.snapshot.Volumes)
	case 5:
		return len(mcpcatalog.Filter(m.servers, m.mcpFilter))
	}
	return 0
}
func (m *Model) switchPage(p int) {
	m.page = p
	m.selected = 0
	m.scroll = 0
	m.message = ""
	m.showItem = false
}
func (m *Model) fetchMCP() tea.Cmd {
	if m.mcpBusy {
		return nil
	}
	m.mcpBusy = true
	return loadMCP(m.client)
}
func (m *Model) ask(op string, c dockercli.Container) {
	m.confirmOperation, m.confirmID, m.confirmName = op, c.ID, c.Names
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
	case snapshotMsg:
		m.busy = false
		if v.err != nil {
			m.message = "Docker unavailable: " + v.err.Error()
		} else {
			if m.snapshot.Context != "" && v.snapshot.Context != m.snapshot.Context {
				m.cpuHistory, m.memHistory = nil, nil
			}
			m.snapshot = v.snapshot
			if len(v.snapshot.Warnings) > 0 {
				m.message = "Partial data: " + strings.Join(v.snapshot.Warnings, " | ")
			}
			if len(v.snapshot.Stats) > 0 {
				cpu, bytes := v.snapshot.TotalUsage()
				m.cpuHistory = appendHistory(m.cpuHistory, cpu)
				m.memHistory = appendHistory(m.memHistory, bytes/1024/1024)
			}
		}
		m.clampSelection()
	case mcpMsg:
		m.mcpBusy = false
		if v.err != nil {
			m.mcpText = "Could not load MCP catalog: " + v.err.Error() + ". Check `docker mcp --help` and pull the catalog."
		} else {
			m.mcpText = v.value
			m.servers = mcpcatalog.Parse(v.value)
			m.mcpLoaded = true
			if len(m.servers) == 0 {
				m.message = "Catalog returned no parseable servers"
			}
		}
		m.clampSelection()
	case logsMsg:
		if v.err != nil {
			m.message = "Logs: " + v.err.Error()
		} else {
			m.detailTitle = "LOGS / " + v.name
			m.detailText = v.value
			m.scroll = 0
		}
	case changedMsg:
		if v.err != nil {
			m.message = fmt.Sprintf("%s %s failed: %v", v.operation, v.name, v.err)
		} else {
			m.message = fmt.Sprintf("%s: %s completed", v.operation, v.name)
		}
		m.busy = true
		return m, poll(m.client)
	case tickMsg:
		cmds := []tea.Cmd{tick()}
		if !m.busy {
			m.busy = true
			cmds = append(cmds, poll(m.client))
		}
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
		key := v.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.confirmOperation != "" {
			if key == "y" || key == "Y" {
				op, id, name := m.confirmOperation, m.confirmID, m.confirmName
				m.confirmOperation, m.confirmID, m.confirmName = "", "", ""
				m.message = "Running " + op + " for " + name
				return m, change(m.client, op, id, name)
			}
			if key == "n" || key == "esc" {
				m.confirmOperation, m.confirmID, m.confirmName = "", "", ""
			}
			return m, nil
		}
		if m.editingFilter {
			switch key {
			case "enter":
				m.editingFilter = false
			case "esc":
				m.editingFilter = false
			case "backspace":
				if len(m.mcpFilter) > 0 {
					_, n := utf8.DecodeLastRuneInString(m.mcpFilter)
					m.mcpFilter = m.mcpFilter[:len(m.mcpFilter)-n]
				}
			case "ctrl+u":
				m.mcpFilter = ""
			default:
				if len(key) == 1 && len(m.mcpFilter) < 80 {
					m.mcpFilter += key
				}
			}
			m.selected = 0
			return m, nil
		}
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if m.detailTitle != "" {
			switch key {
			case "esc", "q":
				m.detailTitle, m.detailText = "", ""
				m.scroll = 0
			case "up", "k":
				m.scroll = max(0, m.scroll-1)
			case "down", "j":
				m.scroll++
			case "pgup":
				m.scroll = max(0, m.scroll-10)
			case "pgdown":
				m.scroll += 10
			}
			return m, nil
		}
		if m.showItem {
			if key == "enter" || key == "esc" || key == "q" {
				m.showItem = false
			}
			return m, nil
		}
		switch key {
		case "q":
			return m, tea.Quit
		case "?":
			m.showHelp = true
		case "tab", "right":
			m.switchPage((m.page + 1) % len(tabs))
			if m.page == 5 && !m.mcpLoaded {
				return m, m.fetchMCP()
			}
		case "shift+tab", "left":
			m.switchPage((m.page + len(tabs) - 1) % len(tabs))
			if m.page == 5 && !m.mcpLoaded {
				return m, m.fetchMCP()
			}
		case "1", "2", "3", "4", "5", "6":
			m.switchPage(int(key[0] - '1'))
			if m.page == 5 && !m.mcpLoaded {
				return m, m.fetchMCP()
			}
		case "up", "k":
			m.selected = max(0, m.selected-1)
		case "down", "j":
			m.selected = min(m.rowCount()-1, m.selected+1)
			m.clampSelection()
		case "pgup":
			m.selected = max(0, m.selected-10)
		case "pgdown":
			m.selected = min(m.rowCount()-1, m.selected+10)
			m.clampSelection()
		case "home":
			m.selected = 0
		case "end":
			m.selected = max(0, m.rowCount()-1)
		case "enter":
			if m.rowCount() > 0 {
				m.showItem = true
			}
		case "r":
			if m.page == 5 {
				return m, m.fetchMCP()
			}
			if !m.busy {
				m.busy = true
				return m, poll(m.client)
			}
		case "/":
			if m.page == 5 {
				m.editingFilter = true
			}
		case "l":
			if m.page == 1 && m.selected < len(m.snapshot.Containers) {
				c := m.snapshot.Containers[m.selected]
				m.message = "Loading logs for " + c.Names
				return m, loadLogs(m.client, c.ID, c.Names)
			}
		case "s":
			if m.page == 1 && m.selected < len(m.snapshot.Containers) {
				c := m.snapshot.Containers[m.selected]
				op := "start"
				if c.State == "running" {
					op = "stop"
				}
				m.ask(op, c)
			}
		case "R":
			if m.page == 1 && m.selected < len(m.snapshot.Containers) {
				m.ask("restart", m.snapshot.Containers[m.selected])
			}
		}
	}
	return m, nil
}

// plain keeps untrusted Docker data out of terminal control sequences and
// handles terminal-cell widths, not just UTF-8 byte lengths.
func plain(input string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	input = dockercli.Clean(strings.ReplaceAll(strings.ReplaceAll(input, "\n", " "), "\t", "    "), 10000)
	var b strings.Builder
	width := 0
	for _, r := range input {
		rw := lipgloss.Width(string(r))
		if width+rw > maxWidth {
			break
		}
		b.WriteRune(r)
		width += rw
	}
	if lipgloss.Width(input) > maxWidth && maxWidth >= 2 {
		out := []rune(b.String())
		for len(out) > 0 && lipgloss.Width(string(out))+1 > maxWidth {
			out = out[:len(out)-1]
		}
		return string(out) + "…"
	}
	return b.String()
}
func padded(value string, width int) string {
	if width < 0 {
		width = 0
	}
	value = plain(value, width)
	return value + strings.Repeat(" ", max(0, width-lipgloss.Width(value)))
}
func wrap(value string, width, lines int) []string {
	value = dockercli.Clean(value, 5000)
	value = strings.ReplaceAll(value, "\t", "    ")
	out := []string{}
	for _, para := range strings.Split(value, "\n") {
		remaining := strings.TrimSpace(para)
		for len(remaining) > 0 && len(out) < lines {
			if lipgloss.Width(remaining) <= width {
				out = append(out, remaining)
				remaining = ""
				break
			}
			// Use cell-aware clipping; preserve full values across lines.
			line := plain(remaining, width)
			if strings.HasSuffix(line, "…") {
				line = strings.TrimSuffix(line, "…")
			}
			if line == "" {
				break
			}
			out = append(out, line)
			remaining = strings.TrimSpace(strings.TrimPrefix(remaining, line))
		}
		if len(out) >= lines {
			break
		}
	}
	return out
}
func frame(title string, content []string, width, height int) string {
	width = max(12, width)
	height = max(3, height)
	inner := width - 2
	title = plain(title, max(0, inner-3))
	used := lipgloss.Width(title) + 2
	var b strings.Builder
	b.WriteString(edge.Render("╭─ ") + cyan.Render(title) + edge.Render(strings.Repeat("─", max(0, inner-used))+"╮") + "\n")
	for i := 0; i < height-2; i++ {
		line := ""
		if i < len(content) {
			line = content[i]
		}
		// Styled lines come only from our own renderer; all Docker text was sanitized.
		if lipgloss.Width(line) > inner {
			line = lipgloss.NewStyle().MaxWidth(inner).Render(line)
		}
		b.WriteString(edge.Render("│") + line + strings.Repeat(" ", max(0, inner-lipgloss.Width(line))) + edge.Render("│"))
		if i < height-3 {
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n" + edge.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return b.String()
}
func sparkline(samples []float64, cells int, minCeiling float64) string {
	if cells <= 0 {
		return ""
	}
	blocks := []rune("▁▂▃▄▅▆▇█")
	if len(samples) > cells {
		samples = samples[len(samples)-cells:]
	}
	top := minCeiling
	for _, v := range samples {
		top = math.Max(top, v)
	}
	if top <= 0 {
		top = 1
	}
	var b strings.Builder
	b.WriteString(strings.Repeat("·", max(0, cells-len(samples))))
	for _, v := range samples {
		idx := min(7, max(0, int(math.Round(v/top*7))))
		b.WriteRune(blocks[idx])
	}
	return b.String()
}
func (m Model) statusLine(w int) string {
	ctx := m.snapshot.Context
	if ctx == "" {
		ctx = "connecting"
	}
	label := "context: " + ctx
	if m.busy {
		label += "  •  syncing"
	} else {
		label += "  •  ready"
	}
	if !m.snapshot.Updated.IsZero() {
		label += "  •  last scan " + m.snapshot.Updated.Format("15:04:05")
	}
	return muted.Render(plain(label, w))
}
func (m Model) navigation(w int) string {
	if w < 52 {
		return selectedStyle.Render(fmt.Sprintf(" %d/%d %s ", m.page+1, len(tabs), tabs[m.page])) + muted.Render("  [Tab] next")
	}
	names := tabs
	if w < 95 {
		names = shortTabs
	}
	var b strings.Builder
	for i, label := range names {
		item := fmt.Sprintf(" %d:%s ", i+1, label)
		if i == m.page {
			b.WriteString(selectedStyle.Render(item))
		} else {
			b.WriteString(muted.Render(item))
		}
	}
	return b.String()
}
func (m Model) footer(w int) string {
	help := "[1–6] Views  [Tab] Next  [↑↓/Pg] Navigate  [Enter] Details  [r] Refresh  [?] Help  [q] Quit"
	if m.page == 1 {
		help = "[l] Logs  [s] Start/Stop  [R] Restart  [Enter] Details  [?] Help  [q] Quit"
	}
	if m.page == 5 {
		help = "[/] Search  [↑↓/Pg] Browse  [Enter] Details  [r] Reload  [?] Help  [q] Quit"
	}
	if w < 80 {
		help = "[1–6] Views  [↑↓] Select  [Enter] Details  [?] Help  [q] Quit"
	}
	return muted.Render(plain(help, w))
}
func (m Model) View() string {
	w, h := m.width, m.height
	if w < 30 || h < 10 {
		return cyan.Render("DOCKTOP") + " — enlarge terminal (at least 30x10)"
	}
	var b strings.Builder
	b.WriteString(cyan.Render("DOCKTOP") + "  " + muted.Render(plain("Docker Engine + MCP Toolkit", max(0, w-12))) + "\n")
	b.WriteString(m.statusLine(w) + "\n")
	b.WriteString(m.navigation(w) + "\n")
	available := max(3, h-5)
	switch {
	case m.showHelp:
		b.WriteString(frame("KEYBOARD SHORTCUTS", []string{
			"1–6 / Tab  Switch views     ↑↓ / j k / PgUp PgDn  Select and scroll",
			"Enter  Item details       r  Refresh current data       q  Quit",
			"Containers: l  Last 100 log lines; s  Start/stop; R  Restart",
			"Mutating actions always require explicit y confirmation.",
			"MCP: /  Live local search; Ctrl+U  Clear search; r  Reload catalog",
			"Esc  Close details / prompts; actions target the current Docker context.",
			"No MCP servers are started or installed by this application.",
		}, w, available))
	case m.confirmOperation != "":
		lines := []string{red.Render("Container state change requires confirmation"), "", "Operation: " + m.confirmOperation}
		lines = append(lines, wrap("Container: "+m.confirmName, max(10, w-4), 3)...)
		lines = append(lines, "ID: "+plain(m.confirmID, w-6), "", "Press y to execute, n or Esc to cancel.")
		b.WriteString(frame("CONFIRM ACTION", lines, w, available))
	case m.detailTitle != "":
		rows := m.logLines(max(1, available-2), max(8, w-4))
		b.WriteString(frame(m.detailTitle+"  (Esc to close)", rows, w, available))
	case m.showItem:
		b.WriteString(frame("DETAILS  (Enter/Esc to return)", m.details(w-4), w, available))
	default:
		b.WriteString(m.body(w, available))
	}
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(amber.Render(plain(m.message, w)) + "\n")
	} else {
		b.WriteString("\n")
	}
	b.WriteString(m.footer(w))
	return b.String()
}
func (m Model) logLines(limit, width int) []string {
	lines := strings.Split(dockercli.Clean(m.detailText, 18000), "\n")
	if len(lines) == 0 {
		return []string{"(no logs)"}
	}
	scroll := min(max(0, m.scroll), max(0, len(lines)-limit))
	out := []string{}
	for _, s := range lines[scroll:min(len(lines), scroll+limit)] {
		out = append(out, plain(s, width))
	}
	return out
}
func (m Model) body(w, h int) string {
	if m.page == 0 {
		return m.dashboard(w, h)
	}
	if w < 82 {
		return frame(tabs[m.page], m.list(w-4, h-2), w, h)
	}
	left := max(34, w*55/100)
	right := w - left - 1
	a := frame(tabs[m.page]+m.countSuffix(), m.list(left-4, h-2), left, h)
	title := "SELECTED ITEM"
	if m.page == 5 {
		title = "MCP SERVER DETAILS"
	}
	d := frame(title, m.details(right-4), right, h)
	return lipgloss.JoinHorizontal(lipgloss.Top, a, " ", d)
}
func (m Model) dashboard(w, h int) string {
	running := 0
	for _, c := range m.snapshot.Containers {
		if c.State == "running" {
			running++
		}
	}
	cpu, mem := m.snapshot.TotalUsage()
	summary := []string{
		cyan.Render(plain(fmt.Sprintf(" CONTAINERS %d/%d running   IMAGES %d   NETWORKS %d   VOLUMES %d", running, len(m.snapshot.Containers), len(m.snapshot.Images), len(m.snapshot.Networks), len(m.snapshot.Volumes)), w-4)),
	}
	overviewHeight := 4
	graphHeight := 8
	if h < 18 {
		graphHeight = 6
	}
	if h < 14 {
		overviewHeight = 3
		graphHeight = 5
	}
	rest := h - overviewHeight - graphHeight - 2
	if rest < 3 {
		return frame("DOCKER OVERVIEW", append(summary, m.list(w-4, h-5)...), w, h)
	}
	top := frame("DOCKER OVERVIEW", summary, w, overviewHeight)
	graphWidth := max(7, w-32)
	if graphWidth > w-5 {
		graphWidth = w - 5
	}
	graphs := []string{
		fmt.Sprintf("CPU sum: %.1f%%  (100%% = one core)", cpu),
		green.Render(sparkline(m.cpuHistory, graphWidth, 100)),
		fmt.Sprintf("Memory in running containers: %.1f MiB", mem/1024/1024),
		cyan.Render(sparkline(m.memHistory, graphWidth, 64)),
		muted.Render("Rolling history: last 60 successful samples, every ~5 seconds"),
	}
	center := frame("LIVE RESOURCE HISTORY", graphs, w, graphHeight)
	bottom := ""
	if w < 82 {
		bottom = frame("CONTAINERS", m.list(w-4, rest-2), w, rest)
	} else {
		left := max(34, w*55/100)
		right := w - left - 1
		bottom = lipgloss.JoinHorizontal(lipgloss.Top,
			frame("CONTAINERS", m.list(left-4, rest-2), left, rest), " ",
			frame("SELECTED CONTAINER", m.details(right-4), right, rest))
	}
	return lipgloss.JoinVertical(lipgloss.Left, top, "", center, "", bottom)
}
func (m Model) countSuffix() string {
	if m.page == 5 {
		return fmt.Sprintf("  (%d/%d)", len(m.filteredServers()), len(m.servers))
	}
	return fmt.Sprintf("  (%d)", m.rowCount())
}
func (m Model) list(width, height int) []string {
	width = max(9, width)
	height = max(1, height)
	if m.page == 5 {
		return m.mcpList(width, height)
	}
	if m.rowCount() == 0 {
		return []string{muted.Render("No resources found (or Docker unavailable).")}
	}
	lines := []string{}
	nameWidth := max(12, width-22)
	if width < 30 {
		nameWidth = width - 4
	}
	switch m.page {
	case 0, 1:
		lines = append(lines, muted.Render(padded("CONTAINER", nameWidth)+"  CPU     MEMORY"))
	case 2:
		lines = append(lines, muted.Render(padded("IMAGE", nameWidth)+"  SIZE"))
	case 3:
		lines = append(lines, muted.Render(padded("NETWORK", nameWidth)+"  DRIVER"))
	case 4:
		lines = append(lines, muted.Render(padded("VOLUME", nameWidth)+"  DRIVER"))
	}
	visible := max(1, height-1)
	start := max(0, m.selected-visible+1)
	for i := start; i < min(m.rowCount(), start+visible); i++ {
		var line string
		switch m.page {
		case 0, 1:
			c := m.snapshot.Containers[i]
			u := m.snapshot.Stats[c.ID]
			if u.ID == "" {
				u = m.snapshot.Stats[c.Names]
			}
			cpu, mem := u.CPUPerc, u.MemUsage
			if cpu == "" {
				cpu = "—"
			}
			if mem == "" {
				mem = "—"
			}
			if before, _, ok := strings.Cut(mem, " /"); ok {
				mem = before
			}
			state := "○ "
			if c.State == "running" {
				state = "● "
			}
			line = state + padded(c.Names, max(1, nameWidth-2)) + " " + padded(cpu, 7) + " " + plain(mem, 10)
		case 2:
			img := m.snapshot.Images[i]
			line = padded(img.Repository+":"+img.Tag, nameWidth) + "  " + plain(img.Size, 12)
		case 3:
			n := m.snapshot.Networks[i]
			line = padded(n.Name, nameWidth) + "  " + plain(n.Driver, 12)
		case 4:
			v := m.snapshot.Volumes[i]
			line = padded(v.Name, nameWidth) + "  " + plain(v.Driver, 12)
		}
		lines = append(lines, m.formatRow(i, line, width))
	}
	return lines
}
func (m Model) formatRow(index int, line string, width int) string {
	line = padded(line, width)
	if index == m.selected {
		return selectedStyle.Render(line)
	}
	return line
}
func (m Model) mcpList(width, height int) []string {
	var lines []string
	filter := m.mcpFilter
	if m.editingFilter {
		filter += "_"
	}
	if filter == "" {
		filter = "all servers"
	}
	lines = append(lines, muted.Render(plain("Search: "+filter+"  (/ to edit)", width)))
	if m.mcpBusy {
		return append(lines, "Loading MCP catalog…")
	}
	filtered := m.filteredServers()
	if len(filtered) == 0 {
		if !m.mcpLoaded {
			return append(lines, "Catalog not loaded yet.")
		}
		if len(m.servers) == 0 {
			return append(lines, wrap(m.mcpText, width, min(3, height-1))...)
		}
		return append(lines, "No servers match your search.")
	}
	visible := max(1, height-1)
	start := max(0, m.selected-visible+1)
	for i := start; i < min(len(filtered), start+visible); i++ {
		s := filtered[i]
		label := s.Name
		if s.Tools > 0 {
			label += fmt.Sprintf("  (%d tools)", s.Tools)
		}
		lines = append(lines, m.formatRow(i, label, width))
	}
	return lines
}
func (m Model) details(width int) []string {
	width = max(10, width)
	var entries [][2]string
	switch m.page {
	case 0, 1:
		if m.selected >= len(m.snapshot.Containers) {
			return []string{"Select a container to inspect."}
		}
		c := m.snapshot.Containers[m.selected]
		u := m.snapshot.Stats[c.ID]
		if u.ID == "" {
			u = m.snapshot.Stats[c.Names]
		}
		entries = [][2]string{{"NAME", c.Names}, {"ID", c.ID}, {"STATE", c.State}, {"STATUS", c.Status}, {"IMAGE", c.Image}, {"PORTS", c.Ports}, {"CPU", u.CPUPerc}, {"MEMORY", u.MemUsage}, {"NETWORK I/O", u.NetIO}, {"BLOCK I/O", u.BlockIO}}
	case 2:
		if m.selected >= len(m.snapshot.Images) {
			return []string{"Select an image to inspect."}
		}
		v := m.snapshot.Images[m.selected]
		entries = [][2]string{{"REPOSITORY", v.Repository}, {"TAG", v.Tag}, {"ID", v.ID}, {"SIZE", v.Size}}
	case 3:
		if m.selected >= len(m.snapshot.Networks) {
			return []string{"Select a network to inspect."}
		}
		v := m.snapshot.Networks[m.selected]
		entries = [][2]string{{"NETWORK", v.Name}, {"ID", v.ID}, {"DRIVER", v.Driver}, {"SCOPE", v.Scope}}
	case 4:
		if m.selected >= len(m.snapshot.Volumes) {
			return []string{"Select a volume to inspect."}
		}
		v := m.snapshot.Volumes[m.selected]
		entries = [][2]string{{"VOLUME NAME", v.Name}, {"DRIVER", v.Driver}, {"MOUNTPOINT", v.Mountpoint}}
	case 5:
		servers := m.filteredServers()
		if m.selected >= len(servers) {
			return []string{muted.Render("Choose an MCP server to inspect.")}
		}
		v := servers[m.selected]
		entries = [][2]string{{"NAME", v.Name}, {"TITLE", v.Title}, {"DESCRIPTION", v.Description}, {"TYPE", v.Type}, {"TOOLS", fmt.Sprint(v.Tools)}, {"IMAGE", v.Image}, {"ENDPOINT", v.Endpoint}}
	}
	lines := []string{}
	for _, entry := range entries {
		if entry[1] == "" {
			continue
		}
		lines = append(lines, cyan.Render(entry[0]))
		lines = append(lines, wrap(entry[1], width, 4)...)
		lines = append(lines, "")
	}
	if m.page == 1 {
		lines = append(lines, muted.Render("l: logs    s: start/stop    R: restart"))
		lines = append(lines, muted.Render("State changes require y confirmation."))
	}
	if m.page == 5 {
		lines = append(lines, muted.Render("Catalog listing only. No MCP server is executed."))
	}
	return lines
}
