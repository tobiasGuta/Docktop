package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

// This modal displays only allowlisted tool metadata, never raw profile JSON.
// Classification is based exclusively on provider-supplied MCP annotations.
// An unspecified hint is not evidence that a tool is safe.
type permissionOverlay struct {
	open bool
	phase string // loading, tools, review-tool, review-remove, saving, complete, error, unverified
	profile, server, tool, operation, errText string
	view dockercli.MCPPermissionView
	selected int // index within filtered tools, not the unfiltered snapshot
	filter, group string
	editingFilter bool
	detailFocus bool
	detailScroll int
	staged map[string]bool
	batchReport string
}
type permissionsLoadedMsg struct {
	profile,server string
	data dockercli.MCPPermissionView
	err error
}
type permissionsChangedMsg struct {
	profile,server,tool,operation string
	err error // Docker write failed; do not claim no change
	verifyErr error // Docker write succeeded but readback could not verify
	after dockercli.MCPPermissionView
}
func loadPermissions(c dockercli.Client,profile,server string) tea.Cmd {
	return func() tea.Msg {
		data,err:=c.MCPReadPermissions(context.Background(),profile,server)
		return permissionsLoadedMsg{profile,server,data,err}
	}
}
func applyMCPPermission(c dockercli.Client,profile,server,tool,operation string,before dockercli.MCPPermissionView) tea.Cmd {
	return func() tea.Msg {
		msg:=permissionsChangedMsg{profile:profile,server:server,tool:tool,operation:operation}
		ctx:=context.Background()
		switch operation {
		case "remove":
			msg.err=c.MCPRemoveProfileServer(ctx,profile,server)
		case "enable","disable":
			msg.err=c.MCPSetSpecificTool(ctx,profile,server,tool,operation=="enable")
		default:
			msg.err=errors.New("unsupported permission operation")
		}
		if msg.err!=nil{return msg}
		if operation=="remove" {
			members,err:=c.MCPProfileMembers(ctx,profile)
			if err!=nil {msg.verifyErr=fmt.Errorf("cannot reread membership: %w",err);return msg}
			for _,member:=range members {
				if member.Name==server {
					msg.verifyErr=errors.New("server still listed as a profile member")
					return msg
				}
			}
			return msg
		}
		msg.after,msg.verifyErr=c.MCPReadPermissions(ctx,profile,server)
		if msg.verifyErr!=nil{return msg}
		expected:="disabled"
		if operation=="enable"{expected="enabled"}
		var located bool
		for _,observed:=range msg.after.Tools{
			if observed.Name==tool{
				located=true
				if observed.Status!=expected{msg.verifyErr=fmt.Errorf("expected %s, read back %s",expected,observed.Status)}
			}
		}
		if !located {msg.verifyErr=errors.New("changed tool absent from readback")}
		if msg.after.Mode=="unknown"{msg.verifyErr=errors.New("tool permissions unknown on readback")}
		// A single-tool command must not silently alter other known tools.
		for _,prior:=range before.Tools {
			if prior.Name==tool{continue}
			found:=false
			for _,observed:=range msg.after.Tools {
				if observed.Name==prior.Name{
					found=true
					if observed.Status!=prior.Status {msg.verifyErr=fmt.Errorf("another tool changed: %s",prior.Name)}
					break
				}
			}
			if !found {msg.verifyErr=fmt.Errorf("previously listed tool disappeared: %s",prior.Name)}
		}
		return msg
	}
}

