package dockercli

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// MCPCatalogServerRef produces only Docker's official, pinned-to-catalog
// reference namespace. No user-supplied URI, local file or remote registry
// address can pass through this path.
func MCPCatalogServerRef(name string) (string, error) {
	if !identifier(name) {
		return "", errors.New("invalid MCP catalog server name")
	}
	return "catalog://mcp/docker-mcp-catalog/" + name, nil
}

// MCPAddCatalogServer uses the catalog server selected by the user to add
// *one* server to one confirmed profile. It does not run the MCP gateway,
// install Docker containers, or request blanket tool enablement.
// IMPORTANT: Docker MCP itself may expose tools when a server is assigned;
// the UI must warn the user of that possibility before asking for approval.
func (c Client) MCPAddCatalogServer(ctx context.Context, profileID, serverName string) error {
	ref, err := MCPCatalogServerRef(serverName)
	if err != nil {
		return err
	}
	if !identifier(profileID) {
		return errors.New("invalid MCP profile identifier")
	}
	profiles, err := c.MCPProfilesSafe(ctx)
	if err != nil {
		return fmt.Errorf("profile recheck failed: %w", err)
	}
	exists := false
	for _, profile := range profiles {
		if profile.ID == profileID {
			exists = true
			break
		}
	}
	if !exists {
		return errors.New("selected profile no longer exists")
	}
	members, err := c.MCPProfileMembers(ctx, profileID)
	if err != nil {
		return fmt.Errorf("assigned-server recheck failed: %w", err)
	}
	for _, member := range members {
		if member.Name == serverName {
			return errors.New("this server is already assigned to the profile")
		}
	}
	details, err := c.MCPServerDetails(ctx, serverName)
	if err != nil {
		return fmt.Errorf("cannot confirm server in Docker MCP catalog: %w", err)
	}
	if !strings.Contains(details, "SERVER ID: "+serverName) {
		return errors.New("unrecognized catalog inspection result")
	}
	_, err = c.run(ctx, "mcp", "profile", "server", "add", profileID, "--server", ref)
	return err
}
