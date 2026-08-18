package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Wave 6 closure disposition journeys (Slice S5, ds09-ds12)
// ---------------------------------------------------------------------------
//
// The ds06/ds08 journeys above (Wave 2) prove the cardinality-one leaf
// disposition plan black-box. Wave 6 relaxes admission to N>=1 closed-class
// closures with descendant-first ordering, an ordered N-node transaction,
// and forward-only resume (internal/reviewtransaction Slices S1-S3) and a
// negotiated `review status --next-transition` route (Slice S4). These four
// journeys are that relaxation's own exit evidence: ds09 proves a real
// multi-hop (N=3) closure derives and disposes end-to-end; ds10 proves the
// over-collection guard holds for everything NOT in the closure; ds11 proves
// a closure interrupted mid-transaction resumes forward-only through the
// real binary; ds12 proves the negotiated route this axis's ds06/ds08 never
// exercised (they always drove --plan-digest/--inventory-revision straight
// from `review repair --preflight`, never from
// `review status --next-transition`).
//
// The closure shape here is a LINEAR chain (seed -> child -> grandchild),
// not a branching tree: `review recover` refuses a second successor from
// the same predecessor ("recovery predecessor already has successor"), a
// real product constraint discovered while building this fixture — a
// lineage can have at most one direct successor through the CLI, so a
// closure with more than one descendant can only ever be a chain, never a
// fork. This still exercises the multi-hop BFS/DFS walk
// authorityDispositionClosure performs (internal/reviewtransaction Slice
// S1's TestAuthorityDispositionClosureMultiChainAssumption already proves
// that walk correct for exactly this report-edge shape) and the N=3 ordered
// transaction (Slice S2) — only the "branching" framing in the original
// task naming does not exist as a constructible product state.
const (
	closureSeedLineage       = "review-damaged-closure-seed"
	closureChildLineage      = "review-damaged-closure-child"
	closureGrandchildLineage = "review-damaged-closure-grandchild"

	scratchClosureSeedLineage        = "damaged-store/closure-seed-lineage"
	scratchClosureSeedRevision       = "damaged-store/closure-seed-revision"
	scratchClosureChildLineage       = "damaged-store/closure-child-lineage"
	scratchClosureChildRevision      = "damaged-store/closure-child-revision"
	scratchClosureGrandchildRevision = "damaged-store/closure-grandchild-revision"
)

