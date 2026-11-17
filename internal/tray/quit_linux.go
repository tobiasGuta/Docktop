package tray

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	trayBusName = "io.github.tobiasGuta.Docktop.Tray"
	trayPath    = dbus.ObjectPath("/io/github/tobiasGuta/Docktop/Tray")
	quitMember  = "QuitDashboards"
)

func connectTray(ctx context.Context) (*dbus.Conn, error) {
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return nil, errors.New("tray needs a desktop D-Bus session")
	}
	if err := checkTrayHost(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}
	reply, err := conn.RequestName(trayBusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("register Docktop tray: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return nil, errors.New("Docktop tray is already running in this desktop session")
	}
	return conn, nil
}

func quitDashboards(ctx context.Context, conn *dbus.Conn) error {
	if err := conn.Emit(trayPath, trayBusName+"."+quitMember); err != nil {
		return err
	}
	// Wait for the bus to process the signal before releasing the sender name.
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetId", 0).Err
}

// WatchQuit subscribes to the tray before the dashboard starts rendering.
// Without an available session bus, the dashboard still starts normally.
func WatchQuit(ctx context.Context, quit func()) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		return
	}
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return
	}
	signals := make(chan *dbus.Signal, 1)
	conn.Signal(signals)
	matchCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	err = conn.AddMatchSignalContext(matchCtx,
		dbus.WithMatchSender(trayBusName),
		dbus.WithMatchObjectPath(trayPath),
		dbus.WithMatchInterface(trayBusName),
		dbus.WithMatchMember(quitMember),
	)
	cancel()
	if err != nil {
		conn.Close()
		return
	}

	go func() {
		defer conn.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case signal, ok := <-signals:
				if !ok {
					return
				}
				if signal.Path == trayPath && signal.Name == trayBusName+"."+quitMember {
					quit()
					return
				}
			}
		}
	}()
}
