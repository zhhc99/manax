//go:build linux

// Package systemproxy applies the OS proxy settings on the target platform.
package systemproxy

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/locale"
)

// Enable points the system proxy at addr for the given user.
func Enable(u config.UserInfo, addr string) error {
	host, port := splitAddr(addr)
	desktop := detect(u)
	switch {
	case strings.Contains(desktop, "GNOME"), strings.Contains(desktop, "UNITY"), strings.Contains(desktop, "CINNAMON"):
		return gnomeSet(u, "manual", host, port)
	case strings.Contains(desktop, "KDE"):
		return kdeSet(u, true, host, port)
	default:
		return fmt.Errorf("%s", locale.T("err.system_unsupported"))
	}
}

// Disable clears the system proxy for the given user.
func Disable(u config.UserInfo) error {
	desktop := detect(u)
	switch {
	case strings.Contains(desktop, "GNOME"), strings.Contains(desktop, "UNITY"), strings.Contains(desktop, "CINNAMON"):
		return gnomeSet(u, "none", "", "")
	case strings.Contains(desktop, "KDE"):
		return kdeSet(u, false, "", "")
	default:
		return fmt.Errorf("%s", locale.T("err.system_unsupported"))
	}
}

func splitAddr(addr string) (string, string) {
	host, port, err := splitHostPort(addr)
	if err != nil {
		return addr, "0"
	}
	return host, port
}

func splitHostPort(addr string) (string, string, error) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return addr, "", fmt.Errorf("invalid addr %q", addr)
	}
	return addr[:i], addr[i+1:], nil
}

func detect(u config.UserInfo) string {
	if d := desktopFromSystemd(u); d != "" {
		return strings.ToUpper(d)
	}
	if _, err := runAsUser(u, "gsettings", "get", "org.gnome.system.proxy", "mode"); err == nil {
		return "GNOME"
	}
	if _, err := exec.LookPath("kwriteconfig6"); err == nil {
		return "KDE"
	}
	if _, err := exec.LookPath("kwriteconfig5"); err == nil {
		return "KDE"
	}
	return ""
}

func desktopFromSystemd(u config.UserInfo) string {
	out, err := runAsUser(u, "systemctl", "--user", "show-environment")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "XDG_CURRENT_DESKTOP="); ok {
			return v
		}
	}
	return ""
}

func gnomeSet(u config.UserInfo, mode, host, port string) error {
	steps := [][]string{{"gsettings", "set", "org.gnome.system.proxy", "mode", mode}}
	if mode != "none" {
		steps = append(steps,
			[]string{"gsettings", "set", "org.gnome.system.proxy.http", "host", host},
			[]string{"gsettings", "set", "org.gnome.system.proxy.http", "port", port},
			[]string{"gsettings", "set", "org.gnome.system.proxy.https", "host", host},
			[]string{"gsettings", "set", "org.gnome.system.proxy.https", "port", port},
			[]string{"gsettings", "set", "org.gnome.system.proxy.socks", "host", host},
			[]string{"gsettings", "set", "org.gnome.system.proxy.socks", "port", port},
			[]string{"gsettings", "set", "org.gnome.system.proxy", "ignore-hosts", "['localhost','127.0.0.0/8','::1']"},
		)
	}
	for _, s := range steps {
		if _, err := runAsUser(u, s[0], s[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func kdeSet(u config.UserInfo, on bool, host, port string) error {
	bin := "kwriteconfig6"
	if _, err := exec.LookPath(bin); err != nil {
		bin = "kwriteconfig5"
	}
	proxyType := "0"
	if on {
		proxyType = "1"
	}
	steps := [][]string{
		{bin, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", proxyType},
	}
	if on {
		steps = append(steps,
			[]string{bin, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpProxy", "http://" + host + ":" + port},
			[]string{bin, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpsProxy", "http://" + host + ":" + port},
			[]string{bin, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "socksProxy", "socks://" + host + ":" + port},
			[]string{bin, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "NoProxyFor", "localhost,127.0.0.1,::1"},
		)
	}
	for _, s := range steps {
		if _, err := runAsUser(u, s[0], s[1:]...); err != nil {
			return err
		}
	}
	// Ask running KIO processes to reload.
	_, _ = runAsUser(u, "dbus-send", "--type=signal", "/KIO/Scheduler",
		"org.kde.KIO.Scheduler.reparseSlaveConfiguration", "string:")
	return nil
}

func runAsUser(u config.UserInfo, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = userEnv(u)
	// Only switch credentials when running privileged; a normal user already
	// runs as the target user.
	if os.Geteuid() == 0 && u.UID > 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(u.UID), Gid: uint32(u.GID)}
	}
	return cmd.CombinedOutput()
}

func userEnv(u config.UserInfo) []string {
	home := u.Home
	if home == "" {
		home = "/tmp"
	}
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin:/usr/local/sbin:/usr/sbin:/sbin",
		"HOME=" + home,
		"USER=" + u.Username,
	}
	if u.UID > 0 {
		runtime := "/run/user/" + strconv.Itoa(u.UID)
		env = append(env,
			"XDG_RUNTIME_DIR="+runtime,
			"DBUS_SESSION_BUS_ADDRESS=unix:path="+runtime+"/bus",
		)
	}
	// Preserve a few environment overrides used in tests.
	for _, key := range []string{"XDG_CURRENT_DESKTOP"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}
	return env
}
