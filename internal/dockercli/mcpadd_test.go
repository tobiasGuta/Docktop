package dockercli

import (
	"context"
	"strings"
	"testing"
)

func catalogAddFixture() *profileFake {
	f := &profileFake{responses: map[string]string{
		"mcp profile list --format json":`[{"id":"development","name":"development"}]`,
		"mcp profile server ls --filter profile=development --format json":`[]`,
		"mcp catalog server inspect mcp/docker-mcp-catalog:latest github-official --format json":`{"title":"GitHub Official","type":"image"}`,
	}}
	return f
}
func isMCPAddWrite(call []string) bool {
	return len(call)>=4 && strings.Join(call[:4]," ")=="mcp profile server add"
}
func TestMCPAddCatalogServerUsesExactURI(t *testing.T) {
	f := catalogAddFixture()
	c := Client{Runner:f}
	if err := c.MCPAddCatalogServer(context.Background(), "development","github-official"); err != nil { t.Fatal(err) }
	if len(f.calls) != 4 { t.Fatalf("expected 3 reads + 1 write; got %v",f.calls) }
	got:=strings.Join(f.calls[len(f.calls)-1]," ")
	want:="mcp profile server add development --server catalog://mcp/docker-mcp-catalog/github-official"
	if got!=want {t.Fatalf("unexpected command: %s",got)}
	for _,call:=range f.calls {
		joined:=strings.Join(call," ")
		if strings.Contains(joined,"gateway") || strings.Contains(joined,"tools --enable-all") || strings.Contains(joined,"feature enable") {
			t.Fatalf("unexpected side effect: %s",joined)
		}
	}
}
func TestMCPAddRejectsInvalidTargetsWithoutCLI(t *testing.T) {
	f:=catalogAddFixture()
	c:=Client{Runner:f}
	for _,bad:=range [][2]string{
		{"-bad","github-official"},
		{"development","../github"},
		{"development","--server"},
		{"development","github;rm"},
		{"development","catalog://mcp/docker-mcp-catalog/github"},
	} {
		if err:=c.MCPAddCatalogServer(context.Background(),bad[0],bad[1]);err==nil {
			t.Fatalf("unsafe identifier accepted: %+v",bad)
		}
	}
	if len(f.calls)!=0 {t.Fatalf("invalid inputs invoked Docker: %+v",f.calls)}
}
func TestMCPAddMustRecheckAndFailClosed(t *testing.T) {
	t.Run("missing profile",func(t *testing.T){
		f:=catalogAddFixture()
		f.responses["mcp profile list --format json"]=`[]`
		if err:=(Client{Runner:f}).MCPAddCatalogServer(context.Background(),"development","github-official");err==nil{t.Fatal("missing profile accepted")}
		checkNoWrite(t,f)
	})
	t.Run("profile lookup fails",func(t *testing.T){
		f:=catalogAddFixture()
		f.fail="mcp profile list --format json"
		if err:=(Client{Runner:f}).MCPAddCatalogServer(context.Background(),"development","github-official");err==nil{t.Fatal("lookup failure accepted")}
		checkNoWrite(t,f)
	})
	t.Run("already assigned",func(t *testing.T){
		f:=catalogAddFixture()
		f.responses["mcp profile server ls --filter profile=development --format json"]=`[{"profile":"development","name":"github-official"}]`
		if err:=(Client{Runner:f}).MCPAddCatalogServer(context.Background(),"development","github-official");err==nil{t.Fatal("duplicate accepted")}
		checkNoWrite(t,f)
	})
	t.Run("unknown members format",func(t *testing.T){
		f:=catalogAddFixture()
		f.responses["mcp profile server ls --filter profile=development --format json"]=`{"unexpected":"data"}`
		if err:=(Client{Runner:f}).MCPAddCatalogServer(context.Background(),"development","github-official");err==nil{t.Fatal("unknown format accepted")}
		checkNoWrite(t,f)
	})
	t.Run("unknown catalog server",func(t *testing.T){
		f:=catalogAddFixture()
		f.fail="mcp catalog server inspect mcp/docker-mcp-catalog:latest github-official --format json"
		if err:=(Client{Runner:f}).MCPAddCatalogServer(context.Background(),"development","github-official");err==nil{t.Fatal("catalog lookup failure accepted")}
		checkNoWrite(t,f)
	})
	t.Run("invalid catalog JSON",func(t *testing.T){
		f:=catalogAddFixture()
		f.responses["mcp catalog server inspect mcp/docker-mcp-catalog:latest github-official --format json"]=`invalid`
		if err:=(Client{Runner:f}).MCPAddCatalogServer(context.Background(),"development","github-official");err==nil{t.Fatal("malformed metadata accepted")}
		checkNoWrite(t,f)
	})
}
func checkNoWrite(t *testing.T, f *profileFake) {
	t.Helper()
	for _,call:=range f.calls {if isMCPAddWrite(call) {t.Fatalf("unexpected mutation: %v",call)}}
}
