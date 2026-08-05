package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// quarantineEntriesForLineage lists every quarantine subdirectory name
// prefixed lineage+"-" under base's quarantine root — the mutation-proof
// primitive for "a committed node must never be re-processed": a correct
// forward-only resume produces exactly one quarantine directory per closure
// member, no matter how many times execution is interrupted and replayed.
func quarantineEntriesForLineage(t *testing.T, base, lineage string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), lineage+"-") {
			matches = append(matches, entry.Name())
		}
	}
	return matches
}

// residueBytesForLineage reads every file under lineage's own quarantine
// residue/ directory, keyed by filename, for byte-comparison across an
// uninterrupted baseline run and an interrupted-then-resumed run.
func residueBytesForLineage(t *testing.T, base, lineage string) map[string]string {
	t.Helper()
	matches := quarantineEntriesForLineage(t, base, lineage)
	if len(matches) != 1 {
		t.Fatalf("expected exactly one quarantine directory for %q, found %v", lineage, matches)
	}
	residueDir := filepath.Join(base, "quarantine", matches[0], "residue")
	entries, err := os.ReadDir(residueDir)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(residueDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[entry.Name()] = string(payload)
	}
	return out
}

// TestAuthorityDispositionResumeSkipsCommittedAndFinishesPrepared satisfies
// tasks.md 3.1/3.2: replaying the same plan_digest after an interruption
// skips the already-committed descendant and completes the remaining
// closure members in order, converging to a fully committed closure with
// exactly one quarantine directory per member (no double-move).
func TestAuthorityDispositionResumeSkipsCommittedAndFinishesPrepared(t *testing.T) {
	repo := initSnapshotRepo(t)
	plan, child, grandchild := threeNodeClosureFixture(t, repo, "resume")
	seed := plan.Closure[len(plan.Closure)-1]

	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}

	originalHook := compactReclaimPhaseHook
	injected := errors.New("injected interruption after grandchild commits")
	compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
		if current == compactReclaimPhaseCommitted && record.LineageID == grandchild.State.LineageID {
			return injected
		}
		return originalHook(ctx, current, record)
	}
	t.Cleanup(func() { compactReclaimPhaseHook = originalHook })

	_, err = executeAuthorityDisposition(context.Background(), repo, plan)
	if !errors.Is(err, injected) {
		t.Fatalf("first (interrupted) execution error = %v, want the injected interruption", err)
	}
	if got := quarantineEntriesForLineage(t, base, grandchild.State.LineageID); len(got) != 1 {
		t.Fatalf("grandchild quarantine entries after interruption = %v, want exactly 1", got)
	}

	compactReclaimPhaseHook = originalHook
	record, err := executeAuthorityDisposition(context.Background(), repo, plan)
	if err != nil {
		t.Fatalf("resumed execution: %v", err)
	}
	if record.Status != CompactReclaimCommitted || record.LineageID != seed {
		t.Fatalf("resumed execution did not converge on the committed seed: %#v", record)
	}
	for _, lineage := range []string{grandchild.State.LineageID, child.State.LineageID, seed} {
		if got := quarantineEntriesForLineage(t, base, lineage); len(got) != 1 {
			t.Fatalf("%q has %v quarantine entries after resume, want exactly 1 (a double-move or re-processed committed node)", lineage, got)
		}
	}
	report, err := InspectCompactRecoveryEdges(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Valid {
		t.Fatalf("post-resume retained graph did not revalidate cleanly: %#v", report.Totals)
	}
}

