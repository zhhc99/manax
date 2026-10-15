//go:build darwin

package config

import (
	"os"
	"os/user"
	"strconv"
)

// CurrentUser returns the user running this process.
func CurrentUser() UserInfo {
	if u, err := user.Current(); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		return UserInfo{Username: u.Username, UID: uid, GID: gid, Home: u.HomeDir}
	}
	return UserInfo{Username: os.Getenv("USER"), UID: os.Getuid(), GID: os.Getgid(), Home: os.Getenv("HOME")}
}

// IsElevated reports whether the process runs as root.
func IsElevated() bool { return os.Geteuid() == 0 }
