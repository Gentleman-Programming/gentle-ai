package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CompactAbandonAuthorizationSchema is the first line of the LF-only
// abandonment maintainer authorization binding. The first eight lines are the
// frozen format gentle-shell renders; an abandonment that moves more than the
// named lineage's readable record (an unverifiable summary, recovery
// successors) gets extra lines appended after `actor=`, so
// the human approves exactly what moves.
//
// It is exported so a refusal in another package that names `review abandon`
// as its continuation can print the template and be checked against the schema
// this gate actually verifies, rather than against a copy of the string.
const CompactAbandonAuthorizationSchema = "gentle-ai.review-abandon-authorization/v2"

const compactLegacyAbandonAuthorizationSchema = "gentle-ai.review-abandon-authorization/v1"

// compactAbandonUnavailable stands in for the revision and snapshot identity of
// an entry whose record cannot be read (unreadable or missing review-state.json):
// the binding still names them, with this word.
const compactAbandonUnavailable = "unavailable"

// CompactAbandonRequest identifies one review lineage to declare lost and
// quarantine, together with the exact maintainer authorization binding for
// everything that abandonment moves.
type CompactAbandonRequest struct {
	LineageID               string
	ExpectedRevision        string
	Reason                  string
	Actor                   string
	MaintainerAuthorization string
	AbandonedAt             time.Time
}

// CompactAbandonResult is the outcome of one abandonment. The embedded record
// is the requested lineage's own v2 quarantine. Cascaded lists every other
// entry quarantined with it: recovery successors, leaf first.
type CompactAbandonResult struct {
	CompactReclaimRecord
	Cascaded []CompactReclaimRecord
}

// CompactPristineAbandonmentProof remains only to decode pre-v2 quarantine
// records during historical audit readback. New abandonments always write
// CompactAbandonmentProof.
type CompactPristineAbandonmentProof struct {
	LineageID          string `json:"lineage_id"`
	Revision           string `json:"revision"`
	SnapshotIdentity   string `json:"snapshot_identity"`
	State              State  `json:"state"`                         // "reviewing" or "invalidated"
	InvalidationReason string `json:"invalidation_reason,omitempty"` // when pre-invalidated
}

// CompactIncompleteAbandonmentProof remains only to decode pre-v2 quarantine
// records during historical audit readback. New abandonments always write
// CompactAbandonmentProof.
type CompactIncompleteAbandonmentProof struct {
	LineageID        string   `json:"lineage_id"`
	Revision         string   `json:"revision"`
	SnapshotIdentity string   `json:"snapshot_identity"`
	CapturedLenses   []string `json:"captured_lenses"`
	UncapturedLenses []string `json:"uncaptured_lenses"`
}

const (
	CompactAbandonReasonOperatorDisposition = "operator_disposition"
	CompactAbandonReasonRetiredSchema       = "retired_schema"
)

// CompactDiscardedWorkSummary describes the review work an abandonment
// discards. Unverifiable is set when the summary could not be derived (the
// record is unreadable, absent, or its captured results no longer re-admit):
// abandonment never refuses over that, it records that the loss is unknown.
type CompactDiscardedWorkSummary struct {
	CapturedLensResults []string `json:"captured_lens_results"`
	FindingsPresent     bool     `json:"findings_present"`
	Unverifiable        bool     `json:"unverifiable,omitempty"`
}

type CompactAbandonmentProof struct {
	Schema           string                      `json:"schema"`
	LineageID        string                      `json:"lineage_id"`
	Revision         string                      `json:"revision"`
	SnapshotIdentity string                      `json:"snapshot_identity"`
	DiscardedWork    CompactDiscardedWorkSummary `json:"discarded_work"`
	// AuthorizationBinding is the exact binding the maintainer authorized, kept
	// on the requested lineage's own entries so an exact replay can be matched.
	AuthorizationBinding string `json:"authorization_binding,omitempty"`
}

func validCompactAbandonReason(reason string) bool {
	return reason == CompactAbandonReasonOperatorDisposition || reason == CompactAbandonReasonRetiredSchema
}

// RenderCompactAbandonAuthorization renders the eight-line V2 authorization
// binding over the exact discarded work the gate re-derives. A lineage whose
// abandonment also moves successors needs the lines
// Eligibility.Authorization appends.
func RenderCompactAbandonAuthorization(lineage, revision, snapshotIdentity, actor, reason string, discarded CompactDiscardedWorkSummary) string {
	return compactAbandonAuthorizationBindingV2(lineage, revision, snapshotIdentity, actor, reason, discarded)
}

