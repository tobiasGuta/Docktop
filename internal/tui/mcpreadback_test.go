package tui

import (
 "context"
 "fmt"
 "strings"
 "testing"

 "github.com/tobiasGuta/Docktop/internal/dockercli"
)

type policyReadbackFake struct {
 allowlist string
 initialMembers bool
 refuseReadback bool
 sideEffect bool
 calls []string
}
func (r *policyReadbackFake) Run(_ context.Context, args ...string) ([]byte,error) {
 key:=strings.Join(args," ")
 r.calls=append(r.calls,key)
 switch key {
 case "mcp profile list --format json":
  return []byte(`[{"id":"development","name":"development"}]`),nil
 case "mcp profile server ls --filter profile=development --format json":
  if !r.initialMembers {return []byte("[]"),nil}
  return []byte(`[{"profile":"development","name":"github-official"}]`),nil
 case "mcp profile show development --format json":
  if r.refuseReadback {return []byte("unsupported response"),nil}
  payload:=fmt.Sprintf(`{"id":"development","servers":[{"snapshot":{"server":{"name":"github-official","tools":[{"name":"read"},{"name":"write"}]}},"tools":%s}]}`,r.allowlist)
  return []byte(payload),nil
 case "mcp profile tools development --enable github-official.write":
  if !r.refuseReadback {
   if r.sideEffect {r.allowlist=`["write"]`} else {r.allowlist=`["read","write"]`}
  }
  return []byte("ok"),nil
 case "mcp profile server remove development --name github-official":
  r.initialMembers=false
  return []byte("removed"),nil
 }
 return nil,fmt.Errorf("unexpected CLI command")
}

func TestMCPWriteVerifiesReadbackAndOtherTools(t *testing.T){
 for _,tc:=range []struct{sideEffect,refuse bool;verified bool}{
  {false,false,true},
  {true,false,false},
  {false,true,false},
 }{
  fake:=&policyReadbackFake{allowlist:`["read"]`,initialMembers:true,sideEffect:tc.sideEffect,refuseReadback:tc.refuse}
  prior:=dockercli.MCPPermissionView{Profile:"development",Server:"github-official",Mode:"allowlist",
   Tools:[]dockercli.MCPToolPermission{{Name:"read",Status:"enabled"},{Name:"write",Status:"disabled"}}}
  msg:=applyMCPPermission(dockercli.Client{Runner:fake},"development","github-official","write","enable",prior)().(permissionsChangedMsg)
  if msg.err!=nil{t.Fatalf("mutation rejected: %v",msg.err)}
  if (msg.verifyErr==nil)!=tc.verified{t.Fatalf("verification mismatch %+v",msg)}
  foundWrite,foundRead:=false,false
  for _,call:=range fake.calls {
   if call=="mcp profile tools development --enable github-official.write"{foundWrite=true}
   if call=="mcp profile show development --format json"{foundRead=true}
  }
  if !foundWrite || !foundRead {t.Fatalf("missing write/readback: %v",fake.calls)}
 }
}

func TestMCPRemoveVerifiesProfileMembership(t *testing.T){
 fake:=&policyReadbackFake{allowlist:"null",initialMembers:true}
 msg:=applyMCPPermission(dockercli.Client{Runner:fake},"development","github-official","","remove",dockercli.MCPPermissionView{})().(permissionsChangedMsg)
 if msg.err!=nil||msg.verifyErr!=nil {t.Fatalf("removal not verified: %+v",msg)}
 if fake.initialMembers{t.Fatal("membership not updated")}
}

func TestMCPAnnotationClassificationFromSnapshot(t *testing.T) {
 // This test is performed separately inside dockercli for parsing hints.
 // Here we ensure unspecified metadata never implies read-only safety.
 m:=testPermissionModel()
 m.perm.view.Tools[3].CapabilityHint=""
 m.perm.group="unspecified"
 v:=m.perm.visibleTools()
 if len(v)!=1||v[0].Name!="custom_task" {t.Fatal("absent hint should remain unspecified")}
}
