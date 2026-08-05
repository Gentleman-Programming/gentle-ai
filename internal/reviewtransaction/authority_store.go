// Package reviewtransaction — v3 new-lineage authority store (Wave 3 Slice
// 2, design decision 3). AuthorityStore persists exactly two artifacts per
// new lineage under <git-common-dir>/gentle-ai/review-transactions/v3/<lineage>/:
// review-state.json (CAS-mutable) and review-receipt.json (immutable
// terminal). v3/ is a sibling of v1/ and v2/ under the same authority root
// (reviewAuthorityRoot, store.go:150; the v2/ sibling-version-root layout
// convention is exercised at compact_inspect.go:93/121 and
// CompactAuthoritativeStore, compact_store.go:898-912) rather than a reuse
// of v2's directory — a shared directory would make "new lineages write no
// legacy artifact" unprovable by inspection.
//
// This file reuses, rather than reimplements, the store's existing
// primitives: acquireStoreLock (store_lock.go:159) for advisory locking,
// writeAtomic's .atomic-*/rename idiom (store.go:947) for the CAS-mutable
// state artifact, publishImmutable (store.go:981) for the terminal receipt,
// and the CompactRevisionForState content-addressed-revision idiom
// (compact_store.go:2003-2022) for NewLineageRevisionForState. The lock is
// store-scoped: v3/LOCK guards every lineage under this v3/ root, mirroring
// v2/LOCK (compact_store.go:911) — there is no per-lineage lock file.
//
// This file owns storage only. ReviewCore (review_core.go, Wave 3 Slice 3)
// is the sole caller that decides WHEN to Mutate or WriteReceipt and WHAT
// CoreTransition to record; AuthorityStore never derives review semantics
// (tier, lenses, budget, admission) on its own — those are frozen by the
// caller before Mutate is invoked.
package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// NewLineageAuthoritySchema identifies the v3 authority-store record shape.
// Lineage kind is established solely by v3/<lineage> record presence
// (design decision 4) — this schema string is the record's own assertion of
// that kind, never an independent marker checked separately from the record.
const NewLineageAuthoritySchema = "gentle-ai.review-authority/v3"

// NewLineageReceiptSchema identifies the v3 immutable terminal receipt shape.
const NewLineageReceiptSchema = "gentle-ai.review-receipt/v3"

// newLineageReplayDomain namespaces the replay-identity digest so it can
// never collide with any other content-addressed digest this package
// produces (the domain-prefix idiom makeCompactRecord already uses,
// compact_store.go:2008).
const newLineageReplayDomain = "gentle-ai.review-replay/v3"

// NewLineageState is the closed five-value persisted-state vocabulary
// (spec rdd-authority-store, "Five Persisted States, Everything Else
// Derived"). Nothing else may ever be assigned to NewLineageAuthority.State:
// invalidated/scope_changed/ambiguous/repairable/corrupted are a distinct,
// unpersisted concept a future reader derives at evaluation time (design
// decision 3) — this package exposes no field, method, or code path that
// writes one of those five words into review-state.json.
type NewLineageState string

const (
	NewLineageStateReviewing  NewLineageState = "reviewing"
	NewLineageStateCorrecting NewLineageState = "correcting"
	NewLineageStateValidating NewLineageState = "validating"
	NewLineageStateApproved   NewLineageState = "approved"
	NewLineageStateEscalated  NewLineageState = "escalated"
)

func (state NewLineageState) valid() bool {
	switch state {
	case NewLineageStateReviewing, NewLineageStateCorrecting, NewLineageStateValidating, NewLineageStateApproved, NewLineageStateEscalated:
		return true
	default:
		return false
	}
}

