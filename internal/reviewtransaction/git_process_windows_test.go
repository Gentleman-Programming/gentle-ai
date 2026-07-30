//go:build windows

package reviewtransaction

import (
	"bytes"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestStartGitProcessTreeComposesCreationAttributesAtStart(t *testing.T) {
	for _, tt := range []struct {
		name         string
		attributes   *syscall.SysProcAttr
		wantNoWindow bool
	}{
		{name: "nil attributes"},
		{
			name:         "prepopulated attributes",
			attributes:   &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true},
			wantNoWindow: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command := exec.Command("cmd.exe", "/c", "echo", "resumed-output")
			command.SysProcAttr = tt.attributes
			original := command.SysProcAttr
			var output bytes.Buffer
			command.Stdout = &output

			release, err := startGitProcessTree(command)
			if err != nil {
				t.Fatal(err)
			}
			if original != nil && command.SysProcAttr != original {
				t.Fatal("SysProcAttr pointer identity changed")
			}
			if command.SysProcAttr == nil || command.SysProcAttr.CreationFlags&windows.CREATE_SUSPENDED == 0 {
				t.Fatal("CREATE_SUSPENDED did not reach the start boundary")
			}
			if got := command.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW != 0; got != tt.wantNoWindow {
				t.Fatalf("CREATE_NO_WINDOW preserved = %t, want %t", got, tt.wantNoWindow)
			}
			if original != nil && !command.SysProcAttr.HideWindow {
				t.Fatal("unrelated SysProcAttr field was not preserved")
			}
			if err := command.Wait(); err != nil {
				t.Fatal(err)
			}
			if err := release(); err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(output.String()) != "resumed-output" {
				t.Fatalf("resumed output = %q", output.String())
			}
		})
	}
}