var permissionGroups = []string{"all","read-only","destructive","other","unspecified"}
func (p permissionOverlay) visibleTools() []dockercli.MCPToolPermission {
	var result []dockercli.MCPToolPermission
	query:=strings.ToLower(strings.TrimSpace(p.filter))
	group:=p.group
	if group==""{group="all"}
	for _,tool:=range p.view.Tools {
		hint:=tool.CapabilityHint
		if hint==""{hint="unspecified"}
		if group!="all" && group!=hint{continue}
		if query!=""&&!strings.Contains(strings.ToLower(tool.Name),query)&&!strings.Contains(strings.ToLower(tool.Description),query){continue}
		result=append(result,tool)
	}
	return result
}
func (p permissionOverlay) chosenTool() (dockercli.MCPToolPermission,bool){
	filtered:=p.visibleTools()
	if p.selected<0 || p.selected>=len(filtered){return dockercli.MCPToolPermission{},false}
	return filtered[p.selected],true
}
func (m Model) permissionFooter()string {
	p:=m.perm
	switch p.phase {
	case "loading","saving":return "[Wait] MCP read / approval operation in progress"
	case "tools":
		if p.editingFilter{return "[Enter] Apply search  [Esc] Done  [Ctrl+U] Clear"}
		if p.detailFocus{return "[Tab] Tools  [↑↓/Pg] Scroll description  [Esc] Back"}
		return "[Esc] Back  [/] Search  [g] Group  [↑↓] Tool  [Tab] Details  [Space] Stage  [v] Review"
	case "review-tool","review-remove","review-plan":return "[y] Confirm exact change  [n/Esc] Cancel"
	case "complete":return "[Esc/Enter] Back  •  change verified by Docker readback"
	case "blocked","unverified","partial":return "[Esc/Enter] Back  •  refresh before retrying changes"
	default:return "[Esc/Enter] Return to profile manager"
	}
}
func (m Model) permissionView(w,h int)string {
	p:=m.perm
	if p.phase=="tools" {return m.permissionBrowse(w,h)}
	if p.phase=="review-plan" {return m.permissionPlanView(w,h)}
	lines:=[]string{cyan.Render("PROFILE  "+plain(p.profile,max(1,w-12))),"SERVER   "+plain(p.server,max(1,w-11)),""}
	switch p.phase{
	case "loading":
		lines=append(lines,muted.Render("Reading profile tool policy (secret fields omitted)..."))
	case "saving":
		lines=append(lines,amber.Render("Applying the approved change and verifying the result..."))
	case "blocked":
		lines=append(lines,amber.Render("PLAN BLOCKED BEFORE ANY WRITE"),p.batchReport,
			"Detected stale policy, invalid state or failed preflight.")
		lines=append(lines,wrap(p.errText,max(1,w-4),max(1,h-13))...)
		lines=append(lines,"Refresh the profile before preparing another plan.")
	case "partial":
		lines=append(lines,amber.Render("STAGED PLAN DID NOT FULLY COMPLETE"),p.batchReport,
			"Verified steps are NOT automatically undone.")
		lines=append(lines,wrap(p.errText,max(1,w-4),max(1,h-13))...)
		lines=append(lines,"Inspect the profile again before further permission changes.")
	case "error":
		lines=append(lines,red.Render("DOCKER MCP OPERATION RETURNED ERROR"))
		lines=append(lines,wrap(p.errText,max(1,w-4),max(1,h-9))...)
		lines=append(lines,"State may require a fresh inspection; no success is claimed.")
	case "unverified":
		headline:="WRITE SUCCEEDED; RESULT NOT VERIFIED"
		if p.operation=="permission plan" {headline="TOOL WRITE OUTCOME NOT VERIFIED"}
		lines=append(lines,amber.Render(headline),p.batchReport,
			"Docker reported success but readback was incomplete or inconsistent.")
		lines=append(lines,wrap(p.errText,max(1,w-4),max(1,h-12))...)
		lines=append(lines,"Inspect again before attempting another permission change.")
	case "complete":
		lines=append(lines,green.Render("CHANGE VERIFIED BY DOCKER READBACK"),"",
			"OPERATION  "+p.operation,"SERVER     "+p.server)
		if p.batchReport!=""{lines=append(lines,green.Render(plain(p.batchReport,max(1,w-4))))}
		if p.tool!=""{lines=append(lines,"TOOL       "+p.tool)}
		lines=append(lines,"","Refresh or reopen the inspector to view the updated permissions.")
	case "review-tool","review-remove":
		action:="REMOVE SERVER FROM PROFILE"
		if p.phase=="review-tool"{action=strings.ToUpper(p.operation)+" ONE MCP TOOL"}
		lines=append(lines,amber.Render("REVIEW EXACT MCP CONFIGURATION CHANGE"),"",
			"PROFILE    "+p.profile,"SERVER     "+p.server)
		if p.tool!=""{lines=append(lines,"TOOL       "+p.tool)}
		lines=append(lines,"ACTION     "+action,"")
		if p.phase=="review-remove"{
			lines=append(lines,amber.Render("Removes this server only from the named profile."),
				"Connected clients may lose access immediately.",
				"Docker MCP may clean up orphaned OAuth registrations.",
				"This does not delete Docker containers or catalog entries.")
		}else{
			lines=append(lines,amber.Render("Updates one named tool in the selected profile."),
				"Connected clients may gain or lose access immediately.",
				"The resulting permission state will be read back.")
		}
		lines=append(lines,"",red.Render("Press y to apply, n or Esc to cancel."))
	}
	return frame("MCP SERVER PERMISSIONS / "+p.phase,lines,w,h)
}
func (m Model) permissionBrowse(w,h int)string{
 p:=m.perm
 if w<88||h<17 {
  compact:=m.permissionListForWidth(p,w,h)
  compact=append(compact,"",cyan.Render("TOOL DESCRIPTION (use wider terminal for scrolling)"))
  tool,ok:=p.chosenTool()
  if ok {
   compact=append(compact,plain(tool.Name,max(1,w-4)))
   desc:=wrap(tool.Description,max(1,w-4),200)
   visible:=max(0,h-len(compact)-3)
   compact=append(compact,trimLines(desc,visible)...)
  }
  return frame("MCP TOOL EXPLORER",compact,w,h)
 }
 left:=max(42,w*54/100)
 right:=w-left-1
 list:=m.permissionListForWidth(p,left,h)
 side:=m.permissionDetailForWidth(p,right,h)
 return lipgloss.JoinHorizontal(lipgloss.Top,
  frame("TOOLS  /  "+fmt.Sprint(len(p.visibleTools()))+" MATCHED",list,left,h)," ",
  frame("POLICY / TOOL DETAILS",side,right,h))
}

