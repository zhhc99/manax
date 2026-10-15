//go:build windows

package daemon

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// bindProcessLifetime ties every descendant of this process to its lifetime by
// placing the process in a job object that kills its members when the job's
// last handle closes, i.e. when the daemon exits by any means. This is the
// Windows counterpart of systemd's control-group and launchd's process-group
// cleanup: without it a killed daemon would leave the kernel running.
func bindProcessLifetime() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return err
	}
	self, err := windows.GetCurrentProcess()
	if err != nil {
		return err
	}
	// The job handle is deliberately never closed: it must outlive the daemon
	// so that the job is torn down exactly when the daemon dies.
	return windows.AssignProcessToJobObject(job, self)
}
