package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type rarPlanFixture struct {
	repo        string
	repository  *RARAuthorityRepository
	publication RARPlanPublication
}

func TestRARPlanAuthorityPublishesResolvesAndReplaysExactLivePlan(t *testing.T) {
	fixture := newRARPlanFixture(t, "plan-exact")
	if _, err := os.Lstat(fixture.repository.root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("RAR root exists before plan publication: %v", err)
	}

	first, err := fixture.repository.PublishPlan(context.Background(), fixture.publication)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := fixture.repository.PublishPlan(context.Background(), fixture.publication)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := fixture.repository.ResolvePlan(context.Background(), first.AuthorityRef)
	if err != nil {
		t.Fatal(err)
	}
	if !rarPlanAuthoritiesEqual(first, replay) || !rarPlanAuthoritiesEqual(first, resolved) {
		t.Fatalf("plan authority diverged:\nfirst=%#v\nreplay=%#v\nresolved=%#v", first, replay, resolved)
	}
	if first.AuthorityRef == first.Plan.Digest ||
		first.RepositoryIdentity != fixture.repository.identity.RepositoryIdentity ||
		first.Subject != first.Applicability.Subject ||
		first.Subject != first.Plan.Subject ||
		!SnapshotsEqualExact(first.Snapshot, fixture.publication.Snapshot) {
		t.Fatalf("published plan authority does not preserve its exact preimages: %#v", first)
	}

	objectPath := fixture.repository.planObjectPath(first.AuthorityRef)
	info, err := os.Lstat(objectPath)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("plan object = %#v, %v", info, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("plan object mode = %o, want 600", info.Mode().Perm())
	}
	for _, path := range []string{fixture.repository.root, fixture.repository.planObjectsRoot()} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("plan directory %q = %#v, %v", path, info, err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
			t.Fatalf("plan directory %q mode = %o, want 700", path, info.Mode().Perm())
		}
	}

	nested := filepath.Join(fixture.repo, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	fromNested, err := OpenRARAuthorityRepository(context.Background(), nested)
	if err != nil {
		t.Fatal(err)
	}
	nestedResolved, err := fromNested.ResolvePlan(context.Background(), first.AuthorityRef)
	if err != nil || !rarPlanAuthoritiesEqual(nestedResolved, first) {
		t.Fatalf("nested resolution = %#v, %v", nestedResolved, err)
	}
}

func TestRARPlanAuthorityConcurrentExactReplayConverges(t *testing.T) {
	fixture := newRARPlanFixture(t, "plan-concurrent")
	const publishers = 8
	results := make(chan RARPlanAuthority, publishers)
	errs := make(chan error, publishers)
	var group sync.WaitGroup
	for range publishers {
		group.Add(1)
		go func() {
			defer group.Done()
			authority, err := fixture.repository.PublishPlan(context.Background(), fixture.publication)
			results <- authority
			errs <- err
		}()
	}
	group.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent PublishPlan() error = %v", err)
		}
	}
	var first *RARPlanAuthority
	for authority := range results {
		if first == nil {
			copy := authority
			first = &copy
			continue
		}
		if !rarPlanAuthoritiesEqual(*first, authority) {
			t.Errorf("concurrent plan publication diverged:\nfirst=%#v\nother=%#v", *first, authority)
		}
	}
}

