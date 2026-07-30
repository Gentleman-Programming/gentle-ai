package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestReviewRepositoryContextPublishesOpaquePrivateBinding(t *testing.T) {
	repo, binding := reviewRepositoryContextFixture(t, "repository-context")
	handle, err := PublishReviewRepositoryContext(context.Background(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	if !validReviewRepositoryContextHandle(handle) || strings.Contains(handle, repo) {
		t.Fatalf("handle %q is not opaque", handle)
	}
	resolved, err := ResolveReviewRepositoryContext(context.Background(), handle, binding)
	if err != nil || resolved != repo {
		t.Fatalf("resolved = %q, %v; want %q", resolved, err, repo)
	}

	path, err := reviewRepositoryContextPath(handle)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("context record = %v, %v", info, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("context record mode = %o, want 600", info.Mode().Perm())
	}
	if payload, err := os.ReadFile(path); err != nil || !strings.Contains(string(payload), repo) {
		t.Fatalf("private record does not retain the repository root: %v", err)
	}
}

func TestReviewRepositoryContextRejectsTamperedExpiredAndWrongBindings(t *testing.T) {
	repo, binding := reviewRepositoryContextFixture(t, "repository-context-bindings")
	handle, err := PublishReviewRepositoryContext(context.Background(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []ReviewRepositoryContextBinding{
		{LineageID: binding.LineageID, TargetIdentity: binding.TargetIdentity, Revision: "sha256:" + strings.Repeat("b", 64)},
		{LineageID: binding.LineageID, TargetIdentity: "sha256:" + strings.Repeat("b", 64), Revision: binding.Revision},
		{LineageID: "wrong-lineage", TargetIdentity: binding.TargetIdentity, Revision: binding.Revision},
	} {
		if _, err := ResolveReviewRepositoryContext(context.Background(), handle, wrong); err == nil {
			t.Fatalf("wrong binding resolved: %#v", wrong)
		}
	}

	path, err := reviewRepositoryContextPath(handle)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record reviewRepositoryContextFile
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	other := initSnapshotRepo(t)
	record.RepositoryRoot = other
	tampered, err := json.Marshal(record)
	if err != nil || os.WriteFile(path, append(tampered, '\n'), 0o600) != nil {
		t.Fatal(err)
	}
	if _, err := ResolveReviewRepositoryContext(context.Background(), handle, binding); err == nil {
		t.Fatal("tampered repository identity resolved")
	}

	// Restore the exact record, then advance authority. The old revision-bound
	// handle must expire without granting access to the new state.
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, binding.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	next := current.State
	if err := next.Invalidate("expire repository context"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(current.Revision, "review/invalidate", next); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveReviewRepositoryContext(context.Background(), handle, binding); err == nil {
		t.Fatal("expired repository context resolved after authority revision changed")
	}
}

func TestReviewRepositoryContextRejectsUnsafeStorageAndDoesNotTraverseAboveHome(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(home, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repo := initSnapshotRepo(t)
	record, _ := pristineReviewingFixture(t, repo, "repository-context-private")
	binding := ReviewRepositoryContextBinding{LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.Revision}
	handle, err := PublishReviewRepositoryContext(context.Background(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(home)
		if err != nil || info.Mode().Perm() != 0o755 {
			t.Fatalf("HOME mode changed above private root: %v, %v", info, err)
		}
	}
	if err := ensurePrivateLocatorDirectory(home, filepath.Join(home, "..", "escape")); err == nil {
		t.Fatal("private directory helper accepted path outside HOME")
	}

	path, err := reviewRepositoryContextPath(handle)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repo, path); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveReviewRepositoryContext(context.Background(), handle, binding); err == nil {
		t.Fatal("symlinked private context record resolved")
	}
}

func TestReviewRepositoryContextRejectsMissingMalformedOversizedAndTerminalRecords(t *testing.T) {
	repo, binding := reviewRepositoryContextFixture(t, "repository-context-invalid-records")
	handle, err := DeriveReviewRepositoryContextHandle(context.Background(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveReviewRepositoryContext(context.Background(), handle, binding); err == nil {
		t.Fatal("missing repository context record resolved")
	}
	if _, err := PublishReviewRepositoryContext(context.Background(), repo, binding); err != nil {
		t.Fatal(err)
	}
	path, err := reviewRepositoryContextPath(handle)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema":"gentle-ai.review-repository-context/v1","unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveReviewRepositoryContext(context.Background(), handle, binding); err == nil {
		t.Fatal("malformed repository context record resolved")
	}
	if err := os.WriteFile(path, make([]byte, reviewRepositoryLocatorMaxBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveReviewRepositoryContext(context.Background(), handle, binding); err == nil {
		t.Fatal("oversized repository context record resolved")
	}
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, binding.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	terminal := current.State
	if err := terminal.Invalidate("terminal context must expire"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(current.Revision, "review/invalidate", terminal); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveReviewRepositoryContext(context.Background(), handle, binding); err == nil {
		t.Fatal("terminal repository context record resolved")
	}
}

func TestReviewRepositoryContextRejectsSymlinkedProviderDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires POSIX semantics")
	}
	repo, binding := reviewRepositoryContextFixture(t, "repository-context-symlink-directory")
	home, err := reviewRepositoryContextHome()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".gentle-ai")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repo, filepath.Join(root, "review-contexts")); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishReviewRepositoryContext(context.Background(), repo, binding); err == nil {
		t.Fatal("publication accepted a symlinked provider directory")
	}
}

func TestReviewRepositoryContextPublicationConvergesAndRejectsUnsafeEntries(t *testing.T) {
	repo, binding := reviewRepositoryContextFixture(t, "repository-context-concurrent")
	const publishers = 12
	handles := make(chan string, publishers)
	errs := make(chan error, publishers)
	var group sync.WaitGroup
	for range publishers {
		group.Add(1)
		go func() {
			defer group.Done()
			handle, err := PublishReviewRepositoryContext(context.Background(), repo, binding)
			handles <- handle
			errs <- err
		}()
	}
	group.Wait()
	close(handles)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent publication: %v", err)
		}
	}
	var handle string
	for candidate := range handles {
		if handle == "" {
			handle = candidate
		} else if candidate != handle {
			t.Fatalf("concurrent handles = %q and %q", handle, candidate)
		}
	}
	if resolved, err := ResolveReviewRepositoryContext(context.Background(), handle, binding); err != nil || resolved != repo {
		t.Fatalf("concurrent context resolved = %q, %v", resolved, err)
	}
	upperSuffix := reviewRepositoryContextHandlePrefix + strings.ToUpper(strings.TrimPrefix(handle, reviewRepositoryContextHandlePrefix))
	if ValidateReviewRepositoryContextHandle(upperSuffix) == nil {
		t.Fatal("non-canonical uppercase repository context handle was accepted")
	}

	path, err := reviewRepositoryContextPath(handle)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveReviewRepositoryContext(context.Background(), handle, binding); err == nil {
			t.Fatal("executable repository context record resolved")
		}
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("replace context with directory: %v", err)
	}
	if _, err := ResolveReviewRepositoryContext(context.Background(), handle, binding); err == nil {
		t.Fatal("directory context record resolved")
	}
}

func TestReviewRepositoryContextPublicationSyncFailureIsRetrySafe(t *testing.T) {
	repo, binding := reviewRepositoryContextFixture(t, "repository-context-sync-retry")
	originalSync := syncReviewDirectory
	t.Cleanup(func() { syncReviewDirectory = originalSync })
	var synced []string
	failPublishedDirectory := true
	syncReviewDirectory = func(path string) error {
		synced = append(synced, filepath.Clean(path))
		if failPublishedDirectory && strings.HasSuffix(filepath.Clean(path), filepath.Join("review-contexts", "v1")) {
			return errors.New("injected repository-context directory sync failure")
		}
		return nil
	}

	handle, err := PublishReviewRepositoryContext(context.Background(), repo, binding)
	if err == nil {
		t.Fatal("fresh repository-context publication ignored its directory sync failure")
	}
	if handle != "" {
		t.Fatalf("failed publication returned handle %q", handle)
	}
	failPublishedDirectory = false
	handle, err = PublishReviewRepositoryContext(context.Background(), repo, binding)
	if err != nil {
		t.Fatalf("retry after interrupted publication: %v", err)
	}
	path, err := reviewRepositoryContextPath(handle)
	if err != nil {
		t.Fatal(err)
	}
	if payload, err := readReviewRepositoryContext(path); err != nil || len(payload) == 0 {
		t.Fatalf("retry did not reopen and verify the immutable destination: %v", err)
	}
	home, err := reviewRepositoryContextHome()
	if err != nil {
		t.Fatal(err)
	}
	wantSyncs := []string{
		home,
		filepath.Join(home, ".gentle-ai"),
		filepath.Join(home, ".gentle-ai", "review-contexts"),
		filepath.Join(home, ".gentle-ai", "review-contexts", "v1"),
	}
	for _, want := range wantSyncs {
		if !slices.Contains(synced, filepath.Clean(want)) {
			t.Fatalf("publication never synced %q; calls = %v", want, synced)
		}
	}
}

func TestReviewRepositoryContextRejectsBroadProviderDirectoryWithoutChmod(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission assertion")
	}
	repo, binding := reviewRepositoryContextFixture(t, "repository-context-broad-directory")
	home, err := reviewRepositoryContextHome()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".gentle-ai")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	broad := filepath.Join(root, "review-contexts")
	if err := os.Mkdir(broad, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishReviewRepositoryContext(context.Background(), repo, binding); err == nil {
		t.Fatal("publication accepted a broadly accessible provider directory")
	}
	info, err := os.Stat(broad)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("publication changed broad directory mode: %v, %v", info, err)
	}
}

func TestReviewRepositoryContextDistinguishesLinkedWorktreesAndRejectsCommonDirMismatch(t *testing.T) {
	repo, binding := reviewRepositoryContextFixture(t, "repository-context-worktree")
	linked := filepath.Join(t.TempDir(), "linked")
	if err := runSnapshotGit(repo, "worktree", "add", "-b", "repository-context-linked", linked, "HEAD"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runSnapshotGit(repo, "worktree", "remove", "--force", linked) })
	mainHandle, err := DeriveReviewRepositoryContextHandle(context.Background(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	linkedHandle, err := DeriveReviewRepositoryContextHandle(context.Background(), linked, binding)
	if err != nil {
		t.Fatal(err)
	}
	if linkedHandle == mainHandle {
		t.Fatal("linked worktree reused the primary worktree repository identity")
	}

	if _, err := PublishReviewRepositoryContext(context.Background(), repo, binding); err != nil {
		t.Fatal(err)
	}
	other := initSnapshotRepo(t)
	fake := reviewRepositoryIdentityRecord{
		RepositoryRoot: repo,
		GitCommonDir:   filepath.Join(other, ".git"),
		GitDir:         filepath.Join(other, ".git"),
	}
	fake.RepositoryIdentity = reviewRepositoryIdentityHash(fake)
	fakeHandle := reviewRepositoryContextHandle(binding, fake)
	fakePath, err := reviewRepositoryContextPath(fakeHandle)
	if err != nil {
		t.Fatal(err)
	}
	record := reviewRepositoryContextFile{
		Schema: ReviewRepositoryContextSchema, Handle: fakeHandle,
		LineageID: binding.LineageID, TargetIdentity: binding.TargetIdentity, Revision: binding.Revision,
		RepositoryIdentity: fake.RepositoryIdentity, RepositoryRoot: fake.RepositoryRoot,
		GitCommonDir: fake.GitCommonDir, GitDir: fake.GitDir,
	}
	payload, err := json.Marshal(record)
	if err != nil || os.WriteFile(fakePath, append(payload, '\n'), 0o600) != nil {
		t.Fatal(err)
	}
	if _, err := ResolveReviewRepositoryContext(context.Background(), fakeHandle, binding); err == nil {
		t.Fatal("repository context with a mismatched Git common directory resolved")
	}
}

func TestReviewRepositoryContextHonorsCancellationAndHashesWindowsPaths(t *testing.T) {
	repo, binding := reviewRepositoryContextFixture(t, "repository-context-cancel")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PublishReviewRepositoryContext(ctx, repo, binding); err == nil {
		t.Fatal("cancelled publication succeeded")
	}
	handle, err := PublishReviewRepositoryContext(context.Background(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	resolveCtx, resolveCancel := context.WithCancel(context.Background())
	resolveCancel()
	if _, err := ResolveReviewRepositoryContext(resolveCtx, handle, binding); err == nil {
		t.Fatal("cancelled resolution succeeded")
	}
	identity := reviewRepositoryIdentityRecord{
		RepositoryRoot: `C:\\Users\\alice\\repo`,
		GitCommonDir:   `C:\\Users\\alice\\repo\\.git`,
		GitDir:         `C:\\Users\\alice\\repo\\.git`,
	}
	identity.RepositoryIdentity = reviewRepositoryIdentityHash(identity)
	windowsHandle := reviewRepositoryContextHandle(binding, identity)
	if !validReviewRepositoryContextHandle(windowsHandle) || strings.Contains(windowsHandle, `C:\\`) {
		t.Fatalf("Windows repository context is not opaque: %q", windowsHandle)
	}
}

func reviewRepositoryContextFixture(t *testing.T, lineage string) (string, ReviewRepositoryContextBinding) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nreviewed change\n")
	record, _ := pristineReviewingFixture(t, repo, lineage)
	return repo, ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.Revision,
	}
}
