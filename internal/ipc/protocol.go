package ipc

import "github.com/zhhc99/manax/internal/config"

// StatusRequest queries the running daemon.
type StatusRequest struct {
	User config.UserInfo `json:"user,omitempty"`
}

// StatusResponse describes the daemon's current view of the world.
type StatusResponse struct {
	KernelRunning bool   `json:"kernel_running"`
	KernelVersion string `json:"kernel_version,omitempty"`
	Mode          string `json:"mode,omitempty"`
	SystemProxy   bool   `json:"system_proxy"`
	ProxyAddr     string `json:"proxy_addr,omitempty"`
	TUN           bool   `json:"tun"`
	Group         string `json:"group,omitempty"`
	Node          string `json:"node,omitempty"`
	DelayMS       int    `json:"delay_ms,omitempty"`
	DelayOK       bool   `json:"delay_ok"`
	DelayKnown    bool   `json:"delay_known"`
	Err           string `json:"error,omitempty"`
}

// ApplyRequest asks the daemon to load a config with the given settings.
type ApplyRequest struct {
	ConfigPath  string          `json:"config_path"`
	ConfigName  string          `json:"config_name,omitempty"`
	Mode        string          `json:"mode"`
	SystemProxy bool            `json:"system_proxy"`
	TUN         bool            `json:"tun"`
	User        config.UserInfo `json:"user,omitempty"`
}

// ApplyResponse reports the result of an apply.
type ApplyResponse struct {
	Status StatusResponse `json:"status"`
	Err    string         `json:"error,omitempty"`
}

// ModeRequest sets the running mode.
type ModeRequest struct {
	Mode string `json:"mode"`
}

// SystemRequest toggles the system proxy for a user.
type SystemRequest struct {
	On   bool            `json:"on"`
	User config.UserInfo `json:"user,omitempty"`
}

// TunRequest toggles TUN.
type TunRequest struct {
	On bool `json:"on"`
}

// NodesRequest asks for proxy groups and nodes.
type NodesRequest struct{}

// NodeGroup is a proxy group and its members.
type NodeGroup struct {
	Name  string   `json:"name"`
	Type  string   `json:"type"`
	Now   string   `json:"now,omitempty"`
	Nodes []string `json:"nodes,omitempty"`
}

// NodesResponse lists all proxy groups and the daemon's cached latencies.
type NodesResponse struct {
	Groups []NodeGroup    `json:"groups,omitempty"`
	Delays map[string]int `json:"delays,omitempty"`
	Err    string         `json:"error,omitempty"`
}

// DelayRequest measures a single node.
type DelayRequest struct {
	Name    string `json:"name"`
	Timeout int    `json:"timeout,omitempty"`
	URL     string `json:"url,omitempty"`
}

// DelayResponse reports a measured latency.
type DelayResponse struct {
	Delay int    `json:"delay"`
	Err   string `json:"error,omitempty"`
}

// SelectRequest selects a node in a proxy group.
type SelectRequest struct {
	Group string `json:"group"`
	Node  string `json:"node"`
}

// SimpleResponse carries only an error.
type SimpleResponse struct {
	Err string `json:"error,omitempty"`
}

// StopRequest asks the daemon to stop the kernel and the system proxy.
type StopRequest struct{}

// Handler is implemented by the daemon.
type Handler interface {
	HandleStatus(StatusRequest) StatusResponse
	HandleApply(ApplyRequest) ApplyResponse
	HandleMode(ModeRequest) SimpleResponse
	HandleSystem(SystemRequest) SimpleResponse
	HandleTun(TunRequest) SimpleResponse
	HandleNodes(NodesRequest) NodesResponse
	HandleDelay(DelayRequest) DelayResponse
	HandleSelect(SelectRequest) SimpleResponse
	HandleStop(StopRequest) SimpleResponse
}
