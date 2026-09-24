package pi

import (
	"errors"
	"os/exec"
)

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
