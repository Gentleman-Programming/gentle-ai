//go:build windows

package pi

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ntResumeTransportProcess                 = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")
	windowsTransportCreateJobObject          = windows.CreateJobObject
	windowsTransportSetInformationJobObject  = windows.SetInformationJobObject
	windowsTransportOpenProcess              = windows.OpenProcess
	windowsTransportAssignProcessToJobObject = windows.AssignProcessToJobObject
	windowsTransportTerminateJobObject       = windows.TerminateJobObject
	windowsTransportCloseHandle              = windows.CloseHandle
	windowsTransportResumeProcess            = resumeWindowsTransportProcess
)

const windowsTransportReapTimeout = 500 * time.Millisecond

var errWindowsTransportReapTimeout = errors.New("Windows transport process did not finish after termination")

func runModelRoutingProcess(ctx context.Context, executable string, request []byte, o ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	command, err := newWindowsTransportCommand(executable)
	if err != nil {
		return ModelRoutingProcessResult{}, err
	}
	command.Stdin = bytes.NewReader(request)
	stdout, stderr := &boundedTransportOutput{limit: o.MaxStdoutBytes}, &boundedTransportOutput{limit: o.MaxStderrBytes}
	command.Stdout, command.Stderr = stdout, stderr
	closeJob, err := startWindowsTransportProcessTree(command)
	if err != nil {
		return ModelRoutingProcessResult{}, transportError(TransportErrorStart, err)
	}

	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	complete := func(waitErr error) (ModelRoutingProcessResult, error) {
		result, processErr := finishTransportProcess(stdout, stderr, waitErr)
		if closeErr := closeJob(false); closeErr != nil {
			return result, transportError(TransportErrorTermination, errors.Join(closeErr, processErr))
		}
		return result, processErr
	}

	select {
	case waitErr := <-wait:
		return complete(waitErr)
	case <-runCtx.Done():
		select {
		case waitErr := <-wait:
			return complete(waitErr)
		default:
		}
		cleanupErr := closeJob(true)
		waitErr, timedOut := waitWindowsTransport(wait)
		if timedOut {
			cleanupErr = errors.Join(cleanupErr, killWindowsTransportProcess(command))
			waitErr, timedOut = waitWindowsTransport(wait)
		}
		if timedOut {
			cleanupErr = errors.Join(cleanupErr, errWindowsTransportReapTimeout)
		} else {
			var exitErr *exec.ExitError
			if waitErr != nil && !errors.As(waitErr, &exitErr) {
				cleanupErr = errors.Join(cleanupErr, waitErr)
			}
		}
		result, _ := finishTransportProcess(stdout, stderr, waitErr)
		if cleanupErr != nil {
			return result, transportError(TransportErrorTermination, errors.Join(runCtx.Err(), cleanupErr))
		}
		kind := TransportErrorCanceled
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			kind = TransportErrorTimeout
		}
		return result, transportError(kind, runCtx.Err())
	}
}
func newWindowsTransportCommand(executable string) (*exec.Cmd, error) {
	plan, err := newWindowsTransportCommandPlan(executable, os.Environ())
	if err != nil {
		return nil, err
	}
	command := exec.Command(plan.Executable(), plan.Arguments()...)
	command.Dir, command.Env = plan.WorkingDirectory(), plan.Environment()
	return command, nil
}
func startWindowsTransportProcessTree(command *exec.Cmd) (func(bool) error, error) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	if err := command.Start(); err != nil {
		return nil, err
	}
	jobHandle, err := windowsTransportCreateJobObject(nil, nil)
	if err != nil {
		return nil, errors.Join(err, (&windowsTransportJob{handle: jobHandle}).close(false), killAndReapWindowsTransport(command))
	}
	job := &windowsTransportJob{handle: jobHandle}
	var process windows.Handle
	closeProcess := func() error {
		if process == 0 {
			return nil
		}
		handle := process
		process = 0
		return windowsTransportCloseHandle(handle)
	}
	fail := func(cause error, assigned bool) (func(bool) error, error) {
		return nil, errors.Join(cause, job.close(assigned), closeProcess(), killAndReapWindowsTransport(command))
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE}}
	if _, err = windowsTransportSetInformationJobObject(job.handle, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return fail(err, false)
	}
	process, err = windowsTransportOpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, uint32(command.Process.Pid))
	if err != nil {
		return fail(err, false)
	}
	if err = windowsTransportAssignProcessToJobObject(job.handle, process); err != nil {
		return fail(err, false)
	}
	if err = windowsTransportResumeProcess(process); err != nil {
		return fail(err, true)
	}
	if err = closeProcess(); err != nil {
		return fail(err, true)
	}
	return func(terminate bool) error { return job.close(terminate) }, nil
}

type windowsTransportJob struct{ handle windows.Handle }

func (job *windowsTransportJob) close(terminate bool) error {
	if job == nil || job.handle == 0 {
		return nil
	}
	handle := job.handle
	job.handle = 0
	if terminate {
		return errors.Join(windowsTransportTerminateJobObject(handle, 1), windowsTransportCloseHandle(handle))
	}
	return windowsTransportCloseHandle(handle)
}
func resumeWindowsTransportProcess(process windows.Handle) error {
	if err := ntResumeTransportProcess.Find(); err != nil {
		return err
	}
	if status, _, _ := ntResumeTransportProcess.Call(uintptr(process)); status != 0 {
		return windows.NTStatus(status)
	}
	return nil
}

