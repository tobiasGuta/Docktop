package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

type profileManager struct {
	open, busy, serverBusy, saving bool
	createEditing, createReview bool
	createName string
	stage int // 0 = select profile, 1 = select its server
	profiles []dockercli.MCPProfile
	servers []dockercli.MCPProfileServer
	profileIndex, serverIndex int
	profileID, action, target, errText, notice string
}

type managerProfilesMsg struct { values []dockercli.MCPProfile; err error }
type managerServersMsg struct { profile string; values []dockercli.MCPProfileServer; err error }
type managerChangedMsg struct { profile, server string; enable bool; err error }
type managerCreatedMsg struct { name string; err error }

func createManagerProfile(c dockercli.Client, name string) tea.Cmd {
	return func() tea.Msg {
		err := c.MCPCreateProfile(context.Background(), name)
		return managerCreatedMsg{name, err}
	}
}

func fetchManagerProfiles(c dockercli.Client) tea.Cmd {
	return func() tea.Msg { p,e:=c.MCPProfilesSafe(context.Background());return managerProfilesMsg{p,e} }
}
func fetchManagerServers(c dockercli.Client, profile string) tea.Cmd {
	return func() tea.Msg { s,e:=c.MCPProfileMembers(context.Background(),profile);return managerServersMsg{profile,s,e} }
}
func setManagerTools(c dockercli.Client,profile,server string,enable bool) tea.Cmd {
	return func() tea.Msg { err:=c.MCPSetToolAccess(context.Background(),profile,server,enable);return managerChangedMsg{profile,server,enable,err} }
}
func (m Model) managerFooter() string {
	p:=m.pm
	switch {
	case p.saving:
		return "Applying approved profile operation... please wait"
	case p.createReview:
		return "[y] Create EMPTY MCP profile  [n/Esc] Back to name"
	case p.createEditing:
		return "[Enter] Review new profile  [Esc] Cancel  [Backspace] Delete"
	case p.action!="":
		return "[y] Apply EXACT policy change  [n/Esc] Cancel"
	case p.stage==0:
		return "[↑↓] Choose profile  [Enter] Open  [c] Create empty profile  [r] Refresh  [Esc/m] Catalog"
	default:
		return "[↑↓] Choose server  [e] Enable ALL tools  [d] Disable ALL tools  [r] Refresh  [Esc] Profiles  [m] Catalog"
	}
}
func (m Model) managerView(w,h int) string {
	p:=m.pm
	title:="MCP PROFILES / TOOL PERMISSIONS"
	items:=[]string{}
	side:=[]string{}
	if p.errText!="" {
		side=append(side,red.Render("MCP data unavailable"))
		side=append(side,wrap(p.errText,max(15,w/2-6),3)...)
		side=append(side,"")
	}
	if p.notice!="" {
		side=append(side,green.Render("LAST ACTION"))
		side=append(side,wrap(p.notice,max(15,w/2-6),3)...)
		side=append(side,"")
	}
	if p.createEditing || p.createReview {
		title="MCP / CREATE EMPTY PROFILE"
		lines:=[]string{cyan.Render("CREATE MCP PROFILE"),"",
			"Name: "+plain(p.createName+"_",max(2,w-8)),"",
			"Allowed: letters, numbers, dots, underscores, hyphens.",
			"Start with a letter or number. Maximum 48 characters.","",
			"No servers or tool permissions will be configured.","",
			"Experimental profiles must first be enabled on Fedora:",
			"docker mcp feature enable profiles","",
		}
		if p.createReview {
			lines=append(lines,amber.Render("CONFIRM PROFILE CREATION"),"Name: "+p.createName,
				"Action: docker mcp profile create --name "+p.createName,"",
				"Press y to create, n or Esc to edit the name.")
		}else{
			lines=append(lines,"Type a profile name; press Enter to review.")
		}
		if p.errText!=""{lines=append(lines,red.Render(plain(p.errText,max(1,w-4))))}
		return frame(title,lines,w,h)
	}
	if p.stage==0 {
		items=append(items,cyan.Render(fmt.Sprintf("SELECT PROFILE  /  %d available",len(p.profiles))),"")
		if p.busy {items=append(items,"Loading profiles...")}
		for i,prof:=range p.profiles {
			line:=fmt.Sprintf(" %s  %s",prof.ID,prof.Name)
			if i==p.profileIndex {items=append(items,selectedStyle.Render(padded("> "+line,max(10,w/2-6))))} else {items=append(items,line)}
		}
		if !p.busy && len(p.profiles)==0 {
			items=append(items,amber.Render("No MCP profiles configured."),"",
				"Your standalone Docker Engine may have profiles disabled.",
				"First, run this in Fedora:",
				cyan.Render("docker mcp feature enable profiles"),"",
				"Then press [c] to create an empty profile.",
				"Refresh with [r] after changing the Docker MCP setting.")
		}
		side=append(side,cyan.Render("PROFILE TOOL POLICY"),"",
			"Select an existing MCP profile to inspect its assigned servers.",
			"",
			"Changes affect tool exposure for that profile. They do not install",
			"or run a server, connect a client, or remove configuration.",
			"",
			amber.Render("Enabling all tools may expose sensitive capabilities."),
			"Every change requires explicit review and confirmation.")
	} else {
		title="MCP SERVER TOOL POLICY / "+p.profileID
		items=append(items,cyan.Render(fmt.Sprintf("SERVERS  /  %d assigned",len(p.servers))),"")
		if p.serverBusy {items=append(items,"Checking profile membership...")}
		for i,server:=range p.servers{
			row:=" "+server.Name
			if i==p.serverIndex {items=append(items,selectedStyle.Render(padded("> "+row,max(10,w/2-6))))}else{items=append(items,row)}
		}
		if !p.serverBusy && len(p.servers)==0 {items=append(items,muted.Render("No assigned servers (or format unsupported)."))}
		side=append(side,cyan.Render("SELECTED SERVER"),"")
		if p.serverIndex<len(p.servers) {side=append(side,plain(p.servers[p.serverIndex].Name,max(12,w/2-6)))}
		side=append(side,"",green.Render("[e] ENABLE ALL TOOLS"),red.Render("[d] DISABLE ALL TOOLS"),"",
			"Does not add/remove server membership.",
			"Current per-tool allowlist is not inferred from this list.",
			"Operations take effect in the selected profile and may affect",
			"already connected clients.",
			"",
			muted.Render("No changes until a confirmation is approved."))
	}
	if p.action!="" {
		name:="ENABLE ALL TOOLS"
		if p.action=="disable" {name="DISABLE ALL TOOLS"}
		warning:="This may grant access to every tool exposed by this server."
		if p.action=="disable" {warning="This will deny all tools from this server in the profile."}
		return frame("CONFIRM MCP TOOL POLICY",[]string{
			amber.Render("Review the exact permission change"),"",
			"PROFILE  "+p.profileID,
			"SERVER   "+p.target,
			"ACTION   "+name,"",warning,"",
			"No server installation, execution, or profile removal.","",
			red.Render("Press y to apply, n or Esc to cancel."),
		},w,h)
	}
	if w<90 || h<16 {
		lines:=append(items,"")
		if len(side)>0{lines=append(lines,trimLines(side,max(0,h-len(lines)-2))...)}
		return frame(title,lines,w,h)
	}
	left:=max(36,w*45/100)
	right:=w-left-1
	return lipgloss.JoinHorizontal(lipgloss.Top,
		frame(title,items,left,h)," ",
		frame("POLICY DETAILS / REVIEW",side,right,h))
}

