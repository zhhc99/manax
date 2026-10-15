//go:build windows

package config

import (
	"os"
	"os/user"

	"golang.org/x/sys/windows"
)

// CurrentUser returns the user running this process.
func CurrentUser() UserInfo {
	info := UserInfo{Username: os.Getenv("USERNAME"), Home: os.Getenv("USERPROFILE")}
	if u, err := user.Current(); err == nil {
		info.Username = u.Username
		info.Home = u.HomeDir
	}
	if tok, err := windows.OpenCurrentProcessToken(); err == nil {
		defer tok.Close()
		if tu, err := tok.GetTokenUser(); err == nil && tu.User.Sid != nil {
			info.SID = tu.User.Sid.String()
		}
	}
	return info
}

// IsElevated reports whether the process has an elevated token.
func IsElevated() bool {
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer tok.Close()
	return tok.IsElevated()
}