// multiHopClosureFixture builds a real, closed-classified 3-node LINEAR
// closure — predecessor -(forged, invalid)-> seed -(valid)-> child
// -(valid)-> grandchild — plus one unrelated approved witness lineage. It
// mirrors damagedLeafEligibleForDisposition's single-edge shape (ds06),
// extended to Wave 6's N>=2 closure, and damagedEdgePair's own 3-node
// predecessor/middle/successor construction order above (mint everything
// first, damage last).
//
// Every mint happens FIRST, while the whole graph is still valid:
// validateCompactRecoveryEdge runs at write time on `review recover` (file
// doc comment above), so the CLI refuses to mint a new edge from ANY
// predecessor once the graph already holds even one invalid edge elsewhere
// — the seed cannot be damaged before its descendants are minted. Damaging
// the seed LAST necessarily changes the seed's own file bytes (and
// therefore its revision — see damageRecordedReason), which would otherwise
// strand the child's already-recorded predecessor_revision/authorization
// against a predecessor that has since moved; realignRecoveryAuthorization
// re-signs the child's own authorization (never its target_identity, actor,
// or reason — only what actually changed) against the seed's post-damage
// revision so the child's edge stays genuinely valid. Realigning the child
// itself changes the CHILD's own revision, so the grandchild needs the same
// treatment, cascading one hop further — exactly the same cascade
// damagedEdgePair's own comment describes, except kept CONSISTENT at each
// step instead of deliberately left stale. This achieves, through the CLI's
// own write-time validation, the same end state
// internal/reviewtransaction's Go-level fixture reaches directly on disk
// (forgedRecoveryPair + forgedRecoveryDescendant, seed damaged before its
// descendant is even constructed) — the CLI simply requires reaching it in
// the opposite construction order.
func multiHopClosureFixture(sandbox *Sandbox) error {
	if err := approvedPredecessor(sandbox); err != nil {
		return err
	}
	if err := approvedUnrelatedDispositionWitness(sandbox); err != nil {
		return err
	}
	// widenWithProse (LOW risk, no lens capture) rather than widenWithCode:
	// the seed and the child here must become approved authority so each
	// can itself be a recovery predecessor for its descendant (mirroring
	// damagedEdgePair's "middle" lineage, ds01/ds07's own multi-hop shape)
	// — a different need than ds06's non-pristine leaf, which stays
	// "reviewing" forever because it is never itself a predecessor.
	// Pristine state has no bearing on plan derivation/admission/execution
	// (only on SanctionedCompactRecoveryExits' abandon-vs-repair
	// exit-naming, which ds09-ds11 do not exercise), so there is nothing to
	// trade away here.
	if err := mintSuccessor(sandbox, widenWithProse, scratchPredecessor, scratchPredecessorRevision, closureSeedLineage); err != nil {
		return err
	}
	if _, err := fixtureCommand(sandbox, "review", "finalize", "--cwd", sandbox.Repo, "--lineage", closureSeedLineage); err != nil {
		return err
	}
	seedRevision, err := approvedEntryRevision(sandbox, closureSeedLineage)
	if err != nil {
		return err
	}
	sandbox.Scratch[scratchClosureSeedLineage] = closureSeedLineage
	sandbox.Scratch[scratchClosureSeedRevision] = seedRevision

	if err := mintSuccessor(sandbox, stageProse("", "closure-hop-child"), scratchClosureSeedLineage, scratchClosureSeedRevision, closureChildLineage); err != nil {
		return err
	}
	if _, err := fixtureCommand(sandbox, "review", "finalize", "--cwd", sandbox.Repo, "--lineage", closureChildLineage); err != nil {
		return err
	}
	childRevision, err := approvedEntryRevision(sandbox, closureChildLineage)
	if err != nil {
		return err
	}
	sandbox.Scratch[scratchClosureChildLineage] = closureChildLineage
	sandbox.Scratch[scratchClosureChildRevision] = childRevision

	if err := mintSuccessor(sandbox, stageProse("", "closure-hop-grandchild"), scratchClosureChildLineage, scratchClosureChildRevision, closureGrandchildLineage); err != nil {
		return err
	}
	sandbox.Scratch[scratchClosureGrandchildRevision] = sandbox.Scratch[scratchSuccessorRevision]

	// Now, and only now, damage the predecessor->seed edge — child and
	// grandchild already exist as genuinely valid edges.
	damagedSeedRevision, err := damageRecordedReason(sandbox, closureSeedLineage, " (edited after the fact)")
	if err != nil {
		return err
	}
	sandbox.Scratch[scratchClosureSeedRevision] = damagedSeedRevision

	newChildRevision, err := realignRecoveryAuthorization(sandbox, closureChildLineage, closureSeedLineage, damagedSeedRevision)
	if err != nil {
		return err
	}
	sandbox.Scratch[scratchClosureChildRevision] = newChildRevision

	newGrandchildRevision, err := realignRecoveryAuthorization(sandbox, closureGrandchildLineage, closureChildLineage, newChildRevision)
	if err != nil {
		return err
	}
	sandbox.Scratch[scratchClosureGrandchildRevision] = newGrandchildRevision

	return requireClosureShape(sandbox)
}

// approvedEntryRevision reads back one lineage's current revision through
// `review status`, and fails loudly if it is not approved — the same proof
// mintSuccessor itself runs for a freshly recovered successor, reused here
// after finalize.
func approvedEntryRevision(sandbox *Sandbox, lineage string) (string, error) {
	status, err := proveStoreStatus(sandbox)
	if err != nil {
		return "", err
	}
	for _, entry := range status.Entries {
		if entry.LineageID == lineage {
			if entry.State != "approved" {
				return "", fmt.Errorf("fixture claims an approved %q but the product reports %q", lineage, entry.State)
			}
			return entry.Revision, nil
		}
	}
	return "", fmt.Errorf("fixture claims an approved %q but review status does not list it", lineage)
}

// realignRecoveryAuthorization re-signs successorLineage's recorded
// authorization against its predecessor's new (post-damage or
// post-realignment) revision — the successor's own target_identity, actor,
// and reason are read back unchanged from its own record, so this touches
// only the one field that actually changed (predecessor_revision) plus the
// authorization text that binds it, exactly mirroring
// compactRecoveryAuthorizationBinding's own six-field domain (schema,
// predecessor_lineage, predecessor_revision, target_identity, actor,
// reason). Unlike damageAuthorizationPredecessorRevision (which
// deliberately leaves the two inconsistent to simulate drift), this keeps
// them consistent, so the resulting edge is genuinely valid — the whole
// point being that only the seed's OWN incoming edge is invalid, not its
// descendants'.
func realignRecoveryAuthorization(sandbox *Sandbox, successorLineage, predecessorLineage, newPredecessorRevision string) (string, error) {
	path, err := storeStatePath(sandbox, successorLineage)
	if err != nil {
		return "", err
	}
	record, err := loadStoreRecord(path)
	if err != nil {
		return "", err
	}
	// target_identity is not its own field in CompactRecoveryProvenance —
	// it is bound only inside the rendered authorization text (mirrors
	// damageAuthorizationPredecessorRevision above, which treats the whole
	// authorization as a string too, never a target_identity JSON field).
	existingAuthorization, err := record.recoveryString("maintainer_authorization")
	if err != nil {
		return "", err
	}
	targetIdentity, err := authorizationFieldValue(existingAuthorization, "target_identity")
	if err != nil {
		return "", err
	}
	actor, err := record.recoveryString("actor")
	if err != nil {
		return "", err
	}
	reason, err := record.recoveryString("reason")
	if err != nil {
		return "", err
	}
	authorization := strings.Join([]string{
		damagedAuthorizationSchema,
		"predecessor_lineage=" + predecessorLineage,
		"predecessor_revision=" + newPredecessorRevision,
		"target_identity=" + targetIdentity,
		"actor=" + actor,
		"reason=" + reason,
	}, "\n")
	if err := record.setRecoveryString("predecessor_revision", newPredecessorRevision); err != nil {
		return "", err
	}
	if err := record.setRecoveryString("maintainer_authorization", authorization); err != nil {
		return "", err
	}
	return record.save()
}

