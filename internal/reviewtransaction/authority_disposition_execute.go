package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
)

// AuthorityDispositionProofSchema identifies AuthorityDispositionProof's shape.
const AuthorityDispositionProofSchema = "gentle-ai.review-authority-disposition-proof/v1"

// errAuthorityDispositionShape is returned whenever a plan's closure does
// not have the one shape this executor admits: exactly one seed, at least
// one closure member, and the seed last (Wave 6's normative
// descendant-first, seed-last ordering — rdd-authority-disposition-plan /
// "Deterministic Closure Derivation From the Graph Source of Record"). N=1
// is the identity of Wave 2's whole executor scope (#1892's historical
// exact-binding edge shape); N>=2 is admitted only for closed, evidence-
// backed anomaly classes (#2014's descendant closure). A closure that does
// not end with the seed is malformed or ambiguous — the same diagnosis
// #1656's unclassifiable multi-lineage shape needs a maintainer to resolve,
// with no delivery-date commitment (rdd-closure-disposition-execution /
// "N-Node Admission for Closed Anomaly Classes").
var errAuthorityDispositionShape = errors.New("authority disposition execution refused: closure shape is not admissible") // refusal:by-design human-authority: an unclassifiable or malformed multi-lineage shape (#1656) needs a maintainer's diagnosis, not a command this refusal can name

// admitClosureDisposition requires exactly one seed and a closure ordered
// descendant-first with the seed last — the N=1 base case (Wave 2) and every
// N>=2 closed-class closure Wave 6 admits. Any other shape refuses before
// any lock acquisition or I/O (design decision 5: cardinality is executor
// admission policy, not a plan-shape constraint).
func admitClosureDisposition(plan AuthorityDispositionPlan) error {
	if len(plan.SeedSet) != 1 || len(plan.Closure) == 0 || plan.Closure[len(plan.Closure)-1] != plan.SeedSet[0] {
		return fmt.Errorf("%w: closure has %d member(s) and does not end with the sole seed; an unclassifiable or malformed multi-lineage shape (#1656) needs a maintainer's diagnosis, with no delivery-date commitment",
			errAuthorityDispositionShape, len(plan.Closure))
	}
	return nil
}

// AdmitAuthorityDispositionClosure is the exported form of
// admitClosureDisposition for Slice S3's `review repair` CLI wiring and
// SanctionedCompactRecoveryExits (compact_inspect.go) — both need the
// identical admission predicate the executor itself enforces, so neither
// ever advertises or accepts a plan the executor would then refuse.
func AdmitAuthorityDispositionClosure(plan AuthorityDispositionPlan) error {
	return admitClosureDisposition(plan)
}

// AuthorityDispositionProof carries the natively re-derived
// AuthorityDispositionPlan binding inside the quarantine audit record; it is
// set only for disposition-plan-bound quarantines this file executes. The
// Authorization is recorded only by SHA-256 digest, matching
// classifyCompactRecoveryEdgeAnomalies' idiom for authorization bytes
// (compact_reconcile.go): the recorded binding itself stays byte-preserved
// in the quarantined residue.
type AuthorityDispositionProof struct {
	Schema                     string                        `json:"schema"`
	PlanDigest                 string                        `json:"plan_digest"`
	AuthorityInventoryRevision string                        `json:"authority_inventory_revision"`
	AnomalyClass               string                        `json:"anomaly_class"`
	Selector                   *AuthorityDispositionSelector `json:"selector,omitempty"`
	SeedSet                    []string                      `json:"ordered_seed_set"`
	Closure                    []string                      `json:"ordered_closure"`
	ExpectedRevisions          map[string]string             `json:"expected_revisions"`
	AuthorizationSHA256        string                        `json:"authorization_sha256"`
}

