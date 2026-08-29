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

// CompactAbandonAuthorizationSchema is the first line of the exact nine-line
// LF-only abandonment maintainer authorization binding.
//
// It is exported so a refusal in another package that names `review abandon`
// as its continuation can print the template and be checked against the schema
// this gate actually verifies, rather than against a copy of the string.
const CompactAbandonAuthorizationSchema = "gentle-ai.review-abandon-authorization/v2"

const compactLegacyAbandonAuthorizationSchema = "gentle-ai.review-abandon-authorization/v1"

// CompactAbandonRequest identifies one non-terminal compact-v2 review lineage to
// quarantine, together with the exact maintainer authorization binding for
// that content.
type CompactAbandonRequest struct {
	LineageID               string
	ExpectedRevision        string
	Reason                  string
	Actor                   string
	MaintainerAuthorization string
	AbandonedAt             time.Time
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

type CompactDiscardedWorkSummary struct {
	CapturedLensResults []string `json:"captured_lens_results"`
	FindingsPresent     bool     `json:"findings_present"`
}

type CompactAbandonmentProof struct {
	Schema           string                      `json:"schema"`
	LineageID        string                      `json:"lineage_id"`
	Revision         string                      `json:"revision"`
	SnapshotIdentity string                      `json:"snapshot_identity"`
	DiscardedWork    CompactDiscardedWorkSummary `json:"discarded_work"`
}

// RenderCompactAbandonAuthorization renders a V2 authorization binding over
// the exact discarded work that the gate will re-derive before accepting it.
func RenderCompactAbandonAuthorization(lineage, revision, snapshotIdentity, actor, reason string, discarded CompactDiscardedWorkSummary) string {
	return compactAbandonAuthorizationBindingV2(lineage, revision, snapshotIdentity, actor, reason, discarded)
}

func compactAbandonAuthorizationBindingV2(lineage, revision, snapshotIdentity, actor, reason string, discarded CompactDiscardedWorkSummary) string {
	return CompactAbandonAuthorizationSchema + "\nlineage=" + lineage + "\nrevision=" + revision +
		"\nsnapshot_identity=" + snapshotIdentity + "\nreason=" + reason +
		"\ncaptured_lens_results=" + strings.Join(discarded.CapturedLensResults, ",") +
		fmt.Sprintf("\nfindings_present=%t", discarded.FindingsPresent) +
		"\nactor=" + strings.TrimSpace(actor)
}

func validCompactAbandonReason(reason string) bool {
	return reason == CompactAbandonReasonOperatorDisposition || reason == CompactAbandonReasonRetiredSchema
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
		if err != nil || envelope.Schema != admittedReviewerResultSchemaForSubject(expected) || envelope.Subject != expected || envelope.Admission.Validate(expected) != nil {
			return CompactDiscardedWorkSummary{}, errors.New("admit captured reviewer record value") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		result, admitted := reAdmitCompactReviewerResult(ctx, envelope, expected, nativeFrozen)
		if !admitted || result.ResultHash != entry.ResultHash {
			return CompactDiscardedWorkSummary{}, errors.New("admit captured reviewer record value") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
	}
	return summary, nil
}

func compactAbandonV2Rerun(repo, lineage, revision, snapshotIdentity, actor, reason string, discarded CompactDiscardedWorkSummary) string {
	return fmt.Sprintf("Re-run `gentle-ai review abandon --cwd %q --lineage %q --expected-revision %q --reason %q --actor %q --maintainer-authorization <the exact binding below>` using schema %s:\n%s",
		repo, lineage, revision, reason, strings.TrimSpace(actor), CompactAbandonAuthorizationSchema,
		compactAbandonAuthorizationBindingV2(lineage, revision, snapshotIdentity, actor, reason, discarded))
}

// compactAbandonTerminalState follows the public authority-status terminal
// contract. An invalidated lineage remains auditable, but is no longer an
// abandonment target.
func compactAbandonTerminalState(state State) bool {
	switch authorityStatusForState(state) {
	case AuthorityStatusApproved, AuthorityStatusEscalated, AuthorityStatusInvalidated:
		return true
	default:
		return false
	}
}

// CompactAbandonEligibility reports whether `review abandon` would accept one
// lineage right now, together with the two persisted values the authorization
// binding has to carry. Both are published so a caller naming the abandonment
// can print them concrete instead of sending the operator to look them up.
type CompactAbandonEligibility struct {
	Eligible         bool
	Revision         string
	SnapshotIdentity string
	DiscardedWork    CompactDiscardedWorkSummary
}

// InspectCompactPristineAbandonment answers, read-only, whether
// AbandonPristineCompactStore would accept this lineage. It applies the same
// terminal-state, discarded-work, superseded and remaining-graph rules as the
// real operation, and takes no lock and writes nothing.
//
// It is fail-closed by construction: every unreadable, missing, mixed-store,
// terminal, superseded or graph-breaking target answers
// not eligible. The maintainer authorization and the compare-and-swap are
// deliberately NOT predicted — they are the operator's own act, and a probe
// that claimed to know them would be claiming the maintainer had already
// decided.
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
	dir := filepath.Join(base, "v2", lineage)
	store := CompactStore{Dir: dir, lineageID: lineage, repo: repo}
	record, err := store.Load()
	if err != nil {
		return CompactAbandonEligibility{}, nil
	}
	if _, statErr := os.Stat(filepath.Join(base, "v1", lineage)); statErr == nil || !os.IsNotExist(statErr) {
		return CompactAbandonEligibility{}, nil
	}
	if compactAbandonTerminalState(record.State.State) {
		return CompactAbandonEligibility{}, nil
	}
	discarded, err := compactDiscardedWorkSummary(ctx, store, record)
	if err != nil {
		return CompactAbandonEligibility{}, nil
	}
	// Scoped like every other authority walk (#2743): the probe answers for
	// THIS lineage, whose own record already loaded above. An unreadable
	// foreign record is absent from the graph, never a repository-wide
	// not-eligible verdict that silently locks the abandon escape valve.
	scan, err := scanCompactAuthority(ctx, repo)
	if err != nil {
		return CompactAbandonEligibility{}, err
	}
	records := scan.records
	for _, related := range records {
		if related.State.Recovery != nil && related.State.Recovery.PredecessorLineageID == lineage {
			return CompactAbandonEligibility{}, nil
		}
	}
	if graphErr := compactAuthorityRemovalRegression(records, compactRecordsWithout(records, lineage)); graphErr != nil {
		return CompactAbandonEligibility{}, nil
	}
	return CompactAbandonEligibility{
		Eligible: true, Revision: record.Revision, SnapshotIdentity: record.State.InitialSnapshot.Identity, DiscardedWork: discarded,
	}, nil
}

// AbandonPristineCompactStore quarantines a non-terminal compact-v2 review
// lineage with V2 authorization bound to its exact discarded-work summary.
// Terminal and graph-breaking targets are refused; source residue is retained
// in the audit record.
func AbandonPristineCompactStore(ctx context.Context, repo string, request CompactAbandonRequest) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return CompactReclaimRecord{}, err
	}
	if strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.Actor) == "" {
		return CompactReclaimRecord{}, errors.New("review abandon requires a non-empty reason and actor")
	}
	if strings.TrimSpace(request.ExpectedRevision) == "" {
		return CompactReclaimRecord{}, errors.New("review abandon requires the exact current store revision")
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	versionRoot := filepath.Join(base, "v2")
	dir := filepath.Join(versionRoot, request.LineageID)
	lock, err := acquireStoreLock(filepath.Join(versionRoot, "LOCK"))
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	defer lock.release()
	store := CompactStore{Dir: dir, lineageID: request.LineageID, repo: repo}
	record, err := store.Load()
	if err != nil {
		if os.IsNotExist(err) {
			return replayCommittedCompactAbandonment(base, request)
		}
		return CompactReclaimRecord{}, fmt.Errorf("load abandon target: %w", err)
	}
	if _, statErr := os.Stat(filepath.Join(base, "v1", request.LineageID)); statErr == nil {
		return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q also exists in the legacy-v1 authority store; mixed-store collisions require explicit maintainer reconciliation", request.LineageID)
	} else if !os.IsNotExist(statErr) {
		return CompactReclaimRecord{}, fmt.Errorf("inspect same-lineage legacy authority: %w", statErr)
	}
	if compactAbandonTerminalState(record.State.State) {
		// refusal:by-design human-authority: terminal authority has no review-abandon continuation
		return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q holds terminal %q authority", request.LineageID, record.State.State)
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("inspect abandon target: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		residue = append(residue, item.Name())
	}
	if record.Revision != request.ExpectedRevision {
		return CompactReclaimRecord{}, fmt.Errorf("%w: expected compact revision %q, current %q", ErrConcurrentUpdate, request.ExpectedRevision, record.Revision)
	}
	discarded, err := compactDiscardedWorkSummary(ctx, store, record)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	if strings.HasPrefix(request.MaintainerAuthorization, compactLegacyAbandonAuthorizationSchema) {
		// refusal:by-design human-authority: a retired V1 token has no valid V2 reason to preserve
		return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: v1 maintainer authorization is retired. %s", compactAbandonV2Rerun(repo, request.LineageID, record.Revision, record.State.InitialSnapshot.Identity, request.Actor, CompactAbandonReasonOperatorDisposition, discarded))
	}
	if !validCompactAbandonReason(request.Reason) {
		// refusal:by-design human-authority: only the closed disposition vocabulary can authorize discarded work
		return CompactReclaimRecord{}, fmt.Errorf("review abandon requires reason %q or %q", CompactAbandonReasonOperatorDisposition, CompactAbandonReasonRetiredSchema)
	}
	if request.MaintainerAuthorization != compactAbandonAuthorizationBindingV2(request.LineageID, record.Revision, record.State.InitialSnapshot.Identity, request.Actor, request.Reason, discarded) {
		// refusal:by-design human-authority: only the maintainer can supply the exact V2 binding the rerun prints
		return CompactReclaimRecord{}, fmt.Errorf("review abandon requires an exact maintainer authorization binding (schema %s over lineage %s@%s, snapshot %s, reason, and discarded work). %s", CompactAbandonAuthorizationSchema, request.LineageID, record.Revision, record.State.InitialSnapshot.Identity, compactAbandonV2Rerun(repo, request.LineageID, record.Revision, record.State.InitialSnapshot.Identity, request.Actor, request.Reason, discarded))
	}
	// Scoped like every other authority walk (#2743): abandon quarantines
	// THIS lineage, whose record, revision, and authorization binding were
	// already validated above. An unreadable foreign record is absent from
	// the graph — the superseded and remaining-graph rules below judge only
	// the records that actually load — so one historical entry can never
	// refuse every abandonment in the repository.
	scan, err := scanCompactAuthority(ctx, repo)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	records := scan.records
	for lineage, related := range records {
		if related.State.Recovery != nil && related.State.Recovery.PredecessorLineageID == request.LineageID {
			return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q is superseded by recovery successor %q; superseded history is never abandoned", request.LineageID, lineage)
		}
	}
	if err := compactAuthorityRemovalRegression(records, compactRecordsWithout(records, request.LineageID)); err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: %w", err)
	}
	sort.Strings(residue)
	if request.AbandonedAt.IsZero() {
		request.AbandonedAt = time.Now().UTC()
	}
	quarantined := CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: request.LineageID,
		Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		ReclaimedAt: request.AbandonedAt.UTC(), SourcePath: dir, Residue: residue,
	}
	quarantined.Abandonment = &CompactAbandonmentProof{Schema: CompactAbandonAuthorizationSchema,
		LineageID: request.LineageID, Revision: record.Revision, SnapshotIdentity: record.State.InitialSnapshot.Identity, DiscardedWork: discarded}
	return quarantineCompactStoreEntry(ctx, base, dir, quarantined)
}

