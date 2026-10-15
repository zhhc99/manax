//go:build windows

package systemproxy

import (
	"github.com/zhhc99/manax/internal/config"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const inetSettings = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// Enable writes the proxy settings into the target user's registry hive.
func Enable(u config.UserInfo, addr string) error {
	sid := u.SID
	if sid == "" {
		sid = config.CurrentUser().SID
	}
	k, _, err := registry.CreateKey(registry.USERS, sid+`\`+inetSettings, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyServer", addr); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyOverride", "localhost;127.*;<local>"); err != nil {
		return err
	}
	notify()
	return nil
}

// Disable turns the proxy off in the target user's registry hive.
func Disable(u config.UserInfo) error {
	sid := u.SID
	if sid == "" {
		sid = config.CurrentUser().SID
	}
	k, err := registry.OpenKey(registry.USERS, sid+`\`+inetSettings, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
		return err
	}
	notify()
	return nil
}

func notify() {
	dll := windows.NewLazySystemDLL("wininet.dll")
	proc := dll.NewProc("InternetSetOptionW")
	const (
		internetOptionRefresh         = 37
		internetOptionSettingsChanged = 39
	)
	_, _, _ = proc.Call(0, internetOptionSettingsChanged, 0, 0)
	_, _, _ = proc.Call(0, internetOptionRefresh, 0, 0)
}
