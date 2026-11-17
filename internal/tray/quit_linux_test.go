package tray

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestTrayQuitReachesDashboardsAcrossTrayRestarts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Run against a private bus so the test never signals real dashboards.
	const privateBus = "DOCKTOP_TEST_PRIVATE_BUS"
	if os.Getenv(privateBus) != "1" {
		launcher, err := exec.LookPath("dbus-run-session")
		if err != nil {
			t.Skip("dbus-run-session is unavailable")
		}
		cmd := exec.CommandContext(ctx, launcher, "--", os.Args[0], "-test.run=^TestTrayQuitReachesDashboardsAcrossTrayRestarts$")
		cmd.Env = append(os.Environ(), privateBus+"=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("private bus test: %v\n%s", err, out)
		}
		return
	}

	quits := make(chan int, 2)
	WatchQuit(ctx, func() { quits <- 1 })
	WatchQuit(ctx, func() { quits <- 2 })

	newTray := func() *dbus.Conn {
		t.Helper()
		conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		if reply, err := conn.RequestName(trayBusName, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
			t.Fatalf("request tray name: %v, %v", reply, err)
		}
		return conn
	}

	firstTray := newTray()
	if _, err := firstTray.ReleaseName(trayBusName); err != nil {
		t.Fatal(err)
	}
	firstTray.Close()

	restartedTray := newTray()
	if err := quitDashboards(ctx, restartedTray); err != nil {
		t.Fatal(err)
	}
	restartedTray.Close()

	closed := make(map[int]bool)
	for len(closed) < 2 {
		select {
		case id := <-quits:
			closed[id] = true
		case <-ctx.Done():
			t.Fatalf("received quit from dashboards %v", closed)
		}
	}
}
