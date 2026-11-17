package tray

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDashboardLauncherPassesContextDirectly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "docktop-gui")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	launch, err := dashboardLauncher("", path)
	if err != nil {
		t.Fatal(err)
	}
	command := launch("/unused/docktop", "work")
	if want := []string{path, "--context", "work"}; !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("got %q, want %q", command.Args, want)
	}
}
