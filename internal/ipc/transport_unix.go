//go:build linux || darwin

package ipc

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"

	"github.com/zhhc99/manax/internal/config"
)

// ErrAlreadyRunning is returned by listen when another instance is active.
var ErrAlreadyRunning = errors.New("service is already running")

func listen() (net.Listener, error) {
	dir := filepath.Dir(config.SocketPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	if conn, err := net.Dial("unix", config.SocketPath); err == nil {
		conn.Close()
		return nil, ErrAlreadyRunning
	}
	_ = os.Remove(config.SocketPath)
	l, err := net.Listen("unix", config.SocketPath)
	if err != nil {
		return nil, err
	}
	// Allow any local user to control the daemon.
	if err := os.Chmod(config.SocketPath, 0666); err != nil {
		_ = l.Close()
		return nil, err
	}
	return l, nil
}

func dial() (net.Conn, error) {
	return net.Dial("unix", config.SocketPath)
}

func dialContext(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", config.SocketPath)
}

func isServicePresent() bool {
	_, err := os.Stat(config.SocketPath)
	return err == nil
}
