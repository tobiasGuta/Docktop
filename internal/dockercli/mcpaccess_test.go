package dockercli

import (
	"context"
	"errors"
	"strings"
	"testing"
)
type profileFake struct{
	calls [][]string
	responses map[string]string
	fail string
}
func (f *profileFake) Run(_ context.Context,args ...string)([]byte,error){
	f.calls=append(f.calls,append([]string(nil),args...))
	key:=strings.Join(args," ")
	if key==f.fail {return nil,errors.New("unavailable")}
	return []byte(f.responses[key]),nil
}
func fixtureProfileFake()*profileFake{
	return &profileFake{responses:map[string]string{
		"mcp profile list --format json":`[{"id":"web-dev","name":"Web Development"},{"id":"default","name":"Default"}]`,
		"mcp profile server ls --filter profile=web-dev --format json":`[{"profile":"web-dev","name":"github-official"},{"profile":"web-dev","name":"playwright"}]`,
	}}
}
func TestProfileMetadataAndToolAllowlist(t *testing.T){
	f:=fixtureProfileFake()
	c:=Client{Runner:f}
	profiles,err:=c.MCPProfilesSafe(context.Background())
	if err!=nil||len(profiles)!=2||profiles[1].ID!="web-dev" {t.Fatalf("profiles: %+v %v",profiles,err)}
	servers,err:=c.MCPProfileMembers(context.Background(),"web-dev")
	if err!=nil||len(servers)!=2 {t.Fatalf("servers: %+v %v",servers,err)}
	if err:=c.MCPSetToolAccess(context.Background(),"web-dev","github-official",false);err!=nil{t.Fatal(err)}
	want:="mcp profile tools web-dev --disable-all github-official"
	if got:=strings.Join(f.calls[len(f.calls)-1]," ");got!=want{t.Fatalf("mutation got %q",got)}
	if err:=c.MCPSetToolAccess(context.Background(),"web-dev","github-official",true);err!=nil{t.Fatal(err)}
	if got:=strings.Join(f.calls[len(f.calls)-1]," ");got!="mcp profile tools web-dev --enable-all github-official"{t.Fatalf("unexpected enable: %q",got)}
}
func TestToolChangeFailsClosed(t *testing.T){
	f:=fixtureProfileFake()
	c:=Client{Runner:f}
	for _,bad:=range [][2]string{{"-p","github-official"},{"web-dev","../secret"},{"web-dev","github;rm"}}{
		if err:=c.MCPSetToolAccess(context.Background(),bad[0],bad[1],true);err==nil{t.Fatalf("accepted %v",bad)}
	}
	if len(f.calls)!=0 {t.Fatalf("ran invalid arguments: %+v",f.calls)}
	if err:=c.MCPSetToolAccess(context.Background(),"web-dev","not-assigned",false);err==nil{t.Fatal("changed non-member")}
	f.responses["mcp profile server ls --filter profile=web-dev --format json"]=`{"servers":[{"profile":"other","name":"github-official"}]}`
	if err:=c.MCPSetToolAccess(context.Background(),"web-dev","github-official",true);err==nil{t.Fatal("accepted cross-profile server")}
	f.responses["mcp profile server ls --filter profile=web-dev --format json"]=`{"unknown":[{"name":"github-official"}]}`
	if err:=c.MCPSetToolAccess(context.Background(),"web-dev","github-official",true);err==nil{t.Fatal("accepted unrecognized JSON")}
	for _,call:=range f.calls{
		if strings.Contains(strings.Join(call," "),"profile tools"){t.Fatalf("unsafe mutation: %+v",call)}
	}
}
func TestProfileAllowlistJSONShapes(t *testing.T){
	profiles,err:=parseProfiles([]byte(`{"profiles":[{"id":"web-dev","name":"Web"}]}`))
	if err!=nil||len(profiles)!=1 {t.Fatalf("envelope: %v %+v",err,profiles)}
	servers,err:=parseProfileServers([]byte(`{"profiles":[{"id":"web-dev","servers":[{"name":"playwright"}]},{"id":"other","servers":[{"name":"github"}]}]}`),"web-dev")
	if err!=nil||len(servers)!=1||servers[0].Name!="playwright"{t.Fatalf("grouped: %v %+v",err,servers)}
	for _,payload:=range []string{`{}`,`{"profiles":[{"id":"--invalid"}]}`}{
		if _,err:=parseProfiles([]byte(payload));err==nil{t.Fatalf("accepted %s",payload)}
	}
}
func TestNoToolChangeIfProfileListUnavailable(t *testing.T){
	f:=fixtureProfileFake()
	f.fail="mcp profile list --format json"
	if err:=(Client{Runner:f}).MCPSetToolAccess(context.Background(),"web-dev","github-official",true);err==nil {t.Fatal("failed to reject lost profile lookup")}
	for _,call:=range f.calls {
		if strings.Contains(strings.Join(call," "),"profile tools"){t.Fatal("unexpected write")}
	}
}


