package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/zhhc99/manax/internal/locale"
)

var httpClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialContext(ctx)
		},
	},
}

func post[Req any, Resp any](ctx context.Context, path string, req Req) (Resp, error) {
	var zero Resp
	body, err := json.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("IPC marshal: %w", err)
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://manax"+path, bytes.NewReader(body))
	if err != nil {
		return zero, err
	}
	r.Header.Set("Content-Type", "application/json")
	client := *httpClient
	if path == "/apply" {
		client.Timeout = 60 * time.Second // Kernel initialization may take longer.
	}
	resp, err := client.Do(r)
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
func PostStatus(ctx context.Context, req StatusRequest) (StatusResponse, error) {
	return post[StatusRequest, StatusResponse](ctx, "/status", req)
}

// PostApply tells the daemon to load a config.
func PostApply(req ApplyRequest) (ApplyResponse, error) {
	return post[ApplyRequest, ApplyResponse](context.Background(), "/apply", req)
}

// PostMode sets the running mode.
func PostMode(req ModeRequest) (SimpleResponse, error) {
	return post[ModeRequest, SimpleResponse](context.Background(), "/mode", req)
}

// PostSystem toggles the system proxy.
func PostSystem(req SystemRequest) (SimpleResponse, error) {
	return post[SystemRequest, SimpleResponse](context.Background(), "/system", req)
}

// PostTun toggles TUN.
func PostTun(req TunRequest) (SimpleResponse, error) {
	return post[TunRequest, SimpleResponse](context.Background(), "/tun", req)
}

// PostNodes lists all nodes.
func PostNodes() (NodesResponse, error) {
	return post[NodesRequest, NodesResponse](context.Background(), "/nodes", NodesRequest{})
}

// PostDelay measures one node.
func PostDelay(ctx context.Context, req DelayRequest) (DelayResponse, error) {
	return post[DelayRequest, DelayResponse](ctx, "/delay", req)
}

// PostSelect selects a node.
func PostSelect(req SelectRequest) (SimpleResponse, error) {
	return post[SelectRequest, SimpleResponse](context.Background(), "/select", req)
}

// PostStop asks the daemon to stop the kernel and clear the system proxy.
func PostStop(req StopRequest) (SimpleResponse, error) {
	return post[StopRequest, SimpleResponse](context.Background(), "/stop", req)
}

// IsRunning reports whether the daemon socket is reachable.
func IsRunning() bool {
	if !isServicePresent() {
		return false
	}
	_, err := PostStatus(context.Background(), StatusRequest{})
	return err == nil
}
