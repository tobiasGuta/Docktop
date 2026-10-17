package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

const refreshInterval = 5 * time.Second
const catalog = "mcp/docker-mcp-catalog:latest"

var (
	accent    = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	faint     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	good      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	danger    = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	highlight = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24"))
)

var tabs = []string{"Dashboard", "Containers", "Images", "Networks", "Volumes", "MCP Catalog"}

type Model struct {
	client                                   dockercli.Client
	snapshot                                 dockercli.Snapshot
	width, height                            int
	page, selected, scroll                   int
	busy, mcpBusy                            bool
	message                                  string
	mcpText                                  string
	mcpFilter                                string
	editingFilter                            bool
	confirmOperation, confirmID, confirmName string
	detailTitle, detailText                  string
	showHelp                                 bool
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
	return func() tea.Msg { s, err := client.Snapshot(context.Background()); return snapshotMsg{s, err} }
}
func loadMCP(client dockercli.Client, filter string) tea.Cmd {
	return func() tea.Msg { value, err := client.MCP(context.Background(), filter); return mcpMsg{value, err} }
}
func loadLogs(client dockercli.Client, id, name string) tea.Cmd {
	return func() tea.Msg { value, err := client.Logs(context.Background(), id); return logsMsg{name, value, err} }
}
func change(client dockercli.Client, op, id, name string) tea.Cmd {
	return func() tea.Msg { err := client.Change(context.Background(), op, id); return changedMsg{op, name, err} }
}
func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}
func (m Model) Init() tea.Cmd { return tea.Batch(poll(m.client), tick()) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case snapshotMsg:
		m.busy = false
		if msg.err != nil {
			m.message = "Docker unavailable: " + msg.err.Error()
		} else {
			m.snapshot = msg.snapshot
			if len(msg.snapshot.Warnings) > 0 {
				m.message = "Partial data: " + strings.Join(msg.snapshot.Warnings, " | ")
			}
		}
		m.clampSelection()
		return m, nil
	case mcpMsg:
		m.mcpBusy = false
		if msg.err != nil {
			m.mcpText = "Could not load MCP catalog: " + msg.err.Error() + "\n\nCheck `docker mcp --help` and that the catalog has been pulled."
		} else {
			m.mcpText = msg.value
		}
		m.scroll = 0
		return m, nil
	case logsMsg:
		if msg.err != nil {
			m.message = "Logs: " + msg.err.Error()
		} else {
			m.detailTitle = "Logs — " + msg.name
			m.detailText = msg.value
			m.scroll = 0
		}
		return m, nil
	case changedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("%s %s failed: %v", msg.operation, msg.name, msg.err)
		} else {
			m.message = fmt.Sprintf("%s: %s completed", msg.operation, msg.name)
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
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.confirmOperation != "" {
			if key == "y" || key == "Y" {
				op, id, name := m.confirmOperation, m.confirmID, m.confirmName
				m.confirmOperation, m.confirmID, m.confirmName = "", "", ""
				m.message = "Running " + op + " for " + name + "…"
				return m, change(m.client, op, id, name)
			}
			if key == "n" || key == "esc" {
				m.confirmOperation = ""
			}
			return m, nil
		}
		if m.editingFilter {
			switch key {
			case "enter":
				m.editingFilter = false
				m.mcpBusy = true
				return m, loadMCP(m.client, m.mcpFilter)
			case "esc":
				m.editingFilter = false
			case "backspace":
				if len(m.mcpFilter) > 0 {
					_, n := utf8.DecodeLastRuneInString(m.mcpFilter)
					m.mcpFilter = m.mcpFilter[:len(m.mcpFilter)-n]
				}
			default:
				if len(key) == 1 && len(m.mcpFilter) < 80 {
					m.mcpFilter += key
				}
			}
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
				if m.scroll > 0 {
					m.scroll--
				}
			case "down", "j":
				m.scroll++
			case "pgup":
				m.scroll -= 10
				if m.scroll < 0 {
					m.scroll = 0
				}
			case "pgdown":
				m.scroll += 10
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
			if m.page == 5 {
				return m, m.fetchMCP()
			}
		case "shift+tab", "left":
			m.switchPage((m.page + len(tabs) - 1) % len(tabs))
			if m.page == 5 {
				return m, m.fetchMCP()
			}
		case "1", "2", "3", "4", "5", "6":
			m.switchPage(int(key[0] - '1'))
			if m.page == 5 {
				return m, m.fetchMCP()
			}
		case "up", "k":
			if m.page == 5 {
				if m.scroll > 0 {
					m.scroll--
				}
			} else if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.page == 5 {
				m.scroll++
			} else if m.selected < m.rowCount()-1 {
				m.selected++
			}
		case "pgup":
			if m.page == 5 {
				m.scroll -= 10
				if m.scroll < 0 {
					m.scroll = 0
				}
			}
		case "pgdown":
			if m.page == 5 {
				m.scroll += 10
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
		return m, nil
	}
	return m, nil
}

func (m *Model) fetchMCP() tea.Cmd { m.mcpBusy = true; return loadMCP(m.client, m.mcpFilter) }
func (m *Model) switchPage(p int)  { m.page = p; m.selected = 0; m.scroll = 0; m.message = "" }
func (m *Model) ask(op string, c dockercli.Container) {
	m.confirmOperation = op
	m.confirmID = c.ID
	m.confirmName = c.Names
}
func (m *Model) clampSelection() {
	if m.selected >= m.rowCount() {
		m.selected = m.rowCount() - 1
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
	default:
		return 0
	}
}
func cut(s string, n int) string {
	s = dockercli.Clean(strings.ReplaceAll(s, "\n", " "), 400)
	runes := []rune(s)
	if n < 1 {
		return ""
	}
	if len(runes) > n {
		if n < 4 {
			return string(runes[:n])
		}
		return string(runes[:n-1]) + "…"
	}
	return s + strings.Repeat(" ", n-len(runes))
}
func (m Model) View() string {
	w := m.width
	if w < 50 {
		w = 50
	}
	h := m.height
	if h < 12 {
		h = 12
	}
	var b strings.Builder
	title := accent.Render("DOCKTOP") + "  " + faint.Render("Docker + MCP terminal dashboard")
	b.WriteString(title + "\n")
	ctx := m.snapshot.Context
	if ctx == "" {
		ctx = "connecting"
	}
	status := "Docker context: " + ctx
	if m.busy {
		status += "  |  updating"
	}
	if m.snapshot.Updated.IsZero() == false {
		status += "  |  last scan " + m.snapshot.Updated.Format("15:04:05")
	}
	b.WriteString(faint.Render(cut(status, w-2)) + "\n\n")
	for i, t := range tabs {
		label := fmt.Sprintf(" %d:%s ", i+1, t)
		if i == m.page {
			b.WriteString(highlight.Render(label))
		} else {
			b.WriteString(faint.Render(label))
		}
	}
	b.WriteString("\n\n")
	available := h - 11
	if available < 3 {
		available = 3
	}
	if m.showHelp {
		b.WriteString(accent.Render("KEYBOARD SHORTCUTS") + "\n")
		b.WriteString("1–6 / Tab  switch screens     j/k or ↑/↓  navigate\n")
		b.WriteString("r  refresh                    q / Ctrl-C  quit\n")
		b.WriteString("Containers: l logs, s start/stop, R restart (confirmation required)\n")
		b.WriteString("MCP: / filter names, Enter search, Esc cancel, PgUp/PgDn scroll\n")
		b.WriteString("Any key closes help. Actions target IDs in the active Docker context.\n")
	} else if m.confirmOperation != "" {
		b.WriteString(danger.Render("CONFIRM CONTAINER ACTION") + "\n\n")
		b.WriteString(fmt.Sprintf("%s container %s (%s)?\n", strings.ToUpper(m.confirmOperation), dockercli.Clean(m.confirmName, 100), m.confirmID))
		b.WriteString("Press y to execute or n / Esc to cancel.\n")
	} else if m.detailTitle != "" {
		b.WriteString(accent.Render(m.detailTitle) + " " + faint.Render("(Esc to return)") + "\n")
		b.WriteString(renderText(m.detailText, m.scroll, available, w-2))
	} else if m.page == 5 {
		b.WriteString(accent.Render("MCP CATALOG") + "  " + faint.Render(catalog) + "\n")
		filter := m.mcpFilter
		if filter == "" {
			filter = "all"
		}
		b.WriteString("Filter: " + cut(filter, 65) + " " + faint.Render("(/ to search)") + "\n")
		if m.editingFilter {
			b.WriteString(good.Render("Search> "+m.mcpFilter+"_") + "\n")
		}
		if m.mcpBusy {
			b.WriteString("Loading catalog…\n")
		} else {
			b.WriteString(renderText(m.mcpText, m.scroll, available-2, w-2))
		}
	} else {
		b.WriteString(m.renderPage(available, w-2))
	}
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(danger.Render(cut(m.message, w-2)) + "\n")
	}
	if m.page == 1 {
		b.WriteString(faint.Render("l logs  s start/stop  R restart  |  Every change asks for confirmation") + "\n")
	}
	b.WriteString(faint.Render("[1-6] Views  [Tab] Next  [↑↓] Navigate  [r] Refresh  [?] Help  [q] Quit"))
	return b.String()
}
func renderText(raw string, scroll, lines, width int) string {
	text := strings.Split(dockercli.Clean(raw, 50000), "\n")
	if scroll > len(text)-1 {
		scroll = len(text) - 1
	}
	if scroll < 0 {
		scroll = 0
	}
	if lines < 1 {
		lines = 1
	}
	end := scroll + lines
	if end > len(text) {
		end = len(text)
	}
	var b strings.Builder
	for _, line := range text[scroll:end] {
		b.WriteString(cut(line, width) + "\n")
	}
	return b.String()
}
func (m Model) renderPage(rows, width int) string {
	var b strings.Builder
	s := m.snapshot
	if m.page == 0 {
		running := 0
		for _, c := range s.Containers {
			if c.State == "running" {
				running++
			}
		}
		b.WriteString(accent.Render(fmt.Sprintf("CONTAINERS %d running / %d total    IMAGES %d    NETWORKS %d    VOLUMES %d", running, len(s.Containers), len(s.Images), len(s.Networks), len(s.Volumes))) + "\n\n")
		b.WriteString(faint.Render("Container overview • live CPU and memory from docker stats") + "\n")
	}
	var lines []string
	switch m.page {
	case 0, 1:
		for _, c := range s.Containers {
			u := s.Stats[c.ID]
			if u.ID == "" {
				u = s.Stats[c.Names]
			}
			cpu := u.CPUPerc
			if cpu == "" {
				cpu = "—"
			}
			mem := u.MemUsage
			if mem == "" {
				mem = "—"
			}
			lines = append(lines, fmt.Sprintf("%-18s %-18s %-26s %-8s %-17s", cut(c.Names, 18), cut(c.State, 18), cut(c.Image, 26), cut(cpu, 8), cut(mem, 17)))
		}
		b.WriteString(faint.Render("NAME               STATE              IMAGE                      CPU      MEMORY") + "\n")
	case 2:
		for _, v := range s.Images {
			lines = append(lines, fmt.Sprintf("%-36s %-22s %-20s", cut(v.Repository, 36), cut(v.Tag, 22), cut(v.Size, 20)))
		}
		b.WriteString(faint.Render("REPOSITORY                           TAG                    SIZE") + "\n")
	case 3:
		for _, v := range s.Networks {
			lines = append(lines, fmt.Sprintf("%-36s %-22s %-15s", cut(v.Name, 36), cut(v.Driver, 22), cut(v.Scope, 15)))
		}
		b.WriteString(faint.Render("NETWORK                              DRIVER                 SCOPE") + "\n")
	case 4:
		for _, v := range s.Volumes {
			lines = append(lines, fmt.Sprintf("%-45s %-25s", cut(v.Name, 45), cut(v.Driver, 25)))
		}
		b.WriteString(faint.Render("VOLUME                                        DRIVER") + "\n")
	}
	if len(lines) == 0 {
		b.WriteString(faint.Render("Nothing to display (or Docker is unavailable).") + "\n")
		return b.String()
	}
	count := rows
	if m.page == 0 {
		count -= 3
	}
	if count < 1 {
		count = 1
	}
	first := 0
	if m.selected >= count {
		first = m.selected - count + 1
	}
	end := first + count
	if end > len(lines) {
		end = len(lines)
	}
	for i := first; i < end; i++ {
		line := cut(lines[i], width-3)
		if i == m.selected {
			b.WriteString(highlight.Render("> "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	return b.String()
}
