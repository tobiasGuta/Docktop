package dockercli

import (
	"context"
	"strings"
	"testing"
)

func TestComposeProjects(t *testing.T) {
	f := &fakeRunner{responses: map[string]string{"compose ls": `[{"Name":"test","Status":"running(2)","ConfigFiles":"/tmp/compose.yaml"}]`}}
	v, e := (Client{Runner: f}).ComposeProjects(context.Background())
	if e != nil || len(v) != 1 || v[0].Name != "test" {
		t.Fatalf("%+v %v", v, e)
	}
}
func TestHubSearchAndPullAdmission(t *testing.T) {
	f := &fakeRunner{responses: map[string]string{"search --limit": "{\"Name\":\"nginx\",\"Description\":\"server\",\"StarCount\":\"122\",\"IsOfficial\":\"true\"}\n"}}
	c := Client{Runner: f}
	images, e := c.SearchHub(context.Background(), "nginx")
	if e != nil || len(images) != 1 || images[0].StarCount != 122 || !images[0].Official {
		t.Fatalf("%+v %v", images, e)
	}
	before := len(f.calls)
	for _, name := range []string{"--privileged", "bad;name", "docker.io/evil:latest"} {
		if err := c.PullHub(context.Background(), name, "default"); err == nil {
			t.Fatal("allowed", name)
		}
	}
	if len(f.calls) != before {
		t.Fatal("executed rejected pull")
	}
	if err := c.PullHub(context.Background(), "nginx", "default"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls[len(f.calls)-1], " "); got != "--context default pull nginx" {
		t.Fatal(got)
	}
}
func TestInspectRedactsAllValues(t *testing.T) {
	f := &fakeRunner{responses: map[string]string{"inspect --type": `[{"Id":"abcdefabcdef","Name":"/api","Config":{"Image":"nginx","Env":["API_KEY=super-secret","NORMAL=also-secret"]},"State":{"Status":"running","Health":{"Status":"healthy"}},"Mounts":[{"Type":"volume","Source":"/src","Destination":"/dst"}],"NetworkSettings":{"Networks":{"app":{}}}}]`}}
	v, e := (Client{Runner: f}).InspectSummary(context.Background(), "container", "abcdefabcdef")
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"super-secret", "also-secret"} {
		if strings.Contains(v, secret) {
			t.Fatal("leaked secret", secret)
		}
	}
	if !strings.Contains(v, "API_KEY=[REDACTED]") || !strings.Contains(v, "app") {
		t.Fatal(v)
	}
}
func TestInspectRejectsUnsafe(t *testing.T) {
	f := &fakeRunner{}
	_, e := (Client{Runner: f}).InspectSummary(context.Background(), "network", "--host")
	if e == nil || len(f.calls) != 0 {
		t.Fatal("unsafe resource executed or error absent")
	}
}
