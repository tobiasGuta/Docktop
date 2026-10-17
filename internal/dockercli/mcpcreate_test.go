package dockercli

import (
	"context"
	"strings"
	"testing"
)

func TestCreateMCPProfileExplicitScopedCommand(t *testing.T) {
	f := fixtureProfileFake()
	c := Client{Runner: f}
	for _, name := range []string{"", "--bad", "a name", "a;b", "../bad", strings.Repeat("x", 49)} {
		if err := c.MCPCreateProfile(context.Background(), name); err == nil {
			t.Fatalf("accepted unsafe name %q", name)
		}
	}
	if len(f.calls) != 0 { t.Fatal("bad name caused a Docker command") }
	if err := c.MCPCreateProfile(context.Background(), "web-dev"); err == nil {
		t.Fatal("duplicate existing profile accepted")
	}
	if len(f.calls) != 1 { t.Fatalf("duplicate caused write: %v",f.calls) }
	if err := c.MCPCreateProfile(context.Background(), "docktop-lab"); err != nil { t.Fatal(err) }
	want := "mcp profile create --name docktop-lab"
	if got := strings.Join(f.calls[len(f.calls)-1], " "); got != want {
		t.Fatalf("unexpected CLI command: %s", got)
	}
	for _,call := range f.calls {
		joined := strings.Join(call," ")
		if strings.Contains(joined,"feature enable") || strings.Contains(joined,"server add") || strings.Contains(joined,"tools ") || strings.Contains(joined,"gateway") {
			t.Fatalf("unexpected side effect: %v",call)
		}
	}
}

func TestCreateMCPProfileFailsClosedWithoutDiscovery(t *testing.T) {
	f := fixtureProfileFake()
	f.fail = "mcp profile list --format json"
	if err := (Client{Runner:f}).MCPCreateProfile(context.Background(),"docktop-lab"); err == nil {
		t.Fatal("creation should require successful profile discovery")
	}
	if len(f.calls)!=1 { t.Fatalf("unexpected write: %+v", f.calls) }
	f.fail=""
	f.responses["mcp profile list --format json"] = "unsupported output"
	if err := (Client{Runner:f}).MCPCreateProfile(context.Background(),"docktop-lab"); err==nil {
		t.Fatal("creation should reject unknown profile list schema")
	}
	for _, args := range f.calls {
		if strings.Contains(strings.Join(args," "),"profile create") { t.Fatal("unexpected profile creation") }
	}
}
