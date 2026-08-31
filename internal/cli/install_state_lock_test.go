package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

func TestPersistSyncManagedAssetStateReReadsLatestStateAfterLockContention(t *testing.T) {
	home := t.TempDir()
	held, err := reviewtransaction.AcquireAuthorityFileLock(mustInstallStateLockPath(t, home))
	if err != nil {
		t.Fatal(err)
	}
	if err := persistSyncManagedAssetStateWithBackground(home, model.Selection{}, "sha256:current-writer", "", ""); err == nil || !strings.Contains(err.Error(), "acquire install state lock") {
		t.Fatalf("contended sync state persistence error = %v", err)
	}
	if err := state.Write(home, state.InstallState{Persona: "concurrent"}); err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if err := persistSyncManagedAssetStateWithBackground(home, model.Selection{CommunityTools: []model.CommunityToolID{model.CommunityToolCodeGraph}}, "sha256:current-writer", "", ""); err != nil {
		t.Fatal(err)
	}
	got, err := state.Read(home)
	if err != nil || got.Persona != "concurrent" || got.ManagedAssetDigest != "sha256:current-writer" || !got.CommunityToolsConfigured || len(got.CommunityTools) != 1 || got.CommunityTools[0] != string(model.CommunityToolCodeGraph) {
		t.Fatalf("persisted sync state = %#v, err = %v", got, err)
	}
}

func TestPersistSyncManagedAssetStateRefusesCorruptLatestState(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.Path(home), []byte("{not valid json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := persistSyncManagedAssetStateWithBackground(home, model.Selection{}, "sha256:current-writer", "", ""); err == nil || !strings.Contains(err.Error(), "run `gentle-ai install`") {
		t.Fatalf("corrupt sync state persistence error = %v", err)
	}
}

func TestPersistInstallStateMergesExplicitAgentsFromLatestState(t *testing.T) {
	home := t.TempDir()
	held, err := reviewtransaction.AcquireAuthorityFileLock(mustInstallStateLockPath(t, home))
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"opencode"}, Persona: "concurrent"}); err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if err := persistInstallState(home, state.InstallState{InstalledAgents: []string{"codex"}}, []string{"codex"}, InstallFlags{Agents: []string{"codex"}}, "sha256:current-writer"); err != nil {
		t.Fatal(err)
	}
	got, err := state.Read(home)
	if err != nil || len(got.InstalledAgents) != 2 || got.InstalledAgents[0] != "opencode" || got.InstalledAgents[1] != "codex" || got.Persona != "concurrent" || got.ManagedAssetDigest != "sha256:current-writer" {
		t.Fatalf("persisted install state = %#v, err = %v", got, err)
	}
}

// TestInstallStateLockAcceptsSymlinkedHome is the RED-first proof for #3926:
// `review mode enable --scope global` on a virgin home whose path crosses a
// symlink (macOS `/var` -> `/private/var`, a `mktemp -d` HOME) used to fail
// with "acquire install state lock: ... not a directory" because the
// O_NOFOLLOW component walk was handed the unresolved path. The lock must be
// taken on the resolved real path so the no-follow property still holds and
// the state lands under the real directory.
func TestInstallStateLockAcceptsSymlinkedHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory symlinks need elevated privileges on Windows")
	}
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", link)
	t.Setenv("USERPROFILE", link)

	var output bytes.Buffer
	if err := RunReviewMode([]string{"enable", "--cwd", t.TempDir(), "--scope", "global", "--json"}, &output); err != nil {
		t.Fatalf("global enable under a symlinked home: %v\n%s", err, output.String())
	}
	persisted, err := state.Read(real)
	if err != nil {
		t.Fatalf("read state under the real home: %v", err)
	}
	if persisted.RDDMode != string(reviewtransaction.RDDModeOn) || persisted.RDDModeRecordedAt == nil {
		t.Fatalf("symlinked-home enable did not persist under the real home: %#v", persisted)
	}
}

func mustInstallStateLockPath(t *testing.T, home string) string {
	t.Helper()
	lockPath, err := installStateLockPath(home)
	if err != nil {
		t.Fatalf("install state lock path: %v", err)
	}
	return lockPath
}

// The lock is a function of the resolved home alone, not of filesystem state.
func TestInstallStateLockPathIsStableAcrossStateDirectoryCreation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory symlinks need elevated privileges on Windows")
	}
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	before := mustInstallStateLockPath(t, link)
	if err := os.MkdirAll(filepath.Dir(state.Path(real)), 0o755); err != nil {
		t.Fatal(err)
	}
	if after := mustInstallStateLockPath(t, link); after != before || after != state.Path(real)+".lock" {
		t.Fatalf("lock path depends on filesystem state: before %s, after %s", before, after)
	}
	if _, err := installStateLockPath(filepath.Join(real, "missing-home")); err == nil {
		t.Fatal("a home that does not resolve must fail closed instead of guessing a lock path")
	}
}
