package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
	"github.com/tobiasGuta/Docktop/internal/hostmetrics"
	"github.com/tobiasGuta/Docktop/internal/mcpcatalog"
	"strings"
	"fmt"
	"testing"
	"time"
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

func TestBtopDashboardResponsiveAndBounded(t *testing.T) {
	for _, dim := range [][2]int{{172, 52}, {120, 35}, {90, 23}, {89, 23}, {50, 16}, {30, 10}} {
		m := testModel()
		m.width = dim[0]
		m.height = dim[1]
		m.cpuHistory = []float64{2.5, 8.2, 3.4, 11.7, 19.0}
		m.memHistory = []float64{15, 16, 15.5, 20}
		rendered := m.View()
		if !strings.Contains(rendered, "DOCKTOP") {
			t.Fatalf("no header at %v", dim)
		}
		for i, line := range strings.Split(rendered, "\n") {
			if lipgloss.Width(line) > dim[0] {
				t.Errorf("size %v line %d too wide: %d", dim, i, lipgloss.Width(line))
			}
		}
		if lipgloss.Height(rendered) > dim[1] {
			t.Errorf("size %v too tall: %d", dim, lipgloss.Height(rendered))
		}
	}
}
func TestHistoryScaleKeepsTruePeak(t *testing.T) {
	if niceScale(275, 10) < 275 {
		t.Fatal("clipped multicore CPU")
	}
	if niceScale(4.7, 10) != 10 {
		t.Fatal("low workload should have readable scaling")
	}
	graph := plotHistory([]float64{0, 1, 7, 14}, 44, 8, 10, green)
	if len(graph) != 8 || !strings.Contains(graph[0], "20.0") {
		t.Fatalf("unexpected graph %v", graph)
	}
	if strings.Join(graph, "") == "" {
		t.Fatal("empty plot")
	}
}
func TestIOCountersAreNotRates(t *testing.T) {
	in, out := ioPair("1.1MB / 500kB")
	if in != 1.1e6 || out != 500e3 {
		t.Fatalf("I/O parsing: %f %f", in, out)
	}
}

func TestHostReadingsAreDistinctFromDockerMetrics(t *testing.T) {
	m := testModel()
	first := hostMsg{snapshot: hostmetrics.Snapshot{CPUCounters: hostmetrics.Counters{Total: 100, Idle: 60}, MemoryUsed: 8 << 30, MemoryTotal: 32 << 30}}
	updated, _ := m.Update(first)
	m = updated.(Model)
	if m.host.CPUReady {
		t.Fatal("one sample cannot produce CPU percentage")
	}
	updated, _ = m.Update(hostMsg{snapshot: hostmetrics.Snapshot{CPUCounters: hostmetrics.Counters{Total: 160, Idle: 80}, MemoryUsed: 9 << 30, MemoryTotal: 32 << 30}})
	m = updated.(Model)
	if !m.host.CPUReady || len(m.hostCPUHistory) != 1 || m.host.CPUPercent < 66 || m.host.CPUPercent > 67 {
		t.Fatalf("incorrect host reading %+v", m.host)
	}
	if len(m.hostMemHistory) != 2 {
		t.Fatal("host memory history missing")
	}
	m.width, m.height = 172, 52
	rendered := m.View()
	if !strings.Contains(rendered, "LOCAL HOST") || !strings.Contains(rendered, "LOCAL LINUX HOST") || !strings.Contains(rendered, "RAM") || !strings.Contains(rendered, "DOCKER ENGINE") {
		t.Fatalf("no distinct host/Docker panels: %q", rendered)
	}
}
func TestDockerIORatesAndResetAcrossContexts(t *testing.T) {
	m := testModel()
	base := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	m.snapshot.Updated = base
	m.snapshot.Stats["0123456789abcdef"] = dockercli.Usage{ID: "0123456789abcdef", NetIO: "1MB / 1MB", BlockIO: "2MB / 2MB", CPUPerc: "2%"}
	next := m.snapshot
	next.Updated = base.Add(5 * time.Second)
	next.Stats = map[string]dockercli.Usage{"0123456789abcdef": {ID: "0123456789abcdef", NetIO: "2MB / 3MB", BlockIO: "3MB / 4MB", CPUPerc: "3%"}}
	updated, _ := m.Update(snapshotMsg{snapshot: next})
	m = updated.(Model)
	if !m.ioRates.Ready || m.ioRates.RX != 200000 || m.ioRates.TX != 400000 {
		t.Fatalf("bad Docker rates %+v", m.ioRates)
	}
	next.Context = "remote"
	next.Updated = next.Updated.Add(5 * time.Second)
	updated, _ = m.Update(snapshotMsg{snapshot: next})
	m = updated.(Model)
	if m.ioRates.Ready || len(m.rxHistory) != 0 {
		t.Fatal("I/O rates crossed Docker contexts")
	}
}

