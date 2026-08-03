package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// derivePlanFixture derives a plan against a live repo through the
// production seam (deriveAuthorityDispositionPlanAtRepo) and fails the test
// on any derivation error.
func derivePlanFixture(t *testing.T, repo, actor, reason string) AuthorityDispositionPlan {
	t.Helper()
	plan, err := deriveAuthorityDispositionPlanAtRepo(context.Background(), repo, actor, reason)
	if err != nil {
		t.Fatalf("deriveAuthorityDispositionPlanAtRepo: %v", err)
	}
	return plan
}

// TestAuthorityDispositionPlanFieldSet satisfies tasks.md 1.3 and
// rdd-authority-disposition-plan's "Plan Field Set" requirement: a plan
// derived for a repairable-classified graph carries all ten spec fields
// (repository_id...authorization), plus the permitted Schema field, per the
// design's spec-field-to-Go-field mapping table. Authorization stays empty
// until execution time (design decision 1).
func TestAuthorityDispositionPlanFieldSet(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, successor, _ := forgedRecoveryPair(t, repo, "field-set", "field set target\n")
	plan := derivePlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")

	if plan.Schema == "" || plan.RepositoryBinding == "" || plan.AuthorityInventoryRevision == "" ||
		plan.AnomalyClass == "" || len(plan.SeedSet) == 0 || len(plan.Closure) == 0 ||
		len(plan.ExpectedRevisions) == 0 || plan.PlanDigest == "" || plan.Actor == "" || plan.Reason == "" {
		t.Fatalf("plan is missing a required field: %#v", plan)
	}
	if plan.Authorization != "" {
		t.Fatalf("plan.Authorization must stay empty until execution time, got %q", plan.Authorization)
	}
	if len(plan.SeedSet) != 1 || plan.SeedSet[0] != successor.State.LineageID {
		t.Fatalf("seed set = %v, want [%q]", plan.SeedSet, successor.State.LineageID)
	}
	if plan.AnomalyClass != compactContentMismatchedRecoveryAuthorizationClass {
		t.Fatalf("anomaly class = %q, want %q", plan.AnomalyClass, compactContentMismatchedRecoveryAuthorizationClass)
	}
}

// TestAuthorityDispositionPlanDeterministicClosureDerivation satisfies
// tasks.md 1.4 and the "Deterministic Closure Derivation From the Graph
// Source of Record" requirement: the same graph state, derived twice with no
// change between derivations, yields identical ordered_seed_set and
// ordered_closure.
func TestAuthorityDispositionPlanDeterministicClosureDerivation(t *testing.T) {
	repo := initSnapshotRepo(t)
	forgedRecoveryPair(t, repo, "determinism", "determinism target\n")

	first := derivePlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")
	second := derivePlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")
	if !reflect.DeepEqual(first.SeedSet, second.SeedSet) || !reflect.DeepEqual(first.Closure, second.Closure) {
		t.Fatalf("closure derivation is not deterministic:\nfirst  = %#v\nsecond = %#v", first, second)
	}
}