// executeAuthorityDisposition is the one executor that consumes an
// AuthorityDispositionPlan with a populated Authorization and admits every
// closure shape rdd-closure-disposition-execution's N-Node Admission
// requirement accepts (N=1, the Wave 2 rdd-leaf-disposition-execution base
// case, through N>=2 for closed anomaly classes). It follows design.md's
// Data Flow exactly: admitClosureDisposition on the submitted plan,
// then an exclusive maintenance lock, then a fresh re-derivation under that
// lock compared by digest (CAS over expected_revisions and every other
// derived field), then Authorization validated against the digest-bound
// plan (mandatory obligation (b) — validateAuthorityDispositionAuthorization
// is reused verbatim from authority_disposition_plan.go, never
// reimplemented), then replay discovery, then quarantine. The lock is
// released before the retained-graph readback: the quarantine mutation is
// already durably committed by then, and readback reuses the ordinary
// InspectCompactRecoveryEdges seam, which cannot run while this executor's
// own exclusive lease is still held (see
// loadCompactRecoveryRecordsUnderMaintenanceHold). It stays unexported: it
// has no CLI entrypoint of its own in this slice — Slice S3 wires it
// (through repairAuthorityDispositionAtRepo, authority_repair.go) behind the
// existing `review repair` verb (rdd-authority-disposition-plan / "No New
// Public Repair Verb").
func executeAuthorityDisposition(ctx context.Context, repo string, plan AuthorityDispositionPlan) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := admitClosureDisposition(plan); err != nil {
		return CompactReclaimRecord{}, err
	}
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	base, binding, err := authorityRepairRoot(root)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	if plan.RepositoryBinding != binding {
		// refusal:by-design operator-knowledge: only the operator knows which repository the plan was derived against; the fix is supplying the matching --cwd, not a command this refusal can name
		return CompactReclaimRecord{}, errors.New("authority disposition execution refused: plan is bound to a different repository")
	}
	seed := plan.SeedSet[0]

	maintenance, err := acquireMaintenanceLock(ctx, compactMaintenanceLockPath(base), maintenanceExclusive)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	record, err := lockedAuthorityDispositionMutation(ctx, maintenance, base, root, binding, seed, plan)
	if err != nil {
		return record, err
	}
	return readBackAuthorityDisposition(ctx, root, record)
}

