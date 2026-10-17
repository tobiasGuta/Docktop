package tui

import (
 "context"
 "fmt"

 tea "github.com/charmbracelet/bubbletea"
 "github.com/charmbracelet/lipgloss"
 "github.com/tobiasGuta/Docktop/internal/dockercli"
)

// A client configuration is not an active MCP connection or a live session.
// This inspector never starts gateways, connects clients or reads credentials.
type mcpClientBrowser struct {
 open,busy bool
 items []dockercli.MCPClientIntegration
 selected int
 errorText string
}
type configuredClientsMsg struct {
 values []dockercli.MCPClientIntegration
 err error
}
func fetchConfiguredClients(c dockercli.Client)tea.Cmd{
 return func() tea.Msg{
  items,err:=c.MCPConfiguredClients(context.Background())
  return configuredClientsMsg{values:items,err:err}
 }
}
func (m Model) configuredClientResult(v configuredClientsMsg) Model {
 if !m.clients.open||!m.clients.busy{return m}
 m.clients.busy=false
 m.clients.items=v.values
 m.clients.selected=0
 if v.err!=nil {
  m.clients.items=nil
  // Do not display raw CLI JSON or potentially sensitive command output.
  m.clients.errorText="Unable to read client configurations. Check docker mcp client ls --global --json in another terminal."
 }else{m.clients.errorText=""}
 return m
}
func (m Model) configuredClientKey(key string)(tea.Model,tea.Cmd){
 p:=&m.clients
 switch key {
 case "esc","q":
  m.clients=mcpClientBrowser{}
 case "r":
  if !p.busy{p.busy=true;p.errorText="";return m,fetchConfiguredClients(m.client)}
 case "up","k":
  if !p.busy{p.selected=max(0,p.selected-1)}
 case "down","j":
  if !p.busy{p.selected=min(max(0,len(p.items)-1),p.selected+1)}
 case "pgup":
  if !p.busy{p.selected=max(0,p.selected-10)}
 case "pgdown":
  if !p.busy{p.selected=min(max(0,len(p.items)-1),p.selected+10)}
 }
 return m,nil
}
func (m Model) configuredClientFooter()string {
 return "[Esc] MCP Catalog  [↑↓] Browse integrations  [r] Refresh  •  Read-only"
}
func (m Model) configuredClientView(w,h int)string {
 p:=m.clients
 head:=[]string{cyan.Render("CONFIGURED MCP CLIENT INTEGRATIONS"),"",
  amber.Render("Configured does NOT mean an active or authenticated session."),
  muted.Render("Source: docker mcp client ls --global --json (read-only)"),""}
 if p.busy {head=append(head,muted.Render("Reading local MCP client configuration..."))}
 if p.errorText!=""{head=append(head,red.Render(plain(p.errorText,max(1,w-4))))}
 if !p.busy&&p.errorText==""&&len(p.items)==0{head=append(head,muted.Render("Docker MCP returned no configured client entries."))}
 if w<88||h<19{
  rows:=head
  for i,item:=range p.items {
   marker:=" "
   if i==p.selected{marker=">"}
   rows=append(rows,plain(fmt.Sprintf("%s %s  [%s]  profile=%s",marker,item.Label,item.State,item.Profile),max(1,w-4)))
  }
  return frame("MCP / CLIENT CONFIGURATIONS",rows,w,h)
 }
 leftWidth:=max(41,w*52/100)
 rightWidth:=w-leftWidth-1
 list:=[]string{cyan.Render(fmt.Sprintf("CLIENTS / %d",len(p.items))),"",
  muted.Render("Configuration only; no live-session claims."),""}
 if p.busy{list=append(list,muted.Render("Reading configured MCP clients..."))}
 if p.errorText!=""{list=append(list,red.Render(plain(p.errorText,max(1,leftWidth-4))))}
 capacity:=max(1,h-len(list)-3)
 start:=max(0,p.selected-capacity+1)
 for i:=start;i<len(p.items)&&i<start+capacity;i++{
  item:=p.items[i]
  label:=plain(item.Label+"  ["+item.State+"]",max(1,leftWidth-6))
  if i==p.selected{
   list=append(list,selectedStyle.Render(padded("> "+label,max(1,leftWidth-3))))
  }else{list=append(list,"  "+label)}
 }
 right:=[]string{cyan.Render("SELECTED CONFIGURATION"),"",
  amber.Render("Not live connection telemetry."),""}
 if !p.busy && p.errorText=="" && p.selected<len(p.items) {
  item:=p.items[p.selected]
  right=append(right,"NAME     "+plain(item.Label,max(1,rightWidth-13)),
   "CLIENT   "+plain(item.ID,max(1,rightWidth-13)),
   "STATE    "+item.State,
   "PROFILE  "+plain(item.Profile,max(1,rightWidth-13)),"",
   "Docker MCP reports local client setup, not whether",
   "a process is connected or currently calling tools.")
 }
 return lipgloss.JoinHorizontal(lipgloss.Top,frame("MCP / CONFIGURED CLIENTS",list,leftWidth,h)," ",
  frame("DETAILS / READ ONLY",right,rightWidth,h))
}