// TestAuthorityDispositionPlanRequiresClosedClassification satisfies
// tasks.md 1.5 and the "Closed Anomaly Classification Required for
// Derivation" requirement: an unclassifiable or incomplete inspection
// produces no plan and a typed refusal — never a partial or generic
// fallback plan.
func TestAuthorityDispositionPlanRequiresClosedClassification(t *testing.T) {
	t.Run("empty graph has no eligible edge", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		report, records, err := loadCompactRecoveryRecords(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := deriveAuthorityDispositionPlan(report, records, "binding", "maintainer@example.com", "no eligible edge"); !errors.Is(err, errAuthorityDispositionPlanNotDerivable) {
			t.Fatalf("empty graph derivation error = %v, want errAuthorityDispositionPlanNotDerivable", err)
		}
	})
	t.Run("unclassifiable multi-lineage shape (#1656) has no anomaly class", func(t *testing.T) {
		report := CompactRecoveryInspectionReport{
			Complete: true, Valid: false,
			Edges: []CompactRecoveryEdgeInspection{
				{PredecessorLineageID: "missing", SuccessorLineageID: "dangling", Valid: false, Problems: []string{"missing predecessor"}},
			},
		}
		if _, err := deriveAuthorityDispositionPlan(report, map[string]CompactRecord{}, "binding", "maintainer@example.com", "unclassifiable"); !errors.Is(err, errAuthorityDispositionPlanNotDerivable) {
			t.Fatalf("unclassifiable shape derivation error = %v, want errAuthorityDispositionPlanNotDerivable", err)
		}
	})
	t.Run("incomplete inspection refuses regardless of edges", func(t *testing.T) {
		report := CompactRecoveryInspectionReport{
			Complete: false,
			EntryDiagnostics: []CompactRecoveryEntryDiagnostic{
				{LineageID: "broken-entry", Problem: compactInspectionEntryMalformed},
			},
		}
		if _, err := deriveAuthorityDispositionPlan(report, map[string]CompactRecord{}, "binding", "maintainer@example.com", "incomplete"); !errors.Is(err, errAuthorityDispositionPlanNotDerivable) {
			t.Fatalf("incomplete inspection derivation error = %v, want errAuthorityDispositionPlanNotDerivable", err)
		}
	})
}

// TestAuthorityDispositionPlanDigestDeterminism satisfies tasks.md 1.6 and
// the "Plan Digest Binds Exact Content" requirement: same records derive the
// same plan_digest; any change to ordered_closure, expected_revisions, or
// anomaly_class changes the digest; Authorization is excluded from the
// pre-image (seven-field digest, ten-field struct). See
// TestAuthorityDispositionPlanDigestExcludesActorAndReason for the
// actor/reason exclusion this same requirement mandates.
func TestAuthorityDispositionPlanDigestDeterminism(t *testing.T) {
	repo := initSnapshotRepo(t)
	forgedRecoveryPair(t, repo, "digest", "digest target\n")
	plan := derivePlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")
	again := derivePlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")
	if plan.PlanDigest != again.PlanDigest {
		t.Fatalf("same records produced different plan digests: %q vs %q", plan.PlanDigest, again.PlanDigest)
	}

	mutatedClosure := plan
	mutatedClosure.Closure = append(append([]string(nil), plan.Closure...), "extra-lineage")
	mutatedClosureDigest, err := authorityDispositionPlanDigest(mutatedClosure)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedClosureDigest == plan.PlanDigest {
		t.Fatal("changing ordered_closure did not change plan_digest")
	}

	mutatedRevisions := plan
	mutatedRevisions.ExpectedRevisions = make(map[string]string, len(plan.ExpectedRevisions))
	for lineage, revision := range plan.ExpectedRevisions {
		mutatedRevisions.ExpectedRevisions[lineage] = revision + "-changed"
	}
	mutatedRevisionsDigest, err := authorityDispositionPlanDigest(mutatedRevisions)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedRevisionsDigest == plan.PlanDigest {
		t.Fatal("changing expected_revisions did not change plan_digest")
	}

	mutatedClass := plan
	mutatedClass.AnomalyClass = "different_class"
	mutatedClassDigest, err := authorityDispositionPlanDigest(mutatedClass)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedClassDigest == plan.PlanDigest {
		t.Fatal("changing anomaly_class did not change plan_digest")
	}

	authorized := plan
	authorized.Authorization = "sha256:forged-authorization-does-not-belong-in-the-preimage"
	authorizedDigest, err := authorityDispositionPlanDigest(authorized)
	if err != nil {
		t.Fatal(err)
	}
	if authorizedDigest != plan.PlanDigest {
		t.Fatal("authorization leaked into the plan_digest pre-image")
	}
}

