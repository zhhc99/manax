// Package tun controls the TUN field of a Clash-compatible config document.
package tun

// Apply sets the tun.enable field of a parsed config document.
func Apply(doc map[string]any, on bool) {
	t, _ := doc["tun"].(map[string]any)
	if t == nil {
		t = map[string]any{}
	}
	t["enable"] = on
	if _, ok := t["stack"]; !ok {
		t["stack"] = "gvisor"
	}
	doc["tun"] = t
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