func TestFrameInheritsTerminalBackground(t *testing.T) {
	// Ordinary panel rows should not force an ANSI background color.
	// Highlighted selected rows are intentionally styled separately.
	rendered := frame("Transparent", []string{"plain", cyan.Render("accent")}, 40, 6)
	if strings.Contains(rendered, "\x1b[48;") || strings.Contains(rendered, "\x1b[40m") {
		t.Fatalf("panel applies an explicit background: %q", rendered)
	}
	for _, line := range strings.Split(rendered, "\n") {
		if lipgloss.Width(line) != 40 {
			t.Fatalf("frame lost alignment: width=%d", lipgloss.Width(line))
		}
	}
}


func TestBrowserRibbonResponsive(t *testing.T) {
	for _, dim := range [][2]int{{90, 23}, {120, 35}, {172, 52}} {
		for _, page := range []int{5, 6, 7} {
			m := testModel()
			m.page = page
			m.width, m.height = dim[0], dim[1]
			m.compose = []dockercli.ComposeProject{{Name: "test-project", Status: "running(1)"}}
			m.hub = []dockercli.HubImage{{Name: "nginx", StarCount: 100}}
			m.hubLoaded = true
			view := m.View()
			if page != 7 && !strings.Contains(view, "READ ONLY") {
				t.Fatalf("page %d lost read-only status", page)
			}
			if page == 7 && !strings.Contains(view, "IMAGE DISCOVERY") {
				t.Fatal("Docker Hub ribbon missing")
			}
			if lipgloss.Height(view) > dim[1] {
				t.Fatalf("page %d rendered %d lines at height %d", page, lipgloss.Height(view), dim[1])
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > dim[0] {
					t.Fatalf("page %d rendered width %d at %d cols", page, lipgloss.Width(line), dim[0])
				}
			}
		}
	}
}


func TestMCPInspectorAndConfiguredNavigation(t *testing.T) {
	m := testModel()
	m.page = 5
	state, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	n := state.(Model)
	if !n.mcpInspectBusy || n.mcpInspectTarget != "github-official" || cmd == nil {
		t.Fatal("Enter should request selected server metadata asynchronously")
	}
	state, _ = n.Update(mcpInspectMsg{name: "github-official", text: "SERVER ID: github-official\nADVERTISED TOOLS\n- read", err: nil})
	n = state.(Model)
	if n.detailTitle == "" || !strings.Contains(n.detailText, "ADVERTISED TOOLS") {
		t.Fatal("read-only metadata detail missing")
	}
	state, _ = n.Update(tea.KeyMsg{Type: tea.KeyEsc})
	n = state.(Model)
	state, cmd = n.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	n = state.(Model)
	if !n.mcpAssigned || n.showProfiles || !n.mcpAssignedBusy || cmd == nil {
		t.Fatal("assigned-server list did not load")
	}
	state, _ = n.Update(mcpAssignedMsg{text: "github-official in dev"})
	n = state.(Model)
	if !strings.Contains(n.View(), "github-official in dev") {
		t.Fatal("missing configured servers")
	}
	state, _ = n.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	n = state.(Model)
	if n.mcpAssigned {
		t.Fatal("configured toggle did not return to catalog")
	}
}
func TestMCPInspectionStaleResponseIgnored(t *testing.T) {
	m := testModel()
	m.page = 2
	state, _ := m.Update(mcpInspectMsg{name: "github-official", text: "STALE"})
	if strings.Contains(state.(Model).View(), "STALE") {
		t.Fatal("stale MCP response displayed after leaving catalog")
	}
}


