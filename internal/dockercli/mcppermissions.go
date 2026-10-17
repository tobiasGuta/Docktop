package dockercli

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "slices"
)

// MCPToolPermission is allowlisted metadata, never raw profile configuration.
type MCPToolPermission struct { Name, Description, Status, CapabilityHint string }
type MCPPermissionView struct {
 Profile, Server, Mode string // all, none, allowlist, unknown
 Tools []MCPToolPermission
}

// MCPReadPermissions reads a profile JSON snapshot but exposes ONLY selected
// tool metadata. The raw output may contain config or secret references.
func (c Client) MCPReadPermissions(ctx context.Context, profile, server string) (MCPPermissionView, error) {
 if !identifier(profile) || !identifier(server) {return MCPPermissionView{},errors.New("invalid MCP target")}
 raw,err:=c.run(ctx,"mcp","profile","show",profile,"--format","json")
 if err!=nil{return MCPPermissionView{},err}
 return parseMCPPermissions(raw,profile,server)
}
func parseMCPPermissions(raw []byte,profile,server string)(MCPPermissionView,error){
 if len(raw)>1024*1024{return MCPPermissionView{},errors.New("profile metadata exceeds limit")}
 var document struct {
  ID string
  Servers []struct {
   Tools json.RawMessage
   Snapshot *struct {Server struct {
    Name string
    Tools []struct {Name,Description string; Annotations *struct { ReadOnlyHint, DestructiveHint *bool }}
   }}
  }
 }
 if err:=json.Unmarshal(raw,&document);err!=nil{return MCPPermissionView{},errors.New("unrecognized MCP profile JSON")}
 if document.ID!=profile || document.Servers==nil{return MCPPermissionView{},errors.New("profile identity or server list missing")}
 view:=MCPPermissionView{Profile:profile,Server:server,Mode:"unknown"}
 found:=false
 for _,s:=range document.Servers{
  if s.Snapshot==nil || s.Snapshot.Server.Name!=server {continue}
  if found{return MCPPermissionView{},errors.New("duplicate server snapshot name")}
  found=true
  if len(s.Tools)==0 {continue} // missing != null
  var enabled []string
  if bytes.Equal(bytes.TrimSpace(s.Tools),[]byte("null")){view.Mode="all"}else{
   if err:=json.Unmarshal(s.Tools,&enabled);err!=nil||enabled==nil{return MCPPermissionView{},errors.New("unrecognized tool allowlist")}
   view.Mode="allowlist"
   if len(enabled)==0{view.Mode="none"}
   for _,name:=range enabled{if !identifier(name){return MCPPermissionView{},errors.New("invalid allowlist tool name")}}
  }
  seen:=map[string]bool{}
  for _,t:=range s.Snapshot.Server.Tools{
   if !identifier(t.Name)||seen[t.Name]{return MCPPermissionView{},errors.New("invalid/duplicate snapshot tool")}
   seen[t.Name]=true
   state:="disabled"
   if view.Mode=="all"||slices.Contains(enabled,t.Name){state="enabled"}
   hint:="unspecified"
   if t.Annotations!=nil {
    switch {
    case t.Annotations.DestructiveHint!=nil && *t.Annotations.DestructiveHint:
     hint="destructive"
    case t.Annotations.ReadOnlyHint!=nil && *t.Annotations.ReadOnlyHint:
     hint="read-only"
    case t.Annotations.ReadOnlyHint!=nil && !*t.Annotations.ReadOnlyHint:
     hint="other"
    }
   }
   view.Tools=append(view.Tools,MCPToolPermission{Name:t.Name,Description:Clean(t.Description,4096),Status:state,CapabilityHint:hint})
  }
  for _,name:=range enabled{if !seen[name]{return MCPPermissionView{},errors.New("allowlisted tool missing from server snapshot")}}
 }
 if !found{return MCPPermissionView{},errors.New("server snapshot absent from profile")}
 return view,nil
}
func (c Client) requireMCPMember(ctx context.Context,profile,server string)error{
 profiles,err:=c.MCPProfilesSafe(ctx)
 if err!=nil{return fmt.Errorf("profile recheck: %w",err)}
 exists:=false
 for _,p:=range profiles{if p.ID==profile{exists=true;break}}
 if !exists{return errors.New("profile no longer exists")}
 members,err:=c.MCPProfileMembers(ctx,profile)
 if err!=nil{return fmt.Errorf("server membership recheck: %w",err)}
 for _,s:=range members{if s.Name==server{return nil}}
 return errors.New("server no longer assigned to profile")
}
// MCPRemoveProfileServer removes only selected profile membership, not a
// profile, catalog or Docker container. Docker may clean orphaned OAuth data.
func (c Client) MCPRemoveProfileServer(ctx context.Context,profile,server string)error{
 if !identifier(profile)||!identifier(server){return errors.New("invalid MCP target")}
 if err:=c.requireMCPMember(ctx,profile,server);err!=nil{return err}
 _,err:=c.run(ctx,"mcp","profile","server","remove",profile,"--name",server)
 return err
}
// MCPSetSpecificTool changes exactly one tool after a fresh read.
// In all-enabled mode, re-enabling would narrow access to one tool in the
// Docker MCP implementation, so already-enabled actions are rejected.
func (c Client) MCPSetSpecificTool(ctx context.Context,profile,server,tool string,enable bool)error{
 if !identifier(profile)||!identifier(server)||!identifier(tool){return errors.New("invalid MCP tool target")}
 if err:=c.requireMCPMember(ctx,profile,server);err!=nil{return err}
 p,err:=c.MCPReadPermissions(ctx,profile,server)
 if err!=nil{return err}
 if p.Mode=="unknown"||len(p.Tools)==0{return errors.New("tool list or permission status unavailable; no changes made")}
 current:=""
 for _,t:=range p.Tools{if t.Name==tool{current=t.Status;break}}
 if current==""{return errors.New("selected tool absent from server snapshot")}
 if enable && current=="enabled"{return errors.New("tool already enabled")}
 if !enable && current=="disabled"{return errors.New("tool already disabled")}
 flag:="--disable"
 if enable{flag="--enable"}
 _,err=c.run(ctx,"mcp","profile","tools",profile,flag,server+"."+tool)
 return err
}
