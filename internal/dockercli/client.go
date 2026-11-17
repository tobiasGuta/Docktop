package dockercli

import (
	"bytes"
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

const (
	maxCommandOutput    = 8 << 20
	maxLogsOutput       = 18_000
	maxDiagnosticOutput = 4096
)

type cappedOutput struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	keep := min(len(p), b.limit-b.buffer.Len())
	b.truncated = b.truncated || keep < len(p)
	_, _ = b.buffer.Write(p[:keep])
	return len(p), nil
}

func (ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("no Docker command specified")
	}
	command := args
	if len(args) > 2 && args[0] == "--context" {
		command = args[2:]
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.WaitDelay = time.Second
	out := &cappedOutput{limit: maxCommandOutput}
	diagnostics := &cappedOutput{limit: maxDiagnosticOutput}
	cmd.Stdout, cmd.Stderr = out, diagnostics
	logs := command[0] == "logs"
	if logs {
		out.limit = maxLogsOutput
		// Docker writes container stderr to the CLI's stderr stream.
		cmd.Stderr = out
	}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s", command[0], err, Clean(diagnostics.buffer.String(), 250))
	}
	if out.truncated {
		if !logs {
			return nil, errors.New("Docker command output exceeded size limit")
		}
		return out.buffer.Bytes(), errLogsTruncated
	}
	return out.buffer.Bytes(), nil
}

var errLogsTruncated = errors.New("Docker log output truncated")

type Client struct {
	Runner  Runner
	Context string
}

func New() Client { return Client{Runner: ExecRunner{}} }

func (c Client) CurrentContext(ctx context.Context) (string, error) {
	if c.Context != "" {
		if !validSnapshotContext(c.Context) {
			return "", errors.New("Docker context name invalid")
		}
		return c.Context, nil
	}
	raw, err := c.run(ctx, "context", "show")
	return parseContext(raw, err)
}

func parseContext(raw []byte, err error) (string, error) {
	if err != nil {
		return "", errors.New("Docker context unavailable")
	}
	name := strings.TrimSpace(string(raw))
	if !validSnapshotContext(name) {
		return "", errors.New("Docker context name invalid")
	}
	return name, nil
}

func (c Client) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	return c.Runner.Run(ctx, args...)
}

type Container struct {
	ID             string `json:"ID"`
	Names          string `json:"Names"`
	Image          string `json:"Image"`
	Status         string `json:"Status"`
	State          string `json:"State"`
	Ports          string `json:"Ports"`
	ComposeProject string `json:"ComposeProject"`
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
	NetIO    string `json:"NetIO"`
	BlockIO  string `json:"BlockIO"`
}

// QueryTiming is one read-only Docker command latency from the latest snapshot.
// Step names come from Docktop, never from Docker output or CLI arguments.
type QueryTiming struct {
	Step     string
	Duration time.Duration
}

type Snapshot struct {
	Context      string
	Containers   []Container
	Images       []Image
	Networks     []Network
	Volumes      []Volume
	Stats        map[string]Usage
	Warnings     []string
	Updated      time.Time
	Duration     time.Duration
	QueryTimings []QueryTiming
}

