package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Source records where a named config came from.
type Source struct {
	Location string `json:"location"`
	URL      bool   `json:"url"`
	Updated  string `json:"updated,omitempty"`
}

// State is the per-user preference and config index.
type State struct {
	Active      string            `json:"active,omitempty"`
	Mode        string            `json:"mode,omitempty"`
	SystemProxy bool              `json:"system_proxy,omitempty"`
	TUN         bool              `json:"tun,omitempty"`
	Configs     map[string]Source `json:"configs,omitempty"`
}

const DirectConfigName = "DIRECT"

// UserInfo identifies the user that owns a request or login task.
type UserInfo struct {
	Username string `json:"username,omitempty"`
	UID      int    `json:"uid,omitempty"`
	GID      int    `json:"gid,omitempty"`
	Home     string `json:"home,omitempty"`
	SID      string `json:"sid,omitempty"`
}

// UserDir returns this user's manax config directory.
func UserDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "manax"), nil
}

// ConfigsDir returns the directory holding named config files.
func ConfigsDir() (string, error) {
	d, err := UserDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "configs"), nil
}

// StatePath returns the path of the user state file.
func StatePath() (string, error) {
	d, err := UserDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "state.json"), nil
}

// ConfigFile returns the file path for a named config.
func ConfigFile(name string) (string, error) {
	d, err := ConfigsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, name+".yaml"), nil
}

func DirectConfigFile() (string, error) {
	d, err := UserDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, ".manax-direct.yaml"), nil
}

// LoadState reads the user state, returning an empty state when absent.
func LoadState() (*State, error) {
	s := &State{Configs: map[string]Source{}}
	p, err := StatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	if s.Configs == nil {
		s.Configs = map[string]Source{}
	}
	return s, nil
}

// Save writes the user state.
func (s *State) Save() error {
	p, err := StatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

// EffectiveMode returns the stored mode or the default.
func (s *State) EffectiveMode() string {
	if s.Mode == "" {
		return "rule"
	}
	return s.Mode
}

// NowStamp returns a human-readable timestamp.
func NowStamp() string { return time.Now().Format("2006-01-02 15:04:05") }
