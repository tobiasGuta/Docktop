package tray

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"fyne.io/systray"
	"github.com/godbus/dbus/v5"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

const refreshInterval = 20 * time.Second

func Run(ctx context.Context, client dockercli.Client, terminal string) error {
	if err := checkTrayHost(ctx); err != nil {
		return err
	}
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve Docktop executable: %w", err)
	}
	launch, err := terminalLauncher(terminal)
	if err != nil && terminal != "" {
		return err
	}

	systray.Run(func() {
		systray.SetTitle("Docktop")
		systray.SetIcon(trayIcon(false))
		status := systray.AddMenuItem("Checking Docker…", "")
		status.Disable()
		open := systray.AddMenuItem("Open Docktop", "Open the dashboard in a terminal")
		if launch == nil {
			open.SetTitle("Open Docktop (terminal unavailable)")
		}
		open.Disable()
		refresh := systray.AddMenuItem("Refresh", "Refresh Docker status")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "Quit the Docktop tray")

		go func() {
			pollCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			timer := time.NewTicker(refreshInterval)
			defer timer.Stop()

			type summary struct {
				dockerContext string
				count         int
				err           error
			}
			results := make(chan summary, 1)
			polling := false
			dockerContext := ""
			poll := func() {
				if polling {
					return
				}
				polling = true
				name := dockerContext
				go func() {
					var result summary
					result.dockerContext = name
					if name == "" {
						result.dockerContext, result.err = client.CurrentContext(pollCtx)
					}
					if result.err == nil {
						result.count, result.err = client.RunningContainerCount(pollCtx, result.dockerContext)
					}
					results <- result
				}()
			}
			poll()

			launchErrors := make(chan error, 1)
			launching := false
			for {
				select {
				case <-pollCtx.Done():
					systray.Quit()
					return
				case <-quit.ClickedCh:
					systray.Quit()
					return
				case <-timer.C:
					poll()
				case <-refresh.ClickedCh:
					poll()
				case result := <-results:
					polling = false
					dockerContext = result.dockerContext
					label := "Docker unavailable"
					if result.err == nil {
						label = fmt.Sprintf("%d containers running", result.count)
					}
					if dockerContext != "" {
						label = dockerContext + " · " + label
						if launch != nil {
							open.Enable()
						}
					}
					systray.SetIcon(trayIcon(result.err == nil))
					systray.SetTooltip("Docktop · " + label)
					status.SetTitle(label)
				case <-open.ClickedCh:
					if launch == nil || dockerContext == "" || launching {
						continue
					}
					launching = true
					command := launch(binary, dockerContext)
					go func() {
						launchErrors <- command.Run()
					}()
				case err := <-launchErrors:
					launching = false
					if err != nil {
						status.SetTitle("Terminal launch failed")
						fmt.Fprintln(os.Stderr, "docktop tray:", err)
					}
				}
			}
		}()
	}, nil)
	return nil
}

const watcher = "org.kde.StatusNotifierWatcher"

func checkTrayHost(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return errors.New("tray needs a desktop D-Bus session")
	}
	defer conn.Close()
	var value dbus.Variant
	err = conn.Object(watcher, "/StatusNotifierWatcher").
		CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, watcher, "IsStatusNotifierHostRegistered").
		Store(&value)
	if registered, _ := value.Value().(bool); err != nil || !registered {
		return errors.New("no desktop tray host found. Enable a StatusNotifierItem host such as the GNOME AppIndicator extension")
	}
	return nil
}

func terminalLauncher(preferred string) (func(string, string) *exec.Cmd, error) {
	candidates := []string{"xdg-terminal-exec", "ptyxis", "gnome-terminal", "kgx", "konsole", "xfce4-terminal", "kitty", "xterm"}
	if preferred != "" {
		candidates = []string{preferred}
	}
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		var prefix []string
		switch filepath.Base(path) {
		case "xdg-terminal-exec", "kitty":
		case "ptyxis":
			prefix = []string{"--new-window", "--"}
		case "gnome-terminal", "kgx":
			prefix = []string{"--"}
		case "xfce4-terminal":
			prefix = []string{"-x"}
		default:
			prefix = []string{"-e"}
		}
		return func(binary, dockerContext string) *exec.Cmd {
			args := append(append([]string{}, prefix...), binary, "--context", dockerContext)
			return exec.Command(path, args...)
		}, nil
	}
	return nil, errors.New("no compatible terminal found. Use --terminal with a terminal executable")
}

func trayIcon(available bool) []byte {
	ink := color.NRGBA{R: 156, G: 163, B: 175, A: 255}
	if available {
		ink = color.NRGBA{R: 55, G: 210, B: 175, A: 255}
	}
	icon := image.NewNRGBA(image.Rect(0, 0, 24, 24))
	source := image.NewUniform(ink)
	for _, block := range []image.Rectangle{
		image.Rect(9, 3, 15, 9),
		image.Rect(2, 10, 8, 16),
		image.Rect(9, 10, 15, 16),
		image.Rect(16, 10, 22, 16),
		image.Rect(3, 18, 21, 21),
		image.Rect(6, 21, 18, 22),
	} {
		draw.Draw(icon, block, source, image.Point{}, draw.Src)
	}
	var data bytes.Buffer
	_ = png.Encode(&data, icon)
	return data.Bytes()
}
