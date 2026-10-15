//go:build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/locale"
)

const daemonLabel = "com.manax.daemon"
const reloadLabel = "com.manax.reload"

// InstallSystem installs the binary, resources and launchd daemon.
func InstallSystem(self, resources string) error {
	if err := installKernel(resources); err != nil {
		return err
	}
	if err := InstallBinary(self); err != nil {
		return err
	}
	_ = os.MkdirAll(config.RuntimeDir, 0755)
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>daemon</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s/daemon.log</string>
  <key>StandardErrorPath</key><string>%s/daemon.log</string>
</dict>
</plist>
`, daemonLabel, config.InstallBin, config.RuntimeDir, config.RuntimeDir)
	if err := os.WriteFile(config.ServicePath, []byte(plist), 0644); err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", "-w", config.ServicePath).Run()
	return exec.Command("launchctl", "load", "-w", config.ServicePath).Run()
}

// UninstallSystem unloads and removes the launchd daemon and binary.
func UninstallSystem() error {
	_ = exec.Command("launchctl", "unload", "-w", config.ServicePath).Run()
	if os.Remove(config.ServicePath) == nil {
		fmt.Printf(locale.T("msg.removed_service")+"\n", config.ServiceName)
	}
	if os.Remove(config.InstallBin) == nil {
		fmt.Printf(locale.T("msg.removed_path")+"\n", config.InstallBin)
	}
	removeDir(config.InstallDir)
	return nil
}

func installLogin(u config.UserInfo) error {
	if u.Home == "" {
		return fmt.Errorf("unknown home directory for %s", u.Username)
	}
	dir := filepath.Join(u.Home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>reload</string></array>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`, reloadLabel, config.InstallBin)
	path := filepath.Join(dir, reloadLabel+".plist")
	if err := os.WriteFile(path, []byte(plist), 0644); err != nil {
		return err
	}
	if u.UID > 0 {
		_ = os.Chown(path, u.UID, u.GID)
		for p := dir; p != u.Home && strings.HasPrefix(p, u.Home); p = filepath.Dir(p) {
			_ = os.Chown(p, u.UID, u.GID)
		}
	}
	return nil
}

func removeLogin(u config.UserInfo) (string, error) {
	if u.Home == "" {
		return "", os.ErrNotExist
	}
	path := filepath.Join(u.Home, "Library", "LaunchAgents", reloadLabel+".plist")
	return path, os.Remove(path)
}

// Elevate re-runs the command with administrator privileges using osascript.
func Elevate(exe string, args []string) error {
	command := shellQuote(exe)
	for _, a := range args {
		command += " " + shellQuote(a)
	}
	script := "do shell script " + appleQuote(command) + " with administrator privileges"
	cmd := exec.Command("osascript", "-e", script)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func appleQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