func compactAbandonAuthorizationBindingV2(lineage, revision, snapshotIdentity, actor, reason string, discarded CompactDiscardedWorkSummary) string {
	binding := CompactAbandonAuthorizationSchema + "\nlineage=" + lineage + "\nrevision=" + revision +
		"\nsnapshot_identity=" + snapshotIdentity + "\nreason=" + reason +
		"\ncaptured_lens_results=" + strings.Join(discarded.CapturedLensResults, ",") +
		fmt.Sprintf("\nfindings_present=%t", discarded.FindingsPresent) +
		"\nactor=" + strings.TrimSpace(actor)
	if discarded.Unverifiable {
		binding += "\ndiscarded_work=unverifiable"
	}
	return binding
}

func compactAbandonV2Rerun(repo, lineage, revision, actor, reason, binding string) string {
	return fmt.Sprintf("Re-run `gentle-ai review abandon --cwd %q --lineage %q --expected-revision %q --reason %q --actor %q --maintainer-authorization <the exact binding below>` using schema %s:\n%s",
		repo, lineage, revision, reason, strings.TrimSpace(actor), CompactAbandonAuthorizationSchema, binding)
}

// compactAbandonDiscardedWork derives the discarded-work summary and degrades
// to "unverifiable" instead of failing: abandonment is the exit for exactly
// the records whose captured evidence can no longer be re-admitted.
func compactAbandonDiscardedWork(ctx context.Context, store CompactStore, record CompactRecord) (CompactDiscardedWorkSummary, error) {
	if err := ctx.Err(); err != nil {
		return CompactDiscardedWorkSummary{}, err
	}
	summary, err := compactDiscardedWorkSummary(ctx, store, record)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return CompactDiscardedWorkSummary{}, ctxErr
		}
		return CompactDiscardedWorkSummary{CapturedLensResults: []string{}, Unverifiable: true}, nil
	}
	return summary, nil
}

