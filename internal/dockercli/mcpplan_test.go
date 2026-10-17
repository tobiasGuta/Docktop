package dockercli

import (
 "context"
 "fmt"
 "strings"
 "testing"
)

type planRunner struct {
 allowlist string
 calls []string
 mutate int
 failOn int
 drift bool
}
func (r *planRunner) Run(_ context.Context,args ...string)([]byte,error){
 s:=strings.Join(args," ");r.calls=append(r.calls,s)
 switch s{
 case "mcp profile list --format json":return []byte(`[{"id":"development","name":"development"}]`),nil
 case "mcp profile server ls --filter profile=development --format json":return []byte(`[{"profile":"development","name":"github-official"}]`),nil
 case "mcp profile show development --format json":
  return []byte(fmt.Sprintf(`{"id":"development","servers":[{"snapshot":{"server":{"name":"github-official","tools":[{"name":"first"},{"name":"second"},{"name":"third"}]}},"tools":%s}]}`,r.allowlist)),nil
 case "mcp profile tools development --disable github-official.first":
  r.mutate++;if r.mutate==r.failOn{return nil,fmt.Errorf("simulated write failure")}
  r.allowlist=`["second","third"]`;return []byte("ok"),nil
 case "mcp profile tools development --disable github-official.second":
  r.mutate++;if r.mutate==r.failOn{return nil,fmt.Errorf("simulated write failure")}
  if r.drift {r.allowlist=`["first"]`} else {r.allowlist=`["third"]`}
  return []byte("ok"),nil
 }
 return nil,fmt.Errorf("unexpected command %s",s)
}
func TestMCPPlanRequiresExplicitTargetsAndPreflight(t *testing.T){
 runner:=&planRunner{allowlist:"null"}
 c:=Client{Runner:runner}
 prior,err:=c.MCPReadPermissions(context.Background(),"development","github-official")
 if err!=nil{t.Fatal(err)}
 invalid:=[]MCPToolChange{{Tool:"first",Enable:true}}
 if result:=c.MCPApplyToolPlan(context.Background(),"development","github-official",prior,invalid);result.Err==nil{t.Fatal("already-enabled tool accepted")}
 changed:=prior;changed.Tools=append([]MCPToolPermission(nil),prior.Tools...);changed.Tools[0].Status="disabled"
 if result:=c.MCPApplyToolPlan(context.Background(),"development","github-official",changed,[]MCPToolChange{{Tool:"second",Enable:false}});result.Err==nil {t.Fatal("stale baseline accepted")}
 if runner.mutate!=0 {t.Fatal("preflight wrote to Docker")}
 if result:=c.MCPApplyToolPlan(context.Background(),"development","github-official",prior,[]MCPToolChange{{Tool:"first",Enable:false},{Tool:"first",Enable:false}});result.Err==nil{t.Fatal("duplicate target accepted")}
 if runner.mutate!=0 {t.Fatal("duplicate request wrote to Docker")}
}
func TestMCPPlanPartialFailureStopsFollowingWrites(t *testing.T){
 runner:=&planRunner{allowlist:"null",failOn:2}
 c:=Client{Runner:runner}
 prior,err:=c.MCPReadPermissions(context.Background(),"development","github-official")
 if err!=nil{t.Fatal(err)}
 result:=c.MCPApplyToolPlan(context.Background(),"development","github-official",prior,[]MCPToolChange{{Tool:"first",Enable:false},{Tool:"second",Enable:false}})
 if result.Err==nil||len(result.Verified)!=1||result.Verified[0]!="first"||result.Attempted!="second" {t.Fatalf("partial not reported: %+v",result)}
 if runner.mutate!=2 {t.Fatal("expected two attempts")}
}
func TestMCPPlanDetectsUnrelatedDrift(t *testing.T){
 runner:=&planRunner{allowlist:"null",drift:true}
 c:=Client{Runner:runner}
 prior,err:=c.MCPReadPermissions(context.Background(),"development","github-official")
 if err!=nil{t.Fatal(err)}
 result:=c.MCPApplyToolPlan(context.Background(),"development","github-official",prior,[]MCPToolChange{{Tool:"second",Enable:false}})
 if result.Err==nil||len(result.Verified)!=0||result.Attempted!="second"{t.Fatalf("drift must be visible: %+v",result)}
}
func TestMCPPlanOneVerifiedChange(t *testing.T){
 runner:=&planRunner{allowlist:"null"}
 c:=Client{Runner:runner}
 prior,err:=c.MCPReadPermissions(context.Background(),"development","github-official")
 if err!=nil{t.Fatal(err)}
 result:=c.MCPApplyToolPlan(context.Background(),"development","github-official",prior,[]MCPToolChange{{Tool:"first",Enable:false}})
 if result.Err!=nil||len(result.Verified)!=1||result.Attempted!=""{t.Fatalf("verification failure: %+v",result)}
 enabled,ok:=toolState(result.After,"first")
 if !ok||enabled!="disabled"{t.Fatal("readback wrong")}
}
