package dockercli

import (
 "context"
 "fmt"
 "strings"
 "testing"
)

type globalClientFake struct{
 profile,connected string
 installed,osSupported bool
 reads,writes int
 writeError bool
 wrongReadback bool
 missingClient bool
 calls []string
}
func (f *globalClientFake)Run(_ context.Context,args ...string)([]byte,error){
 key:=strings.Join(args," ");f.calls=append(f.calls,key)
 switch key{
 case "mcp profile list --format json":
  return []byte(`[{"id":"development","name":"Development"}]`),nil
 case "mcp client ls --global --json":
  f.reads++
  if f.missingClient{return []byte(`{"other":{"displayName":"Other","isInstalled":true,"isOsSupported":true,"dockerMCPCatalogConnected":false}}`),nil}
  config:=fmt.Sprintf(`{"codex":{"displayName":"Codex","isInstalled":%t,"isOsSupported":%t,"dockerMCPCatalogConnected":%s,"profile":"%s","Cfg":{"STDIOServers":[{"command":"very secret"}]}}}`,f.installed,f.osSupported,f.connected,f.profile)
  return []byte(config),nil
 case "mcp client connect --global --profile development --quiet codex":
  f.writes++
  if f.writeError{return nil,fmt.Errorf("private file path and token")}
  f.connected="true"
  if f.wrongReadback{f.profile="other"}else{f.profile="development"}
  return []byte("ok"),nil
 }
 return nil,fmt.Errorf("unexpected command %s",key)
}
func clientConnectFixture()*globalClientFake{return &globalClientFake{installed:true,osSupported:true,connected:"false"}}
func TestClientGlobalConnectExactTargetAndReadback(t *testing.T){
 f:=clientConnectFixture()
 result:=(Client{Runner:f}).MCPConnectGlobalClient(context.Background(),"codex","development")
 if result.Outcome!="verified"||f.writes!=1||len(result.Updated)!=1||result.Updated[0].Profile!="development"{t.Fatalf("not verified %+v",result)}
 if got:=f.calls[len(f.calls)-2];got!="mcp client connect --global --profile development --quiet codex"{
  t.Fatalf("wrong CLI: %q",got)
 }
 if strings.Contains(result.Reason,"secret"){t.Fatal("leak")}
}
func TestClientConnectBlockedBeforeMutation(t *testing.T){
 cases:=[]struct{name string; modify func(*globalClientFake);client,profile string}{
  {"not installed",func(f *globalClientFake){f.installed=false},"codex","development"},
  {"unsupported",func(f *globalClientFake){f.osSupported=false},"codex","development"},
  {"connected",func(f *globalClientFake){f.connected="true";f.profile="default"},"codex","development"},
  {"missing",func(f *globalClientFake){f.missingClient=true},"codex","development"},
  {"invalid ID",func(f *globalClientFake){},"--global","development"},
  {"invalid profile",func(f *globalClientFake){},"codex","-bad"},
  {"unknown profile",func(f *globalClientFake){},"codex","unknown"},
 }
 for _,tc:=range cases {
  f:=clientConnectFixture();tc.modify(f)
  res:=(Client{Runner:f}).MCPConnectGlobalClient(context.Background(),tc.client,tc.profile)
  if res.Outcome!="blocked"||f.writes!=0{t.Errorf("%s: unexpected %+v, calls %v",tc.name,res,f.calls)}
 }
}
func TestClientConnectAfterDispatchMustNotAssumeSuccess(t *testing.T){
 for _,tc:=range []struct{fail,wrong bool}{
  {true,false},{false,true},
 }{
  f:=clientConnectFixture();f.writeError=tc.fail;f.wrongReadback=tc.wrong
  res:=(Client{Runner:f}).MCPConnectGlobalClient(context.Background(),"codex","development")
  if res.Outcome!="unverified"||f.writes!=1||strings.Contains(res.Reason,"token"){
   t.Fatalf("unverified condition mishandled %+v",res)
  }
 }
}
func TestClientConfigurationParserPreservesUnknownAndError(t *testing.T){
 raw:=[]byte(`{
 "codex":{"displayName":"Codex","dockerMCPCatalogConnected":false,"isInstalled":true,"isOsSupported":true,"error":null,"Cfg":{"secret":"private"}},
 "other":{"displayName":"Other","dockerMCPCatalogConnected":false,"error":{"error":"private path"}}
 }`)
 clients,err:=parseMCPClientIntegrations(raw)
 if err!=nil||len(clients)!=2{t.Fatalf("parse %+v %v",clients,err)}
 if clients[0].Installed!="installed"||clients[0].OSSupport!="supported"||clients[0].HasConfigError {t.Fatalf("unexpected codex %+v",clients[0])}
 if clients[1].Installed!="unknown"||!clients[1].HasConfigError{t.Fatalf("unknown/error not preserved %+v",clients[1])}
 if MCPPlausibleGlobalClient(clients[1])==nil{t.Fatal("unknown installation accepted")}
 if strings.Contains(fmt.Sprint(clients),"private"){t.Fatal("secret leaked to UI structure")}
}
