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
	"github.com/tobiasGuta/Docktop/internal/hostmetrics"
	"github.com/tobiasGuta/Docktop/internal/mcpcatalog"
)

const refreshInterval = 5 * time.Second
const catalog = "mcp/docker-mcp-catalog:latest"
const historyLimit = 60

var (
	cyan          = lipgloss.NewStyle().Foreground(lipgloss.Color(colorCyan)).Bold(true)
	muted         = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
	green         = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	amber         = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAmber))
	red           = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText)).Background(lipgloss.Color(colorSelection)).Bold(true)
	edge          = lipgloss.NewStyle().Foreground(lipgloss.Color(colorBorder))
)
var tabs = []string{"Dashboard", "Containers", "Images", "Networks", "Volumes", "MCP Catalog", "Compose", "Docker Hub"}
var shortTabs = []string{"Home", "Ctr", "Img", "Net", "Vol", "MCP", "Cmp", "Hub"}

type Model struct {
	client                                                                                                 dockercli.Client
	pm profileManager
	clients mcpClientBrowser
	imageRemoval imageRemovalOverlay
	mcpSecretsBusy bool
	perm permissionOverlay
	catalogAdd catalogAddModel
	snapshot                                                                                               dockercli.Snapshot
	width, height                                                                                          int
	page, selected, scroll                                                                                 int
	busy, mcpBusy, mcpLoaded                                                                               bool
	message                                                                                                string
	mcpText, mcpFilter                                                                                     string
	servers                                                                                                []mcpcatalog.Server
	editingFilter                                                                                          bool
	confirmOperation, confirmID, confirmName                                                               string
	detailTitle, detailText                                                                                string
	showHelp, showItem                                                                                     bool
	cpuHistory, memHistory                                                                                 []float64
	hostCPUHistory, hostMemHistory                                                                         []float64
	host                                                                                                   hostmetrics.Snapshot
	hostError                                                                                              string
	hostBusy                                                                                               bool
	hostTicks                                                                                              int
	lastGPU                                                                                                hostmetrics.GPU
	ioRates                                                                                                dockercli.IORates
	rxHistory, diskHistory                                                                                 []float64
	mcpInspectBusy, mcpAssigned, mcpAssignedBusy, mcpAssignedLoaded                                   bool
	mcpInspectTarget, mcpAssignedText, mcpAssignedError                                               string
	compose                                                                                                []dockercli.ComposeProject
	hub                                                                                                    []dockercli.HubImage
	hubQuery, hubError, composeError, profilesError, profilesText                                          string
	composeBusy, composeLoaded, hubBusy, hubLoaded, hubEditing, profilesBusy, profilesLoaded, showProfiles bool
}

