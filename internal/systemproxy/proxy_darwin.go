//go:build darwin

package systemproxy

import (
	"os/exec"
	"strings"

	"github.com/zhhc99/manax/internal/config"
)

// Enable points the system proxy at addr for all network services.
func Enable(_ config.UserInfo, addr string) error {
	host, port, err := splitHostPort(addr)
	if err != nil {
		return err
	}
	for _, svc := range services() {
		for _, mode := range []string{"web", "secureweb", "socksfirewall"} {
			if _, err := command("networksetup", "-set"+mode+"proxy", svc, host, port); err != nil {
				return err
			}
			if _, err := command("networksetup", "-set"+mode+"proxystate", svc, "on"); err != nil {
				return err
			}
		}
	}
	return nil
}

// Disable clears the system proxy for all network services.
func Disable(_ config.UserInfo) error {
	for _, svc := range services() {
		for _, mode := range []string{"web", "secureweb", "socksfirewall"} {
			if _, err := command("networksetup", "-set"+mode+"proxystate", svc, "off"); err != nil {
				return err
			}
		}
	}
	return nil
}

func services() []string {
	out, err := command("networksetup", "-listallnetworkservices")
	if err != nil {
		return nil
	}
	var svcs []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		svcs = append(svcs, line)
	}
	return svcs
}

func splitHostPort(addr string) (string, string, error) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return addr, "", nil
	}
	return addr[:i], addr[i+1:], nil
}

func command(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}