// NewLineageAuthority is the exact review-state.json payload (design
// decision 3): the promoted CandidateIdentity frozen at start, tier/lenses/
// budget frozen at creation and never recomputed by this store, admitted
// finding references, and an in-record replay identity so exact-replay
// detection needs no external journal (spec rdd-authority-store, "Replay
// identity is self-contained").
type NewLineageAuthority struct {
	LineageID             string            `json:"lineage_id"`
	State                 NewLineageState   `json:"state"`
	CandidateIdentity     CandidateIdentity `json:"candidate_identity"`
	Tier                  RiskLevel         `json:"tier"`
	SelectedLenses        []string          `json:"selected_lenses,omitempty"`
	CorrectionBudgetLines int               `json:"correction_budget_lines"`
	AdmittedFindingIDs    []string          `json:"admitted_finding_ids,omitempty"`
	ReplayIdentity        string            `json:"replay_identity,omitempty"`
	LastTransition        json.RawMessage   `json:"last_transition,omitempty"`
	// CapturedResults is C-A's minimal capture primitive (Wave 5 fix cycle 2,
	// coordinator decision: not a rebuilt v2-shaped admission pipeline --
	// ArtifactSubject/FrozenCandidateContext/reopen machinery is Wave 7
	// deletion scope). Written only through AuthorityStore.CaptureLensResult
	// (new_lineage_capture.go), never directly: one entry per captured lens,
	// one-shot (no reopen), each bound to its own provider-owned subject hash
	// and (C-E, fix cycle 3) its own validated findings.
	CapturedResults []NewLineageCapturedResult `json:"captured_results,omitempty"`
}

// NewLineageCapturedResult is one captured reviewer result's persisted
// binding: which lens, at which frozen selected-lens order, under which
// provider-owned subject hash (NewLineageArtifactSubjectHash), carrying the
// reviewer's OWN validated findings (C-E, Wave 5 fix cycle 3). Cycle 2
// deliberately omitted Findings ("no findings/evidence/admission-decision
// richness"); cycle 3's verify (CRITICAL-E) found that omission was a
// fail-open, not a scope boundary -- capture-result already required and
// validated findings/evidence were present, so silently discarding them let
// a candidate-causal BLOCKER approve unadmitted. It still carries no
// evidence/admission-DECISION richness (no re-derivable ArtifactSubject,
// no reopen) -- only the findings themselves, fed into the existing
// AdmitCandidateCausalFindings at finalize time.
type NewLineageCapturedResult struct {
	Lens        string            `json:"lens"`
	Order       int               `json:"order"`
	SubjectHash string            `json:"subject_hash"`
	Findings    []FindingEvidence `json:"findings,omitempty"`
}

// CapturedLensNames returns the plain lens-name list
// FinalizeAdvanceRequest.CapturedLensResults (and hasCapturedAllSelectedLenses)
// need, derived from the richer CapturedResults slice rather than storing
// the same names twice.
func (authority NewLineageAuthority) CapturedLensNames() []string {
	names := make([]string, len(authority.CapturedResults))
	for index, captured := range authority.CapturedResults {
		names[index] = captured.Lens
	}
	return names
}

// MissingCapturedLensNames returns the frozen SelectedLenses not yet present
// in CapturedLensNames, in SelectedLenses' own order (W-8, Wave 5 fix cycle
// 3, verify-report #10186 cycle 2): a partial capture is an ORDINARY
// incomplete state, not a fault, and naming exactly which lenses remain is
// what makes the continuation runnable rather than a bare "capture more".
func (authority NewLineageAuthority) MissingCapturedLensNames() []string {
	captured := make(map[string]bool, len(authority.CapturedResults))
	for _, result := range authority.CapturedResults {
		captured[result.Lens] = true
	}
	var missing []string
	for _, lens := range authority.SelectedLenses {
		if !captured[lens] {
			missing = append(missing, lens)
		}
	}
	return missing
}

// CapturedFindingEvidence flattens every captured lens's own Findings into
// the single []FindingEvidence AdmitCandidateCausalFindings consumes (C-E) --
// the exact shape --admission-findings already reads from a caller-supplied
// file, sourced here from the reviewer channel's own persisted captures
// instead. Order matches CapturedResults' own append order (capture order),
// which AdmitCandidateCausalFindings does not depend on (it partitions by
// Causality alone).
func (authority NewLineageAuthority) CapturedFindingEvidence() []FindingEvidence {
	var findings []FindingEvidence
	for _, captured := range authority.CapturedResults {
		findings = append(findings, captured.Findings...)
	}
	return findings
}

