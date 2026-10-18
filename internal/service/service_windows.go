//go:build windows

package service

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unicode/utf8"
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

// loginTaskXML is the logon task definition. The trigger and the principal are
// both bound to one user: `schtasks /SC ONLOGON` creates a trigger for *any*
// user, which Task Scheduler only lets administrators register ("Access is
// denied" for the unprivileged user phase). A user-scoped trigger needs no
// elevation. The settings also override the schtasks defaults that would skip
// the task on battery power and kill it after 72 hours.
const loginTaskXML = `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>%[1]s</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>%[1]s</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%[2]s</Command>
      <Arguments>reload</Arguments>
    </Exec>
  </Actions>
</Task>
`

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func installLogin(u config.UserInfo) error {
	if u.Username == "" {
		u = config.CurrentUser()
	}
	doc := fmt.Sprintf(loginTaskXML, xmlEscape(u.Username), xmlEscape(config.InstallBin))

	// schtasks /XML only reads UTF-16LE with a BOM.
	u16 := utf16.Encode([]rune(doc))
	buf := make([]byte, 0, 2+2*len(u16))
	buf = append(buf, 0xFF, 0xFE)
	for _, c := range u16 {
		buf = append(buf, byte(c), byte(c>>8))
	}
	f, err := os.CreateTemp("", "manax-task-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, werr := f.Write(buf)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return werr
	}

	out, err := exec.Command("schtasks", "/Create", "/TN", config.LoginTaskName,
		"/XML", f.Name(), "/F").CombinedOutput()
	if err == nil {
		return nil
	}
	// schtasks explains the real reason; report it instead of a bare "exit status 1".
	if msg := strings.TrimSpace(oemToUTF8(out)); msg != "" {
		return fmt.Errorf("schtasks: %s", msg)
	}
	return fmt.Errorf("schtasks: %w", err)
}

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procGetOEMCP            = kernel32.NewProc("GetOEMCP")
	procMultiByteToWideChar = kernel32.NewProc("MultiByteToWideChar")
)

// oemToUTF8 decodes output of a native console tool. When stdout is a pipe,
// tools such as schtasks write in the OEM code page (e.g. GBK on a Chinese
// system), which would print as garbage if treated as UTF-8.
func oemToUTF8(b []byte) string {
	if len(b) == 0 || utf8.Valid(b) {
		return string(b)
	}
	cp, _, _ := procGetOEMCP.Call()
	src := uintptr(unsafe.Pointer(&b[0]))
	n, _, _ := procMultiByteToWideChar.Call(cp, 0, src, uintptr(len(b)), 0, 0)
	if n == 0 {
		return string(b)
	}
	w := make([]uint16, n)
	procMultiByteToWideChar.Call(cp, 0, src, uintptr(len(b)), uintptr(unsafe.Pointer(&w[0])), n)
	return string(utf16.Decode(w))
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
