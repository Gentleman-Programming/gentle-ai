//go:build windows

package tui

import (
	"fmt"
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/testsupport/windowsrunner"
)

// The Gentle Shell flows below run the real Windows inspection, so the test
// process must look like an unelevated Windows 11 account:
//   - windowsrunner owns new objects by the user SID and expands the 8.3 temp
//     alias (the elevated GitHub runner otherwise yields "foreign Windows
//     owner" refusals);
//   - LOCALAPPDATA, the Windows install parent, points at a private temp
//     folder so no test reads or writes the runner profile; managed-install
//     fixtures override it per test;
//   - only when the real host qualification refuses this host (Windows
//     Server or an elevated runner) is UserKernelGate stubbed, so a
//     qualified Windows 11 developer host keeps the real gate.
//
// Product code is unchanged.
func TestMain(m *testing.M) {
	windowsrunner.Prepare("tui")
	local, err := os.MkdirTemp("", "gentle-ai-tui-localappdata-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "tui tests: cannot create a private LOCALAPPDATA: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("LOCALAPPDATA", local); err != nil {
		fmt.Fprintf(os.Stderr, "tui tests: cannot set LOCALAPPDATA: %v\n", err)
		os.Exit(1)
	}
	if err := shellinstaller.UserKernelCheck(); err != nil {
		fmt.Fprintf(os.Stderr, "tui tests: host is not a qualified Gentle Shell target (%v); stubbing the kernel gate\n", err)
		shellinstaller.UserKernelGate = func() error { return nil }
	}
	code := m.Run()
	_ = os.RemoveAll(local)
	os.Exit(code)
}
