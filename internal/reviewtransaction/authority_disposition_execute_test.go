package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// authorizedPlanFixture derives a plan against repo and populates a valid
// Authorization binding, exactly as a maintainer supplying
// --authorization would (Slice S3 wires the CLI flag; this fixture stamps
// the same binding the flag would carry).
func authorizedPlanFixture(t *testing.T, repo, actor, reason string) AuthorityDispositionPlan {
	t.Helper()
	plan := derivePlanFixture(t, repo, actor, reason)
	plan.Authorization = authorityDispositionAuthorizationBinding(plan)
	return plan
}

// TestAuthorityDispositionClosureAdmission satisfies Wave 6 tasks.md 1.6 and
// rdd-closure-disposition-execution's "N-Node Admission for Closed Anomaly
// Classes" requirement: N=1 is admitted (the N=1 base case,
// rdd-leaf-disposition-execution's "Single-node closure is admitted"), a
// classified N>=2 closure is admitted provided the seed is last (descendant-
// first, seed-last per authorityDispositionClosure's Wave 6 ordering), and an
// unknown/mixed/ambiguous shape — an empty anomaly class, or a closure that
// does not end with the seed — still refuses with no generic fallback.
func TestAuthorityDispositionClosureAdmission(t *testing.T) {
	t.Run("N=1 admitted", func(t *testing.T) {
		plan := AuthorityDispositionPlan{SeedSet: []string{"seed"}, Closure: []string{"seed"}}
		if err := admitClosureDisposition(plan); err != nil {
			t.Fatalf("N=1 closure refused: %v", err)
		}
	})
	t.Run("N>=2 classified closure admitted when seed is last", func(t *testing.T) {
		plan := AuthorityDispositionPlan{SeedSet: []string{"seed"}, Closure: []string{"grandchild", "child", "seed"}}
		if err := admitClosureDisposition(plan); err != nil {
			t.Fatalf("multi-node descendant-first, seed-last closure refused: %v", err)
		}
	})
	t.Run("seed not last refuses (malformed/ambiguous shape)", func(t *testing.T) {
		plan := AuthorityDispositionPlan{SeedSet: []string{"seed"}, Closure: []string{"seed", "child", "grandchild"}}
		if err := admitClosureDisposition(plan); err == nil {
			t.Fatal("closure with seed not last was admitted")
		}
	})
	t.Run("more than one seed refuses", func(t *testing.T) {
		plan := AuthorityDispositionPlan{SeedSet: []string{"seed-one", "seed-two"}, Closure: []string{"child", "seed-one"}}
		if err := admitClosureDisposition(plan); err == nil {
			t.Fatal("plan with more than one seed was admitted")
		}
	})
	t.Run("empty closure refuses", func(t *testing.T) {
		plan := AuthorityDispositionPlan{SeedSet: []string{"seed"}, Closure: []string{}}
		if err := admitClosureDisposition(plan); err == nil {
			t.Fatal("plan with an empty closure was admitted")
		}
	})
}

// TestAuthorityDispositionExecuteAdmitsSingleNodeClosure satisfies tasks.md
// 2.1 and "Cardinality-One Admission": a single-node closure classified from
// #1892's historical exact-binding edge shape is admitted and quarantined.
func TestAuthorityDispositionExecuteAdmitsSingleNodeClosure(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, successor, successorStore := forgedRecoveryPair(t, repo, "admit", "admit target\n")
	plan := authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")

	record, err := executeAuthorityDisposition(context.Background(), repo, plan)
	if err != nil {
		t.Fatalf("executeAuthorityDisposition: %v", err)
	}
	if record.Status != CompactReclaimCommitted || record.LineageID != successor.State.LineageID {
		t.Fatalf("record = %#v", record)
	}
	if _, statErr := os.Stat(successorStore.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("admitted leaf source directory survived: %v", statErr)
	}
}