// TestAuthorityDispositionResumeNeverReprocessesCommittedNode is the
// skip-committed mutation proof the coordinator's batch explicitly requires:
// once a node is committed, a resumed execution must never touch its source
// path again. This asserts the STRONGER claim directly — the committed
// node's SourcePath never exists again after resume (a broken skip-check
// that re-ran the two-phase quarantine on an already-quarantined lineage
// would either error on a missing source directory or, worse, silently
// operate on a resurrected one).
func TestAuthorityDispositionResumeNeverReprocessesCommittedNode(t *testing.T) {
	repo := initSnapshotRepo(t)
	plan, _, grandchild := threeNodeClosureFixture(t, repo, "no-reprocess")

	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	grandchildSourcePath := filepath.Join(base, "v2", grandchild.State.LineageID)

	originalHook := compactReclaimPhaseHook
	injected := errors.New("injected interruption after grandchild commits")
	fired := false
	compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
		if current == compactReclaimPhaseCommitted && record.LineageID == grandchild.State.LineageID && !fired {
			fired = true
			return injected
		}
		return originalHook(ctx, current, record)
	}
	t.Cleanup(func() { compactReclaimPhaseHook = originalHook })

	if _, err := executeAuthorityDisposition(context.Background(), repo, plan); !errors.Is(err, injected) {
		t.Fatalf("first execution error = %v, want injected interruption", err)
	}
	if _, statErr := os.Stat(grandchildSourcePath); !os.IsNotExist(statErr) {
		t.Fatalf("grandchild source path survived its own commit: %v", statErr)
	}

	compactReclaimPhaseHook = originalHook
	if _, err := executeAuthorityDisposition(context.Background(), repo, plan); err != nil {
		t.Fatalf("resumed execution: %v", err)
	}
	// A broken skip-check would try to quarantine grandchildSourcePath again,
	// which — since it does not exist — resurrects it as an empty directory
	// via os.ReadDir + quarantine bookkeeping, or errors. Either way, its
	// continued absence after a SUCCESSFUL resume proves it was skipped, not
	// re-processed.
	if _, statErr := os.Stat(grandchildSourcePath); !os.IsNotExist(statErr) {
		t.Fatalf("grandchild source path was resurrected by a re-processed (not skipped) committed node: %v", statErr)
	}
}

// TestAuthorityDispositionResumeAmbiguousResidueRefusesPerNode extends
// Wave 2's single-seed "ambiguous prepared residue state" refusal
// (resumeAuthorityDispositionRecord's sourceExists==residueExists check) to
// a NON-SEED closure member: the discriminator is per-node, not
// seed-specific.
func TestAuthorityDispositionResumeAmbiguousResidueRefusesPerNode(t *testing.T) {
	repo := initSnapshotRepo(t)
	plan, child, grandchild := threeNodeClosureFixture(t, repo, "ambiguous")

	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}

	originalHook := compactReclaimPhaseHook
	injected := errors.New("injected interruption after grandchild residue rename")
	compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
		if current == compactReclaimPhaseRenamed && record.LineageID == grandchild.State.LineageID {
			return injected
		}
		return originalHook(ctx, current, record)
	}
	t.Cleanup(func() { compactReclaimPhaseHook = originalHook })

	if _, err := executeAuthorityDisposition(context.Background(), repo, plan); !errors.Is(err, injected) {
		t.Fatalf("first execution error = %v, want injected interruption", err)
	}
	compactReclaimPhaseHook = originalHook

	// Manufacture the ambiguous state: the grandchild's residue/ was already
	// renamed (interrupted right after), so recreate a directory at its
	// original source path too, making sourceExists == residueExists == true
	// for grandchild specifically (not the seed).
	entries := quarantineEntriesForLineage(t, base, grandchild.State.LineageID)
	if len(entries) != 1 {
		t.Fatalf("grandchild quarantine entries = %v, want exactly 1", entries)
	}
	grandchildSourcePath := filepath.Join(base, "v2", grandchild.State.LineageID)
	if err := os.MkdirAll(grandchildSourcePath, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = executeAuthorityDisposition(context.Background(), repo, plan)
	if err == nil || !strings.Contains(err.Error(), "ambiguous prepared residue state") {
		t.Fatalf("ambiguous per-node residue execution error = %v, want the ambiguous-residue refusal", err)
	}
	_ = child
}

