# Linux desktop tray

Docktop's tray shows whether Docker is available, the current Docker context, and the number of running containers. It refreshes every 20 seconds. Tray mode requires a Linux desktop session with a StatusNotifierItem host, such as KDE Plasma or GNOME with [AppIndicator support](https://github.com/ubuntu/gnome-shell-extension-appindicator).

## Start and use the tray

Run `docktop tray` in your desktop session. Use `docktop tray --context work` to select a Docker context for the tray and the dashboards it opens. Docktop runs one tray per D-Bus session.

| Menu item | Behavior |
| --- | --- |
| **Open Docktop** | Opens the dashboard with the tray's Docker context. |
| **Refresh** | Refreshes Docker status immediately. |
| **Quit** | Closes the tray and exits open Docktop dashboards on the same D-Bus session. |

Closing a dashboard window keeps the tray running. The tray's **Quit** action asks each dashboard to exit normally, leaving Docker workloads running. Dashboards in other D-Bus sessions exit using their own `q` shortcut.

The tray opens a terminal by default. Pass `--terminal /path/to/terminal` to choose one. If you have a graphical wrapper, pass `--launcher /path/to/wrapper`. Docktop calls the wrapper directly with `--context <name>`, so the wrapper can open its own window.

## Fedora GNOME app launcher

The optional native launcher uses a GTK window with an embedded terminal. Opening Docktop from the app grid or dock also starts the tray. The tray's **Open Docktop** action opens the same GTK window. After choosing **Quit**, launch Docktop again to bring back the window and tray.

Install the Fedora dependencies and enable the AppIndicator extension:

```bash
sudo dnf install python3-gobject gtk4 libadwaita vte291-gtk4 gnome-shell-extension-appindicator
gnome-extensions enable appindicatorsupport@rgcjonas.gmail.com
```

If GNOME has yet to load the newly installed extension, sign out and back in before enabling it.

From the extracted release archive or source checkout, run the commands below. For a source checkout, first build `./docktop` with `go build -o docktop ./cmd/docktop`.

```bash
install -Dm755 ./docktop "$HOME/.local/bin/docktop"
install -Dm755 desktop/gnome/docktop-gui "$HOME/.local/bin/docktop-gui"
install -Dm644 desktop/gnome/docktop.svg \
  "${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor/scalable/apps/docktop.svg"
install -Dm644 desktop/gnome/docktop-tray.service \
  "${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/docktop-tray.service"

applications="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
mkdir -p "$applications"
sed "s|@BINDIR@|$HOME/.local/bin|g" desktop/gnome/docktop.desktop > "$applications/docktop.desktop"
systemctl --user daemon-reload
```

Open **Docktop** from the app grid. The launcher uses a systemd user service so repeat launches share the same tray. **Start Docktop Tray** in the shortcut's right-click menu starts just the tray. The service starts on demand when you use either launcher action.

### Check the GNOME tray

If the icon is missing, confirm the AppIndicator extension is enabled and check the service state:

```bash
gnome-extensions info appindicatorsupport@rgcjonas.gmail.com
systemctl --user status docktop-tray.service
journalctl --user -u docktop-tray.service -n 30
```

If a dashboard was open during an upgrade, reopen it to load the newer executable and its tray integration.
