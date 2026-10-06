package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestHistoricalRctx1ContextIsReadOnly(t *testing.T) {
	repo, binding := historicalReviewRepositoryContextFixture(t, "historical-rctx1")
	handle, err := DeriveHistoricalReviewRepositoryContextHandle(t.Context(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".gentle-ai", "review-contexts", "v1", handle+".json"))
	if err != nil {
		t.Fatal(err)
	}
	root, resolved, err := ResolveHistoricalReviewRepositoryContextBinding(t.Context(), handle)
	if err != nil || root != repo || resolved != binding {
		t.Fatalf("historical rctx1 resolution = %q, %#v, %v", root, resolved, err)
	}
	if _, err := ResolveReviewRepositoryContext(t.Context(), repo, handle, binding); err == nil {
		t.Fatal("current lifecycle resolver accepted historical rctx1")
	}
	after, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".gentle-ai", "review-contexts", "v1", handle+".json"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("historical rctx1 read changed locator bytes: %v", err)
	}
}

func TestRctx2HandleIsAnOpaqueDigestThatCarriesNoPath(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "rctx2-opaque")
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := ReviewRepositoryContextBinding{
		LineageID:      record.State.LineageID,
		TargetIdentity: record.State.InitialSnapshot.Identity,
		Revision:       record.State.CapturePhaseRevision,
	}
	handle, err := deriveReviewRepositoryContextV2Token(t.Context(), fixture.store.repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	// The capability is declared as review.opaque_repository_context and the
	// handle is relayed on command lines and through host logs, so a reader who
	// holds it must learn nothing about the filesystem it names.
	if !validReviewRepositoryContextV2Handle(handle) {
		t.Fatalf("rctx2 handle is not a canonical digest: %q", handle)
	}
	if len(handle) != len(reviewRepositoryContextV2HandlePrefix)+reviewRepositoryContextV2DigestBytes {
		t.Fatalf("rctx2 handle = %d bytes, want a fixed-width digest", len(handle))
	}
	identity := reviewRepositoryIdentityRecord{}
	if lease, leaseErr := OpenRepositoryIdentityLease(t.Context(), fixture.store.repo); leaseErr == nil {
		live := lease.Identity()
		identity = reviewRepositoryIdentityRecord{RepositoryRoot: live.RepositoryRoot, GitCommonDir: live.GitCommonDir, GitDir: live.GitDir}
	}
	for _, secret := range []string{identity.RepositoryRoot, identity.GitCommonDir, identity.GitDir, os.Getenv("HOME"), `C:\\`} {
		if secret == "" {
			continue
		}
		if strings.Contains(handle, secret) {
			t.Fatalf("rctx2 handle is not opaque: %q leaks %q", handle, secret)
		}
	}
	// A digest is only opaque if it cannot be decoded back into its preimage.
	// base64 and hex are the two shapes a reader would try first.
	if decoded, decodeErr := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(handle, reviewRepositoryContextV2HandlePrefix)); decodeErr == nil {
		if json.Valid(decoded) {
			t.Fatalf("rctx2 handle decodes to structured data: %s", decoded)
		}
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(handle, reviewRepositoryContextV2HandlePrefix))
	if err != nil || len(raw) != sha256.Size || json.Valid(raw) {
		t.Fatalf("rctx2 handle is not a bare sha256 digest: %q", handle)
	}
}

