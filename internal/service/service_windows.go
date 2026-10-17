//go:build windows

package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/locale"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// InstallSystem installs the binary, resources and Windows service.
func InstallSystem(self, resources string) error {
	if err := config.EnsureDataDir(); err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(config.ServiceName)
	if err != nil {
		// Not installed yet; it is created once the binaries are in place.
		s = nil
	} else if err := stopService(s); err != nil {
		// A running service maps manax.exe, and Windows refuses to replace a
		// running image, so the old instance must be gone before installing.
		s.Close()
		return err
	} else {
		defer s.Close()
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

	if s == nil {
		s, err = m.CreateService(config.ServiceName, config.InstallBin, mgr.Config{
			DisplayName: "manax",
			StartType:   mgr.StartAutomatic,
		}, "daemon")
		if err != nil {
			return err
		}
		defer s.Close()
	}

	// A pre-existing service may have been registered without the daemon
	// argument; repair its image path so it actually runs the daemon.
	if cfg, cfgErr := s.Config(); cfgErr == nil {
		if want := syscall.EscapeArg(config.InstallBin) + " " + syscall.EscapeArg("daemon"); cfg.BinaryPathName != want {
			cfg.BinaryPathName = want
			if err := s.UpdateConfig(cfg); err != nil {
				return err
			}
		}
	}
	return s.Start()
}

// stopService asks the service to stop and waits for the process to exit.
func stopService(s *mgr.Service) error {
	st, err := s.Query()
	if err != nil {
		return err
	}
	if st.State == svc.Stopped {
		return nil
	}
	_, _ = s.Control(svc.Stop)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		st, err := s.Query()
		if err != nil {
			return err
		}
		if st.State == svc.Stopped {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf(locale.T("err.service_stop_timeout"), config.ServiceName)
}

// UninstallSystem stops and removes the Windows service, binary and resources.
func UninstallSystem() error {
	if m, err := mgr.Connect(); err == nil {
		if s, err := m.OpenService(config.ServiceName); err == nil {
			if err := stopService(s); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
			if err := s.Delete(); err == nil {
				fmt.Printf(locale.T("msg.removed_service")+"\n", config.ServiceName)
			}
			s.Close()
		}
		m.Disconnect()
	}
	removeDir(config.InstallDir)
	removeDir(config.DataDir)
	return nil
}

func installLogin(_ config.UserInfo) error {
	tr := `"` + config.InstallBin + `" reload`
	out, err := exec.Command("schtasks", "/Create", "/TN", config.LoginTaskName, "/SC", "ONLOGON",
		"/TR", tr, "/F").CombinedOutput()
	if err == nil {
		return nil
	}
	// schtasks explains the real reason (usually "Access is denied"); report it
	// instead of a bare "exit status 1".
	if msg := strings.TrimSpace(string(out)); msg != "" {
		return fmt.Errorf("schtasks: %s", msg)
	}
	return fmt.Errorf("schtasks: %w", err)
}

func removeLogin(_ config.UserInfo) (string, error) {
	return config.LoginTaskName, exec.Command("schtasks", "/Delete", "/TN", config.LoginTaskName, "/F").Run()
}

// localUsers returns one entry because the Windows login task is global.
func localUsers() []config.UserInfo { return []config.UserInfo{{}} }

// ShellExecuteEx flags used to start an elevated helper.
const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskFlagNoUI       = 0x00000400
	swHide                = 0
)

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         windows.Handle
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     windows.Handle
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    windows.Handle
	dwHotKey     uint32
	hIcon        windows.Handle
	hProcess     windows.Handle
}

// Elevate re-runs the command with administrator privileges via UAC and
// returns its combined output. The elevated process gets its own console, so
// its output is redirected to a file; otherwise every error message would be
// lost with the closed window.
func Elevate(exe string, args []string) (string, error) {
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("manax-elevate-%d.log", time.Now().UnixNano()))
	defer os.Remove(logPath)

	line := syscall.EscapeArg(exe)
	for _, arg := range args {
		line += " " + syscall.EscapeArg(arg)
	}
	line += " > " + syscall.EscapeArg(logPath) + " 2>&1"

	comspec := os.Getenv("COMSPEC")
	if comspec == "" {
		comspec = "cmd.exe"
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return "", err
	}
	file, err := windows.UTF16PtrFromString(comspec)
	if err != nil {
		return "", err
	}
	params, err := windows.UTF16PtrFromString(`/d /s /c "` + line + `"`)
	if err != nil {
		return "", err
	}
	dir, err := windows.UTF16PtrFromString(os.Getenv("SystemRoot"))
	if err != nil {
		return "", err
	}

	info := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskFlagNoUI,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		lpDirectory:  dir,
		nShow:        swHide,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	if r, _, callErr := proc.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return "", fmt.Errorf("%s", locale.T("err.elevate_cancelled"))
		}
		return "", fmt.Errorf(locale.T("err.elevate"), callErr)
	}
	defer windows.CloseHandle(info.hProcess)
	if _, err := windows.WaitForSingleObject(info.hProcess, windows.INFINITE); err != nil {
		return "", err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return "", err
	}
	data, _ := os.ReadFile(logPath)
	out := string(data)
	if code != 0 {
		return out, ErrElevatedFailed
	}
	return out, nil
}
