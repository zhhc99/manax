//go:build linux || darwin

package config

import "os"

// EnsureDataDir creates the system data directory every user may read.
func EnsureDataDir() error {
	if err := os.MkdirAll(DataDir, 0755); err != nil {
		return err
	}
	return os.Chmod(DataDir, 0755)
}

// EnsureWorkDir creates the root-owned kernel work directory.
func EnsureWorkDir() error {
	if err := os.MkdirAll(WorkDir, 0700); err != nil {
		return err
	}
	return os.Chmod(WorkDir, 0700)
}