// TestRARPlanAuthorityConvergesOnExhaustedLockOverIdenticalPublishedAuthority
// is the deterministic reproduction of the Darwin-only 1872 failure shape: an
// equivalent publisher exhausts the bounded lock wait while the identical
// content-addressed authority already exists on disk. The lock is the real
// advisory primitive held on the real LOCK path; only the timing is scripted.
// The honest outcome is convergence on the published authority, not a timeout
// for work that already succeeded.
func TestRARPlanAuthorityConvergesOnExhaustedLockOverIdenticalPublishedAuthority(t *testing.T) {
	t.Parallel()
	fixture := newRARPlanFixture(t, "plan-lock-converge")
	published, err := fixture.repository.PublishPlan(context.Background(), fixture.publication)
	if err != nil {
		t.Fatal(err)
	}
	held, err := acquireRARAuthorityLock(context.Background(), filepath.Join(fixture.repository.root, "LOCK"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.release() }()

	converged, err := fixture.repository.PublishPlan(context.Background(), fixture.publication)
	if err != nil {
		t.Fatalf("PublishPlan() behind a continuously-held lock with the identical published authority = %v, want convergence", err)
	}
	if !rarPlanAuthoritiesEqual(published, converged) {
		t.Fatalf("converged plan authority diverged:\npublished=%#v\nconverged=%#v", published, converged)
	}
}

// TestRARPlanAuthorityLockExhaustionWithoutConvergentAuthorityStaysTyped is
// the guard on the convergence above: exhaustion with genuinely divergent
// state — no published object, or foreign bytes occupying the content address
// — must keep failing with the typed *AuthorityLockTimeoutError and must not
// mutate the store. Do not relax this test to make contention disappear.
func TestRARPlanAuthorityLockExhaustionWithoutConvergentAuthorityStaysTyped(t *testing.T) {
	t.Parallel()
	t.Run("no published authority", func(t *testing.T) {
		t.Parallel()
		fixture := newRARPlanFixture(t, "plan-lock-exhausted")
		if err := ensureRARRepositoryRoot(fixture.repository.identity.GitCommonDir, fixture.repository.root, true); err != nil {
			t.Fatal(err)
		}
		if err := ensurePrivateRARDirectoryTree(fixture.repository.root, fixture.repository.planObjectsRoot(), true); err != nil {
			t.Fatal(err)
		}
		held, err := acquireRARAuthorityLock(context.Background(), filepath.Join(fixture.repository.root, "LOCK"))
		if err != nil {
			t.Fatal(err)
		}

		_, err = fixture.repository.PublishPlan(context.Background(), fixture.publication)
		var timeout *AuthorityLockTimeoutError
		if !errors.As(err, &timeout) || !errors.Is(err, ErrAuthorityLockTimeout) {
			t.Fatalf("PublishPlan() with no published authority behind a held lock error = %v, want *AuthorityLockTimeoutError", err)
		}
		objectPath := fixture.repository.planObjectPath(fixture.planAuthority(t).AuthorityRef)
		if _, err := os.Lstat(objectPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("exhausted publisher mutated the plan store: %v", err)
		}

		if err := held.release(); err != nil {
			t.Fatal(err)
		}
		authority, err := fixture.repository.PublishPlan(context.Background(), fixture.publication)
		if err != nil {
			t.Fatalf("PublishPlan() after the obstruction cleared = %v", err)
		}
		if _, err := fixture.repository.ResolvePlan(context.Background(), authority.AuthorityRef); err != nil {
			t.Fatalf("ResolvePlan() after the obstruction cleared = %v", err)
		}
	})
	t.Run("foreign bytes occupy the content address", func(t *testing.T) {
		t.Parallel()
		fixture := newRARPlanFixture(t, "plan-lock-foreign")
		if err := ensureRARRepositoryRoot(fixture.repository.identity.GitCommonDir, fixture.repository.root, true); err != nil {
			t.Fatal(err)
		}
		if err := ensurePrivateRARDirectoryTree(fixture.repository.root, fixture.repository.planObjectsRoot(), true); err != nil {
			t.Fatal(err)
		}
		objectPath := fixture.repository.planObjectPath(fixture.planAuthority(t).AuthorityRef)
		if err := os.WriteFile(objectPath, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		held, err := acquireRARAuthorityLock(context.Background(), filepath.Join(fixture.repository.root, "LOCK"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = held.release() }()

		if _, err := fixture.repository.PublishPlan(context.Background(), fixture.publication); !errors.Is(err, ErrAuthorityLockTimeout) {
			t.Fatalf("PublishPlan() over foreign occupied bytes behind a held lock error = %v, want ErrAuthorityLockTimeout", err)
		}
	})
}

func TestRARAuthorityRepositoryRejectsReplacedGitIdentityWithoutPublishing(
	t *testing.T,
) {
	t.Parallel()

	fixture := newRARPlanFixture(t, "plan-git-identity-swap")
	authority, err := fixture.repository.PublishPlan(
		context.Background(),
		fixture.publication,
	)
	if err != nil {
		t.Fatal(err)
	}
	originalGit := filepath.Join(fixture.repo, ".git")
	retiredGit := filepath.Join(fixture.repo, ".git-retired")
	if err := os.Rename(originalGit, retiredGit); err != nil {
		t.Skipf("cannot replace Git control directory on this platform: %v", err)
	}
	if err := runSnapshotGit(fixture.repo, "init"); err != nil {
		t.Fatalf("reinitialize Git repository: %v", err)
	}

	if _, err := fixture.repository.ResolvePlan(
		context.Background(),
		authority.AuthorityRef,
	); !errors.Is(err, ErrRepositoryIdentityChanged) {
		t.Fatalf("ResolvePlan() after Git identity replacement error = %v", err)
	}
	replacementRoot := filepath.Join(
		originalGit,
		"gentle-ai",
		"review-transactions",
		rarAuthorityDirectory,
		rarAuthorityVersion,
	)
	if _, err := os.Lstat(replacementRoot); !os.IsNotExist(err) {
		t.Fatalf(
			"stale RAR handle published under replacement Git identity: %v",
			err,
		)
	}
}

func TestRARPlanAuthorityDetectsConflictCorruptionAndDurabilityFailure(t *testing.T) {
	t.Run("occupied content address conflicts", func(t *testing.T) {
		fixture := newRARPlanFixture(t, "plan-conflict")
		authority := fixture.planAuthority(t)
		if err := ensureRARRepositoryRoot(fixture.repository.identity.GitCommonDir, fixture.repository.root, true); err != nil {
			t.Fatal(err)
		}
		if err := ensurePrivateRARDirectoryTree(fixture.repository.root, fixture.repository.planObjectsRoot(), true); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixture.repository.planObjectPath(authority.AuthorityRef), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repository.PublishPlan(context.Background(), fixture.publication); !errors.Is(err, ErrRARAuthorityConflict) {
			t.Fatalf("PublishPlan() conflict error = %v", err)
		}
	})

	t.Run("stored object corruption", func(t *testing.T) {
		fixture := newRARPlanFixture(t, "plan-corrupt")
		authority, err := fixture.repository.PublishPlan(context.Background(), fixture.publication)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixture.repository.planObjectPath(authority.AuthorityRef), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repository.ResolvePlan(context.Background(), authority.AuthorityRef); !errors.Is(err, ErrRARAuthorityCorrupt) {
			t.Fatalf("ResolvePlan() corruption error = %v", err)
		}
	})

	t.Run("directory sync failure is retry safe", func(t *testing.T) {
		fixture := newRARPlanFixture(t, "plan-durability")
		original := syncReviewDirectory
		failed := false
		syncReviewDirectory = func(path string) error {
			if !failed {
				failed = true
				return errors.New("injected plan directory sync failure")
			}
			return original(path)
		}
		_, publishErr := fixture.repository.PublishPlan(context.Background(), fixture.publication)
		syncReviewDirectory = original
		t.Cleanup(func() { syncReviewDirectory = original })
		if publishErr == nil {
			t.Fatal("PublishPlan() ignored directory sync failure")
		}
		authority, err := fixture.repository.PublishPlan(context.Background(), fixture.publication)
		if err != nil {
			t.Fatalf("PublishPlan() retry after sync failure = %v", err)
		}
		if _, err := fixture.repository.ResolvePlan(context.Background(), authority.AuthorityRef); err != nil {
			t.Fatalf("ResolvePlan() after retry = %v", err)
		}
	})
}

func TestRARPlanAuthorityRejectsUnsafePlanStorage(t *testing.T) {
	fixture := newRARPlanFixture(t, "plan-path-safety")
	if err := ensureRARRepositoryRoot(fixture.repository.identity.GitCommonDir, fixture.repository.root, true); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, fixture.repository.planObjectsRoot()); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := fixture.repository.PublishPlan(context.Background(), fixture.publication); err == nil {
		t.Fatal("PublishPlan() accepted symlinked plan storage")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unsafe plan storage target mutated: %v", entries)
	}
}

func TestRARPlanAuthorityRejectsStaleSnapshotPathsAndPolicyDrift(t *testing.T) {
	t.Run("live candidate changed", func(t *testing.T) {
		fixture := newRARPlanFixture(t, "plan-stale-candidate")
		authority, err := fixture.repository.PublishPlan(context.Background(), fixture.publication)
		if err != nil {
			t.Fatal(err)
		}
		writeSnapshotFile(t, fixture.repo, "tracked.txt", "candidate changed after plan publication\n")
		if _, err := fixture.repository.ResolvePlan(context.Background(), authority.AuthorityRef); !errors.Is(err, ErrRARAuthorityStale) {
			t.Fatalf("ResolvePlan() stale snapshot error = %v", err)
		}
	})

	t.Run("live path set changed", func(t *testing.T) {
		fixture := newRARPlanFixture(t, "plan-stale-paths")
		authority, err := fixture.repository.PublishPlan(context.Background(), fixture.publication)
		if err != nil {
			t.Fatal(err)
		}
		writeSnapshotFile(t, fixture.repo, "deleted.txt", "second changed path\n")
		live, err := (SnapshotBuilder{Repo: fixture.repo}).Build(context.Background(), Target{
			Kind: TargetCurrentChanges, IntendedUntracked: []string{},
		})
		if err != nil {
			t.Fatal(err)
		}
		if live.PathsDigest == fixture.publication.Snapshot.PathsDigest {
			t.Fatal("path drift fixture did not change PathsDigest")
		}
		if _, err := fixture.repository.ResolvePlan(context.Background(), authority.AuthorityRef); !errors.Is(err, ErrRARAuthorityStale) {
			t.Fatalf("ResolvePlan() stale paths error = %v", err)
		}
	})

	t.Run("policy preimages differ", func(t *testing.T) {
		fixture := newRARPlanFixture(t, "plan-policy-drift")
		different, err := NewVerificationPlanRegistry(
			verificationTestHash("different-plan-policy"),
			[]string{},
			fixture.publication.Registry.Obligations,
		)
		if err != nil {
			t.Fatal(err)
		}
		drift := fixture.publication
		drift.Registry = different
		if _, err := fixture.repository.PublishPlan(context.Background(), drift); err == nil {
			t.Fatal("PublishPlan() accepted applicability/plan from a stale policy registry")
		}
		if _, err := os.Lstat(fixture.repository.root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid policy preimages created RAR storage: %v", err)
		}
	})

	t.Run("caller applicability differs from native classifier", func(t *testing.T) {
		fixture := newRARPlanFixture(t, "plan-caller-applicability")
		forged := fixture.publication.Applicability
		forged.Decision = VerificationApplicable
		forged.ContentActivity = VerificationContentActive
		forged.Reasons = []VerificationApplicabilityReason{{
			Code: VerificationReasonActiveSource,
			Path: "tracked.txt",
		}}
		var err error
		forged.Digest, err = verificationApplicabilityDigest(forged)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := BuildVerificationPlan(forged, fixture.publication.Registry)
		if err != nil {
			t.Fatal(err)
		}
		drift := fixture.publication
		drift.Applicability = forged
		drift.Plan = plan
		if _, err := fixture.repository.PublishPlan(context.Background(), drift); err == nil {
			t.Fatal("PublishPlan() accepted a caller-forged native applicability decision")
		}
		if _, err := os.Lstat(fixture.repository.root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("forged applicability created RAR storage: %v", err)
		}
	})

	t.Run("snapshot subject paths differ", func(t *testing.T) {
		fixture := newRARPlanFixture(t, "plan-subject-path-drift")
		drift := fixture.publication
		drift.Snapshot.PathsDigest = verificationTestHash("forged-plan-paths")
		if _, err := fixture.repository.PublishPlan(context.Background(), drift); err == nil {
			t.Fatal("PublishPlan() accepted a snapshot that differs from the plan subject")
		}
		if _, err := os.Lstat(fixture.repository.root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid path preimages created RAR storage: %v", err)
		}
	})
}

func TestRARPlanAuthorityBindsLinkedWorktreeIdentity(t *testing.T) {
	repo := initSnapshotRepo(t)
	linked := filepath.Join(t.TempDir(), "linked-plan-worktree")
	gitSnapshot(t, repo, "worktree", "add", "--detach", linked, "HEAD")
	writeSnapshotFile(t, linked, "tracked.txt", "linked candidate\n")

	fixture := newRARPlanFixtureAtRepo(t, linked, "plan-linked")
	authority, err := fixture.repository.PublishPlan(context.Background(), fixture.publication)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.ResolvePlan(context.Background(), authority.AuthorityRef); err != nil {
		t.Fatalf("linked worktree ResolvePlan() = %v", err)
	}

	mainRepository, err := OpenRARAuthorityRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if mainRepository.root != fixture.repository.root {
		t.Fatalf("worktrees do not share Git-common-dir RAR root: %q != %q", mainRepository.root, fixture.repository.root)
	}
	if _, err := mainRepository.ResolvePlan(context.Background(), authority.AuthorityRef); !errors.Is(err, ErrRARAuthorityStale) {
		t.Fatalf("different worktree identity ResolvePlan() error = %v", err)
	}
}

func TestRARPlanAuthorityRejectsNonLiveExactRevision(t *testing.T) {
	repo := initSnapshotRepo(t)
	head := gitSnapshot(t, repo, "rev-parse", "HEAD")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetExactRevision, Revision: head,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := rarPlanFixtureForSnapshot(t, repo, "plan-exact-revision", snapshot)
	if _, err := fixture.repository.PublishPlan(context.Background(), fixture.publication); !errors.Is(err, ErrRARAuthorityStale) {
		t.Fatalf("PublishPlan() non-live exact revision error = %v", err)
	}
	if _, err := os.Lstat(fixture.repository.root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-live exact revision created RAR storage: %v", err)
	}
}

// TestRARPlanAuthorityBindsEveryVerificationDimension proves the authority is
// the single place where applicability, cost, mutation effect, and permission
// effect are frozen together. Omitting any one of them is rejected.
func TestRARPlanAuthorityBindsEveryVerificationDimension(t *testing.T) {
	t.Parallel()
	complete := VerificationEffectProfile{
		Applicability: VerificationApplicable, Cost: VerificationCostQuick,
		MutationEffect: VerificationMutationDestructive, PermissionEffect: VerificationPermissionOrdinary,
	}
	authority := syntheticPlanAuthority(t, "bind-all", VerificationCostQuick, &complete)
	if err := authority.Validate(); err != nil {
		t.Fatal(err)
	}
	gate, err := authority.Gate()
	if err != nil {
		t.Fatal(err)
	}
	if !gate.RequiresImmediateAuthorization || gate.RequiresFrozenPlanConsent {
		t.Fatalf("quick destructive plan gate = %#v", gate)
	}

	t.Run("applicable plan cannot omit the effect profile", func(t *testing.T) {
		t.Parallel()
		tampered := syntheticPlanAuthority(t, "bind-all", VerificationCostQuick, nil)
		if err := tampered.Validate(); !errors.Is(err, ErrVerificationDimensionMissing) {
			t.Fatalf("Validate() error = %v, want ErrVerificationDimensionMissing", err)
		}
	})
	for _, missing := range []struct {
		name    string
		profile VerificationEffectProfile
	}{
		{"cost", VerificationEffectProfile{
			Applicability:  VerificationApplicable,
			MutationEffect: VerificationMutationReadOnly, PermissionEffect: VerificationPermissionOrdinary,
		}},
		{"mutation effect", VerificationEffectProfile{
			Applicability: VerificationApplicable, Cost: VerificationCostQuick,
			PermissionEffect: VerificationPermissionOrdinary,
		}},
		{"permission effect", VerificationEffectProfile{
			Applicability: VerificationApplicable, Cost: VerificationCostQuick,
			MutationEffect: VerificationMutationReadOnly,
		}},
	} {
		t.Run("applicable plan cannot omit "+missing.name, func(t *testing.T) {
			t.Parallel()
			profile := missing.profile
			tampered := syntheticPlanAuthority(t, "bind-all", VerificationCostQuick, &profile)
			if err := tampered.Validate(); !errors.Is(err, ErrVerificationDimensionMissing) {
				t.Fatalf("Validate() error = %v, want ErrVerificationDimensionMissing", err)
			}
		})
	}
	t.Run("declared cost must bind the exact obligation costs", func(t *testing.T) {
		t.Parallel()
		profile := complete
		profile.Cost = VerificationCostLong
		tampered := syntheticPlanAuthority(t, "bind-all", VerificationCostQuick, &profile)
		if err := tampered.Validate(); err == nil {
			t.Fatal("Validate() accepted an effect cost that contradicts the frozen obligations")
		}
	})
	t.Run("declared applicability must bind the exact plan", func(t *testing.T) {
		t.Parallel()
		profile := complete
		profile.Applicability = VerificationUnknown
		tampered := syntheticPlanAuthority(t, "bind-all", VerificationCostQuick, &profile)
		if err := tampered.Validate(); err == nil {
			t.Fatal("Validate() accepted an effect applicability that contradicts the frozen plan")
		}
	})
}

// TestNotApplicablePlanAuthorityPlansAndRunsNothing proves a plan that runs
// nothing neither needs nor may claim effect dimensions.
func TestNotApplicablePlanAuthorityPlansAndRunsNothing(t *testing.T) {
	t.Parallel()
	authority := syntheticNonExecutablePlanAuthority(t, "no-work", VerificationNotApplicable, nil)
	if err := authority.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(authority.Plan.Obligations) != 0 {
		t.Fatalf("not-applicable plan carried %d obligations", len(authority.Plan.Obligations))
	}
	gate, err := authority.Gate()
	if err != nil {
		t.Fatal(err)
	}
	if gate.Decision != VerificationDecisionRecordNotApplicable ||
		gate.RequiresFrozenPlanConsent || gate.RequiresImmediateAuthorization || gate.AllowsAutomaticRun() {
		t.Fatalf("not-applicable authority gate = %#v", gate)
	}

	profile := VerificationEffectProfile{
		Applicability: VerificationNotApplicable, Cost: VerificationCostQuick,
		MutationEffect: VerificationMutationReadOnly, PermissionEffect: VerificationPermissionOrdinary,
	}
	tampered := syntheticNonExecutablePlanAuthority(t, "no-work", VerificationNotApplicable, &profile)
	if err := tampered.Validate(); err == nil {
		t.Fatal("Validate() accepted effect dimensions on a plan that runs nothing")
	}
}

func TestUnknownPlanAuthorityRecordsEvidenceGap(t *testing.T) {
	t.Parallel()
	authority := syntheticNonExecutablePlanAuthority(t, "gap", VerificationUnknown, nil)
	gate, err := authority.Gate()
	if err != nil {
		t.Fatal(err)
	}
	if gate.Decision != VerificationDecisionRecordEvidenceGap ||
		gate.RequiresFrozenPlanConsent || gate.RequiresImmediateAuthorization || gate.AllowsAutomaticRun() {
		t.Fatalf("unknown authority gate = %#v", gate)
	}
	if _, err := NewVerificationFrozenPlanConsent(authority); err == nil {
		t.Fatal("NewVerificationFrozenPlanConsent() asked for consent on a recorded evidence gap")
	}
}

// TestFrozenPlanConsentResumeReusesTheIdenticalPlan proves one consent is
// enough: the resume replays the exact frozen plan and never asks again.
func TestFrozenPlanConsentResumeReusesTheIdenticalPlan(t *testing.T) {
	t.Parallel()
	profile := VerificationEffectProfile{
		Applicability: VerificationApplicable, Cost: VerificationCostLong,
		MutationEffect: VerificationMutationReadOnly, PermissionEffect: VerificationPermissionOrdinary,
	}
	authority := syntheticPlanAuthority(t, "frozen-consent", VerificationCostLong, &profile)
	gate, err := authority.Gate()
	if err != nil {
		t.Fatal(err)
	}
	if gate.Decision != VerificationDecisionFrozenPlanConsent || !gate.RequiresFrozenPlanConsent {
		t.Fatalf("long read-only authority gate = %#v", gate)
	}
	consent, err := NewVerificationFrozenPlanConsent(authority)
	if err != nil {
		t.Fatal(err)
	}
	if consent.AuthorityRef != authority.AuthorityRef || consent.PlanDigest != authority.Plan.Digest {
		t.Fatalf("consent = %#v, want the exact frozen plan binding", consent)
	}

	for range 3 {
		plan, resumed, err := ResumeFrozenVerificationPlan(authority, consent)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(plan, authority.Plan) {
			t.Fatalf("resume regenerated the plan:\ngot  %#v\nwant %#v", plan, authority.Plan)
		}
		if resumed.RequiresFrozenPlanConsent {
			t.Fatalf("resume asked for consent a second time: %#v", resumed)
		}
		if !resumed.AllowsAutomaticRun() {
			t.Fatalf("resumed gate = %#v, want the consented plan to launch", resumed)
		}
	}

	t.Run("consent cannot cross to another frozen plan", func(t *testing.T) {
		t.Parallel()
		other := syntheticPlanAuthority(t, "frozen-consent-other", VerificationCostLong, &profile)
		if _, _, err := ResumeFrozenVerificationPlan(other, consent); !errors.Is(err, ErrVerificationAuthorizationStale) {
			t.Fatalf("ResumeFrozenVerificationPlan() error = %v, want ErrVerificationAuthorizationStale", err)
		}
	})
	t.Run("consent cannot be forged", func(t *testing.T) {
		t.Parallel()
		forged := consent
		forged.Effects.Cost = VerificationCostQuick
		if _, _, err := ResumeFrozenVerificationPlan(authority, forged); err == nil {
			t.Fatal("ResumeFrozenVerificationPlan() accepted a forged consent")
		}
	})
}

// TestVerificationEffectAuthorizationRejectsStaleAndCrossBoundReplay proves an
// authorization is only ever valid for the exact frozen plan it was issued
// against, so a replay against any other plan is rejected before any effect.
func TestVerificationEffectAuthorizationRejectsStaleAndCrossBoundReplay(t *testing.T) {
	t.Parallel()
	profile := VerificationEffectProfile{
		Applicability: VerificationApplicable, Cost: VerificationCostQuick,
		MutationEffect: VerificationMutationDestructive, PermissionEffect: VerificationPermissionOrdinary,
	}
	authority := syntheticPlanAuthority(t, "effect-auth", VerificationCostQuick, &profile)
	authorization, err := NewVerificationEffectAuthorization(authority, "unit")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateVerificationEffectAuthorization(authority, authorization); err != nil {
		t.Fatal(err)
	}

	t.Run("stale authority ref", func(t *testing.T) {
		t.Parallel()
		superseded := syntheticPlanAuthority(t, "effect-auth-next", VerificationCostQuick, &profile)
		if err := ValidateVerificationEffectAuthorization(superseded, authorization); !errors.Is(
			err, ErrVerificationAuthorizationStale,
		) {
			t.Fatalf("ValidateVerificationEffectAuthorization() error = %v, want stale", err)
		}
	})
	t.Run("cross-bound plan digest", func(t *testing.T) {
		t.Parallel()
		tampered := authorization
		tampered.PlanDigest = verificationTestHash("other-plan")
		if err := ValidateVerificationEffectAuthorization(authority, tampered); err == nil {
			t.Fatal("ValidateVerificationEffectAuthorization() accepted a cross-bound plan digest")
		}
	})
	t.Run("unplanned obligation", func(t *testing.T) {
		t.Parallel()
		if _, err := NewVerificationEffectAuthorization(authority, "absent"); err == nil {
			t.Fatal("NewVerificationEffectAuthorization() authorized an unplanned obligation")
		}
	})
	t.Run("forged effect profile", func(t *testing.T) {
		t.Parallel()
		tampered := authorization
		tampered.Effects.MutationEffect = VerificationMutationReadOnly
		if err := ValidateVerificationEffectAuthorization(authority, tampered); err == nil {
			t.Fatal("ValidateVerificationEffectAuthorization() accepted a downgraded mutation effect")
		}
	})
	t.Run("ordinary read-only plan needs no authorization", func(t *testing.T) {
		t.Parallel()
		harmless := VerificationEffectProfile{
			Applicability: VerificationApplicable, Cost: VerificationCostQuick,
			MutationEffect: VerificationMutationReadOnly, PermissionEffect: VerificationPermissionOrdinary,
		}
		ordinary := syntheticPlanAuthority(t, "effect-auth-ordinary", VerificationCostQuick, &harmless)
		if _, err := NewVerificationEffectAuthorization(ordinary, "unit"); err == nil {
			t.Fatal("NewVerificationEffectAuthorization() issued an unnecessary authorization")
		}
	})
}

// syntheticPlanAuthority freezes a pure in-memory applicable plan authority.
// It needs no live repository because only ResolvePlan revalidates liveness.
func syntheticPlanAuthority(
	t *testing.T,
	name string,
	cost VerificationCost,
	effects *VerificationEffectProfile,
) RARPlanAuthority {
	t.Helper()
	registry, applicability := syntheticPlanContracts(t, name, cost)
	plan, err := BuildVerificationPlan(applicability, registry)
	if err != nil {
		t.Fatal(err)
	}
	return syntheticAuthorityFor(t, name, applicability, registry, plan, effects)
}

func syntheticNonExecutablePlanAuthority(
	t *testing.T,
	name string,
	decision VerificationApplicabilityValue,
	effects *VerificationEffectProfile,
) RARPlanAuthority {
	t.Helper()
	registry, applicability := syntheticPlanContracts(t, name, VerificationCostQuick)
	applicability.Decision = decision
	if decision == VerificationNotApplicable {
		applicability.ContentActivity = VerificationContentPassive
		applicability.Reasons = []VerificationApplicabilityReason{
			{Code: VerificationReasonPassiveDocument, Path: "README.md"},
		}
	} else {
		applicability.ContentActivity = VerificationContentUnknown
		applicability.Reasons = []VerificationApplicabilityReason{
			{Code: VerificationReasonUnknownContent, Path: "asset.opaque"},
		}
	}
	var err error
	if applicability.Digest, err = verificationApplicabilityDigest(applicability); err != nil {
		t.Fatal(err)
	}
	plan, err := BuildVerificationPlan(applicability, registry)
	if err != nil {
		t.Fatal(err)
	}
	return syntheticAuthorityFor(t, name, applicability, registry, plan, effects)
}

func syntheticPlanContracts(
	t *testing.T,
	name string,
	cost VerificationCost,
) (VerificationPlanRegistry, VerificationApplicability) {
	t.Helper()
	registry, err := NewVerificationPlanRegistry(
		verificationTestHash(name+"-policy"),
		[]string{},
		[]VerificationObligation{{
			ID: "unit", RequirementRef: verificationTestHash(name + "-requirement"),
			ArgvRef: verificationTestHash(name + "-argv"), CapabilityRef: "host.exec", Cost: cost,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	applicability := verificationTestApplicability(
		t,
		registry,
		strings.Repeat("b", 40),
		verificationTestHash(name+"-identity"),
	)
	return registry, applicability
}

func syntheticAuthorityFor(
	t *testing.T,
	name string,
	applicability VerificationApplicability,
	registry VerificationPlanRegistry,
	plan VerificationPlan,
	effects *VerificationEffectProfile,
) RARPlanAuthority {
	t.Helper()
	authority := RARPlanAuthority{
		Schema:             RARPlanAuthoritySchema,
		RepositoryIdentity: verificationTestHash(name + "-repository"),
		Snapshot: Snapshot{
			Kind: applicability.Subject.Kind, BaseTree: applicability.Subject.BaseTree,
			CandidateTree: applicability.Subject.CandidateTree,
			PathsDigest:   applicability.Subject.PathsDigest,
			Identity:      applicability.Subject.SnapshotIdentity,
		},
		Subject:       applicability.Subject,
		Applicability: applicability,
		Registry:      registry,
		Plan:          plan,
		Effects:       effects,
	}
	var err error
	if authority.AuthorityRef, err = rarPlanAuthorityDigest(authority); err != nil {
		t.Fatal(err)
	}
	return authority
}

func newRARPlanFixture(t *testing.T, name string) rarPlanFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "planned candidate "+name+"\n")
	return newRARPlanFixtureAtRepo(t, repo, name)
}

func newRARPlanFixtureAtRepo(t *testing.T, repo, name string) rarPlanFixture {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return rarPlanFixtureForSnapshot(t, repo, name, snapshot)
}

func rarPlanFixtureForSnapshot(t *testing.T, repo, name string, snapshot Snapshot) rarPlanFixture {
	t.Helper()
	policyHash := verificationTestHash(name + "-policy")
	registry, err := NewVerificationPlanRegistry(
		policyHash,
		[]string{},
		[]VerificationObligation{{
			ID:              "unit",
			RequirementRef:  verificationTestHash(name + "-requirement"),
			ArgvRef:         verificationTestHash(name + "-argv"),
			CapabilityRef:   "host.exec",
			Cost:            VerificationCostQuick,
			MandatoryGlobal: true,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	applicability, err := (SnapshotBuilder{Repo: repo}).ClassifyVerificationApplicability(
		context.Background(),
		snapshot,
		registry,
		[]string{},
	)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildVerificationPlan(applicability, registry)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := OpenRARAuthorityRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return rarPlanFixture{
		repo:       repo,
		repository: repository,
		publication: RARPlanPublication{
			Snapshot: snapshot, Applicability: applicability,
			Registry: registry, Plan: plan,
		},
	}
}

func (fixture rarPlanFixture) planAuthority(t *testing.T) RARPlanAuthority {
	t.Helper()
	subject, err := VerificationSubjectFromSnapshot(fixture.publication.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	authority := RARPlanAuthority{
		Schema:             RARPlanAuthoritySchema,
		RepositoryIdentity: fixture.repository.identity.RepositoryIdentity,
		Snapshot:           fixture.publication.Snapshot,
		Subject:            subject,
		Applicability:      fixture.publication.Applicability,
		Registry:           fixture.publication.Registry,
		Plan:               fixture.publication.Plan,
	}
	authority.AuthorityRef, err = rarPlanAuthorityDigest(authority)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func rarPlanAuthoritiesEqual(left, right RARPlanAuthority) bool {
	return left.Schema == right.Schema &&
		left.AuthorityRef == right.AuthorityRef &&
		left.RepositoryIdentity == right.RepositoryIdentity &&
		SnapshotsEqualExact(left.Snapshot, right.Snapshot) &&
		left.Subject == right.Subject &&
		reflect.DeepEqual(left.Applicability, right.Applicability) &&
		reflect.DeepEqual(left.Registry, right.Registry) &&
		reflect.DeepEqual(left.Plan, right.Plan) &&
		reflect.DeepEqual(left.Effects, right.Effects)
}