// lockedAuthorityDispositionMutation runs every step that must happen while
// base's exclusive maintenance lock is held: replay discovery, lock+CAS
// reinspection, Authorization validation, CAS-all-N validation, and the
// ordered, byte-preserving per-node quarantine over the whole closure,
// forward-only resuming an interrupted closure in place. It always releases
// maintenance before returning.
//
// Wave 6 (rdd-closure-disposition-execution / "Descendant-First Ordered
// Disposition", "Atomic Visibility With Forward-Only Convergence", "Forward-
// Only Resume via Plan Digest and Residue Discriminator"): every closure
// member's ExpectedRevision is checked (CAS-all-N) before the first move on
// a fresh execution, naming exactly which member drifted; then plan.Closure
// is processed in order (descendants first, seed last — the same order
// authorityDispositionClosure derives), each member discovered first
// (committed = skip, prepared = resume via residue/, absent = quarantine
// fresh) and quarantined via quarantineCompactStoreEntry's existing
// two-phase hooks, unchanged; then the forensic-only closure manifest is
// written once, inside the seed's own quarantine directory, only after
// every member has committed.
//
// Slice S3's forward-only resume (this function, replacing S2's
// always-fresh loop): because disposition always starts with the deepest
// descendant (plan.Closure[0]), that member's state alone distinguishes a
// genuinely fresh execution (its v2/ entry still exists, untouched) from any
// kind of prior progress (already resumed/committed under this exact digest,
// or a mismatched digest) — no closure member past the first can show
// progress if the first has none. A fresh execution runs the full
// re-derive/CAS-all-N/authorize validation exactly as S1/S2 did; any
// non-fresh state skips straight to the per-node loop, which never attempts
// a narrowing re-derivation — it trusts the plan_digest binding that already
// passed authorization the first time this exact digest began, and lets each
// node's own discovery surface either a clean skip/resume or a named digest-
// mismatch refusal.
func lockedAuthorityDispositionMutation(ctx context.Context, maintenance *MaintenanceLock, base, root, binding, seed string, plan AuthorityDispositionPlan) (CompactReclaimRecord, error) {
	defer maintenance.Release()

	if existing, found, err := discoverAuthorityDispositionRecord(ctx, base, seed, plan.PlanDigest); err != nil {
		return CompactReclaimRecord{}, err
	} else if found {
		return existing, nil
	}

	freshExecution, err := authorityDispositionClosureIsFresh(ctx, base, plan)
	if err != nil {
		return CompactReclaimRecord{}, err
	}

	// Fix cycle 1 (CRITICAL-1, security): Authorization and CAS-all-N
	// validate on EVERY execution path, fresh or resume — only the plan
	// RE-DERIVATION comparison below is fresh-only, because that is the one
	// part that legitimately cannot work mid-closure (re-deriving against a
	// store that has already shrunk as prior members were quarantined away
	// is a narrowing re-derivation the spec forbids, not a real staleness
	// signal). Gating validateAuthorityDispositionAuthorization inside the
	// fresh-only branch (as this function previously did) meant every
	// RESUME executed unauthorized: RepairAuthorityDisposition's resume path
	// (reconstructAuthorityDispositionPlanForResume, authority_repair.go)
	// sets Authorization from the CURRENT caller's argument, unverified, so
	// any caller who merely knew an in-progress plan_digest could resume
	// with a forged Authorization and it would commit every remaining
	// closure member.
	var records map[string]CompactRecord
	var report CompactRecoveryInspectionReport
	var freshRecords map[string]CompactRecord
	validationInventoryRevision := plan.AuthorityInventoryRevision
	if freshExecution {
		report, freshRecords, err = loadCompactRecoveryRecordsUnderMaintenanceHold(ctx, root)
		if err != nil {
			return CompactReclaimRecord{}, err
		}
		records = freshRecords
		requested := []AuthorityDispositionSelector{}
		if plan.Selector != nil {
			requested = append(requested, *plan.Selector)
		}
		currentPlan, err := deriveAuthorityDispositionPlan(report, records, binding, plan.Actor, plan.Reason, requested...)
		if err != nil {
			return CompactReclaimRecord{}, fmt.Errorf("authority disposition execution refused: re-derivation under lock did not reproduce a closed classification: %w", err)
		}
		if err := admitClosureDisposition(currentPlan); err != nil {
			return CompactReclaimRecord{}, err
		}
		if currentPlan.PlanDigest != plan.PlanDigest {
			return CompactReclaimRecord{}, fmt.Errorf("%w: plan digest drifted under lock re-derivation (expected_revisions or authority_inventory_revision changed)", ErrConcurrentUpdate)
		}
		currentInventoryRevision, err := authorityInventoryRevision(records, report.historical)
		if err != nil {
			return CompactReclaimRecord{}, err
		}
		validationInventoryRevision = currentInventoryRevision
	}
	// A non-fresh closure (freshExecution == false) skips re-derivation
	// entirely: this exact plan_digest already passed re-derivation and CAS
	// the one time its per-node loop first began (when plan.Closure[0] still
	// had a live, untouched v2/ entry). Authorization, though, binds
	// plan_digest and plan.AuthorityInventoryRevision to each other
	// cryptographically (authorityDispositionAuthorizationBinding);
	// comparing plan's own frozen inventory revision to itself below makes
	// the drift half of validateAuthorityDispositionAuthorization a no-op on
	// resume (as it must be — the store has legitimately shrunk), while the
	// binding half still catches a forged or mismatched Authorization on
	// every single call, fresh or resumed.
	if err := validateAuthorityDispositionAuthorization(plan, validationInventoryRevision); err != nil {
		return CompactReclaimRecord{}, err
	}

	// CAS-all-N: every closure member's expected revision is checked before
	// the first move — drift on ANY member, not only the seed, refuses
	// pre-move and names exactly which member drifted. On resume, records
	// were not already loaded above (freshExecution == false skipped
	// re-derivation), so load them once here, for CAS-all-N only.
	if records == nil {
		report, freshRecords, err = loadCompactRecoveryRecordsUnderMaintenanceHold(ctx, root)
		if err != nil {
			return CompactReclaimRecord{}, err
		}
		records = freshRecords
	}
	for _, lineage := range plan.Closure {
		expectedRevision, expectedFound := plan.ExpectedRevisions[lineage]
		if historical, found := report.historical[lineage]; found {
			if !expectedFound || historical.RawDigest != expectedRevision {
				return CompactReclaimRecord{}, fmt.Errorf("%w: expected revision for closure member %q drifted", ErrConcurrentUpdate, lineage)
			}
			continue
		}
		targetRecord, found := records[lineage]
		if !found {
			// On a fresh execution a missing member is drift — nothing has
			// legitimately removed it yet. On resume it is very likely
			// already quarantined by the interrupted attempt this call
			// resumes; the per-node loop below discovers and either skips
			// it cleanly or refuses it by name (a genuine digest mismatch),
			// so CAS-all-N does not refuse pre-emptively for a member that
			// simply is not live anymore.
			if freshExecution {
				return CompactReclaimRecord{}, fmt.Errorf("%w: expected revision for closure member %q drifted", ErrConcurrentUpdate, lineage)
			}
			continue
		}
		if !expectedFound || targetRecord.Revision != expectedRevision {
			return CompactReclaimRecord{}, fmt.Errorf("%w: expected revision for closure member %q drifted", ErrConcurrentUpdate, lineage)
		}
	}

	recordedAuthorization := sha256.Sum256([]byte(plan.Authorization))
	var lastCommitted CompactReclaimRecord
	for _, lineage := range plan.Closure {
		if existing, found, err := discoverAuthorityDispositionRecord(ctx, base, lineage, plan.PlanDigest); err != nil {
			return lastCommitted, err
		} else if found {
			// Committed under this exact digest: skip. Prepared: already
			// resumed to committed by discoverAuthorityDispositionRecord
			// itself (the residue/ discriminator) before returning found.
			lastCommitted = existing
			continue
		}

		dir := filepath.Join(base, "v2", lineage)
		items, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				// Missing, but no record for THIS digest was found either:
				// some other quarantine (a mismatched/stale digest, or an
				// unrelated operation) already moved it. Forward-only resume
				// never attempts a narrowing re-derivation here — it refuses
				// and names whatever concrete, escalatable path it can find.
				return lastCommitted, authorityDispositionResumeDigestMismatchRefusal(base, plan.Closure[len(plan.Closure)-1], lineage, plan.PlanDigest)
			}
			return lastCommitted, fmt.Errorf("inspect authority disposition target %q: %w", lineage, err)
		}
		residue := make([]string, 0, len(items))
		for _, item := range items {
			residue = append(residue, item.Name())
		}
		sort.Strings(residue)

		committed, err := quarantineCompactStoreEntry(ctx, base, dir, CompactReclaimRecord{
			Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: lineage,
			Reason: plan.Reason, Actor: plan.Actor, ReclaimedAt: time.Now().UTC(), SourcePath: dir, Residue: residue,
			AuthorityDisposition: &AuthorityDispositionProof{
				Schema: AuthorityDispositionProofSchema, PlanDigest: plan.PlanDigest,
				AuthorityInventoryRevision: plan.AuthorityInventoryRevision, AnomalyClass: plan.AnomalyClass,
				Selector: plan.Selector,
				SeedSet:  append([]string(nil), plan.SeedSet...), Closure: append([]string(nil), plan.Closure...),
				ExpectedRevisions:   cloneAuthorityDispositionRevisions(plan.ExpectedRevisions),
				AuthorizationSHA256: "sha256:" + hex.EncodeToString(recordedAuthorization[:]),
			},
		})
		if err != nil {
			// A partial closure never reports success: this returns before
			// the seed (the last, closure-ending member) ever commits, so
			// the caller never reaches readBackAuthorityDisposition. Nodes
			// already committed in earlier loop iterations stay committed —
			// there is no rollback (design decision D3); the retained graph
			// at this exact interruption point is still valid because every
			// closure prefix is (rdd-closure-disposition-execution /
			// "Descendant-First Ordered Disposition").
			return committed, err
		}
		lastCommitted = committed
	}

	// The forensic-only closure manifest is written once, only after every
	// closure member has committed (design.md's Data Flow places this step
	// after the ordered loop, before readback) — it is never a lifecycle
	// dependency (design caps RDD at two operational artifacts: the reclaim
	// record and residue/; this manifest is a third, forensic-only artifact
	// that recovery never reads).
	if err := writeAuthorityDispositionClosureManifest(lastCommitted.QuarantinePath, AuthorityDispositionClosureManifest{
		Schema: AuthorityDispositionClosureManifestSchema, PlanDigest: plan.PlanDigest,
		AuthorityInventoryRevision: plan.AuthorityInventoryRevision, AnomalyClass: plan.AnomalyClass,
		OrderedClosure: append([]string(nil), plan.Closure...), Disposed: append([]string(nil), plan.Closure...),
	}); err != nil {
		return lastCommitted, err
	}

	return lastCommitted, nil
}

