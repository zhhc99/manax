// Package daemon runs the system-wise service: it supervises the kernel
// process and performs privileged network operations on behalf of users.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/core"
	"github.com/zhhc99/manax/internal/ipc"
	"github.com/zhhc99/manax/internal/locale"
	"github.com/zhhc99/manax/internal/systemproxy"
)

type Daemon struct {
	applyMu sync.Mutex
	mu      sync.Mutex

	kernel *core.Kernel
	api    *core.Client

	configPath string
	configName string
	workDir    string
	mode       string
	tunOn      bool

	systemProxy bool
	proxyAddr   string
	proxyUser   config.UserInfo

	delays map[string]int
}

// New builds a daemon with no user config loaded.
func New() *Daemon {
	return &Daemon{
		kernel: core.NewKernel(config.KernelPath, filepath.Join(config.RuntimeDir, "kernel.log")),
		api:    core.NewClient(core.APIBase, ""),
		mode:   "rule",
		delays: map[string]int{},
	}
}

// Serve starts the IPC server and blocks until ctx is cancelled.
func (d *Daemon) Serve(ctx context.Context) error {
	_ = os.MkdirAll(config.RuntimeDir, 0755)
	srv, err := ipc.NewServer(d)
	if err != nil {
		if errors.Is(err, ipc.ErrAlreadyRunning) {
			return fmt.Errorf("%s", locale.T("err.service_not_responding"))
		}
		return err
	}
	go srv.Serve()
	<-ctx.Done()
	d.Shutdown()
	srv.Close()
	return nil
}

// Shutdown stops the kernel and clears the system proxy.
func (d *Daemon) Shutdown() {
	d.kernel.Stop()
	d.disableProxy()
}

func (d *Daemon) HandleStatus(_ ipc.StatusRequest) ipc.StatusResponse {
	return d.status()
}

func (d *Daemon) status() ipc.StatusResponse {
	d.mu.Lock()
	resp := ipc.StatusResponse{
		Mode:        d.mode,
		SystemProxy: d.systemProxy,
		ProxyAddr:   d.proxyAddr,
		TUN:         d.tunOn,
	}
	delays := make(map[string]int, len(d.delays))
	for k, v := range d.delays {
		delays[k] = v
	}
	d.mu.Unlock()

	if d.kernel.Running() {
		resp.KernelRunning = true
		resp.KernelVersion = core.VersionLine(config.KernelPath)
		if group, node, ok := d.currentNode(); ok {
			resp.Group, resp.Node = group, node
			if ms, known := delays[node]; known {
				resp.DelayKnown = true
				if ms >= 0 {
					resp.DelayMS = ms
					resp.DelayOK = true
				}
			}
		}
	}
	return resp
}

