//go:build linux

package config

const (
	InstallBin    = "/usr/local/bin/manax"
	InstallDir    = "/usr/local/lib/manax"
	KernelPath    = "/usr/local/lib/manax/clash.meta"
	RuntimeDir    = "/run/manax"
	SocketPath    = "/run/manax/manax.sock"
	ServiceName   = "manax.service"
	ServicePath   = "/etc/systemd/system/manax.service"
	LoginTaskName = "manax-reload.service"
)
