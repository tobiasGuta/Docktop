package dockercli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Runner executes Docker arguments without passing user values through a shell.
type Runner interface {
	Run(context.Context, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s", args[0], err, Clean(string(out), 250))
	}
	return out, nil
}

type Client struct{ Runner Runner }

func New() Client { return Client{Runner: ExecRunner{}} }

func (c Client) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	return c.Runner.Run(ctx, args...)
}

type Container struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	Status string `json:"Status"`
	State  string `json:"State"`
	Ports  string `json:"Ports"`
}
type Image struct {
	ID         string `json:"ID"`
	Repository string `json:"Repository"`
	Tag        string `json:"Tag"`
	Size       string `json:"Size"`
}
type Network struct {
	ID     string `json:"ID"`
	Name   string `json:"Name"`
	Driver string `json:"Driver"`
	Scope  string `json:"Scope"`
}
type Volume struct {
	Name       string `json:"Name"`
	Driver     string `json:"Driver"`
	Mountpoint string `json:"Mountpoint"`
}
type Usage struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
	MemPerc  string `json:"MemPerc"`
}
type Snapshot struct {
	Context    string
	Containers []Container
	Images     []Image
	Networks   []Network
	Volumes    []Volume
	Stats      map[string]Usage
	Warnings   []string
	Updated    time.Time
}

func decodeLines[T any](raw []byte) ([]T, error) {
	var result []T
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var item T
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("invalid docker JSON: %w", err)
		}
		result = append(result, item)
	}
	return result, nil
}

// Snapshot reads the active Docker CLI context. It never modifies Docker state.
func (c Client) Snapshot(ctx context.Context) (Snapshot, error) {
	s := Snapshot{Stats: make(map[string]Usage), Updated: time.Now()}
	raw, err := c.run(ctx, "context", "show")
	if err != nil {
		return s, err
	}
	s.Context = strings.TrimSpace(string(raw))
	raw, err = c.run(ctx, "ps", "-a", "--format", "{{json .}}")
	if err != nil {
		return s, err
	}
	s.Containers, err = decodeLines[Container](raw)
	if err != nil {
		return s, err
	}
	for _, request := range []struct {
		kind string
		args []string
		read func([]byte) error
	}{
		{"images", []string{"image", "ls", "--format", "{{json .}}"}, func(b []byte) error { v, e := decodeLines[Image](b); s.Images = v; return e }},
		{"networks", []string{"network", "ls", "--format", "{{json .}}"}, func(b []byte) error { v, e := decodeLines[Network](b); s.Networks = v; return e }},
		{"volumes", []string{"volume", "ls", "--format", "{{json .}}"}, func(b []byte) error { v, e := decodeLines[Volume](b); s.Volumes = v; return e }},
		{"stats", []string{"stats", "--no-stream", "--format", "{{json .}}"}, func(b []byte) error {
			v, e := decodeLines[Usage](b)
			for _, u := range v {
				s.Stats[u.ID] = u
				s.Stats[u.Name] = u
			}
			return e
		}},
	} {
		output, runErr := c.run(ctx, request.args...)
		if runErr != nil {
			s.Warnings = append(s.Warnings, request.kind+": "+runErr.Error())
			continue
		}
		if parseErr := request.read(output); parseErr != nil {
			s.Warnings = append(s.Warnings, request.kind+": "+parseErr.Error())
		}
	}
	return s, nil
}

var idRE = regexp.MustCompile(`^[a-fA-F0-9]{12,64}$`)

// Change allows only explicitly supported container operations on Docker IDs.
func (c Client) Change(ctx context.Context, operation, id string) error {
	if !idRE.MatchString(id) {
		return errors.New("invalid container ID")
	}
	switch operation {
	case "start", "stop", "restart":
	default:
		return errors.New("unsupported operation")
	}
	_, err := c.run(ctx, operation, id)
	return err
}

func (c Client) Logs(ctx context.Context, id string) (string, error) {
	if !idRE.MatchString(id) {
		return "", errors.New("invalid container ID")
	}
	out, err := c.run(ctx, "logs", "--tail", "100", "--timestamps", id)
	if err != nil {
		return "", err
	}
	return Clean(string(out), 18000), nil
}

func (c Client) MCP(ctx context.Context, filter string) (string, error) {
	args := []string{"mcp", "catalog", "server", "ls", "mcp/docker-mcp-catalog:latest"}
	if filter != "" {
		args = append(args, "--filter", "name="+filter)
	}
	raw, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	return Clean(string(raw), 50000), nil
}

// Clean rejects terminal control characters and limits untrusted CLI output.
// This also removes escape sequences, not just the escape character itself.
func Clean(input string, max int) string {
	var b strings.Builder
	runes := []rune(input)
	for i := 0; i < len(runes) && b.Len() < max; i++ {
		r := runes[i]
		if r == '\x1b' {
			if i+1 < len(runes) && runes[i+1] == '[' {
				i += 2
				for i < len(runes) && !(runes[i] >= '@' && runes[i] <= '~') {
					i++
				}
			} else if i+1 < len(runes) && runes[i+1] == ']' {
				i += 2
				for i < len(runes) && runes[i] != '\a' && runes[i] != '\x1b' {
					i++
				}
				if i < len(runes) && runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '\\' {
					i++
				}
			}
			continue
		}
		if r == '\n' || r == '\t' || (unicode.IsPrint(r) && !unicode.Is(unicode.Cf, r)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