func (d *Daemon) currentNode() (string, string, bool) {
	proxies, err := d.api.Proxies()
	if err != nil {
		return "", "", false
	}
	var names []string
	for name, p := range proxies {
		if p.Type == "Selector" && name != "GLOBAL" && p.Now != "" && len(p.All) > 0 {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", "", false
	}
	sort.Strings(names)
	p := proxies[names[0]]
	return p.Name, p.Now, true
}

func (d *Daemon) HandleApply(req ipc.ApplyRequest) ipc.ApplyResponse {
	if err := d.apply(req); err != nil {
		return ipc.ApplyResponse{Err: err.Error()}
	}
	return ipc.ApplyResponse{Status: d.status()}
}

func (d *Daemon) apply(req ipc.ApplyRequest) error {
	d.applyMu.Lock()
	defer d.applyMu.Unlock()

	if req.ConfigPath == "" {
		return fmt.Errorf("%s", locale.T("err.no_active"))
	}
	if req.Mode == "" {
		req.Mode = "rule"
	}
	_ = os.MkdirAll(config.RuntimeDir, 0755)

	runtimePath, workDir, proxyAddr, err := core.BuildRuntimeConfig(req.ConfigPath, req.Mode, req.TUN)
	if err != nil {
		return err
	}

	d.mu.Lock()
	d.configPath = req.ConfigPath
	d.configName = req.ConfigName
	d.workDir = workDir
	d.mode = req.Mode
	d.tunOn = req.TUN
	d.proxyAddr = proxyAddr
	d.delays = map[string]int{} // cached latencies belong to the previous config
	d.mu.Unlock()
	d.api.ResetProviders() // provider membership belongs to the loaded config

	if err := d.kernel.Start(runtimePath, workDir); err != nil {
		return fmt.Errorf("start kernel: %w", err)
	}
	if err := d.waitAPI(5 * time.Second); err != nil {
		return err
	}
	if req.Mode == "global" {
		d.syncGlobal()
	}

	if req.SystemProxy {
		if err := d.enableProxy(req.User); err != nil {
			return err
		}
	} else {
		_ = d.disableProxy()
	}
	return nil
}

func (d *Daemon) waitAPI(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if d.api.Healthy() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("kernel API not ready after %s", timeout)
}

func (d *Daemon) enableProxy(u config.UserInfo) error {
	d.mu.Lock()
	addr := d.proxyAddr
	d.mu.Unlock()
	if addr == "" {
		return fmt.Errorf("kernel proxy address unknown")
	}
	if err := systemproxy.Enable(u, addr); err != nil {
		return err
	}
	d.mu.Lock()
	d.systemProxy = true
	d.proxyUser = u
	d.mu.Unlock()
	return nil
}

func (d *Daemon) disableProxy() error {
	d.mu.Lock()
	on := d.systemProxy
	u := d.proxyUser
	d.mu.Unlock()
	if !on {
		return nil
	}
	if err := systemproxy.Disable(u); err != nil {
		return err
	}
	d.mu.Lock()
	d.systemProxy = false
	d.mu.Unlock()
	return nil
}

func (d *Daemon) HandleMode(req ipc.ModeRequest) ipc.SimpleResponse {
	if req.Mode == "" {
		return ipc.SimpleResponse{Err: locale.T("err.invalid_mode", req.Mode)}
	}
	if !d.kernel.Running() {
		d.mu.Lock()
		d.mode = req.Mode
		d.mu.Unlock()
		return ipc.SimpleResponse{}
	}
	if err := d.api.SetMode(req.Mode); err != nil {
		return ipc.SimpleResponse{Err: err.Error()}
	}
	if req.Mode == "global" {
		d.syncGlobal()
	}
	d.mu.Lock()
	d.mode = req.Mode
	d.mu.Unlock()
	return ipc.SimpleResponse{}
}

// syncGlobal makes global mode use the user's current node. Users only think in
// terms of a single selected node, so GLOBAL is pointed at the active group
// instead of keeping its own independent selection.
func (d *Daemon) syncGlobal() {
	if group, _, ok := d.currentNode(); ok {
		_ = d.api.Select("GLOBAL", group)
	}
}

func (d *Daemon) HandleSystem(req ipc.SystemRequest) ipc.SimpleResponse {
	if req.On {
		if err := d.enableProxy(req.User); err != nil {
			return ipc.SimpleResponse{Err: err.Error()}
		}
		return ipc.SimpleResponse{}
	}
	if err := d.disableProxy(); err != nil {
		return ipc.SimpleResponse{Err: err.Error()}
	}
	return ipc.SimpleResponse{}
}

func (d *Daemon) HandleTun(req ipc.TunRequest) ipc.SimpleResponse {
	d.mu.Lock()
	cfg, name, mode, sp, user := d.configPath, d.configName, d.mode, d.systemProxy, d.proxyUser
	d.mu.Unlock()
	if cfg == "" {
		return ipc.SimpleResponse{Err: locale.T("err.no_active")}
	}
	err := d.apply(ipc.ApplyRequest{
		ConfigPath:  cfg,
		ConfigName:  name,
		Mode:        mode,
		SystemProxy: sp,
		TUN:         req.On,
		User:        user,
	})
	if err != nil {
		return ipc.SimpleResponse{Err: err.Error()}
	}
	return ipc.SimpleResponse{}
}

func (d *Daemon) HandleNodes(_ ipc.NodesRequest) ipc.NodesResponse {
	proxies, err := d.api.Proxies()
	if err != nil {
		return ipc.NodesResponse{Err: err.Error()}
	}
	var groups []ipc.NodeGroup
	for name, p := range proxies {
		if p.All == nil {
			continue
		}
		groups = append(groups, ipc.NodeGroup{Name: name, Type: p.Type, Now: p.Now, Nodes: p.All})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	d.mu.Lock()
	delays := make(map[string]int, len(d.delays))
	for k, v := range d.delays {
		delays[k] = v
	}
	d.mu.Unlock()
	return ipc.NodesResponse{Groups: groups, Delays: delays}
}

func (d *Daemon) HandleDelay(req ipc.DelayRequest) ipc.DelayResponse {
	if !d.kernel.Running() {
		return ipc.DelayResponse{Err: locale.T("err.kernel_missing")}
	}
	ms, err := d.api.Delay(req.Name, req.Timeout, req.URL)
	d.mu.Lock()
	if err != nil {
		d.delays[req.Name] = -1
	} else {
		d.delays[req.Name] = ms
	}
	d.mu.Unlock()
	if err != nil {
		return ipc.DelayResponse{Err: err.Error()}
	}
	return ipc.DelayResponse{Delay: ms}
}

func (d *Daemon) HandleSelect(req ipc.SelectRequest) ipc.SimpleResponse {
	if err := d.api.Select(req.Group, req.Node); err != nil {
		return ipc.SimpleResponse{Err: err.Error()}
	}
	return ipc.SimpleResponse{}
}

func (d *Daemon) HandleStop(_ ipc.StopRequest) ipc.SimpleResponse {
	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	d.kernel.Stop()
	if err := d.disableProxy(); err != nil {
		return ipc.SimpleResponse{Err: err.Error()}
	}
	d.mu.Lock()
	d.tunOn = false
	d.mu.Unlock()
	return ipc.SimpleResponse{}
}