func compactDiscardedWorkSummary(ctx context.Context, store CompactStore, record CompactRecord) (CompactDiscardedWorkSummary, error) {
	view, err := record.State.CompactReviewView()
	if err != nil {
		return CompactDiscardedWorkSummary{}, fmt.Errorf("derive admitted discarded work: %w", err)
	}
	captured := make([]string, 0, len(record.State.AdmittedRoleResults))
	for _, entry := range record.State.AdmittedRoleResults {
		if !record.State.IsAccountingOnlyAdmittedRoleResult(entry) && entry.Role == CompactRoleLens {
			captured = append(captured, fmt.Sprintf("%02d-%s", entry.SelectedOrder, entry.Lens))
		}
	}
	sort.Strings(captured)
	summary := CompactDiscardedWorkSummary{CapturedLensResults: captured, FindingsPresent: len(view.Findings) > 0}
	if len(captured) == 0 {
		return summary, nil
	}
	if store.repo == "" {
		return CompactDiscardedWorkSummary{}, errors.New("reviewer result store has no repository") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	builder := SnapshotBuilder{Repo: store.repo}
	frozen, err := builder.FrozenCandidateContext(ctx, record.State.InitialSnapshot)
	if err != nil {
		return CompactDiscardedWorkSummary{}, fmt.Errorf("derive frozen reviewer context: %w", err)
	}
	for _, entry := range record.State.AdmittedRoleResults {
		if entry.Role != CompactRoleLens {
			continue
		}
		value, err := canonicalCompactRoleValue(entry.Value)
		if err != nil || compactPreservedPayloadDigest(append(value, '\n')) != entry.ArtifactDigest {
			return CompactDiscardedWorkSummary{}, errors.New("admitted reviewer record value is invalid") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		var envelope compactAdmittedReviewerResult
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&envelope); err != nil || decoder.Decode(new(struct{})) != io.EOF {
			return CompactDiscardedWorkSummary{}, errors.New("decode admitted reviewer record value") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		nativeFrozen, expected, err := artifactSubjectForSchema(ctx, builder, record.State, entry.CapturePhaseRevision,
			frozen, entry.Lens, entry.SelectedOrder, envelope.Subject.CorrectionTargetIdentity, envelope.Subject.Schema)
		if err != nil || envelope.Schema != admittedReviewerResultSchemaForSubject(expected) || envelope.Subject != expected ||
			envelope.Admission.Validate(expected, append(append([]byte(nil), envelope.Result...), '\n')) != nil {
			return CompactDiscardedWorkSummary{}, errors.New("admit captured reviewer record value") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		result, admitted := reAdmitCompactReviewerResult(ctx, envelope, expected, nativeFrozen)
		if !admitted || result.ResultHash != entry.ResultHash {
			return CompactDiscardedWorkSummary{}, errors.New("admit captured reviewer record value") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
	}
	return summary, nil
}

// CompactAbandonEligibility reports whether `review abandon` would accept one
// lineage right now, together with the persisted values the authorization
// binding has to carry. They are published so a caller naming the abandonment
// can print them concrete instead of sending the operator to look them up.
// Revision and SnapshotIdentity are "unavailable" for an entry whose record
// cannot be read.
type CompactAbandonEligibility struct {
	Eligible         bool
	Revision         string
	SnapshotIdentity string
	DiscardedWork    CompactDiscardedWorkSummary
	// Cascade names every other entry the abandonment moves with the lineage.
	Cascade []string
	suffix  string
}

// Authorization renders the exact binding the gate verifies for this lineage:
// the eight frozen lines, then one line per entry that moves with it.
func (e CompactAbandonEligibility) Authorization(lineage, actor, reason string) string {
	return compactAbandonAuthorizationBindingV2(lineage, e.Revision, e.SnapshotIdentity, actor, reason, e.DiscardedWork) + e.suffix
}

// InspectCompactPristineAbandonment answers, read-only, whether
// AbandonCompactStore could act on this lineage: abandonment accepts any
// lineage that holds a v2 entry, whatever its state or
// readability. It takes no lock and writes nothing. The maintainer
// authorization and the compare-and-swap are deliberately NOT predicted: they
// are the operator's own act.
func InspectCompactPristineAbandonment(ctx context.Context, repo, lineage string) (CompactAbandonEligibility, error) {
	if err := ctx.Err(); err != nil {
		return CompactAbandonEligibility{}, err
	}
	if err := validateLineageID(lineage); err != nil {
		return CompactAbandonEligibility{}, nil
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactAbandonEligibility{}, err
	}
	v2Present, err := compactAbandonEntryPresent(filepath.Join(base, "v2", lineage))
	if err != nil {
		return CompactAbandonEligibility{}, err
	}
	if !v2Present {
		return CompactAbandonEligibility{}, nil
	}
	plan, err := planCompactAbandon(ctx, repo, base, lineage, v2Present)
	if err != nil {
		return CompactAbandonEligibility{}, err
	}
	return CompactAbandonEligibility{
		Eligible: true, Revision: plan.revision, SnapshotIdentity: plan.snapshotIdentity,
		DiscardedWork: plan.discarded, Cascade: plan.cascade, suffix: plan.suffix,
	}, nil
}

func compactAbandonEntryPresent(path string) (bool, error) {
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect abandon target: %w", err)
	}
	return true, nil
}

// compactAbandonDescendants returns every recovery successor reachable from
// lineage through readable records, deepest first, so removing them in the
// returned order never leaves a successor whose predecessor is already gone.
func compactAbandonDescendants(records map[string]CompactRecord, lineage string) []string {
	children := map[string][]string{}
	for candidate, record := range records {
		if record.State.Recovery != nil {
			parent := record.State.Recovery.PredecessorLineageID
			children[parent] = append(children[parent], candidate)
		}
	}
	depth := map[string]int{lineage: 0}
	queue := []string{lineage}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		next := children[current]
		sort.Strings(next)
		for _, child := range next {
			if _, seen := depth[child]; seen {
				continue
			}
			depth[child] = depth[current] + 1
			queue = append(queue, child)
		}
	}
	delete(depth, lineage)
	ordered := make([]string, 0, len(depth))
	for descendant := range depth {
		ordered = append(ordered, descendant)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if depth[ordered[i]] != depth[ordered[j]] {
			return depth[ordered[i]] > depth[ordered[j]]
		}
		return ordered[i] < ordered[j]
	})
	return ordered
}

type compactAbandonStep struct {
	lineage string
	dir     string
	residue []string
	proof   CompactAbandonmentProof
	// primary marks the requested lineage's own entry: its proof carries the authorization binding for replay.
	primary bool
}

// compactAbandonPlan is everything one abandonment moves, in quarantine order
// (recovery successors deepest first, then the lineage's own v2 entry), plus the values its authorization binding is made of.
type compactAbandonPlan struct {
	steps            []compactAbandonStep
	revision         string
	snapshotIdentity string
	discarded        CompactDiscardedWorkSummary
	cascade          []string
	suffix           string
}

func compactAbandonResidue(dir string) ([]string, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		if info, statErr := os.Lstat(dir); statErr == nil && !info.IsDir() {
			return []string{}, nil
		}
		return nil, fmt.Errorf("inspect abandon target: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)
	return residue, nil
}

// planCompactAbandon derives, read-only, every entry an abandonment of lineage
// moves and the binding suffix that names them. The caller decides whether it
// holds the store lock.
func planCompactAbandon(ctx context.Context, repo, base, lineage string, v2Present bool) (compactAbandonPlan, error) {
	plan := compactAbandonPlan{
		revision: compactAbandonUnavailable, snapshotIdentity: compactAbandonUnavailable,
		discarded: CompactDiscardedWorkSummary{CapturedLensResults: []string{}, Unverifiable: true},
	}
	unreadable := func(schemaLineage string) CompactAbandonmentProof {
		return CompactAbandonmentProof{
			Schema: CompactAbandonAuthorizationSchema, LineageID: schemaLineage,
			Revision: compactAbandonUnavailable, SnapshotIdentity: compactAbandonUnavailable,
			DiscardedWork: CompactDiscardedWorkSummary{CapturedLensResults: []string{}, Unverifiable: true},
		}
	}
	lines := []string{}
	if v2Present {
		dir := filepath.Join(base, "v2", lineage)
		scan, err := scanCompactAuthority(ctx, repo)
		if err != nil {
			return compactAbandonPlan{}, err
		}
		for _, victim := range compactAbandonDescendants(scan.records, lineage) {
			record := scan.records[victim]
			discarded, err := compactAbandonDiscardedWork(ctx, scan.stores[victim], record)
			if err != nil {
				return compactAbandonPlan{}, err
			}
			residue, err := compactAbandonResidue(scan.stores[victim].Dir)
			if err != nil {
				return compactAbandonPlan{}, err
			}
			plan.steps = append(plan.steps, compactAbandonStep{lineage: victim, dir: scan.stores[victim].Dir, residue: residue, proof: CompactAbandonmentProof{
				Schema: CompactAbandonAuthorizationSchema, LineageID: victim, Revision: record.Revision,
				SnapshotIdentity: record.State.InitialSnapshot.Identity, DiscardedWork: discarded,
			}})
			plan.cascade = append(plan.cascade, victim)
			line := fmt.Sprintf("cascade=%s@%s snapshot_identity=%s captured_lens_results=%s findings_present=%t",
				victim, record.Revision, record.State.InitialSnapshot.Identity, strings.Join(discarded.CapturedLensResults, ","), discarded.FindingsPresent)
			if discarded.Unverifiable {
				line += " discarded_work=unverifiable"
			}
			lines = append(lines, line)
		}
		target := compactAbandonStep{lineage: lineage, dir: dir, primary: true, proof: unreadable(lineage)}
		store := CompactStore{Dir: dir, lineageID: lineage, repo: repo}
		record, loadErr := store.LoadContext(ctx)
		switch {
		case loadErr == nil:
			discarded, err := compactAbandonDiscardedWork(ctx, store, record)
			if err != nil {
				return compactAbandonPlan{}, err
			}
			target.proof.Revision = record.Revision
			target.proof.SnapshotIdentity = record.State.InitialSnapshot.Identity
			target.proof.DiscardedWork = discarded
		case errors.Is(loadErr, os.ErrNotExist) || !IsCompactAuthorityOperationalFailure(loadErr):
			// Missing review-state.json (an interrupted START) or a record that
			// cannot be read: quarantined whole without being interpreted.
		default:
			return compactAbandonPlan{}, fmt.Errorf("load abandon target: %w", loadErr)
		}
		var err2 error
		if target.residue, err2 = compactAbandonResidue(dir); err2 != nil {
			return compactAbandonPlan{}, err2
		}
		plan.steps = append(plan.steps, target)
		plan.revision, plan.snapshotIdentity, plan.discarded = target.proof.Revision, target.proof.SnapshotIdentity, target.proof.DiscardedWork
		// The cascade removes every readable successor of the target, so the
		// remaining graph can only have lost edges. Verify that invariant
		// before anything moves instead of trusting it.
		removed := scan.records
		for _, step := range plan.steps {
			removed = compactRecordsWithout(removed, step.lineage)
		}
		if graphErr := compactAuthorityRemovalRegression(scan.records, removed); graphErr != nil {
			return compactAbandonPlan{}, fmt.Errorf("review abandon refused: %w", graphErr)
		}
	}
	if len(lines) > 0 {
		plan.suffix = "\n" + strings.Join(lines, "\n")
	}
	return plan, nil
}

// AbandonCompactStore declares one review lineage lost and quarantines it,
// bound to the maintainer's exact authorization of what moves. It accepts
// every state:
//
//   - any lifecycle state, including terminal approved, escalated and
//     invalidated authority;
//   - an unreadable or missing review-state.json (including an interrupted
//     START), quarantined whole without interpreting it;
//   - a lineage that is the predecessor of recovery successors: the
//     successors are abandoned with it, deepest first, so no orphan recovery
//     edge ever remains. Ancestors and unrelated lineages are never touched;
//
// The authorization binding names the lineage and every extra entry that
// moves with it, so the human approves exactly what is abandoned. A
// discarded-work summary that cannot be derived degrades to "unverifiable"
// (named in the binding). The v2 store lock is taken without waiting: a live
// holder fails the call with the lock-busy error before anything moves. Every
// entry is byte-preserved in the audited quarantine; nothing is deleted. On a
// partial failure the records already committed (and the prepared record in
// flight) are returned alongside the error.
func AbandonCompactStore(ctx context.Context, repo string, request CompactAbandonRequest) (CompactAbandonResult, error) {
	if err := ctx.Err(); err != nil {
		return CompactAbandonResult{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return CompactAbandonResult{}, err
	}
	if strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.Actor) == "" {
		return CompactAbandonResult{}, errors.New("review abandon requires a non-empty reason and actor")
	}
	if strings.TrimSpace(request.ExpectedRevision) == "" {
		return CompactAbandonResult{}, errors.New("review abandon requires the exact current store revision")
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactAbandonResult{}, err
	}
	versionRoot := filepath.Join(base, "v2")
	dir := filepath.Join(versionRoot, request.LineageID)
	lock, err := acquireStoreLock(filepath.Join(versionRoot, "LOCK"))
	if err != nil {
		return CompactAbandonResult{}, err
	}
	defer lock.release()
	v2Present, err := compactAbandonEntryPresent(dir)
	if err != nil {
		return CompactAbandonResult{}, err
	}
	if !v2Present {
		replayed, replayErr := replayCommittedCompactAbandonment(base, request)
		return CompactAbandonResult{CompactReclaimRecord: replayed}, replayErr
	}
	plan, err := planCompactAbandon(ctx, repo, base, request.LineageID, v2Present)
	if err != nil {
		return CompactAbandonResult{}, err
	}
	if plan.revision != request.ExpectedRevision {
		return CompactAbandonResult{}, fmt.Errorf("%w: expected compact revision %q, current %q", ErrConcurrentUpdate, request.ExpectedRevision, plan.revision)
	}
	binding := compactAbandonAuthorizationBindingV2(request.LineageID, plan.revision, plan.snapshotIdentity, request.Actor, request.Reason, plan.discarded) + plan.suffix
	if strings.HasPrefix(request.MaintainerAuthorization, compactLegacyAbandonAuthorizationSchema) {
		// refusal:by-design human-authority: a retired V1 token has no valid V2 reason to preserve
		return CompactAbandonResult{}, fmt.Errorf("review abandon refused: v1 maintainer authorization is retired. %s", compactAbandonV2Rerun(repo, request.LineageID, plan.revision, request.Actor, CompactAbandonReasonOperatorDisposition,
			compactAbandonAuthorizationBindingV2(request.LineageID, plan.revision, plan.snapshotIdentity, request.Actor, CompactAbandonReasonOperatorDisposition, plan.discarded)+plan.suffix))
	}
	if !validCompactAbandonReason(request.Reason) {
		// refusal:by-design human-authority: only the closed disposition vocabulary can authorize discarded work
		return CompactAbandonResult{}, fmt.Errorf("review abandon requires reason %q or %q", CompactAbandonReasonOperatorDisposition, CompactAbandonReasonRetiredSchema)
	}
	if request.MaintainerAuthorization != binding {
		// refusal:by-design human-authority: only the maintainer can supply the exact V2 binding the rerun prints
		return CompactAbandonResult{}, fmt.Errorf("review abandon requires an exact maintainer authorization binding (schema %s over lineage %s@%s, snapshot %s, reason, discarded work, and every entry that moves with it). %s", CompactAbandonAuthorizationSchema, request.LineageID, plan.revision, plan.snapshotIdentity, compactAbandonV2Rerun(repo, request.LineageID, plan.revision, request.Actor, request.Reason, binding))
	}

	if request.AbandonedAt.IsZero() {
		request.AbandonedAt = time.Now().UTC()
	}
	records := make([]CompactReclaimRecord, 0, len(plan.steps))
	for _, step := range plan.steps {
		proof := step.proof
		// Only an extended binding (cascade, unverifiable work) is
		// kept: the replay rebuilds the plain eight-line binding from the proof,
		// so a simple abandonment record stays byte-compatible with older readers.
		if step.primary && plan.suffix != "" {
			proof.AuthorizationBinding = binding
		}
		quarantined, quarantineErr := quarantineCompactStoreEntry(ctx, base, step.dir, CompactReclaimRecord{
			Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: step.lineage,
			Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
			ReclaimedAt: request.AbandonedAt.UTC(), SourcePath: step.dir, Residue: step.residue, Abandonment: &proof,
		})
		if quarantineErr != nil {
			return compactAbandonResult(request.LineageID, base, append(records, quarantined)), quarantineErr
		}
		records = append(records, quarantined)
	}
	return compactAbandonResult(request.LineageID, base, records), nil
}

// compactAbandonResult picks the requested lineage's own v2 record as the
// primary one; everything else is reported as cascaded, in quarantine order.
func compactAbandonResult(lineage, base string, records []CompactReclaimRecord) CompactAbandonResult {
	primary := -1
	for index, record := range records {
		if record.LineageID == lineage && record.SourcePath == filepath.Join(base, "v2", lineage) {
			primary = index
		}
	}
	if primary < 0 {
		for index, record := range records {
			if record.LineageID == lineage {
				primary = index
			}
		}
	}
	if primary < 0 {
		primary := records[len(records)-1]
		primary.LineageID = lineage
		return CompactAbandonResult{CompactReclaimRecord: primary, Cascaded: records[:len(records)-1]}
	}
	result := CompactAbandonResult{CompactReclaimRecord: records[primary]}
	for index, record := range records {
		if index != primary {
			result.Cascaded = append(result.Cascaded, record)
		}
	}
	return result
}

// replayCommittedCompactAbandonment resolves an abandon request whose lineage
// holds no entry any more: an exact replay (same lineage, reason and actor)
// re-emits the committed record so repeated identical requests (same
// lineage, revision, reason, actor and authorization binding) converge
// without minting a second quarantine; anything else is refused.
func replayCommittedCompactAbandonment(base string, request CompactAbandonRequest) (CompactReclaimRecord, error) {
	quarantineRoot := filepath.Join(base, "quarantine")
	entries, err := os.ReadDir(quarantineRoot)
	if err != nil && !os.IsNotExist(err) {
		return CompactReclaimRecord{}, fmt.Errorf("inspect quarantine for abandonment replay: %w", err)
	}
	var matched *CompactReclaimRecord
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(quarantineRoot, entry.Name(), "reclaim-record.json"))
		if readErr != nil {
			continue
		}
		var record CompactReclaimRecord
		if json.Unmarshal(payload, &record) != nil || record.Status != CompactReclaimCommitted {
			continue
		}
		proof := record.Abandonment
		if proof == nil || proof.Schema != CompactAbandonAuthorizationSchema || proof.LineageID != request.LineageID ||
			proof.Revision != request.ExpectedRevision ||
			record.Reason != strings.TrimSpace(request.Reason) || record.Actor != strings.TrimSpace(request.Actor) {
			continue
		}
		binding := proof.AuthorizationBinding
		if binding == "" {
			binding = compactAbandonAuthorizationBindingV2(proof.LineageID, proof.Revision, proof.SnapshotIdentity, request.Actor, request.Reason, proof.DiscardedWork)
		}
		if request.MaintainerAuthorization != binding {
			continue
		}
		if matched == nil || record.SourcePath == filepath.Join(base, "v2", request.LineageID) {
			copied := record
			matched = &copied
		}
	}
	if matched != nil {
		return *matched, nil
	}
	// refusal:by-design operator-knowledge: the named lineage is absent; the operator must name an existing one, which a flagless `gentle-ai review abandon --cwd <repo>` lists
	return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q holds no review authority entry and no committed abandonment record matches the request; if a prior abandonment was interrupted, the prepared reclaim-record.json under %s locates the moved residue for manual reconciliation", request.LineageID, quarantineRoot)
}
