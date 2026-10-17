package tui

import (
 "context"
 "fmt"
 "sort"
 "strings"

 tea "github.com/charmbracelet/bubbletea"
 "github.com/tobiasGuta/Docktop/internal/dockercli"
)

type mcpPlanDoneMsg struct {
 profile,server string
 result dockercli.MCPToolPlanResult
}
func applyStagedMCPPlan(c dockercli.Client,profile,server string,before dockercli.MCPPermissionView,staged map[string]bool)tea.Cmd{
 // Freeze the exact proposed set before starting the asynchronous operation.
 changes:=make([]dockercli.MCPToolChange,0,len(staged))
 for tool,enable:=range staged {
  changes=append(changes,dockercli.MCPToolChange{Tool:tool,Enable:enable})
 }
 sort.Slice(changes,func(i,j int)bool{return changes[i].Tool<changes[j].Tool})
 return func()tea.Msg{
  result:=c.MCPApplyToolPlan(context.Background(),profile,server,before,changes)
  return mcpPlanDoneMsg{profile:profile,server:server,result:result}
 }
}
func (p permissionOverlay) stageLines()[]string{
 names:=make([]string,0,len(p.staged))
 for name:=range p.staged{names=append(names,name)}
 sort.Strings(names)
 lines:=make([]string,0,len(names))
 for _,name:=range names{
  old,_:=toolSnapshotStatus(p.view,name)
  desired:="disabled"
  if p.staged[name]{desired="enabled"}
  lines=append(lines,name+": "+old+" -> "+desired)
 }
 return lines
}
func toolSnapshotStatus(view dockercli.MCPPermissionView,name string)(string,bool){
 for _,tool:=range view.Tools{if tool.Name==name{return tool.Status,true}}
 return "",false
}
func (m Model) permissionPlanView(w,h int)string{
 p:=m.perm
 lines:=[]string{
  cyan.Render("REVIEW MCP TOOL PERMISSION PLAN"),"",
  "PROFILE   "+plain(p.profile,max(1,w-15)),
  "SERVER    "+plain(p.server,max(1,w-15)),
  fmt.Sprintf("CHANGES   %d  (maximum 12)",len(p.staged)),"",
  amber.Render("DOCKER MCP APPLIES THESE CHANGES SEQUENTIALLY, NOT ATOMICALLY."),
  amber.Render("Earlier steps are NOT automatically reversed if a later one fails."),
  "Every step is rechecked and verified before proceeding.","",
 }
 for _,change:=range p.stageLines(){
  lines=append(lines,plain(change,max(1,w-4)))
 }
 lines=append(lines,"",
  "Connected clients may gain or lose access immediately.",
  "No actions occur until you explicitly confirm.","",
  red.Render("Press y to apply exactly this plan; n/Esc cancels."))
 return frame("MCP / REVIEW STAGED PERMISSIONS",lines,w,h)
}
func (p *permissionOverlay) stageSelected() {
 if p.view.Mode=="unknown"{return}
 tool,ok:=p.chosenTool()
 if !ok{return}
 if p.staged==nil{p.staged=map[string]bool{}}
 if _,exists:=p.staged[tool.Name];exists{
  delete(p.staged,tool.Name)
  return
 }
 if len(p.staged)>=12{return}
 p.staged[tool.Name]=tool.Status=="disabled"
}
func (m Model) handlePlanKey(key string)(tea.Model,tea.Cmd){
 p:=&m.perm
 switch key{
 case "n","esc":
  p.phase="tools"
 case "y","Y":
  if len(p.staged)==0||len(p.staged)>12{return m,nil}
  p.phase="saving"
  p.operation="permission plan"
  p.batchReport=""
  return m,applyStagedMCPPlan(m.client,p.profile,p.server,p.view,p.staged)
 }
 return m,nil
}
func (m Model) mcpPlanResult(v mcpPlanDoneMsg)Model{
 p:=&m.perm
 if !p.open||p.phase!="saving"||p.operation!="permission plan"||p.profile!=v.profile||p.server!=v.server{return m}
 result:=v.result
 p.staged=nil
 p.batchReport=fmt.Sprintf("%d changes verified",len(result.Verified))
 if len(result.Verified)>0 {
  p.batchReport+=": "+strings.Join(result.Verified,", ")
 }
 if result.Err!=nil {
  switch result.FailureKind {
  case "blocked":p.phase="blocked"
  case "uncertain":p.phase="unverified"
  default:p.phase="partial"
  }
  p.errText=result.Err.Error()
  if result.Attempted!=""{
   p.errText+="; latest attempted tool: "+result.Attempted+" (outcome not independently verified)"
  }
  return m
 }
 p.phase="complete"
 p.view=result.After
 p.errText=""
 return m
}