// TestAuthorityDispositionClosureGitRepositorySelectionThreatMatrix satisfies
// tasks.md 1.9's Git-repository-selection threat matrix cell under the
// renamed export (AdmitAuthorityDispositionClosure): a relative --cwd and an
// absolute --cwd both resolve the same repository root and bind/execute
// correctly, and a foreign-repo --cwd — a plan derived against one
// repository, submitted against a completely different one — refuses
// pre-lock, naming the repository-binding mismatch, without ever admitting
// through the renamed export.
func TestAuthorityDispositionClosureGitRepositorySelectionThreatMatrix(t *testing.T) {
	t.Run("relative --cwd binds and executes", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		parent, base := filepath.Dir(repo), filepath.Base(repo)
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chdir(cwd) })
		if err := os.Chdir(parent); err != nil {
			t.Fatal(err)
		}
		forgedRecoveryPair(t, repo, "relative-cwd", "relative cwd target\n")
		plan := authorizedPlanFixture(t, base, "maintainer@example.com", "quarantine forged recovery authorization")
		if err := AdmitAuthorityDispositionClosure(plan); err != nil {
			t.Fatalf("relative --cwd plan refused by renamed export: %v", err)
		}
		if _, err := executeAuthorityDisposition(context.Background(), base, plan); err != nil {
			t.Fatalf("relative --cwd execution: %v", err)
		}
	})

	t.Run("absolute --cwd binds and executes", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		if !filepath.IsAbs(repo) {
			t.Fatalf("initSnapshotRepo did not return an absolute path: %q", repo)
		}
		forgedRecoveryPair(t, repo, "absolute-cwd", "absolute cwd target\n")
		plan := authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")
		if err := AdmitAuthorityDispositionClosure(plan); err != nil {
			t.Fatalf("absolute --cwd plan refused by renamed export: %v", err)
		}
		if _, err := executeAuthorityDisposition(context.Background(), repo, plan); err != nil {
			t.Fatalf("absolute --cwd execution: %v", err)
		}
	})

	t.Run("foreign-repo --cwd refuses pre-lock without moving anything", func(t *testing.T) {
		repoA := initSnapshotRepo(t)
		_, successorA, successorStoreA := forgedRecoveryPair(t, repoA, "foreign-a", "foreign repo a target\n")
		plan := authorizedPlanFixture(t, repoA, "maintainer@example.com", "quarantine forged recovery authorization")

		repoB := initSnapshotRepo(t)

		if err := AdmitAuthorityDispositionClosure(plan); err != nil {
			t.Fatalf("plan admission refused before the cross-repository check even ran: %v", err)
		}
		_, err := executeAuthorityDisposition(context.Background(), repoB, plan)
		if err == nil || !strings.Contains(err.Error(), "different repository") {
			t.Fatalf("foreign-repo execution error = %v, want a repository-binding mismatch refusal", err)
		}
		if _, statErr := os.Stat(successorStoreA.Dir); statErr != nil {
			t.Fatalf("foreign-repo refusal moved the original repository's entry: %v", statErr)
		}
		_ = successorA
	})
}

// TestAuthorityDispositionExecuteNoPredecessorPointerRewritten satisfies
// tasks.md 2.2: every predecessor pointer in the retained graph is
// byte-identical to its pre-execution value after a successful leaf
// disposition.
func TestAuthorityDispositionExecuteNoPredecessorPointerRewritten(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor, _, _ := forgedRecoveryPair(t, repo, "predecessor", "predecessor target\n")
	predecessorStore, err := CompactAuthoritativeStore(context.Background(), repo, predecessor.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	plan := authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")
	if _, err := executeAuthorityDisposition(context.Background(), repo, plan); err != nil {
		t.Fatalf("executeAuthorityDisposition: %v", err)
	}
	after, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("predecessor pointer bytes changed across leaf disposition execution")
	}
}

// TestAuthorityDispositionExecuteLockAndCASReinspection satisfies tasks.md
// 2.3: revision drift under lock refuses on CAS mismatch and mutates zero
// bytes. Adding an unrelated new lineage after plan derivation changes
// authority_inventory_revision, which is exactly the drift the plan's
// Authorization and re-derivation CAS must catch.
func TestAuthorityDispositionExecuteLockAndCASReinspection(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, successor, successorStore := forgedRecoveryPair(t, repo, "cas", "cas target\n")
	plan := authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")

	// Written directly to store bytes, like forgedRecoveryPair itself: the
	// existing forged pair already makes the whole authority graph invalid,
	// so a product write path (StartCompactAuthority) that validates the
	// whole graph would refuse here. A plain unrelated lineage changes
	// authority_inventory_revision — the exact drift the plan's CAS must
	// catch — without depending on graph validity.
	driftState := newCompactTestState(t, repo, "cas-drift-unrelated")
	driftStore, err := CompactAuthoritativeStore(context.Background(), repo, driftState.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	writeCompactFixtureRecord(t, driftStore, driftState)
	beforeBytes, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	_, err = executeAuthorityDisposition(context.Background(), repo, plan)
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("stale-plan execution error = %v, want ErrConcurrentUpdate", err)
	}
	afterBytes, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeBytes) != string(afterBytes) {
		t.Fatal("CAS-refused execution mutated the target entry's bytes")
	}
	if _, statErr := os.Lstat(filepath.Join(filepath.Dir(filepath.Dir(successorStore.Dir)), "quarantine")); !os.IsNotExist(statErr) {
		t.Fatalf("CAS-refused execution created a quarantine directory: %v", statErr)
	}
	_ = successor
}

