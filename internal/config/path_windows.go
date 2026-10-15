//go:build windows

package config

import (
	"os"
	"path/filepath"
)

var (
	InstallDir    = filepath.Join(os.Getenv("ProgramFiles"), "manax")
	InstallBin    = filepath.Join(InstallDir, "manax.exe")
	KernelPath    = filepath.Join(InstallDir, "clash.meta.exe")
	RuntimeDir    = filepath.Join(os.Getenv("ProgramData"), "manax")
	ServiceName   = "manax"
	LoginTaskName = "manax-reload"
	PipeName      = `\\.\pipe\manax`
)