// authorityDispositionClosureIsFresh reports whether NO closure member has
// any prior disposition progress: disposition always starts with the
// deepest descendant (plan.Closure[0]) first, so that single member's state
// is sufficient to decide — no later member can show progress if the first
// one has none. It is fresh only if plan.Closure[0] has no existing
// discoverable record under plan.PlanDigest AND its v2/ entry still exists
// untouched. Any other state (a record under this digest, a record under a
// different digest, or a missing v2/ entry with no matching record at all)
// means SOME prior attempt already began, so the caller must resume instead
// of re-deriving.
func authorityDispositionClosureIsFresh(ctx context.Context, base string, plan AuthorityDispositionPlan) (bool, error) {
	firstMember := plan.Closure[0]
	if _, found, err := discoverAuthorityDispositionRecord(ctx, base, firstMember, plan.PlanDigest); err != nil {
		return false, err
	} else if found {
		return false, nil
	}
	if _, statErr := os.Stat(filepath.Join(base, "v2", firstMember)); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}
		return false, fmt.Errorf("inspect authority disposition target %q: %w", firstMember, statErr)
	}
	return true, nil
}

// authorityDispositionResumeDigestMismatchRefusal is the digest-mismatch
// refusal rdd-closure-disposition-execution's "Forward-Only Resume via Plan
// Digest and Residue Discriminator" requires: memberLineage's v2/ entry is
// gone, but no quarantine record for it matches planDigest. It names the
// SEED's closure-manifest.json when one already exists (a completed run
// under a different digest — the most concrete, human-facing artifact), or
// falling back to memberLineage's own quarantine directory under any digest
// (a still-incomplete run), or a generic escalation with neither path when
// even that cannot be found. It never attempts a narrowing re-derivation.
//
// SUGGESTION (fix cycle 1): both loops select a candidate quarantine
// directory by name-prefix alone, mirroring the directory-naming scheme
// (`<lineage>-<random-suffix>`) but not verifying it — a lineage ID that is
// itself a prefix of another (e.g. "foo" and "foo-bar") could otherwise
// match the wrong directory. quarantineDirectoryLineageMatches decodes the
// candidate's own reclaim-record.json and checks its LineageID field
// explicitly, exactly like discoverAuthorityDispositionRecord already does.
func authorityDispositionResumeDigestMismatchRefusal(base, seedLineage, memberLineage, planDigest string) error {
	quarantineRoot := filepath.Join(base, "quarantine")
	entries, err := os.ReadDir(quarantineRoot)
	if err != nil {
		// refusal:by-design human-authority: a closure member missing from the authority store with an unreadable quarantine root needs a maintainer's diagnosis; forward-only resume never attempts a narrowing re-derivation, so there is no command this refusal can name
		return fmt.Errorf("authority disposition execution refused: closure member %q is missing from the authority store with no matching quarantine record for plan_digest %q; escalate — resume never attempts a narrowing re-derivation", memberLineage, planDigest)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), seedLineage+"-") ||
			!quarantineDirectoryLineageMatches(quarantineRoot, entry.Name(), seedLineage) {
			continue
		}
		manifestPath := filepath.Join(quarantineRoot, entry.Name(), "closure-manifest.json")
		if _, statErr := os.Stat(manifestPath); statErr == nil {
			// refusal:by-design human-authority: a digest-mismatched closure that already completed under a different plan_digest needs a maintainer's diagnosis; forward-only resume never attempts a narrowing re-derivation, so there is no command this refusal can name
			return fmt.Errorf("authority disposition execution refused: closure member %q was already quarantined under a different plan_digest than %q; inspect %s and escalate — resume never attempts a narrowing re-derivation", memberLineage, planDigest, manifestPath)
		}
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), memberLineage+"-") &&
			quarantineDirectoryLineageMatches(quarantineRoot, entry.Name(), memberLineage) {
			// refusal:by-design human-authority: a digest-mismatched or otherwise ambiguous partial closure needs a maintainer's diagnosis before any resume continues; forward-only resume never attempts a narrowing re-derivation, so there is no command this refusal can name
			return fmt.Errorf("authority disposition execution refused: closure member %q was already quarantined under a different plan_digest than %q; inspect %s and escalate — resume never attempts a narrowing re-derivation", memberLineage, planDigest, filepath.Join(quarantineRoot, entry.Name()))
		}
	}
	// refusal:by-design human-authority: a closure member missing from the authority store with no matching quarantine record under any digest needs a maintainer's diagnosis; forward-only resume never attempts a narrowing re-derivation, so there is no command this refusal can name
	return fmt.Errorf("authority disposition execution refused: closure member %q is missing from the authority store with no matching quarantine record for plan_digest %q; escalate — resume never attempts a narrowing re-derivation", memberLineage, planDigest)
}