func (m Model) managerKey(key string)(tea.Model,tea.Cmd){
	p:=&m.pm
	if p.saving {return m,nil}
	if p.createReview {
		switch key {
		case "n","esc":
			p.createReview=false
			p.createEditing=true
		case "y","Y":
			if !dockercli.ValidMCPProfileName(p.createName) {
				p.errText="Invalid profile name; return to edit it."
				return m,nil
			}
			name:=p.createName
			p.createEditing=false
			p.createReview=false
			p.saving=true
			p.errText=""
			p.notice="Creating empty profile "+name+"..."
			return m,createManagerProfile(m.client,name)
		}
		return m,nil
	}
	if p.createEditing {
		switch key {
		case "esc":
			p.createEditing=false;p.createName="";p.errText=""
		case "enter":
			if !dockercli.ValidMCPProfileName(p.createName) {
				p.errText="Use 1-48 characters: letters/numbers then letters, numbers, . _ -"
			}else{p.errText="";p.createReview=true;p.createEditing=false}
		case "backspace":
			if len(p.createName)>0 {p.createName=p.createName[:len(p.createName)-1]}
		default:
			if len(key)==1 && len(p.createName)<48 && strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-",rune(key[0])) {
				p.createName+=key
			}
		}
		return m,nil
	}
	if p.action!="" {
		switch key {
		case "n","esc":
			p.action,p.target="",""
		case "y","Y":
			profile,server,enable:=p.profileID,p.target,p.action=="enable"
			p.action,p.target="",""
			p.saving=true
			p.errText=""
			p.notice="Applying approved tool-policy change..."
			return m,setManagerTools(m.client,profile,server,enable)
		}
		return m,nil
	}
	switch key{
	case "c":
		if p.stage==0 && !p.busy {
			p.createEditing=true;p.createName="";p.createReview=false;p.errText="";p.notice=""
		}
	case "m","q":
		m.pm=profileManager{}
		return m,nil
	case "esc":
		if p.stage==1 {
			p.stage=0;p.profileID="";p.servers=nil;p.serverIndex=0;p.errText=""
		}else{m.pm=profileManager{}}
	case "r":
		p.errText="";p.notice=""
		if p.stage==0{
			p.busy=true
			return m,fetchManagerProfiles(m.client)
		}
		p.serverBusy=true
		return m,fetchManagerServers(m.client,p.profileID)
	case "up","k":
		if p.stage==0 {p.profileIndex=max(0,p.profileIndex-1)} else {p.serverIndex=max(0,p.serverIndex-1)}
	case "down","j":
		if p.stage==0 {p.profileIndex=min(max(0,len(p.profiles)-1),p.profileIndex+1)} else {p.serverIndex=min(max(0,len(p.servers)-1),p.serverIndex+1)}
	case "pgup":
		if p.stage==0 {p.profileIndex=max(0,p.profileIndex-10)} else {p.serverIndex=max(0,p.serverIndex-10)}
	case "pgdown":
		if p.stage==0 {p.profileIndex=min(max(0,len(p.profiles)-1),p.profileIndex+10)} else {p.serverIndex=min(max(0,len(p.servers)-1),p.serverIndex+10)}
	case "enter":
		if p.stage==0 && !p.busy && p.profileIndex<len(p.profiles){
			p.profileID=p.profiles[p.profileIndex].ID
			p.stage=1;p.servers=nil;p.serverIndex=0;p.serverBusy=true;p.errText="";p.notice=""
			return m,fetchManagerServers(m.client,p.profileID)
		}
	case "e","d":
		if p.stage==1 && !p.serverBusy && p.serverIndex<len(p.servers){
			p.target=p.servers[p.serverIndex].Name
			p.action="enable"
			if key=="d" {p.action="disable"}
		}
	}
	return m,nil
}
