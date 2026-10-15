//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/locale"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// InstallSystem installs the binary, resources and Windows service.
func InstallSystem(self, resources string) error {
	if err := installKernel(resources); err != nil {
		return err
	}
	if err := InstallBinary(self); err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(config.ServiceName)
	if err != nil {
		s, err = m.CreateService(config.ServiceName, config.InstallBin, mgr.Config{
			DisplayName: "manax",
			StartType:   mgr.StartAutomatic,
		}, "daemon")
		if err != nil {
			return err
		}
	} else if cfg, cfgErr := s.Config(); cfgErr == nil {
		// A pre-existing service may have been registered without the daemon
		// argument; repair its image path so it actually runs the daemon.
		if want := syscall.EscapeArg(config.InstallBin) + " " + syscall.EscapeArg("daemon"); cfg.BinaryPathName != want {
			cfg.BinaryPathName = want
			if err := s.UpdateConfig(cfg); err != nil {
				return err
			}
		}
	}
	defer s.Close()
	return s.Start()
}

// UninstallSystem stops and removes the Windows service and binary.
func UninstallSystem() error {
	if m, err := mgr.Connect(); err == nil {
		if s, err := m.OpenService(config.ServiceName); err == nil {
			_, _ = s.Control(svc.Stop)
			_ = s.Delete()
			s.Close()
			fmt.Printf(locale.T("msg.removed_service")+"\n", config.ServiceName)
		}
		m.Disconnect()
	}
	removeDir(config.InstallDir)
	return nil
}

func installLogin(u config.UserInfo) error {
	tr := `"` + config.InstallBin + `" reload`
	return exec.Command("schtasks", "/Create", "/TN", "manax-reload", "/SC", "ONLOGON",
		"/TR", tr, "/F").Run()
}

func removeLogin(_ config.UserInfo) (string, error) {
	return config.LoginTaskName, exec.Command("schtasks", "/Delete", "/TN", config.LoginTaskName, "/F").Run()
}

// localUsers returns one entry because the Windows login task is global.
func localUsers() []config.UserInfo { return []config.UserInfo{{}} }

// Elevate re-runs the command with administrator privileges via UAC.
func Elevate(exe string, args []string) error {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = syscall.EscapeArg(arg)
	}
	argLine := strings.Join(quoted, " ")
	ps := "Start-Process -FilePath '" + strings.ReplaceAll(exe, "'", "''") + "' -ArgumentList '" +
		strings.ReplaceAll(argLine, "'", "''") + "' -Verb RunAs -Wait"
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
