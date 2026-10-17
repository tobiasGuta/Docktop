package dockercli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Catalog inspection is metadata-only. No profile mutation, gateway execution,
// live tool discovery, or credential/configuration retrieval happens here.
const mcpCatalogRef = "mcp/docker-mcp-catalog:latest"

var mcpServerID = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$")

// MCPServerDetails inspects only a named catalog entry. The response is decoded
// with a strict display allowlist: never echo arbitrary config/secrets fields.
func (c Client) MCPServerDetails(ctx context.Context, name string) (string, error) {
	if !mcpServerID.MatchString(name) {
		return "", errors.New("invalid MCP server ID")
	}
	raw, err := c.run(ctx, "mcp", "catalog", "server", "inspect", mcpCatalogRef, name, "--format", "json")
	if err != nil {
		return "", err
	}
	return formatMCPServerDetails(name, raw)
}

func (c Client) MCPConfiguredServers(ctx context.Context) (string, error) {
	// Lists server names and catalog references across profiles. Never use
	// 'profile show' or 'profile config', which may include sensitive values.
	raw, err := c.run(ctx, "mcp", "profile", "server", "ls")
	if err != nil {
		return "", err
	}
	output := Clean(string(raw), 16000)
	if strings.TrimSpace(output) == "" {
		return "(no configured MCP servers)", nil
	}
	return output, nil
}

func formatMCPServerDetails(requested string, raw []byte) (string, error) {
	if len(raw) > 1024*1024 {
		return "", errors.New("MCP catalog metadata exceeds size limit")
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("invalid MCP catalog JSON: %w", err)
	}
	if nested, ok := value["server"]; ok {
		var server map[string]json.RawMessage
		if json.Unmarshal(nested, &server) == nil && len(server) > 0 {
			value = server
		}
	}
	getString := func(key string) string {
		var result string
		_ = json.Unmarshal(value[key], &result)
		return Clean(result, 1500)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "SERVER ID: %s\n", Clean(requested, 128))
	for _, field := range []struct{ key, label string }{
		{"title", "TITLE"}, {"description", "DESCRIPTION"}, {"type", "TYPE"},
		{"image", "IMAGE"}, {"endpoint", "ENDPOINT"},
	} {
		if value := getString(field.key); value != "" {
			fmt.Fprintf(&b, "\n%s\n%s\n", field.label, value)
		}
	}
	b.WriteString("\nADVERTISED TOOLS (catalog metadata; not live gateway discovery)\n")
	type tool struct { Name, Description string }
	var advertised []tool
	var toolList []map[string]json.RawMessage
	if json.Unmarshal(value["tools"], &toolList) == nil {
		for _, entry := range toolList {
			var t tool
			_ = json.Unmarshal(entry["name"], &t.Name)
			_ = json.Unmarshal(entry["description"], &t.Description)
			if t.Name != "" { advertised = append(advertised, t) }
		}
	} else {
		// Catalog versions may represent tools as a map keyed by tool name.
		var toolMap map[string]json.RawMessage
		if json.Unmarshal(value["tools"], &toolMap) == nil {
			for key, rawTool := range toolMap {
				t := tool{Name: key}
				var entry struct { Description string }
				_ = json.Unmarshal(rawTool, &entry)
				t.Description = entry.Description
				advertised = append(advertised, t)
			}
		}
	}
	sort.Slice(advertised, func(i, j int) bool { return advertised[i].Name < advertised[j].Name })
	if len(advertised) == 0 {
		b.WriteString("(tool names are not supplied by this catalog entry)\n")
	}
	for i, t := range advertised {
		if i >= 80 {
			fmt.Fprintf(&b, "(%d additional tools omitted)\n", len(advertised)-i)
			break
		}
		fmt.Fprintf(&b, "- %s\n", Clean(t.Name, 150))
		if t.Description != "" {
			fmt.Fprintf(&b, "  %s\n", Clean(t.Description, 350))
		}
	}
	b.WriteString("\nREAD ONLY: no MCP server or tool was started, configured, or called.\n")
	return Clean(b.String(), 25000), nil
}
