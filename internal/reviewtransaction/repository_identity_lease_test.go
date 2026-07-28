package reviewtransaction

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRepositoryIdentityLeaseReopensStableIdentity(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	nested := filepath.Join(repo, "nested", "path")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	first, err := OpenRepositoryIdentityLease(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenRepositoryIdentityLease(context.Background(), nested)
	if err != nil {
		t.Fatal(err)
	}
	if first.Identity() != second.Identity() {
		t.Fatalf("reopened identity changed: first=%#v second=%#v", first.Identity(), second.Identity())
	}
	if first.StorageKey() != second.StorageKey() {
		t.Fatalf("reopened storage key changed: %q != %q", first.StorageKey(), second.StorageKey())
	}
	assertRepositoryIdentityStorageKey(t, first)
	if err := first.Validate(context.Background()); err != nil {
		t.Fatalf("first.Validate() error = %v", err)
	}
	if err := second.Validate(context.Background()); err != nil {
		t.Fatalf("second.Validate() error = %v", err)
	}
}

func TestRepositoryIdentityLeaseSeparatesLinkedWorktrees(t *testing.T) {
	primary, linked := initRepositoryIdentityLeaseWorktree(t)

	mainLease, err := OpenRepositoryIdentityLease(context.Background(), primary)
	if err != nil {
		t.Fatal(err)
	}
	linkedLease, err := OpenRepositoryIdentityLease(context.Background(), linked)
	if err != nil {
		t.Fatal(err)
	}
	mainIdentity, linkedIdentity := mainLease.Identity(), linkedLease.Identity()
	if mainIdentity.GitCommonDir != linkedIdentity.GitCommonDir {
		t.Fatalf("linked worktrees do not share common dir: %q != %q", mainIdentity.GitCommonDir, linkedIdentity.GitCommonDir)
	}
	if mainIdentity.RepositoryRoot == linkedIdentity.RepositoryRoot ||
		mainIdentity.GitDir == linkedIdentity.GitDir ||
		mainIdentity.RepositoryRef == linkedIdentity.RepositoryRef ||
		mainLease.StorageKey() == linkedLease.StorageKey() {
		t.Fatalf("linked worktree identities were not isolated: main=%#v linked=%#v", mainIdentity, linkedIdentity)
	}
	assertRepositoryIdentityStorageKey(t, mainLease)
	assertRepositoryIdentityStorageKey(t, linkedLease)
}

func TestRepositoryIdentityLeaseRejectsGitDirectoryReplacement(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	lease, err := OpenRepositoryIdentityLease(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(repo, ".git"), filepath.Join(repo, ".git-before-lease-swap")); err != nil {
		t.Fatal(err)
	}
	if err := runSnapshotGit(repo, "init"); err != nil {
		t.Fatal(err)
	}

	if err := lease.Validate(context.Background()); !errors.Is(err, ErrRepositoryIdentityChanged) {
		t.Fatalf("Validate() error = %v, want ErrRepositoryIdentityChanged", err)
	}
}

func TestRepositoryIdentityLeaseRejectsGitControlReplacement(t *testing.T) {
	_, linked := initRepositoryIdentityLeaseWorktree(t)
	lease, err := OpenRepositoryIdentityLease(context.Background(), linked)
	if err != nil {
		t.Fatal(err)
	}
	controlPath := filepath.Join(linked, ".git")
	payload, err := os.ReadFile(controlPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(controlPath)
	if err != nil {
		t.Fatal(err)
	}
	replacementPath := controlPath + ".replacement"
	if err := os.WriteFile(replacementPath, payload, info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacementPath, controlPath); err != nil {
		// Windows does not replace an occupied destination with os.Rename.
		// The replacement inode/file-ID was allocated before removing the
		// original, so this fallback cannot recycle the captured identity.
		if removeErr := os.Remove(controlPath); removeErr != nil {
			t.Fatal(removeErr)
		}
		if renameErr := os.Rename(replacementPath, controlPath); renameErr != nil {
			t.Fatal(renameErr)
		}
	}
	replaced, err := os.Stat(controlPath)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(info, replaced) {
		t.Fatal("control replacement fixture reused the captured file identity")
	}

	if err := lease.Validate(context.Background()); !errors.Is(err, ErrRepositoryIdentityChanged) {
		t.Fatalf("Validate() error = %v, want ErrRepositoryIdentityChanged", err)
	}
}

func TestRepositoryIdentityLeaseRejectsCommonControlContentMutation(t *testing.T) {
	_, linked := initRepositoryIdentityLeaseWorktree(t)
	lease, err := OpenRepositoryIdentityLease(context.Background(), linked)
	if err != nil {
		t.Fatal(err)
	}
	controlPath := filepath.Join(lease.Identity().GitDir, "commondir")
	info, err := os.Stat(controlPath)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the relationship semantically valid while changing its exact
	// control payload in place.
	payload := []byte(lease.Identity().GitCommonDir + "\n")
	if err := os.WriteFile(controlPath, payload, info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}

	if err := lease.Validate(context.Background()); !errors.Is(err, ErrRepositoryIdentityChanged) {
		t.Fatalf("Validate() error = %v, want ErrRepositoryIdentityChanged", err)
	}
}

func TestRepositoryIdentityLeaseOpenAndValidateAreReadOnly(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	storageRoot := filepath.Join(repo, ".git", "gentle-ai")
	if _, err := os.Lstat(storageRoot); !os.IsNotExist(err) {
		t.Fatalf("unexpected pre-existing authority storage: %v", err)
	}

	lease, err := OpenRepositoryIdentityLease(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(storageRoot); !os.IsNotExist(err) {
		t.Fatalf("read-only identity lease created authority storage: %v", err)
	}
}

func TestRepositoryIdentityLeaseSupportsConcurrentValidation(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	lease, err := OpenRepositoryIdentityLease(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 16
	start := make(chan struct{})
	errs := make(chan error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for index := 0; index < workers; index++ {
		go func() {
			defer group.Done()
			<-start
			errs <- lease.Validate(context.Background())
		}()
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent Validate() error = %v", err)
		}
	}
}

func initRepositoryIdentityLeaseWorktree(t *testing.T) (string, string) {
	t.Helper()
	requireSnapshotGit(t)
	primary := initSnapshotRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	if err := runSnapshotGit(primary, "worktree", "add", "-b", "identity-lease-linked", linked, "HEAD"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = runSnapshotGit(primary, "worktree", "remove", "--force", linked)
	})
	return primary, linked
}

func assertRepositoryIdentityStorageKey(t *testing.T, lease *RepositoryIdentityLease) {
	t.Helper()
	key := lease.StorageKey()
	if len(key) != 64 || key != strings.ToLower(key) {
		t.Fatalf("StorageKey() = %q, want 64 lowercase hex", key)
	}
	if _, err := hex.DecodeString(key); err != nil {
		t.Fatalf("StorageKey() = %q, want hex: %v", key, err)
	}
	if lease.Identity().RepositoryRef != "sha256:"+key {
		t.Fatalf("RepositoryRef = %q, StorageKey = %q", lease.Identity().RepositoryRef, key)
	}
}
