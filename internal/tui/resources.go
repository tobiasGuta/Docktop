package tui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

func (m Model) engineStatus() string {
	switch {
	case m.scanError != "" && m.snapshot.Context != "":
		return amber.Render("● STALE")
	case m.scanError != "":
		return red.Render("○ OFFLINE")
	case m.snapshot.Context == "":
		return muted.Render("○ CONNECTING")
	case m.busy:
		return amber.Render("◌ SYNCING")
	case len(m.snapshot.Warnings) > 0:
		return amber.Render("● PARTIAL")
	default:
		return green.Render("● READY")
	}
}

func reviewLines(width int, fields ...string) []string {
	var lines []string
	for _, field := range fields {
		field = dockercli.Clean(strings.NewReplacer("\n", " ", "\t", " ").Replace(field), 2000)
		lines = append(lines, strings.Split(ansi.Hardwrap(field, max(1, width-2), true), "\n")...)
	}
	return lines
}

func (m Model) actionReviewLines() []string {
	subject := "Container: "
	if m.confirmOperation == "pull" {
		subject = "Image: "
	}
	fields := []string{"Operation: " + m.confirmOperation, subject + m.confirmName, "Context: " + m.confirmContext}
	if m.confirmID != "" {
		fields = append(fields, "ID: "+m.confirmID)
	}
	return reviewLines(m.width, fields...)
}

func (m Model) actionReviewFits(lines []string) bool {
	return m.width >= 30 && m.height >= 10 && len(lines) <= m.height-8
}

func (m Model) visibleContainers() []dockercli.Container {
	all := m.snapshot.Containers
	if m.page != 1 || (m.containerFilter == "" && m.composeProject == "") {
		return all
	}
	query := strings.ToLower(strings.TrimSpace(m.containerFilter))
	matched := make([]dockercli.Container, 0, len(all))
	for _, c := range all {
		if m.composeProject != "" && c.ComposeProject != m.composeProject {
			continue
		}
		if strings.Contains(strings.ToLower(c.Names), query) ||
			strings.Contains(strings.ToLower(c.Image), query) ||
			strings.Contains(strings.ToLower(c.ID), query) {
			matched = append(matched, c)
		}
	}
	return matched
}

func (m Model) selectedContainer() (dockercli.Container, bool) {
	if m.page == 0 || m.page == 1 {
		if containers := m.visibleContainers(); m.selected >= 0 && m.selected < len(containers) {
			return containers[m.selected], true
		}
	}
	return dockercli.Container{}, false
}

func (m Model) selectedContainerID() string {
	c, _ := m.selectedContainer()
	return c.ID
}

func (m *Model) restoreContainerSelection(id string) {
	if id == "" {
		return
	}
	for i, c := range m.visibleContainers() {
		if c.ID == id {
			m.selected = i
			return
		}
	}
}

func (m Model) containerSearchLabel() string {
	label := ""
	if m.composeProject != "" {
		label = "Project: " + m.composeProject + "  "
	}
	if m.containerEditing || m.containerFilter != "" {
		label += "Filter: " + m.containerFilter
		if m.containerEditing {
			label += "_"
		}
	}
	return label
}

func (m *Model) editContainerFilter(key tea.KeyMsg) {
	switch key.Type {
	case tea.KeyEnter, tea.KeyEsc:
		m.containerEditing = false
	default:
		updated := editSearch(m.containerFilter, key, 100)
		if updated != m.containerFilter {
			m.containerFilter = updated
			m.selected = 0
		}
	}
}

func editSearch(value string, key tea.KeyMsg, limit int) string {
	switch key.Type {
	case tea.KeyCtrlU:
		return ""
	case tea.KeyBackspace:
		runes := []rune(value)
		if len(runes) > 0 {
			return string(runes[:len(runes)-1])
		}
	case tea.KeySpace, tea.KeyRunes:
		for _, ch := range key.Runes {
			if unicode.IsPrint(ch) && !unicode.Is(unicode.Cf, ch) && len(value)+len(string(ch)) <= limit {
				value += string(ch)
			}
		}
	}
	return value
}

func (m *Model) clearComposeProject() {
	if m.composeProject != "" {
		m.containerFilter = m.previousContainerFilter
		m.composeProject = ""
	}
}
