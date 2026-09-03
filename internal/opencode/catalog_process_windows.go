//go:build windows

package opencode

import (
	"os/exec"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// configureProcessGroup isolates the child in a Windows Job Object so context
// cancellation kills the whole descendant tree, mirroring the Unix process
// group kill. Without this, a descendant that inherited stdout or stderr
// keeps the pipes open and holds discovery open past the context deadline
// even after the direct child has exited.
//
// It returns an afterStart hook the caller must invoke immediately after a
// successful cmd.Start() — assignment is only possible once cmd.Process
// exists — plus a release func that closes the job handle. Both are nil when
// the job could not be created, in which case cancellation falls back to the
// default exec.CommandContext behavior of killing the direct child.
//
// Assignment is best effort: when the host process already belongs to a job
// (nested jobs are denied by default), AssignProcessToJobObject fails and
// cancellation degrades to killing the direct child. cmd.WaitDelay stays set
// regardless, so Wait cannot hang indefinitely on inherited pipe handles even
// if a descendant escapes the job.
func configureProcessGroup(cmd *exec.Cmd) (afterStart func(), release func()) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, nil
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = windows.CloseHandle(job)
		return nil, nil
	}

	cmd.WaitDelay = 5 * time.Second
	cmd.Cancel = func() error {
		_ = windows.TerminateJobObject(job, 1)
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return nil
	}

	var once sync.Once
	release = func() {
		once.Do(func() { _ = windows.CloseHandle(job) })
	}
	afterStart = func() {
		if cmd.Process == nil {
			return
		}
		// os.Process exposes no Windows HANDLE, so open one with the exact
		// rights job assignment needs. Best effort (see above): on failure
		// the job stays empty and cancellation degrades to the direct kill.
		h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
		if err != nil {
			return
		}
		defer windows.CloseHandle(h)
		_ = windows.AssignProcessToJobObject(job, h)
	}
	return afterStart, release
}
