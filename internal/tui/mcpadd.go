package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

// Catalog-to-profile assignment is a modal flow. Discovery and confirmation
// are separate from the mutating command; browsing never adds a server.
type catalogAddModel struct {
	open bool
	server string
	phase string // loading, choose, checking, review, saving, complete, error
	profiles []dockercli.MCPProfile
	selected int
	profile string
	errText string
}

type catalogAddProfilesMsg struct {
	server string
	profiles []dockercli.MCPProfile
	err error
}
type catalogAddMembersMsg struct {
	server, profile string
	members []dockercli.MCPProfileServer
	err error
}
type catalogAddCompletedMsg struct {
	server, profile string
	err error
}

func fetchAddProfiles(c dockercli.Client, server string) tea.Cmd {
	return func() tea.Msg {
		p,err:=c.MCPProfilesSafe(context.Background())
		return catalogAddProfilesMsg{server:server,profiles:p,err:err}
	}
}
func fetchAddMembers(c dockercli.Client, server, profile string) tea.Cmd {
	return func() tea.Msg {
		members,err:=c.MCPProfileMembers(context.Background(),profile)
		return catalogAddMembersMsg{server:server,profile:profile,members:members,err:err}
	}
}
func addCatalogMember(c dockercli.Client, server, profile string) tea.Cmd {
	return func() tea.Msg {
		err:=c.MCPAddCatalogServer(context.Background(),profile,server)
		return catalogAddCompletedMsg{server:server,profile:profile,err:err}
	}
}

func (m Model) catalogAddFooter() string {
	p:=m.catalogAdd
	switch p.phase {
	case "loading", "checking", "saving":
		return "[Wait] Checking MCP state  •  no other actions available"
	case "choose":
		return "[↑↓] Select profile  [Enter] Review assignment  [Esc] Cancel"
	case "review":
		return "[y] Add server to profile  [n/Esc] Cancel"
	case "complete":
		return "[Esc/Enter] Return to catalog  [a] View configured servers afterward"
	default:
		return "[Esc] Return to catalog"
	}
}
func (m Model) catalogAddView(w,h int) string {
	p:=m.catalogAdd
	lines:=[]string{
		cyan.Render("MCP CATALOG  /  ADD SERVER TO PROFILE"),"",
		"SERVER      "+plain(p.server,max(1,w-15)),
		"CATALOG     mcp/docker-mcp-catalog:latest","",
	}
	switch p.phase {
	case "loading":
		lines=append(lines,"Loading existing profiles (read-only)...")
	case "choose":
		lines=append(lines,cyan.Render(fmt.Sprintf("CHOOSE PROFILE  /  %d available",len(p.profiles))),"")
		if len(p.profiles)==0 {
			lines=append(lines,amber.Render("No profiles configured."),
				"Use [m] on the MCP Catalog to create an empty profile first.")
		} else {
			visible:=max(1,h-17)
			start:=max(0,p.selected-visible+1)
			if p.selected<visible {start=0}
			for i:=start;i<len(p.profiles)&&i<start+visible;i++{
				prof:=p.profiles[i]
				line:=prof.ID
				if prof.Name!=prof.ID {line+="  ("+plain(prof.Name,max(1,w-20))+")"}
				if i==p.selected{
					lines=append(lines,selectedStyle.Render(padded("> "+line,max(1,w-4))))
				}else{lines=append(lines,"  "+plain(line,max(1,w-7)))}
			}
		}
	case "checking":
		lines=append(lines,cyan.Render("PROFILE    "+p.profile),"",
			"Checking existing membership before review (read-only)...")
	case "review":
		ref,_:=dockercli.MCPCatalogServerRef(p.server)
		lines=append(lines,cyan.Render("REVIEW EXACT MCP CHANGE"),"",
			"PROFILE     "+p.profile,
			"SERVER      "+p.server,
			"REFERENCE   "+ref,"",
			amber.Render("This will CHANGE your Docker MCP profile configuration."),
			amber.Render("Docker MCP may expose server tools to connected clients."),
			"Review trust, scopes and server credentials before connecting agents.","",
			"Docktop will NOT start the MCP Gateway, call tools,",
			"enable-all tools, or install/run a container itself.","",
			red.Render("Press y to ADD exactly this server; n or Esc cancels."))
	case "saving":
		lines=append(lines,amber.Render("Applying approved server assignment..."),
			"PROFILE    "+p.profile,"SERVER     "+p.server,
			"Please wait for the Docker MCP result.")
	case "complete":
		lines=append(lines,green.Render("SERVER ADDED TO PROFILE"),"",
			"PROFILE    "+p.profile,"SERVER     "+p.server,"",
			"Return to the catalog, then use [a] to check configured servers.",
			"Review server tool permissions before connecting an AI client.")
	case "error":
		lines=append(lines,red.Render("MCP ASSIGNMENT NOT COMPLETED"),"",
			wrap(p.errText,max(1,w-4),max(1,h-12))...)
	}
	return frame("MCP / CATALOG SERVER ASSIGNMENT",lines,w,h)
}