// TestAuthorityDispositionPlanDigestExcludesActorAndReason satisfies the
// "Plan Digest Binds Exact Content" requirement's actor/reason exclusion:
// Actor and Reason are execution-time provenance, not plan identity, so a
// plan derived read-only with no actor/reason (e.g. `review repair
// --preflight`) MUST publish the exact same plan_digest a later execution
// re-derives with the real actor/reason for the same graph state. This is
// the regression test for the S3/S4 defect where the preflight-published
// digest could never equal what any real execution re-derived.
func TestAuthorityDispositionPlanDigestExcludesActorAndReason(t *testing.T) {
	repo := initSnapshotRepo(t)
	forgedRecoveryPair(t, repo, "digest-actor-reason", "digest actor reason target\n")

	preflight := derivePlanFixture(t, repo, "", "")
	executed := derivePlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")

	if preflight.PlanDigest != executed.PlanDigest {
		t.Fatalf("preflight-published plan_digest %q does not match execution's re-derived plan_digest %q — actor/reason leaked into plan identity", preflight.PlanDigest, executed.PlanDigest)
	}

	mutatedActor := executed
	mutatedActor.Actor = "someone-else@example.com"
	mutatedActorDigest, err := authorityDispositionPlanDigest(mutatedActor)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedActorDigest != executed.PlanDigest {
		t.Fatal("changing actor changed plan_digest — actor must stay excluded from the pre-image")
	}

	mutatedReason := executed
	mutatedReason.Reason = "a completely different reason"
	mutatedReasonDigest, err := authorityDispositionPlanDigest(mutatedReason)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedReasonDigest != executed.PlanDigest {
		t.Fatal("changing reason changed plan_digest — reason must stay excluded from the pre-image")
	}
}

// TestAuthorityDispositionPlanAuthorizationBindsWithNoExpiry satisfies
// tasks.md 1.7 and "Authorization Binds to Digest and Revision, No
// Wall-Clock Expiry" (pending-confirmation assumption 1): a stale
// authority_inventory_revision refuses regardless of elapsed time; a valid
// revision proceeds with no expiry check anywhere in the validator.
func TestAuthorityDispositionPlanAuthorizationBindsWithNoExpiry(t *testing.T) {
	repo := initSnapshotRepo(t)
	forgedRecoveryPair(t, repo, "authz", "authz target\n")
	plan := derivePlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization")
	plan.Authorization = authorityDispositionAuthorizationBinding(plan)

	if err := validateAuthorityDispositionAuthorization(plan, plan.AuthorityInventoryRevision); err != nil {
		t.Fatalf("valid authorization refused: %v", err)
	}
	// Called again with the exact same revision: there is no elapsed-time
	// input to this function at all, so repeating it can never surface a
	// different (expiry) outcome.
	if err := validateAuthorityDispositionAuthorization(plan, plan.AuthorityInventoryRevision); err != nil {
		t.Fatalf("repeat validation with unchanged revision refused: %v", err)
	}
	if err := validateAuthorityDispositionAuthorization(plan, "sha256:drifted-inventory-revision"); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("stale revision error = %v, want ErrConcurrentUpdate", err)
	}

	forged := plan
	forged.Authorization = "sha256:not-a-real-binding"
	if err := validateAuthorityDispositionAuthorization(forged, plan.AuthorityInventoryRevision); err == nil {
		t.Fatal("forged authorization was admitted")
	}
}

// TestAuthorityDispositionPlanCardinalityIsExecutorPolicyNotShapeConstraint
// satisfies tasks.md 1.8 and "Cardinality Is an Executor Admission Policy,
// Not a Plan-Shape Constraint": the plan shape has no leaf-specific field —
// a cardinality-one and a cardinality-N closure use the identical Go struct.
func TestAuthorityDispositionPlanCardinalityIsExecutorPolicyNotShapeConstraint(t *testing.T) {
	single := AuthorityDispositionPlan{
		Schema: AuthorityDispositionPlanSchema, RepositoryBinding: "binding", AuthorityInventoryRevision: "rev",
		AnomalyClass: compactContentMismatchedRecoveryAuthorizationClass, SeedSet: []string{"seed"}, Closure: []string{"seed"},
		ExpectedRevisions: map[string]string{"seed": "rev-seed"}, Actor: "a", Reason: "r",
	}
	multi := single
	multi.Closure = []string{"seed", "descendant-one", "descendant-two"}
	multi.ExpectedRevisions = map[string]string{"seed": "rev-seed", "descendant-one": "rev-one", "descendant-two": "rev-two"}

	if reflect.TypeOf(single) != reflect.TypeOf(multi) {
		t.Fatal("cardinality-one and cardinality-N plans do not share the same Go shape")
	}
	singleDigest, err := authorityDispositionPlanDigest(single)
	if err != nil {
		t.Fatal(err)
	}
	multiDigest, err := authorityDispositionPlanDigest(multi)
	if err != nil {
		t.Fatal(err)
	}
	if singleDigest == multiDigest {
		t.Fatal("cardinality-one and cardinality-three closures produced the same digest")
	}
}

