package dockercli

import (
 "context"
 "errors"
 "fmt"
 "sort"
)

// MCPToolChange is a proposed explicit permission change for one named tool.
// Each operation is applied separately by Docker MCP, never as an atomic batch.
type MCPToolChange struct { Tool string; Enable bool }
type MCPToolPlanResult struct {
 Verified []string
 Attempted string
 After MCPPermissionView
 Err error
 FailureKind string // blocked, partial (earlier steps verified), uncertain (attempted tool unresolved)
 WritesAttempted int
}

// MCPApplyToolPlan revalidates both profile membership and every observed
// tool permission before writing, then verifies after EACH step. A failure
// halts further writes; completed steps are not automatically rolled back.
// Callers MUST show this partial-application limitation before approval.
func (c Client) MCPApplyToolPlan(ctx context.Context, profile, server string, expected MCPPermissionView, changes []MCPToolChange) MCPToolPlanResult {
 result:=MCPToolPlanResult{}
 fail:=func(err error) MCPToolPlanResult {
  result.Err=err
  if result.Attempted!="" {result.FailureKind="uncertain"
  } else if len(result.Verified)>0 {result.FailureKind="partial"
  } else {result.FailureKind="blocked"}
  return result
 }
 if !identifier(profile)||!identifier(server)||expected.Profile!=profile||expected.Server!=server {
  return fail(errors.New("invalid tool plan target"))
 }
 if len(changes)==0 || len(changes)>12 {return fail(errors.New("plan must contain 1-12 tool changes"))}
 if expected.Mode=="unknown" || len(expected.Tools)==0 {return fail(errors.New("plan has no reliable baseline"))}
 if err:=c.requireMCPMember(ctx,profile,server);err!=nil{return fail(err)}
 current,err:=c.MCPReadPermissions(ctx,profile,server)
 if err!=nil{return fail(fmt.Errorf("plan preflight read failed: %w",err))}
 if err:=sameMCPPolicy(expected,current);err!=nil{return fail(fmt.Errorf("plan baseline changed: %w",err))}
 seen:=map[string]bool{}
 for _,change:=range changes {
  if !identifier(change.Tool)||seen[change.Tool]{return fail(errors.New("invalid or duplicate tool in staged plan"))}
  seen[change.Tool]=true
  state,exists:=toolState(current,change.Tool)
  if !exists{return fail(fmt.Errorf("staged tool absent: %s",change.Tool))}
  if (state=="enabled")==change.Enable{return fail(fmt.Errorf("staged tool already has requested state: %s",change.Tool))}
 }
 sorted:=append([]MCPToolChange(nil),changes...)
 sort.Slice(sorted,func(i,j int)bool{return sorted[i].Tool<sorted[j].Tool})
 for _,change:=range sorted {
  // A previous step may have changed the policy, or another actor may have
  // modified it. Compare the COMPLETE current policy immediately before
  // every write, not only the selected tool. This is best-effort: Docker MCP
  // does not expose compare-and-swap, so cross-process TOCTOU remains.
  latest,err:=c.MCPReadPermissions(ctx,profile,server)
  if err!=nil{return fail(fmt.Errorf("cannot read current policy before %s: %w",change.Tool,err))}
  if err:=sameMCPPolicy(current,latest);err!=nil {
   return fail(fmt.Errorf("policy changed before %s: %w",change.Tool,err))
  }
  if err:=c.requireMCPMember(ctx,profile,server);err!=nil {
   return fail(fmt.Errorf("membership changed before %s: %w",change.Tool,err))
  }
  result.Attempted=change.Tool
  // Once dispatch begins, an error (including cancellation) cannot prove
  // Docker did not apply it. Preserve that uncertainty in the result.
  result.WritesAttempted++
  if err:=c.MCPSetSpecificTool(ctx,profile,server,change.Tool,change.Enable);err!=nil {
   return fail(fmt.Errorf("tool write may not have completed for %s: %w",change.Tool,err))
  }
  after,err:=c.MCPReadPermissions(ctx,profile,server)
  if err!=nil{return fail(fmt.Errorf("tool command succeeded for %s but readback failed: %w",change.Tool,err))}
  if err:=changedMCPPolicy(current,after,change.Tool,change.Enable);err!=nil {
   return fail(fmt.Errorf("tool command succeeded for %s but readback did not match: %w",change.Tool,err))
  }
  result.Verified=append(result.Verified,change.Tool)
  result.After=after
  current=after
  result.Attempted=""
 }
 return result
}
func toolState(p MCPPermissionView,name string)(string,bool){
 for _,t:=range p.Tools {if t.Name==name{return t.Status,true}}
 return "",false
}
func sameMCPPolicy(a,b MCPPermissionView)error{
 if a.Profile!=b.Profile||a.Server!=b.Server||a.Mode!=b.Mode||len(a.Tools)!=len(b.Tools) {
  return errors.New("profile, server, mode or tool count changed")
 }
 for _,t:=range a.Tools {
  other,ok:=toolState(b,t.Name)
  if !ok||other!=t.Status{return fmt.Errorf("tool changed or disappeared: %s",t.Name)}
 }
 return nil
}
func changedMCPPolicy(before,after MCPPermissionView,name string,enable bool)error{
 if after.Mode=="unknown"||len(after.Tools)!=len(before.Tools)||after.Profile!=before.Profile||after.Server!=before.Server {
  return errors.New("readback has unknown or incompatible tool snapshot")
 }
 for _,prior:=range before.Tools {
  observed,ok:=toolState(after,prior.Name)
  if !ok{return fmt.Errorf("tool disappeared: %s",prior.Name)}
  wanted:=prior.Status
  if prior.Name==name{
   wanted="disabled";if enable{wanted="enabled"}
  }
  if observed!=wanted{return fmt.Errorf("unexpected permission for %s: got %s, expected %s",prior.Name,observed,wanted)}
 }
 return nil
}