func TestRctx2HandleResolvesAgainstTheCallerRepositoryWithoutMutation(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "rctx2-round-trip")
	before, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := ReviewRepositoryContextBinding{
		LineageID:      before.State.LineageID,
		TargetIdentity: before.State.InitialSnapshot.Identity,
		Revision:       before.State.CapturePhaseRevision,
	}
	stateBefore, err := os.ReadFile(fixture.store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	handle, err := deriveReviewRepositoryContextV2Token(t.Context(), fixture.store.repo, binding)
	if err != nil {
		t.Fatal(err)
	}

	root, resolved, err := resolveReviewRepositoryContextV2Token(t.Context(), fixture.store.repo, handle, binding)
	if err != nil || root != fixture.store.repo || resolved != binding {
		t.Fatalf("initial rctx2 resolution = root %q, binding %#v, error %v", root, resolved, err)
	}
	stateAfterResolve, err := os.ReadFile(fixture.store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(stateAfterResolve) != string(stateBefore) {
		t.Fatal("rctx2 resolution mutated compact authority")
	}

	if _, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), fixture.request); err != nil {
		t.Fatal(err)
	}
	afterCapture, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterCapture.Revision == before.Revision || afterCapture.State.CapturePhaseRevision != binding.Revision {
		t.Fatalf("capture did not advance only Rn: before=%#v after=%#v", before, afterCapture)
	}
	root, resolved, err = resolveReviewRepositoryContextV2Token(t.Context(), fixture.store.repo, handle, binding)
	if err != nil || root != fixture.store.repo || resolved != binding {
		t.Fatalf("rctx2 resolution after sibling Rn advance = root %q, binding %#v, error %v", root, resolved, err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".gentle-ai", "review-contexts")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rctx2 core created a v1 locator: %v", err)
	}
}

func TestRctx2HandleRefusesTamperAndConfinementWithoutMutation(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "rctx2-refusals")
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := ReviewRepositoryContextBinding{
		LineageID:      record.State.LineageID,
		TargetIdentity: record.State.InitialSnapshot.Identity,
		Revision:       record.State.CapturePhaseRevision,
	}
	handle, err := deriveReviewRepositoryContextV2Token(t.Context(), fixture.store.repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	other := initSnapshotRepo(t)
	digest := strings.TrimPrefix(handle, reviewRepositoryContextV2HandlePrefix)

	// Every case supplies a repository and a binding the handle does not commit
	// to. The digest, not a self-reported path, is what has to catch them.
	for _, tt := range []struct {
		name    string
		repo    string
		handle  string
		binding ReviewRepositoryContextBinding
	}{
		{name: "unknown prefix", handle: "rctx3_" + digest},
		{name: "malformed alphabet", handle: reviewRepositoryContextV2HandlePrefix + strings.Repeat("%", reviewRepositoryContextV2DigestBytes)},
		{name: "uppercase digest", handle: reviewRepositoryContextV2HandlePrefix + strings.ToUpper(digest)},
		{name: "truncated digest", handle: reviewRepositoryContextV2HandlePrefix + digest[:len(digest)-1]},
		{name: "oversized digest", handle: handle + "a"},
		{name: "wrong repository", repo: other},
		{name: "traversal", repo: fixture.store.repo + string(filepath.Separator) + ".."},
		{name: "wrong lineage", binding: ReviewRepositoryContextBinding{LineageID: "rctx2-wrong-lineage", TargetIdentity: binding.TargetIdentity, Revision: binding.Revision}},
		{name: "wrong target", binding: ReviewRepositoryContextBinding{LineageID: binding.LineageID, TargetIdentity: hash("rctx2-wrong-target"), Revision: binding.Revision}},
		{name: "stale phase", binding: ReviewRepositoryContextBinding{LineageID: binding.LineageID, TargetIdentity: binding.TargetIdentity, Revision: hash("rctx2-stale-phase")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			candidateRepo := tt.repo
			if candidateRepo == "" {
				candidateRepo = fixture.store.repo
			}
			candidate := tt.handle
			if candidate == "" {
				candidate = handle
			}
			candidateBinding := tt.binding
			if candidateBinding == (ReviewRepositoryContextBinding{}) {
				candidateBinding = binding
			}
			before, err := os.ReadFile(fixture.store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			root, actual, err := resolveReviewRepositoryContextV2Token(t.Context(), candidateRepo, candidate, candidateBinding)
			if err == nil || root != "" || actual != (ReviewRepositoryContextBinding{}) {
				t.Fatalf("invalid rctx2 token resolved root %q, binding %#v, error %v", root, actual, err)
			}
			if strings.Contains(err.Error(), fixture.store.repo) || strings.Contains(err.Error(), other) {
				t.Fatalf("rctx2 refusal leaked repository identity: %q", err)
			}
			after, err := os.ReadFile(fixture.store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("invalid rctx2 token mutated compact authority")
			}
		})
	}
}

func TestRctx2HandleRejectsMovedWorktree(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "rctx2-moved-worktree")
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	}
	handle, err := deriveReviewRepositoryContextV2Token(t.Context(), fixture.store.repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	moved := fixture.store.repo + "-moved"
	if err := os.Rename(fixture.store.repo, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Rename(moved, fixture.store.repo); err != nil {
			t.Errorf("restore moved worktree: %v", err)
		}
	})
	root, resolved, err := resolveReviewRepositoryContextV2Token(t.Context(), moved, handle, binding)
	if err == nil || root != "" || resolved != (ReviewRepositoryContextBinding{}) {
		t.Fatalf("moved worktree resolved root %q, binding %#v, error %v", root, resolved, err)
	}
	if strings.Contains(err.Error(), fixture.store.repo) || strings.Contains(err.Error(), moved) {
		t.Fatalf("moved-worktree refusal leaked a repository path: %q", err)
	}
}