// authorizationFieldValue extracts one "key=value" line's value from a
// rendered recovery authorization text (the domain-separated multi-line
// format compactRecoveryAuthorizationBinding renders — schema, then one
// "field=value" line per field).
func authorizationFieldValue(authorization, key string) (string, error) {
	prefix := key + "="
	for _, line := range strings.Split(authorization, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix), nil
		}
	}
	return "", fmt.Errorf("recorded authorization does not bind a %q field", key)
}

// requireClosureShape is multiHopClosureFixture's own proof: exactly three
// edges (predecessor->seed, seed->child, child->grandchild), exactly one
// invalid — the seed's own — the other two genuinely valid, and the store
// non-authoritative overall. It is the multi-hop analogue of
// requireInvalidEdges, which requires every edge invalid and cannot express
// a shape with valid descendant edges.
func requireClosureShape(sandbox *Sandbox) error {
	inspection, err := proveInspection(sandbox)
	if err != nil {
		return err
	}
	if inspection.Totals.Edges != 3 || inspection.Totals.InvalidEdges != 1 || inspection.Totals.ValidEdges != 2 {
		return fmt.Errorf("fixture claims a 1-invalid/2-valid three-edge closure shape but inspect-authority reports %+v", inspection.Totals)
	}
	invalidCount := 0
	for _, edge := range inspection.Edges {
		if !edge.Valid {
			invalidCount++
			if edge.SuccessorLineageID != closureSeedLineage {
				return fmt.Errorf("fixture claims the seed's own edge is the sole invalid one but %q is invalid instead", edge.SuccessorLineageID)
			}
			if len(edge.AnomalyClasses) != 0 {
				return fmt.Errorf("fixture's invalid edge now classifies as %v; the closure derivation this journey measures needs it outside every anomaly class", edge.AnomalyClasses)
			}
		}
	}
	if invalidCount != 1 {
		return fmt.Errorf("fixture claims exactly one invalid edge, inspect-authority reports %d", invalidCount)
	}
	return requireDamagedStoreReportsItsDamage(sandbox)
}

// closureMemberLineages is the fixture's whole closure, in construction
// (ancestor-first) order — the reverse of ordered_closure, which is
// descendant-first.
var closureMemberLineages = []string{closureSeedLineage, closureChildLineage, closureGrandchildLineage}

// requireDispositionClosureFullyQuarantined proves the WHOLE closure
// disposed, not only the seed: `review repair`'s committed response only
// ever names the seed's own LineageID (the returned record is the seed's),
// so the full-closure claim needs this direct filesystem check — the
// seed's, child's, and grandchild's own v2/ store directories must all be
// gone.
func requireDispositionClosureFullyQuarantined(r *journeyRun) error {
	for _, lineage := range closureMemberLineages {
		dir, err := storeLineageDir(r.sandbox, lineage)
		if err != nil {
			return err
		}
		if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
			return fmt.Errorf("closure member %q was not quarantined: its v2/ directory still exists", lineage)
		}
	}
	return nil
}

// requireUnrelatedPredecessorByteIdentical is ds10's second half of the
// over-collection guard, alongside requireDispositionWitnessBytesUnchanged:
// the top-level predecessor is not itself a closure member (only its
// outgoing edge into the seed is), so its own store bytes must never move
// either.
func requireUnrelatedPredecessorByteIdentical(r *journeyRun) error {
	lineage, err := scratchValue(r.sandbox, scratchPredecessor)
	if err != nil {
		return err
	}
	path, err := storeStatePath(r.sandbox, lineage)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return fmt.Errorf("the unrelated predecessor %q was itself moved during closure disposition: %w", lineage, statErr)
	}
	return nil
}

