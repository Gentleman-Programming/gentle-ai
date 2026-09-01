//go:build windows

package engram

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var ntResumeProbeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

// startProbeProcessTree assigns the suspended child to a kill-on-close Job
// Object before it can create descendants that inherit the stdout handle.
func startProbeProcessTree(command *exec.Cmd) (func() error, error) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	if err := command.Start(); err != nil {
		return nil, err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, err
	}
	terminate := func() error {
		terminateErr := windows.TerminateJobObject(job, 1)
		closeErr := windows.CloseHandle(job)
		if terminateErr != nil {
			return terminateErr
		}
		return closeErr
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = terminate()
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, err
	}

	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, uint32(command.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, process)
	}
	if err == nil {
		err = ntResumeProbeProcess.Find()
	}
	if err == nil {
		status, _, _ := ntResumeProbeProcess.Call(uintptr(process))
		if status != 0 {
			err = windows.NTStatus(status)
		}
	}
	if process != 0 {
		_ = windows.CloseHandle(process)
	}
	if err != nil {
		_ = terminate()
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("assign engram mcp process tree: %w", err)
	}
	return terminate, nil
}