func historicalReviewRepositoryContextFixture(t *testing.T, lineage string) (string, ReviewRepositoryContextBinding) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// os.UserHomeDir reads USERPROFILE on Windows, so the Windows-only readers
	// of this fixture would otherwise resolve the real user profile.
	t.Setenv("USERPROFILE", home)
	repo := initSnapshotRepo(t)
	record, _ := pristineReviewingFixture(t, repo, lineage)
	binding := ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	}
	return repo, binding
}

func DeriveHistoricalReviewRepositoryContextHandle(ctx context.Context, repo string, binding ReviewRepositoryContextBinding) (string, error) {
	identity, err := reviewRepositoryIdentity(ctx, repo)
	if err != nil {
		return "", err
	}
	handle := reviewRepositoryContextHandle(binding, identity)
	path, err := reviewRepositoryContextPath(handle)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	record := reviewRepositoryContextFile{
		Schema: ReviewRepositoryContextSchema, Handle: handle, LineageID: binding.LineageID,
		TargetIdentity: binding.TargetIdentity, Revision: binding.Revision,
		RepositoryIdentity: identity.RepositoryIdentity, RepositoryRoot: identity.RepositoryRoot,
		GitCommonDir: identity.GitCommonDir, GitDir: identity.GitDir,
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		return "", err
	}
	return handle, nil
}

func rctx3FixtureBinding(t *testing.T, store CompactStore) ReviewRepositoryContextBinding {
	t.Helper()
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	}
}

// isolatedRctx3KeyHome points the sealing key at a fresh home, so a test that
// inspects or damages the key never shares it with another test.
func isolatedRctx3KeyHome(t *testing.T) string {
	t.Helper()
	home := canonicalTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestRctx3HandleSealsItsRootOpaquely(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "rctx3-opaque")
	binding := rctx3FixtureBinding(t, fixture.store)
	handle, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), fixture.store.repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	// The capability is declared as review.opaque_repository_context and the
	// handle is relayed on command lines and through host logs, so a reader who
	// holds it must learn nothing about the filesystem it names.
	if !strings.HasPrefix(handle, reviewRepositoryContextV3HandlePrefix) || ValidateReviewRepositoryContextHandle(handle) != nil {
		t.Fatalf("rctx3 handle has an invalid transport shape: %q", handle)
	}
	lease, err := OpenRepositoryIdentityLease(t.Context(), fixture.store.repo)
	if err != nil {
		t.Fatal(err)
	}
	live := lease.Identity()
	secrets := []string{live.RepositoryRoot, live.GitCommonDir, live.GitDir, filepath.Base(live.RepositoryRoot), os.Getenv("HOME")}
	sealed, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(handle, reviewRepositoryContextV3HandlePrefix))
	if err != nil {
		t.Fatalf("rctx3 handle is not unpadded base64url: %v", err)
	}
	if json.Valid(sealed) {
		t.Fatalf("rctx3 handle decodes to structured data: %q", sealed)
	}
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if strings.Contains(handle, secret) || bytes.Contains(sealed, []byte(secret)) {
			t.Fatalf("rctx3 handle is not opaque: it carries %q", secret)
		}
	}
}