func TestMCPDetailFooterAndCompletedStatus(t *testing.T) {
	m := testModel()
	m.page = 5
	m.message = "Inspecting catalog metadata for github-official"
	m.mcpInspectBusy = true
	m.mcpInspectTarget = "github-official"

	updated, _ := m.Update(mcpInspectMsg{name: "github-official", text: "SERVER ID: github-official\nADVERTISED TOOLS"})
	m = updated.(Model)
	if m.message != "" || m.mcpInspectBusy || m.detailTitle != "MCP CATALOG / github-official" {
		t.Fatalf("inspection did not finish cleanly: %+v", m)
	}
	view := m.View()
	if strings.Contains(view, "Inspecting catalog metadata") {
		t.Fatal("stale inspection banner remains")
	}
	if !strings.Contains(view, "[Esc] Return to catalog") {
		t.Fatal("modal does not explain the return path")
	}
	if !strings.Contains(m.footer(120), "[p/a] Available after returning") {
		t.Fatalf("footer did not explain modal shortcuts: %q", m.footer(120))
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	if m.mcpAssigned || m.detailTitle == "" {
		t.Fatal("a must not toggle configured servers while inspection is open")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.detailTitle != "" || !strings.Contains(m.footer(120), "[a] Configured") {
		t.Fatal("Esc did not restore MCP catalog shortcuts")
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	if !m.mcpAssigned || cmd == nil {
		t.Fatal("a must load configured servers after returning")
	}
}

func TestMCPInspectErrorsAndNotifications(t *testing.T) {
	m := testModel()
	m.page = 5
	m.message = "Inspecting catalog metadata for github-official"
	m.mcpInspectTarget = "github-official"
	updated, _ := m.Update(mcpInspectMsg{name: "github-official", err: fmt.Errorf("metadata unavailable")})
	m = updated.(Model)
	if !strings.Contains(m.message, "metadata unavailable") {
		t.Fatal("inspection errors must remain visible")
	}

	m.message = "Unrelated notification"
	updated, _ = m.Update(mcpInspectMsg{name: "github-official", text: "server"})
	m = updated.(Model)
	if m.message != "Unrelated notification" {
		t.Fatal("inspection should not clear unrelated notifications")
	}
}

func TestContextFooterForOtherModals(t *testing.T) {
	m := testModel()
	m.confirmOperation = "stop"
	if strings.Contains(m.footer(100), "[a]") || !strings.Contains(m.footer(100), "[y] Confirm") {
		t.Fatal("confirmation footer is not contextual")
	}
	m.confirmOperation = ""
	m.detailTitle = "LOGS / database-production"
	if strings.Contains(m.footer(100), "MCP") || !strings.Contains(m.footer(100), "Scroll") {
		t.Fatal("logs footer is not contextual")
	}
}


func TestMCPManagerReviewRequired(t *testing.T){
	m:=testModel()
	m.page=5
	updated,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'m'}})
	m=updated.(Model)
	if !m.pm.open || !m.pm.busy || cmd==nil {t.Fatal("manager must load profiles")}
	updated,_=m.Update(managerProfilesMsg{values:[]dockercli.MCPProfile{{ID:"web-dev",Name:"Web Development"}}})
	m=updated.(Model)
	updated,cmd=m.Update(tea.KeyMsg{Type:tea.KeyEnter})
	m=updated.(Model)
	if m.pm.stage!=1 || m.pm.profileID!="web-dev" || cmd==nil {t.Fatal("profile selection failed")}
	updated,_=m.Update(managerServersMsg{profile:"web-dev",values:[]dockercli.MCPProfileServer{{Name:"github-official"}}})
	m=updated.(Model)
	updated,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'e'}})
	m=updated.(Model)
	if m.pm.action!="enable" || m.pm.target!="github-official" || cmd!=nil {t.Fatal("enable must only stage review")}
	if !strings.Contains(m.View(),"ENABLE ALL TOOLS") || !strings.Contains(m.View(),"Press y") {t.Fatal("confirmation not rendered")}
	updated,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'n'}})
	m=updated.(Model)
	if m.pm.action!="" || cmd!=nil {t.Fatal("cancel should never invoke Docker")}
	updated,_=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'d'}})
	m=updated.(Model)
	if m.pm.action!="disable" {t.Fatal("disable not staged")}
	updated,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'y'}})
	m=updated.(Model)
	if cmd==nil || !m.pm.saving {t.Fatal("explicit y must create approved command")}
	updated,_=m.Update(managerChangedMsg{profile:"web-dev",server:"github-official",enable:false})
	m=updated.(Model)
	if m.pm.saving || !m.pm.serverBusy || !strings.Contains(m.pm.notice,"disabled all") {t.Fatal("success needs refresh and notice")}
}
func TestManagerEscapeAndLateResponse(t *testing.T){
	m:=testModel();m.page=5
	updated,_:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'m'}});m=updated.(Model)
	updated,_=m.Update(tea.KeyMsg{Type:tea.KeyEsc});m=updated.(Model)
	if m.pm.open {t.Fatal("did not close manager")}
	updated,_=m.Update(managerProfilesMsg{values:[]dockercli.MCPProfile{{ID:"secret",Name:"bad"}}})
	m=updated.(Model)
	if m.pm.open || len(m.pm.profiles)!=0 {t.Fatal("stale async response revived manager")}
}
