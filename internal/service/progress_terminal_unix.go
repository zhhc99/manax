//go:build !windows

package service

import (
	"os"

	"golang.org/x/sys/unix"
)

func progressColumns() int {
	winsize, err := unix.IoctlGetWinsize(int(os.Stderr.Fd()), unix.TIOCGWINSZ)
	if err != nil || winsize.Col == 0 {
		return 80
	}
	return int(winsize.Col)
}