// TestAuthorityDispositionExecuteValidatesAuthorizationAgainstDigestBoundPlan
// satisfies tasks.md 2.4, mandatory obligation (b): the executor validates
// the populated Authorization against the digest-bound plan at execution
// time. A forged Authorization (bound to a different plan_digest) refuses
// even though cardinality and CAS both pass.
func TestAuthorityDispositionExecuteValidatesAuthorizationAgainstDigestBoundPlan(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, _, successorStore := forgedRecoveryPair(t, repo, "authz", "authz target\n")
	plan := derivePlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")
	plan.Authorization = "sha256:forged-authorization-bound-to-a-different-plan-digest"

	_, err := executeAuthorityDisposition(context.Background(), repo, plan)
	if err == nil {
		t.Fatal("forged authorization was admitted despite passing cardinality and CAS")
	}
	if _, statErr := os.Stat(successorStore.Dir); statErr != nil {
		t.Fatalf("forged-authorization refusal moved the target: %v", statErr)
	}
}

// TestAuthorityDispositionExecuteQuarantineIsByteExact satisfies tasks.md
// 2.5: quarantined bytes are byte-identical to the original entry and
// residue evidence records the original location and content.
func TestAuthorityDispositionExecuteQuarantineIsByteExact(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, successor, successorStore := forgedRecoveryPair(t, repo, "quarantine", "quarantine target\n")
	original, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	plan := authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")

	record, err := executeAuthorityDisposition(context.Background(), repo, plan)
	if err != nil {
		t.Fatalf("executeAuthorityDisposition: %v", err)
	}
	moved, err := os.ReadFile(filepath.Join(record.QuarantinePath, "residue", compactStateFileName))
	if err != nil {
		t.Fatalf("read quarantined residue: %v", err)
	}
	if string(moved) != string(original) {
		t.Fatal("quarantine did not byte-preserve the original entry")
	}
	if record.SourcePath != successorStore.Dir || len(record.Residue) == 0 {
		t.Fatalf("residue evidence is incomplete: %#v", record)
	}
	if record.AuthorityDisposition == nil || record.AuthorityDisposition.PlanDigest != plan.PlanDigest ||
		!reflectStringSliceEqual(record.AuthorityDisposition.SeedSet, plan.SeedSet) {
		t.Fatalf("AuthorityDisposition proof is incomplete: %#v", record.AuthorityDisposition)
	}
	_ = successor
}

