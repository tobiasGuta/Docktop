package mcpcatalog

import "testing"

func TestParseAndFilter(t *testing.T) {
	raw := `Catalog: mcp/docker-mcp-catalog:latest
Title: Docker MCP Catalog
Servers (2):

  github-official
    Title: GitHub Official
    Description: Official GitHub integration
    Type: image
    Image: ghcr.io/example
    Tools: 40

  ais-fleet
    Title: AIS Fleet
    Type: remote
    Endpoint: https://example.com/sse
`
	servers := Parse(raw)
	if len(servers) != 2 || servers[0].Tools != 40 || servers[1].Endpoint != "https://example.com/sse" {
		t.Fatalf("unexpected servers: %+v", servers)
	}
	got := Filter(servers, "official")
	if len(got) != 1 || got[0].Name != "github-official" {
		t.Fatalf("filter: %+v", got)
	}
	if len(Filter(servers, "nope")) != 0 {
		t.Fatal("unexpected matches")
	}
}
func TestParseRejectsUnstructuredText(t *testing.T) {
	if len(Parse("Docker unavailable\nCheck installation\n")) != 0 {
		t.Fatal("expected no servers")
	}
}