// Validate enforces the structural half of the two-artifact contract this
// slice owns. It does not — and cannot — validate review semantics (whether
// this tier/lens/budget combination is the one ClassifyRisk/SelectReviewLenses/
// CorrectionBudget would have produced): that binding is ReviewCore's
// responsibility (design decision 6, Wave 3 Slice 3), reused rather than
// re-derived here.
func (authority NewLineageAuthority) Validate() error {
	if err := validateLineageID(authority.LineageID); err != nil {
		return err
	}
	if !authority.State.valid() {
		return errors.New("new-lineage authority state must be one of the five persisted states") // refusal:by-design world-action: the caller (ReviewCore, Wave 3 Slice 3) constructs State; a bad value is a programmer invariant break, fixed by editing the caller, not by an operator command
	}
	if authority.CandidateIdentity == (CandidateIdentity{}) {
		return errors.New("new-lineage authority requires a frozen candidate identity") // refusal:by-design world-action: a zero CandidateIdentity means the caller never froze one; the fix is the caller's construction code, not an operator command
	}
	for _, tree := range []string{authority.CandidateIdentity.BaseTree, authority.CandidateIdentity.CandidateTree} {
		if !validGitTree(tree) {
			return errors.New("new-lineage authority candidate identity trees must be valid git tree ids") // refusal:by-design world-action: malformed tree ids can only come from a caller bug in identity resolution; the fix is that code, not an operator command
		}
	}
	switch authority.Tier {
	case RiskLow, RiskMedium, RiskHigh:
	default:
		return errors.New("new-lineage authority tier must be a native risk classification") // refusal:by-design world-action: Tier is frozen from ClassifyRisk's own closed vocabulary by the caller; an out-of-vocabulary value is a caller bug, not an operator-fixable state
	}
	if authority.CorrectionBudgetLines < 0 || authority.CorrectionBudgetLines > MaxCorrectionChangedLines {
		return fmt.Errorf("new-lineage authority correction budget must be between 0 and %d", MaxCorrectionChangedLines) // refusal:by-design world-action: CorrectionBudgetLines is frozen from CorrectionBudget's own bounded output by the caller; an out-of-range value is a caller bug, not an operator-fixable state
	}
	for _, lens := range authority.SelectedLenses {
		if strings.TrimSpace(lens) != lens || lens == "" {
			return errors.New("new-lineage authority selected lenses must be canonical non-empty strings") // refusal:by-design world-action: SelectedLenses is frozen from SelectReviewLenses's own output by the caller; malformed entries are a caller bug, not an operator-fixable state
		}
	}
	for _, id := range authority.AdmittedFindingIDs {
		if strings.TrimSpace(id) != id || id == "" {
			return errors.New("new-lineage authority admitted finding ids must be canonical non-empty strings") // refusal:by-design world-action: AdmittedFindingIDs is set by ReviewCore's causal-admission logic; malformed entries are a caller bug, not an operator-fixable state
		}
	}
	seenCapturedLenses := make(map[string]bool, len(authority.CapturedResults))
	for _, captured := range authority.CapturedResults {
		if strings.TrimSpace(captured.Lens) == "" || captured.Order < 0 || !validSHA256(captured.SubjectHash) {
			return errors.New("new-lineage authority captured results must carry a non-empty lens, a non-negative order, and a canonical subject hash") // refusal:by-design world-action: captured results are only ever written by AuthorityStore.CaptureLensResult after validating them; malformed entries here mean in-process corruption, not something an operator command repairs
		}
		if seenCapturedLenses[captured.Lens] {
			return errors.New("new-lineage authority captured results must name each lens at most once") // refusal:by-design world-action: CaptureLensResult enforces one-shot-per-lens before ever appending; a duplicate here means in-process corruption, not something an operator command repairs
		}
		seenCapturedLenses[captured.Lens] = true
		for _, finding := range captured.Findings {
			if strings.TrimSpace(finding.FindingID) == "" {
				return errors.New("new-lineage authority captured findings must carry a non-empty finding id") // refusal:by-design world-action: captured findings are only ever written by AuthorityStore.CaptureLensResult from an already-decoded reviewer result; a malformed entry here means in-process corruption, not something an operator command repairs
			}
		}
	}
	if authority.ReplayIdentity != "" && !validSHA256(authority.ReplayIdentity) {
		return errors.New("new-lineage authority replay identity must be a canonical digest") // refusal:by-design world-action: the replay identity is only ever set by RecordTransition's own digest computation; a malformed value means in-process corruption, not something an operator command repairs
	}
	if len(authority.LastTransition) > 0 && !json.Valid(authority.LastTransition) {
		return errors.New("new-lineage authority last transition must be valid JSON") // refusal:by-design world-action: the transition payload is only ever set by RecordTransition after validating it; malformed JSON here means in-process corruption, not something an operator command repairs
	}
	return nil
}