// TestAuthorityDispositionResumeDigestMismatchRefusesNamingQuarantinePath
// satisfies tasks.md 3.3: a closure member already quarantined under a
// DIFFERENT plan_digest than the one currently submitted refuses — no
// narrowing re-derivation is attempted — and the refusal names a concrete,
// escalatable path (the quarantine directory holding that member, since no
// closure-manifest.json exists yet for a still-incomplete closure).
func TestAuthorityDispositionResumeDigestMismatchRefusesNamingQuarantinePath(t *testing.T) {
	repo := initSnapshotRepo(t)
	plan, _, grandchild := threeNodeClosureFixture(t, repo, "digest-mismatch")

	// Complete the grandchild's own two-phase quarantine under the CURRENT
	// plan digest, then interrupt right after — this is a normal, honest
	// partial closure, matching the earlier resume tests.
	originalHook := compactReclaimPhaseHook
	injected := errors.New("injected interruption after grandchild commits")
	compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
		if current == compactReclaimPhaseCommitted && record.LineageID == grandchild.State.LineageID {
			return injected
		}
		return originalHook(ctx, current, record)
	}
	t.Cleanup(func() { compactReclaimPhaseHook = originalHook })
	if _, err := executeAuthorityDisposition(context.Background(), repo, plan); !errors.Is(err, injected) {
		t.Fatalf("first execution error = %v, want injected interruption", err)
	}
	compactReclaimPhaseHook = originalHook

	// Now simulate "the world changed": submit a plan bearing a DIFFERENT
	// plan_digest for the exact same closure shape. Forward-only resume
	// must never treat grandchild's now-missing v2/ entry as "fresh" just
	// because it doesn't match the new digest.
	staleDigestPlan := plan
	staleDigestPlan.PlanDigest = "sha256:a-completely-different-plan-digest-simulating-drift"
	staleDigestPlan.Authorization = authorityDispositionAuthorizationBinding(staleDigestPlan)

	_, err := executeAuthorityDisposition(context.Background(), repo, staleDigestPlan)
	if err == nil {
		t.Fatal("digest-mismatched resume was admitted")
	}
	if !strings.Contains(err.Error(), grandchild.State.LineageID) {
		t.Fatalf("digest-mismatch refusal does not name the affected closure member %q: %v", grandchild.State.LineageID, err)
	}
	if !strings.Contains(err.Error(), "quarantine") {
		t.Fatalf("digest-mismatch refusal does not name an escalatable quarantine path: %v", err)
	}
	for _, commitment := range []string{"will ship", "will land", "scheduled for", "narrowing"} {
		if strings.Contains(err.Error(), commitment) && commitment != "narrowing" {
			t.Fatalf("digest-mismatch refusal makes an unexpected commitment (%q): %v", commitment, err)
		}
	}
}

// TestAuthorityDispositionResumeStalePlanRefusesNotProceeds is the
// digest-check mutation proof the coordinator's batch explicitly requires:
// resuming with a plan whose PlanDigest field itself is stale (does not
// match what re-derivation under lock currently produces) must refuse, and
// must NOT proceed to move anything — not even the nodes that would
// otherwise still be untouched.
func TestAuthorityDispositionResumeStalePlanRefusesNotProceeds(t *testing.T) {
	repo := initSnapshotRepo(t)
	plan, child, grandchild := threeNodeClosureFixture(t, repo, "stale-plan")

	stale := plan
	stale.PlanDigest = "sha256:stale-plan-digest-does-not-match-current-graph-state"
	stale.Authorization = authorityDispositionAuthorizationBinding(stale)

	_, err := executeAuthorityDisposition(context.Background(), repo, stale)
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("stale-plan-digest execution error = %v, want ErrConcurrentUpdate", err)
	}

	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, lineage := range []string{grandchild.State.LineageID, child.State.LineageID, plan.Closure[len(plan.Closure)-1]} {
		if got := quarantineEntriesForLineage(t, base, lineage); len(got) != 0 {
			t.Fatalf("stale-plan execution moved %q despite refusing: %v", lineage, got)
		}
	}
}

