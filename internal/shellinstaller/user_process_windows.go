//go:build windows

package shellinstaller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const userWindowsJobFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_JOB_MEMORY | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS

// The Windows API defines a pair of DWORDs, not a pointer-sized CPU structure.
// ENABLE | HARD_CAP limits this job to one logical CPU's share of the machine.
// Job memory is committed memory; this does not claim to disable the pagefile.
type userWindowsCPU struct {
	Flags uint32
	Rate  uint32
}

var resumeUserWindowsProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")
var assignUserWindowsProcessToJob = windows.AssignProcessToJobObject

func userWindowsJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	refuse := func(err error) (windows.Handle, error) {
		return 0, errors.Join(err, windows.CloseHandle(job))
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: userWindowsJobFlags, ActiveProcessLimit: 64,
		},
		JobMemoryLimit: 3221225472,
	}
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	runtime.KeepAlive(&limits)
	if err != nil {
		return refuse(err)
	}
	cpu := userWindowsCPU{Flags: 5, Rate: uint32(10000 / runtime.NumCPU())}
	if cpu.Rate == 0 {
		return refuse(errors.New("physical CPU count exceeds supported job cap"))
	}
	_, err = windows.SetInformationJobObject(job, windows.JobObjectCpuRateControlInformation, uintptr(unsafe.Pointer(&cpu)), uint32(unsafe.Sizeof(cpu)))
	runtime.KeepAlive(&cpu)
	if err != nil {
		return refuse(err)
	}
	var observed windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&observed)), uint32(unsafe.Sizeof(observed)), nil); err != nil {
		return refuse(err)
	}
	if observed.BasicLimitInformation.LimitFlags != limits.BasicLimitInformation.LimitFlags || observed.BasicLimitInformation.ActiveProcessLimit != 64 || observed.JobMemoryLimit != limits.JobMemoryLimit {
		return refuse(errors.New("physical Windows job memory/process limits differ"))
	}
	var observedCPU userWindowsCPU
	if err := windows.QueryInformationJobObject(job, windows.JobObjectCpuRateControlInformation, uintptr(unsafe.Pointer(&observedCPU)), uint32(unsafe.Sizeof(observedCPU)), nil); err != nil {
		return refuse(err)
	}
	if observedCPU != cpu {
		return refuse(errors.New("physical Windows job CPU limit differs"))
	}
	return job, nil
}

// No CREATE_NO_WINDOW: the eventual interactive entry inherits its console.
// This primitive supplies process custody, not path ownership, artifact trust,
// a Windows 11 qualification, or a complete installer/terminal contract.
func userWindowsStart(command *exec.Cmd) (func() error, error) {
	job, err := userWindowsJob()
	if err != nil {
		return nil, err
	}
	release := func() error {
		return errors.Join(windows.TerminateJobObject(job, 1), windows.CloseHandle(job))
	}
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	if command.WaitDelay == 0 {
		command.WaitDelay = 2 * time.Second
	}
	if err := command.Start(); err != nil {
		return nil, errors.Join(err, release())
	}
	refuse := func(cause error) (func() error, error) {
		stop := release()
		kill := command.Process.Kill() // Covers failure before job assignment.
		wait := command.Wait()         // Never leave a suspended child unreaped.
		return nil, errors.Join(cause, stop, kill, wait)
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, uint32(command.Process.Pid))
	if err != nil {
		return refuse(err)
	}
	if err := assignUserWindowsProcessToJob(job, process); err != nil {
		return refuse(errors.Join(err, windows.CloseHandle(process)))
	}
	if err := resumeUserWindowsProcess.Find(); err != nil {
		return refuse(errors.Join(err, windows.CloseHandle(process)))
	}
	status, _, _ := resumeUserWindowsProcess.Call(uintptr(process))
	closeErr := windows.CloseHandle(process)
	if status != 0 || closeErr != nil {
		return refuse(errors.Join(fmt.Errorf("resume bound Windows process: NTSTATUS %#x", status), closeErr))
	}
	return release, nil
}