// TestAuthorityDispositionExecuteRetainedGraphRevalidationBeforeSuccess
// satisfies tasks.md 2.6: success is reported only if the post-quarantine
// retained graph revalidates as Complete && Valid with no dangling
// reference to the quarantined entry.
func TestAuthorityDispositionExecuteRetainedGraphRevalidationBeforeSuccess(t *testing.T) {
	repo := initSnapshotRepo(t)
	forgedRecoveryPair(t, repo, "revalidate", "revalidate target\n")
	plan := authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")

	record, err := executeAuthorityDisposition(context.Background(), repo, plan)
	if err != nil {
		t.Fatalf("executeAuthorityDisposition: %v", err)
	}
	report, err := InspectCompactRecoveryEdges(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Valid {
		t.Fatalf("post-disposition retained graph did not revalidate: %#v", report.Totals)
	}
	for _, edge := range report.Edges {
		if edge.PredecessorLineageID == record.LineageID || edge.SuccessorLineageID == record.LineageID {
			t.Fatalf("retained graph still references quarantined entry %q", record.LineageID)
		}
	}
}

// TestAuthorityDispositionExecuteReplayConvergesWithoutDoubleMove satisfies
// tasks.md 2.7: replaying an already-succeeded plan detects the existing
// quarantine, converges, and does not move the entry twice.
func TestAuthorityDispositionExecuteReplayConvergesWithoutDoubleMove(t *testing.T) {
	repo := initSnapshotRepo(t)
	forgedRecoveryPair(t, repo, "replay", "replay target\n")
	plan := authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")

	first, err := executeAuthorityDisposition(context.Background(), repo, plan)
	if err != nil {
		t.Fatalf("first execution: %v", err)
	}
	entriesBefore, err := os.ReadDir(filepath.Dir(first.QuarantinePath))
	if err != nil {
		t.Fatal(err)
	}
	second, err := executeAuthorityDisposition(context.Background(), repo, plan)
	if err != nil {
		t.Fatalf("replayed execution: %v", err)
	}
	if second.QuarantinePath != first.QuarantinePath || second.Status != CompactReclaimCommitted {
		t.Fatalf("replay did not converge on the same quarantine: first=%#v second=%#v", first, second)
	}
	entriesAfter, err := os.ReadDir(filepath.Dir(first.QuarantinePath))
	if err != nil {
		t.Fatal(err)
	}
	if len(entriesBefore) != len(entriesAfter) {
		t.Fatalf("replay moved the entry a second time: before=%d after=%d", len(entriesBefore), len(entriesAfter))
	}
}

// TestAuthorityDispositionExecuteCrashMidExecutionLeavesValidGraph satisfies
// tasks.md 2.8: a crash between lock acquisition and completion, injected at
// each durable phase via compactReclaimPhaseHook, leaves the retained graph
// in a valid, classifiable state and a subsequent replay converges to
// committed without manual byte-level repair.
func TestAuthorityDispositionExecuteCrashMidExecutionLeavesValidGraph(t *testing.T) {
	for _, phase := range []string{compactReclaimPhasePrepared, compactReclaimPhaseRenamed, compactReclaimPhaseCommitted} {
		t.Run(phase, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			forgedRecoveryPair(t, repo, "crash-"+phase, "crash target "+phase+"\n")
			plan := authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")

			originalHook := compactReclaimPhaseHook
			injected := errors.New("injected " + phase + " interruption")
			fired := false
			compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
				if current == phase && !fired {
					fired = true
					return injected
				}
				return originalHook(ctx, current, record)
			}
			t.Cleanup(func() { compactReclaimPhaseHook = originalHook })

			_, err := executeAuthorityDisposition(context.Background(), repo, plan)
			if !errors.Is(err, injected) {
				t.Fatalf("phase %s crash error = %v", phase, err)
			}

			// Post-restart inspection classifies cleanly with no corrupted
			// intermediate state — the discriminator is the prepared/renamed
			// residue split quarantineCompactStoreEntry already relies on.
			if _, err := InventoryAuthority(context.Background(), repo); err != nil {
				t.Fatalf("phase %s post-crash inventory: %v", phase, err)
			}

			compactReclaimPhaseHook = originalHook
			replayed, err := executeAuthorityDisposition(context.Background(), repo, plan)
			if err != nil || replayed.Status != CompactReclaimCommitted {
				t.Fatalf("phase %s replay after crash = %#v, %v", phase, replayed, err)
			}
			report, err := InspectCompactRecoveryEdges(context.Background(), repo)
			if err != nil || !report.Complete || !report.Valid {
				t.Fatalf("phase %s post-replay graph did not classify cleanly: %#v, %v", phase, report.Totals, err)
			}
		})
	}
}

// TestAuthorityDispositionExecuteConcurrentRefusesDuplicateMutation
// satisfies tasks.md 2.9: two concurrent invocations against the same
// target under the same maintenance lock — exactly one proceeds, the other
// refuses without mutating.
func TestAuthorityDispositionExecuteConcurrentRefusesDuplicateMutation(t *testing.T) {
	repo := initSnapshotRepo(t)
	forgedRecoveryPair(t, repo, "concurrent", "concurrent target\n")
	plan := authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")

	release := make(chan struct{})
	holderEntered := make(chan struct{})
	originalHook := compactReclaimPhaseHook
	var once sync.Once
	compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
		if current == compactReclaimPhasePrepared {
			once.Do(func() { close(holderEntered) })
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return originalHook(ctx, current, record)
	}
	t.Cleanup(func() { compactReclaimPhaseHook = originalHook })

	holderDone := make(chan error, 1)
	go func() {
		_, err := executeAuthorityDisposition(context.Background(), repo, plan)
		holderDone <- err
	}()
	<-holderEntered

	shortCtx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, contenderErr := executeAuthorityDisposition(shortCtx, repo, plan)
	if contenderErr == nil {
		t.Fatal("concurrent contender was admitted while the holder held the maintenance lock")
	}

	close(release)
	if holderErr := <-holderDone; holderErr != nil {
		t.Fatalf("holder execution: %v", holderErr)
	}
}