// quarantineDirectoryLineageMatches reports whether the quarantine directory
// entryName (a child of quarantineRoot) holds a decodable reclaim-record.json
// whose own LineageID field equals expectedLineage. It never returns true on
// any read or decode failure — an unreadable or malformed record is not
// evidence of a match, only a name-prefix coincidence would be.
func quarantineDirectoryLineageMatches(quarantineRoot, entryName, expectedLineage string) bool {
	payload, err := os.ReadFile(filepath.Join(quarantineRoot, entryName, "reclaim-record.json"))
	if err != nil {
		return false
	}
	var record CompactReclaimRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return false
	}
	return record.LineageID == expectedLineage
}

// loadCompactRecoveryRecordsUnderMaintenanceHold mirrors
// loadCompactRecoveryRecords's exact read-and-classify body
// (compact_inspect.go) for a caller that already holds base's exclusive
// maintenance lock. Calling the ordinary seam here would self-contend:
// CompactStore.Load acquires its own shared maintenance lock per entry
// (compact_store.go acquireReadMaintenance), and flock is not reentrant
// across file descriptors even within the same process, so a second lock
// attempt while this executor's own exclusive lease is held would time out
// on every entry. This reuses the exact same classification algorithm
// (inspectCompactRecoveryRecordSet) the seam calls — only the per-entry file
// read differs: loadCompactRecordLocked (the package's existing
// "uncoordinated read for a caller that already holds the required
// coordination" primitive, compact_store.go) instead of Load.
func loadCompactRecoveryRecordsUnderMaintenanceHold(ctx context.Context, repo string) (CompactRecoveryInspectionReport, map[string]CompactRecord, error) {
	report := CompactRecoveryInspectionReport{Complete: true, Valid: true, Edges: []CompactRecoveryEdgeInspection{}, EntryDiagnostics: []CompactRecoveryEntryDiagnostic{}, historical: map[string]historicalCompactForensicRecord{}}
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return report, nil, err
	}
	if err := ensureNoPreparedCompactBatchReconciliation(base); err != nil {
		return report, nil, err
	}
	versionRoot := filepath.Join(base, "v2")
	entries, err := os.ReadDir(versionRoot)
	if os.IsNotExist(err) {
		return report, map[string]CompactRecord{}, nil
	}
	if err != nil {
		return report, nil, fmt.Errorf("inspect compact authority v2 root: %w", err)
	}
	records := make(map[string]CompactRecord, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return report, nil, err
		}
		if !entry.IsDir() {
			if entry.Name() != "LOCK" {
				report.EntryDiagnostics = append(report.EntryDiagnostics, CompactRecoveryEntryDiagnostic{
					LineageID: entry.Name(), Problem: compactInspectionEntryUnexpected,
				})
			}
			continue
		}
		report.Totals.CompactEntries++
		store := CompactStore{Dir: filepath.Join(versionRoot, entry.Name()), lineageID: entry.Name(), repo: root,
			lockPath: filepath.Join(versionRoot, "LOCK"), maintenanceLockPath: compactMaintenanceLockPath(base)}
		record, loadErr := store.loadCompactRecordLocked()
		if loadErr != nil {
			if payload, err := os.ReadFile(store.StatePath()); err == nil {
				if historical, ok := forensicHistoricalCompactRecord(payload, entry.Name()); ok {
					report.historical[entry.Name()] = historical
				}
			}
			report.EntryDiagnostics = append(report.EntryDiagnostics, CompactRecoveryEntryDiagnostic{
				LineageID: entry.Name(), Problem: compactRecoveryEntryProblem(loadErr),
			})
			continue
		}
		report.Totals.LoadedEntries++
		records[record.State.LineageID] = record
	}
	report, err = inspectCompactRecoveryRecordSet(ctx, records, report)
	return report, records, err
}

