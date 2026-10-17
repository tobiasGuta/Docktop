package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
	"github.com/tobiasGuta/Docktop/internal/mcpcatalog"
	"strings"
	"testing"
)

func testModel() Model {
	m := New()
	m.width = 110
	m.height = 32
	m.snapshot = dockercli.Snapshot{
		Context:    "default",
		Containers: []dockercli.Container{{ID: "0123456789abcdef", Names: "database-production", State: "running", Status: "Up 1 day", Image: "postgres:15.13-alpine"}},
		Stats:      map[string]dockercli.Usage{"0123456789abcdef": {ID: "0123456789abcdef", CPUPerc: "2.5%", MemUsage: "15MiB / 3GiB"}},
	}
	m.servers = []mcpcatalog.Server{{Name: "github-official", Title: "GitHub Official", Tools: 40}, {Name: "playwright", Title: "Playwright", Tools: 23}}
	m.mcpLoaded = true
	return m
}
func TestViewFitsTerminal(t *testing.T) {
	for _, dim := range [][2]int{{110, 32}, {80, 24}, {50, 16}, {30, 10}} {
		for page := 0; page < 8; page++ {
			m := testModel()
			m.width, m.height = dim[0], dim[1]
			m.page = page
			rendered := m.View()
			for i, line := range strings.Split(rendered, "\n") {
				if lipgloss.Width(line) > dim[0] {
					t.Errorf("page %d size %v line %d width %d: %q", page, dim, i, lipgloss.Width(line), line)
				}
			}
			if lipgloss.Height(rendered) > dim[1] {
				t.Errorf("page %d height %d > %d", page, lipgloss.Height(rendered), dim[1])
			}
		}
	}
}
func TestHistoryBounded(t *testing.T) {
	var values []float64
	for i := 0; i < 150; i++ {
		values = appendHistory(values, float64(i))
	}
	if len(values) != historyLimit || values[0] != 90 || values[len(values)-1] != 149 {
		t.Fatalf("history %v", values)
	}
}
func TestMCPFilteringAndSelection(t *testing.T) {
	m := testModel()
	m.page = 5
	m.mcpFilter = "play"
	if m.rowCount() != 1 {
		t.Fatal("search did not filter")
	}
	if !strings.Contains(strings.Join(m.details(50), " "), "Playwright") {
		t.Fatal("wrong selected details")
	}
}
func TestPlainRemovesEscapeCodes(t *testing.T) {
	if got := plain("abc\x1b[2Jdef", 10); got != "abcdef" {
		t.Fatalf("unsafe output: %q", got)
	}
}

func TestExtraPagesAndHubAdmission(t *testing.T) {
	m := testModel()
	m.page = 7
	m.hub = []dockercli.HubImage{{Name: "nginx", Description: "web server", StarCount: 100}}
	m.hubLoaded = true
	if m.rowCount() != 1 || !strings.Contains(strings.Join(m.details(70), " "), "web server") {
		t.Fatal("Hub browsing failed")
	}
	before, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	next := before.(Model)
	if next.confirmOperation != "pull" || next.confirmName != "nginx" {
		t.Fatal("pull must require confirmation")
	}
	before, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	next = before.(Model)
	if next.confirmOperation != "" {
		t.Fatal("confirmation did not cancel")
	}
	m.page = 6
	m.compose = []dockercli.ComposeProject{{Name: "web", Status: "running(1)"}}
	if m.rowCount() != 1 || !strings.Contains(strings.Join(m.details(70), " "), "web") {
		t.Fatal("compose browsing failed")
	}
}
func TestProfileToggle(t *testing.T) {
	m := testModel()
	m.page = 5
	m.profilesLoaded = true
	m.profilesText = "Profile: test"
	v, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	n := v.(Model)
	if !n.showProfiles || !strings.Contains(n.View(), "Profile: test") {
		t.Fatal("profile view not toggled")
	}
}
