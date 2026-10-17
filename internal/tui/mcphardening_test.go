package tui

import (
 "fmt"
 "strings"
 "testing"

 tea "github.com/charmbracelet/bubbletea"
 "github.com/tobiasGuta/Docktop/internal/dockercli"
)
func TestConfiguredClientInspectorReadOnlyModal(t *testing.T){
 m:=testModel();m.page=5;m.width=130;m.height=36
 state,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'i'}});m=state.(Model)
 if !m.clients.open||!m.clients.busy||cmd==nil{t.Fatal("i must trigger configuration query")}
 results:=[]dockercli.MCPClientIntegration{
  {ID:"codex",Label:"Codex",Profile:"development",State:"configured"},
  {ID:"editor",Label:"Editor",Profile:"(unspecified)",State:"not configured"},
 }
 state,_=m.Update(configuredClientsMsg{values:results});m=state.(Model)
 view:=m.View()
 if m.clients.busy||!strings.Contains(view,"Not live connection telemetry")||
  !strings.Contains(view,"development")||!strings.Contains(view,"Codex") {
  t.Fatalf("configured-client view incomplete: %s",view)
 }
 state,cmd=m.Update(tea.KeyMsg{Type:tea.KeyDown});m=state.(Model)
 if cmd!=nil||m.clients.selected!=1{t.Fatal("navigation should not invoke Docker")}
 state,cmd=m.Update(tea.KeyMsg{Type:tea.KeyEsc});m=state.(Model)
 if cmd!=nil||m.clients.open{t.Fatal("close should not invoke Docker")}
}
func TestConfiguredClientNoRawCLIError(t *testing.T){
 m:=testModel();m.page=5;m.width=130;m.height=36
 m.clients=mcpClientBrowser{open:true,busy:true}
 state,_:=m.Update(configuredClientsMsg{err:fmt.Errorf("MCP_TOKEN=private-value")});m=state.(Model)
 if strings.Contains(m.View(),"private-value"){t.Fatal("CLI error leaked")}
 if !strings.Contains(m.View(),"Unable to read client configurations"){t.Fatal("missing actionable error")}
}
func TestToolPlanOutcomeStatesAreDistinct(t *testing.T){
 for _,tc:=range []struct{kind,phase,notice string}{
  {"blocked","blocked","PLAN BLOCKED BEFORE ANY WRITE"},
  {"partial","partial","STAGED PLAN DID NOT FULLY COMPLETE"},
  {"uncertain","unverified","TOOL WRITE OUTCOME NOT VERIFIED"},
 }{
  m:=testPermissionModel()
  m.perm.phase="saving";m.perm.operation="permission plan"
  state,_:=m.Update(mcpPlanDoneMsg{
   profile:"development",server:"github-official",
   result:dockercli.MCPToolPlanResult{
    FailureKind:tc.kind,Err:fmt.Errorf("simulated failure"),Attempted:"get_file",
   },
  })
  m=state.(Model)
  if m.perm.phase!=tc.phase||!strings.Contains(m.View(),tc.notice){
   t.Fatalf("%s status rendered as %+v",tc.kind,m.perm)
  }
 }
}
