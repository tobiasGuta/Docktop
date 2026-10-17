package dockercli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type secretInventoryRunner struct {
	calls int
	raw []byte
	err error
}
func (f *secretInventoryRunner) Run(_ context.Context, args ...string) ([]byte,error) {
	f.calls++
	if strings.Join(args, " ") != "mcp secret ls --json" {
		return nil, errors.New("unexpected command")
	}
	return f.raw, f.err
}

func TestMCPSecretsStrictMetadataOnly(t *testing.T) {
	f := &secretInventoryRunner{raw: []byte(`[
		{"name":"github.personal_access_token","provider":"docker-pass","value":"DO_NOT_SHOW"},
		{"name":"oauth/github","provider":"docker-pass"}
	]`)}
	got,err:=(Client{Runner:f}).MCPSecrets(context.Background())
	if err!=nil || f.calls!=1 || len(got)!=2 {
		t.Fatalf("inventory result %+v %v",got,err)
	}
	if got[0].Name!="github.personal_access_token"||got[1].Name!="oauth/github" {
		t.Fatalf("unsorted names: %+v",got)
	}
	if strings.Contains(strings.TrimSpace(strings.Join([]string{got[0].Name,got[0].Provider,got[1].Name,got[1].Provider}," ")), "DO_NOT_SHOW") {
		t.Fatal("secret value leaked")
	}
}

func TestMCPSecretsEmptyAndAdversarialInput(t *testing.T) {
	if values,err:=parseMCPSecrets([]byte("[]"));err!=nil||len(values)!=0 {
		t.Fatalf("empty inventory %+v %v",values,err)
	}
	for _,raw:=range []string{
		"null", "{}", "[{}]", "not json",
		`[{"name":"a","provider":"docker-pass"},{"name":"a","provider":"docker-pass"}]`,
		`[{"name":"\u001b[2J","provider":"docker-pass"}]`,
		`[{"name":"github","provider":"bad\nprovider"}]`,
	} {
		if _,err:=parseMCPSecrets([]byte(raw));err==nil {
			t.Fatalf("accepted invalid inventory %q",raw)
		}
	}
	if _,err:=parseMCPSecrets([]byte(strings.Repeat("x",1024*1024+1)));err==nil {
		t.Fatal("accepted oversized inventory")
	}
}

func TestMCPSecretsDoNotSurfaceCLIError(t *testing.T) {
	f:=&secretInventoryRunner{err:errors.New("Authorization: ghp_PRIVATE_TOKEN")}
	_,err:=(Client{Runner:f}).MCPSecrets(context.Background())
	if err==nil||strings.Contains(err.Error(),"PRIVATE_TOKEN") {
		t.Fatalf("unsafe error: %v",err)
	}
}
