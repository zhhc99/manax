package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	APIHost = "127.0.0.1"
	APIPort = 9090
	APIBase = "http://127.0.0.1:9090"
)

// Proxy is a single Clash API proxy entry.
type Proxy struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	Now  string   `json:"now,omitempty"`
	All  []string `json:"all,omitempty"`
}

// Client talks to the kernel's Clash-compatible API.
type Client struct {
	base   string
	secret string
	http   *http.Client

	mu        sync.Mutex
	providers map[string]string // proxy name -> provider name
}

// NewClient returns a client for the given API base URL.
func NewClient(base, secret string) *Client {
	return &Client{base: base, secret: secret, http: &http.Client{Timeout: 5 * time.Second}}
}

func (c *Client) do(method, path string, body any, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Message != "" {
			return fmt.Errorf("%s", e.Message)
		}
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Version returns the kernel version string.
func (c *Client) Version() (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	if err := c.do(http.MethodGet, "/version", nil, &v); err != nil {
		return "", err
	}
	return v.Version, nil
}

// Healthy reports whether the API responds.
func (c *Client) Healthy() bool {
	_, err := c.Version()
	return err == nil
}

// Proxies returns all proxies keyed by name.
func (c *Client) Proxies() (map[string]Proxy, error) {
	var out struct {
		Proxies map[string]Proxy `json:"proxies"`
	}
	if err := c.do(http.MethodGet, "/proxies", nil, &out); err != nil {
		return nil, err
	}
	return out.Proxies, nil
}

// Delay measures a single node and returns its latency in milliseconds.
// Provider-backed nodes are not in the global proxies map, so they are routed
// through their provider's healthcheck endpoint instead.
func (c *Client) Delay(name string, timeoutMS int, testURL string) (int, error) {
	if timeoutMS <= 0 {
		timeoutMS = 5000
	}
	if testURL == "" {
		testURL = "http://www.gstatic.com/generate_204"
	}
	q := url.Values{}
	q.Set("timeout", fmt.Sprintf("%d", timeoutMS))
	q.Set("url", testURL)

	path := "/proxies/" + url.PathEscape(name) + "/delay?" + q.Encode()
	if provider, ok := c.providerOf(name); ok {
		path = "/providers/proxies/" + url.PathEscape(provider) + "/" + url.PathEscape(name) +
			"/healthcheck?" + q.Encode()
	}
	var out struct {
		Delay int `json:"delay"`
	}
	if err := c.do(http.MethodGet, path, nil, &out); err != nil {
		return 0, err
	}
	return out.Delay, nil
}

// providerOf returns the provider that owns a proxy, loading and caching the
// provider list on first use.
func (c *Client) providerOf(name string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.providers == nil {
		c.providers = map[string]string{}
		var out struct {
			Providers map[string]struct {
				Proxies []struct {
					Name string `json:"name"`
				} `json:"proxies"`
			} `json:"providers"`
		}
		if err := c.do(http.MethodGet, "/providers/proxies", nil, &out); err == nil {
			for pn, p := range out.Providers {
				for _, x := range p.Proxies {
					c.providers[x.Name] = pn
				}
			}
		}
	}
	p, ok := c.providers[name]
	return p, ok
}

// ResetProviders drops the cached proxy -> provider map. Provider membership
// belongs to the loaded config, so the cache must be discarded whenever the
// kernel reloads a different config.
func (c *Client) ResetProviders() {
	c.mu.Lock()
	c.providers = nil
	c.mu.Unlock()
}

// Select picks a node inside a proxy group.
func (c *Client) Select(group, node string) error {
	return c.do(http.MethodPut, "/proxies/"+url.PathEscape(group), map[string]string{"name": node}, nil)
}

// SetMode changes the kernel running mode.
func (c *Client) SetMode(mode string) error {
	return c.do(http.MethodPatch, "/configs", map[string]any{"mode": mode}, nil)
}

// SetTUN toggles TUN without restarting the kernel process.
func (c *Client) SetTUN(on bool) error {
	return c.do(http.MethodPatch, "/configs", map[string]any{"tun": map[string]bool{"enable": on}}, nil)
}
