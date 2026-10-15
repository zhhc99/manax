//go:build windows

package ipc

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/zhhc99/manax/internal/config"
	"golang.org/x/sys/windows"
)

// ErrAlreadyRunning is returned by listen when another instance is active.
var ErrAlreadyRunning = errors.New("service is already running")

type pipeAddr string

func (pipeAddr) Network() string  { return "pipe" }
func (a pipeAddr) String() string { return string(a) }

type pipeConn struct {
	handle windows.Handle
}

func (c *pipeConn) Read(b []byte) (int, error) {
	var done uint32
	err := windows.ReadFile(c.handle, b, &done, nil)
	return int(done), err
}

func (c *pipeConn) Write(b []byte) (int, error) {
	var done uint32
	err := windows.WriteFile(c.handle, b, &done, nil)
	return int(done), err
}

func (c *pipeConn) Close() error                       { return windows.CloseHandle(c.handle) }
func (c *pipeConn) LocalAddr() net.Addr                { return pipeAddr(config.PipeName) }
func (c *pipeConn) RemoteAddr() net.Addr               { return pipeAddr(config.PipeName) }
func (c *pipeConn) SetDeadline(_ time.Time) error      { return nil }
func (c *pipeConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *pipeConn) SetWriteDeadline(_ time.Time) error { return nil }

type pipeListener struct {
	name string
	mu   sync.Mutex
	h    windows.Handle
	done chan struct{}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	for {
		select {
		case <-l.done:
			return nil, net.ErrClosed
		default:
		}

		l.mu.Lock()
		h := l.h
		l.mu.Unlock()
		if h == windows.InvalidHandle {
			return nil, net.ErrClosed
		}

		err := windows.ConnectNamedPipe(h, nil)
		if err != nil {
			switch err {
			case windows.ERROR_PIPE_CONNECTED:
			case windows.ERROR_NO_DATA:
				_ = windows.DisconnectNamedPipe(h)
				continue
			default:
				select {
				case <-l.done:
					return nil, net.ErrClosed
				default:
					return nil, &net.OpError{Op: "accept", Net: "pipe", Addr: pipeAddr(l.name), Err: err}
				}
			}
		}

		next, err2 := createNamedPipeHandle(l.name, false)
		l.mu.Lock()
		if err2 == nil {
			l.h = next
		} else {
			l.h = windows.InvalidHandle
		}
		l.mu.Unlock()
		return &pipeConn{handle: h}, nil
	}
}

func (l *pipeListener) Close() error {
	select {
	case <-l.done:
		return nil
	default:
		close(l.done)
	}
	l.mu.Lock()
	h := l.h
	l.h = windows.InvalidHandle
	l.mu.Unlock()
	if h != windows.InvalidHandle {
		_ = windows.CloseHandle(h)
	}
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr(l.name) }

func createNamedPipeHandle(name string, firstInstance bool) (windows.Handle, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	openMode := uint32(windows.PIPE_ACCESS_DUPLEX)
	if firstInstance {
		openMode |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	h, err := windows.CreateNamedPipe(
		namePtr, openMode,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		windows.PIPE_UNLIMITED_INSTANCES,
		65536, 65536, 0, nil,
	)
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("CreateNamedPipe: %w", err)
	}
	return h, nil
}

func listen() (net.Listener, error) {
	h, err := createNamedPipeHandle(config.PipeName, true)
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}
	return &pipeListener{name: config.PipeName, h: h, done: make(chan struct{})}, nil
}

func dial() (net.Conn, error) {
	namePtr, err := windows.UTF16PtrFromString(config.PipeName)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(
		namePtr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0,
	)
	if err != nil {
		return nil, &net.OpError{Op: "dial", Net: "pipe", Addr: pipeAddr(config.PipeName), Err: err}
	}
	return &pipeConn{handle: h}, nil
}

func isServicePresent() bool {
	conn, err := dial()
	if err == nil {
		conn.Close()
		return true
	}
	return false
}
