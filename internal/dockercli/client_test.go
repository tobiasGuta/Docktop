package dockercli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls     [][]string
	responses map[string]string
	fail      string
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	// An explicit --context is required on all resource-listing reads.
	command := args
	if len(args) >= 2 && args[0] == "--context" {
		command = args[2:]
	}
	key := strings.Join(command[:min(2, len(command))], " ")
	if key == f.fail {
		return nil, errors.New("PRIVATE_DOCKER_SECRET_DO_NOT_RENDER")
	}
	return []byte(f.responses[key]), nil
}
func TestSnapshot(t *testing.T) {
	f := &fakeRunner{responses: map[string]string{
		"context show":      "default\n",
		"ps -a":             `{"ID":"abcdefabcdef","Names":"web","State":"running","Status":"Up 1h"}` + "\n",
		"image ls":          `{"ID":"sha256:foo","Repository":"nginx","Tag":"latest"}` + "\n",
		"network ls":        `{"Name":"bridge","Driver":"bridge"}` + "\n",
		"volume ls":         `{"Name":"db-data","Driver":"local"}` + "\n",
		"stats --no-stream": `{"ID":"abcdefabcdef","Name":"web","CPUPerc":"2%"}` + "\n",
	}}
	snap, err := (Client{Runner: f}).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Context != "default" || len(snap.Containers) != 1 || len(snap.Images) != 1 || len(snap.Networks) != 1 || len(snap.Volumes) != 1 || snap.Stats["abcdefabcdef"].CPUPerc != "2%" {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
}
func TestChangeRejectsUnsafeArguments(t *testing.T) {
	f := &fakeRunner{responses: map[string]string{}}
	c := Client{Runner: f}
	for _, p := range [][2]string{{"rm", "abcdefabcdef"}, {"stop", "--all"}, {"start", "bad;command"}} {
		if err := c.Change(context.Background(), p[0], p[1]); err == nil {
			t.Fatalf("accepted %v", p)
		}
	}
	if len(f.calls) != 0 {
		t.Fatal("ran rejected request")
	}
	if err := c.Change(context.Background(), "restart", "abcdefabcdef"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.calls[0], []string{"restart", "abcdefabcdef"}) {
		t.Fatal(f.calls)
	}
}
func TestMCPSearchIsOneArgument(t *testing.T) {
	f := &fakeRunner{responses: map[string]string{}}
	_, err := (Client{Runner: f}).MCP(context.Background(), "github; rm -rf /")
	if err != nil {
		t.Fatal(err)
	}
	got := f.calls[0]
	if got[len(got)-1] != "name=github; rm -rf /" {
		t.Fatalf("unexpected args: %q", got)
	}
}
func TestClean(t *testing.T) {
	got := Clean("a\x1b[31mRED\x1b[0m\x1b]0;title\a\x00b", 200)
	if got != "aREDb" {
		t.Fatalf("got %q", got)
	}
}
func TestSnapshotPreservesWarnings(t *testing.T) {
	f := &fakeRunner{responses: map[string]string{"context show": "default", "ps -a": ""}, fail: "stats --no-stream"}
	snap, err := (Client{Runner: f}).Snapshot(context.Background())
	if err != nil || len(snap.Warnings) != 1 {
		t.Fatalf("snapshot=%+v err=%v", snap, err)
	}
}

func TestSnapshotPinsAllReadsAndTimesEachCommand(t *testing.T) {
	f := &fakeRunner{responses: map[string]string{
		"context show": "default\n",
		"ps -a": `{"ID":"0123456789abcdef","Names":"web","State":"running"}` + "\n",
		"image ls": `{"ID":"sha256:abcd","Repository":"busybox","Tag":"latest"}` + "\n",
	}}
	snapshot, err := (Client{Runner: f}).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 6 || len(snapshot.QueryTimings) != 6 {
		t.Fatalf("expected six measured reads, calls=%d timings=%d", len(f.calls), len(snapshot.QueryTimings))
	}
	if !reflect.DeepEqual(f.calls[0], []string{"context", "show"}) {
		t.Fatalf("initial context command changed: %v", f.calls[0])
	}
	for _, args := range f.calls[1:] {
		if len(args) < 4 || args[0] != "--context" || args[1] != "default" {
			t.Fatalf("unscoped Docker query: %v", args)
		}
	}
	if snapshot.Duration < 0 {
		t.Fatal("snapshot duration cannot be negative")
	}
	step, duration := snapshot.SlowestQuery()
	if step == "" || duration < 0 {
		t.Fatalf("missing slowest command: %s %s", step, duration)
	}
}

func TestSnapshotRejectsUnsafeOrMissingContextBeforeReadingResources(t *testing.T) {
	for _, value := range []string{"", "\n", "--unexpected\n", "default\x1b[31m\n", "default\tmalicious\n", strings.Repeat("a", 129)} {
		f := &fakeRunner{responses: map[string]string{"context show": value}}
		s, err := (Client{Runner: f}).Snapshot(context.Background())
		if err == nil || len(f.calls) != 1 || s.Context != "" {
			t.Fatalf("unsafe context %q led to resource reads: err=%v calls=%v", value, err, f.calls)
		}
	}
}

func TestSnapshotErrorsNeverExposeDockerDiagnostics(t *testing.T) {
	f := &fakeRunner{responses: map[string]string{"context show": "default", "ps -a": ""}, fail: "stats --no-stream"}
	s, err := (Client{Runner: f}).Snapshot(context.Background())
	if err != nil || len(s.Warnings) != 1 || strings.Contains(strings.Join(s.Warnings, " "), "PRIVATE_DOCKER_SECRET") {
		t.Fatalf("raw Docker diagnostics leaked: snap=%+v err=%v", s, err)
	}
	f = &fakeRunner{responses: map[string]string{"context show": "default"}, fail: "ps -a"}
	_, err = (Client{Runner: f}).Snapshot(context.Background())
	if err == nil || strings.Contains(err.Error(), "PRIVATE_DOCKER_SECRET") {
		t.Fatalf("unsafe early failure: %v", err)
	}
}
