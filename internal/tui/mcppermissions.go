package tui

import (
 "context"
 "fmt"
 "strings"

 tea "github.com/charmbracelet/bubbletea"
 "github.com/charmbracelet/lipgloss"
 "github.com/tobiasGuta/Docktop/internal/dockercli"
)

// Independent modal state; opening it never mutates a profile or runs a server.
type permissionOverlay struct {
 open bool
 phase string // loading, tools, review-tool, review-remove, saving, complete, error
 profile, server, tool, operation, errText string
 view dockercli.MCPPermissionView
 selected int
}
type permissionsLoadedMsg struct {
 profile,server string
 data dockercli.MCPPermissionView
 err error
}
type permissionsChangedMsg struct {
 profile,server,tool,operation string
 err error
}
func loadPermissions(c dockercli.Client,profile,server string) tea.Cmd {
 return func() tea.Msg {
  data,err:=c.MCPReadPermissions(context.Background(),profile,server)
  return permissionsLoadedMsg{profile,server,data,err}
 }
}
func applyMCPPermission(c dockercli.Client,profile,server,tool,operation string) tea.Cmd {
 return func() tea.Msg {
  var err error
  switch operation {
  case "remove":
   err=c.MCPRemoveProfileServer(context.Background(),profile,server)
  case "enable","disable":
   err=c.MCPSetSpecificTool(context.Background(),profile,server,tool,operation=="enable")
  default:
   err=fmt.Errorf("unsupported permission operation")
  }
  return permissionsChangedMsg{profile,server,tool,operation,err}
 }
}
func (m Model) permissionFooter()string {
 switch m.perm.phase {
 case "loading","saving":return "[Wait] Docker MCP operation in progress"
 case "tools":return "[Esc] Back  [↑↓] Select tool  [e] Enable one  [d] Disable one  [r] Refresh"
 case "review-tool","review-remove":return "[y] Confirm exact change  [n/Esc] Cancel"
 default:return "[Esc/Enter] Return to profile manager"
 }
}
func (m Model) permissionView(w,h int)string {
 p:=m.perm
 lines:=[]string{cyan.Render("PROFILE  "+plain(p.profile,max(1,w-12))),"SERVER   "+plain(p.server,max(1,w-11)),""}
 if p.phase=="loading" {lines=append(lines,muted.Render("Reading profile tool policy without showing secrets..."))}
 if p.phase=="saving" {lines=append(lines,amber.Render("Applying approved MCP change..."))}
 if p.phase=="error" {lines=append(lines,red.Render("NO CHANGE CONFIRMED"),wrap(p.errText,max(1,w-4),max(1,h-9))[0])}
 if p.phase=="complete" {
  lines=append(lines,green.Render("DOCKER MCP REPORTED SUCCESS"),"",
   "OPERATION  "+p.operation,
   "SERVER     "+p.server)
  if p.tool!="" {lines=append(lines,"TOOL       "+p.tool)}
  lines=append(lines,"","Press Esc to return. Refresh to inspect the resulting state.")
 }
 if p.phase=="review-tool" || p.phase=="review-remove" {
  action:="REMOVE SERVER FROM PROFILE"
  if p.phase=="review-tool" {action=strings.ToUpper(p.operation)+" ONE MCP TOOL"}
  lines=append(lines,amber.Render("REVIEW EXACT MCP CONFIGURATION CHANGE"),"",
   "PROFILE    "+p.profile,"SERVER     "+p.server)
  if p.tool!="" {lines=append(lines,"TOOL       "+p.tool)}
  lines=append(lines,"ACTION     "+action,"")
  if p.phase=="review-remove" {
   lines=append(lines,
    amber.Render("Removes this server from only the named profile."),
    "Connected clients may lose access immediately.",
    "Docker MCP may also clean up orphaned OAuth client registrations.",
    "This does not delete a catalog entry or Docker container.")
  }else{
   lines=append(lines,
    amber.Render("Changes a single allowlisted tool in this profile."),
    "Connected AI clients may gain or lose access immediately.",
    "This does not start a server or modify all other tools.")
  }
  lines=append(lines,"",red.Render("Press y to apply, n or Esc to cancel."))
 }
 if p.phase=="tools" {
  lines=append(lines,cyan.Render("TOOL PERMISSIONS  /  "+p.view.Mode),"")
  if p.view.Mode=="unknown" {
   lines=append(lines,amber.Render("UNKNOWN: no reliable allowlist in Docker profile JSON."),
    "Individual tool changes are disabled. No permission is inferred.")
  }else if len(p.view.Tools)==0 {
   lines=append(lines,muted.Render("No tool names present in server snapshot; no changes available."))
  }else{
   limit:=max(1,h-17)
   start:=max(0,p.selected-limit+1)
   if p.selected<limit {start=0}
   for i:=start;i<len(p.view.Tools)&&i<start+limit;i++{
    tool:=p.view.Tools[i]
    label:=tool.Name+"  ["+tool.Status+"]"
    if i==p.selected {lines=append(lines,selectedStyle.Render(padded("> "+plain(label,max(1,w-6)),max(1,w-4))))} else {lines=append(lines,"  "+plain(label,max(1,w-6)))}
   }
   if p.selected<len(p.view.Tools){
    tool:=p.view.Tools[p.selected]
    lines=append(lines,"",cyan.Render("SELECTED TOOL  "+plain(tool.Name,max(1,w-20))),plain(tool.Description,max(1,w-5)))
   }
  }
  lines=append(lines,"",muted.Render("Status from profile show JSON; no live MCP gateway calls."))
 }
 return frame("MCP SERVER PERMISSIONS / "+p.phase,lines,w,h)
}
func (m Model) permissionKey(key string)(tea.Model,tea.Cmd){
 p:=&m.perm
 if p.phase=="loading"||p.phase=="saving" {return m,nil}
 switch p.phase{
 case "tools":
  switch key{
  case "esc","q":m.perm=permissionOverlay{}
  case "up","k":p.selected=max(0,p.selected-1)
  case "down","j":p.selected=min(max(0,len(p.view.Tools)-1),p.selected+1)
  case "pgup":p.selected=max(0,p.selected-10)
  case "pgdown":p.selected=min(max(0,len(p.view.Tools)-1),p.selected+10)
  case "r":p.phase="loading";return m,loadPermissions(m.client,p.profile,p.server)
  case "e","d":
   if p.view.Mode=="unknown"||p.selected>=len(p.view.Tools){return m,nil}
   tool:=p.view.Tools[p.selected]
   requested:="enable"
   if key=="d"{requested="disable"}
   if (requested=="enable"&&tool.Status=="enabled")||(requested=="disable"&&tool.Status=="disabled"){return m,nil}
   p.operation=requested;p.tool=tool.Name;p.phase="review-tool"
  }
 case "review-tool","review-remove":
  switch key{
  case "n","esc":if p.phase=="review-tool"{p.phase="tools";p.operation="";p.tool=""}else{m.perm=permissionOverlay{}}
  case "y","Y":
   p.phase="saving"
   return m,applyMCPPermission(m.client,p.profile,p.server,p.tool,p.operation)
  }
 case "complete","error":
  switch key {
  case "esc","q","enter":
   profile:=p.profile
   m.perm=permissionOverlay{}
   if m.pm.open && m.pm.stage==1 && m.pm.profileID==profile {
    m.pm.serverBusy=true
    return m,fetchManagerServers(m.client,profile)
   }
  }
 }
 return m,nil
}
func (m Model) permissionResult(v permissionsLoadedMsg)Model{
 p:=&m.perm
 if !p.open||p.phase!="loading"||p.profile!=v.profile||p.server!=v.server{return m}
 if v.err!=nil {p.phase="error";p.errText=v.err.Error()}else{p.view=v.data;p.selected=0;p.phase="tools"}
 return m
}
func (m Model) permissionChanged(v permissionsChangedMsg)Model{
 p:=&m.perm
 if !p.open||p.phase!="saving"||p.profile!=v.profile||p.server!=v.server||p.tool!=v.tool||p.operation!=v.operation{return m}
 if v.err!=nil{p.phase="error";p.errText=v.err.Error()}else{p.phase="complete"}
 return m
}
var _ = lipgloss.Width
