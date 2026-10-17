package dockercli

import (
 "context"
 "errors"
 "strings"
 "testing"
)
type concurrentPolicyRunner struct {
 mode string
 calls []string
 writes int
 reads int
 injectBeforeSecond bool
}
func (r *concurrentPolicyRunner) Run(_ context.Context,args ...string)([]byte,error){
 key:=strings.Join(args," ")
 r.calls=append(r.calls,key)
 switch key {
 case "mcp profile list --format json":
  return []byte(`[{"id":"development","name":"development"}]`),nil
 case "mcp profile server ls --filter profile=development --format json":
  return []byte(`[{"profile":"development","name":"github-official"}]`),nil
 case "mcp profile show development --format json":
  r.reads++
  if r.injectBeforeSecond && r.writes==1 && r.reads>=8{
   // Simulated separate MCP client changing a DIFFERENT tool between steps.
   r.mode=`["second"]`
  }
  return []byte(`{"id":"development","servers":[{"snapshot":{"server":{"name":"github-official","tools":[{"name":"first"},{"name":"second"},{"name":"third"}]}},"tools":`+r.mode+`}]}`),nil
 case "mcp profile tools development --disable github-official.first":
  r.writes++
  r.mode=`["second","third"]`
  return []byte("ok"),nil
 case "mcp profile tools development --disable github-official.second":
  r.writes++
  r.mode=`["third"]`
  return []byte("ok"),nil
 }
 return nil,errors.New("unexpected command "+key)
}
func TestMCPPlanReportsBlockedPreflightWithoutWrite(t *testing.T){
 r:=&concurrentPolicyRunner{mode:"null"}
 c:=Client{Runner:r}
 expected,err:=c.MCPReadPermissions(context.Background(),"development","github-official")
 if err!=nil{t.Fatal(err)}
 r.mode=`["first","second","third"]` // equivalent permissions but mode changed concurrently
 result:=c.MCPApplyToolPlan(context.Background(),"development","github-official",expected,[]MCPToolChange{{Tool:"first",Enable:false}})
 if result.FailureKind!="blocked"||result.WritesAttempted!=0||r.writes!=0||result.Err==nil{
  t.Fatalf("unexpected preflight accounting: %+v",result)
 }
}
func TestMCPPlanConcurrentChangeBlocksFollowingWrite(t *testing.T){
 r:=&concurrentPolicyRunner{mode:"null",injectBeforeSecond:true}
 c:=Client{Runner:r}
 expected,err:=c.MCPReadPermissions(context.Background(),"development","github-official")
 if err!=nil{t.Fatal(err)}
 result:=c.MCPApplyToolPlan(context.Background(),"development","github-official",expected,[]MCPToolChange{{Tool:"first",Enable:false},{Tool:"second",Enable:false}})
 if result.Err==nil||result.FailureKind!="uncertain"||result.WritesAttempted!=1||len(result.Verified)!=1||r.writes!=1{
  t.Fatalf("unexpected concurrent-change accounting: %+v, writes=%d",result,r.writes)
 }
}
func TestParseMCPClientIntegrationsNoConfigDisclosure(t *testing.T){
 raw:=`{
  "codex":{"displayName":"Codex","profile":"development","dockerMCPCatalogConnected":true,"Cfg":{"STDIOServers":[{"command":"private-command --token super-secret"}]}},
  "other":{"displayName":"Other","dockerMCPCatalogConnected":false,"error":{"message":"private error"}}
 }`
 got,err:=parseMCPClientIntegrations([]byte(raw))
 if err!=nil||len(got)!=2{t.Fatalf("clients %+v %v",got,err)}
 if got[0].ID!="codex"||got[0].Profile!="development"||got[0].State!="configured"{
  t.Fatalf("unexpected client metadata %+v",got[0])
 }
 if strings.Contains(got[0].Label+got[1].Label,"super-secret")||strings.Contains(got[1].Label,"private"){
  t.Fatal("sensitive config leaked")
 }
 if _,err:=parseMCPClientIntegrations([]byte(`[]`));err==nil{t.Fatal("non-map format accepted")}
}
