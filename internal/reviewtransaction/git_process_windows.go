//go:build windows

package reviewtransaction

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
	"unsafe"
)

var ntResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

func startGitProcessTree(command *exec.Cmd) (func() error, error) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	if err := command.Start(); err != nil {
		return nil, err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	release := func() error { _ = windows.TerminateJobObject(job, 1); return windows.CloseHandle(job) }
	var process windows.Handle
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE}})), uint32(unsafe.Sizeof(windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}))); err != nil {
	} else if process, err = windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, uint32(command.Process.Pid)); err != nil {
	} else if err = windows.AssignProcessToJobObject(job, process); err != nil {
	} else if err = ntResumeProcess.Find(); err != nil {
	} else if status, _, _ := ntResumeProcess.Call(uintptr(process)); status != 0 {
		err = windows.NTStatus(status)
	}
	_ = windows.CloseHandle(process)
	return release, err
}
