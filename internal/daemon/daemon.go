// Package daemon runs the system-wise service: it supervises the kernel
// process and performs privileged network operations on behalf of users.
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/core"
	"github.com/zhhc99/manax/internal/ipc"
	"github.com/zhhc99/manax/internal/locale"
	"github.com/zhhc99/manax/internal/systemproxy"
)

// runtimeState saves restart information; the daemon manages current state.
type runtimeState struct {
	ipc.ApplyRequest
	Stopped bool `json:"stopped"`
}

type delayInfo struct {
	ms      int
	updated time.Time
}

type Daemon struct {
	applyMu sync.Mutex
	mu      sync.Mutex

	kernel *core.Kernel
	api    *core.Client

	runtimeState
	proxyAddr string

	delays map[string]delayInfo
}

// New builds a daemon with no user config loaded.
func New() *Daemon {
	return &Daemon{
		kernel:       core.NewKernel(config.KernelPath, filepath.Join(config.WorkDir, "kernel.log")),
		api:          core.NewClient(core.APIBase, ""),
		runtimeState: runtimeState{ApplyRequest: ipc.ApplyRequest{Mode: "rule"}},
		delays:       map[string]delayInfo{},
	}
}

// Serve starts the IPC server and blocks until ctx is cancelled.
func (d *Daemon) Serve(ctx context.Context) error {
	if err := config.EnsureWorkDir(); err != nil {
		return err
	}
	if err := config.LinkGeoData(); err != nil {
		return err
	}
	srv, err := ipc.NewServer(d)
	if err != nil {
		if errors.Is(err, ipc.ErrAlreadyRunning) {
			return fmt.Errorf("%s", locale.T("err.service_not_responding"))
		}
		return err
	}
	defer srv.Close()
	data, err := os.ReadFile(filepath.Join(config.WorkDir, "state.json"))
	if err == nil {
		if err := json.Unmarshal(data, &d.runtimeState); err != nil {
			return err
		}
		saved := d.runtimeState
		if saved.ConfigPath != "" && !saved.Stopped {
			if err := d.apply(saved.ApplyRequest); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	go srv.Serve()
	<-ctx.Done()
	d.Shutdown()
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
		ConfigName:  d.ConfigName,
		Mode:        d.Mode,
		SystemProxy: d.SystemProxy,
		ProxyAddr:   d.proxyAddr,
		TUN:         d.TUN,
	}
	delays := make(map[string]delayInfo, len(d.delays))
	for k, v := range d.delays {
		delays[k] = v
	}
	d.mu.Unlock()

	if d.kernel.Running() {
		resp.KernelRunning = true
		resp.KernelVersion = core.VersionLine(config.KernelPath)
		if node := d.currentNode(); node != "" {
			resp.Node = node
			if delay, known := delays[node]; known {
				resp.DelayKnown = true
				resp.DelayUpdated = delay.updated
				if delay.ms >= 0 {
					resp.DelayMS = delay.ms
					resp.DelayOK = true
				}
			}
		}
	}
	return resp
}

func (d *Daemon) currentNode() string {
	proxies, err := d.api.Proxies()
	if err != nil {
		return ""
	}
	return proxies[core.NodeGroupName].Now
}

func (d *Daemon) HandleApply(req ipc.ApplyRequest) ipc.ApplyResponse {
	if err := d.apply(req); err != nil {
		return ipc.ApplyResponse{Err: err.Error()}
	}
	if resp := d.persist(); resp.Err != "" {
		return ipc.ApplyResponse{Err: resp.Err}
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

	runtimePath, proxyAddr, err := core.BuildRuntimeConfig(req.ConfigPath, req.Mode, req.TUN, config.WorkDir)
	if err != nil {
		return err
	}

	d.mu.Lock()
	d.ConfigPath = runtimePath
	d.ConfigName = req.ConfigName
	d.Mode = req.Mode
	d.TUN = req.TUN
	d.Stopped = false
	d.proxyAddr = proxyAddr
	d.delays = map[string]delayInfo{} // cached latencies belong to the previous config
	d.mu.Unlock()
	d.api.ResetProviders() // provider membership belongs to the loaded config

	if err := d.kernel.Start(runtimePath, config.WorkDir); err != nil {
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
	d.SystemProxy = true
	d.User = u
	d.mu.Unlock()
	return nil
}

func (d *Daemon) disableProxy() error {
	d.mu.Lock()
	on := d.SystemProxy
	u := d.User
	d.mu.Unlock()
	if !on {
		return nil
	}
	if err := systemproxy.Disable(u); err != nil {
		return err
	}
	d.mu.Lock()
	d.SystemProxy = false
	d.mu.Unlock()
	return nil
}

func (d *Daemon) HandleMode(req ipc.ModeRequest) ipc.SimpleResponse {
	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	if req.Mode == "" {
		return ipc.SimpleResponse{Err: locale.T("err.invalid_mode", req.Mode)}
	}
	if d.kernel.Running() {
		if err := d.api.SetMode(req.Mode); err != nil {
			return ipc.SimpleResponse{Err: err.Error()}
		}
		if req.Mode == "global" {
			d.syncGlobal()
		}
	}
	d.mu.Lock()
	d.Mode = req.Mode
	d.mu.Unlock()
	return d.persist()
}

// syncGlobal makes global mode use the same internal selector.
func (d *Daemon) syncGlobal() {
	_ = d.api.Select("GLOBAL", core.NodeGroupName)
}

func (d *Daemon) HandleSystem(req ipc.SystemRequest) ipc.SimpleResponse {
	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	var err error
	if req.On {
		err = d.enableProxy(req.User)
	} else {
		err = d.disableProxy()
	}
	if err != nil {
		return ipc.SimpleResponse{Err: err.Error()}
	}
	return d.persist()
}

func (d *Daemon) HandleTun(req ipc.TunRequest) ipc.SimpleResponse {
	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	if d.kernel.Running() {
		if err := d.api.SetTUN(req.On); err != nil {
			return ipc.SimpleResponse{Err: err.Error()}
		}
	}
	d.mu.Lock()
	d.TUN = req.On
	d.mu.Unlock()
	return d.persist()
}

func (d *Daemon) HandleNodes(_ ipc.NodesRequest) ipc.NodesResponse {
	proxies, err := d.api.Proxies()
	if err != nil {
		return ipc.NodesResponse{Err: err.Error()}
	}
	d.mu.Lock()
	delays := make(map[string]int, len(d.delays))
	for k, v := range d.delays {
		delays[k] = v.ms
	}
	d.mu.Unlock()
	return ipc.NodesResponse{Nodes: proxies[core.NodeGroupName].All, Delays: delays}
}

func (d *Daemon) HandleDelay(req ipc.DelayRequest) ipc.DelayResponse {
	if !d.kernel.Running() {
		return ipc.DelayResponse{Err: locale.T("err.kernel_missing")}
	}
	ms, err := d.api.Delay(req.Name, req.Timeout, req.URL)
	d.mu.Lock()
	if err != nil {
		ms = -1
	}
	d.delays[req.Name] = delayInfo{ms: ms, updated: time.Now()}
	d.mu.Unlock()
	if err != nil {
		return ipc.DelayResponse{Err: err.Error()}
	}
	return ipc.DelayResponse{Delay: ms}
}

func (d *Daemon) HandleSelect(req ipc.SelectRequest) ipc.SimpleResponse {
	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	if err := d.api.Select(core.NodeGroupName, req.Node); err != nil {
		return ipc.SimpleResponse{Err: err.Error()}
	}
	proxies, err := d.api.Proxies()
	if err != nil {
		return ipc.SimpleResponse{Err: err.Error()}
	}
	for name, p := range proxies {
		if p.Type == "Selector" && name != core.NodeGroupName && slices.Contains(p.All, core.NodeGroupName) {
			if err := d.api.Select(name, core.NodeGroupName); err != nil {
				return ipc.SimpleResponse{Err: err.Error()}
			}
		}
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
	d.TUN = false
	d.Stopped = true
	d.mu.Unlock()
	return d.persist()
}

func (d *Daemon) persist() ipc.SimpleResponse {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ConfigPath != "" {
		if _, _, err := core.BuildRuntimeConfig(d.ConfigPath, d.Mode, d.TUN, config.WorkDir); err != nil {
			return ipc.SimpleResponse{Err: err.Error()}
		}
	}
	data, err := json.Marshal(d.runtimeState)
	if err == nil {
		err = config.WriteFile(filepath.Join(config.WorkDir, "state.json"), data)
	}
	if err != nil {
		return ipc.SimpleResponse{Err: err.Error()}
	}
	return ipc.SimpleResponse{}
}
