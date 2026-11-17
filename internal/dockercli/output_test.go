package dockercli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCappedOutputDrainsInputWithoutRetainingBeyondLimit(t *testing.T) {
	stream := strings.NewReader(strings.Repeat("0123", 20_000))
	out := &cappedOutput{limit: 5}
	written, err := io.Copy(out, stream)
	if written != 80_000 || err != nil || !out.truncated || out.buffer.String() != "01230" {
		t.Fatalf("unexpected copy result: %d, %v, %q, truncated %t", written, err, out.buffer.String(), out.truncated)
	}
}

func TestExecRunnerSeparatesStructuredOutputAndBoundsLogs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Docker CLI fixture uses a Unix shell")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKTOP_TEST_LOG", filepath.Join(dir, "log"))
	fixture := `#!/bin/sh
if [ "$1" = "--context" ]; then
    shift 2
fi
if [ "$1" = "logs" ]; then
    printf 'container stdout\n'
    printf 'container stderr\n' >&2
    cat "$DOCKTOP_TEST_LOG"
else
    printf '%s\n' '{"key":"value"}'
    printf 'Docker warning\n' >&2
fi
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(fixture), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "log"), []byte(strings.Repeat("line\n", 5000)), 0600); err != nil {
		t.Fatal(err)
	}

	output, err := (ExecRunner{}).Run(context.Background(), "inspect")
	if err != nil || string(output) != "{\"key\":\"value\"}\n" {
		t.Fatalf("unexpected structured result %q, %v", output, err)
	}
	logs, err := (Client{Runner: ExecRunner{}}).Logs(context.Background(), "default", "abcdefabcdef")
	if err != nil || !strings.HasPrefix(logs, "[Log output truncated]\ncontainer stdout\ncontainer stderr\n") || len(logs) > maxLogsOutput+24 {
		t.Fatalf("unexpected log result of %d bytes, %v", len(logs), err)
	}
}
