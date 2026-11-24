//go:build darwin

package cli

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/shellinstaller"
)

// On darwin the installer runs in-process: the inline parent must not turn on
// the step output, which would be printed over the Bubble Tea screen.
func TestShellInlineDarwinWritesNothingToStdout(t *testing.T) {
	oldInspect := shellInstallInspect
	t.Cleanup(func() { shellInstallInspect = oldInspect })
	shellInstallInspect = func(req shellinstaller.UserInstallRequest) (string, error) { return req.Confirmation, nil }
	t.Setenv("GENTLE_AI_SHELL_STEPS", "")
	_ = os.Unsetenv("GENTLE_AI_SHELL_STEPS")

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	selection := ShellInstallSelection{Confirmed: true, Request: shellinstaller.UserInstallRequest{Destination: t.TempDir() + "/dest", Mode: "separate", Confirmation: "x"}}
	_ = RunShellInstallInline(context.Background(), selection, func(shellinstaller.UserInstallProgress) {}, io.Discard)
	os.Stdout = oldStdout
	_ = writer.Close()
	out, _ := io.ReadAll(reader)
	_ = reader.Close()
	if len(out) != 0 {
		t.Fatalf("inline install wrote to stdout: %q", out)
	}
}
