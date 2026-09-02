package update

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestCheckAllWithCooldown_ConcurrentReviewModeDisablePreservesMode(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	stale := now.Add(-7 * time.Hour)
	recordedAt := now.Add(-time.Hour)
	if err := state.Write(home, state.InstallState{
		LastUpdateCheck:   &stale,
		RDDMode:           "on",
		RDDModeRecordedAt: &recordedAt,
	}); err != nil {
		t.Fatalf("state.Write initial state: %v", err)
	}

	binary := buildCandidateBinary(t)
	persistReached := make(chan struct{}, 1)
	releasePersist := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releasePersist) }) }
	t.Cleanup(release)
	actorADone := make(chan struct{})

	go func() {
		defer close(actorADone)
		checkAllWithCooldown(
			context.Background(),
			"1.0.0",
			system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			home,
			6*time.Hour,
			func() time.Time { return now },
			func(context.Context, string, system.PlatformProfile) []UpdateResult {
				return []UpdateResult{{Status: UpToDate}}
			},
			func(homeDir string, timestamp time.Time) error {
				persistReached <- struct{}{}
				<-releasePersist
				return persistLastUpdateCheck(homeDir, timestamp)
			},
		)
	}()

	select {
	case <-persistReached:
	case <-time.After(10 * time.Second):
		t.Fatal("actor A did not reach timestamp persistence before actor B disables review mode")
	}

	actorB := exec.Command(binary, "review", "mode", "disable", "--scope", "global")
	actorB.Dir = repositoryRoot(t)
	actorB.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	if output, err := actorB.CombinedOutput(); err != nil {
		t.Fatalf("actor B review mode disable failed: %v\n%s", err, output)
	}

	intermediate, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read after actor B: %v", err)
	}
	if intermediate.RDDMode != "off" {
		t.Fatalf("actor B did not persist disabled review mode while actor A was blocked: got %q, want off", intermediate.RDDMode)
	}
	if intermediate.LastUpdateCheck == nil || !intermediate.LastUpdateCheck.Equal(stale) {
		t.Fatalf("actor B changed the timestamp while disabling review mode: got %v, want %v", intermediate.LastUpdateCheck, stale)
	}

	release()
	select {
	case <-actorADone:
	case <-time.After(10 * time.Second):
		t.Fatal("actor A did not complete after its write boundary was released")
	}

	final, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read after actor A: %v", err)
	}
	if final.LastUpdateCheck == nil || !final.LastUpdateCheck.Equal(now) {
		t.Fatalf("last_update_check = %v, want %v after actor A writes", final.LastUpdateCheck, now)
	}
	if final.RDDMode != "off" {
		t.Fatalf("stale cooldown write reverted actor B's persisted disabled review mode: got %q, want off", final.RDDMode)
	}
}

func buildCandidateBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "gentle-ai")
	command := exec.Command("go", "build", "-o", binary, "./cmd/gentle-ai")
	command.Dir = repositoryRoot(t)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build candidate gentle-ai binary: %v\n%s", err, output)
	}
	return binary
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	command := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("discover repository root: %v", err)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		t.Fatal("discover repository root returned an empty path")
	}
	return root
}
