package dockercli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type mcpInspectionRunner struct {
	calls [][]string
	response string
	err error
}
func (f *mcpInspectionRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	if f.err != nil { return nil, f.err }
	return []byte(f.response), nil
}

func TestMCPInspectIsReadOnlyAndValidated(t *testing.T) {
	f := &mcpInspectionRunner{response: `{"title":"Tools","tools":[{"name":"list_repos","description":"Read repos"}],"secrets":{"API_TOKEN":"do-not-display"},"config":{"password":"do-not-display"}}`}
	c := Client{Runner: f}
	for _, invalid := range []string{"", "--help", "github; echo bad", "../etc/passwd", "a b", strings.Repeat("x",129)} {
		if _, err := c.MCPServerDetails(context.Background(), invalid); err == nil {
			t.Fatalf("accepted invalid MCP ID %q", invalid)
		}
	}
	if len(f.calls) != 0 { t.Fatal("rejected inputs executed commands") }
	text, err := c.MCPServerDetails(context.Background(), "github-official")
	if err != nil { t.Fatal(err) }
	want := []string{"mcp","catalog","server","inspect","mcp/docker-mcp-catalog:latest","github-official","--format","json"}
	if strings.Join(f.calls[0], "|") != strings.Join(want, "|") { t.Fatalf("unexpected command: %v", f.calls[0]) }
	for _, s := range []string{"list_repos","Read repos","READ ONLY"} {
		if !strings.Contains(text,s) { t.Errorf("missing %q",s) }
	}
	for _, secret := range []string{"do-not-display","API_TOKEN","password"} {
		if strings.Contains(text,secret) { t.Errorf("displayed restricted catalog field: %s",secret) }
	}
}

func TestMCPInspectMapToolsAndNoRawFallback(t *testing.T) {
	f := &mcpInspectionRunner{response:`{"server":{"title":"Nested","tools":{"beta":{"description":"Two"},"alpha":{}},"credentials":{"auth":"secret"}}}`}
	text, err := (Client{Runner:f}).MCPServerDetails(context.Background(), "playwright")
	if err != nil || !strings.Contains(text,"alpha") || !strings.Contains(text,"beta") { t.Fatalf("%q: %v",text,err) }
	if strings.Index(text,"alpha") > strings.Index(text,"beta") { t.Fatal("tools not sorted") }
	f.response = "not-json with secret"
	text, err = (Client{Runner:f}).MCPServerDetails(context.Background(), "playwright")
	if err == nil || text != "" || strings.Contains(err.Error(),"secret") { t.Fatalf("unsafe raw fallback %q %v",text,err) }
	f.err = errors.New("CLI unavailable")
	if _, err := (Client{Runner:f}).MCPServerDetails(context.Background(), "playwright"); err == nil { t.Fatal("missing CLI error") }
}

func TestMCPConfiguredListIsReadOnly(t *testing.T) {
	f := &mcpInspectionRunner{response:"Configured: github-official\n"}
	text, err := (Client{Runner:f}).MCPConfiguredServers(context.Background())
	if err != nil || !strings.Contains(text,"github-official") { t.Fatal(text,err) }
	if strings.Join(f.calls[0]," ") != "mcp profile server ls" { t.Fatalf("unexpected CLI call: %v",f.calls[0]) }
}