// TestRepairAuthorityDispositionResumesThroughPublicEntrypoint proves the
// SAME forward-only resume works through RepairAuthorityDisposition — the
// public entrypoint internal/cli's `review repair` calls — not only through
// executeAuthorityDisposition called directly, as every other resume test
// in this file does. RepairAuthorityDisposition previously always
// re-derived the plan fresh from actor/reason on every call, discarding the
// caller-supplied plan_digest/inventory_revision entirely; once ANY closure
// member is already quarantined, that fresh re-derivation necessarily
// produces a smaller, mismatched closure (a missing member contributes no
// report edge), so a genuine crash-then-resume through this public
// entrypoint always refused, even though executeAuthorityDisposition's own
// internal resume logic (this file's other tests) was already correct.
// discovered via bench's ds11-crash-recovery-mid-closure journey (the
// black-box twin of TestAuthorityDispositionResumeCrashPositionMatrix),
// which is the only place this exact call path — through
// RepairAuthorityDisposition, not executeAuthorityDisposition directly —
// was ever exercised.
func TestRepairAuthorityDispositionResumesThroughPublicEntrypoint(t *testing.T) {
	repo := initSnapshotRepo(t)
	plan, _, grandchild := threeNodeClosureFixture(t, repo, "public-resume")

	originalHook := compactReclaimPhaseHook
	injected := errors.New("injected interruption after grandchild commits")
	compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
		if current == compactReclaimPhaseCommitted && record.LineageID == grandchild.State.LineageID {
			return injected
		}
		return originalHook(ctx, current, record)
	}
	t.Cleanup(func() { compactReclaimPhaseHook = originalHook })

	_, err := RepairAuthorityDisposition(context.Background(), repo, plan.PlanDigest, plan.AuthorityInventoryRevision, plan.Actor, plan.Reason, plan.Authorization)
	if !errors.Is(err, injected) {
		t.Fatalf("first (interrupted) public execution error = %v, want the injected interruption", err)
	}

	compactReclaimPhaseHook = originalHook
	record, err := RepairAuthorityDisposition(context.Background(), repo, plan.PlanDigest, plan.AuthorityInventoryRevision, plan.Actor, plan.Reason, plan.Authorization)
	if err != nil {
		t.Fatalf("resumed public execution through RepairAuthorityDisposition: %v", err)
	}
	if record.Status != CompactReclaimCommitted || record.LineageID != plan.Closure[len(plan.Closure)-1] {
		t.Fatalf("resumed public execution did not converge on the committed seed: %#v", record)
	}
}

// TestAuthorityDispositionResumeRefusesForgedAuthorization is fix cycle 1's
// CRITICAL-1 (security) mutation proof: a resumed closure execution must
// validate Authorization on EVERY execution path, not only a fresh one.
// lockedAuthorityDispositionMutation previously gated
// validateAuthorityDispositionAuthorization inside the `if freshExecution`
// block — its ONLY non-test call site — so every resume executed
// unauthorized: an attacker who knew nothing but an in-progress plan_digest
// could resume with a garbage Authorization and a different Actor, and it
// would commit every remaining closure member. This interrupts a real
// 3-node closure exactly like
// TestAuthorityDispositionResumeSkipsCommittedAndFinishesPrepared, then
// resumes with Authorization replaced by garbage and Actor replaced by
// "attacker" (mirroring the verifier's exact repro): this must refuse, and
// nothing beyond the grandchild's already-committed member may move. If
// authorization validation is ever re-gated inside a fresh-only branch
// again, this test fails immediately (the forged resume would be admitted).
func TestAuthorityDispositionResumeRefusesForgedAuthorization(t *testing.T) {
	repo := initSnapshotRepo(t)
	plan, child, grandchild := threeNodeClosureFixture(t, repo, "forged-resume")
	seed := plan.Closure[len(plan.Closure)-1]

	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}

	originalHook := compactReclaimPhaseHook
	injected := errors.New("injected interruption after grandchild commits")
	compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
		if current == compactReclaimPhaseCommitted && record.LineageID == grandchild.State.LineageID {
			return injected
		}
		return originalHook(ctx, current, record)
	}
	t.Cleanup(func() { compactReclaimPhaseHook = originalHook })

	if _, err := executeAuthorityDisposition(context.Background(), repo, plan); !errors.Is(err, injected) {
		t.Fatalf("first (interrupted) execution error = %v, want the injected interruption", err)
	}
	compactReclaimPhaseHook = originalHook

	forged := plan
	forged.Actor = "attacker"
	forged.Authorization = "garbage authorization forged by an attacker"

	if _, err := executeAuthorityDisposition(context.Background(), repo, forged); err == nil {
		t.Fatal("resumed execution with a forged authorization and a different actor was admitted")
	} else if !strings.Contains(err.Error(), "authorization does not bind") {
		t.Fatalf("forged-authorization resume error = %v, want the authorization binding refusal", err)
	}

	for _, lineage := range []string{child.State.LineageID, seed} {
		if got := quarantineEntriesForLineage(t, base, lineage); len(got) != 0 {
			t.Fatalf("forged-authorization resume moved %q despite refusing: %v", lineage, got)
		}
	}
	if got := quarantineEntriesForLineage(t, base, grandchild.State.LineageID); len(got) != 1 {
		t.Fatalf("grandchild quarantine entries after refused forged resume = %v, want exactly 1 (unchanged)", got)
	}
}