// SlowestQuery identifies the most expensive Docker CLI read in the last scan.
func (s Snapshot) SlowestQuery() (step string, duration time.Duration) {
	for _, sample := range s.QueryTimings {
		if sample.Duration > duration {
			step, duration = sample.Step, sample.Duration
		}
	}
	return step, duration
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

// validSnapshotContext rejects untrusted context-show output that cannot be
// safely used as a single Docker CLI argument or displayed in the TUI.
func validSnapshotContext(name string) bool {
	return name != "" && len(name) <= 128 && !strings.HasPrefix(name, "-") &&
		!strings.ContainsAny(name, "\x00\r\n\t\x1b") && Clean(name, 129) == name
}

// Snapshot reads from a single captured Docker context for all six Docker CLI
// queries. Docker resources may still change during the sequential scan; this
// prevents cross-context mixing, not concurrent changes within one engine.
// The complete scan has a deadline so a slow or remote engine cannot leave the
// UI waiting indefinitely. Diagnostics contain only fixed, safe wording.
func (c Client) Snapshot(ctx context.Context) (s Snapshot, snapshotErr error) {
	started := time.Now()
	s = Snapshot{Stats: make(map[string]Usage), Updated: started}
	defer func() { s.Duration = time.Since(started) }()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	read := func(step string, args ...string) ([]byte, error) {
		start := time.Now()
		data, err := c.run(ctx, args...)
		s.QueryTimings = append(s.QueryTimings, QueryTiming{Step: step, Duration: time.Since(start)})
		return data, err
	}

	name := c.Context
	if name == "" {
		raw, err := read("context", "context", "show")
		name, err = parseContext(raw, err)
		if err != nil {
			return s, err
		}
	} else if !validSnapshotContext(name) {
		return s, errors.New("Docker context name invalid")
	}
	s.Context = name

	// Every remaining query is pinned to the same captured context.
	raw, err := read("containers", "--context", name, "ps", "-a", "--no-trunc", "--format", containerFormat)
	if err != nil {
		return s, errors.New("Docker container listing unavailable")
	}
	s.Containers, err = decodeLines[Container](raw)
	if err != nil {
		return s, errors.New("Docker container listing invalid")
	}

	for _, request := range []struct {
		kind string
		args []string
		read func([]byte) error
	}{
		{"images", []string{"image", "ls", "--no-trunc", "--format", "{{json .}}"}, func(b []byte) error { v, e := decodeLines[Image](b); s.Images = v; return e }},
		{"networks", []string{"network", "ls", "--no-trunc", "--format", "{{json .}}"}, func(b []byte) error { v, e := decodeLines[Network](b); s.Networks = v; return e }},
		{"volumes", []string{"volume", "ls", "--format", "{{json .}}"}, func(b []byte) error { v, e := decodeLines[Volume](b); s.Volumes = v; return e }},
		{"stats", []string{"stats", "--no-stream", "--no-trunc", "--format", "{{json .}}"}, func(b []byte) error {
			v, e := decodeLines[Usage](b)
			for _, u := range v {
				s.Stats[u.ID] = u
				s.Stats[u.Name] = u
			}
			return e
		}},
	} {
		args := append([]string{"--context", name}, request.args...)
		output, runErr := read(request.kind, args...)
		if runErr != nil {
			s.Warnings = append(s.Warnings, request.kind+": Docker query unavailable")
			continue
		}
		if parseErr := request.read(output); parseErr != nil {
			s.Warnings = append(s.Warnings, request.kind+": invalid Docker data")
		}
	}
	return s, nil
}

const containerFormat = `{"ID":{{json .ID}},"Names":{{json .Names}},"Image":{{json .Image}},` +
	`"Status":{{json .Status}},"State":{{json .State}},"Ports":{{json .Ports}},` +
	`"ComposeProject":{{json (.Label "com.docker.compose.project")}}}`

var idRE = regexp.MustCompile(`^[a-fA-F0-9]{12,64}$`)

// Change allows only explicitly supported container operations on Docker IDs.
func (c Client) Change(ctx context.Context, dockerContext, operation, id string) error {
	if !idRE.MatchString(id) {
		return errors.New("invalid container ID")
	}
	switch operation {
	case "start", "stop", "restart":
	default:
		return errors.New("unsupported operation")
	}
	active, err := c.CurrentContext(ctx)
	if err != nil || active != dockerContext {
		return errors.New("Docker context changed or is unavailable. Refresh and review the container again.")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if _, err := c.Runner.Run(ctx, "--context", dockerContext, operation, "--", id); err != nil {
		return errors.New("Docker did not confirm the operation. Check the container state before retrying.")
	}
	return nil
}

func (c Client) Logs(ctx context.Context, dockerContext, id string) (string, error) {
	if !validSnapshotContext(dockerContext) {
		return "", errors.New("invalid Docker context")
	}
	if !idRE.MatchString(id) {
		return "", errors.New("invalid container ID")
	}
	out, err := c.run(ctx, "--context", dockerContext, "logs", "--tail", "100", "--timestamps", "--", id)
	if err != nil && !errors.Is(err, errLogsTruncated) {
		return "", err
	}
	if errors.Is(err, errLogsTruncated) || len(out) > maxLogsOutput {
		return "[Log output truncated]\n" + Clean(string(out), maxLogsOutput), nil
	}
	return Clean(string(out), maxLogsOutput), nil
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
	return Clean(string(raw), 300000), nil
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

// RunningContainerCount uses a single, read-only Docker command for the tray.
func (c Client) RunningContainerCount(ctx context.Context, dockerContext string) (int, error) {
	if !validSnapshotContext(dockerContext) {
		return 0, errors.New("invalid Docker context")
	}
	raw, err := c.run(ctx, "--context", dockerContext, "ps", "--quiet", "--filter", "status=running")
	if err != nil {
		return 0, err
	}
	return len(bytes.Fields(raw)), nil
}
