// Package service installs the system service, the kernel and the per-user
// login task on each platform.
package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/locale"
)

// KernelRepo is the compatible kernel release source.
const KernelRepo = "MetaCubeX/mihomo"

// ErrNeedSudo is returned when elevation requires the user to run sudo.
var ErrNeedSudo = errors.New("pkexec is not available")

// ErrElevatedFailed is returned when the elevated helper ran and failed. Its
// output has already been handed to the caller, so there is nothing left to
// report.
var ErrElevatedFailed = errors.New("elevated command failed")

func installKernel(resources string) error {
	if resources != "" {
		return InstallKernelPackage(resources)
	}
	info, err := os.Stat(config.KernelPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("kernel not found at %s", config.KernelPath)
	}
	return nil
}

// installGeoData installs the shared GeoData from a resource package, or
// verifies an existing system-wide installation when none is given.
func installGeoData(resources string) error {
	if resources != "" {
		return InstallGeoDataPackage(resources, config.DataDir)
	}
	for _, name := range config.GeoDataFiles {
		info, err := os.Stat(filepath.Join(config.DataDir, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("geodata not found at %s: %s", config.DataDir, name)
		}
	}
	return nil
}

// InstallBinary copies the running executable to the fixed install path.
func InstallBinary(self string) error {
	if err := os.MkdirAll(config.InstallDir, 0755); err != nil {
		return err
	}
	if abs(self) == abs(config.InstallBin) {
		return nil
	}
	return copyFile(self, config.InstallBin)
}

// InstallLogin creates the login task that runs "manax reload" as the user.
// The user phase runs unprivileged, so nothing is written outside the user's home.
func InstallLogin(u config.UserInfo) error {
	return installLogin(u)
}

// RemoveAllLogins removes the login task of every local user.
func RemoveAllLogins() {
	seen := map[string]bool{}
	for _, u := range localUsers() {
		key := fmt.Sprintf("%d|%s", u.UID, u.Home)
		if seen[key] {
			continue
		}
		seen[key] = true
		if item, err := removeLogin(u); err == nil && item != "" {
			fmt.Printf(locale.T("msg.removed_task")+"\n", item)
		}
	}
}

// removeDir removes the manax install directory and reports it if it existed.
func removeDir(dir string) {
	if _, err := os.Stat(dir); err != nil {
		return
	}
	if os.RemoveAll(dir) == nil {
		fmt.Printf(locale.T("msg.removed_path")+"\n", dir)
	}
}

// KernelRelease is the latest kernel release asset for the current platform.
type KernelRelease struct {
	Asset  string
	URL    string
	SHA256 string
}

func assetName(tag string) (string, error) {
	compatible := ""
	if runtime.GOARCH == "amd64" {
		compatible = "-compatible"
	}
	switch runtime.GOOS {
	case "linux", "darwin":
		return fmt.Sprintf("mihomo-%s-%s%s-%s.gz", runtime.GOOS, runtime.GOARCH, compatible, tag), nil
	case "windows":
		return fmt.Sprintf("mihomo-windows-%s%s-%s.zip", runtime.GOARCH, compatible, tag), nil
	default:
		return "", fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func writeBinary(dst string, r io.Reader) error {
	return writeAtomic(dst, func(out *os.File) error {
		_, err := io.Copy(out, r)
		return err
	})
}

func copyFile(src, dst string) error {
	return copyFileMode(src, dst, 0755)
}

func copyFileMode(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("%s: %w", locale.T("err.copy"), err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return writeAtomicMode(dst, mode, func(out *os.File) error {
		if _, err := io.Copy(out, in); err != nil {
			return fmt.Errorf("%s: %w", locale.T("err.copy"), err)
		}
		return nil
	})
}

// writeAtomic writes to a temp file in the destination directory, then renames
// it into place, so an interrupted copy or download never leaves a partial file
// at the final path.
func writeAtomic(dst string, fill func(*os.File) error) error {
	return writeAtomicMode(dst, 0755, fill)
}

func writeAtomicMode(dst string, mode os.FileMode, fill func(*os.File) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".manax-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	if err := fill(tmp); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	tmpName = ""
	return nil
}

func abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}