func TestRctx3HandleResolvesFromAnyCallerRepositoryWithoutMutation(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "rctx3-round-trip")
	before, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := rctx3FixtureBinding(t, fixture.store)
	stateBefore, err := os.ReadFile(fixture.store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	handle, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), fixture.store.repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	// The caller repository is never consulted: an unrelated repository, a
	// git-less directory, and no directory at all all resolve the same root.
	for _, caller := range []string{fixture.store.repo, initSnapshotRepo(t), canonicalTempDir(t), ""} {
		root, resolved, err := ResolveReviewRepositoryContextBinding(t.Context(), caller, handle, binding)
		if err != nil || root != fixture.store.repo || resolved != binding {
			t.Fatalf("rctx3 resolution from %q = root %q, binding %#v, error %v", caller, root, resolved, err)
		}
	}
	stateAfterResolve, err := os.ReadFile(fixture.store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(stateAfterResolve) != string(stateBefore) {
		t.Fatal("rctx3 resolution mutated compact authority")
	}

	if _, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), fixture.request); err != nil {
		t.Fatal(err)
	}
	afterCapture, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterCapture.Revision == before.Revision || afterCapture.State.CapturePhaseRevision != binding.Revision {
		t.Fatalf("capture did not advance only Rn: before=%#v after=%#v", before, afterCapture)
	}
	root, resolved, err := ResolveReviewRepositoryContextBinding(t.Context(), "", handle, binding)
	if err != nil || root != fixture.store.repo || resolved != binding {
		t.Fatalf("rctx3 resolution after sibling Rn advance = root %q, binding %#v, error %v", root, resolved, err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".gentle-ai", "review-contexts")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rctx3 core created a v1 locator: %v", err)
	}
}

// A linked worktree and a submodule are ordinary roots to the sealed handle:
// nothing about the session that relays it, nor the host's worktree registry,
// takes part in resolution, so a dead registration cannot deny it either.
func TestRctx3HandleResolvesLinkedWorktreesAndSubmodulesWithoutHostDiscovery(t *testing.T) {
	t.Run("linked worktree beside a dead registration", func(t *testing.T) {
		host := initSnapshotRepo(t)
		deadRegisteredWorktree(t, host)
		target := filepath.Join(canonicalTempDir(t), "review-target")
		if err := runSnapshotGit(host, "worktree", "add", "-q", "-b", "rctx3-linked-target", target, "HEAD"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = runSnapshotGit(host, "worktree", "remove", "--force", target) })
		assertRctx3ResolvesWithoutMutation(t, host, target, "rctx3-linked")
	})

	t.Run("submodule", func(t *testing.T) {
		module := initSnapshotRepo(t)
		superproject := initSnapshotRepo(t)
		if err := runSnapshotGit(superproject, "-c", "protocol.file.allow=always", "submodule", "add", "-q", module, "module"); err != nil {
			t.Fatal(err)
		}
		if err := runSnapshotGit(superproject, "commit", "-qm", "register module"); err != nil {
			t.Fatal(err)
		}
		assertRctx3ResolvesWithoutMutation(t, superproject, filepath.Join(superproject, "module"), "rctx3-submodule")
	})
}

