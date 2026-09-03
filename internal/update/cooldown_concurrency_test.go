package update

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	persistReached := make(chan struct{}, 1)
	releasePersist := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releasePersist) }) }
	t.Cleanup(release)
	actorADone := make(chan struct{})

	go func() {
		defer close(actorADone)
		checkAllWithCooldown(
			ctx,
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
				select {
				case <-releasePersist:
					return persistLastUpdateCheck(homeDir, timestamp)
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		)
	}()

	awaitSignal(t, ctx, persistReached, "actor A did not reach timestamp persistence before actor B disables review mode")

	actorB := exec.CommandContext(ctx, binary, "review", "mode", "disable", "--scope", "global")
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
	awaitSignal(t, ctx, actorADone, "actor A did not complete after its write boundary was released")

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

func TestCheckAllWithCooldown_ContendedPersistenceRejectsReviewModeDisable(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	stale := now.Add(-7 * time.Hour)
	recordedAt := now.Add(-time.Hour)
	initial := state.InstallState{
		LastUpdateCheck:   &stale,
		RDDMode:           "on",
		RDDModeRecordedAt: &recordedAt,
	}
	if err := state.Write(home, initial); err != nil {
		t.Fatalf("state.Write initial state: %v", err)
	}

	binary := buildCandidateBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	writerEntered := make(chan struct{}, 1)
	releaseWriter := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWriter) }) }
	t.Cleanup(release)
	actorADone := make(chan struct{})

	go func() {
		defer close(actorADone)
		checkAllWithCooldown(
			ctx,
			"1.0.0",
			system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			home,
			6*time.Hour,
			func() time.Time { return now },
			func(context.Context, string, system.PlatformProfile) []UpdateResult {
				return []UpdateResult{{Status: UpToDate}}
			},
			func(homeDir string, timestamp time.Time) error {
				return persistLastUpdateCheckWithWriter(homeDir, timestamp, func(homeDir string, updated state.InstallState) error {
					writerEntered <- struct{}{}
					select {
					case <-releaseWriter:
						return state.WriteReconciled(homeDir, updated)
					case <-ctx.Done():
						return ctx.Err()
					}
				})
			},
		)
	}()

	awaitSignal(t, ctx, writerEntered, "actor A did not hold the install-state lock after updating the timestamp")
	before, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read before contended actor B: %v", err)
	}

	actorBContext, cancelActorB := context.WithTimeout(ctx, 5*time.Second)
	defer cancelActorB()
	actorB := exec.CommandContext(actorBContext, binary, "review", "mode", "disable", "--scope", "global")
	actorB.Dir = repositoryRoot(t)
	actorB.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	if output, err := actorB.CombinedOutput(); err == nil {
		t.Fatalf("actor B unexpectedly disabled review mode while cooldown held the lock: %s", output)
	}

	afterAttempt, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read after contended actor B: %v", err)
	}
	if !reflect.DeepEqual(before, afterAttempt) {
		t.Fatalf("contended actor B changed persisted state: before %#v, after %#v", before, afterAttempt)
	}

	release()
	awaitSignal(t, ctx, actorADone, "actor A did not complete after its write boundary was released")

	afterActorA, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read after actor A: %v", err)
	}
	if afterActorA.RDDMode != "on" || afterActorA.LastUpdateCheck == nil || !afterActorA.LastUpdateCheck.Equal(now) {
		t.Fatalf("actor A state = %#v, want mode on with timestamp %v", afterActorA, now)
	}

	retryContext, cancelRetry := context.WithTimeout(ctx, 5*time.Second)
	defer cancelRetry()
	retry := exec.CommandContext(retryContext, binary, "review", "mode", "disable", "--scope", "global")
	retry.Dir = repositoryRoot(t)
	retry.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	if output, err := retry.CombinedOutput(); err != nil {
		t.Fatalf("actor B retry review mode disable failed: %v\n%s", err, output)
	}

	final, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read after actor B retry: %v", err)
	}
	if final.RDDMode != "off" || final.LastUpdateCheck == nil || !final.LastUpdateCheck.Equal(now) {
		t.Fatalf("final state = %#v, want mode off with timestamp %v", final, now)
	}
}

func awaitSignal(t *testing.T, ctx context.Context, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("%s: %v", message, ctx.Err())
	}
}

func buildCandidateBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "gentle-ai")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/gentle-ai")
	command.Dir = repositoryRoot(t)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build candidate gentle-ai binary: %v\n%s", err, output)
	}
	return binary
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("discover repository root from runtime caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
}
