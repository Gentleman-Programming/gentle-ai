package pi

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func validTransportOptions() ModelRoutingProcessOptions {
	return ModelRoutingProcessOptions{Timeout: time.Second, MaxRequestBytes: 4, MaxStdoutBytes: 4, MaxStderrBytes: 4}
}
func validTransportPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "model-routing")
}
func requireTransportKind(t *testing.T, err error, want TransportErrorKind) {
	t.Helper()
	var typed *TransportError
	if !errors.As(err, &typed) || typed == nil || typed.Kind != want || !errors.Is(err, &TransportError{Kind: want}) {
		t.Fatalf("error=%T %v; want transport kind %q", err, err, want)
	}
}

func TestRunBoundedModelRoutingProcessRejectsInvalidOptions(t *testing.T) {
	for _, options := range []ModelRoutingProcessOptions{
		{MaxRequestBytes: 1, MaxStdoutBytes: 1, MaxStderrBytes: 1}, {Timeout: -1, MaxRequestBytes: 1, MaxStdoutBytes: 1, MaxStderrBytes: 1},
		{Timeout: time.Second, MaxStdoutBytes: 1, MaxStderrBytes: 1}, {Timeout: time.Second, MaxRequestBytes: -1, MaxStdoutBytes: 1, MaxStderrBytes: 1},
		{Timeout: time.Second, MaxRequestBytes: 1, MaxStderrBytes: 1}, {Timeout: time.Second, MaxRequestBytes: 1, MaxStdoutBytes: -1, MaxStderrBytes: 1},
		{Timeout: time.Second, MaxRequestBytes: 1, MaxStdoutBytes: 1}, {Timeout: time.Second, MaxRequestBytes: 1, MaxStdoutBytes: 1, MaxStderrBytes: -1},
	} {
		_, err := RunBoundedModelRoutingProcess(context.Background(), validTransportPath(t), nil, options)
		requireTransportKind(t, err, TransportErrorInvalidOptions)
	}
}

func TestRunBoundedModelRoutingProcessRejectsUncleanExecutablePaths(t *testing.T) {
	clean := validTransportPath(t)
	for _, tc := range []struct{ name, path string }{
		{"empty", ""}, {"relative", "model-routing"}, {"unclean", clean + string(filepath.Separator) + "."}, {"nul", clean + "\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RunBoundedModelRoutingProcess(context.Background(), tc.path, nil, validTransportOptions())
			requireTransportKind(t, err, TransportErrorInvalidPath)
		})
	}
}

func TestRunBoundedModelRoutingProcessUsesExactRequestLimit(t *testing.T) {
	options := validTransportOptions()
	_, err := RunBoundedModelRoutingProcess(context.Background(), validTransportPath(t), []byte("1234"), options)
	requireTransportKind(t, err, TransportErrorUnsupportedPlatform)
	options.MaxRequestBytes = 3
	_, err = RunBoundedModelRoutingProcess(context.Background(), validTransportPath(t), []byte("1234"), options)
	requireTransportKind(t, err, TransportErrorInvalidRequest)
}

func TestRunBoundedModelRoutingProcessReportsPreCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RunBoundedModelRoutingProcess(ctx, validTransportPath(t), nil, validTransportOptions())
	requireTransportKind(t, err, TransportErrorCanceled)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation cause was not preserved: %v", err)
	}
	deadline, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err = RunBoundedModelRoutingProcess(deadline, validTransportPath(t), nil, validTransportOptions())
	requireTransportKind(t, err, TransportErrorTimeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline cause was not preserved: %v", err)
	}
}

func TestTransportErrorNamedKindsMatchSentinels(t *testing.T) {
	cases := []struct {
		kind     TransportErrorKind
		sentinel error
	}{
		{TransportErrorInvalidOptions, ErrTransportInvalidOptions}, {TransportErrorInvalidPath, ErrTransportInvalidPath},
		{TransportErrorInvalidRequest, ErrTransportInvalidRequest}, {TransportErrorStart, ErrTransportStart},
		{TransportErrorWait, ErrTransportWait}, {TransportErrorCanceled, ErrTransportCanceled}, {TransportErrorTimeout, ErrTransportTimeout},
		{TransportErrorStdoutOverflow, ErrTransportStdoutOverflow}, {TransportErrorStderrOverflow, ErrTransportStderrOverflow},
		{TransportErrorNonzeroExit, ErrTransportNonzeroExit}, {TransportErrorTermination, ErrTransportTermination},
		{TransportErrorUnsupportedPlatform, ErrTransportUnsupportedPlatform},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			err := transportError(tc.kind, errors.New("cause"))
			requireTransportKind(t, err, tc.kind)
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("sentinel %v did not match %v", tc.sentinel, err)
			}
		})
	}
}

func TestTransportErrorIsNilSafeAndPreservesCause(t *testing.T) {
	var nilError *TransportError
	if nilError.Error() != "<nil>" || nilError.Unwrap() != nil || nilError.Is(nil) {
		t.Fatal("nil TransportError methods were not safe")
	}
	cause := errors.New("cause")
	err := &TransportError{Kind: TransportErrorInvalidOptions, Cause: cause}
	if !errors.Is(err, cause) || !errors.Is(err, &TransportError{Kind: TransportErrorInvalidOptions}) || !errors.Is(err, ErrTransportInvalidOptions) {
		t.Fatalf("cause or kind was not preserved: %v", err)
	}
	var typed *TransportError
	if !errors.As(err, &typed) || typed != err {
		t.Fatalf("errors.As did not recover typed error: %#v", typed)
	}
}