func searchLabel(filter string, editing bool) string {
	if filter=="" {
		if editing{return "_"}
		return "(all tools)"
	}
	if editing{return filter+"_"}
	return filter
}
func (m Model) permissionListForWidth(p permissionOverlay,w,h int)[]string{
 visible:=p.visibleTools()
 en,dis:=0,0
 ro,des,other,unspec:=0,0,0,0
 for _,t:=range p.view.Tools {
  if t.Status=="enabled"{en++}else if t.Status=="disabled"{dis++}
  switch t.CapabilityHint{
  case "read-only":ro++
  case "destructive":des++
  case "other":other++
  default:unspec++
  }
 }
 group:=p.group
 if group==""{group="all"}
 out:=[]string{cyan.Render(plain("PROFILE  "+p.profile,max(1,w-4))),
  fmt.Sprintf("TOTAL %d  EN %d  DIS %d",len(p.view.Tools),en,dis),
  muted.Render(plain(fmt.Sprintf("HINTS RO %d  DESTR %d  OTHER %d  UNKNOWN %d",ro,des,other,unspec),max(1,w-4))),
  muted.Render(plain("GROUP  "+group+"  [g] cycle",max(1,w-4))),
  muted.Render(plain("SEARCH  "+searchLabel(p.filter,p.editingFilter),max(1,w-4))),
  muted.Render(plain(fmt.Sprintf("STAGED %d/12  [Space] toggle  [v] review  [c] clear",len(p.staged)),max(1,w-4))),""}
 if p.view.Mode=="unknown"{out=append(out,amber.Render("Unknown policy; write controls unavailable."))}
 if len(visible)==0{out=append(out,muted.Render("No matching tools."))}
 capacity:=max(1,h-len(out)-3)
 start:=max(0,p.selected-capacity+1)
 if p.selected<capacity{start=0}
 for i:=start;i<len(visible)&&i<start+capacity;i++{
  tool:=visible[i]
  label:="[DISABLED]"
  style:=red
  if tool.Status=="enabled"{label="[ENABLED]";style=green}
  mark:=" "
  if enabled,ok:=p.staged[tool.Name];ok {
   if enabled {mark="+"}else{mark="-"}
  }
  room:=max(1,w-8-lipgloss.Width(label))
  name:=plain(tool.Name,room)
  if i==p.selected{
   out=append(out,selectedStyle.Render(padded("> "+mark+" "+name+" "+label,max(1,w-2))))
  }else{
   out=append(out," "+mark+" "+name+" "+style.Render(label))
  }
 }
 return out
}

