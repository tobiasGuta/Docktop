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
	key := strings.Join(args[:min(2, len(args))], " ")
	if key == f.fail {
		return nil, errors.New("unavailable")
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
