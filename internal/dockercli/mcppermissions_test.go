package dockercli

import (
 "context"
 "strings"
 "testing"
)
func policyFixture(allowlist string)*profileFake{
 f:=fixtureProfileFake()
 f.responses["mcp profile list --format json"]=`[{"id":"development","name":"development"}]`
 f.responses["mcp profile server ls --filter profile=development --format json"]=`[{"profile":"development","name":"github-official"}]`
 f.responses["mcp profile show development --format json"]=`{"id":"development","secrets":{"S":"should-not-show"},"servers":[{"config":{"token":"should-not-show"},"snapshot":{"server":{"name":"github-official","tools":[{"name":"read","description":"Read files"},{"name":"write","description":"Write files"}]}},"tools":`+allowlist+`}]}`
 return f
}
func TestMCPPolicyAllNoneAndCustom(t *testing.T){
 for _,tc:=range []struct{v,mode string;count int}{
  {"null","all",2},{"[]","none",0},{`["read"]`,"allowlist",1},
 }{
  f:=policyFixture(tc.v)
  p,err:=(Client{Runner:f}).MCPReadPermissions(context.Background(),"development","github-official")
  if err!=nil||p.Mode!=tc.mode||len(p.Tools)!=2{t.Fatalf("%s: %+v %v",tc.v,p,err)}
  enabled:=0
  for _,tool:=range p.Tools{if tool.Status=="enabled"{enabled++};if strings.Contains(tool.Description,"should-not-show"){t.Fatal("secret leaked")}}
  if enabled!=tc.count{t.Fatalf("wrong enabled count %d",enabled)}
 }
}
func TestMCPPolicyUnknownAndSnapshotSafety(t *testing.T){
 f:=policyFixture("null")
 f.responses["mcp profile show development --format json"]=`{"id":"development","servers":[{"snapshot":{"server":{"name":"github-official","tools":[{"name":"read"}]}}}]}`
 p,err:=(Client{Runner:f}).MCPReadPermissions(context.Background(),"development","github-official")
 if err!=nil||p.Mode!="unknown"{t.Fatalf("missing allowlist must be unknown: %+v %v",p,err)}
 if err:=(Client{Runner:f}).MCPSetSpecificTool(context.Background(),"development","github-official","read",false);err==nil{t.Fatal("allowed unknown state")}
 for _,call:=range f.calls{if len(call)>2&&strings.Join(call[:3]," ")=="mcp profile tools"{t.Fatal("unsafe tool write")}}
 f=policyFixture(`["unknown"]`)
 if _,err:=(Client{Runner:f}).MCPReadPermissions(context.Background(),"development","github-official");err==nil{t.Fatal("accepted stale unknown tool")}
}
func TestMCPRemoveGuardAndExactCLI(t *testing.T){
 f:=policyFixture("null")
 if err:=(Client{Runner:f}).MCPRemoveProfileServer(context.Background(),"development","github-official");err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.calls[len(f.calls)-1]," ");got!="mcp profile server remove development --name github-official"{t.Fatal(got)}
 f=policyFixture("null")
 f.responses["mcp profile server ls --filter profile=development --format json"]="[]"
 if err:=(Client{Runner:f}).MCPRemoveProfileServer(context.Background(),"development","github-official");err==nil{t.Fatal("removed nonmember")}
 for _,call:=range f.calls{if strings.Contains(strings.Join(call," "),"server remove"){t.Fatal("unauthorized mutation")}}
}
func TestMCPIndividualToolRechecks(t *testing.T){
 for _,tc:=range []struct{state,tool string;enable,write bool}{
  {"null","read",true,false},
  {"null","read",false,true},
  {"[]","read",true,true},
  {"[]","read",false,false},
  {`["read"]`,"write",true,true},
 }{
  f:=policyFixture(tc.state)
  err:=(Client{Runner:f}).MCPSetSpecificTool(context.Background(),"development","github-official",tc.tool,tc.enable)
  wrote:=false
  for _,call:=range f.calls{if len(call)>=3&&strings.Join(call[:3]," ")=="mcp profile tools"{wrote=true}}
  if wrote!=tc.write{t.Fatalf("%s %s enable=%v wrong write=%v err=%v",tc.state,tc.tool,tc.enable,wrote,err)}
 }
}
