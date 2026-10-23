package shellinstaller

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
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

func TestUserServiceResultNamesNamespaceSetupFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	childErr := exec.CommandContext(ctx, "/bin/sh", "-c", "exit 226").Run()
	var child *exec.ExitError
	if !errors.As(childErr, &child) || child.ExitCode() != 226 {
		t.Fatalf("invalid namespace fixture: %v", childErr)
	}
	got := userServiceResult(childErr, nil)
	var failure *PrivateRuntimeError
	if !errors.As(got, &failure) || failure.Kind != "refused" || !errors.Is(got, childErr) {
		t.Fatalf("namespace setup failure not named: %v", got)
	}
	for _, fragment := range []string{"exit status 226", "user namespace", "apparmor_restrict_unprivileged_userns"} {
		if !strings.Contains(got.Error(), fragment) {
			t.Fatalf("namespace guidance missing %q: %v", fragment, got)
		}
	}
	readbackErr := errors.New("unit still populated")
	joined := userServiceResult(childErr, readbackErr)
	if !errors.Is(joined, readbackErr) || !strings.Contains(joined.Error(), "exit status 226") {
		t.Fatalf("readback precedence lost the named cause: %v", joined)
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