func killWindowsTransportProcess(command *exec.Cmd) error {
	if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func killAndReapWindowsTransport(command *exec.Cmd) error {
	cleanupErr := killWindowsTransportProcess(command)
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	waitErr, timedOut := waitWindowsTransport(wait)
	if timedOut {
		return errors.Join(cleanupErr, errWindowsTransportReapTimeout)
	}
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		cleanupErr = errors.Join(cleanupErr, waitErr)
	}
	return cleanupErr
}

func waitWindowsTransport(wait <-chan error) (error, bool) {
	select {
	case waitErr := <-wait:
		return waitErr, false
	case <-time.After(windowsTransportReapTimeout):
		return nil, true
	}
}

type windowsTransportCommandPlan struct {
	executable string
	args       []string
	env        []string
	cwd        string
}

func (p windowsTransportCommandPlan) Executable() string       { return p.executable }
func (p windowsTransportCommandPlan) Arguments() []string      { return append([]string(nil), p.args...) }
func (p windowsTransportCommandPlan) Environment() []string    { return append([]string(nil), p.env...) }
func (p windowsTransportCommandPlan) WorkingDirectory() string { return p.cwd }

func newWindowsTransportCommandPlan(executable string, environment []string) (windowsTransportCommandPlan, error) {
	if err := validateWindowsTransportPath(executable); err != nil {
		return windowsTransportCommandPlan{}, windowsTransportPlanError(err)
	}
	plan := windowsTransportCommandPlan{
		executable: executable,
		env:        windowsTransportEnvironment(environment),
		cwd:        filepath.VolumeName(executable) + string(filepath.Separator),
	}
	if ext := strings.ToLower(filepath.Ext(executable)); ext == ".cmd" || ext == ".bat" {
		if err := validateWindowsTransportBatchPath(executable); err != nil {
			return windowsTransportCommandPlan{}, windowsTransportPlanError(err)
		}
		comspec, err := validatedWindowsTransportComSpec(environment)
		if err != nil {
			return windowsTransportCommandPlan{}, windowsTransportPlanError(err)
		}
		plan.executable = comspec
		plan.args = []string{"/d", "/s", "/c", `""` + executable + `""`}
	}
	return plan, nil
}

func windowsTransportPlanError(cause error) error {
	return transportError(TransportErrorInvalidPath, errors.Join(ErrTransportInvalidPath, cause))
}

func validateWindowsTransportPath(path string) error {
	if path == "" || strings.IndexByte(path, 0) >= 0 || !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.VolumeName(path) == "" {
		return ErrTransportInvalidPath
	}
	return nil
}

func validateWindowsTransportBatchPath(path string) error {
	if strings.ContainsAny(path, "%!&|<>^()\"'") {
		return ErrTransportInvalidPath
	}
	for _, r := range path {
		if unicode.IsControl(r) {
			return ErrTransportInvalidPath
		}
	}
	return nil
}

func validatedWindowsTransportComSpec(environment []string) (string, error) {
	values := windowsTransportEnvironmentValues(environment)
	if comspec, present := values["COMSPEC"]; present {
		if err := validateWindowsTransportInterpreter(comspec); err != nil {
			return "", err
		}
		return comspec, nil
	}
	root, present := values["SYSTEMROOT"]
	if !present {
		return "", ErrTransportInvalidPath
	}
	if err := validateWindowsTransportPath(root); err != nil {
		return "", err
	}
	interpreter := filepath.Join(root, "System32", "cmd.exe")
	if err := validateWindowsTransportInterpreter(interpreter); err != nil {
		return "", err
	}
	return interpreter, nil
}

func validateWindowsTransportInterpreter(path string) error {
	if err := validateWindowsTransportPath(path); err != nil || !strings.EqualFold(filepath.Base(path), "cmd.exe") {
		return ErrTransportInvalidPath
	}
	info, err := os.Stat(path)
	if err != nil {
		return errors.Join(ErrTransportInvalidPath, err)
	}
	if !info.Mode().IsRegular() {
		return ErrTransportInvalidPath
	}
	return nil
}

var windowsTransportEnvironmentKeys = [...]string{
	"COMSPEC", "LANG", "LANGUAGE", "LC_ALL", "LC_COLLATE", "LC_CTYPE", "LC_MESSAGES", "LC_MONETARY", "LC_NUMERIC", "LC_TIME", "PATH", "PATHEXT", "SYSTEMDRIVE", "SYSTEMROOT", "TEMP", "TMP", "TMPDIR", "WINDIR",
}

func windowsTransportEnvironmentValues(entries []string) map[string]string {
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			key = strings.ToUpper(key)
			if slices.Contains(windowsTransportEnvironmentKeys[:], key) {
				values[key] = value
			}
		}
	}
	return values
}

func windowsTransportEnvironment(entries []string) []string {
	values := windowsTransportEnvironmentValues(entries)
	if values["PATH"] == "" {
		root := values["SYSTEMROOT"]
		if root == "" {
			root = values["WINDIR"]
		}
		if root == "" {
			root = `C:\Windows`
		}
		values["PATH"] = filepath.Join(root, "System32") + ";" + root
	}
	env := make([]string, 0, len(windowsTransportEnvironmentKeys))
	for _, key := range windowsTransportEnvironmentKeys {
		if value, present := values[key]; present {
			env = append(env, key+"="+value)
		}
	}
	return env
}
