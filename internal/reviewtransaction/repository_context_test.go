package reviewtransaction

import (
	"context"
	"encoding/base64"
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
	if _, err := ResolveReviewRepositoryContext(t.Context(), handle, binding); err == nil {
		t.Fatal("current lifecycle resolver accepted historical rctx1")
	}
	after, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".gentle-ai", "review-contexts", "v1", handle+".json"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("historical rctx1 read changed locator bytes: %v", err)
	}
}

func TestRctx2TokenCoreRoundTripBindsOnlyActiveAuthorityFacts(t *testing.T) {
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
	if reviewRepositoryContextV2MaxEncodedBytes != len(reviewRepositoryContextV2HandlePrefix)+base64.RawURLEncoding.EncodedLen(reviewRepositoryContextV2MaxDecodedBytes) {
		t.Fatalf("rctx2 encoded bound = %d", reviewRepositoryContextV2MaxEncodedBytes)
	}
	if !strings.HasPrefix(handle, reviewRepositoryContextV2HandlePrefix) || len(handle) > reviewRepositoryContextV2MaxEncodedBytes {
		t.Fatalf("rctx2 handle = %q", handle)
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(handle, reviewRepositoryContextV2HandlePrefix))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"schema", "repository_root", "git_common_dir", "git_dir", "repository_ref",
		"lineage_id", "target_identity", "capture_phase_revision",
	} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("rctx2 payload omitted %q: %s", name, payload)
		}
	}
	if len(fields) != 8 {
		t.Fatalf("rctx2 payload carries mutable or role data: %s", payload)
	}

	root, resolved, err := resolveReviewRepositoryContextV2Token(t.Context(), handle)
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
	root, resolved, err = resolveReviewRepositoryContextV2Token(t.Context(), handle)
	if err != nil || root != fixture.store.repo || resolved != binding {
		t.Fatalf("rctx2 resolution after sibling Rn advance = root %q, binding %#v, error %v", root, resolved, err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".gentle-ai", "review-contexts")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rctx2 core created a v1 locator: %v", err)
	}
}

func TestRctx2TokenCoreRefusesTamperAndConfinementWithoutMutation(t *testing.T) {
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
	otherLease, err := OpenRepositoryIdentityLease(t.Context(), other)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name   string
		handle string
		mutate func(*reviewRepositoryContextV2Token)
	}{
		{name: "unknown prefix", handle: "rctx3_" + strings.TrimPrefix(handle, reviewRepositoryContextV2HandlePrefix)},
		{name: "malformed alphabet", handle: reviewRepositoryContextV2HandlePrefix + "%%%"},
		{name: "noncanonical base64", handle: handle + "="},
		{name: "oversized", handle: reviewRepositoryContextV2HandlePrefix + strings.Repeat("A", reviewRepositoryContextV2MaxEncodedPayloadBytes+1)},
		{name: "noncanonical JSON", handle: rctx2TokenWithTrailingNewline(t, handle)},
		{name: "unknown field", handle: rctx2TokenWithUnknownField(t, handle)},
		{name: "unknown schema", mutate: func(token *reviewRepositoryContextV2Token) { token.Schema = "gentle-ai.review-repository-context/v3" }},
		{name: "traversal", mutate: func(token *reviewRepositoryContextV2Token) { token.RepositoryRoot += string(filepath.Separator) + ".." }},
		{name: "wrong repository", mutate: func(token *reviewRepositoryContextV2Token) {
			identity := otherLease.Identity()
			token.RepositoryRoot = identity.RepositoryRoot
			token.GitCommonDir = identity.GitCommonDir
			token.GitDir = identity.GitDir
			token.RepositoryRef = identity.RepositoryRef
		}},
		{name: "wrong lineage", mutate: func(token *reviewRepositoryContextV2Token) { token.LineageID = "rctx2-wrong-lineage" }},
		{name: "wrong target", mutate: func(token *reviewRepositoryContextV2Token) { token.TargetIdentity = hash("rctx2-wrong-target") }},
		{name: "stale phase", mutate: func(token *reviewRepositoryContextV2Token) { token.CapturePhaseRevision = hash("rctx2-stale-phase") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			candidate := tt.handle
			if tt.mutate != nil {
				token, err := decodeReviewRepositoryContextV2Token(handle)
				if err != nil {
					t.Fatal(err)
				}
				tt.mutate(&token)
				candidate = encodeRctx2TestToken(t, token)
			}
			before, err := os.ReadFile(fixture.store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			root, actual, err := resolveReviewRepositoryContextV2Token(t.Context(), candidate)
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

func TestRctx2TokenCoreRejectsMovedWorktree(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "rctx2-moved-worktree")
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	handle, err := deriveReviewRepositoryContextV2Token(t.Context(), fixture.store.repo, ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	})
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
	root, binding, err := resolveReviewRepositoryContextV2Token(t.Context(), handle)
	if err == nil || root != "" || binding != (ReviewRepositoryContextBinding{}) {
		t.Fatalf("moved worktree resolved root %q, binding %#v, error %v", root, binding, err)
	}
	if strings.Contains(err.Error(), fixture.store.repo) || strings.Contains(err.Error(), moved) {
		t.Fatalf("moved-worktree refusal leaked a repository path: %q", err)
	}
}

func encodeRctx2TestToken(t *testing.T, token reviewRepositoryContextV2Token) string {
	t.Helper()
	payload, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	return reviewRepositoryContextV2HandlePrefix + base64.RawURLEncoding.EncodeToString(payload)
}

func rctx2TokenWithTrailingNewline(t *testing.T, handle string) string {
	t.Helper()
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(handle, reviewRepositoryContextV2HandlePrefix))
	if err != nil {
		t.Fatal(err)
	}
	return reviewRepositoryContextV2HandlePrefix + base64.RawURLEncoding.EncodeToString(append(payload, '\n'))
}

func rctx2TokenWithUnknownField(t *testing.T, handle string) string {
	t.Helper()
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(handle, reviewRepositoryContextV2HandlePrefix))
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) == 0 || payload[len(payload)-1] != '}' {
		t.Fatalf("rctx2 payload is not an object: %q", payload)
	}
	payload = append(payload[:len(payload)-1], []byte(`,"unknown":true}`)...)
	return reviewRepositoryContextV2HandlePrefix + base64.RawURLEncoding.EncodeToString(payload)
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
