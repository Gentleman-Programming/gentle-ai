//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A cancel that arrives after the publication commit point must not turn a
// published installation into a failure (#6177); before it, the cancel stays
// a failure.
func TestUserFinishIgnoresCancelAfterCommit(t *testing.T) {
	for _, committed := range []bool{true, false} {
		parent, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(parent, 0700); err != nil {
			t.Fatal(err)
		}
		workspace, err := os.MkdirTemp(parent, ".user-test-")
		if err != nil {
			t.Fatal(err)
		}
		identity, err := privateDirectory(workspace)
		if err != nil {
			t.Fatal(err)
		}
		ctx, mark := userWithPublicationCommit(context.Background())
		*mark = committed
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		_, err = userFinish(cancelled, UserInstallResult{}, nil, workspace, identity, filepath.Join(parent, "dest"))
		if committed && err != nil {
			t.Fatalf("published installation reported a failure after a late cancel: %v", err)
		}
		if !committed && err == nil {
			t.Fatal("a cancel before the commit point was swallowed")
		}
	}
}

// The inline context reporter must deliver the publish step to the caller.
func TestWithInstallProgressReportsPublishStep(t *testing.T) {
	var got []UserInstallProgress
	ctx := WithInstallProgress(context.Background(), func(p UserInstallProgress) { got = append(got, p) })
	userShellStepBegin(ctx, 5, 5, "Publish")
	userShellDetail(ctx, 5, 5, "Publishing")
	if len(got) != 2 || got[0].Index != 5 || got[0].State != "running" || got[1].State != "detail" {
		t.Fatalf("reports = %+v", got)
	}
}
