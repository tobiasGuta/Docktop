package dockercli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type imageRemoveRunner struct {
	calls [][]string
	initial string
	current string
	contextName string
	failAt string
	changed bool
}

func (f *imageRemoveRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	key := strings.Join(args, " ")
	if key == f.failAt {return nil, errors.New("PRIVATE_TOKEN_NOT_FOR_UI")}
	if key == "context show" {return []byte(f.contextName+"\n"), nil}
	if key == "--context default image ls --no-trunc --format {{json .}}" {
		if f.changed {return []byte(f.current),nil}
		return []byte(f.initial),nil
	}
	if key == "--context default image rm -- hello-world:latest" {
		f.changed = true
		return []byte("Untagged: hello-world:latest"),nil
	}
	return nil,errors.New("unexpected Docker invocation")
}

const helloImage = `{"ID":"sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd","Repository":"hello-world","Tag":"latest"}` + "\n"
const changedImage = `{"ID":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","Repository":"hello-world","Tag":"latest"}` + "\n"

func helloTarget() ImageRemovalTarget {
	return ImageRemovalTarget{Context:"default",Repository:"hello-world",Tag:"latest",ID:"abcdefabcdef"}
}

func TestImageTagRemovalUsesExactReferenceNoForceAndReadback(t *testing.T) {
	f:=&imageRemoveRunner{initial:helloImage,contextName:"default"}
	r:=(Client{Runner:f}).RemoveImageTag(context.Background(),helloTarget())
	if r.Outcome!="verified" || len(f.calls)!=4 {t.Fatalf("outcome %+v; calls %#v",r,f.calls)}
	want:=[]string{"--context","default","image","rm","--","hello-world:latest"}
	if !reflect.DeepEqual(f.calls[2],want) {t.Fatalf("unsafe CLI args %q",f.calls[2])}
}

func TestImageRemovalBlocksStaleOrUnsafeWithoutWrite(t *testing.T) {
	cases:=[]struct{name string; target ImageRemovalTarget; context, list string}{
		{"context change",helloTarget(),"remote",helloImage},
		{"retargeted tag",helloTarget(),"default",changedImage},
		{"missing tag",helloTarget(),"default",""},
		{"ambiguous duplicate",helloTarget(),"default",helloImage+helloImage},
		{"invalid repository",ImageRemovalTarget{Context:"default",Repository:"--force",Tag:"latest",ID:"abcdefabcdef"},"default",helloImage},
		{"dangling",ImageRemovalTarget{Context:"default",Repository:"<none>",Tag:"<none>",ID:"abcdefabcdef"},"default",helloImage},
		{"command injection",ImageRemovalTarget{Context:"default",Repository:"hello-world;rm",Tag:"latest",ID:"abcdefabcdef"},"default",helloImage},
		{"no identity",ImageRemovalTarget{Context:"default",Repository:"hello-world",Tag:"latest",ID:"<none>"},"default",helloImage},
	}
	for _,tc:=range cases {
		t.Run(tc.name,func(t *testing.T){
			f:=&imageRemoveRunner{initial:tc.list,contextName:tc.context}
			r:=(Client{Runner:f}).RemoveImageTag(context.Background(),tc.target)
			if r.Outcome!="blocked" {t.Fatalf("not blocked: %+v",r)}
			for _,call:=range f.calls {if strings.Contains(strings.Join(call," ")," image rm ") {t.Fatalf("dispatched %q",call)}}
		})
	}
}

func TestImageRemovalUnverifiedAfterAttemptOrBadReadback(t *testing.T) {
	f:=&imageRemoveRunner{initial:helloImage,contextName:"default",failAt:"--context default image rm -- hello-world:latest"}
	r:=(Client{Runner:f}).RemoveImageTag(context.Background(),helloTarget())
	if r.Outcome!="unverified"||strings.Contains(r.Reason,"PRIVATE_TOKEN") {t.Fatalf("unsafe result %+v",r)}
	f=&imageRemoveRunner{initial:helloImage,current:helloImage,contextName:"default"}
	r=(Client{Runner:f}).RemoveImageTag(context.Background(),helloTarget())
	if r.Outcome!="unverified" {t.Fatalf("readback falsely verified %+v",r)}
	f=&imageRemoveRunner{initial:helloImage,contextName:"default",failAt:"--context default image ls --no-trunc --format {{json .}}"}
	r=(Client{Runner:f}).RemoveImageTag(context.Background(),helloTarget())
	if r.Outcome!="blocked" {t.Fatalf("preflight failure must block %+v",r)}
}

