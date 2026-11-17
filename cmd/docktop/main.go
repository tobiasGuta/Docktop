package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
	"github.com/tobiasGuta/Docktop/internal/tray"
	"github.com/tobiasGuta/Docktop/internal/tui"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "docktop:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	trayMode := len(args) > 0 && args[0] == "tray"
	name := "docktop"
	if trayMode {
		name += " tray"
		args = args[1:]
	}
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	showVersion := flags.Bool("version", false, "Print the Docktop version")
	dockerContext := flags.String("context", "", "Docker context (defaults to the active context)")
	var terminal, launcher string
	if trayMode {
		flags.StringVar(&terminal, "terminal", "", "Terminal executable for opening the dashboard")
		flags.StringVar(&launcher, "launcher", "", "Dashboard executable that accepts --context")
	}
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s [options]\n", name)
		if !trayMode {
			fmt.Fprintln(flags.Output(), "       docktop tray [options]")
		}
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if *showVersion {
		fmt.Println("Docktop", version)
		return nil
	}
	client := dockercli.New()
	client.Context = *dockerContext
	if trayMode {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return tray.Run(ctx, client, terminal, launcher)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	program := tea.NewProgram(tui.NewWithClient(client), tea.WithAltScreen())
	tray.WatchQuit(ctx, program.Quit)
	_, err := program.Run()
	return err
}
