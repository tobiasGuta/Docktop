package dockercli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ComposeProject is a summary from `docker compose ls --all`. This does not
// start, stop, or otherwise interact with individual projects.
type ComposeProject struct {
	Name        string `json:"Name"`
	Status      string `json:"Status"`
	ConfigFiles string `json:"ConfigFiles"`
}

func (c Client) ComposeProjects(ctx context.Context) ([]ComposeProject, error) {
	raw, err := c.run(ctx, "compose", "ls", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	var projects []ComposeProject
	if err := json.Unmarshal(raw, &projects); err != nil {
		return nil, fmt.Errorf("invalid compose response: %w", err)
	}
	if len(projects) > 500 {
		projects = projects[:500]
	}
	return projects, nil
}

type HubImage struct {
	Name        string `json:"Name"`
	Description string `json:"Description"`
	StarCount   int    `json:"-"`
	Official    bool   `json:"-"`
}

// Docker CLI uses string values for some fields, including star counts.
func (h *HubImage) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	_ = json.Unmarshal(fields["Name"], &h.Name)
	_ = json.Unmarshal(fields["Description"], &h.Description)
	var stars string
	if err := json.Unmarshal(fields["StarCount"], &stars); err == nil {
		h.StarCount, _ = strconv.Atoi(stars)
	} else {
		_ = json.Unmarshal(fields["StarCount"], &h.StarCount)
	}
	var official string
	if err := json.Unmarshal(fields["IsOfficial"], &official); err == nil {
		h.Official = strings.EqualFold(official, "true") || strings.EqualFold(official, "[OK]")
	} else {
		_ = json.Unmarshal(fields["IsOfficial"], &h.Official)
	}
	return nil
}

var searchRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,79}$`)
var imageRE = regexp.MustCompile(`^[a-z0-9]+(?:[._/-][a-z0-9]+)*$`)

func (c Client) SearchHub(ctx context.Context, query string) ([]HubImage, error) {
	if !searchRE.MatchString(query) {
		return nil, errors.New("search must be 1-80 letters, numbers, dots, hyphens or underscores")
	}
	raw, err := c.run(ctx, "search", "--limit", "15", "--format", "{{json .}}", query)
	if err != nil {
		return nil, err
	}
	images, err := decodeLines[HubImage](raw)
	if err != nil {
		return nil, err
	}
	for i := range images {
		images[i].Description = Clean(images[i].Description, 500)
	}
	return images, nil
}

// PullHub pins the Docker context captured when the user reviewed the image.
// It must only be called after explicit UI confirmation.
func (c Client) PullHub(ctx context.Context, name, dockerContext string) error {
	if len(name) > 180 || !imageRE.MatchString(name) {
		return errors.New("invalid Docker Hub image name")
	}
	if dockerContext == "" || len(dockerContext) > 128 || strings.HasPrefix(dockerContext, "-") || strings.ContainsAny(dockerContext, "\x00\r\n") {
		return errors.New("invalid Docker context")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	_, err := c.Runner.Run(ctx, "--context", dockerContext, "pull", name)
	return err
}

// InspectSummary only uses selected fields from Docker's inspect response.
// Environment *values* and container labels are never exposed, even if their
// names look harmless, since they commonly contain credentials.
func (c Client) InspectSummary(ctx context.Context, kind, id string) (string, error) {
	switch kind {
	case "container":
		if !idRE.MatchString(id) {
			return "", errors.New("invalid container ID")
		}
	case "network", "volume":
		if id == "" || len(id) > 256 || strings.HasPrefix(id, "-") || strings.ContainsAny(id, "\x00\r\n") {
			return "", errors.New("invalid resource name")
		}
	default:
		return "", errors.New("unsupported inspect type")
	}
	args := []string{"inspect", "--type", "container", id}
	if kind == "network" {
		args = []string{"network", "inspect", id}
	}
	if kind == "volume" {
		args = []string{"volume", "inspect", id}
	}
	raw, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	var values []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return "", fmt.Errorf("invalid inspect response: %w", err)
	}
	if len(values) != 1 {
		return "", errors.New("unexpected inspect response count")
	}
	data := values[0]
	var b strings.Builder
	appendLine := func(label, value string) {
		if value != "" {
			fmt.Fprintf(&b, "%s: %s\n", label, Clean(value, 600))
		}
	}
	get := func(key string) string { var str string; _ = json.Unmarshal(data[key], &str); return str }
	appendLine("Name", get("Name"))
	appendLine("ID", get("Id"))
	switch kind {
	case "container":
		var config struct {
			Image string
			Env   []string
		}
		_ = json.Unmarshal(data["Config"], &config)
		appendLine("Image", config.Image)
		var state struct {
			Status string
			Health struct{ Status string }
		}
		_ = json.Unmarshal(data["State"], &state)
		appendLine("State", state.Status)
		appendLine("Health", state.Health.Status)
		var mounts []struct {
			Type, Name, Source, Destination, Mode string
			RW                                    bool
		}
		_ = json.Unmarshal(data["Mounts"], &mounts)
		b.WriteString("\nMOUNTS\n")
		for i, v := range mounts {
			if i == 30 {
				b.WriteString("(more mounts omitted)\n")
				break
			}
			appendLine("- ", v.Type+" "+v.Source+" => "+v.Destination+" ("+v.Mode+")")
		}
		var networks struct{ Networks map[string]json.RawMessage }
		_ = json.Unmarshal(data["NetworkSettings"], &networks)
		names := make([]string, 0, len(networks.Networks))
		for k := range networks.Networks {
			names = append(names, k)
		}
		sort.Strings(names)
		b.WriteString("\nNETWORKS\n")
		for i, k := range names {
			if i == 30 {
				break
			}
			appendLine("- ", k)
		}
		b.WriteString("\nENVIRONMENT KEYS (values always hidden)\n")
		keys := make([]string, 0, len(config.Env))
		for _, env := range config.Env {
			k, _, ok := strings.Cut(env, "=")
			if ok {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i == 40 {
				b.WriteString("(more keys omitted)\n")
				break
			}
			appendLine("- ", k+"=[REDACTED]")
		}
	case "network":
		appendLine("Driver", get("Driver"))
		appendLine("Scope", get("Scope"))
		var containers map[string]struct{ Name string }
		_ = json.Unmarshal(data["Containers"], &containers)
		names := make([]string, 0, len(containers))
		for _, v := range containers {
			names = append(names, v.Name)
		}
		sort.Strings(names)
		b.WriteString("\nATTACHED CONTAINERS\n")
		if len(names) == 0 {
			b.WriteString("(none)\n")
		}
		for i, n := range names {
			if i == 50 {
				break
			}
			appendLine("- ", n)
		}
	case "volume":
		appendLine("Driver", get("Driver"))
		appendLine("Mountpoint", get("Mountpoint"))
		// Query only when a user inspects a volume; never poll every volume.
		attached, findErr := c.run(ctx, "ps", "-a", "--filter", "volume="+id, "--format", "{{json .}}")
		b.WriteString("\nATTACHED CONTAINERS\n")
		if findErr != nil {
			b.WriteString("(attachment query unavailable)\n")
		} else {
			containers, parseErr := decodeLines[Container](attached)
			if parseErr != nil {
				b.WriteString("(attachment output invalid)\n")
			} else if len(containers) == 0 {
				b.WriteString("(none)\n")
			} else {
				for i, v := range containers {
					if i == 50 {
						b.WriteString("(more omitted)\n")
						break
					}
					appendLine("- ", v.Names+" ["+v.State+"]")
				}
			}
		}
	}
	return Clean(b.String(), 16000), nil
}

func (c Client) MCPProfiles(ctx context.Context) (string, error) {
	raw, err := c.run(ctx, "mcp", "profile", "list")
	if err != nil {
		return "", err
	}
	return Clean(string(raw), 12000), nil
}
