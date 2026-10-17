package dockercli

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Profile creation is opt-in, constrained to a simple identifier, and does
// not activate experimental features, add servers, or enable any tools.
var newProfileNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,47}$`)

func ValidMCPProfileName(name string) bool {
	return newProfileNameRE.MatchString(name)
}

func (c Client) MCPCreateProfile(ctx context.Context, name string) error {
	if !ValidMCPProfileName(name) {
		return errors.New("profile name must be 1-48 letters, digits, dots, underscores or hyphens, starting with a letter or digit")
	}
	// Fail closed if profile discovery becomes unavailable or changes shape.
	profiles, err := c.MCPProfilesSafe(ctx)
	if err != nil {
		return fmt.Errorf("cannot verify existing profiles: %w", err)
	}
	for _, profile := range profiles {
		if strings.EqualFold(profile.ID, name) || strings.EqualFold(profile.Name, name) {
			return errors.New("a profile with this name already exists")
		}
	}
	_, err = c.run(ctx, "mcp", "profile", "create", "--name", name)
	return err
}
