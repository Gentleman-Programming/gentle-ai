package cli

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// A Main SDK failure whose diagnostic outgrows the bounded worker log must
// still reach the caller as a Main SDK failure, so the Main fallback is
// offered; an unrelated long failure must not be flagged.
func TestRunShellInstallInlineFlagsMainSDKFailureBeyondLogLimit(t *testing.T) {
	diag := strings.Repeat("npm ERR! x\n", 1000)
	mainSDK := "Error: Main SDK overlay: Windows provision overlay failed: exit status 1; stderr=" + strconv.Quote(diag) + "\n"
	unrelated := "Error: stock provider failed; stderr=" + strconv.Quote(diag) + "\n"
	for _, direct := range []bool{true, false} {
		for _, tc := range []struct {
			name   string
			output string
			want   bool
		}{{"main sdk", diag + mainSDK, true}, {"unrelated", diag + unrelated, false}} {
			t.Run(tc.name+map[bool]string{true: "/direct", false: "/delegated"}[direct], func(t *testing.T) {
				oldDirect, oldWorker, oldProbe, oldInspect, oldEntry := shellInlineDirect, shellDirectWorker, shellLaunchProbe, shellInstallInspect, shellInstallEntry
				t.Cleanup(func() {
					shellInlineDirect, shellDirectWorker, shellLaunchProbe, shellInstallInspect, shellInstallEntry = oldDirect, oldWorker, oldProbe, oldInspect, oldEntry
				})
				shellInlineDirect = direct
				shellInstallInspect = func(shellinstaller.UserInstallRequest) (string, error) { return "token", nil }
				shellLaunchProbe = func(context.Context, string) error { return nil }
				failure := errors.New("exit status 1")
				shellDirectWorker = func(_ context.Context, _ string, _ []string, _, stderr io.Writer) error {
					_, _ = io.WriteString(stderr, tc.output)
					return failure
				}
				shellInstallEntry = func(_ context.Context, _ string, _ []string, _ io.Reader, _, stderr io.Writer) error {
					_, _ = io.WriteString(stderr, tc.output)
					return failure
				}
				selection := ShellInstallSelection{Request: shellinstaller.UserInstallRequest{Destination: t.TempDir(), Mode: "separate", Confirmation: "token"}, Confirmed: true, Executed: true}
				err := RunShellInstallInline(context.Background(), selection, nil, io.Discard)
				if err == nil {
					t.Fatal("expected the worker failure")
				}
				got := errors.Is(err, shellinstaller.ErrMainSDKInstall) || strings.Contains(err.Error(), "Main SDK")
				if got != tc.want {
					t.Fatalf("Main SDK flagged = %v, want %v (error tail %q)", got, tc.want, err.Error()[max(0, len(err.Error())-80):])
				}
				if !errors.Is(err, failure) {
					t.Fatalf("original cause lost: %v", err)
				}
			})
		}
	}
}