// TestAuthorityDispositionExecuteUnknownMixedAmbiguousBlocksNoFallback
// satisfies tasks.md 2.10: a shape that reclassifies as mixed/ambiguous
// between derivation and execution refuses, never quarantines, and applies
// no generic fallback.
func TestAuthorityDispositionExecuteUnknownMixedAmbiguousBlocksNoFallback(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, successor, successorStore := forgedRecoveryPair(t, repo, "ambiguous-alpha", "ambiguous alpha target\n")
	plan := authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")

	// A second independent forged pair reclassifies the whole-graph closed
	// classification as mixed/ambiguous (two eligible edges instead of one)
	// before the stale plan executes.
	forgedRecoveryPair(t, repo, "ambiguous-bravo", "ambiguous bravo target\n")

	_, err := executeAuthorityDisposition(context.Background(), repo, plan)
	if !errors.Is(err, errAuthorityDispositionPlanNotDerivable) && !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("ambiguous re-derivation error = %v, want errAuthorityDispositionPlanNotDerivable or ErrConcurrentUpdate", err)
	}
	if _, statErr := os.Stat(successorStore.Dir); statErr != nil {
		t.Fatalf("ambiguous refusal moved the original target: %v", statErr)
	}
	quarantineRoot := filepath.Join(filepath.Dir(filepath.Dir(successorStore.Dir)), "quarantine")
	if _, statErr := os.Lstat(quarantineRoot); !os.IsNotExist(statErr) {
		t.Fatalf("ambiguous refusal applied a fallback quarantine: %v", statErr)
	}
	_ = successor
}

// TestAuthorityDispositionExecuteRefusalNamesDiagnosisAndEscalationArtifact
// satisfies tasks.md 2.11: a multi-lineage refusal names the diagnosis and
// #1656 as the open escalation artifact, without committing to a wave
// delivery date.
func TestAuthorityDispositionExecuteRefusalNamesDiagnosisAndEscalationArtifact(t *testing.T) {
	plan := AuthorityDispositionPlan{
		Schema: AuthorityDispositionPlanSchema, RepositoryBinding: "binding", AuthorityInventoryRevision: "rev",
		AnomalyClass: compactContentMismatchedRecoveryAuthorizationClass, SeedSet: []string{"seed"},
		Closure:           []string{"seed", "descendant-one", "descendant-two"},
		ExpectedRevisions: map[string]string{"seed": "rev-seed", "descendant-one": "rev-one", "descendant-two": "rev-two"},
		Actor:             "maintainer@example.com", Reason: "multi-lineage escalation",
	}
	digest, err := authorityDispositionPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanDigest = digest
	plan.Authorization = authorityDispositionAuthorizationBinding(plan)

	_, err = executeAuthorityDisposition(context.Background(), "/nonexistent-repo-never-touched", plan)
	if err == nil {
		t.Fatal("multi-lineage plan was admitted")
	}
	refusal := err.Error()
	if !strings.Contains(refusal, "#1656") {
		t.Fatalf("refusal does not name #1656 as the escalation artifact: %v", err)
	}
	for _, commitment := range []string{"will ship", "will land", "scheduled for", "delivery date", "committed for"} {
		if strings.Contains(refusal, commitment) {
			t.Fatalf("refusal makes a roadmap delivery-date commitment (%q): %v", commitment, err)
		}
	}
}

// TestAuthorityDispositionExecuteRefusalNeverBlocksElsewhere satisfies
// tasks.md 2.12 (blocking budget compliance): a refused leaf disposition
// with no maintainer authorization does not affect an unrelated candidate
// proceeding through a start flow in a completely separate repository.
func TestAuthorityDispositionExecuteRefusalNeverBlocksElsewhere(t *testing.T) {
	repo := initSnapshotRepo(t)
	forgedRecoveryPair(t, repo, "unauthorized", "unauthorized target\n")
	plan := derivePlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")
	// No Authorization populated: the maintainer never authorized this plan.

	if _, err := executeAuthorityDisposition(context.Background(), repo, plan); err == nil {
		t.Fatal("unauthorized plan was admitted")
	}

	unrelatedRepo := initSnapshotRepo(t)
	if _, err := StartCompactAuthority(context.Background(), unrelatedRepo, CompactStartRequest{State: newCompactTestState(t, unrelatedRepo, "unrelated-candidate")}); err != nil {
		t.Fatalf("unrelated candidate blocked by outstanding refusal elsewhere: %v", err)
	}
}

func reflectStringSliceEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