// NewLineageRecord is the persisted review-state.json envelope: the schema
// identity, the content-addressed revision (the CAS token — the
// CompactRevisionForState idiom, reused rather than reimplemented), and the
// authority payload itself.
type NewLineageRecord struct {
	Schema    string              `json:"schema"`
	Revision  string              `json:"revision"`
	Authority NewLineageAuthority `json:"authority"`
}

func makeNewLineageRecord(authority NewLineageAuthority) (NewLineageRecord, []byte, error) {
	authorityPayload, err := json.Marshal(authority)
	if err != nil {
		return NewLineageRecord{}, nil, err
	}
	sum := sha256.Sum256(append([]byte(NewLineageAuthoritySchema+"\x00"), authorityPayload...))
	record := NewLineageRecord{Schema: NewLineageAuthoritySchema, Revision: "sha256:" + hex.EncodeToString(sum[:]), Authority: authority}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return NewLineageRecord{}, nil, err
	}
	return record, append(payload, '\n'), nil
}

// NewLineageRevisionForState derives the exact content-addressed revision a
// given authority would receive without writing it — the
// CompactRevisionForState idiom (compact_store.go:2019), reused for the v3
// record shape rather than reimplemented.
func NewLineageRevisionForState(authority NewLineageAuthority) (string, error) {
	record, _, err := makeNewLineageRecord(authority)
	return record.Revision, err
}

func parseNewLineageRecord(payload []byte, lineageID string) (NewLineageRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var record NewLineageRecord
	if err := decoder.Decode(&record); err != nil {
		return NewLineageRecord{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return NewLineageRecord{}, errors.New("multiple JSON values in new-lineage authority record") // refusal:by-design world-action: a well-formed writer never appends a second JSON value; this can only follow direct on-disk tampering or storage corruption outside any command's reach
	}
	if record.Schema != NewLineageAuthoritySchema {
		return NewLineageRecord{}, errors.New("invalid new-lineage authority schema") // refusal:by-design world-action: a mismatched schema string can only come from a foreign or corrupted file at this exact path; there is no command that repairs storage corruption, only maintainer inspection
	}
	if err := record.Authority.Validate(); err != nil {
		return NewLineageRecord{}, err
	}
	if lineageID != "" && record.Authority.LineageID != lineageID {
		return NewLineageRecord{}, errors.New("new-lineage authority record lineage does not match store") // refusal:by-design world-action: a lineage mismatch between the directory and its own record content is storage corruption or a caller path bug, not an operator-fixable state
	}
	want, err := NewLineageRevisionForState(record.Authority)
	if err != nil || want != record.Revision {
		return NewLineageRecord{}, errors.New("new-lineage authority record revision does not match its content") // refusal:by-design world-action: a revision that does not match its own content is storage corruption (the content-addressed digest is recomputed, never trusted from disk); no command repairs corrupted authority content
	}
	return record, nil
}

// AuthorityStore is the v3 two-artifact store for one new lineage.
// lockPath is store-scoped (design decision 3): every lineage under the
// same v3/ root shares v3/LOCK — there is no per-lineage lock file.
type AuthorityStore struct {
	Dir       string
	lineageID string
	repo      string
	lockPath  string
}

// NewLineageAuthorityStore resolves the v3 store for one lineage under the
// resolved common-dir authority root (reviewAuthorityRoot, store.go:150),
// the same root v1/v2 already share — v3 is a sibling version root, not a
// reuse of v2's directory.
func NewLineageAuthorityStore(ctx context.Context, repo, lineageID string) (AuthorityStore, error) {
	if err := validateLineageID(lineageID); err != nil {
		return AuthorityStore{}, err
	}
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return AuthorityStore{}, err
	}
	versionRoot := filepath.Join(base, "v3")
	return AuthorityStore{
		Dir: filepath.Join(versionRoot, lineageID), lineageID: lineageID, repo: root,
		lockPath: filepath.Join(versionRoot, "LOCK"),
	}, nil
}

func (store AuthorityStore) StatePath() string { return filepath.Join(store.Dir, compactStateFileName) }
func (store AuthorityStore) ReceiptPath() string {
	return filepath.Join(store.Dir, compactReceiptFileName)
}