// reviewTransactionsBase resolves the same base directory
// internal/reviewtransaction's reviewAuthorityRoot derives — the git common
// directory's gentle-ai/review-transactions subtree — so ds11 can author a
// crash-position quarantine state directly, exactly like this axis's other
// fixtures author review-state.json directly (file doc comment above): this
// state (a real forward-only resume mid-transaction) is unreachable through
// the CLI, because a clean `review repair` run never stops halfway.
func reviewTransactionsBase(sandbox *Sandbox) (string, error) {
	common, err := gitCommonDir(sandbox, sandbox.Repo)
	if err != nil {
		return "", err
	}
	return filepath.Join(common, "gentle-ai", "review-transactions"), nil
}

// requireForgedResumeMovedNothingFurther is fix cycle 1's CRITICAL-1
// (security) journey-level mutation proof: a forged-authorization resume
// attempt against an in-progress closure must refuse through the real
// `review repair` binary before touching anything beyond the crash
// fixture's own pre-authored member — the grandchild's single quarantine
// directory must be unchanged, and child/seed must have none. This is the
// black-box, N=3, real-binary twin of
// TestAuthorityDispositionResumeRefusesForgedAuthorization.
func requireForgedResumeMovedNothingFurther(r *journeyRun) error {
	base, err := reviewTransactionsBase(r.sandbox)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for _, lineage := range closureMemberLineages {
			if strings.HasPrefix(entry.Name(), lineage+"-") {
				counts[lineage]++
			}
		}
	}
	if counts[closureGrandchildLineage] != 1 {
		return fmt.Errorf("grandchild quarantine directories after a refused forged-authorization resume = %d, want exactly 1 (unchanged from before the attempt)", counts[closureGrandchildLineage])
	}
	for _, lineage := range []string{closureChildLineage, closureSeedLineage} {
		if counts[lineage] != 0 {
			return fmt.Errorf("%q has %d quarantine directories after a refused forged-authorization resume, want 0 — the forged authorization moved something it should have refused", lineage, counts[lineage])
		}
	}
	return nil
}

