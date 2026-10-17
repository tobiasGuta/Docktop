package dockercli

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
)

// MCPSecretMetadata contains only secret identifiers and provider labels.
// Never retrieve or expose secret values through this read-only feature.
type MCPSecretMetadata struct {
	Name     string
	Provider string
}

var mcpSecretNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,127}$`)
var mcpSecretProviderRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// MCPSecrets queries Docker's metadata-only inventory on demand.
// CLI errors are not returned verbatim because they may contain sensitive text.
func (c Client) MCPSecrets(ctx context.Context) ([]MCPSecretMetadata, error) {
	raw, err := c.run(ctx, "mcp", "secret", "ls", "--json")
	if err != nil {
		return nil, errors.New("Docker MCP secret inventory unavailable")
	}
	return parseMCPSecrets(raw)
}

func parseMCPSecrets(raw []byte) ([]MCPSecretMetadata, error) {
	if len(raw) > 1024*1024 {
		return nil, errors.New("MCP secret inventory exceeds size limit")
	}
	var records []json.RawMessage
	if err := json.Unmarshal(raw, &records); err != nil || records == nil {
		return nil, errors.New("invalid MCP secret inventory format")
	}
	if len(records) > 200 {
		return nil, errors.New("too many MCP secret records")
	}
	out := make([]MCPSecretMetadata, 0, len(records))
	seen := make(map[string]bool)
	for _, record := range records {
		var fields struct {
			Name string `json:"name"`
			Provider string `json:"provider"`
		}
		if err := json.Unmarshal(record, &fields); err != nil ||
			!mcpSecretNameRE.MatchString(fields.Name) ||
			!mcpSecretProviderRE.MatchString(fields.Provider) ||
			seen[fields.Name] {
			return nil, errors.New("invalid MCP secret inventory record")
		}
		seen[fields.Name] = true
		out = append(out, MCPSecretMetadata{Name: fields.Name, Provider: fields.Provider})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
