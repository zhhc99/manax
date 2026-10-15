package core

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// RestartInterval is how long the daemon waits before relaunching a crashed
// kernel. Crashes never touch the system proxy or TUN.
const RestartInterval = 3 * time.Second

// Kernel supervises the Clash-compatible kernel process.
type Kernel struct {
	Path    string
	LogPath string

	mu       sync.Mutex
	cmd      *exec.Cmd
	logFile  *os.File
	gen      int
	cfgPath  string
	workDir  string
	stopping bool
}

// NewKernel returns a kernel supervisor for the given binary.
func NewKernel(path, logPath string) *Kernel {
	return &Kernel{Path: path, LogPath: logPath}
}

// VersionLine runs the kernel with -v and returns its identity line, i.e. the
// kernel name and version (for example "Mihomo Meta v1.19.32"), or "" if it
// cannot be obtained.
func VersionLine(path string) string {
	out, err := exec.Command(path, "-v").Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	fields := strings.Fields(line)
	if len(fields) > 3 {
		fields = fields[:3]
	}
	return strings.Join(fields, " ")
}

// Start (re)launches the kernel with the given config and working directory.
// Any running instance is replaced.
func (k *Kernel) Start(cfgPath, workDir string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.cfgPath = cfgPath
	k.workDir = workDir
	k.stopping = false
	return k.restartLocked()
}

// Stop terminates the kernel and disables auto-restart.
func (k *Kernel) Stop() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.stopping = true
	k.cfgPath = ""
	k.killLocked()
}

// Running reports whether a kernel process is active.
func (k *Kernel) Running() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.cmd != nil
}

func (k *Kernel) restartLocked() error {
	k.killLocked()
	if k.cfgPath == "" {
		return nil
	}
	return k.startLocked()
}

func (k *Kernel) killLocked() {
	if k.cmd == nil {
		return
	}
	k.gen++
	cmd := k.cmd
	k.cmd = nil
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

func (k *Kernel) startLocked() error {
	args := []string{"-f", k.cfgPath, "-d", k.workDir,
		"-ext-ctl", "127.0.0.1:9090", "-secret", ""}
	cmd := exec.Command(k.Path, args...)
	if w := k.logWriterLocked(); w != nil {
		cmd.Stdout = w
		cmd.Stderr = w
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	k.gen++
	gen := k.gen
	k.cmd = cmd
	go k.watch(cmd, gen)
	return nil
}

func (k *Kernel) watch(cmd *exec.Cmd, gen int) {
	_ = cmd.Wait()
	k.mu.Lock()
	if k.gen != gen {
		k.mu.Unlock()
		return
	}
	k.cmd = nil
	k.mu.Unlock()

	// Only retry the kernel at an interval; nothing else is touched.
	time.Sleep(RestartInterval)

	k.mu.Lock()
	defer k.mu.Unlock()
	if k.gen != gen || k.cmd != nil || k.cfgPath == "" || k.stopping {
		return
	}
	_ = k.startLocked()
}

func (k *Kernel) logWriterLocked() io.Writer {
	if k.LogPath == "" {
		return nil
	}
	if k.logFile == nil {
		f, err := os.OpenFile(k.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil
		}
		k.logFile = f
	}
	return k.logFile
}
