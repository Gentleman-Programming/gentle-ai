package shellinstaller

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestUserServiceResultPreservesKnownExit(t *testing.T) {
	for _, status := range []int{2, 130} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			childErr := exec.CommandContext(ctx, "/bin/sh", "-c", "exit "+strconv.Itoa(status)).Run()
			var child *exec.ExitError
			if !errors.As(childErr, &child) || child.ExitCode() != status {
				t.Fatalf("invalid child fixture: %v", childErr)
			}
			if got := userServiceResult(childErr, nil); got != childErr {
				t.Fatalf("known exit became installation uncertainty: %v", got)
			}
			incomplete := userServiceResult(errors.Join(childErr, exec.ErrWaitDelay), nil)
			var unsettled *PrivateRuntimeError
			if !errors.As(incomplete, &unsettled) || unsettled.Kind != "uncertain" {
				t.Fatalf("child status hid incomplete wait: %v", incomplete)
			}
			readbackErr := errors.New("unit still populated")
			got := userServiceResult(childErr, readbackErr)
			var failure *PrivateRuntimeError
			if !errors.As(got, &failure) || failure.Kind != "uncertain" || !errors.Is(got, childErr) || !errors.Is(got, readbackErr) {
				t.Fatalf("readback failure lost precedence or causes: %v", got)
			}
		})
	}
}

func TestUserServiceResultRequiresSettledWaitAndReadback(t *testing.T) {
	for _, tc := range []struct {
		name          string
		run, readback error
	}{
		{"wait incomplete", exec.ErrWaitDelay, nil},
		{"readback incomplete", nil, errors.New("missing receipt")},
		{"unknown wait failure", errors.New("wait failed"), nil},
		{"missing child process state", &exec.ExitError{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := userServiceResult(tc.run, tc.readback)
			var failure *PrivateRuntimeError
			if !errors.As(got, &failure) || failure.Kind != "uncertain" {
				t.Fatalf("unsettled lifecycle lost uncertainty: %v", got)
			}
		})
	}
	if got := userServiceResult(nil, nil); got != nil {
		t.Fatalf("settled successful unit: %v", got)
	}
}