// replayCommittedCompactAbandonment resolves an abandon request whose lineage
// no longer holds a v2 entry: an exact replay finishes a prepared or
// re-emits the committed record so repeated identical requests converge
// without minting a second quarantine; anything else is refused.
func replayCommittedCompactAbandonment(base string, request CompactAbandonRequest) (CompactReclaimRecord, error) {
	quarantineRoot := filepath.Join(base, "quarantine")
	entries, err := os.ReadDir(quarantineRoot)
	if err != nil && !os.IsNotExist(err) {
		return CompactReclaimRecord{}, fmt.Errorf("inspect quarantine for abandonment replay: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(quarantineRoot, entry.Name(), "reclaim-record.json"))
		if readErr != nil {
			continue
		}
		var record CompactReclaimRecord
		if json.Unmarshal(payload, &record) != nil {
			continue
		}
		if record.Status != CompactReclaimCommitted {
			continue
		}
		proof := record.Abandonment
		if proof == nil || proof.Schema != CompactAbandonAuthorizationSchema {
			continue
		}
		lineageID, revision, snapshotIdentity := proof.LineageID, proof.Revision, proof.SnapshotIdentity
		binding := compactAbandonAuthorizationBindingV2(lineageID, revision, snapshotIdentity, request.Actor, request.Reason, proof.DiscardedWork)
		if lineageID != request.LineageID || revision != request.ExpectedRevision ||
			record.Reason != strings.TrimSpace(request.Reason) || record.Actor != strings.TrimSpace(request.Actor) {
			continue
		}
		if request.MaintainerAuthorization != binding {
			continue
		}
		if record.Status == CompactReclaimPrepared {
			quarantineDir := filepath.Join(quarantineRoot, entry.Name())
			residue, statErr := os.Lstat(filepath.Join(quarantineDir, "residue"))
			if record.LineageID != request.LineageID || record.SourcePath != filepath.Join(base, "v2", request.LineageID) || record.QuarantinePath != quarantineDir || statErr != nil || !residue.IsDir() {
				continue
			}
			record.Status = CompactReclaimCommitted
			if persistErr := persistReclaimRecord(record); persistErr != nil {
				return record, fmt.Errorf("commit prepared abandonment replay: %w", persistErr)
			}
		}
		return record, nil
	}
	return CompactReclaimRecord{}, fmt.Errorf("review abandon refused: lineage %q holds no compact authority state and no committed abandonment record matches the request; if a prior abandonment was interrupted, the prepared reclaim-record.json under %s locates the moved residue for manual reconciliation", request.LineageID, quarantineRoot)
}