// discoverAuthorityDispositionRecord finds an existing quarantine record for
// seed whose AuthorityDisposition binds the exact planDigest, so a replayed
// execution converges without moving the entry a second time (exact replay,
// rdd-leaf-disposition-execution / "Exact Replay Converges Without
// Double-Move"). A record still Prepared (a crash between the residue
// rename and the committed rewrite) is resumed to completion first, mirroring
// resumePreparedClassifiedAuthorityRepair's discriminator
// (classified_authority_replay.go): the residue/ subdirectory's presence
// decides whether the rename already ran.
func discoverAuthorityDispositionRecord(ctx context.Context, base, seed, planDigest string) (CompactReclaimRecord, bool, error) {
	quarantineRoot := filepath.Join(base, "quarantine")
	if err := ensureCanonicalReviewQuarantineRoot(base, quarantineRoot); err != nil {
		return CompactReclaimRecord{}, false, err
	}
	entries, err := os.ReadDir(quarantineRoot)
	if os.IsNotExist(err) {
		return CompactReclaimRecord{}, false, nil
	}
	if err != nil {
		return CompactReclaimRecord{}, false, fmt.Errorf("inspect authority disposition quarantine: %w", err)
	}
	var matched *CompactReclaimRecord
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return CompactReclaimRecord{}, false, err
		}
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), seed+"-") {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(quarantineRoot, entry.Name(), "reclaim-record.json"))
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return CompactReclaimRecord{}, false, fmt.Errorf("read authority disposition quarantine record: %w", readErr)
		}
		var record CompactReclaimRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			return CompactReclaimRecord{}, false, fmt.Errorf("decode authority disposition quarantine record: %w", err)
		}
		if record.LineageID != seed || record.AuthorityDisposition == nil || record.AuthorityDisposition.PlanDigest != planDigest {
			continue
		}
		if matched != nil {
			return CompactReclaimRecord{}, false, errors.New("authority disposition execution refused: duplicate quarantine records for the same plan digest; run `gentle-ai review inspect-authority` and escalate the report")
		}
		found := record
		matched = &found
	}
	if matched == nil {
		return CompactReclaimRecord{}, false, nil
	}
	if matched.Status == CompactReclaimPrepared {
		resumed, err := resumeAuthorityDispositionRecord(ctx, *matched)
		return resumed, true, err
	}
	return *matched, true, nil
}