// TestAuthorityDispositionPlanNoNewPublicRepairVerb satisfies tasks.md 1.9
// and "No New Public Repair Verb" (pending-confirmation assumption 2): the
// Wave 2 CLI surface (internal/cli) introduces no new disposition command —
// deriveAuthorityDispositionPlanAtRepo stays package-private and reachable
// only through wiring a future slice adds to the pre-existing `review
// repair` verb. This reads internal/cli's source text directly (not an
// import — reviewtransaction cannot import internal/cli without a cycle,
// since internal/cli already imports reviewtransaction) to prove no new
// command name was introduced in this slice.
func TestAuthorityDispositionPlanNoNewPublicRepairVerb(t *testing.T) {
	cliDir := filepath.Join("..", "cli")
	entries, err := os.ReadDir(cliDir)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately excludes "review dispose" and "RunReviewDispose": those are
	// substrings of the existing, unrelated `review dispose-result` verb
	// (review_dispose_result.go), which disposes one preserved reviewer
	// result and has nothing to do with AuthorityDispositionPlan.
	forbidden := []string{
		"review dispose-authority", "review disposition-plan", "review authority-disposition",
		"RunReviewDisposeAuthority", "RunReviewDispositionPlan", "RunReviewAuthorityDisposition",
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(cliDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, word := range forbidden {
			if strings.Contains(string(payload), word) {
				t.Fatalf("%s introduces a new disposition command surface via %q", entry.Name(), word)
			}
		}
	}
}

// TestAuthorityDispositionPlanPR2111SupersessionProbe is the S1 probe named
// in design.md Open Question 1: "does #2111's fixture re-derive with a
// non-empty DispositionClass?" PR #2111 itself is not available inside this
// repository checkout or test fixture set, so this probe validates the exact
// shape the supersession rule describes — a content-mismatched recovery
// authorization edge whose successor is a leaf — using the same
// forgedRecoveryPair fixture the rest of this class's tests use. If PR
// #2111's actual fixture differs from this shape once it can be inspected,
// this probe documents that supersession stays withdrawn and the PR stays
// open for Wave 6, per design.md's stated fallback.
func TestAuthorityDispositionPlanPR2111SupersessionProbe(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, successor, _ := forgedRecoveryPair(t, repo, "pr2111", "pr2111 target\n")
	report, records, err := loadCompactRecoveryRecords(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	predecessorLineage := ""
	for _, edge := range report.Edges {
		if edge.SuccessorLineageID == successor.State.LineageID {
			predecessorLineage = edge.PredecessorLineageID
		}
	}
	if predecessorLineage == "" {
		t.Fatal("#2111-shaped fixture produced no recovery edge to probe")
	}
	classification := classifyCompactRecoveryEdgeAnomalies(records[predecessorLineage], records[successor.State.LineageID])
	if classification.DispositionClass == "" {
		t.Fatal("#2111-shaped fixture did not re-derive a non-empty DispositionClass — supersession stays withdrawn (design.md Open Question 1)")
	}
	leaf := true
	for _, edge := range report.Edges {
		if edge.PredecessorLineageID == successor.State.LineageID {
			leaf = false
		}
	}
	if !leaf {
		t.Fatal("#2111-shaped fixture's successor is not a leaf — supersession stays withdrawn (design.md Open Question 1)")
	}
}
