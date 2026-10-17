package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

type mcpSecretsMsg struct {
	values []dockercli.MCPSecretMetadata
	err error
}

func fetchMCPSecrets(c dockercli.Client) tea.Cmd {
	return func() tea.Msg {
		values, err := c.MCPSecrets(context.Background())
		return mcpSecretsMsg{values: values, err: err}
	}
}

func mcpSecretsText(values []dockercli.MCPSecretMetadata) string {
	lines := []string{
		"READ-ONLY DOCKER MCP SECRET INVENTORY",
		"",
		"These are stored secret identifiers and storage providers.",
		"No secret values are fetched. Stored does NOT mean authenticated,",
		"valid, correctly scoped, or available to every MCP profile.",
		"",
		fmt.Sprintf("SECRETS LISTED: %d", len(values)),
		"",
	}
	if len(values) == 0 {
		lines = append(lines, "Secret storage is reachable; no MCP secrets listed.")
	}
	for _, secret := range values {
		lines = append(lines, fmt.Sprintf("%s  [provider: %s]", secret.Name, secret.Provider))
	}
	return strings.Join(lines, "\n")
}

