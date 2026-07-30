//go:build windows

package cli

import (
	"os/exec"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sysproc"
)

func TestExecuteCommandAppliesConsolePolicyAtConstruction(t *testing.T) {
	for _, tt := range []struct {
		name     string
		streamed bool
		hidden   bool
	}{
		{name: "captured command is hidden", hidden: true},
		{name: "streamed command preserves console", streamed: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			originalFactory := executeCommandFactory
			originalStreaming := streamCommandOutput
			t.Cleanup(func() {
				executeCommandFactory = originalFactory
				streamCommandOutput = originalStreaming
			})
			var command *exec.Cmd
			executeCommandFactory = func(string, ...string) *exec.Cmd {
				command = exec.Command("cmd.exe", "/c", "exit", "/b", "0")
				return command
			}
			streamCommandOutput = tt.streamed

			if err := executeCommand("ignored"); err != nil {
				t.Fatal(err)
			}
			hidden := command.SysProcAttr != nil && command.SysProcAttr.CreationFlags&sysproc.CREATE_NO_WINDOW != 0
			if hidden != tt.hidden {
				t.Fatalf("CREATE_NO_WINDOW present = %t, want %t", hidden, tt.hidden)
			}
		})
	}
}