// resumeAuthorityDispositionRecord completes a prepared-but-interrupted
// disposition quarantine, reusing canonicalClassifiedRepairDirectory
// (classified_authority_replay.go) to decide whether the residue rename
// already ran.
func resumeAuthorityDispositionRecord(ctx context.Context, record CompactReclaimRecord) (CompactReclaimRecord, error) {
	residuePath := filepath.Join(record.QuarantinePath, "residue")
	sourceExists, err := canonicalClassifiedRepairDirectory(record.SourcePath)
	if err != nil {
		return record, err
	}
	residueExists, err := canonicalClassifiedRepairDirectory(residuePath)
	if err != nil {
		return record, err
	}
	if sourceExists == residueExists {
		return record, errors.New("authority disposition execution refused: ambiguous prepared residue state; run `gentle-ai review inspect-authority` and escalate the report")
	}
	if sourceExists {
		if err := reclaimQuarantineResidue(record.SourcePath, residuePath); err != nil {
			return record, fmt.Errorf("resume authority disposition residue quarantine: %w", err)
		}
		if err := SyncReviewDirectory(filepath.Dir(record.SourcePath)); err != nil {
			return record, err
		}
		if err := SyncReviewDirectory(record.QuarantinePath); err != nil {
			return record, err
		}
		if err := compactReclaimPhaseHook(ctx, compactReclaimPhaseRenamed, record); err != nil {
			return record, err
		}
	}
	committed := record
	committed.Status = CompactReclaimCommitted
	if err := persistReclaimRecord(committed); err != nil {
		return record, err
	}
	if err := compactReclaimPhaseHook(ctx, compactReclaimPhaseCommitted, committed); err != nil {
		return committed, err
	}
	return committed, nil
}