func assertRctx3ResolvesWithoutMutation(t *testing.T, host, target, lineage string) {
	t.Helper()
	record, _ := pristineReviewingFixture(t, target, lineage)
	store, err := CompactAuthoritativeStore(t.Context(), target, lineage)
	if err != nil {
		t.Fatal(err)
	}
	binding := ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	}
	handle, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), target, binding)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	root, resolved, err := ResolveReviewRepositoryContextBinding(t.Context(), host, handle, binding)
	if err != nil || root != target || resolved != binding {
		t.Fatalf("rctx3 resolution relayed from %q = root %q, binding %#v, error %v; want %q", host, root, resolved, err, target)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("rctx3 resolution mutated compact authority")
	}
}

func TestRctx3HandleRefusesTamperedAndForgedHandlesWithoutMutation(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "rctx3-refusals")
	binding := rctx3FixtureBinding(t, fixture.store)
	handle, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), fixture.store.repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	other := initSnapshotRepo(t)
	key, err := loadReviewRepositoryContextKey(false)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := OpenRepositoryIdentityLease(t.Context(), fixture.store.repo)
	if err != nil {
		t.Fatal(err)
	}
	targetDigest, err := reviewRepositoryContextV3Digest(lease.Identity(), binding)
	if err != nil {
		t.Fatal(err)
	}
	// Sealed with this user's own key, so only the digest can catch it: the
	// root names an unrelated repository while the digest commits to the target.
	forgedRoot, err := sealReviewRepositoryContextV3(key, other, targetDigest, binding)
	if err != nil {
		t.Fatal(err)
	}
	// A genuine handle for the same binding derived at an unrelated repository
	// matches its digest, but that repository holds no live authority for it.
	foreign, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), other, binding)
	if err != nil {
		t.Fatal(err)
	}
	encoded := strings.TrimPrefix(handle, reviewRepositoryContextV3HandlePrefix)
	flipped := []byte(encoded)
	flipped[len(flipped)/2] = 'A'
	if encoded[len(encoded)/2] == 'A' {
		flipped[len(flipped)/2] = 'B'
	}

	for _, tt := range []struct {
		name    string
		handle  string
		binding ReviewRepositoryContextBinding
	}{
		{name: "unknown prefix", handle: "rctx4_" + encoded},
		{name: "malformed alphabet", handle: reviewRepositoryContextV3HandlePrefix + strings.Repeat("%", len(encoded))},
		{name: "padded", handle: handle + "="},
		{name: "truncated", handle: handle[:len(reviewRepositoryContextV3HandlePrefix)+40]},
		{name: "oversized", handle: reviewRepositoryContextV3HandlePrefix + strings.Repeat("A", reviewRepositoryContextV3Encoding.EncodedLen(reviewRepositoryContextV3MaxSealed)+4)},
		{name: "flipped sealed byte", handle: reviewRepositoryContextV3HandlePrefix + string(flipped)},
		{name: "forged root", handle: forgedRoot},
		{name: "unrelated repository", handle: foreign},
		{name: "wrong lineage", binding: ReviewRepositoryContextBinding{LineageID: "rctx3-wrong-lineage", TargetIdentity: binding.TargetIdentity, Revision: binding.Revision}},
		{name: "wrong target", binding: ReviewRepositoryContextBinding{LineageID: binding.LineageID, TargetIdentity: hash("rctx3-wrong-target"), Revision: binding.Revision}},
		{name: "stale phase", binding: ReviewRepositoryContextBinding{LineageID: binding.LineageID, TargetIdentity: binding.TargetIdentity, Revision: hash("rctx3-stale-phase")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			candidate := tt.handle
			if candidate == "" {
				candidate = handle
			}
			candidateBinding := tt.binding
			if candidateBinding == (ReviewRepositoryContextBinding{}) {
				candidateBinding = binding
			}
			before, err := os.ReadFile(fixture.store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			root, actual, err := ResolveReviewRepositoryContextBinding(t.Context(), fixture.store.repo, candidate, candidateBinding)
			if err == nil || root != "" || actual != (ReviewRepositoryContextBinding{}) {
				t.Fatalf("invalid rctx3 handle resolved root %q, binding %#v, error %v", root, actual, err)
			}
			if !strings.Contains(err.Error(), "repository context") {
				t.Fatalf("refusal does not name the repository context: %v", err)
			}
			if _, _, relayErr := ResolveOpenCodeReviewRepositoryContextBinding(t.Context(), candidate, candidateBinding); relayErr == nil {
				t.Fatal("the OpenCode relay resolver accepted a handle the shared resolver refused")
			}
			if strings.Contains(err.Error(), fixture.store.repo) || strings.Contains(err.Error(), other) {
				t.Fatalf("rctx3 refusal leaked repository identity: %q", err)
			}
			after, err := os.ReadFile(fixture.store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("invalid rctx3 handle mutated compact authority")
			}
		})
	}
}

