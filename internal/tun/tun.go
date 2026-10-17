// Package tun controls the TUN-related fields of a Clash-compatible config document.
package tun

// Apply sets the tun fields of a parsed config document. When on, it also makes
// sure the kernel's DNS module is enabled so dns-hijack has a resolver to hand
// queries to.
func Apply(doc map[string]any, on bool) {
	t, _ := doc["tun"].(map[string]any)
	if t == nil {
		t = map[string]any{}
	}
	t["enable"] = on
	if on {
		setDefault(t, "stack", "gvisor")
		t["auto-route"] = true
		t["auto-detect-interface"] = true
		t["strict-route"] = false
		t["dns-hijack"] = []any{"any:53"}
		applyDNS(doc)
	}
	doc["tun"] = t
}

// applyDNS enables the fake-ip DNS module. Other enhanced-mode values (for
// example redir-host) are the subscription's choice and left untouched.
func applyDNS(doc map[string]any) {
	d, _ := doc["dns"].(map[string]any)
	if d == nil {
		d = map[string]any{}
	}
	mode, _ := d["enhanced-mode"].(string)
	if mode == "" {
		mode = "fake-ip"
	}
	if mode != "fake-ip" {
		return
	}
	d["enable"] = true

	// nameserver override
	d["default-nameserver"] = []any{
		// "system",
		"8.8.8.8",
		// "223.6.6.6",
	}
	d["nameserver"] = []any{
		"https://doh.pub/dns-query",
		"https://dns.alidns.com/dns-query",
	}
	d["proxy-server-nameserver"] = []any{
		"https://doh.pub/dns-query",
		"https://dns.alidns.com/dns-query",
		"tls://223.5.5.5",
	}

	setDefault(d, "enhanced-mode", "fake-ip")
	setDefault(d, "fake-ip-range", "198.18.0.1/16")
	doc["dns"] = d
}

func setDefault(m map[string]any, key string, val any) {
	if _, ok := m[key]; !ok {
		m[key] = val
	}
}

// Enabled reads the tun.enable field of a parsed config document.
func Enabled(doc map[string]any) bool {
	t, _ := doc["tun"].(map[string]any)
	if t == nil {
		return false
	}
	v, _ := t["enable"].(bool)
	return v
}
