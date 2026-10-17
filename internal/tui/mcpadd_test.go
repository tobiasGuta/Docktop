package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

func TestCatalogAddIsExplicitAndReadOnlyUntilConfirmed(t *testing.T) {
	m:=testModel()
	m.page=5
	next,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'A'}})
	m=next.(Model)
	if !m.catalogAdd.open || m.catalogAdd.server!="github-official" || m.catalogAdd.phase!="loading" || cmd==nil {
		t.Fatal("Shift+A must load profiles for chosen catalog server")
	}
	next,_=m.Update(catalogAddProfilesMsg{server:"github-official",profiles:[]dockercli.MCPProfile{{ID:"development",Name:"development"}}})
	m=next.(Model)
	if m.catalogAdd.phase!="choose" {t.Fatal("profile selection not shown")}
	next,cmd=m.Update(tea.KeyMsg{Type:tea.KeyEnter})
	m=next.(Model)
	if m.catalogAdd.phase!="checking" || m.catalogAdd.profile!="development" || cmd==nil {t.Fatal("membership not checked before confirmation")}
	next,_=m.Update(catalogAddMembersMsg{server:"github-official",profile:"development",members:[]dockercli.MCPProfileServer{}})
	m=next.(Model)
	if m.catalogAdd.phase!="review" {t.Fatal("review not opened")}
	view:=m.View()
	if !strings.Contains(view,"catalog://mcp/docker-mcp-catalog/github-official") || !strings.Contains(view,"Press y") || !strings.Contains(view,"may expose") {t.Fatal("review omitted exact URI or warning")}
	next,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'n'}})
	m=next.(Model)
	if m.catalogAdd.open || cmd!=nil {t.Fatal("cancel must not cause a Docker write")}

	next,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'A'}})
	m=next.(Model)
	next,_=m.Update(catalogAddProfilesMsg{server:"github-official",profiles:[]dockercli.MCPProfile{{ID:"development"}}})
	m=next.(Model)
	next,_=m.Update(tea.KeyMsg{Type:tea.KeyEnter})
	m=next.(Model)
	next,_=m.Update(catalogAddMembersMsg{server:"github-official",profile:"development"})
	m=next.(Model)
	next,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'y'}})
	m=next.(Model)
	if m.catalogAdd.phase!="saving"||cmd==nil {t.Fatal("only y should initiate backend write")}
	next,_=m.Update(catalogAddCompletedMsg{server:"github-official",profile:"development"})
	m=next.(Model)
	if m.catalogAdd.phase!="complete" || !strings.Contains(m.View(),"SERVER ADDED") {t.Fatal("no success acknowledgment")}
}

func TestCatalogAddDuplicateOrStaleResponseNeverGivesReview(t *testing.T) {
	m:=testModel();m.page=5
	m.catalogAdd=catalogAddModel{open:true,server:"github-official",profile:"development",phase:"checking"}
	next,_:=m.Update(catalogAddMembersMsg{server:"github-official",profile:"development",members:[]dockercli.MCPProfileServer{{Name:"github-official"}}})
	m=next.(Model)
	if m.catalogAdd.phase!="error" {t.Fatal("duplicate should not enter review")}
	next,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'y'}})
	m=next.(Model)
	if cmd!=nil || m.catalogAdd.phase!="error" {t.Fatal("error state must not mutate")}
	m.catalogAdd=catalogAddModel{open:true,server:"playwright",phase:"loading"}
	next,_=m.Update(catalogAddProfilesMsg{server:"github-official",profiles:[]dockercli.MCPProfile{{ID:"bad"}}})
	m=next.(Model)
	if m.catalogAdd.phase!="loading" {t.Fatal("stale catalog result changed modal")}
}

func TestCatalogAddModalFooterAndBounds(t *testing.T) {
	for _,dim:=range [][2]int{{110,32},{80,24},{50,16}} {
		m:=testModel();m.page=5;m.width=dim[0];m.height=dim[1]
		m.catalogAdd=catalogAddModel{open:true,server:"github-official",phase:"choose",profiles:[]dockercli.MCPProfile{{ID:"development"}}}
		view:=m.View()
		if !strings.Contains(view,"[Esc] Cancel") {t.Fatal("catalog add footer missing")}
		if !strings.Contains(view,"MCP / CATALOG SERVER ASSIGNMENT") {t.Fatal("add modal missing")}
	}
}