func (m Model) permissionDetailForWidth(p permissionOverlay,w,h int)[]string{
 tool,ok:=p.chosenTool()
 out:=[]string{cyan.Render("PERMISSION SUMMARY"),"",
  "PROFILE "+plain(p.profile,max(1,w-12)),
  "SERVER  "+plain(p.server,max(1,w-12)),
  "MODE    "+p.view.Mode,""}
 if !ok{return append(out,muted.Render("Select a matching tool in the left panel."))}
 hint:=tool.CapabilityHint
 if hint==""{hint="unspecified"}
 out=append(out,cyan.Render("SELECTED TOOL"),plain(tool.Name,max(1,w-4)),"")
 status:=red.Render(tool.Status)
 if tool.Status=="enabled"{status=green.Render(tool.Status)}
 out=append(out,"PERMISSION  "+status,"CAPABILITY  "+hint,"")
 if hint=="unspecified"{
  out=append(out,amber.Render("No explicit capability hint provided."))
 }else{
  out=append(out,muted.Render("Provider-declared hint, not proof of safety."))
 }
 if enable,staged:=p.staged[tool.Name];staged{
  desired:="disabled";if enable{desired="enabled"}
  out=append(out,amber.Render("STAGED CHANGE: "+tool.Status+" -> "+desired))
 }
 out=append(out,"",cyan.Render("DESCRIPTION  [Tab] focus  [↑↓/Pg] scroll"))
 desc:=wrap(tool.Description,max(1,w-4),1024)
 capacity:=max(1,h-len(out)-6)
 scroll:=min(max(0,p.detailScroll),max(0,len(desc)-capacity))
 if len(desc)==0{desc=[]string{"(no description supplied)"}}
 end:=min(len(desc),scroll+capacity)
 if scroll<len(desc){out=append(out,desc[scroll:end]...)}
 out=append(out,muted.Render(fmt.Sprintf("Lines %d-%d/%d  %s",min(scroll+1,len(desc)),end,len(desc),map[bool]string{true:"DETAILS FOCUSED",false:"Tab to scroll"}[p.detailFocus])),
  "",muted.Render("Snapshot only. No MCP gateway calls."))
 return out
}

