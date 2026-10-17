package tui

import (
 "context"
 "fmt"
 "strings"
 "unicode/utf8"

 tea "github.com/charmbracelet/bubbletea"
 "github.com/charmbracelet/lipgloss"
 "github.com/tobiasGuta/Docktop/internal/dockercli"
)

// The MCP client's *configuration*, not a live process or active session.
// Profile choice + separate y confirmation are required to edit client config.
type mcpClientBrowser struct {
 open,busy bool
 items []dockercli.MCPClientIntegration
 selected int // index in locally filtered list
 errorText, query, notice string
 editing bool
 phase string // browse, load-profiles, choose-profile, review, saving, result
 profiles []dockercli.MCPProfile
 profileSelected int
 clientID,profileID string
 outcome string
}
type configuredClientsMsg struct {
 values []dockercli.MCPClientIntegration
 err error
}
type clientProfilesLoadedMsg struct {
 clientID string
 values []dockercli.MCPProfile
 err error
}
type clientConnectDoneMsg struct{
 clientID, profileID string
 result dockercli.MCPClientConnectResult
}
func fetchConfiguredClients(c dockercli.Client)tea.Cmd{
 return func() tea.Msg {
  items,err:=c.MCPConfiguredClients(context.Background())
  return configuredClientsMsg{values:items,err:err}
 }
}
func fetchClientProfiles(c dockercli.Client,clientID string)tea.Cmd{
 return func()tea.Msg{
  profiles,err:=c.MCPProfilesSafe(context.Background())
  return clientProfilesLoadedMsg{clientID:clientID,values:profiles,err:err}
 }
}
func connectMCPClient(c dockercli.Client,clientID,profileID string)tea.Cmd{
 return func()tea.Msg{
  result:=c.MCPConnectGlobalClient(context.Background(),clientID,profileID)
  return clientConnectDoneMsg{clientID:clientID,profileID:profileID,result:result}
 }
}
func (p mcpClientBrowser) filtered()[]dockercli.MCPClientIntegration{
 query:=strings.ToLower(strings.TrimSpace(p.query))
 if query==""{return p.items}
 out:=[]dockercli.MCPClientIntegration{}
 for _,item:=range p.items{
  if strings.Contains(strings.ToLower(item.Label),query)||
   strings.Contains(strings.ToLower(item.ID),query)||
   strings.Contains(strings.ToLower(item.State),query)||
   strings.Contains(strings.ToLower(item.Profile),query){
   out=append(out,item)
  }
 }
 return out
}
func (p mcpClientBrowser) selectedClient()(dockercli.MCPClientIntegration,bool){
 visible:=p.filtered()
 if p.selected<0||p.selected>=len(visible){return dockercli.MCPClientIntegration{},false}
 return visible[p.selected],true
}
func (m Model) configuredClientResult(v configuredClientsMsg)Model{
 p:=&m.clients
 if !p.open||!p.busy||p.phase!=""&&p.phase!="browse"{return m}
 p.busy=false
 if v.err!=nil{
  p.items=nil
  p.errorText="Unable to read client configurations. No changes made."
 }else{
  p.errorText=""
  p.items=v.values
  p.selected=0
 }
 return m
}
func (m Model) clientProfilesResult(v clientProfilesLoadedMsg)Model{
 p:=&m.clients
 if !p.open||p.phase!="load-profiles"||p.clientID!=v.clientID{return m}
 if v.err!=nil{
  p.phase="browse"
  p.notice="Unable to load existing profiles. Nothing changed."
  return m
 }
 p.profiles=v.values;p.profileSelected=0
 if len(v.values)==0{
  p.phase="browse"
  p.notice="No MCP profiles available; create one first in the Profile Manager."
 }else{p.phase="choose-profile"}
 return m
}
func (m Model) clientConnectedResult(v clientConnectDoneMsg)Model{
 p:=&m.clients
 if !p.open||p.phase!="saving"||p.clientID!=v.clientID||p.profileID!=v.profileID{return m}
 p.phase="result";p.outcome=v.result.Outcome
 switch v.result.Outcome {
 case "verified":
  p.notice="Docker MCP readback verified the exact client and profile."
  p.items=v.result.Updated
 case "blocked":
  p.notice="Connection blocked before any write: "+v.result.Reason
 default:
  p.outcome="unverified"
  p.notice="Connection result unverified. Inspect the client's configuration before retrying."
 }
 return m
}
func (m Model) configuredClientKey(key string)(tea.Model,tea.Cmd){
 p:=&m.clients
 if p.busy||p.phase=="load-profiles"||p.phase=="saving"{return m,nil}
 if p.phase=="choose-profile"{
  switch key{
  case "esc","n":p.phase="browse"
  case "up","k":p.profileSelected=max(0,p.profileSelected-1)
  case "down","j":p.profileSelected=min(max(0,len(p.profiles)-1),p.profileSelected+1)
  case "pgup":p.profileSelected=max(0,p.profileSelected-10)
  case "pgdown":p.profileSelected=min(max(0,len(p.profiles)-1),p.profileSelected+10)
  case "enter":
   if p.profileSelected>=0&&p.profileSelected<len(p.profiles){
    p.profileID=p.profiles[p.profileSelected].ID
    p.phase="review"
   }
  }
  return m,nil
 }
 if p.phase=="review"{
  switch key{
  case "esc","n":p.phase="choose-profile"
  case "y","Y":
   p.phase="saving"
   return m,connectMCPClient(m.client,p.clientID,p.profileID)
  }
  return m,nil
 }
 if p.phase=="result"{
  if key=="esc"||key=="enter"||key=="q"{p.phase="browse";p.selected=0;p.notice=""}
  return m,nil
 }
 if p.editing{
  switch key{
  case "esc","enter":p.editing=false
  case "ctrl+u":p.query=""
  case "backspace":
   if len(p.query)>0 {_,n:=utf8.DecodeLastRuneInString(p.query);p.query=p.query[:len(p.query)-n]}
  default:
   if len(key)==1&&len(p.query)<80{p.query+=key}
  }
  p.selected=0
  return m,nil
 }
 switch key{
 case "esc","q":m.clients=mcpClientBrowser{}
 case "/":p.editing=true;p.notice=""
 case "r":p.busy=true;p.errorText="";p.notice="";return m,fetchConfiguredClients(m.client)
 case "up","k":p.selected=max(0,p.selected-1)
 case "down","j":p.selected=min(max(0,len(p.filtered())-1),p.selected+1)
 case "pgup":p.selected=max(0,p.selected-10)
 case "pgdown":p.selected=min(max(0,len(p.filtered())-1),p.selected+10)
 case "c":
  selected,ok:=p.selectedClient()
  if !ok{return m,nil}
  if err:=dockercli.MCPPlausibleGlobalClient(selected);err!=nil{
   p.notice="Connection unavailable: "+err.Error()+". No changes made."
   return m,nil
  }
  p.clientID=selected.ID;p.profileID="";p.notice=""
  p.phase="load-profiles"
  return m,fetchClientProfiles(m.client,p.clientID)
 }
 return m,nil
}
func (m Model) configuredClientFooter()string{
 p:=m.clients
 switch p.phase {
 case "load-profiles","saving":return "[Wait] MCP configuration operation in progress"
 case "choose-profile":return "[Esc] Cancel  [↑↓] Choose existing profile  [Enter] Review"
 case "review":return "[y] Connect selected client/profile  [n/Esc] Cancel"
 case "result":return "[Esc/Enter] Return to configured client browser"
 }
 if p.editing {return "[Enter] Apply search  [Esc] End search  [Ctrl+U] Clear"}
 return "[Esc] Catalog  [/] Search  [↑↓] Clients  [c] Connect  [r] Refresh"
}
func (m Model) configuredClientView(w,h int)string {
 p:=m.clients
 if p.phase=="load-profiles"||p.phase=="choose-profile"||p.phase=="review"||p.phase=="saving"||p.phase=="result"{
  return m.clientConnectView(w,h)
 }
 filtered:=p.filtered()
 if w<88||h<19{
  lines:=[]string{cyan.Render(fmt.Sprintf("CONFIGURED CLIENTS  /  %d matched",len(filtered))),"",
   amber.Render("Setup status only; no live-session claims."),
   "Search: "+plain(p.query,max(1,w-13))}
  if p.busy{lines=append(lines,muted.Render("Loading MCP client configurations..."))}
  if p.errorText!=""{lines=append(lines,red.Render(plain(p.errorText,max(1,w-4))))}
  if p.notice!=""{lines=append(lines,amber.Render(plain(p.notice,max(1,w-4))))}
  if len(filtered)==0&&!p.busy{lines=append(lines,muted.Render("No matching clients."))}
  capacity:=max(1,h-len(lines)-3)
  start:=max(0,p.selected-capacity+1)
  for i:=start;i<len(filtered)&&i<start+capacity;i++{
   item:=filtered[i]
   prefix:=" ";if i==p.selected{prefix=">"}
   line:=plain(fmt.Sprintf("%s %s [%s] %s",prefix,item.Label,item.State,item.Installed),max(1,w-4))
   if i==p.selected{lines=append(lines,selectedStyle.Render(line))}else{lines=append(lines,line)}
  }
  return frame("MCP / CLIENT CONFIGURATIONS",lines,w,h)
 }
 leftWidth:=max(41,w*52/100);rightWidth:=w-leftWidth-1
 list:=[]string{cyan.Render(fmt.Sprintf("CLIENTS  /  %d MATCHED",len(filtered))),"",
  muted.Render("Configured != active session."),
  muted.Render(plain("SEARCH  "+searchLabel(p.query,p.editing),max(1,leftWidth-4))),""}
 if p.busy{list=append(list,muted.Render("Loading client configuration..."))}
 if p.errorText!=""{list=append(list,red.Render(plain(p.errorText,max(1,leftWidth-4))))}
 if p.notice!=""{list=append(list,amber.Render(plain(p.notice,max(1,leftWidth-4))))}
 if len(filtered)==0&&!p.busy{list=append(list,muted.Render("No matching clients."))}
 capacity:=max(1,h-len(list)-3);start:=max(0,p.selected-capacity+1)
 for i:=start;i<len(filtered)&&i<start+capacity;i++{
  item:=filtered[i]
  label:=plain(item.Label+"  ["+item.State+"]",max(1,leftWidth-6))
  if i==p.selected{list=append(list,selectedStyle.Render(padded("> "+label,max(1,leftWidth-3))))
  }else{list=append(list,"  "+label)}
 }
 right:=[]string{cyan.Render("SELECTED CLIENT"),"",
  amber.Render("Not live connection telemetry."),""}
 if item,ok:=p.selectedClient();ok&&!p.busy&&p.errorText=="" {
  status:=muted.Render(item.State)
  if item.State=="configured"{status=green.Render(item.State)}
  right=append(right,"NAME       "+plain(item.Label,max(1,rightWidth-16)),
   "CLIENT     "+plain(item.ID,max(1,rightWidth-16)),
   "STATE      "+status,
   "INSTALLED  "+item.Installed,
   "OS SUPPORT "+item.OSSupport,
   "PROFILE    "+plain(item.Profile,max(1,rightWidth-16)),"")
  if item.HasConfigError {right=append(right,red.Render("Docker MCP reports a client configuration error."))}
  if dockercli.MCPPlausibleGlobalClient(item)==nil{
   right=append(right,green.Render("[c] Connect this client to an existing profile"),"",
    "Requires profile selection, exact review, and y confirmation.")
  }else{
   right=append(right,muted.Render("Connection unavailable until installation, OS support,"),
    muted.Render("and unconfigured status are verified."),
    "Already connected? Use Docker MCP CLI to review changes.")
  }
  right=append(right,"","A connection writes client configuration, but does not",
   "prove a client is running or authenticated.")
 }
 return lipgloss.JoinHorizontal(lipgloss.Top,
  frame("MCP / CONFIGURED CLIENTS",list,leftWidth,h)," ",
  frame("DETAILS / GLOBAL CONFIG",right,rightWidth,h))
}
func (m Model) clientConnectView(w,h int)string {
 p:=m.clients
 lines:=[]string{cyan.Render("DOCKER MCP / GLOBAL CLIENT CONFIGURATION"),"",
  "CLIENT  "+plain(p.clientID,max(1,w-12)),""}
 switch p.phase {
 case "load-profiles":
  lines=append(lines,muted.Render("Loading existing profiles; no client changes made..."))
 case "choose-profile":
  lines=append(lines,cyan.Render("CHOOSE AN EXISTING MCP PROFILE"),"",
   fmt.Sprintf("%d profiles available",len(p.profiles)),"")
  capacity:=max(1,h-len(lines)-4);start:=max(0,p.profileSelected-capacity+1)
  for i:=start;i<len(p.profiles)&&i<start+capacity;i++{
   name:=p.profiles[i].ID
   if p.profiles[i].Name!=""&&p.profiles[i].Name!=name{name+=" ("+p.profiles[i].Name+")"}
   line:=plain(name,max(1,w-9))
   if i==p.profileSelected{lines=append(lines,selectedStyle.Render(padded("> "+line,max(1,w-4))))
   }else{lines=append(lines,"  "+line)}
  }
 case "review":
  lines=append(lines,cyan.Render("REVIEW EXACT CLIENT CONNECTION"),"",
   "CLIENT       "+p.clientID,
   "SCOPE        global",
   "PROFILE      "+p.profileID,
   "COMMAND      docker mcp client connect --global",
   "             --profile "+p.profileID+" --quiet "+p.clientID,"",
   amber.Render("This WILL MODIFY the selected MCP client's local config file."),
   amber.Render("The approved profile's MCP tools may become available to that client."),
   "Your profile's current tool allowlist controls advertised tool access.",
   "Docker may require restarting the client to apply configuration.",
   "Docktop will not execute tools, open a gateway session,",
   "authenticate a client, or connect any other applications.","",
   red.Render("Press y to CONNECT ONLY this client and profile; n/Esc cancels."))
 case "saving":
  lines=append(lines,amber.Render("Applying approved client connection..."),
   "Docker MCP will be read again to verify the exact profile association.")
 case "result":
  switch p.outcome{
  case "verified":
   lines=append(lines,green.Render("CLIENT CONFIGURATION VERIFIED"),"")
  case "blocked":
   lines=append(lines,amber.Render("CONNECTION BLOCKED BEFORE ANY WRITE"),"")
  default:
   lines=append(lines,red.Render("CONNECTION OUTCOME NOT VERIFIED"),"",
    "Docker may have modified client configuration. Do not retry",
    "without checking the client's current configuration.")
  }
  lines=append(lines,wrap(p.notice,max(1,w-4),max(1,h-11))...)
  lines=append(lines,"","CLIENT  "+p.clientID,"PROFILE "+p.profileID)
 }
 return frame("MCP / CLIENT CONNECTION REVIEW",lines,w,h)
}
