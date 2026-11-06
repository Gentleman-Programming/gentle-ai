package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestShellInstallSelectionExecutionRechecksConsent(t *testing.T) {
	oldInspect, oldEntry := shellInstallInspect, shellInstallEntry
	t.Cleanup(func() { shellInstallInspect, shellInstallEntry = oldInspect, oldEntry })
	selection, _ := ShellInstallOutcome(NewShellInstallModel(nil))
	selection.Request.Destination = "/owned/model-fixture"
	inspected, entered := 0, 0
	var executedContext context.Context
	output := &bytes.Buffer{}
	failure := errors.New("entry fixture failure")
	shellInstallInspect = func(req shellinstaller.UserInstallRequest) (string, error) {
		inspected++
		if req != selection.Request {
			t.Fatal("inspection lost reviewed request")
		}
		return "fresh-token", nil
	}
	shellInstallEntry = func(ctx context.Context, self string, args []string, input io.Reader, stdout, stderr io.Writer) error {
		entered++
		executedContext = ctx
		if ctx.Err() != nil || self == "" || input != os.Stdin || stdout != output || stderr != os.Stderr || !reflect.DeepEqual(args, ShellInstallArguments(selection)) {
			t.Fatal("execution lost signal context, executable, streams or platform protocol")
		}
		return failure
	}
	if err := runShellInstallSelectionWithEntry(selection, output, shellInstallEntry); err != nil || inspected != 0 || entered != 0 {
		t.Fatal("unconfirmed selection has execution effects")
	}
	selection.Confirmed, selection.Request.Confirmation = true, "stale-token"
	if err := runShellInstallSelectionWithEntry(selection, output, shellInstallEntry); err == nil || entered != 0 {
		t.Fatal("stale confirmation starts the worker")
	}
	selection.Request.Confirmation = "fresh-token"
	if err := runShellInstallSelectionWithEntry(selection, output, shellInstallEntry); !errors.Is(err, failure) || entered != 1 || inspected != 2 {
		t.Fatalf("guarded execution = %v, inspected=%d entered=%d", err, inspected, entered)
	}
	if executedContext.Err() != context.Canceled {
		t.Fatal("execution left its signal context running")
	}
}
