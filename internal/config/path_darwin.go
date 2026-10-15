//go:build darwin

package config

const (
	InstallBin    = "/usr/local/bin/manax"
	InstallDir    = "/usr/local/lib/manax"
	KernelPath    = "/usr/local/lib/manax/clash.meta"
	RuntimeDir    = "/var/run/manax"
	SocketPath    = "/var/run/manax/manax.sock"
	ServiceName   = "com.manax.daemon"
	ServicePath   = "/Library/LaunchDaemons/com.manax.daemon.plist"
	LoginTaskName = "com.manax.reload"
)
