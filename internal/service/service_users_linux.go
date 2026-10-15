//go:build linux

package service

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/zhhc99/manax/internal/config"
)

// localUsers lists local accounts that may own a login task.
func localUsers() []config.UserInfo {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil
	}
	defer f.Close()

	var users []config.UserInfo
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Split(sc.Text(), ":")
		if len(parts) < 7 {
			continue
		}
		uid, err := strconv.Atoi(parts[2])
		if err != nil || uid == 0 {
			continue
		}
		home := parts[5]
		if !strings.HasPrefix(home, "/") || home == "/nonexistent" || home == "/var/empty" {
			continue
		}
		users = append(users, config.UserInfo{Username: parts[0], UID: uid, Home: home})
	}
	return users
}
