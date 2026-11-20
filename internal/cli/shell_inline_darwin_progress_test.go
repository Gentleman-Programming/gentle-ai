//go:build darwin

package cli

import (
	"context"
	"io"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// The in-process darwin installer has no worker stdout to parse: the inline
// report must travel in the context so the publish step reaches the TUI before
// the rename (#6177).
func TestShellInlineDarwinAttachesProgressReporterToContext(t *testing.T) {
	oldInspect, oldEntry, oldAttach := shellInstallInspect, shellInstallEntry, shellInlineProgressContext
	t.Cleanup(func() {
		shellInstallInspect, shellInstallEntry, shellInlineProgressContext = oldInspect, oldEntry, oldAttach
	})
	shellInstallInspect = func(req shellinstaller.UserInstallRequest) (string, error) { return req.Confirmation, nil }
	type marker struct{}
	attached := 0
	shellInlineProgressContext = func(ctx context.Context, report func(shellinstaller.UserInstallProgress)) context.Context {
		if report == nil {
			t.Fatal("nil reporter attached")
		}
		attached++
		return context.WithValue(ctx, marker{}, true)
	}
	var entryCtx context.Context
	shellInstallEntry = func(ctx context.Context, _ string, _ []string, _ io.Reader, _, _ io.Writer) error {
		entryCtx = ctx
		return nil
	}
	selection := ShellInstallSelection{Confirmed: true, Request: shellinstaller.UserInstallRequest{Destination: t.TempDir() + "/dest", Mode: "separate", Confirmation: "x"}}
	if err := RunShellInstallInline(context.Background(), selection, func(shellinstaller.UserInstallProgress) {}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if attached != 1 || entryCtx == nil || entryCtx.Value(marker{}) == nil {
		t.Fatalf("entry did not receive the reporter context: attached=%d", attached)
	}
}
