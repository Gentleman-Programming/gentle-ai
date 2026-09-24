//go:build !windows

package pi

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const transportWorkingDirectory = "/"

func runModelRoutingProcess(ctx context.Context, executable string, request []byte, o ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	command := exec.Command(executable)
	command.Dir = transportWorkingDirectory
	command.Env = transportEnvironment()
	command.Stdin = bytes.NewReader(request)
	// Process-group kill covers ordinary inherited descendants; setsid escapes it.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout := &boundedTransportOutput{limit: o.MaxStdoutBytes}
	stderr := &boundedTransportOutput{limit: o.MaxStderrBytes}
	command.Stdout = stdout
	command.Stderr = stderr

	if err := command.Start(); err != nil {
		return ModelRoutingProcessResult{}, transportError(TransportErrorStart, err)
	}

	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	select {
	case waitErr := <-wait:
		return finishTransportProcess(stdout, stderr, waitErr)
	case <-runCtx.Done():
		// Prefer a completed wait over cancellation.
		select {
		case waitErr := <-wait:
			return finishTransportProcess(stdout, stderr, waitErr)
		default:
		}

		terminationErr := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		waitErr := <-wait
		result, _ := finishTransportProcess(stdout, stderr, waitErr)
		if terminationErr != nil && !errors.Is(terminationErr, syscall.ESRCH) {
			return result, transportError(TransportErrorTermination, errors.Join(runCtx.Err(), terminationErr, waitErr))
		}
		kind := TransportErrorCanceled
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			kind = TransportErrorTimeout
		}
		return result, transportError(kind, runCtx.Err())
	}
}
func transportEnvironment() []string {
	env := make([]string, 0)
	pathPresent := false
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch key {
		case "PATH", "LANG", "LANGUAGE", "LC_ALL", "LC_COLLATE", "LC_CTYPE", "LC_MESSAGES", "LC_MONETARY", "LC_NUMERIC", "LC_TIME", "TMPDIR", "TMP", "TEMP":
			if key == "PATH" && value == "" {
				continue
			}
			env = append(env, entry)
			pathPresent = pathPresent || key == "PATH"
		}
	}
	if !pathPresent {
		env = append(env, "PATH=/usr/local/bin:/usr/bin:/bin")
	}
	return env
}

func finishTransportProcess(stdout, stderr *boundedTransportOutput, waitErr error) (ModelRoutingProcessResult, error) {
	result := ModelRoutingProcessResult{Stdout: stdout.buf.Bytes(), StderrBytes: stderr.count}
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		return result, transportError(TransportErrorWait, waitErr)
	}
	if exitErr != nil {
		result.ExitCode = exitErr.ExitCode()
	}
	if stdout.overflow {
		cause := []error{ErrTransportStdoutOverflow}
		if stderr.overflow {
			cause = append(cause, ErrTransportStderrOverflow)
		}
		cause = append(cause, waitErr)
		return result, transportError(TransportErrorStdoutOverflow, errors.Join(cause...))
	}
	if stderr.overflow {
		return result, transportError(TransportErrorStderrOverflow, errors.Join(ErrTransportStderrOverflow, waitErr))
	}
	if waitErr != nil {
		return result, transportError(TransportErrorNonzeroExit, waitErr)
	}
	return result, nil
}
