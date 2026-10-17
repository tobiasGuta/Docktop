package dockercli

import (
 "context"
 "encoding/json"
 "errors"
 "sort"
)

// MCPClientIntegration is a local configuration observation, NOT a live
// connection, process or proof of an authenticated client session.
type MCPClientIntegration struct {
 ID, Label, Profile, State string
 Installed, OSSupport string
 HasConfigError bool
}
func (c Client) MCPConfiguredClients(ctx context.Context)([]MCPClientIntegration,error){
 raw,err:=c.run(ctx,"mcp","client","ls","--global","--json")
 if err!=nil{return nil,err}
 return parseMCPClientIntegrations(raw)
}
func parseMCPClientIntegrations(raw []byte)([]MCPClientIntegration,error){
 if len(raw)>1024*1024{return nil,errors.New("MCP client configuration exceeds limit")}
 var document map[string]json.RawMessage
 if err:=json.Unmarshal(raw,&document);err!=nil||document==nil{
  return nil,errors.New("unsupported MCP client JSON object")
 }
 if len(document)>200{return nil,errors.New("too many MCP clients")}
 result:=make([]MCPClientIntegration,0,len(document))
 for id,entry:=range document {
  if !identifier(id){return nil,errors.New("invalid MCP client ID")}
  var fields struct {
   DisplayName string `json:"displayName"`
   Profile string `json:"profile"`
   Connected *bool `json:"dockerMCPCatalogConnected"`
   Installed *bool `json:"isInstalled"`
   OSSupported *bool `json:"isOsSupported"`
   ConfigError json.RawMessage `json:"error"`
  }
  if err:=json.Unmarshal(entry,&fields);err!=nil{return nil,errors.New("malformed MCP client record")}
  state:="unknown"
  if fields.Connected!=nil{
   state="not configured"
   if *fields.Connected{state="configured"}
  }
  profile:="(unspecified)"
  if fields.Profile!=""{
   if !identifier(fields.Profile){return nil,errors.New("invalid MCP profile in client record")}
   profile=fields.Profile
  }
  installed:="unknown"
  if fields.Installed!=nil {installed="not installed";if *fields.Installed {installed="installed"}}
  osSupport:="unknown"
  if fields.OSSupported!=nil {osSupport="unsupported";if *fields.OSSupported {osSupport="supported"}}
  hasError:=len(fields.ConfigError)>0 && string(fields.ConfigError)!="null"
  label:=Clean(fields.DisplayName,80)
  if label==""{label=id}
  result=append(result,MCPClientIntegration{ID:id,Label:label,Profile:profile,State:state,Installed:installed,OSSupport:osSupport,HasConfigError:hasError})
 }
 sort.Slice(result,func(i,j int)bool{return result[i].ID<result[j].ID})
 return result,nil
}
