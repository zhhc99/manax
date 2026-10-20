package core

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/tun"
	"gopkg.in/yaml.v3"
)

// RuntimeFileName is the generated config in the kernel work directory.
const RuntimeFileName = ".manax.runtime.yaml"

// NodeGroupName is the internal selector containing all nodes.
const NodeGroupName = ".manax"

// EnsureDirectConfig writes the built-in configuration used when no user
// config can be loaded. The GLOBAL group contains only DIRECT so global mode
// stays direct too.
func EnsureDirectConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(`mixed-port: 7890
allow-lan: false
bind-address: 127.0.0.1
proxy-groups:
  - name: GLOBAL
    type: select
    proxies:
      - DIRECT
rules:
  - MATCH,DIRECT
`), 0600)
}

// BuildRuntimeConfig parses srcPath, injects manax-controlled settings and
// writes a runtime config into workDir, the daemon-owned kernel home. The
// user's own config stays where the user put it; the generated copy, which the
// privileged kernel reads, never sits in a user-writable directory.
func BuildRuntimeConfig(srcPath, mode string, tunOn bool, workDir string) (runtimePath, proxyAddr string, err error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", "", err
	}
	doc := map[string]any{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		// Not a Clash config: the file is a subscription (base64 or a plain
		// URI list). clash.meta only understands those through a proxy-provider,
		// so wrap the file instead of parsing it as the top-level config.
		subPath := filepath.Join(workDir, ".manax.subscription")
		if err := config.WriteFile(subPath, data); err != nil {
			return "", "", err
		}
		doc = subscriptionDoc(subPath)
	}
	if doc == nil {
		doc = map[string]any{}
	}

	doc["external-controller"] = fmt.Sprintf("%s:%d", APIHost, APIPort)
	doc["secret"] = ""
	if mode != "" {
		doc["mode"] = mode
	}
	tun.Apply(doc, tunOn)
	groups, _ := doc["proxy-groups"].([]any)
	var userGroups []any
	for _, item := range groups {
		g, _ := item.(map[string]any)
		if g["name"] == NodeGroupName {
			continue
		}
		if g["type"] == "select" {
			proxies, _ := g["proxies"].([]any)
			hasNodes := g["name"] == "GLOBAL" || g["use"] != nil || g["include-all"] == true ||
				g["include-all-proxies"] == true || g["include-all-providers"] == true ||
				slices.ContainsFunc(proxies, func(p any) bool {
					name, _ := p.(string)
					return !IsBuiltin(name)
				})
			if hasNodes && !slices.ContainsFunc(proxies, func(p any) bool { return p == NodeGroupName }) {
				g["proxies"] = append(proxies, NodeGroupName)
			}
		}
		userGroups = append(userGroups, item)
	}
	doc["proxy-groups"] = append(userGroups, map[string]any{"name": NodeGroupName, "type": "select", "include-all": true})
	profile, _ := doc["profile"].(map[string]any)
	if profile == nil {
		profile = map[string]any{}
	}
	profile["store-selected"] = true
	doc["profile"] = profile

	port := proxyPort(doc)
	if port == 0 {
		port = 7890
		doc["mixed-port"] = 7890
	}
	proxyAddr = fmt.Sprintf("%s:%d", APIHost, port)

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", "", err
	}
	runtimePath = filepath.Join(workDir, RuntimeFileName)
	if err := config.WriteFile(runtimePath, out); err != nil {
		return "", "", err
	}
	return runtimePath, proxyAddr, nil
}

// subscriptionDoc wraps a subscription file in a minimal Clash config so
// clash.meta can load it. proxy-providers are the only place clash.meta converts
// base64 or URI-list subscriptions into proxies.
func subscriptionDoc(srcPath string) map[string]any {
	const provider = "manax-sub"
	return map[string]any{
		"proxy-providers": map[string]any{
			provider: map[string]any{
				"type": "file",
				"path": srcPath,
			},
		},
		"proxy-groups": []any{
			map[string]any{"name": "PROXY", "type": "select", "use": []any{provider}},
		},
		"rules": []any{"MATCH,PROXY"},
	}
}

func proxyPort(doc map[string]any) int {
	for _, key := range []string{"mixed-port", "port", "socks-port"} {
		if v, ok := toInt(doc[key]); ok && v > 0 {
			return v
		}
	}
	return 0
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i, true
		}
	}
	return 0, false
}

// IsBuiltin reports whether name is a built-in kernel policy.
func IsBuiltin(name string) bool {
	switch name {
	case "DIRECT", "REJECT", "GLOBAL", "COMPATIBLE", "PASS", "PASS-RULE":
		return true
	}
	return false
}
