//go:build windows

package ipc

import (
	"context"
	"errors"
	"net"

	"github.com/Microsoft/go-winio"
	"github.com/zhhc99/manax/internal/config"
	"golang.org/x/sys/windows"
)

// ErrAlreadyRunning is returned by listen when another instance is active.
var ErrAlreadyRunning = errors.New("service is already running")

// pipeSecurity grants full access to SYSTEM and administrators and read/write
// access to every authenticated user, mirroring the 0666 unix socket. Without
// an explicit descriptor the pipe would inherit the SYSTEM token's default
// DACL, which no ordinary user can open.
const pipeSecurity = "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FR;;;AU)(A;;FW;;;AU)"

func listen() (net.Listener, error) {
	l, err := winio.ListenPipe(config.PipeName, &winio.PipeConfig{SecurityDescriptor: pipeSecurity})
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}
	return l, nil
}

func dial() (net.Conn, error) {
	return winio.DialPipe(config.PipeName, nil)
}

func dialContext(ctx context.Context) (net.Conn, error) {
	return winio.DialPipeContext(ctx, config.PipeName)
}

func isServicePresent() bool {
	conn, err := dial()
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
