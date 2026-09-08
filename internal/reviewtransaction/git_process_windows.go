//go:build windows

package reviewtransaction

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"
)

var ntResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

// assignGitProcessToJob is the bind step; tests swap it to force a failure
// after the child exists and prove the suspended child is killed.
var assignGitProcessToJob = windows.AssignProcessToJobObject

// startGitProcessTree starts the git command suspended, binds it to a job
// object that kills the whole tree when the job closes, then resumes it.
//
// Every Windows git launch funnels through here, so it has to be exact about
// object lifetimes: the job limit structure stays a named local kept alive
// across the raw pointer hand-off, handles are closed only when they were
// opened, and a child that could not be bound is killed instead of being left
// suspended forever (#4081, #4128, #4152).
func startGitProcessTree(command *exec.Cmd) (func() error, error) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	if err := command.Start(); err != nil {
		return nil, err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		_ = command.Process.Kill()
		return nil, err
	}
	release := func() error { _ = windows.TerminateJobObject(job, 1); return windows.CloseHandle(job) }
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	runtime.KeepAlive(&limits)
	var process windows.Handle
	if err == nil {
		process, err = windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, uint32(command.Process.Pid))
	}
	if err == nil {
		err = assignGitProcessToJob(job, process)
	}
	if err == nil {
		err = ntResumeProcess.Find()
	}
	if err == nil {
		if status, _, _ := ntResumeProcess.Call(uintptr(process)); status != 0 {
			err = windows.NTStatus(status)
		}
	}
	if process != 0 {
		_ = windows.CloseHandle(process)
	}
	if err != nil {
		// The child is still suspended; releasing the job kills it when it was
		// bound, and the direct kill covers the case where binding failed.
		_ = release()
		_ = command.Process.Kill()
		return nil, err
	}
	return release, nil
}