func TestRctx3HandleRejectsAMovedOrReplacedWorktree(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "rctx3-moved-worktree")
	binding := rctx3FixtureBinding(t, fixture.store)
	handle, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), fixture.store.repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	moved := fixture.store.repo + "-moved"
	if err := os.Rename(fixture.store.repo, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(fixture.store.repo)
		if err := os.Rename(moved, fixture.store.repo); err != nil {
			t.Errorf("restore moved worktree: %v", err)
		}
	})
	for _, caller := range []string{moved, ""} {
		root, resolved, err := ResolveReviewRepositoryContextBinding(t.Context(), caller, handle, binding)
		if err == nil || root != "" || resolved != (ReviewRepositoryContextBinding{}) {
			t.Fatalf("moved worktree resolved root %q, binding %#v, error %v", root, resolved, err)
		}
		if strings.Contains(err.Error(), fixture.store.repo) || strings.Contains(err.Error(), moved) {
			t.Fatalf("moved-worktree refusal leaked a repository path: %q", err)
		}
	}
	// A fresh repository at the sealed root has the same path identity but no
	// live authority for the binding, so it cannot stand in for the original.
	if err := os.Mkdir(fixture.store.repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runSnapshotGit(fixture.store.repo, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	if root, _, err := ResolveReviewRepositoryContextBinding(t.Context(), "", handle, binding); err == nil {
		t.Fatalf("replacement repository resolved root %q", root)
	}
}

func TestRctx3HandleIsDeterministicAcrossProcesses(t *testing.T) {
	if os.Getenv("GENTLE_AI_RCTX3_DERIVE_REPO") != "" {
		t.Setenv("HOME", os.Getenv("GENTLE_AI_RCTX3_DERIVE_HOME"))
		t.Setenv("USERPROFILE", os.Getenv("GENTLE_AI_RCTX3_DERIVE_HOME"))
		var binding ReviewRepositoryContextBinding
		if err := json.Unmarshal([]byte(os.Getenv("GENTLE_AI_RCTX3_DERIVE_BINDING")), &binding); err != nil {
			t.Fatal(err)
		}
		handle, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), os.Getenv("GENTLE_AI_RCTX3_DERIVE_REPO"), binding)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println("RCTX3_HANDLE=" + handle)
		return
	}
	home := isolatedRctx3KeyHome(t)
	repo := initSnapshotRepo(t)
	record, _ := pristineReviewingFixture(t, repo, "rctx3-deterministic")
	binding := ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	}
	handle, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	again, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), repo, binding)
	if err != nil || again != handle {
		t.Fatalf("in-process rederivation = %q, %v; want %q", again, err, handle)
	}
	encodedBinding, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestRctx3HandleIsDeterministicAcrossProcesses$", "-test.count=1")
	command.Env = append(os.Environ(),
		"GENTLE_AI_RCTX3_DERIVE_REPO="+repo, "GENTLE_AI_RCTX3_DERIVE_HOME="+home, "GENTLE_AI_RCTX3_DERIVE_BINDING="+string(encodedBinding))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("child derivation: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "RCTX3_HANDLE="+handle+"\n") {
		t.Fatalf("child process derived a different handle:\n%s\nwant %q", output, handle)
	}
}