func (m Model) permissionKey(key string)(tea.Model,tea.Cmd){
 p:=&m.perm
 if p.phase=="loading"||p.phase=="saving"{return m,nil}
 if p.phase=="review-plan" {return m.handlePlanKey(key)}
 switch p.phase{
 case "tools":
  if p.editingFilter{
   switch key{
   case "enter","esc":p.editingFilter=false
   case "backspace":if len(p.filter)>0{p.filter=p.filter[:len(p.filter)-1]}
   case "ctrl+u":p.filter=""
   default:if len(key)==1&&len(p.filter)<80{p.filter+=key}
   }
   p.selected=0;p.detailScroll=0
   return m,nil
  }
  if key=="tab"||key=="shift+tab"{
   p.detailFocus=!p.detailFocus
   return m,nil
  }
  if p.detailFocus {
   tool,ok:=p.chosenTool()
   if !ok {p.detailFocus=false;return m,nil}
   width:=max(1,m.width-m.width*54/100-7)
   description:=wrap(tool.Description,width,1024)
   visible:=max(1,m.height-23)
   ceiling:=max(0,len(description)-visible)
   switch key {
   case "up","k":p.detailScroll=max(0,p.detailScroll-1)
   case "down","j":p.detailScroll=min(ceiling,p.detailScroll+1)
   case "pgup":p.detailScroll=max(0,p.detailScroll-visible)
   case "pgdown":p.detailScroll=min(ceiling,p.detailScroll+visible)
   case "home":p.detailScroll=0
   case "end":p.detailScroll=ceiling
   case "esc":p.detailFocus=false
   }
   return m,nil
  }
  switch key {
  case "esc","q":m.perm=permissionOverlay{}
  case "/":p.editingFilter=true
  case "g":
   pos:=0
   for i,group:=range permissionGroups{if group==p.group{pos=i;break}}
   p.group=permissionGroups[(pos+1)%len(permissionGroups)]
   p.selected=0;p.detailScroll=0
  case "up","k":p.selected=max(0,p.selected-1);p.detailScroll=0
  case "down","j":p.selected=min(max(0,len(p.visibleTools())-1),p.selected+1);p.detailScroll=0
  case "pgup":p.selected=max(0,p.selected-10);p.detailScroll=0
  case "pgdown":p.selected=min(max(0,len(p.visibleTools())-1),p.selected+10);p.detailScroll=0
  case "r":
   p.phase="loading";p.editingFilter=false;p.staged=nil;p.detailScroll=0
   return m,loadPermissions(m.client,p.profile,p.server)
  case " ", "space":
   p.stageSelected()
  case "c":
   p.staged=nil
  case "v":
   if len(p.staged)>0&&len(p.staged)<=12&&p.view.Mode!="unknown"{p.phase="review-plan"}
  case "e","d":
   if p.view.Mode=="unknown"||len(p.staged)>0{return m,nil}
   tool,ok:=p.chosenTool()
   if !ok{return m,nil}
   requested:="enable"
   if key=="d"{requested="disable"}
   if (requested=="enable"&&tool.Status=="enabled")||(requested=="disable"&&tool.Status=="disabled"){return m,nil}
   p.operation=requested;p.tool=tool.Name;p.phase="review-tool"
  }
 case "review-tool","review-remove":
  switch key{
  case "n","esc":
   if p.phase=="review-tool"{p.phase="tools";p.operation="";p.tool=""}else{m.perm=permissionOverlay{}}
  case "y","Y":
   p.phase="saving"
   return m,applyMCPPermission(m.client,p.profile,p.server,p.tool,p.operation,p.view)
  }
 case "complete","blocked","unverified","partial","error":
  switch key{
  case "esc","q","enter":
   profile:=p.profile
   m.perm=permissionOverlay{}
   if m.pm.open&&m.pm.stage==1&&m.pm.profileID==profile{
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
	if v.err!=nil{p.phase="error";p.errText=v.err.Error()}else{
		// Refresh keeps a local search but never reuses a stale tool selection.
		p.view=v.data;p.selected=0;p.detailScroll=0;p.staged=nil;p.phase="tools"
	}
	return m
}
func (m Model) permissionChanged(v permissionsChangedMsg)Model {
	p:=&m.perm
	if !p.open||p.phase!="saving"||p.profile!=v.profile||p.server!=v.server||p.tool!=v.tool||p.operation!=v.operation{return m}
	if v.err!=nil{p.phase="error";p.errText=v.err.Error()
	}else if v.verifyErr!=nil{p.phase="unverified";p.errText=v.verifyErr.Error()
	}else{p.phase="complete";p.view=v.after}
	return m
}