// requireNoDoubleMoveAcrossClosure is ds11's resume-convergence proof:
// after the resumed `review repair` run, every closure member has exactly
// one quarantine directory — the crash-position fixture's pre-authored one
// for the grandchild was skipped, not re-processed, and child/seed were
// quarantined fresh exactly once each.
func requireNoDoubleMoveAcrossClosure(r *journeyRun) error {
	base, err := reviewTransactionsBase(r.sandbox)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for _, lineage := range closureMemberLineages {
			if strings.HasPrefix(entry.Name(), lineage+"-") {
				counts[lineage]++
			}
		}
	}
	for _, lineage := range closureMemberLineages {
		if counts[lineage] != 1 {
			return fmt.Errorf("closure member %q has %d quarantine directories after resume, want exactly 1 (a double-move or a skip that never happened)", lineage, counts[lineage])
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Wave 6 fix cycle 2: ds11 genuine crash-position coverage
// ---------------------------------------------------------------------------
//
// Fix cycle 1 shipped ds11 as a SINGLE journey that authored the crash state
// directly on disk (this axis's established "fixtures author
// review-state.json directly" convention) rather than genuinely interrupting
// a live process, and covered only one of the six ordered positions
// internal/reviewtransaction's own TestAuthorityDispositionResumeCrashPositionMatrix
// proves in-process. sdd-verify's cycle-2 WARNING named this as the wave's
// last open gap: the spec's "interrupt at each ordered position" is
// load-bearing at the journey layer specifically because this wave's S5
// found a real public-entrypoint defect Go-level tests structurally could
// not see (RepairAuthorityDisposition's own re-derivation, not
// executeAuthorityDisposition's internal resume logic) — so a journey that
// only authors state, or only covers one position, cannot rule out the same
// class of gap recurring at a position it never reaches.
//
// This is resolved by reusing the EXACT deterministic phase-hook
// interruption the Go matrix uses (compactReclaimPhaseHook), made reachable
// through the real binary via a build-tag-gated product hook
// (internal/reviewtransaction/bench_fixture.go, `-tags bench_fixture`,
// mirroring internal/sddstatus/bench_fixture.go's own established pattern
// for j57): GENTLE_AI_BENCH_CRASH_AT_PHASE names the exact "<phase>:<lineage>"
// pair to refuse right after, a genuine interruption of the real command
// with nothing after that point in the SAME process ever executing — not an
// authored on-disk state. Six journeys are generated, one per (phase,
// closure member) pair, each proving: the crash-inducing attempt genuinely
// refuses through the real binary; the resumed attempt then converges with
// no double move; and every closure member's quarantined residue bytes are
// byte-identical to that SAME member's own pre-disposition store bytes —
// the ground truth an uninterrupted run would also have to reproduce, since
// disposition only ever moves bytes, never rewrites them.

// crashPositionRole names one of the 3-node closure's members a
// crash-position journey can target.
type crashPositionRole struct {
	label   string
	lineage string
}

var crashPositionRoles = []crashPositionRole{
	{"grandchild", closureGrandchildLineage},
	{"child", closureChildLineage},
	{"seed", closureSeedLineage},
}

// crashPositionPhases reproduces, as plain strings, internal/reviewtransaction's
// own compactReclaimPhasePrepared / compactReclaimPhaseCommitted literals
// ("prepared" / "committed") — this package cannot import them, and
// GENTLE_AI_BENCH_CRASH_AT_PHASE's own contract (bench_fixture.go) is
// exactly these two literal values.
var crashPositionPhases = []string{"prepared", "committed"}

const benchFixtureCrashMarker = "bench_fixture: deterministic crash injected"

// crashInjectedDispositionRepairArgs sets the sandbox's crash trigger for
// exactly this one invocation (Sandbox.env reads it fresh per invoke) before
// delegating to dispositionRepairArgs for the actual argv.
func crashInjectedDispositionRepairArgs(phase, lineage, reason string) func(*Sandbox) ([]string, error) {
	inner := dispositionRepairArgs(reason)
	return func(sandbox *Sandbox) ([]string, error) {
		sandbox.BenchCrashAtPhase = phase + ":" + lineage
		return inner(sandbox)
	}
}

// clearedCrashDispositionRepairArgs clears any crash trigger left set by a
// prior step before the resume attempt, which must run to completion.
func clearedCrashDispositionRepairArgs(reason string) func(*Sandbox) ([]string, error) {
	inner := dispositionRepairArgs(reason)
	return func(sandbox *Sandbox) ([]string, error) {
		sandbox.BenchCrashAtPhase = ""
		return inner(sandbox)
	}
}

// requireGenuineBenchFixtureCrash is the crash-inducing step's After check.
// A nonzero exit alone is not enough — the executor also refuses for
// entirely unrelated, real reasons — so this requires the exact
// bench_fixture.go marker text, proof the interruption is the deterministic
// one this journey asked for. A binary without the bench_fixture build tag
// never links that hook, so GENTLE_AI_BENCH_CRASH_AT_PHASE has no effect and
// the disposition simply completes (exit 0) instead: that specific shape
// reports the journey unsupported rather than failed, mirroring j57's own
// graceful degradation when its equivalent build-tag seam is absent
// (axis_source_coupled.go).
func requireGenuineBenchFixtureCrash(_ *Sandbox, observation Observation) error {
	if observation.ExitCode != 0 && strings.Contains(observation.Stderr, benchFixtureCrashMarker) {
		return nil
	}
	if observation.ExitCode == 0 {
		return errSourceCoupledFixtureUnavailable
	}
	return fmt.Errorf("crash-inducing repair attempt exited %d without the expected bench_fixture marker: %s", observation.ExitCode, observation.Stderr)
}

// canonicalStoreDirectoryDigest hashes every regular file directly under dir
// (name and content both folded in, sorted by name) into one comparable
// string — enough to catch any byte difference, anywhere in the directory,
// without carrying every file's content through Sandbox.Scratch (a
// map[string]string) by hand.
func canonicalStoreDirectoryDigest(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		payload, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		hash.Write(payload)
		hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

const scratchCrashPositionSnapshotPrefix = "crashpos-snapshot/"

// captureCrashPositionSnapshot records each closure member's PRE-disposition
// store-directory digest — the ground truth its quarantined residue must
// still equal, byte for byte, after a genuine interrupt-then-resume.
func captureCrashPositionSnapshot(r *journeyRun) error {
	for _, lineage := range closureMemberLineages {
		dir, err := storeLineageDir(r.sandbox, lineage)
		if err != nil {
			return err
		}
		digest, err := canonicalStoreDirectoryDigest(dir)
		if err != nil {
			return fmt.Errorf("snapshot pre-disposition bytes for %q: %w", lineage, err)
		}
		r.sandbox.Scratch[scratchCrashPositionSnapshotPrefix+lineage] = digest
	}
	return nil
}

// crashPositionResidueDigest finds lineage's single post-disposition
// quarantine directory and digests its residue/ subtree exactly like
// canonicalStoreDirectoryDigest digests a live v2/ entry, so the two are
// directly comparable.
func crashPositionResidueDigest(base, lineage string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil {
		return "", err
	}
	var match string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), lineage+"-") {
			if match != "" {
				return "", fmt.Errorf("closure member %q has more than one quarantine directory", lineage)
			}
			match = entry.Name()
		}
	}
	if match == "" {
		return "", fmt.Errorf("closure member %q has no quarantine directory", lineage)
	}
	return canonicalStoreDirectoryDigest(filepath.Join(base, "quarantine", match, "residue"))
}

