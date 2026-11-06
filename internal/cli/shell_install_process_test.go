package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestShellInstallProcessSelectionConsent(t *testing.T) {
	old := shellInstallInspect
	t.Cleanup(func() { shellInstallInspect = old })
	inspected := 0
	failure := errors.New("physical selection unavailable")
	shellInstallInspect = func(shellinstaller.UserInstallRequest) (string, error) { inspected++; return "", failure }
	var output bytes.Buffer
	if err := RunShellInstallSelectionProcess(ShellInstallSelection{}, &output); err != nil || inspected != 0 || output.Len() != 0 {
		t.Fatal("canceled selection was inspected", err)
	}
	selection := ShellInstallSelection{Confirmed: true}
	if err := RunShellInstallSelectionProcess(selection, &output); !errors.Is(err, failure) || inspected != 1 || output.Len() != 0 {
		t.Fatal("inspection failure was lost", err)
	}
	shellInstallInspect = func(shellinstaller.UserInstallRequest) (string, error) { return "fresh", nil }
	selection.Request.Confirmation = "stale"
	if err := RunShellInstallSelectionProcess(selection, &output); err == nil || output.Len() != 0 {
		t.Fatal("stale selection was accepted", err)
	}
}