// TestAuthorityDispositionResumeCrashPositionMatrix is the load-bearing
// proof tasks.md 3.5 and the coordinator's batch require: for a 3-node
// closure, interrupting at EVERY position — after each node's prepared
// phase and after each node's committed phase (6 positions total) — and
// then resuming converges to the identical final state an uninterrupted run
// produces: byte-identical residue content per closure member, a cleanly
// classifying retained graph, and byte-identical unrelated-witness-lineage
// bytes (the over-collection guard's runtime half, asserted per resume).
func TestAuthorityDispositionResumeCrashPositionMatrix(t *testing.T) {
	positions := []string{compactReclaimPhasePrepared, compactReclaimPhaseCommitted}
	nodeRoles := []string{"grandchild", "child", "seed"}

	// buildFixture constructs an identical closure fixture (repo + witness +
	// three-node closure) from the SAME suffix, so a "control" (uninterrupted)
	// and an "experiment" (interrupted-then-resumed) run derive the exact
	// same lineage IDs and target-file content — the only two things
	// residue bytes can legitimately depend on — and are therefore
	// byte-comparable. reclaim-record.json's own ReclaimedAt timestamp is
	// deliberately excluded from the comparison (residueBytesForLineage
	// reads only residue/, the byte-preserved original entry, never the
	// audit record).
	buildFixture := func(t *testing.T, suffix string) (repo string, plan AuthorityDispositionPlan, child, grandchild CompactRecord, witnessStore CompactStore) {
		t.Helper()
		repo = initSnapshotRepo(t)
		// The witness must exist BEFORE the plan is derived, exactly like
		// TestAuthorityDispositionExecuteUnrelatedLineageByteIdenticalAcrossClosure:
		// authority_inventory_revision covers every loaded record, so adding
		// one after derivation would trip the unrelated "plan digest
		// drifted" staleness guard on the very first execution attempt.
		witness := newCompactTestState(t, repo, "matrix-witness-"+suffix)
		var err error
		witnessStore, err = CompactAuthoritativeStore(context.Background(), repo, witness.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		writeCompactFixtureRecord(t, witnessStore, witness)
		plan, child, grandchild = threeNodeClosureFixture(t, repo, suffix)
		return repo, plan, child, grandchild, witnessStore
	}

	for _, phase := range positions {
		for _, role := range nodeRoles {
			t.Run(phase+"-"+role, func(t *testing.T) {
				suffix := "matrix-" + phase + "-" + role

				// Control: an uninterrupted run of the IDENTICAL fixture
				// (same suffix, hence same lineage IDs and target content —
				// control and experiment live in separate temp repos, so
				// nothing prevents reusing the identical suffix/lineage IDs
				// across both).
				controlRepo, controlPlan, controlChild, controlGrandchild, _ := buildFixture(t, suffix)
				if _, err := executeAuthorityDisposition(context.Background(), controlRepo, controlPlan); err != nil {
					t.Fatalf("control (uninterrupted) execution for %s/%s: %v", phase, role, err)
				}
				controlBase, _, err := reviewAuthorityRoot(context.Background(), controlRepo)
				if err != nil {
					t.Fatal(err)
				}
				controlSeed := controlPlan.Closure[len(controlPlan.Closure)-1]
				wantResidue := map[string]map[string]string{
					"grandchild": residueBytesForLineage(t, controlBase, controlGrandchild.State.LineageID),
					"child":      residueBytesForLineage(t, controlBase, controlChild.State.LineageID),
					"seed":       residueBytesForLineage(t, controlBase, controlSeed),
				}

				// Experiment: the identical fixture again (same suffix, a
				// separate temp repo) — content and lineage IDs match the
				// control exactly, so the only difference is the injected
				// interruption and resume.
				repo, plan, child, grandchild, witnessStore := buildFixture(t, suffix)
				witnessBefore, err := os.ReadFile(witnessStore.StatePath())
				if err != nil {
					t.Fatal(err)
				}
				seed := plan.Closure[len(plan.Closure)-1]

				var targetLineage string
				switch role {
				case "grandchild":
					targetLineage = grandchild.State.LineageID
				case "child":
					targetLineage = child.State.LineageID
				case "seed":
					targetLineage = seed
				}

				originalHook := compactReclaimPhaseHook
				injected := errors.New("injected crash at " + phase + "/" + role)
				fired := false
				compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
					if current == phase && record.LineageID == targetLineage && !fired {
						fired = true
						return injected
					}
					return originalHook(ctx, current, record)
				}
				t.Cleanup(func() { compactReclaimPhaseHook = originalHook })

				// The seed/committed position is the final hook of a fully
				// successful run: injecting there still interrupts before
				// the manifest write and readback, which is exactly the
				// "crash right at the last position" case tasks.md 3.5 asks
				// for. Every other position genuinely interrupts mid-closure.
				_, err = executeAuthorityDisposition(context.Background(), repo, plan)
				if !errors.Is(err, injected) {
					t.Fatalf("interrupted execution error = %v, want the injected interruption at %s/%s", err, phase, role)
				}

				compactReclaimPhaseHook = originalHook
				record, err := executeAuthorityDisposition(context.Background(), repo, plan)
				if err != nil {
					t.Fatalf("resumed execution after %s/%s: %v", phase, role, err)
				}
				if record.Status != CompactReclaimCommitted || record.LineageID != seed {
					t.Fatalf("resumed execution after %s/%s did not converge on the committed seed: %#v", phase, role, record)
				}

				base, _, err := reviewAuthorityRoot(context.Background(), repo)
				if err != nil {
					t.Fatal(err)
				}
				for _, lineage := range []string{grandchild.State.LineageID, child.State.LineageID, seed} {
					if got := quarantineEntriesForLineage(t, base, lineage); len(got) != 1 {
						t.Fatalf("%q has %v quarantine entries after resume from %s/%s, want exactly 1", lineage, got, phase, role)
					}
				}

				report, err := InspectCompactRecoveryEdges(context.Background(), repo)
				if err != nil {
					t.Fatal(err)
				}
				if !report.Complete || !report.Valid {
					t.Fatalf("post-resume retained graph from %s/%s did not revalidate cleanly: %#v", phase, role, report.Totals)
				}

				gotResidue := map[string]map[string]string{
					"grandchild": residueBytesForLineage(t, base, grandchild.State.LineageID),
					"child":      residueBytesForLineage(t, base, child.State.LineageID),
					"seed":       residueBytesForLineage(t, base, seed),
				}
				for _, member := range []string{"grandchild", "child", "seed"} {
					gotFiles, wantFiles := gotResidue[member], wantResidue[member]
					if len(gotFiles) != len(wantFiles) {
						t.Fatalf("%s/%s: residue file count for %s = %d, control = %d", phase, role, member, len(gotFiles), len(wantFiles))
					}
					for name, wantBytes := range wantFiles {
						if gotFiles[name] != wantBytes {
							t.Fatalf("%s/%s: residue file %q for %s does not byte-match the uninterrupted control run", phase, role, name, member)
						}
					}
				}

				witnessAfter, err := os.ReadFile(witnessStore.StatePath())
				if err != nil {
					t.Fatal(err)
				}
				if string(witnessBefore) != string(witnessAfter) {
					t.Fatalf("unrelated witness lineage bytes changed across a crash-then-resume at %s/%s", phase, role)
				}
			})
		}
	}
}