// readBackAuthorityDisposition re-runs classification over the retained
// graph and refuses to report success unless it is `Complete` — the intentional sequential-repair
// postcondition — with no dangling reference to ANY closure member, not only record.LineageID (the seed). This is called once, only after
// executeAuthorityDisposition's ordered loop has committed every closure
// member (rdd-closure-disposition-execution / "Descendant-First Ordered
// Disposition"'s readback gate), and its over-collection guard widens
// rdd-leaf-disposition-execution's "Retained-Graph Revalidation Before
// Success" (which checked only the single leaf) to the whole closure
// (design decision D6): a retained edge naming any closure member is still
// evidence disposition did not fully remove it.
func readBackAuthorityDisposition(ctx context.Context, root string, record CompactReclaimRecord) (CompactReclaimRecord, error) {
	if record.Status != CompactReclaimCommitted {
		return record, fmt.Errorf("authority disposition execution refused: readback observed a non-committed record; run `gentle-ai review inspect-authority --cwd %s` and escalate the report", pathquote.Quote(root))
	}
	report, err := InspectCompactRecoveryEdges(ctx, root)
	if err != nil {
		return record, fmt.Errorf("authority disposition readback: %w", err)
	}
	if !report.Complete {
		return record, fmt.Errorf("authority disposition execution refused: retained-graph readback is incomplete; run `gentle-ai review inspect-authority --cwd %s` and escalate the report", pathquote.Quote(root))
	}
	closureMembers := authorityDispositionClosureMembers(record)
	for _, edge := range report.Edges {
		if member := edge.PredecessorLineageID; closureMembers[member] {
			return record, fmt.Errorf("authority disposition execution refused: retained graph still references quarantined closure member %q; run `gentle-ai review inspect-authority --cwd %s` and escalate the report", member, pathquote.Quote(root))
		}
		if member := edge.SuccessorLineageID; closureMembers[member] {
			return record, fmt.Errorf("authority disposition execution refused: retained graph still references quarantined closure member %q; run `gentle-ai review inspect-authority --cwd %s` and escalate the report", member, pathquote.Quote(root))
		}
	}
	return record, nil
}

// authorityDispositionClosureMembers is the over-collection guard's
// membership set: record.LineageID (the seed — every pre-Wave-6 caller and
// every N=1 execution) unioned with every entry in
// record.AuthorityDisposition.Closure, when present.
func authorityDispositionClosureMembers(record CompactReclaimRecord) map[string]bool {
	members := map[string]bool{record.LineageID: true}
	if record.AuthorityDisposition != nil {
		for _, lineage := range record.AuthorityDisposition.Closure {
			members[lineage] = true
		}
	}
	return members
}

func cloneAuthorityDispositionRevisions(revisions map[string]string) map[string]string {
	cloned := make(map[string]string, len(revisions))
	for lineage, revision := range revisions {
		cloned[lineage] = revision
	}
	return cloned
}