// requireCrashPositionConvergedByteIdentical is the resumed run's
// convergence proof: no double move (reusing requireNoDoubleMoveAcrossClosure),
// every closure member's residue bytes exactly matching its own
// pre-disposition snapshot, and a cleanly revalidating retained graph.
func requireCrashPositionConvergedByteIdentical(r *journeyRun) error {
	if err := requireNoDoubleMoveAcrossClosure(r); err != nil {
		return err
	}
	base, err := reviewTransactionsBase(r.sandbox)
	if err != nil {
		return err
	}
	for _, lineage := range closureMemberLineages {
		want, ok := r.sandbox.Scratch[scratchCrashPositionSnapshotPrefix+lineage]
		if !ok {
			return fmt.Errorf("no pre-disposition snapshot recorded for %q", lineage)
		}
		got, err := crashPositionResidueDigest(base, lineage)
		if err != nil {
			return fmt.Errorf("residue digest for %q: %w", lineage, err)
		}
		if got != want {
			return fmt.Errorf("closure member %q residue bytes do not match its own pre-disposition snapshot — resume did not converge byte-identically", lineage)
		}
	}
	inspection, err := proveInspection(r.sandbox)
	if err != nil {
		return err
	}
	return requireRetainedGraphValid(inspection)
}

// crashRecoveryPositionJourney builds one genuine-interrupt-then-resume
// journey for exactly one (phase, role) position. extraSteps, when given,
// are inserted between the crash attempt and the resume attempt — used only
// for the committed/grandchild position, which carries forward fix cycle 1's
// forged-authorization-on-resume mutation proof (the position the
// hand-authored ds11 originally covered).
func crashRecoveryPositionJourney(phase string, role crashPositionRole, extraSteps ...Step) Journey {
	steps := []Step{
		{Name: "fixture: one damaged seed with a two-hop descendant chain, plus an unrelated witness lineage",
			Fixture: multiHopClosureFixture},
		{Name: "ask what review repair would do", Requires: repairPreflightCapability,
			Args: productArgs("review", "repair", "--preflight=true"), After: requireDispositionPlanEligible},
		{Name: "snapshot every closure member's pre-disposition store bytes",
			Composite: captureCrashPositionSnapshot},
		{Name: "genuinely interrupt right after " + role.label + "'s " + phase + " phase, through the real binary",
			Requires: repairDispositionExecuteCapability,
			Args:     crashInjectedDispositionRepairArgs(phase, role.lineage, "quarantine the multi-hop closure"),
			After:    requireGenuineBenchFixtureCrash},
	}
	steps = append(steps, extraSteps...)
	steps = append(steps,
		Step{Name: "resume the interrupted closure with the identical plan", Requires: repairDispositionExecuteCapability,
			Args: clearedCrashDispositionRepairArgs("quarantine the multi-hop closure"), After: requireDispositionQuarantineCommitted},
		Step{Name: "every closure member converged byte-identically to its own pre-disposition bytes, with no double move, and the retained graph revalidates cleanly",
			Composite: requireCrashPositionConvergedByteIdentical},
	)
	return Journey{
		ID:     "ds11-crash-recovery-" + phase + "-" + role.label,
		Review: reviewOptedIn,
		Title:  "A closure genuinely interrupted right after its " + role.label + "'s " + phase + " phase resumes byte-identically through the real binary",
		Source: "rdd-root-simplification-wave6 fix cycle 2 (journey-level crash-position coverage, sdd-verify cycle-2 WARNING) — the real-binary twin of TestAuthorityDispositionResumeCrashPositionMatrix's " + phase + "/" + role.label + " case",
		Steps:  steps,
	}
}

// crashRecoveryPositionJourneys generates all 6 ordered positions
// (prepared/committed x grandchild/child/seed) a 3-node closure has, mirroring
// TestAuthorityDispositionResumeCrashPositionMatrix's own 2x3 table exactly.
func crashRecoveryPositionJourneys() []Journey {
	// Fix cycle 1 (CRITICAL-1, security): before Wave 6's fix,
	// validateAuthorityDispositionAuthorization was gated inside a
	// fresh-execution-only branch, so a real `review repair` call against
	// an in-progress closure executed unauthorized regardless of what
	// --authorization it was given. This step submits the CORRECT
	// --plan-digest/--inventory-revision (so CAS and plan-match both pass)
	// but an authorization bound to a repository identity that can never be
	// the real one, mirroring ds08's N=1 forged-authorization journey — the
	// N=3, mid-closure-resume twin. It belongs on the committed/grandchild
	// position specifically: that is the exact state (grandchild's own
	// two-phase move fully committed, nothing else touched) the original
	// single-position ds11 built by hand before this fix cycle replaced it.
	forgedAuthorizationSteps := []Step{
		{Name: "attempt to resume with an authorization bound to the wrong repository — refused, nothing moves further", Requires: repairDispositionExecuteCapability,
			Args: forgedDispositionRepairArgs("quarantine the multi-hop closure")},
		{Name: "the forged-authorization resume attempt moved nothing beyond the pre-authored member",
			Composite: requireForgedResumeMovedNothingFurther},
	}

	journeys := make([]Journey, 0, len(crashPositionPhases)*len(crashPositionRoles))
	for _, phase := range crashPositionPhases {
		for _, role := range crashPositionRoles {
			if phase == "committed" && role.label == "grandchild" {
				journeys = append(journeys, crashRecoveryPositionJourney(phase, role, forgedAuthorizationSteps...))
				continue
			}
			journeys = append(journeys, crashRecoveryPositionJourney(phase, role))
		}
	}
	return journeys
}

