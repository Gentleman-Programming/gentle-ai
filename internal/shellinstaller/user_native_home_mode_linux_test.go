//go:build linux && amd64

package shellinstaller

import (
	"context"
	"os"
	"testing"
)

func TestPrivateNativeDirectoryAcceptsGroupReadOnlyHome(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0750); err != nil {
		t.Fatal(err)
	}
	if err := privateNativeHome(home); err != nil {
		t.Fatalf("0750 home must stay accepted: %v", err)
	}
	if err := privateNativeDirectory(home); err == nil {
		t.Fatal("owned directories must keep rejecting 0750")
	}
	if err := os.Chmod(home, 0710); err != nil {
		t.Fatal(err)
	}
	if err := privateNativeHome(home); err == nil {
		t.Fatal("group-write-capable home must be refused")
	}
}

func TestUserProgressEnvironmentCrossesServiceHops(t *testing.T) {
	t.Setenv("GENTLE_AI_SHELL_STEPS", "")
	ctx := withUserInstallProgressForTest(context.Background(), func(UserInstallProgress) {})
	if got := userProgressEnvironment(ctx); len(got) != 1 || got[0] != "GENTLE_AI_SHELL_STEPS=1" {
		t.Fatalf("observer context env = %v", got)
	}
	if got := userProgressEnvironment(context.Background()); got != nil {
		t.Fatalf("plain context env = %v", got)
	}
	t.Setenv("GENTLE_AI_SHELL_STEPS", "1")
	if got := userProgressEnvironment(context.Background()); len(got) != 1 {
		t.Fatalf("inherited service env = %v", got)
	}
}

func TestUserShellProgressReportsOrderedStates(t *testing.T) {
	var got []UserInstallProgress
	ctx := withUserInstallProgressForTest(context.Background(), func(progress UserInstallProgress) {
		got = append(got, progress)
	})
	for index := 1; index <= 5; index++ {
		userShellStepBegin(ctx, index, 5, "step")
		userShellStep(ctx, index, 5, "step")
	}
	if len(got) != 10 {
		t.Fatalf("progress count = %d, want 10", len(got))
	}
	for index, progress := range got {
		wantIndex := index/2 + 1
		if progress.Index != wantIndex || progress.Total != 5 {
			t.Fatalf("progress[%d] = %+v", index, progress)
		}
		wantState := "running"
		if index%2 == 1 {
			wantState = "done"
		}
		if progress.State != wantState {
			t.Fatalf("progress[%d] state = %q, want %q", index, progress.State, wantState)
		}
	}
}