func (m Model) catalogAddKey(key string) (tea.Model,tea.Cmd) {
	p:=&m.catalogAdd
	switch p.phase {
	case "loading","checking","saving":
		return m,nil // refuse closure during an in-flight operation
	case "choose":
		switch key {
		case "esc","q":
			m.catalogAdd=catalogAddModel{}
		case "up","k":
			p.selected=max(0,p.selected-1)
		case "down","j":
			p.selected=min(max(0,len(p.profiles)-1),p.selected+1)
		case "pgup":
			p.selected=max(0,p.selected-10)
		case "pgdown":
			p.selected=min(max(0,len(p.profiles)-1),p.selected+10)
		case "enter":
			if p.selected<len(p.profiles) {
				p.profile=p.profiles[p.selected].ID
				p.phase="checking"
				return m,fetchAddMembers(m.client,p.server,p.profile)
			}
		}
	case "review":
		switch key {
		case "y","Y":
			p.phase="saving"
			return m,addCatalogMember(m.client,p.server,p.profile)
		case "esc","n":
			m.catalogAdd=catalogAddModel{}
		}
	case "complete","error":
		switch key {
		case "esc","q","enter":
			m.catalogAdd=catalogAddModel{}
		}
	}
	return m,nil
}

// The view never exposes raw catalog fields or CLI configuration. A status
// transition can only be driven by a matching async result for the active flow.
func (m Model) catalogAddProfilesResult(v catalogAddProfilesMsg) Model {
	p:=&m.catalogAdd
	if !p.open || p.server!=v.server || p.phase!="loading" {return m}
	if v.err!=nil {p.phase="error";p.errText=v.err.Error();return m}
	p.profiles=v.profiles
	p.selected=0
	p.phase="choose"
	return m
}
func (m Model) catalogAddMembersResult(v catalogAddMembersMsg) Model {
	p:=&m.catalogAdd
	if !p.open||p.phase!="checking"||p.server!=v.server||p.profile!=v.profile {return m}
	if v.err!=nil {p.phase="error";p.errText="Membership lookup failed: "+v.err.Error();return m}
	for _,member:=range v.members {
		if member.Name==p.server {
			p.phase="error"
			p.errText="Server is already assigned to this profile; no change made."
			return m
		}
	}
	p.phase="review"
	return m
}
func (m Model) catalogAddCompleteResult(v catalogAddCompletedMsg) Model {
	p:=&m.catalogAdd
	if !p.open||p.phase!="saving"||p.server!=v.server||p.profile!=v.profile {return m}
	if v.err!=nil {p.phase="error";p.errText=v.err.Error()} else {p.phase="complete"}
	return m
}

// Reserved for future extension: never mutate profile permissions here.
var _ = strings.TrimSpace
