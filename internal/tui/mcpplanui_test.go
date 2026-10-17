package tui

import (
 "strings"
 "testing"
 "fmt"

 tea "github.com/charmbracelet/bubbletea"
 "github.com/charmbracelet/lipgloss"
 "github.com/tobiasGuta/Docktop/internal/dockercli"
)
func TestToolStagingRequiresReviewAndDoesNotWriteOnBrowse(t *testing.T) {
 m:=testPermissionModel()
 step,cmd:=m.Update(tea.KeyMsg{Type:tea.KeySpace});m=step.(Model)
 if cmd!=nil||m.perm.staged["get_file"]!=false||len(m.perm.staged)!=1 {t.Fatalf("space did not stage disable: %+v",m.perm.staged)}
 if !strings.Contains(m.View(),"STAGED 1/12"){t.Fatal("staged count not displayed")}
 step,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'/'}});m=step.(Model)
 for _,key:=range "create" {
  step,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{key}});m=step.(Model)
  if cmd!=nil {t.Fatal("search called Docker")}
 }
 step,_=m.Update(tea.KeyMsg{Type:tea.KeyEnter});m=step.(Model)
 step,cmd=m.Update(tea.KeyMsg{Type:tea.KeySpace});m=step.(Model)
 if cmd!=nil||len(m.perm.staged)!=2||!m.perm.staged["create_issue"] {t.Fatalf("filtered stage wrong: %+v",m.perm.staged)}
 step,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'v'}});m=step.(Model)
 if cmd!=nil||m.perm.phase!="review-plan"||!strings.Contains(m.View(),"NOT ATOMICALLY"){t.Fatal("review must be local with warning")}
 if !strings.Contains(m.View(),"create_issue: disabled -> enabled")||!strings.Contains(m.View(),"get_file: enabled -> disabled"){t.Fatal("review does not show exact plan")}
 step,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'n'}});m=step.(Model)
 if cmd!=nil||m.perm.phase!="tools"||len(m.perm.staged)!=2 {t.Fatal("cancel must preserve staged changes without write")}
 step,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'v'}});m=step.(Model)
 step,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'y'}});m=step.(Model)
 if cmd==nil||m.perm.phase!="saving" {t.Fatal("only explicit y should dispatch plan")}
}
func TestToolStageClearAndPartialAccounting(t *testing.T){
 m:=testPermissionModel()
 m.perm.staged=map[string]bool{"get_file":false}
 step,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'c'}});m=step.(Model)
 if cmd!=nil||len(m.perm.staged)!=0 {t.Fatal("clear must not mutate Docker")}
 m.perm.staged=map[string]bool{"get_file":false,"create_issue":true}
 m.perm.phase="saving";m.perm.operation="permission plan"
 msg:=mcpPlanDoneMsg{profile:"development",server:"github-official",result:dockercli.MCPToolPlanResult{
  Verified:[]string{"get_file"},Attempted:"create_issue",Err:fmt.Errorf("readback mismatch"),
 }}
 step,cmd=m.Update(msg);m=step.(Model)
 if cmd!=nil||m.perm.phase!="partial"||!strings.Contains(m.View(),"1 changes verified")||!strings.Contains(m.View(),"may be unknown") {
  t.Fatal("partial result incorrectly claims complete success")
 }
}
func TestFullDescriptionScrollAndResponsiveUI(t *testing.T){
 m:=testPermissionModel()
 long:=strings.Repeat("Long description sentence that needs to wrap across the details panel. ",26)
 m.perm.view.Tools[0].Description=long
 view:=m.View()
 if !strings.Contains(view,"Lines ") {t.Fatal("description scroll status missing")}
 step,_:=m.Update(tea.KeyMsg{Type:tea.KeyTab});m=step.(Model)
 if !m.perm.detailFocus {t.Fatal("Tab should focus details")}
 step,_=m.Update(tea.KeyMsg{Type:tea.KeyPgDown});m=step.(Model)
 if m.perm.detailScroll==0{t.Fatal("description did not scroll")}
 view=m.View()
 if !strings.Contains(view,"DETAILS FOCUSED"){t.Fatal("focus not shown")}
 step,_=m.Update(tea.KeyMsg{Type:tea.KeyTab});m=step.(Model)
 step,_=m.Update(tea.KeyMsg{Type:tea.KeyDown});m=step.(Model)
 if m.perm.detailScroll!=0{t.Fatal("scroll offset not reset for new tool")}
 for _,dim:=range [][2]int{{172,52},{120,35},{88,23},{55,18}}{
  m:=testPermissionModel();m.width=dim[0];m.height=dim[1]
  view=m.View()
  if !strings.Contains(view,"HINTS") {t.Fatalf("missing capability counters %v",dim)}
  for _,line:=range strings.Split(view,"\n"){
   if lipgloss.Width(line)>dim[0]{t.Fatalf("line exceeded width %v",dim)}
  }
  if lipgloss.Height(view)>dim[1]{t.Fatalf("height exceeded %v",dim)}
 }
}
