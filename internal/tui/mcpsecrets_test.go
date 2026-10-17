package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

func TestMCPSecretsOnDemandReadOnlyAndStatus(t *testing.T) {
	m:=testModel()
	m.page=5
	state,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'s'}})
	m=state.(Model)
	if !m.mcpSecretsBusy||cmd==nil||m.detailTitle!="" {
		t.Fatal("MCP secret inventory must be on-demand")
	}
	state,_=m.Update(mcpSecretsMsg{values:[]dockercli.MCPSecretMetadata{
		{Name:"github.personal_access_token",Provider:"docker-pass"},
	}})
	m=state.(Model)
	if m.mcpSecretsBusy||!strings.Contains(m.View(),"github.personal_access_token")||
		!strings.Contains(m.View(),"Stored does NOT mean authenticated") {
		t.Fatal("secret metadata inspection missing warning or name")
	}
	state,_=m.Update(tea.KeyMsg{Type:tea.KeyEsc})
	if state.(Model).detailTitle!="" {t.Fatal("Escape should close inspector")}
}

func TestMCPSecretErrorAndStaleResultAreSafe(t *testing.T) {
	m:=testModel()
	m.page=5
	state,_:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'s'}})
	m=state.(Model)
	state,_=m.Update(mcpSecretsMsg{err:errors.New("TOKEN_LEAK_MARKER")})
	m=state.(Model)
	if strings.Contains(m.View(),"TOKEN_LEAK_MARKER")||!strings.Contains(m.View(),"inventory unavailable") {
		t.Fatal("CLI diagnostic leaked or unavailable status omitted")
	}
	state,_=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'s'}})
	m=state.(Model)
	m.switchPage(0)
	state,_=m.Update(mcpSecretsMsg{values:[]dockercli.MCPSecretMetadata{{Name:"github.personal_access_token",Provider:"docker-pass"}}})
	m=state.(Model)
	if m.detailTitle!=""||strings.Contains(m.View(),"github.personal_access_token") {
		t.Fatal("stale metadata displayed after page transition")
	}
}