// Load reads the current record. Reads never take the store lock:
// writeAtomic's rename-publish guarantees a reader either sees the previous
// complete bytes or the next complete bytes, never a torn write
// (store.go:947), the same guarantee CompactStore.loadCompactRecordLocked
// relies on for its own uncoordinated reads.
func (store AuthorityStore) Load() (NewLineageRecord, error) {
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		return NewLineageRecord{}, err
	}
	return parseNewLineageRecord(payload, store.lineageID)
}

// NewLineageRevisionConflictError proves a Mutate call wrote nothing: the
// expected revision the caller supplied no longer matches the current
// record (spec rdd-authority-store, "Stale revision refuses the write").
type NewLineageRevisionConflictError struct {
	LineageID        string
	ExpectedRevision string
	CurrentRevision  string
}

func (err *NewLineageRevisionConflictError) Error() string {
	return fmt.Sprintf("new-lineage authority %q revision changed concurrently: expected %q, current %q",
		err.LineageID, err.ExpectedRevision, err.CurrentRevision)
}

// Mutate applies apply to the current authority under compare-and-swap on
// expectedRevision (spec rdd-authority-store, "CAS Mutation With In-Record
// Replay Identity"). A lineage that does not yet exist has an empty current
// authority and an empty expectedRevision. apply must set every field the
// resulting NewLineageAuthority needs — Mutate does not infer state
// transitions or review semantics; deciding what changes is ReviewCore's
// job (Wave 3 Slice 3). Applying a mutation that leaves the authority
// byte-identical to the current record is a no-op: it returns the current
// revision and writes nothing, matching replaceContextGuarded's own
// idempotent-retry behavior (compact_store.go:1690-1692).
func (store AuthorityStore) Mutate(ctx context.Context, expectedRevision string, apply func(*NewLineageAuthority) error) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if apply == nil {
		return "", errors.New("new-lineage authority mutation requires an apply function") // refusal:by-design world-action: a nil apply function is a caller programming error at the call site; the fix is that code, not an operator command
	}
	lock, err := acquireStoreLock(store.lockPath)
	if err != nil {
		return "", err
	}
	defer lock.release()

	var current *NewLineageRecord
	payload, err := os.ReadFile(store.StatePath())
	if err == nil {
		loaded, parseErr := parseNewLineageRecord(payload, store.lineageID)
		if parseErr != nil {
			return "", parseErr
		}
		current = &loaded
	} else if !os.IsNotExist(err) {
		return "", err
	}
	currentRevision := ""
	var next NewLineageAuthority
	if current != nil {
		currentRevision = current.Revision
		next = current.Authority
	}
	if currentRevision != expectedRevision {
		return "", &NewLineageRevisionConflictError{LineageID: store.lineageID, ExpectedRevision: expectedRevision, CurrentRevision: currentRevision}
	}
	if err := apply(&next); err != nil {
		return "", err
	}
	if next.LineageID == "" {
		next.LineageID = store.lineageID
	}
	if store.lineageID != "" && next.LineageID != store.lineageID {
		return "", fmt.Errorf("%w: new-lineage authority does not match store", ErrInvalidSuccessor)
	}
	if err := next.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidSuccessor, err)
	}
	record, recordPayload, err := makeNewLineageRecord(next)
	if err != nil {
		return "", err
	}
	if current != nil && current.Revision == record.Revision {
		return record.Revision, nil
	}
	if err := writeAtomic(store.StatePath(), recordPayload, 0o644); err != nil {
		return "", err
	}
	return record.Revision, nil
}

