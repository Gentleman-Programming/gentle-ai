package pi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"
)

type ModelRoutingProcessOptions struct {
	Timeout                                         time.Duration
	MaxRequestBytes, MaxStdoutBytes, MaxStderrBytes int
}

type ModelRoutingProcessResult struct {
	Stdout                []byte
	ExitCode, StderrBytes int
}

type TransportErrorKind string

const (
	TransportErrorInvalidOptions      TransportErrorKind = "invalid-options"
	TransportErrorInvalidPath         TransportErrorKind = "invalid-path"
	TransportErrorInvalidRequest      TransportErrorKind = "invalid-request"
	TransportErrorStart               TransportErrorKind = "start"
	TransportErrorWait                TransportErrorKind = "wait"
	TransportErrorCanceled            TransportErrorKind = "canceled"
	TransportErrorTimeout             TransportErrorKind = "timeout"
	TransportErrorStdoutOverflow      TransportErrorKind = "stdout-overflow"
	TransportErrorStderrOverflow      TransportErrorKind = "stderr-overflow"
	TransportErrorNonzeroExit         TransportErrorKind = "nonzero-exit"
	TransportErrorTermination         TransportErrorKind = "termination"
	TransportErrorUnsupportedPlatform TransportErrorKind = "unsupported-platform"
)

var (
	ErrTransportInvalidOptions      = errors.New("invalid model-routing process options")
	ErrTransportInvalidPath         = errors.New("invalid model-routing executable path")
	ErrTransportInvalidRequest      = errors.New("model-routing request exceeds its limit")
	ErrTransportStart               = errors.New("model-routing process failed to start")
	ErrTransportWait                = errors.New("model-routing process wait failed")
	ErrTransportCanceled            = context.Canceled
	ErrTransportTimeout             = context.DeadlineExceeded
	ErrTransportStdoutOverflow      = errors.New("model-routing stdout exceeds its limit")
	ErrTransportStderrOverflow      = errors.New("model-routing stderr exceeds its limit")
	ErrTransportNonzeroExit         = errors.New("model-routing process exited unsuccessfully")
	ErrTransportTermination         = errors.New("model-routing process termination failed")
	ErrTransportUnsupportedPlatform = errors.ErrUnsupported
)

var transportErrorSentinels = map[TransportErrorKind]error{
	TransportErrorInvalidOptions: ErrTransportInvalidOptions, TransportErrorInvalidPath: ErrTransportInvalidPath,
	TransportErrorInvalidRequest: ErrTransportInvalidRequest, TransportErrorStart: ErrTransportStart,
	TransportErrorWait: ErrTransportWait, TransportErrorCanceled: ErrTransportCanceled,
	TransportErrorTimeout: ErrTransportTimeout, TransportErrorStdoutOverflow: ErrTransportStdoutOverflow,
	TransportErrorStderrOverflow: ErrTransportStderrOverflow, TransportErrorNonzeroExit: ErrTransportNonzeroExit,
	TransportErrorTermination: ErrTransportTermination, TransportErrorUnsupportedPlatform: ErrTransportUnsupportedPlatform,
}

type TransportError struct {
	Kind  TransportErrorKind
	Cause error
}

func (e *TransportError) Error() string {
	if e == nil {
		return "<nil>"
	}
	message := "model-routing process transport: " + string(e.Kind)
	if e.Cause != nil {
		message += ": " + e.Cause.Error()
	}
	return message
}

func (e *TransportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *TransportError) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	if typed, ok := target.(*TransportError); ok {
		return typed != nil && e.Kind != "" && e.Kind == typed.Kind
	}
	return target == transportErrorSentinels[e.Kind]
}

func RunBoundedModelRoutingProcess(ctx context.Context, executable string, request []byte, o ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
	if ctx == nil {
		return ModelRoutingProcessResult{}, transportError(TransportErrorInvalidOptions, ErrTransportInvalidOptions)
	}
	if err := ctx.Err(); err != nil {
		kind := TransportErrorCanceled
		if errors.Is(err, context.DeadlineExceeded) {
			kind = TransportErrorTimeout
		}
		return ModelRoutingProcessResult{}, transportError(kind, err)
	}
	if o.Timeout <= 0 || o.MaxRequestBytes <= 0 || o.MaxStdoutBytes <= 0 || o.MaxStderrBytes <= 0 {
		return ModelRoutingProcessResult{}, transportError(TransportErrorInvalidOptions, ErrTransportInvalidOptions)
	}
	if executable == "" || strings.IndexByte(executable, 0) >= 0 || !filepath.IsAbs(executable) || executable != filepath.Clean(executable) {
		return ModelRoutingProcessResult{}, transportError(TransportErrorInvalidPath, ErrTransportInvalidPath)
	}
	if len(request) > o.MaxRequestBytes {
		return ModelRoutingProcessResult{}, transportError(TransportErrorInvalidRequest, ErrTransportInvalidRequest)
	}
	result, err := runModelRoutingProcess(ctx, executable, cloneTransportBytes(request), o)
	result.Stdout = cloneTransportBytes(result.Stdout)
	return result, err
}

func transportError(kind TransportErrorKind, cause error) error {
	return &TransportError{Kind: kind, Cause: cause}
}

type boundedTransportOutput struct {
	limit, count int
	buf          bytes.Buffer
	overflow     bool
}

func (w *boundedTransportOutput) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	maxInt := int(^uint(0) >> 1)
	if len(p) > maxInt-w.count {
		w.count, w.overflow = maxInt, true
	} else {
		w.count += len(p)
	}
	if w.count > w.limit {
		w.overflow = true
	}
	retention := w.limit
	if retention < 0 {
		retention = 0
	}
	if retention < maxInt {
		retention++
	}
	if remaining := retention - w.buf.Len(); remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = w.buf.Write(p[:remaining])
	}
	return len(p), nil
}

func drainTransportOutput(reader io.Reader, output *boundedTransportOutput) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		if reader != nil && output != nil {
			_, _ = io.Copy(output, reader)
		}
		close(done)
	}()
	return done
}

func cloneTransportBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	copyValue := make([]byte, len(value))
	copy(copyValue, value)
	return copyValue
}
