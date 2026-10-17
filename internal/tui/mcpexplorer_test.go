package tui

import (
	"strings"
	"testing"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

func testPermissionModel()Model{
	m:=testModel()
	m.page=5
	m.width,m.height=120,35
	m.pm=profileManager{open:true,stage:1,profileID:"development",servers:[]dockercli.MCPProfileServer{{Name:"github-official"}}}
	m.perm=permissionOverlay{open:true,phase:"tools",profile:"development",server:"github-official",
		view:dockercli.MCPPermissionView{Profile:"development",Server:"github-official",Mode:"allowlist",Tools:[]dockercli.MCPToolPermission{
			{Name:"get_file",Description:"Read the contents of a repository file",Status:"enabled",CapabilityHint:"read-only"},
			{Name:"create_issue",Description:"Create a GitHub issue",Status:"disabled",CapabilityHint:"other"},
			{Name:"delete_repo",Description:"Delete an entire repo",Status:"enabled",CapabilityHint:"destructive"},
			{Name:"custom_task",Description:"Some helpful thing",Status:"disabled",CapabilityHint:"unspecified"},
		}}}
	return m
}
func TestPermissionSearchFilterAndGroup(t *testing.T){
	m:=testPermissionModel()
	a,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'/'}})
	m=a.(Model)
	if !m.perm.editingFilter||cmd!=nil{t.Fatal("search must be local")}
	for _,ch:=range "issue"{
		a,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{ch}})
		m=a.(Model)
		if cmd!=nil{t.Fatal("search issued a command")}
	}
	a,_=m.Update(tea.KeyMsg{Type:tea.KeyEnter});m=a.(Model)
	visible:=m.perm.visibleTools()
	if len(visible)!=1||visible[0].Name!="create_issue"{t.Fatalf("search returned wrong tools: %+v",visible)}
	if !strings.Contains(m.View(),"1 MATCHED"){t.Fatal("search count absent")}
	a,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'d'}});m=a.(Model)
	if cmd!=nil||m.perm.phase!="tools"{t.Fatal("disabled tool must not stage a disable")}
	a,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'e'}});m=a.(Model)
	if cmd!=nil||m.perm.phase!="review-tool"||m.perm.tool!="create_issue"{t.Fatal("filter selected wrong target")}
	a,_=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'n'}});m=a.(Model)
	m.perm.filter=""
	a,_=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'g'}});m=a.(Model)
	visible=m.perm.visibleTools()
	if len(visible)!=1||visible[0].Name!="get_file"{t.Fatal("read-only group incorrect")}
}
func TestPermissionSummaryAndResponsivePanels(t *testing.T) {
	for _,dim:=range [][2]int{{172,52},{120,35},{89,23},{55,18}} {
		m:=testPermissionModel();m.width=dim[0];m.height=dim[1]
		view:=m.View()
		if !strings.Contains(view,"TOTAL 4")||!strings.Contains(view,"EN 2")||!strings.Contains(view,"DIS 2"){
			t.Fatalf("missing permission summary at %v",dim)
		}
		if dim[0]>=120 && !strings.Contains(view,"POLICY / TOOL DETAILS"){t.Fatal("two-panel view missing")}
		for _,line:=range strings.Split(view,"\n"){
			if lipgloss.Width(line)>dim[0]{t.Fatalf("width %d exceeds %d",lipgloss.Width(line),dim[0])}
		}
		if lipgloss.Height(view)>dim[1]{t.Fatalf("height %d exceeds %d",lipgloss.Height(view),dim[1])}
	}
}
func TestPermissionVerificationStatesNeverImplyUnverifiedSuccess(t *testing.T){
	m:=testPermissionModel()
	m.perm.phase="saving";m.perm.tool="get_file";m.perm.operation="disable"
	msg:=permissionsChangedMsg{profile:"development",server:"github-official",tool:"get_file",operation:"disable",verifyErr:fmt.Errorf("readback unavailable")}
	a,_:=m.Update(msg);m=a.(Model)
	if m.perm.phase!="unverified"||!strings.Contains(m.View(),"NOT VERIFIED"){t.Fatal("missing uncertainty warning")}
	m.perm.phase="saving"
	msg.verifyErr=nil
	msg.after=m.perm.view
	a,_=m.Update(msg);m=a.(Model)
	if m.perm.phase!="complete"||!strings.Contains(m.View(),"VERIFIED"){t.Fatal("verified state missing")}
}
