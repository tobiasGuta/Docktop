//go:build !linux

package tray

import (
	"context"
	"errors"

	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

func Run(context.Context, dockercli.Client, string, string) error {
	return errors.New("tray mode is available on Linux desktop sessions")
}

func WatchQuit(context.Context, func()) {}
