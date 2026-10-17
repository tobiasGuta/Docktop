package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

func TestEmptyProfileShowsActionableGuidance(t *testing.T) {
	m:=testModel()
	m.page=5
	m.pm=profileManager{open:true}
	v:=m.View()
	if !strings.Contains(v,"No MCP profiles configured.") || !strings.Contains(v,"docker mcp feature enable profiles") || !strings.Contains(v,"[c]") {
		t.Fatalf("missing onboarding guidance: %s",v)
	}
}
func TestProfileCreationRequiresNameReviewAndConfirmation(t *testing.T) {
	m:=testModel();m.page=5;m.pm=profileManager{open:true}
	state,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'c'}})
	m=state.(Model)
	if !m.pm.createEditing || cmd!=nil {t.Fatal("c should open input without a Docker command")}
	for _,ch:=range "docktop-lab"{
		state,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{ch}})
		m=state.(Model)
		if cmd!=nil {t.Fatal("typing invoked a Docker command")}
	}
	state,cmd=m.Update(tea.KeyMsg{Type:tea.KeyEnter})
	m=state.(Model)
	if !m.pm.createReview || cmd!=nil || !strings.Contains(m.View(),"CONFIRM PROFILE CREATION") {
		t.Fatal("Enter must only show review")
	}
	state,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'n'}})
	m=state.(Model)
	if cmd!=nil || !m.pm.createEditing {t.Fatal("cancel must go back to input without mutation")}
	state,_=m.Update(tea.KeyMsg{Type:tea.KeyEnter});m=state.(Model)
	state,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'y'}})
	m=state.(Model)
	if cmd==nil || !m.pm.saving {t.Fatal("y should admit profile creation")}
	state,cmd=m.Update(managerCreatedMsg{name:"docktop-lab"})
	m=state.(Model)
	if cmd==nil || !m.pm.busy || !strings.Contains(m.pm.notice,"No servers or tools enabled") {t.Fatal("successful creation must refresh")}
	state,_=m.Update(managerProfilesMsg{values:[]dockercli.MCPProfile{{ID:"docktop-lab",Name:"docktop-lab"}}})
	m=state.(Model)
	if m.pm.busy || len(m.pm.profiles)!=1 {t.Fatal("profile list not updated")}
}
func TestInvalidProfileNameNeverTriggersWrite(t *testing.T) {
	m:=testModel();m.page=5;m.pm=profileManager{open:true,createEditing:true}
	state,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyEnter})
	m=state.(Model)
	if m.pm.createReview || cmd!=nil || m.pm.errText=="" {t.Fatal("empty profile name accepted")}
	m.pm.createEditing=false;m.pm.createReview=true;m.pm.createName="-bad"
	state,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'y'}})
	m=state.(Model)
	if cmd!=nil || m.pm.saving {t.Fatal("invalid profile name admitted write")}
}
