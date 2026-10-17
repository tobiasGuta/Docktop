package tui

import (
 "strings"
 "testing"
 tea "github.com/charmbracelet/bubbletea"
 "github.com/tobiasGuta/Docktop/internal/dockercli"
)
func TestMCPServerRemoveReviewAndCancel(t *testing.T){
 m:=testModel();m.page=5
 m.pm=profileManager{open:true,stage:1,profileID:"development",servers:[]dockercli.MCPProfileServer{{Name:"github-official"}}}
 a,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'x'}});m=a.(Model)
 if cmd!=nil||m.perm.phase!="review-remove"||!strings.Contains(m.View(),"orphaned OAuth"){t.Fatal("remove review missing")}
 a,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'n'}});m=a.(Model)
 if cmd!=nil||m.perm.open{t.Fatal("cancel should not cause write")}
 a,_=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'x'}});m=a.(Model)
 a,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'y'}});m=a.(Model)
 if cmd==nil||m.perm.phase!="saving"{t.Fatal("confirmed removal must dispatch")}
}
func TestMCPToolInspectAndSingleToolReview(t *testing.T){
 m:=testModel();m.page=5
 m.pm=profileManager{open:true,stage:1,profileID:"development",servers:[]dockercli.MCPProfileServer{{Name:"github-official"}}}
 a,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'t'}});m=a.(Model)
 if cmd==nil||m.perm.phase!="loading"{t.Fatal("inspect did not load")}
 p:=dockercli.MCPPermissionView{Profile:"development",Server:"github-official",Mode:"allowlist",Tools:[]dockercli.MCPToolPermission{{Name:"read",Status:"enabled"},{Name:"write",Status:"disabled"}}}
 a,_=m.Update(permissionsLoadedMsg{profile:"development",server:"github-official",data:p});m=a.(Model)
 if m.perm.phase!="tools"||!strings.Contains(m.View(),"enabled"){t.Fatal("tool statuses missing")}
 a,_=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'d'}});m=a.(Model)
 if m.perm.phase!="review-tool"||m.perm.operation!="disable"{t.Fatal("tool review missing")}
 a,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'n'}});m=a.(Model)
 if cmd!=nil||m.perm.phase!="tools"{t.Fatal("cancel must not write")}
 m.perm.view.Mode="unknown"
 a,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'d'}});m=a.(Model)
 if cmd!=nil||m.perm.phase!="tools"{t.Fatal("unknown tool state authorized change")}
}