// closureDispositionJourneys returns Wave 6 Slice S5's exit-evidence
// journeys (ds09-ds12), appended to the Wave 2 damaged-store corpus above.
func closureDispositionJourneys() []Journey {
	journeys := []Journey{
		{
			ID:     "ds09-multi-chain-closure",
			Review: reviewOptedIn,
			Title:  "A multi-hop closure — one damaged seed with a two-hop descendant chain — derives and disposes end-to-end",
			Source: "rdd-root-simplification-wave6 Slices S1/S2 (topological ordering, ordered N-node transaction)",
			// ds06 above proves the N=1 base case. This is Wave 6's own
			// answer to the question ds06 could not ask: does the SAME
			// `review repair` verb, unchanged from the operator's
			// perspective, actually dispose a real N=3 closure spanning
			// more than one descendant hop from the seed?
			Steps: []Step{
				{Name: "fixture: one damaged seed with a two-hop descendant chain, plus an unrelated witness lineage",
					Fixture: multiHopClosureFixture},
				{Name: "inspect the authority, which is what an operator does first",
					Requires: inspectAuthorityCapability,
					Args:     productArgs("review", "inspect-authority"),
					After: inspectionAssertion("one edge outside every anomaly class, two valid descendant edges", func(inspection storeInspection) error {
						if inspection.Totals.InvalidEdges != 1 || inspection.Totals.ValidEdges != 2 {
							return fmt.Errorf("invalid_edges=%d valid_edges=%d, want 1 and 2", inspection.Totals.InvalidEdges, inspection.Totals.ValidEdges)
						}
						return nil
					})},
				{Name: "ask what review repair would do", Requires: repairPreflightCapability,
					Args: productArgs("review", "repair", "--preflight=true"), After: requireDispositionPlanEligible},
				{Name: "repair the whole closure through its disposition plan", Requires: repairDispositionExecuteCapability,
					Args: dispositionRepairArgs("quarantine the multi-hop closure"), After: requireDispositionQuarantineCommitted},
				{Name: "the authority graph after repair", Requires: inspectAuthorityCapability,
					Args:  productArgs("review", "inspect-authority"),
					After: inspectionAssertion("the retained graph after a multi-hop closure repair", requireRetainedGraphValid)},
				{Name: "the store governs again", Composite: proveStoreRecovered},
				{Name: "every closure member was quarantined, not only the seed", Composite: requireDispositionClosureFullyQuarantined},
			},
		},
		{
			ID:     "ds10-cross-lineage-closure",
			Review: reviewOptedIn,
			Title:  "The over-collection guard: everything NOT in the closure — the predecessor and an unrelated lineage — stays byte-identical",
			Source: "rdd-root-simplification-wave6 design decision D6 (over-collection guard)",
			// ds09 proves the closure disposes. This journey isolates the
			// complementary claim design decision D6 makes: closure
			// derivation reaches only report-edge-reachable descendants —
			// the top-level predecessor (upstream of the seed, never a
			// closure member) and a wholly unrelated approved lineage both
			// keep byte-identical store bytes across the exact same
			// disposition.
			Steps: []Step{
				{Name: "fixture: one damaged seed with a two-hop descendant chain, plus an unrelated witness lineage",
					Fixture: multiHopClosureFixture},
				{Name: "ask what review repair would do", Requires: repairPreflightCapability,
					Args: productArgs("review", "repair", "--preflight=true"), After: requireDispositionPlanEligible},
				{Name: "repair the whole closure through its disposition plan", Requires: repairDispositionExecuteCapability,
					Args: dispositionRepairArgs("quarantine the multi-hop closure"), After: requireDispositionQuarantineCommitted},
				{Name: "every closure member was quarantined, not only the seed", Composite: requireDispositionClosureFullyQuarantined},
				{Name: "the unrelated witness lineage never moved", Composite: requireDispositionWitnessBytesUnchanged},
				{Name: "the closure's own predecessor never moved either", Composite: requireUnrelatedPredecessorByteIdentical},
			},
		},
		{
			ID:     "ds12-negotiated-transition-route",
			Review: reviewOptedIn,
			Title:  "The negotiated route: `review status --next-transition` surfaces the closure disposition collect/execute, not a raw flag triad",
			Source: "rdd-root-simplification-wave6 Slice S4/D7 (negotiated transition route)",
			// ds06/ds08/ds09/ds10/ds11 above all drive --plan-digest and
			// --inventory-revision straight from `review repair
			// --preflight`'s dedicated JSON fields — a caller who already
			// knows the disposition surface exists. This journey drives the
			// SAME repair verb through the generic negotiated surface every
			// other lifecycle transition in this product already uses:
			// `review status --next-transition` offers collect{} naming the
			// same two values, then execute{review.repair, ...} once actor/
			// reason/authorization are supplied — proving Slice S4 actually
			// reaches a caller who never knew `review repair --preflight`
			// existed.
			Steps: []Step{
				{Name: "fixture: one damaged recovery edge, non-pristine successor, plus an unrelated witness lineage",
					Fixture: damagedLeafEligibleForDisposition},
				{Name: "ask the negotiated surface what happens next for the damaged leaf", Requires: statusCapability,
					// The selector is the whole point after authority
					// validation became lineage-scoped: an operator asking
					// what happens next for their own live candidate is now
					// answered about that candidate (ds13), so reaching the
					// disposition route means naming the entry the
					// disposition is about. The route itself is unchanged --
					// the same collect, the same two values, the same execute
					// form -- which is what this journey has always measured.
					Args: productArgs("review", "status", "--contract", reviewContract, "--next-transition",
						"--lineage", damagedSuccessor),
					After: func(sandbox *Sandbox, observation Observation) error {
						var envelope statusEnvelope
						if err := decodeWaveObservation(observation, &envelope, "review status --next-transition over a closed content-mismatched leaf"); err != nil {
							return err
						}
						if envelope.NextTransition.Kind != "collect" || len(envelope.NextTransition.Collect.Inputs) == 0 ||
							envelope.NextTransition.Collect.Inputs[0].Name != "disposition_authorization" {
							return fmt.Errorf("next_transition = %+v, want a disposition_authorization collect", envelope.NextTransition)
						}
						planDigest := envelope.argument("plan-digest")
						inventoryRevision := envelope.argument("inventory-revision")
						if !validDispositionSHA256(planDigest) || !validDispositionSHA256(inventoryRevision) {
							return fmt.Errorf("negotiated collect did not carry a valid plan_digest/inventory_revision preview: %+v", envelope.NextTransition.Collect.Inputs[0].Arguments)
						}
						sandbox.Scratch[scratchDispositionPlanDigest] = planDigest
						sandbox.Scratch[scratchDispositionInventoryRevision] = inventoryRevision
						return nil
					}},
				{Name: "preview the execute form once actor/reason/authorization are supplied", Requires: statusCapability,
					Args: func(sandbox *Sandbox) ([]string, error) {
						planDigest, inventoryRevision, binding, err := dispositionRepairExecutionInputs(sandbox)
						if err != nil {
							return nil, err
						}
						const actor, reason = "bench", "quarantine the content-mismatched leaf"
						authorization := dispositionAuthorization(binding, planDigest, inventoryRevision, actor, reason)
						return []string{
							"review", "status", "--cwd", sandbox.Repo, "--contract", reviewContract, "--next-transition",
							"--lineage", damagedSuccessor,
							"--repair-actor", actor, "--repair-reason", reason, "--repair-authorization", authorization,
						}, nil
					},
					After: func(_ *Sandbox, observation Observation) error {
						var envelope statusEnvelope
						if err := decodeWaveObservation(observation, &envelope, "review status --next-transition with disposition authorization supplied"); err != nil {
							return err
						}
						if envelope.NextTransition.Kind != "execute" || envelope.NextTransition.Execute.Operation != "review.repair" {
							return fmt.Errorf("next_transition = %+v, want an execute of review.repair", envelope.NextTransition)
						}
						for _, want := range []string{"--plan-digest", "--inventory-revision", "--actor", "--reason", "--authorization"} {
							if !strings.Contains(envelope.NextTransition.Execute.Command, want) {
								return fmt.Errorf("negotiated execute command is missing %q: %q", want, envelope.NextTransition.Execute.Command)
							}
						}
						return nil
					}},
				{Name: "run the repair the negotiated route named", Requires: repairDispositionExecuteCapability,
					Args: dispositionRepairArgs("quarantine the content-mismatched leaf"), After: requireDispositionQuarantineCommitted},
				{Name: "the authority graph after the negotiated repair", Requires: inspectAuthorityCapability,
					Args:  productArgs("review", "inspect-authority"),
					After: inspectionAssertion("the retained graph after the negotiated route's repair", requireRetainedGraphValid)},
				{Name: "the store governs again", Composite: proveStoreRecovered},
				{Name: "the unrelated witness lineage never moved", Composite: requireDispositionWitnessBytesUnchanged},
			},
		},
	}
	return append(journeys, crashRecoveryPositionJourneys()...)
}