func TestImageRemovalTargetValidation(t *testing.T) {
	target:=helloTarget()
	if ValidateImageRemovalTarget(target)!=nil {t.Fatal("hello-world selection invalid")}
	for _,ref:=range []string{"registry.example.com:5000/app/image","GHCR.IO/repository"} {
		target.Repository=ref
		if ValidateImageRemovalTarget(target)!=nil{t.Errorf("legitimate repo rejected: %s",ref)}
	}
	if _,err:=parseImageListing([]byte(strings.Repeat("x",4*1024*1024+1)));err==nil{
		t.Fatal("accepted unbounded listing")
	}
}

type imageRemovalErrorRunner struct {
	after      string
	removeErr  error
	readErr    bool
	readCount  int
	removeCount int
}

func (f *imageRemovalErrorRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	switch strings.Join(args, " ") {
	case "context show":
		return []byte("default\n"), nil
	case "--context default image ls --no-trunc --format {{json .}}":
		f.readCount++
		if f.readCount == 1 {
			return []byte(helloImage), nil
		}
		if f.readErr {
			return nil, errors.New("PRIVATE_TOKEN_NOT_FOR_UI")
		}
		return []byte(f.after), nil
	case "--context default image rm -- hello-world:latest":
		f.removeCount++
		return nil, f.removeErr
	default:
		return nil, errors.New("unexpected Docker invocation")
	}
}

func TestImageRemovalErrorReportingAfterReadback(t *testing.T) {
	tests := []struct {
		name        string
		removeErr   error
		after       string
		readErr     bool
		wantOutcome string
		wantReason  string
	}{
		{
			name: "container conflict unchanged tag",
			removeErr: errors.New("conflict: image is being used by stopped container 566fb4 PRIVATE_TOKEN_NOT_FOR_UI"),
			after: helloImage,
			wantOutcome: "not_removed",
			wantReason: "existing container",
		},
		{
			name: "generic command failure unchanged tag",
			removeErr: errors.New("unknown flag or permission denied PRIVATE_TOKEN_NOT_FOR_UI"),
			after: helloImage,
			wantOutcome: "not_removed",
			wantReason: "Docker reported an error",
		},
		{
			name: "removal error but tag absent",
			removeErr: errors.New("PRIVATE_TOKEN_NOT_FOR_UI"),
			after: "",
			wantOutcome: "verified",
			wantReason: "does not prove the command succeeded",
		},
		{
			name: "retargeted image after command failure",
			removeErr: errors.New("PRIVATE_TOKEN_NOT_FOR_UI"),
			after: changedImage,
			wantOutcome: "unverified",
			wantReason: "ambiguous",
		},
		{
			name: "duplicate reference after command failure",
			removeErr: errors.New("PRIVATE_TOKEN_NOT_FOR_UI"),
			after: helloImage + helloImage,
			wantOutcome: "unverified",
			wantReason: "ambiguous",
		},
		{
			name: "readback failure",
			removeErr: errors.New("PRIVATE_TOKEN_NOT_FOR_UI"),
			readErr: true,
			wantOutcome: "unverified",
			wantReason: "readback failed",
		},
		{
			name: "success status but tag still present",
			after: helloImage,
			wantOutcome: "unverified",
			wantReason: "still lists",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &imageRemovalErrorRunner{after: tc.after, removeErr: tc.removeErr, readErr: tc.readErr}
			result := (Client{Runner: f}).RemoveImageTag(context.Background(), helloTarget())
			if result.Outcome != tc.wantOutcome || !strings.Contains(result.Reason, tc.wantReason) {
				t.Fatalf("unexpected outcome: %+v", result)
			}
			if strings.Contains(result.Reason, "PRIVATE_TOKEN_NOT_FOR_UI") || strings.Contains(result.Reason, "566fb4") {
				t.Fatalf("Docker stderr leaked: %+v", result)
			}
			if f.removeCount != 1 || f.readCount != 2 {
				t.Fatalf("expected one removal attempt and both image-list reads: %+v", f)
			}
		})
	}
}
