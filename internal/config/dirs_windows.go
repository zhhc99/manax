//go:build windows

package config

import (
	"os"

	"golang.org/x/sys/windows"
)

// DataDir holds the shared GeoData: read-only for ordinary users, writable
// only by SYSTEM and administrators. WorkDir holds generated runtime configs
// and kernel state, so it stays private to SYSTEM.
const (
	dataDirSDDL = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FR;;;BU)"
	workDirSDDL = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
)

// EnsureDataDir creates the system data directory and fixes its permissions.
func EnsureDataDir() error { return ensureDir(DataDir, dataDirSDDL) }

// EnsureWorkDir creates the daemon-owned kernel work directory.
func EnsureWorkDir() error { return ensureDir(WorkDir, workDirSDDL) }

func ensureDir(dir, sddl string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