type hostMsg struct {
	snapshot  hostmetrics.Snapshot
	err       error
	gpuPolled bool
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
type composeMsg struct {
	data []dockercli.ComposeProject
	err  error
}
type hubMsg struct {
	data []dockercli.HubImage
	err  error
}
type profilesMsg struct {
	data string
	err  error
}
type inspectMsg struct {
	title, text string
	err         error
}
type mcpInspectMsg struct {
	name, text string
	err        error
}
type mcpAssignedMsg struct {
	text string
	err  error
}
type pulledMsg struct {
	image string
	err   error
}

func New() Model { return Model{client: dockercli.New(), width: 100, height: 30, busy: true} }
func pollHost(gpu bool) tea.Cmd {
	return func() tea.Msg { s, err := hostmetrics.Read(gpu); return hostMsg{snapshot: s, err: err, gpuPolled: gpu} }
}
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
func loadCompose(c dockercli.Client) tea.Cmd {
	return func() tea.Msg { v, e := c.ComposeProjects(context.Background()); return composeMsg{v, e} }
}
func searchHub(c dockercli.Client, q string) tea.Cmd {
	return func() tea.Msg { v, e := c.SearchHub(context.Background(), q); return hubMsg{v, e} }
}
func loadProfiles(c dockercli.Client) tea.Cmd {
	return func() tea.Msg { v, e := c.MCPProfiles(context.Background()); return profilesMsg{v, e} }
}
func inspectMCPServer(c dockercli.Client, name string) tea.Cmd {
	return func() tea.Msg { v, e := c.MCPServerDetails(context.Background(), name); return mcpInspectMsg{name, v, e} }
}
func loadMCPAssigned(c dockercli.Client) tea.Cmd {
	return func() tea.Msg { v, e := c.MCPConfiguredServers(context.Background()); return mcpAssignedMsg{v, e} }
}
func inspectResource(c dockercli.Client, kind, id, title string) tea.Cmd {
	return func() tea.Msg {
		v, e := c.InspectSummary(context.Background(), kind, id)
		return inspectMsg{title, v, e}
	}
}
func pullHub(c dockercli.Client, name, ctx string) tea.Cmd {
	return func() tea.Msg { e := c.PullHub(context.Background(), name, ctx); return pulledMsg{name, e} }
}
func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}
func (m Model) Init() tea.Cmd { return tea.Batch(poll(m.client), pollHost(true), tick()) }

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
		if m.showProfiles || m.mcpAssigned {
			return 0
		}
		return len(mcpcatalog.Filter(m.servers, m.mcpFilter))
	case 6:
		return len(m.compose)
	case 7:
		return len(m.hub)
	}
	return 0
}
func (m *Model) switchPage(p int) {
	m.page = p
	m.selected = 0
	m.scroll = 0
	m.message = ""
	m.showItem = false
	m.mcpSecretsBusy = false
}
func (m *Model) loadExtraPage() tea.Cmd {
	switch m.page {
	case 5:
		if m.mcpAssigned {
			if !m.mcpAssignedLoaded && !m.mcpAssignedBusy {
				m.mcpAssignedBusy = true
				return loadMCPAssigned(m.client)
			}
			return nil
		}
		if m.showProfiles {
			if !m.profilesLoaded && !m.profilesBusy {
				m.profilesBusy = true
				return loadProfiles(m.client)
			}
			return nil
		}
		if !m.mcpLoaded {
			return m.fetchMCP()
		}
	case 6:
		if !m.composeLoaded && !m.composeBusy {
			m.composeBusy = true
			return loadCompose(m.client)
		}
	}
	return nil
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
	case clientProfilesLoadedMsg:
		m=m.clientProfilesResult(v)
	case clientConnectDoneMsg:
		m=m.clientConnectedResult(v)
	case configuredClientsMsg:
		m=m.configuredClientResult(v)
	case imageRemovalDoneMsg:
		updated, cmd := m.imageRemovalResult(v)
		if cmd != nil { return updated, cmd }
		m = updated
	case mcpSecretsMsg:
		if m.mcpSecretsBusy {
			m.mcpSecretsBusy = false
			if m.message == "Reading Docker MCP secret inventory..." { m.message = "" }
			if m.page == 5 && !m.pm.open && !m.clients.open && !m.perm.open && !m.catalogAdd.open && m.detailTitle == "" {
				if v.err != nil {
					m.message = "Docker MCP secret inventory unavailable; check docker mcp secret ls --json"
				} else {
					m.detailTitle = "MCP / STORED SECRET METADATA"
					m.detailText = mcpSecretsText(v.values)
					m.scroll = 0
				}
			}
		}
	case mcpPlanDoneMsg:
		m=m.mcpPlanResult(v)
	case permissionsLoadedMsg:
		m=m.permissionResult(v)
	case permissionsChangedMsg:
		m=m.permissionChanged(v)
	case catalogAddProfilesMsg:
		m=m.catalogAddProfilesResult(v)
	case catalogAddMembersMsg:
		m=m.catalogAddMembersResult(v)
	case catalogAddCompletedMsg:
		m=m.catalogAddCompleteResult(v)
	case managerProfilesMsg:
		if m.pm.open && m.pm.stage==0 {
			m.pm.busy=false
			if v.err!=nil { m.pm.errText=v.err.Error();m.pm.profiles=nil }else{m.pm.errText="";m.pm.profiles=v.values;m.pm.profileIndex=0}
		}
	case managerServersMsg:
		if m.pm.open && m.pm.stage==1 && m.pm.profileID==v.profile {
			m.pm.serverBusy=false
			if v.err!=nil {m.pm.errText=v.err.Error();m.pm.servers=nil}else{m.pm.errText="";m.pm.servers=v.values;m.pm.serverIndex=0}
		}
	case managerCreatedMsg:
		if m.pm.open && m.pm.saving {
			m.pm.saving=false
			if v.err!=nil {
				m.pm.notice=""
				m.pm.errText="Profile creation failed: "+v.err.Error()
			}else{
				m.pm.errText=""
				m.pm.notice="Created empty profile "+v.name+". No servers or tools enabled."
				m.pm.busy=true
				return m,fetchManagerProfiles(m.client)
			}
		}
	case managerChangedMsg:
		if m.pm.open && m.pm.profileID==v.profile {
			m.pm.saving=false
			if v.err!=nil {
				m.pm.notice=""
				m.pm.errText="Permission change failed: "+v.err.Error()
			}else{
				op:="enabled all tools for"
				if !v.enable {op="disabled all tools for"}
				m.pm.notice=op+" "+v.server+" in "+v.profile
				m.pm.errText=""
				m.pm.serverBusy=true
				return m,fetchManagerServers(m.client,v.profile)
			}
		}
	case hostMsg:
		m.hostBusy = false
		if v.err != nil {
			m.hostError = v.err.Error()
		} else {
			m.hostError = ""
			v.snapshot.CPUPercent, v.snapshot.CPUReady = hostmetrics.CPUPercent(m.host.CPUCounters, v.snapshot.CPUCounters)
			if v.gpuPolled {
				m.lastGPU = v.snapshot.GPU
			}
			v.snapshot.GPU = m.lastGPU
			m.host = v.snapshot
			if m.host.CPUReady {
				m.hostCPUHistory = appendHistory(m.hostCPUHistory, m.host.CPUPercent)
			}
			m.hostMemHistory = appendHistory(m.hostMemHistory, float64(m.host.MemoryUsed)*100/float64(m.host.MemoryTotal))
		}
	case snapshotMsg:
		m.busy = false
		if v.err != nil {
			m.message = "Docker unavailable: " + v.err.Error()
		} else {
			if m.snapshot.Context != "" && v.snapshot.Context != m.snapshot.Context {
				m.cpuHistory, m.memHistory = nil, nil
				m.ioRates = dockercli.IORates{}
				m.rxHistory, m.diskHistory = nil, nil
			}
			if m.snapshot.Context != "" && m.snapshot.Context == v.snapshot.Context {
				m.ioRates = dockercli.DeltaIO(m.snapshot.IOCounters(), v.snapshot.IOCounters(), v.snapshot.Updated.Sub(m.snapshot.Updated))
				if m.ioRates.Ready {
					m.rxHistory = appendHistory(m.rxHistory, m.ioRates.RX+m.ioRates.TX)
					m.diskHistory = appendHistory(m.diskHistory, m.ioRates.Read+m.ioRates.Write)
				}
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
	case composeMsg:
		m.composeBusy = false
		m.composeLoaded = true
		if v.err != nil {
			m.composeError = v.err.Error()
		} else {
			m.compose = v.data
			m.composeError = ""
		}
		m.clampSelection()
	case hubMsg:
		m.hubBusy = false
		m.hubLoaded = true
		if v.err != nil {
			m.hubError = v.err.Error()
			m.hub = nil
		} else {
			m.hub = v.data
			m.hubError = ""
		}
		m.selected = 0
	case profilesMsg:
		m.profilesBusy = false
		m.profilesLoaded = true
		if v.err != nil {
			m.profilesError = v.err.Error()
			m.profilesText = ""
		} else {
			m.profilesText = v.data
			m.profilesError = ""
		}
	case mcpInspectMsg:
		m.mcpInspectBusy = false
		// Clear the temporary loading banner when that request finishes.
		// Preserve unrelated errors or notifications from later actions.
		if m.message == "Inspecting catalog metadata for "+v.name {
			m.message = ""
		}
		if m.page == 5 && !m.mcpAssigned && !m.showProfiles && m.mcpInspectTarget == v.name {
			if v.err != nil {
				m.message = "MCP metadata: " + v.err.Error()
			} else {
				m.detailTitle = "MCP CATALOG / " + v.name
				m.detailText = v.text
				m.scroll = 0
			}
		}
	case mcpAssignedMsg:
		m.mcpAssignedBusy = false
		m.mcpAssignedLoaded = true
		if v.err != nil {
			m.mcpAssignedError = v.err.Error()
			m.mcpAssignedText = ""
		} else {
			m.mcpAssignedError = ""
			m.mcpAssignedText = v.text
		}
	case inspectMsg:
		if v.err != nil {
			m.message = "Inspect: " + v.err.Error()
		} else {
			m.detailTitle = v.title
			m.detailText = v.text
			m.scroll = 0
		}
	case pulledMsg:
		if v.err != nil {
			m.message = "Image pull failed: " + v.err.Error()
		} else {
			m.message = "Pulled image: " + v.image
			m.busy = true
			return m, poll(m.client)
		}
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
		if !m.hostBusy {
			m.hostBusy = true
			m.hostTicks++
			cmds = append(cmds, pollHost(m.hostTicks%3 == 0))
		}
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
		if m.imageRemoval.open {
			return m.imageRemovalKey(key)
		}
		if m.confirmOperation != "" {
			if key == "y" || key == "Y" {
				op, id, name := m.confirmOperation, m.confirmID, m.confirmName
				m.confirmOperation, m.confirmID, m.confirmName = "", "", ""
				m.message = "Running " + op + " for " + name
				if op == "pull" {
					return m, pullHub(m.client, name, id)
				}
				return m, change(m.client, op, id, name)
			}
			if key == "n" || key == "esc" {
				m.confirmOperation, m.confirmID, m.confirmName = "", "", ""
			}
			return m, nil
		}
		if m.hubEditing {
			switch key {
			case "enter":
				m.hubEditing = false
				if m.hubQuery == "" {
					m.hubError = "Type a search term first"
					m.hubLoaded = true
					return m, nil
				}
				m.hubBusy = true
				return m, searchHub(m.client, m.hubQuery)
			case "esc":
				m.hubEditing = false
			case "backspace":
				if len(m.hubQuery) > 0 {
					_, n := utf8.DecodeLastRuneInString(m.hubQuery)
					m.hubQuery = m.hubQuery[:len(m.hubQuery)-n]
				}
			case "ctrl+u":
				m.hubQuery = ""
			default:
				if len(key) == 1 && len(m.hubQuery) < 80 {
					m.hubQuery += key
				}
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
		if m.clients.open {
			return m.configuredClientKey(key)
		}
		if m.perm.open {
			return m.permissionKey(key)
		}
		if m.pm.open {
			return m.managerKey(key)
		}
		if m.catalogAdd.open {
			return m.catalogAddKey(key)
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
			return m, m.loadExtraPage()
		case "shift+tab", "left":
			m.switchPage((m.page + len(tabs) - 1) % len(tabs))
			return m, m.loadExtraPage()
		case "1", "2", "3", "4", "5", "6", "7", "8":
			m.switchPage(int(key[0] - '1'))
			return m, m.loadExtraPage()
		case "up", "k":
			if m.page == 5 && (m.showProfiles || m.mcpAssigned) {
				m.scroll = max(0, m.scroll-1)
			} else {
				m.selected = max(0, m.selected-1)
			}
		case "down", "j":
			if m.page == 5 && (m.showProfiles || m.mcpAssigned) {
				m.scroll++
			} else {
				m.selected = min(m.rowCount()-1, m.selected+1)
				m.clampSelection()
			}
		case "pgup":
			if m.page == 5 && (m.showProfiles || m.mcpAssigned) {
				m.scroll = max(0, m.scroll-10)
			} else {
				m.selected = max(0, m.selected-10)
			}
		case "pgdown":
			if m.page == 5 && (m.showProfiles || m.mcpAssigned) {
				m.scroll += 10
			} else {
				m.selected = min(m.rowCount()-1, m.selected+10)
				m.clampSelection()
			}
		case "home":
			m.selected = 0
		case "end":
			m.selected = max(0, m.rowCount()-1)
		case "enter":
			if m.page==5 && m.mcpAssigned {
				m.pm=profileManager{open:true,busy:true}
				return m,fetchManagerProfiles(m.client)
			}
			if m.page == 5 && !m.showProfiles && !m.mcpAssigned {
				servers := m.filteredServers()
				if m.selected < len(servers) && !m.mcpInspectBusy {
					name := servers[m.selected].Name
					m.mcpInspectTarget, m.mcpInspectBusy = name, true
					m.message = "Inspecting catalog metadata for " + name
					return m, inspectMCPServer(m.client, name)
				}
				return m, nil
			}
			if (m.page == 0 || m.page == 1) && m.selected < len(m.snapshot.Containers) {
				c := m.snapshot.Containers[m.selected]
				return m, inspectResource(m.client, "container", c.ID, "INSPECT / "+c.Names)
			}
			if m.page == 3 && m.selected < len(m.snapshot.Networks) {
				n := m.snapshot.Networks[m.selected]
				return m, inspectResource(m.client, "network", n.Name, "NETWORK / "+n.Name)
			}
			if m.page == 4 && m.selected < len(m.snapshot.Volumes) {
				v := m.snapshot.Volumes[m.selected]
				return m, inspectResource(m.client, "volume", v.Name, "VOLUME / "+v.Name)
			}
			if m.rowCount() > 0 {
				m.showItem = true
			}
		case "r":
			if m.page == 5 {
				if m.mcpAssigned {
					if !m.mcpAssignedBusy {
						m.mcpAssignedBusy = true
						return m, loadMCPAssigned(m.client)
					}
					return m, nil
				}
				if m.showProfiles {
					m.profilesBusy = true
					return m, loadProfiles(m.client)
				}
				return m, m.fetchMCP()
			}
			if m.page == 6 {
				m.composeBusy = true
				return m, loadCompose(m.client)
			}
			if m.page == 7 {
				if m.hubQuery != "" {
					m.hubBusy = true
					return m, searchHub(m.client, m.hubQuery)
				}
				return m, nil
			}
			if !m.busy {
				m.busy = true
				return m, poll(m.client)
			}
		case "/":
			if m.page == 5 && !m.showProfiles {
				m.editingFilter = true
			}
			if m.page == 7 {
				m.hubEditing = true
			}
		case "A":
			if m.page==5 && !m.showProfiles && !m.mcpAssigned && m.mcpLoaded && !m.mcpBusy {
				servers:=m.filteredServers()
				if m.selected>=0 && m.selected<len(servers) {
					name:=servers[m.selected].Name
					if _,err:=dockercli.MCPCatalogServerRef(name);err!=nil {
						m.message="Invalid catalog server name; cannot add"
						return m,nil
					}
					m.catalogAdd=catalogAddModel{open:true,server:name,phase:"loading"}
					return m,fetchAddProfiles(m.client,name)
				}
			}
		case "a":
			if m.page == 5 {
				m.mcpAssigned = !m.mcpAssigned
				m.showProfiles = false
				m.scroll = 0
				return m, m.loadExtraPage()
			}
		case "i":
			if m.page==5 {
				m.clients=mcpClientBrowser{open:true,busy:true}
				return m,fetchConfiguredClients(m.client)
			}
		case "m":
			if m.page==5 && !m.showProfiles && !m.mcpAssigned {
				m.pm=profileManager{open:true,busy:true}
				return m,fetchManagerProfiles(m.client)
			}
		case "p":
			if m.page == 5 {
				m.mcpAssigned = false
				m.showProfiles = !m.showProfiles
				m.scroll = 0
				return m, m.loadExtraPage()
			}
			if m.page == 7 && !m.hubBusy && m.selected < len(m.hub) && m.hub[m.selected].Name != "" {
				m.confirmOperation = "pull"
				m.confirmName = m.hub[m.selected].Name
				m.confirmID = m.snapshot.Context
			}
		case "l":
			if (m.page == 0 || m.page == 1) && m.selected < len(m.snapshot.Containers) {
				c := m.snapshot.Containers[m.selected]
				m.message = "Loading logs for " + c.Names
				return m, loadLogs(m.client, c.ID, c.Names)
			}
		case "d":
			if m.page == 2 && m.selected >= 0 && m.selected < len(m.snapshot.Images) {
				img := m.snapshot.Images[m.selected]
				target := dockercli.ImageRemovalTarget{Context: m.snapshot.Context, Repository: img.Repository, Tag: img.Tag, ID: img.ID}
				if err := dockercli.ValidateImageRemovalTarget(target); err != nil {
					m.message = "This image cannot be removed safely from Docktop; only tagged, identifiable images are supported."
					return m, nil
				}
				m.imageRemoval = imageRemovalOverlay{open: true, phase: "review", target: target}
			}
		case "s":
			if m.page == 5 && !m.mcpSecretsBusy {
				m.mcpSecretsBusy = true
				m.message = "Reading Docker MCP secret inventory..."
				return m, fetchMCPSecrets(m.client)
			}
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
// panelAccent gives the dashboard panels distinct visual identities without
// changing their data, commands or layout dimensions.
func panelAccent(title string) lipgloss.Style {
	switch {
	case strings.HasPrefix(title, "CPU"):
		return green
	case strings.HasPrefix(title, "MEMORY"):
		return cyan
	case strings.HasPrefix(title, "NETWORK"), strings.HasPrefix(title, "Docker Hub"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorViolet)).Bold(true)
	case strings.HasPrefix(title, "CONTAINERS"):
		return green
	default:
		return cyan
	}
}

func frame(title string, content []string, width, height int) string {
	width = max(12, width)
	height = max(3, height)
	inner := width - 2
	title = plain(title, max(0, inner-3))
	used := lipgloss.Width(title) + 2
	var b strings.Builder
	b.WriteString(edge.Render("╭─ ") + panelAccent(title).Render(title) + edge.Render(strings.Repeat("─", max(0, inner-used))+"╮") + "\n")
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
	// Paint every cell, including padding spaces, so transparent terminals do
	// not show desktop wallpaper through the panel content.
	return panelSurface.Render(b.String())
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
func (m Model) header(w int) string {
	title := cyan.Render("◆ DOCKTOP") + "  " + muted.Render("ENGINE / MCP")
	if w < 80 {
		return title
	}
	cpu, mem := m.snapshot.TotalUsage()
	stats := fmt.Sprintf("CPU %.1f%%  MEM %.1f MiB", cpu, mem/1048576)
	gap := max(1, w-lipgloss.Width(title)-lipgloss.Width(stats)-1)
	return title + strings.Repeat(" ", gap) + green.Render(stats)
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
	if w >= 125 && len(m.snapshot.QueryTimings) > 0 {
		step, duration := m.snapshot.SlowestQuery()
		label += fmt.Sprintf("  •  poll %dms (slow: %s %dms)",
			m.snapshot.Duration.Milliseconds(), step, duration.Milliseconds())
	}
	return muted.Render(plain(label, w))
}
func (m Model) navigation(w int) string {
	// Keep navigation within the real terminal width even with eight tabs.
	if w < 75 {
		current := fmt.Sprintf(" %d:%s ", m.page+1, shortTabs[m.page])
		hint := plain("  Tab: next  Shift+Tab: previous  1–8: views", max(0, w-lipgloss.Width(current)))
		return selectedStyle.Render(current) + muted.Render(hint)
	}
	names := tabs
	if w < 130 {
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
	return plainStyledNavigation(b.String(), w)
}
func plainStyledNavigation(value string, w int) string {
	if lipgloss.Width(value) <= w {
		return value
	}
	return selectedStyle.Render(" " + plain("Docktop / use 1–8 or Tab to switch", w-2) + " ")
}
func (m Model) footer(w int) string {
	// Modal and text-entry shortcuts take precedence over page-level shortcuts.
	// In particular, p and a are disabled inside MCP metadata inspection.
	if m.imageRemoval.open {
		return muted.Render(plain(m.imageRemovalFooter(), w))
	}
	if m.confirmOperation != "" {
		return muted.Render(plain("[y] Confirm  [n/Esc] Cancel", w))
	}
	if m.clients.open {
		return muted.Render(plain(m.configuredClientFooter(),w))
	}
	if m.perm.open {
		return muted.Render(plain(m.permissionFooter(), w))
	}
	if m.pm.open {
		return muted.Render(plain(m.managerFooter(), w))
	}
	if m.catalogAdd.open {
		return muted.Render(plain(m.catalogAddFooter(), w))
	}
	if m.detailTitle != "" {
		help := "[Esc/q] Back  [↑↓/PgUp/PgDn] Scroll"
		if m.page == 5 && strings.HasPrefix(m.detailTitle, "MCP CATALOG / ") {
			help = "[Esc/q] Back to MCP Catalog  [↑↓/PgUp/PgDn] Scroll  [p/a] Available after returning"
		}
		return muted.Render(plain(help, w))
	}
	if m.showItem {
		return muted.Render(plain("[Enter/Esc/q] Back to list", w))
	}
	if m.showHelp {
		return muted.Render(plain("[Any key] Close help", w))
	}
	if m.editingFilter {
		return muted.Render(plain("[Enter] Apply filter  [Esc] Close search  [Ctrl+U] Clear", w))
	}
	if m.hubEditing {
		return muted.Render(plain("[Enter] Search Docker Hub  [Esc] Cancel  [Ctrl+U] Clear", w))
	}
	help := "[1–8] Views  [Tab] Next  [↑↓/Pg] Navigate  [Enter] Details  [r] Refresh  [?] Help  [q] Quit"
	if m.page == 1 {
		help = "[l] Logs  [s] Start/Stop  [R] Restart  [Enter] Details  [?] Help  [q] Quit"
	}
	if m.page == 2 {
		help = "[d] Remove image tag (review)  [Enter] Details  [r] Refresh  [?] Help  [q] Quit"
	}
	if m.page == 5 {
		help = "[/] Search  [Enter] Inspect  [Shift+A] Add  [m] Profiles  [i] Clients  [s] Secrets  [a] Configured  [q] Quit"
	}
	if m.page == 6 {
		help = "[7] Compose  [↑↓] Browse  [r] Reload  [q] Quit"
	}
	if m.page == 7 {
		help = "[/] Search Hub  [p] Pull (confirmation)  [↑↓] Browse  [r] Reload  [q] Quit"
	}
	if w < 80 {
		help = "[1–8] Views  [↑↓] Select  [Enter] Details  [?] Help  [q] Quit"
	}
	return muted.Render(plain(help, w))
}
func (m Model) View() string {
	w, h := m.width, m.height
	if w < 30 || h < 10 {
		return cyan.Render("DOCKTOP") + " — enlarge terminal (at least 30x10)"
	}
	var b strings.Builder
	b.WriteString(m.header(w) + "\n")
	b.WriteString(m.statusLine(w) + "\n")
	b.WriteString(m.navigation(w) + "\n")
	available := max(3, h-6)
	switch {
	case m.showHelp:
		b.WriteString(frame("KEYBOARD SHORTCUTS", []string{
			"1–8 / Tab  Switch views     ↑↓ / j k / PgUp PgDn  Select and scroll",
			"Enter  Item details       r  Refresh current data       q  Quit",
			"Containers: l  Last 100 log lines; s  Start/stop; R  Restart",
			"Images: d  Review removal of one tagged image (no force, context pinned)",
			"Mutating actions always require explicit y confirmation.",
			"MCP: / Search; Enter inspect; Shift+A add; i clients; s stored-secret metadata (read-only)",
			"Docker Hub: / Search; p Pull selected image (requires confirmation)",
			"Compose: list existing projects (read-only)",
			"Esc  Close details / prompts; actions target the current Docker context.",
			"No MCP servers are started or installed by this application.",
		}, w, available))
	case m.imageRemoval.open:
		b.WriteString(m.imageRemovalView(w, available))
	case m.confirmOperation != "":
		lines := []string{red.Render("Container state change requires confirmation"), "", "Operation: " + m.confirmOperation}
		lines = append(lines, wrap("Container: "+m.confirmName, max(10, w-4), 3)...)
		lines = append(lines, "ID: "+plain(m.confirmID, w-6), "", "Press y to execute, n or Esc to cancel.")
		b.WriteString(frame("CONFIRM ACTION", lines, w, available))
	case m.detailTitle != "":
		limit := max(1, available-2)
		rows := m.logLines(limit, max(8, w-4))
		if m.page == 5 && strings.HasPrefix(m.detailTitle, "MCP CATALOG / ") && limit >= 4 {
			rows = append([]string{muted.Render("[Esc] Return to catalog to use [a] configured / [p] profiles"), ""}, m.logLines(limit-2, max(8, w-4))...)
		}
		b.WriteString(frame(m.detailTitle+"  (Esc to close)", rows, w, available))
	case m.clients.open:
		b.WriteString(m.configuredClientView(w,available))
	case m.perm.open:
		b.WriteString(m.permissionView(w,available))
	case m.pm.open:
		b.WriteString(m.managerView(w,available))
	case m.catalogAdd.open:
		b.WriteString(m.catalogAddView(w,available))
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
	// Keep the entire screen visually stable even when the terminal emulator
	// itself is configured for transparency. This is presentation-only.
	return paintCanvas(b.String(), w, h)
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
	if m.page == 5 && m.mcpAssigned {
		lines := []string{muted.Render("Configured MCP servers across profiles  [Enter] detailed manager  [a] catalog")}
		if m.mcpAssignedBusy {
			lines = append(lines, "Loading configured server list…")
		} else if m.mcpAssignedError != "" {
			lines = append(lines, wrap("MCP configured servers unavailable: "+m.mcpAssignedError, w-4, 4)...)
		} else {
			lines = append(lines, splitAndScroll(m.mcpAssignedText, m.scroll, max(1, h-4), w-4)...)
		}
		return frame("CONFIGURED MCP SERVERS / READ ONLY", lines, w, h)
	}
	if m.page == 5 && m.showProfiles {
		lines := []string{muted.Render("Read-only MCP profile list  [p] to return to catalog")}
		if m.profilesBusy {
			lines = append(lines, "Loading profiles…")
		} else if m.profilesError != "" {
			lines = append(lines, wrap("MCP profiles unavailable: "+m.profilesError, w-4, 4)...)
		} else {
			lines = append(lines, splitAndScroll(m.profilesText, m.scroll, max(1, h-4), w-4)...)
		}
		return frame("MCP PROFILES", lines, w, h)
	}
	if m.page >= 5 && m.page <= 7 && w >= 82 && h >= 16 {
		return lipgloss.JoinVertical(lipgloss.Left, m.browserRibbon(w), "", m.browseSplit(w, h-5))
	}
	return m.browseSplit(w, h)
}

// browserRibbon labels the discovery views without extra Docker requests.
func (m Model) browserRibbon(w int) string {
	var title, summary, hint string
	switch m.page {
	case 5:
		title = "MCP  /  CATALOG EXPLORER"
		summary = fmt.Sprintf("SERVERS  %d matched / %d loaded  •  READ ONLY", len(m.filteredServers()), len(m.servers))
		hint = "[/] search   [Enter] inspect   [Shift+A] add to profile   [m] profiles   [a] configured"
	case 6:
		title = "DOCKER  /  COMPOSE PROJECTS"
		summary = fmt.Sprintf("PROJECTS  %d discovered   •   READ ONLY", len(m.compose))
		hint = "[Enter] inspect config paths   [r] refresh   (never starts services)"
	case 7:
		title = "DOCKER HUB  /  IMAGE DISCOVERY"
		summary = fmt.Sprintf("RESULTS  %d   •   QUERY  %s", len(m.hub), m.hubQuery)
		hint = "[/] search   [Enter] image details   [p] pull (asks for confirmation)"
	}
	return frame(title, []string{cyan.Render(plain(summary, w-4)), muted.Render(plain(hint, w-4))}, w, 4)
}

func (m Model) browseSplit(w, h int) string {
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
	if m.page == 6 {
		if m.composeBusy {
			return []string{"Loading Compose projects…"}
		}
		if m.composeError != "" {
			return wrap("Compose unavailable: "+m.composeError, width, 4)
		}
	}
	if m.page == 7 {
		if m.hubEditing {
			return []string{cyan.Render("Search Docker Hub> " + plain(m.hubQuery, width-22) + "_")}
		}
		if m.hubBusy {
			return []string{"Searching Docker Hub…"}
		}
		if m.hubError != "" {
			return wrap("Hub search error: "+m.hubError, width, 4)
		}
		if !m.hubLoaded {
			return []string{"Press / to search Docker Hub (no automatic requests)."}
		}
	}
	if m.rowCount() == 0 {
		return []string{muted.Render("No results found.")}
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
	case 6:
		lines = append(lines, muted.Render(padded("COMPOSE PROJECT", nameWidth)+"  STATUS"))
	case 7:
		query := m.hubQuery
		if m.hubEditing {
			query += "_"
		}
		if query == "" {
			query = "(press / to search)"
		}
		lines = append(lines, muted.Render(plain("Search Docker Hub: "+query, width)))
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
		case 6:
			v := m.compose[i]
			line = padded(v.Name, nameWidth) + "  " + plain(v.Status, 12)
		case 7:
			v := m.hub[i]
			line = padded(v.Name, nameWidth) + "  " + plain(fmt.Sprintf("★%d", v.StarCount), 12)
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
	case 6:
		if m.selected >= len(m.compose) {
			return []string{"No Compose projects discovered."}
		}
		v := m.compose[m.selected]
		entries = [][2]string{{"PROJECT", v.Name}, {"STATUS", v.Status}, {"CONFIG FILES", v.ConfigFiles}}
	case 7:
		if m.selected >= len(m.hub) {
			return []string{"Press / to search Docker Hub."}
		}
		v := m.hub[m.selected]
		entries = [][2]string{{"IMAGE", v.Name}, {"DESCRIPTION", v.Description}, {"STARS", fmt.Sprint(v.StarCount)}, {"OFFICIAL", fmt.Sprint(v.Official)}}
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
		lines = append(lines, muted.Render("Enter: catalog metadata + advertised tool names (read only)"))
		lines = append(lines, muted.Render("p: profiles   a: configured servers   no server execution"))
	}
	if m.page == 7 {
		lines = append(lines, muted.Render("Press p to request pull; requires y confirmation."))
	}
	return lines
}

func splitAndScroll(raw string, offset, maxLines, width int) []string {
	lines := strings.Split(dockercli.Clean(raw, 12000), "\n")
	if len(lines) == 0 {
		return []string{"(none)"}
	}
	offset = min(max(0, offset), max(0, len(lines)-maxLines))
	out := []string{}
	for _, line := range lines[offset:min(len(lines), offset+maxLines)] {
		out = append(out, plain(line, width))
	}
	return out
}
