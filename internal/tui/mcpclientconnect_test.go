package tui

import (
 "strings"
 "testing"

 tea "github.com/charmbracelet/bubbletea"
 "github.com/tobiasGuta/Docktop/internal/dockercli"
)
func clientConnectionModel()Model{
 m:=testModel()
 m.page=5;m.width=130;m.height=38
 m.clients=mcpClientBrowser{open:true,phase:"browse",items:[]dockercli.MCPClientIntegration{
  {ID:"codex",Label:"Codex",State:"not configured",Installed:"installed",OSSupport:"supported",Profile:"(unspecified)"},
  {ID:"claude-code",Label:"Claude Code",State:"not configured",Installed:"not installed",OSSupport:"supported",Profile:"(unspecified)"},
 }}
 return m
}
func TestMCPClientConnectionRequiresProfileAndReview(t *testing.T){
 m:=clientConnectionModel()
 next,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'c'}});m=next.(Model)
 if m.clients.phase!="load-profiles"||cmd==nil{t.Fatal("eligible client must load existing profiles")}
 next,cmd=m.Update(clientProfilesLoadedMsg{clientID:"codex",values:[]dockercli.MCPProfile{{ID:"development",Name:"development"},{ID:"sandbox",Name:"Sandbox"}}});m=next.(Model)
 if cmd!=nil||m.clients.phase!="choose-profile"{t.Fatal("profile chooser not displayed")}
 next,cmd=m.Update(tea.KeyMsg{Type:tea.KeyDown});m=next.(Model)
 next,cmd=m.Update(tea.KeyMsg{Type:tea.KeyEnter});m=next.(Model)
 if cmd!=nil||m.clients.phase!="review"||m.clients.profileID!="sandbox" {t.Fatal("profile selection invoked write")}
 view:=m.View()
 if !strings.Contains(view,"--profile sandbox --quiet codex")||!strings.Contains(view,"Press y")||!strings.Contains(view,"MODIFY"){t.Fatal("review lacks exact CLI and warning")}
 next,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'n'}});m=next.(Model)
 if cmd!=nil||m.clients.phase!="choose-profile"{t.Fatal("review cancellation wrote or lost chooser")}
 next,_=m.Update(tea.KeyMsg{Type:tea.KeyEnter});m=next.(Model)
 next,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'y'}});m=next.(Model)
 if cmd==nil||m.clients.phase!="saving"{t.Fatal("only y must dispatch connector")}
}
func TestMCPClientUnsupportedDoesNotConnect(t *testing.T){
 m:=clientConnectionModel();m.clients.selected=1
 next,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'c'}});m=next.(Model)
 if cmd!=nil||m.clients.phase!="browse"||!strings.Contains(m.clients.notice,"not confirmed installed"){t.Fatal("uninstalled client shouldn't dispatch")}
 m.clients.selected=0;m.clients.items[0].State="configured"
 next,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'c'}});m=next.(Model)
 if cmd!=nil||m.clients.phase!="browse"{t.Fatal("already configured client should be blocked")}
}
func TestMCPClientSearchIsLocalAndBindsSelection(t *testing.T){
 m:=clientConnectionModel()
 next,_:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'/'}});m=next.(Model)
 for _,ch:=range "claude"{
  next,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{ch}});m=next.(Model)
  if cmd!=nil{t.Fatal("typing should not query Docker")}
 }
 next,_=m.Update(tea.KeyMsg{Type:tea.KeyEnter});m=next.(Model)
 selected,ok:=m.clients.selectedClient()
 if !ok||selected.ID!="claude-code"||!strings.Contains(m.View(),"1 MATCHED"){t.Fatalf("wrong filtered client: %+v",selected)}
 next,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'c'}});m=next.(Model)
 if cmd!=nil||m.clients.phase!="browse"{t.Fatal("filter must not select a different installed client")}
}
func TestMCPClientWriteResultVerifiedVsUnknown(t *testing.T){
 for _,tc:=range []struct{outcome,expected string}{
  {"verified","CLIENT CONFIGURATION VERIFIED"},
  {"blocked","CONNECTION BLOCKED BEFORE ANY WRITE"},
  {"unverified","CONNECTION OUTCOME NOT VERIFIED"},
 }{
  m:=clientConnectionModel()
  m.clients.phase="saving";m.clients.clientID="codex";m.clients.profileID="development"
  result:=dockercli.MCPClientConnectResult{ClientID:"codex",ProfileID:"development",Outcome:tc.outcome,
    Reason:"token=SECRET",Updated:[]dockercli.MCPClientIntegration{{ID:"codex",Label:"Codex",State:"configured",Profile:"development"}}}
  next,_:=m.Update(clientConnectDoneMsg{clientID:"codex",profileID:"development",result:result});m=next.(Model)
  if m.clients.phase!="result"||!strings.Contains(m.View(),tc.expected){t.Fatalf("%s missing status",tc.outcome)}
  if strings.Contains(m.View(),"SECRET"){t.Fatal("raw CLI text leaked to display")}
 }
}