// NewLineageReplayIdentity is the exact-replay identity described by design
// decision 3: sha256(domain, lineage, state, candidate identity, request
// digest). It is self-contained in review-state.json — no external journal
// is consulted to decide whether a request is a byte-equal replay.
func NewLineageReplayIdentity(lineageID string, state NewLineageState, candidate CandidateIdentity, requestDigest string) (string, error) {
	if err := validateLineageID(lineageID); err != nil {
		return "", err
	}
	if !state.valid() {
		return "", errors.New("new-lineage replay identity requires one of the five persisted states") // refusal:by-design world-action: this mirrors the State invariant above: only a caller bug supplies a state outside the five-value vocabulary
	}
	payload, err := json.Marshal(struct {
		LineageID     string            `json:"lineage_id"`
		State         NewLineageState   `json:"state"`
		Candidate     CandidateIdentity `json:"candidate_identity"`
		RequestDigest string            `json:"request_digest"`
	}{lineageID, state, candidate, requestDigest})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(newLineageReplayDomain+"\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// RecordTransition sets the fields that make a later exact-replay lookup
// self-contained: the replay identity computed for THIS transition, and its
// opaque byte payload. transition is ReviewCore's CoreTransition (Wave 3
// Slice 3), stored as json.RawMessage so AuthorityStore never depends on
// ReviewCore's transition type — the storage layer never re-derives review
// semantics from what it persists.
func (authority *NewLineageAuthority) RecordTransition(requestDigest string, transition json.RawMessage) error {
	identity, err := NewLineageReplayIdentity(authority.LineageID, authority.State, authority.CandidateIdentity, requestDigest)
	if err != nil {
		return err
	}
	if len(transition) > 0 && !json.Valid(transition) {
		return errors.New("new-lineage transition payload must be valid JSON") // refusal:by-design world-action: RecordTransition validates its own input; malformed JSON here is a caller bug in what it hands to the transition, not an operator-fixable state
	}
	authority.ReplayIdentity = identity
	authority.LastTransition = append(json.RawMessage(nil), transition...)
	return nil
}

// ErrOneCorrectionOnly is refused when a non-replay request arrives while
// the lineage is already correcting: ordinary review permits exactly one
// bounded correction transaction (spec rdd-review-core-transitions, "One
// Bounded Correction, Exact Replay Exempt"; design decision 3, "different
// request in correcting ⇒ refuse (budget spent)").
var ErrOneCorrectionOnly = errors.New("new-lineage authority already has one bounded correction in flight") // refusal:by-design world-action: the one-bounded-correction rule is a review policy invariant ReviewCore enforces on the caller's behalf; there is no command that grants a second correction, only a fresh lineage

// ResolveReplay decides, from review-state.json alone, whether requestDigest
// is an exact replay of the last recorded transition. When it is, the
// stored transition is returned unchanged and the caller MUST NOT call
// Mutate — consuming no budget (spec rdd-authority-store, "Replay identity
// is self-contained"). When the authority is `correcting` and the request is
// not a replay, ResolveReplay refuses: the one-correction budget is already
// spent.
func (record NewLineageRecord) ResolveReplay(requestDigest string) (transition json.RawMessage, isReplay bool, err error) {
	identity, err := NewLineageReplayIdentity(record.Authority.LineageID, record.Authority.State, record.Authority.CandidateIdentity, requestDigest)
	if err != nil {
		return nil, false, err
	}
	if record.Authority.ReplayIdentity != "" && identity == record.Authority.ReplayIdentity {
		return record.Authority.LastTransition, true, nil
	}
	if record.Authority.State == NewLineageStateCorrecting {
		return nil, false, ErrOneCorrectionOnly
	}
	return nil, false, nil
}

// NewLineageReceipt is the exact review-receipt.json payload: the immutable
// terminal artifact issued exactly once per lineage (spec
// rdd-review-core-transitions, "Terminal Receipt Issuance Exactly Once").
// Its shape is deliberately minimal here — ReviewCore (Wave 3 Slice 3) and
// the reason-taxonomy regression (Wave 3 Slice 5) both extend what a
// terminal transition carries; this slice proves only that whatever content
// is issued becomes byte-immutable and bound to the exact authority
// revision that authorized it.
type NewLineageReceipt struct {
	Schema            string            `json:"schema"`
	LineageID         string            `json:"lineage_id"`
	TerminalState     NewLineageState   `json:"terminal_state"`
	AuthorityRevision string            `json:"authority_revision"`
	CandidateIdentity CandidateIdentity `json:"candidate_identity"`
	IssuedTransition  json.RawMessage   `json:"issued_transition,omitempty"`
}

// Validate enforces the structural half of receipt issuance; ReviewCore
// binds the semantic content (design decision 6).
func (receipt NewLineageReceipt) Validate() error {
	if receipt.Schema != NewLineageReceiptSchema {
		return errors.New("invalid new-lineage receipt schema") // refusal:by-design world-action: a mismatched schema string can only come from a foreign or corrupted file at this exact path; there is no command that repairs storage corruption, only maintainer inspection
	}
	if err := validateLineageID(receipt.LineageID); err != nil {
		return err
	}
	if receipt.TerminalState != NewLineageStateApproved && receipt.TerminalState != NewLineageStateEscalated {
		return errors.New("new-lineage receipt terminal state must be approved or escalated") // refusal:by-design world-action: TerminalState is constructed by the caller from the closed five-state vocabulary; an out-of-vocabulary value is a caller bug, not an operator-fixable state
	}
	if !validSHA256(receipt.AuthorityRevision) {
		return errors.New("new-lineage receipt must bind to a valid authority revision") // refusal:by-design world-action: AuthorityRevision is the caller's own copy of a content-addressed digest it already read; a malformed value is a caller bug in that copy, not an operator-fixable state
	}
	if receipt.CandidateIdentity == (CandidateIdentity{}) {
		return errors.New("new-lineage receipt requires the frozen candidate identity") // refusal:by-design world-action: a zero CandidateIdentity on a receipt means the caller never copied the frozen one from the authority record; the fix is that code, not an operator command
	}
	if len(receipt.IssuedTransition) > 0 && !json.Valid(receipt.IssuedTransition) {
		return errors.New("new-lineage receipt issued transition must be valid JSON") // refusal:by-design world-action: the issued-transition payload is the caller's own copy of an already-validated transition; malformed JSON here is a caller bug, not an operator-fixable state
	}
	return nil
}

// WriteReceipt publishes review-receipt.json exactly once (spec
// rdd-authority-store, "Receipt Immutability After Issuance"): it binds the
// receipt to the exact authority revision, state, and candidate identity
// recorded under the store lock, then reuses publishImmutable
// (store.go:981) — a second WriteReceipt with byte-identical content is
// idempotent, and one with different content is refused with the original
// bytes unchanged (ImmutablePublicationConflictError).
func (store AuthorityStore) WriteReceipt(ctx context.Context, receipt NewLineageReceipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := receipt.Validate(); err != nil {
		return err
	}
	lock, err := acquireStoreLock(store.lockPath)
	if err != nil {
		return err
	}
	defer lock.release()

	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		return err
	}
	record, err := parseNewLineageRecord(payload, store.lineageID)
	if err != nil {
		return err
	}
	if record.Authority.LineageID != receipt.LineageID || record.Revision != receipt.AuthorityRevision {
		return errors.New("new-lineage receipt does not match authority") // refusal:by-design world-action: a receipt whose identity/revision does not match the authority it claims to close is a caller construction bug; there is no command that reconciles a mismatched receipt, only correct construction
	}
	if record.Authority.State != receipt.TerminalState {
		return errors.New("new-lineage receipt terminal state does not match authority state") // refusal:by-design world-action: the terminal receipt must reflect the exact state Mutate already committed; a mismatch is a caller ordering bug (WriteReceipt called before or after the matching Mutate), not an operator-fixable state
	}
	if record.Authority.CandidateIdentity != receipt.CandidateIdentity {
		return errors.New("new-lineage receipt candidate identity does not match authority") // refusal:by-design world-action: the receipt must carry the exact frozen identity the authority already recorded; a mismatch is a caller construction bug, not an operator-fixable state
	}
	receiptPayload, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	return publishImmutable(store.ReceiptPath(), append(receiptPayload, '\n'), 0o644)
}

// LoadReceipt reads and structurally validates the immutable terminal
// receipt this store may have published (WriteReceipt). C5 remediation
// (verify-report CRITICAL, "default-deny at gates"): gate authorization for
// an `approved` authority MUST NOT trust the persisted state field alone —
// it must confirm a receipt was genuinely published for it. Any read,
// parse, or structural-validation failure here is reported as an error; the
// caller (resolveGoverningAuthority) treats that identically to "no receipt
// exists", never as "trust the state field instead".
func (store AuthorityStore) LoadReceipt() (NewLineageReceipt, error) {
	payload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		return NewLineageReceipt{}, err
	}
	var receipt NewLineageReceipt
	if err := json.Unmarshal(payload, &receipt); err != nil {
		return NewLineageReceipt{}, err
	}
	if err := receipt.Validate(); err != nil {
		return NewLineageReceipt{}, err
	}
	return receipt, nil
}
