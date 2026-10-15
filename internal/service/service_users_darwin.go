//go:build darwin

package service

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/zhhc99/manax/internal/config"
)

// localUsers lists local accounts that may own a login task.
func localUsers() []config.UserInfo {
	out, err := exec.Command("dscl", ".", "-list", "/Users").Output()
	if err != nil {
		return nil
	}
	var users []config.UserInfo
	for _, name := range strings.Fields(string(out)) {
		if name == "" || strings.HasPrefix(name, "_") {
			continue
		}
		uid, _ := strconv.Atoi(readUserAttr(name, "UniqueID"))
		home := readUserAttr(name, "NFSHomeDirectory")
		if uid == 0 || !strings.HasPrefix(home, "/") || home == "/var/empty" {
			continue
		}
		users = append(users, config.UserInfo{Username: name, UID: uid, Home: home})
	}
	return users
}

// readUserAttr returns the string value of a dscl user attribute.
func readUserAttr(name, attr string) string {
	out, err := exec.Command("dscl", ".", "-read", "/Users/"+name, attr).Output()
	if err != nil {
		return ""
	}
	// Output looks like "UniqueID: 501" or "NFSHomeDirectory: /Users/x".
	_, value, ok := strings.Cut(strings.TrimSpace(string(out)), ":")
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}
