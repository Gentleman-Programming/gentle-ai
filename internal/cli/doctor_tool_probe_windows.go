package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Batch launchers need cmd.exe; Go's CreateProcess cannot run them directly.
func doctorToolProbeCommand(ctx context.Context, path, arg string) (*exec.Cmd, error) {
	ext := filepath.Ext(path)
	if !strings.EqualFold(ext, ".cmd") && !strings.EqualFold(ext, ".bat") {
		return exec.CommandContext(ctx, path, arg), nil
	}
	// cmd expands percent variables even inside quotes. Refuse ambiguous paths
	// rather than executing a different command than the PATH-resolved launcher.
	if strings.ContainsAny(path, "%!\r\n\"") {
		return nil, fmt.Errorf("cannot safely probe batch launcher path %q; move the launcher to a path without expansion characters or quotes, then run 'gentle-ai doctor' again", path)
	}
	root := os.Getenv("SystemRoot")
	if root == "" {
		return nil, fmt.Errorf("SystemRoot is unset; restore it to your Windows directory, then run 'gentle-ai doctor' again")
	}
	shell := filepath.Join(root, "System32", "cmd.exe")
	cmd := exec.CommandContext(ctx, shell)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `"` + shell + `" /d /v:off /s /c ""` + path + `" ` + arg + `"`}
	return cmd, nil
}

var ntResumeDoctorProbeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

// startDoctorProbeProcessTree assigns the suspended child to a kill-on-close Job
// Object before it can create descendants that inherit the stdout handle.
func startDoctorProbeProcessTree(command *exec.Cmd) (func() error, error) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
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
		err = ntResumeDoctorProbeProcess.Find()
	}
	if err == nil {
		status, _, _ := ntResumeDoctorProbeProcess.Call(uintptr(process))
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
		return nil, fmt.Errorf("assign doctor probe process tree: %w", err)
	}
	return terminate, nil
}