func TestRctx3HandleSealedUnderAnotherUsersKeyIsRefused(t *testing.T) {
	isolatedRctx3KeyHome(t)
	repo := initSnapshotRepo(t)
	record, store := pristineReviewingFixture(t, repo, "rctx3-other-user")
	binding := ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	}
	handle, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	isolatedRctx3KeyHome(t)
	if _, _, err := ResolveReviewRepositoryContextBinding(t.Context(), repo, handle, binding); err == nil {
		t.Fatal("a handle resolved for a user who holds no sealing key")
	}
	ownHandle, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	if ownHandle == handle {
		t.Fatal("two users' keys sealed the same tuple into the same handle")
	}
	if root, _, err := ResolveReviewRepositoryContextBinding(t.Context(), repo, handle, binding); err == nil {
		t.Fatalf("another user's key opened the handle at %q", root)
	}
	if root, _, err := ResolveReviewRepositoryContextBinding(t.Context(), repo, ownHandle, binding); err != nil || root != repo {
		t.Fatalf("own handle resolution = %q, %v", root, err)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("foreign-key resolution mutated compact authority")
	}
}

func TestReviewRepositoryContextKeyIsCreatedOncePrivatelyByConcurrentDerivers(t *testing.T) {
	home := isolatedRctx3KeyHome(t)
	keys := make([][]byte, 8)
	errs := make([]error, len(keys))
	var wg sync.WaitGroup
	for index := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			keys[index], errs[index] = loadReviewRepositoryContextKey(true)
		}()
	}
	wg.Wait()
	for index := range keys {
		if errs[index] != nil || len(keys[index]) != reviewRepositoryContextKeyBytes || !bytes.Equal(keys[index], keys[0]) {
			t.Fatalf("concurrent key creator %d = %x, %v; want the winner's key", index, keys[index], errs[index])
		}
	}
	path := filepath.Join(home, ".gentle-ai", reviewRepositoryContextKeyFile)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != reviewRepositoryContextKeyBytes {
		t.Fatalf("key file = %v, %v", info, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode = %v, want 0600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != reviewRepositoryContextKeyFile {
			t.Fatalf("key creation left staging residue %q", entry.Name())
		}
	}
}

func TestReviewRepositoryContextKeyRefusesUnsafeFiles(t *testing.T) {
	for _, tt := range []struct {
		name    string
		posix   bool
		prepare func(t *testing.T, path string)
	}{
		{name: "symlink", posix: true, prepare: func(t *testing.T, path string) {
			real := filepath.Join(filepath.Dir(path), "elsewhere.key")
			if err := os.WriteFile(real, bytes.Repeat([]byte{7}, reviewRepositoryContextKeyBytes), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "group readable", posix: true, prepare: func(t *testing.T, path string) {
			if err := os.WriteFile(path, bytes.Repeat([]byte{7}, reviewRepositoryContextKeyBytes), 0o640); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o640); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "wrong size", prepare: func(t *testing.T, path string) {
			if err := os.WriteFile(path, bytes.Repeat([]byte{7}, reviewRepositoryContextKeyBytes-1), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "directory", prepare: func(t *testing.T, path string) {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.posix && runtime.GOOS == "windows" {
				t.Skip("POSIX key file modes and unprivileged symlinks")
			}
			home := isolatedRctx3KeyHome(t)
			repo := initSnapshotRepo(t)
			record, store := pristineReviewingFixture(t, repo, "rctx3-unsafe-key")
			binding := ReviewRepositoryContextBinding{
				LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
			}
			handle, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), repo, binding)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(home, ".gentle-ai", reviewRepositoryContextKeyFile)
			if err := os.Rename(path, path+".original"); err != nil {
				t.Fatal(err)
			}
			tt.prepare(t, path)
			before, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			_, deriveErr := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), repo, binding)
			_, _, resolveErr := ResolveReviewRepositoryContextBinding(t.Context(), repo, handle, binding)
			for _, err := range []error{deriveErr, resolveErr} {
				if !errors.Is(err, ErrReviewRepositoryContextKeyUnsafe) ||
					!strings.Contains(err.Error(), reviewRepositoryContextKeyFile) || !strings.Contains(err.Error(), "gentle-ai review status") {
					t.Fatalf("unsafe key refusal = %v, want an actionable key refusal", err)
				}
			}
			after, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("unsafe key refusal mutated compact authority")
			}
		})
	}
}