const (
	userWindowsCancelEnv    = "GENTLE_SHELL_WINDOWS_CANCEL"
	userWindowsCancelPrefix = `Local\gentle-shell-windows-cancel-`
)

// Cancellation first signals an unguessable session-local event so the worker
// cancels its own context and runs its owned cleanup. Only after grace does
// Wait kill the worker; release then terminates the whole job as before.
func userWindowsCooperative(command *exec.Cmd, grace time.Duration) (func() error, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, err
	}
	name := userWindowsCancelPrefix + hex.EncodeToString(random[:])
	pointer, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	event, err := windows.CreateEvent(nil, 1, 0, pointer)
	if err != nil { // Includes ERROR_ALREADY_EXISTS: never share a foreign event.
		if event != 0 {
			err = errors.Join(err, windows.CloseHandle(event))
		}
		return nil, err
	}
	command.Env = append(command.Env, userWindowsCancelEnv+"="+name)
	command.Cancel = func() error { return windows.SetEvent(event) }
	command.WaitDelay = grace
	return func() error { return windows.CloseHandle(event) }, nil
}

// Worker side of userWindowsCooperative. An empty name is a direct internal
// invocation (console interrupt only); any other name must be the exact form.
func userWindowsCancelWatch(ctx context.Context, name string) (context.Context, func(), error) {
	if name == "" {
		ctx, cancel := context.WithCancel(ctx)
		return ctx, cancel, nil
	}
	suffix, found := strings.CutPrefix(name, userWindowsCancelPrefix)
	if !found || !userWindowsLowerHex(suffix, 32) {
		return nil, nil, errors.New("Windows worker cancellation channel differs")
	}
	pointer, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, nil, err
	}
	event, err := windows.OpenEvent(windows.SYNCHRONIZE, false, pointer)
	if err != nil {
		return nil, nil, err
	}
	done, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, nil, errors.Join(err, windows.CloseHandle(event))
	}
	ctx, cancel := context.WithCancel(ctx)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		if index, err := windows.WaitForMultipleObjects([]windows.Handle{event, done}, false, windows.INFINITE); err == nil && index == windows.WAIT_OBJECT_0 {
			cancel()
		}
	}()
	return ctx, func() {
		_ = windows.SetEvent(done)
		<-finished
		_ = windows.CloseHandle(done)
		_ = windows.CloseHandle(event)
		cancel()
	}, nil
}

// Terminates the other members of this worker's own job and confirms that only
// the worker remains. The exact supervisor-created limits are required first,
// so a process inside any foreign job never terminates anything. A member is
// terminated only while its pinned handle still matches a listed member.
func userWindowsJobQuiesce(timeout time.Duration) error {
	if err := userWindowsJobOwned(); err != nil {
		return errors.Join(err, errors.New("not inside the owned Windows worker job"))
	}
	self := windows.GetCurrentProcessId()
	members := func() (map[uint32]bool, error) {
		var list struct {
			Assigned, Listed uint32
			IDs              [64]uintptr // ActiveProcessLimit
		}
		if err := windows.QueryInformationJobObject(0, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&list)), uint32(unsafe.Sizeof(list)), nil); err != nil {
			return nil, err
		}
		if list.Listed != list.Assigned || list.Listed > uint32(len(list.IDs)) {
			return nil, errors.New("Windows job member list incomplete")
		}
		listed := map[uint32]bool{}
		for _, id := range list.IDs[:list.Listed] {
			listed[uint32(id)] = true
		}
		if !listed[self] {
			return nil, errors.New("Windows worker is not listed in its job")
		}
		return listed, nil
	}
	for deadline := time.Now().Add(timeout); ; time.Sleep(100 * time.Millisecond) {
		listed, err := members()
		if err != nil {
			return err
		}
		if len(listed) == 1 {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("owned Windows job processes remain after cancellation")
		}
		for id := range listed {
			if id == self {
				continue
			}
			process, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, id)
			if err != nil {
				continue // Already exited; the next listing decides.
			}
			if again, err := members(); err == nil && again[id] {
				_ = windows.TerminateProcess(process, 1)
			}
			_ = windows.CloseHandle(process)
		}
	}
}
