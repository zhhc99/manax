//go:build linux

package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/locale"
)

// InstallSystem installs the binary, resources and systemd service.
func InstallSystem(self, resources string) error {
	if err := config.EnsureDataDir(); err != nil {
		return err
	}
	if err := installKernel(resources); err != nil {
		return err
	}
	if err := installGeoData(resources); err != nil {
		return err
	}
	if err := InstallBinary(self); err != nil {
		return err
	}
	unit := fmt.Sprintf(`[Unit]
Description=manax service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s daemon
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
`, config.InstallBin)
	if err := os.WriteFile(config.ServicePath, []byte(unit), 0644); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = exec.Command("systemctl", "enable", config.ServiceName).Run()
	return exec.Command("systemctl", "restart", config.ServiceName).Run()
}

// UninstallSystem stops and removes the systemd service and binary.
func UninstallSystem() error {
	_ = exec.Command("systemctl", "stop", config.ServiceName).Run()
	_ = exec.Command("systemctl", "disable", config.ServiceName).Run()
	if os.Remove(config.ServicePath) == nil {
		fmt.Printf(locale.T("msg.removed_service")+"\n", config.ServiceName)
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	if os.Remove(config.InstallBin) == nil {
		fmt.Printf(locale.T("msg.removed_path")+"\n", config.InstallBin)
	}
	removeDir(config.InstallDir)
	removeDir(config.DataDir)
	removeDir(config.RuntimeDir)
	return nil
}

func installLogin(u config.UserInfo) error {
	home := u.Home
	if home == "" {
		return fmt.Errorf("unknown home directory for %s", u.Username)
	}
	dir := filepath.Join(home, ".config", "systemd", "user")
	wants := filepath.Join(dir, "default.target.wants")
	if err := os.MkdirAll(wants, 0700); err != nil {
		return err
	}
	unit := filepath.Join(dir, "manax-reload.service")
	content := fmt.Sprintf(`[Unit]
Description=Apply manax config at login

[Service]
Type=oneshot
ExecStart=%s reload
`, config.InstallBin)
	if err := os.WriteFile(unit, []byte(content), 0600); err != nil {
		return err
	}
	link := filepath.Join(wants, "manax-reload.service")
	_ = os.Remove(link)
	if err := os.Symlink("../manax-reload.service", link); err != nil {
		return err
	}
	chownTree(dir, u)
	chownParents(home, wants, u)
	return nil
}

func removeLogin(u config.UserInfo) (string, error) {
	if u.Home == "" {
		return "", os.ErrNotExist
	}
	dir := filepath.Join(u.Home, ".config", "systemd", "user")
	unit := filepath.Join(dir, "manax-reload.service")
	paths := []string{
		filepath.Join(dir, "default.target.wants", "manax-reload.service"),
		unit,
	}
	removed := false
	for _, p := range paths {
		if err := os.Remove(p); err == nil {
			removed = true
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	if !removed {
		return "", os.ErrNotExist
	}
	return unit, nil
}

func chownTree(root string, u config.UserInfo) {
	if u.UID <= 0 {
		return
	}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		_ = os.Chown(path, u.UID, u.GID)
		return nil
	})
}

// chownParents chowns the directories between home (exclusive) and path so a
// privileged install does not leave root-owned ~/.config components behind.
func chownParents(home, path string, u config.UserInfo) {
	if u.UID <= 0 {
		return
	}
	for p := path; p != home && strings.HasPrefix(p, home); p = filepath.Dir(p) {
		_ = os.Chown(p, u.UID, u.GID)
	}
}

// Elevate re-runs the command with administrator privileges, preferring
// pkexec and falling back to an internal sudo subprocess. The helper shares
// this terminal, so its error output needs no relaying.
func Elevate(exe string, args []string) (string, error) {
	argv := append([]string{exe}, args...)

	if pkexec, err := exec.LookPath("pkexec"); err == nil {
		err = runPrivileged(pkexec, argv)
		if err == nil {
			return "", nil
		}
		// pkexec could not obtain authorization (no polkit agent etc.) -
		// fall back to sudo. Other exit codes come from the helper itself.
		var ee *exec.ExitError
		if !errors.As(err, &ee) || (ee.ExitCode() != 126 && ee.ExitCode() != 127) {
			return "", ErrElevatedFailed
		}
	}

	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return "", ErrNeedSudo
	}
	if err := runPrivileged(sudo, argv); err != nil {
		return "", ErrElevatedFailed
	}
	return "", nil
}

func runPrivileged(bin string, argv []string) error {
	cmd := exec.Command(bin, argv...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