// deadRegisteredWorktree leaves a worktree registered with the host while
// removing its Git control file, reproducing the real shape of a stale editor
// or agent worktree: still listed by `git worktree list`, no longer openable.
func deadRegisteredWorktree(t *testing.T, host string) string {
	t.Helper()
	dead := filepath.Join(canonicalTempDir(t), "aaa-dead-worktree")
	if err := runSnapshotGit(host, "worktree", "add", "-q", "-b", "dead-worktree-branch", dead, "HEAD"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runSnapshotGit(host, "worktree", "remove", "--force", dead) })
	if err := os.RemoveAll(filepath.Join(dead, ".git")); err != nil {
		t.Fatal(err)
	}
	return dead
}

// The OpenCode relay resolves the sealed rctx3 handle only. An rctx2 digest
// names no root, so the relay refuses it with the STATUS that reissues the
// Task, while every other consumer keeps resolving rctx2 exactly as before.
func TestOpenCodeRelayResolverAcceptsOnlyTheSealedHandle(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "rctx3-relay-only")
	binding := rctx3FixtureBinding(t, fixture.store)
	sealed, err := DeriveOpenCodeReviewRepositoryContextHandle(t.Context(), fixture.store.repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := DeriveReviewRepositoryContextHandle(t.Context(), fixture.store.repo, binding)
	if err != nil || !strings.HasPrefix(digest, reviewRepositoryContextV2HandlePrefix) {
		t.Fatalf("non-OpenCode derivation = %q, %v; want the unchanged rctx2 digest", digest, err)
	}
	before, err := os.ReadFile(fixture.store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if root, resolved, err := ResolveOpenCodeReviewRepositoryContextBinding(t.Context(), sealed, binding); err != nil || root != fixture.store.repo || resolved != binding {
		t.Fatalf("relay resolution of rctx3 = %q, %#v, %v", root, resolved, err)
	}
	root, resolved, err := ResolveOpenCodeReviewRepositoryContextBinding(t.Context(), digest, binding)
	if err == nil || root != "" || resolved != (ReviewRepositoryContextBinding{}) || !errors.Is(err, ErrUnsealedReviewRepositoryContext) ||
		!strings.Contains(err.Error(), "gentle-ai review status") || !strings.Contains(err.Error(), "--agent <agent>") {
		t.Fatalf("relay resolution of rctx2 = %q, %#v, %v; want a typed refusal naming a fresh OpenCode STATUS", root, resolved, err)
	}
	if root, resolved, err := ResolveReviewRepositoryContextBinding(t.Context(), fixture.store.repo, digest, binding); err != nil || root != fixture.store.repo || resolved != binding {
		t.Fatalf("shared resolution of rctx2 = %q, %#v, %v", root, resolved, err)
	}
	after, err := os.ReadFile(fixture.store.StatePath())
	if err != nil || string(after) != string(before) {
		t.Fatalf("relay resolution mutated compact authority: %v", err)
	}
}
