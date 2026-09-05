package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestRctx2HandleResolvesFromARegisteredHostWorktreeWithoutMutation(t *testing.T) {
	host, target, store, binding, handle := registeredRctx2HostFixture(t, "rctx2-registered-host")
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	for _, repo := range []string{target, host} {
		root, resolved, err := ResolveReviewRepositoryContextBindingFromHost(t.Context(), repo, handle, binding)
		if err != nil || root != target || resolved != binding {
			t.Fatalf("rctx2 host resolution from %q = root %q, binding %#v, error %v", repo, root, resolved, err)
		}
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("registered-host rctx2 resolution mutated compact authority")
	}
}

func TestRctx2HostResolverRefusesUnregisteredAndUnrelatedWorktreesWithoutMutation(t *testing.T) {
	t.Run("unrelated clone", func(t *testing.T) {
		host, _, store, binding, handle := registeredRctx2HostFixture(t, "rctx2-unrelated-host")
		unrelated := filepath.Join(t.TempDir(), "unrelated-clone")
		if err := runSnapshotGit(host, "clone", "--quiet", host, unrelated); err != nil {
			t.Fatal(err)
		}
		assertRctx2HostResolutionRefusedWithoutMutation(t, unrelated, store, handle, binding)
	})

	t.Run("moved registered target", func(t *testing.T) {
		host, target, store, binding, handle := registeredRctx2HostFixture(t, "rctx2-moved-host")
		moved := target + "-moved"
		if err := os.Rename(target, moved); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Rename(moved, target); err != nil {
				t.Errorf("restore moved worktree: %v", err)
			}
		})
		assertRctx2HostResolutionRefusedWithoutMutation(t, host, store, handle, binding)
	})
}

func registeredRctx2HostFixture(t *testing.T, lineage string) (string, string, CompactStore, ReviewRepositoryContextBinding, string) {
	t.Helper()
	host := initSnapshotRepo(t)
	// canonicalTempDir, not t.TempDir: the resolver answers with the canonical
	// worktree root, and the Windows runner's TEMP is an 8.3 short name.
	target := filepath.Join(canonicalTempDir(t), "review-target")
	if err := runSnapshotGit(host, "worktree", "add", "-q", "-b", lineage+"-target", target, "HEAD"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runSnapshotGit(host, "worktree", "remove", "--force", target) })
	record, _ := pristineReviewingFixture(t, target, lineage)
	store, err := CompactAuthoritativeStore(t.Context(), target, lineage)
	if err != nil {
		t.Fatal(err)
	}
	binding := ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	}
	handle, err := deriveReviewRepositoryContextV2Token(t.Context(), target, binding)
	if err != nil {
		t.Fatal(err)
	}
	return host, target, store, binding, handle
}

func assertRctx2HostResolutionRefusedWithoutMutation(t *testing.T, host string, store CompactStore, handle string, binding ReviewRepositoryContextBinding) {
	t.Helper()
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	root, resolved, err := ResolveReviewRepositoryContextBindingFromHost(t.Context(), host, handle, binding)
	if err == nil || root != "" || resolved != (ReviewRepositoryContextBinding{}) {
		t.Fatalf("invalid host resolution = root %q, binding %#v, error %v", root, resolved, err)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("refused host resolution mutated compact authority")
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
