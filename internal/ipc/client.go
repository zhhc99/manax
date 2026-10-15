package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/zhhc99/manax/internal/locale"
)

var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dial()
		},
	},
}

func post[Req any, Resp any](path string, req Req) (Resp, error) {
	var zero Resp
	body, err := json.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("IPC marshal: %w", err)
	}
	resp, err := httpClient.Post("http://manax"+path, "application/json", bytes.NewReader(body))
	if err != nil {
		if !isServicePresent() {
			return zero, fmt.Errorf("%s", locale.T("err.service_not_running"))
		}
		return zero, fmt.Errorf("%s", locale.T("err.service_not_responding"))
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&zero); err != nil {
		return zero, fmt.Errorf("IPC decode: %w", err)
	}
	return zero, nil
}

// PostStatus queries the daemon status.
func PostStatus(req StatusRequest) (StatusResponse, error) {
	return post[StatusRequest, StatusResponse]("/status", req)
}

// PostApply tells the daemon to load a config.
func PostApply(req ApplyRequest) (ApplyResponse, error) {
	return post[ApplyRequest, ApplyResponse]("/apply", req)
}

// PostMode sets the running mode.
func PostMode(req ModeRequest) (SimpleResponse, error) {
	return post[ModeRequest, SimpleResponse]("/mode", req)
}

// PostSystem toggles the system proxy.
func PostSystem(req SystemRequest) (SimpleResponse, error) {
	return post[SystemRequest, SimpleResponse]("/system", req)
}

// PostTun toggles TUN.
func PostTun(req TunRequest) (SimpleResponse, error) {
	return post[TunRequest, SimpleResponse]("/tun", req)
}

// PostNodes lists proxy groups.
func PostNodes() (NodesResponse, error) {
	return post[NodesRequest, NodesResponse]("/nodes", NodesRequest{})
}

// PostDelay measures one node.
func PostDelay(req DelayRequest) (DelayResponse, error) {
	return post[DelayRequest, DelayResponse]("/delay", req)
}

// PostSelect selects a node in a group.
func PostSelect(req SelectRequest) (SimpleResponse, error) {
	return post[SelectRequest, SimpleResponse]("/select", req)
}

// PostStop asks the daemon to stop the kernel and clear the system proxy.
func PostStop(req StopRequest) (SimpleResponse, error) {
	return post[StopRequest, SimpleResponse]("/stop", req)
}

// IsRunning reports whether the daemon socket is reachable.
func IsRunning() bool {
	if !isServicePresent() {
		return false
	}
	_, err := PostStatus(StatusRequest{})
	return err == nil
}