func TestDockerMCPRealSearchResultShapeUsesSnapshotServerName(t *testing.T) {
	// docker mcp profile server ls --filter profile=development --format json
	// returns profile-level SearchResult objects, not individual server rows.
	payload := `[{"id":"development","name":"development","servers":[{"type":"image","image":"ghcr.io/github/github-mcp-server@sha256:abc","snapshot":{"server":{"name":"github-official","tools":[{"name":"get_file_contents"}]}},"tools":null,"config":{"secret":"must-not-display"}}]}]`
	got, err := parseProfileServers([]byte(payload), "development")
	if err != nil || len(got)!=1 || got[0].Name!="github-official" {
		t.Fatalf("expected github-official, not profile name: %+v %v",got,err)
	}
	f:=fixtureProfileFake()
	f.responses["mcp profile list --format json"]=`[{"id":"development","name":"development"}]`
	f.responses["mcp profile server ls --filter profile=development --format json"]=payload
	client:=Client{Runner:f}
	members,err:=client.MCPProfileMembers(context.Background(),"development")
	if err!=nil || len(members)!=1 || members[0].Name!="github-official" {t.Fatalf("actual query shape: %+v %v",members,err)}
	// A profile label must never be accepted as a target for permission changes.
	if err:=client.MCPSetToolAccess(context.Background(),"development","development",true);err==nil {
		t.Fatal("profile display name incorrectly accepted as server")
	}
	for _,call:=range f.calls{
		if strings.Contains(strings.Join(call," "), "profile tools") {t.Fatalf("wrong-server write: %v",call)}
	}
}
func TestDockerMCPGroupedProfilesAndMalformedIdentity(t *testing.T) {
	payload:=`[{"id":"other","name":"other","servers":[{"snapshot":{"server":{"name":"not-ours"}}}]},{"id":"development","name":"Display title","servers":[{"snapshot":{"server":{"name":"playwright"}}},{"snapshot":{"server":{"name":"github-official"}}}]}]`
	got,err:=parseProfileServers([]byte(payload),"development")
	if err!=nil||len(got)!=2||got[0].Name!="github-official"||got[1].Name!="playwright"{t.Fatalf("group parse: %+v %v",got,err)}
	empty,err:=parseProfileServers([]byte(`[{"id":"development","name":"Development","servers":[]}]`),"development")
	if err!=nil||len(empty)!=0{t.Fatalf("empty profile: %+v %v",empty,err)}
	empty,err=parseProfileServers([]byte(`[]`),"development")
	if err!=nil||len(empty)!=0{t.Fatalf("empty list: %+v %v",empty,err)}
}
func TestDockerMCPGroupedServerIdentityFailsClosed(t *testing.T) {
	for _,payload:=range []string{
		`[{"id":"development","name":"development","servers":[{"type":"image","name":"development"}]}]`,
		`[{"id":"development","name":"development","servers":[{"snapshot":null,"name":"development"}]}]`,
		`[{"id":"development","name":"development","servers":[{"name":"development","snapshot":{"server":{"name":"github-official"}}}]}]`,
		`[{"id":"development","name":"development","servers":[{"snapshot":{"server":{"name":"github-official"}}},{"snapshot":{"server":{"name":"github-official"}}}]}]`,
		`[{"id":"other","name":"other","servers":[{"snapshot":{"server":{"name":"github-official"}}}]}]`,
	} {
		if got,err:=parseProfileServers([]byte(payload),"development");err==nil {
			t.Fatalf("accepted ambiguous grouped membership: %+v | %s",got,payload)
		}
	}
}
