package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// compactResultDispositionAuthorizationSchema is the first line of the exact
// eleven-line LF-only reviewer-result disposition maintainer authorization
// binding; the remaining lines are, in order: repository, lineage, revision,
// target_identity, lens, order, artifact_digest, class, actor, and reason.
const compactResultDispositionAuthorizationSchema = "gentle-ai.review-result-disposition-authorization/v1"

// CompactResultDispositionOperation is the audited compact-v2 operation that
// terminally escalates a reviewing lineage whose preserved reviewer result
// cannot be admitted.
const CompactResultDispositionOperation = "review/dispose-result"

// CompactResultDispositionRequest identifies one preserved incident artifact
// that cannot be replayed through capture-result, together with the exact
// candidate-inapplicability evidence and maintainer authorization binding that
// justify terminally escalating its lineage.
type CompactResultDispositionRequest struct {
	LineageID               string
	ExpectedRevision        string
	TargetIdentity          string
	Lens                    string
	SelectedOrder           int
	ArtifactDigest          string
	Class                   ResultDispositionClass
	Diagnostic              string
	AbsentPaths             []string
	Reason                  string
	Actor                   string
	MaintainerAuthorization string
	DisposedAt              time.Time
}

// CompactResultDispositionRecord reports the committed disposition together
// with the escalated authority revision and the captured reviewer results the
// transition retained untouched.
type CompactResultDispositionRecord struct {
	LineageID           string                   `json:"lineage_id"`
	PreviousRevision    string                   `json:"previous_revision"`
	Revision            string                   `json:"revision"`
	State               State                    `json:"state"`
	ArtifactPath        string                   `json:"artifact_path"`
	Disposition         CompactResultDisposition `json:"disposition"`
	RetainedLensResults []string                 `json:"retained_lens_results"`
	Replayed            bool                     `json:"replayed"`
}

func compactResultDispositionAuthorizationBinding(repository, lineage, revision, targetIdentity, lens string, order int, artifactDigest string, class ResultDispositionClass, actor, reason string) string {
	return compactResultDispositionAuthorizationSchema +
		"\nrepository=" + repository +
		"\nlineage=" + lineage +
		"\nrevision=" + revision +
		"\ntarget_identity=" + targetIdentity +
		"\nlens=" + lens +
		"\norder=" + strconv.Itoa(order) +
		"\nartifact_digest=" + artifactDigest +
		"\nclass=" + string(class) +
		"\nactor=" + strings.TrimSpace(actor) +
		"\nreason=" + strings.TrimSpace(reason)
}

// CompactResultDispositionAuthorization renders the exact LF-only maintainer
// authorization binding one reviewer-result disposition requires. Callers
// derive it from the same values they pass to the operation; any divergence is
// refused.
func CompactResultDispositionAuthorization(repository, lineage, revision, targetIdentity, lens string, order int, artifactDigest string, class ResultDispositionClass, actor, reason string) string {
	return compactResultDispositionAuthorizationBinding(repository, lineage, revision, targetIdentity, lens, order, artifactDigest, class, actor, reason)
}

// CompactPreservedResultPath locates the durable incident artifact one
// preserved reviewer result was written to. The name is fully derived from the
// selected order, lens, and preserved digest, so no caller supplies a path.
func CompactPreservedResultPath(ctx context.Context, repo, lineageID, lens string, order int, artifactDigest string) (string, error) {
	incidents, err := CompactIncidentsDir(ctx, repo, lineageID)
	if err != nil {
		return "", err
	}
	if !validSHA256(artifactDigest) || order < 0 {
		return "", errors.New("preserved reviewer result requires a lowercase sha256 digest and a non-negative selected order")
	}
	return filepath.Join(incidents, fmt.Sprintf("%02d-%s-%s.raw", order, lens, strings.TrimPrefix(artifactDigest, "sha256:")[:12])), nil
}

func compactPreservedPayloadDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// DisposeUnreplayablePreservedResult terminally escalates one reviewing
// compact-v2 lineage whose preserved reviewer result cannot be admitted,
// because the raw payload is not syntactically decodable at all or because it
// proves the reviewer inspected a different candidate. The preserved bytes are
// read and digest-verified but never rewritten or deleted; every already
// captured reviewer result artifact and every frozen review field is retained
// untouched; and no path exists through this operation by which the refused
// payload becomes an admitted lens result. The operation binds the exact
// repository, lineage, target identity, lens, selected order, current
// authority revision, and preserved-artifact digest, requires natively
// re-derived candidate-inapplicability evidence plus an exact maintainer
// authorization binding, and compare-and-swaps on the authority revision. An
// exact replay of a committed disposition converges on the committed record.
// The supported forward path afterwards is the ordinary escalated recovery.
func DisposeUnreplayablePreservedResult(ctx context.Context, repo string, request CompactResultDispositionRequest) (CompactResultDispositionRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactResultDispositionRecord{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return CompactResultDispositionRecord{}, err
	}
	switch request.Lens {
	case LensRisk, LensResilience, LensReadability, LensReliability:
	default:
		return CompactResultDispositionRecord{}, fmt.Errorf("review dispose-result requires one exact canonical lens; got %q", request.Lens)
	}
	switch request.Class {
	case ResultDispositionTransportSyntax, ResultDispositionWrongTarget:
	default:
		return CompactResultDispositionRecord{}, fmt.Errorf("review dispose-result requires class %q or %q; got %q",
			ResultDispositionTransportSyntax, ResultDispositionWrongTarget, request.Class)
	}
	if !validSHA256(request.ExpectedRevision) {
		return CompactResultDispositionRecord{}, errors.New("review dispose-result requires the exact current authority revision")
	}
	if !validSHA256(request.TargetIdentity) || !validSHA256(request.ArtifactDigest) || request.SelectedOrder < 0 {
		return CompactResultDispositionRecord{}, errors.New("review dispose-result requires the exact frozen target identity, preserved artifact digest, and selected lens order")
	}
	if strings.TrimSpace(request.Diagnostic) == "" || strings.TrimSpace(request.Reason) == "" ||
		strings.TrimSpace(request.Actor) == "" || strings.TrimSpace(request.MaintainerAuthorization) == "" {
		return CompactResultDispositionRecord{}, errors.New("review dispose-result requires a non-empty diagnostic, reason, actor, and maintainer authorization")
	}
	_, repository, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactResultDispositionRecord{}, err
	}
	store, err := CompactAuthoritativeStore(ctx, repository, request.LineageID)
	if err != nil {
		return CompactResultDispositionRecord{}, err
	}
	record, err := store.Load()
	if err != nil {
		return CompactResultDispositionRecord{}, fmt.Errorf("load result disposition target: %w", err)
	}
	// Bind lens, order, and target identity before anything else observes the
	// authority: a wrong lens, a wrong order, or a changed target is refused
	// identically on a first attempt and on a replay.
	if request.TargetIdentity != record.State.InitialSnapshot.Identity {
		return CompactResultDispositionRecord{}, fmt.Errorf("review dispose-result refused: bound target identity %q is not the frozen target %q of lineage %q",
			request.TargetIdentity, record.State.InitialSnapshot.Identity, request.LineageID)
	}
	if request.SelectedOrder >= len(record.State.SelectedLenses) || record.State.SelectedLenses[request.SelectedOrder] != request.Lens {
		return CompactResultDispositionRecord{}, fmt.Errorf("review dispose-result refused: lens %q is not the selected lens at order %d of lineage %q",
			request.Lens, request.SelectedOrder, request.LineageID)
	}
	disposedResult := fmt.Sprintf("%02d-%s.json", request.SelectedOrder, request.Lens)
	captured, err := compactCapturedReviewerResults(store.Dir)
	if err != nil {
		return CompactResultDispositionRecord{}, err
	}
	// This is only the early, cheap refusal. It is deliberately not the
	// authority: capture-result publishes without bumping the compact revision,
	// so the same scan is repeated under the commit lock below.
	if err := compactRefuseCapturedLens(captured, disposedResult, request); err != nil {
		return CompactResultDispositionRecord{}, err
	}
	path, err := CompactPreservedResultPath(ctx, repository, request.LineageID, request.Lens, request.SelectedOrder, request.ArtifactDigest)
	if err != nil {
		return CompactResultDispositionRecord{}, err
	}
	// The preserved payload is only ever read. Its bytes stay exactly as
	// preserve-result wrote them, both on refusal and on a committed
	// disposition, so the incident artifact remains the untouched evidence.
	payload, err := os.ReadFile(path)
	if err != nil {
		return CompactResultDispositionRecord{}, fmt.Errorf("review dispose-result requires the preserved incident artifact for lens %q at order %d: %w", request.Lens, request.SelectedOrder, err)
	}
	if digest := compactPreservedPayloadDigest(payload); digest != request.ArtifactDigest {
		return CompactResultDispositionRecord{}, fmt.Errorf("review dispose-result refused: preserved artifact %s hashes to %s, not the bound digest %s", path, digest, request.ArtifactDigest)
	}
	absent, err := compactValidateResultInapplicability(record.State, request, payload)
	if err != nil {
		return CompactResultDispositionRecord{}, err
	}
	if request.MaintainerAuthorization != compactResultDispositionAuthorizationBinding(
		repository, request.LineageID, request.ExpectedRevision, request.TargetIdentity,
		request.Lens, request.SelectedOrder, request.ArtifactDigest, request.Class, request.Actor, request.Reason,
	) {
		return CompactResultDispositionRecord{}, fmt.Errorf("review dispose-result requires an exact maintainer authorization binding (schema %s over repository, lineage, revision, target_identity, lens, order, artifact_digest, class, actor, and reason)",
			compactResultDispositionAuthorizationSchema)
	}
	disposition := CompactResultDisposition{
		Lens: request.Lens, SelectedOrder: request.SelectedOrder, TargetIdentity: request.TargetIdentity,
		ArtifactDigest: request.ArtifactDigest, Class: request.Class,
		// Recorded from the preserved bytes themselves, never from the request,
		// so the persisted audit claim is the one that was actually proven.
		PayloadDecodable: json.Valid(payload),
		Diagnostic:       strings.TrimSpace(request.Diagnostic), AbsentPaths: absent,
		Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		MaintainerAuthorization: strings.TrimSpace(request.MaintainerAuthorization),
	}
	if record.State.State == StateEscalated {
		return replayCommittedResultDisposition(record, path, captured, disposition)
	}
	if record.State.State != StateReviewing {
		return CompactResultDispositionRecord{}, fmt.Errorf("review dispose-result refused: lineage %q holds %q authority; only a reviewing lineage may dispose a preserved reviewer result", request.LineageID, record.State.State)
	}
	if record.Revision != request.ExpectedRevision {
		return CompactResultDispositionRecord{}, fmt.Errorf("%w: expected compact revision %q, current %q", ErrConcurrentUpdate, request.ExpectedRevision, record.Revision)
	}
	if request.DisposedAt.IsZero() {
		request.DisposedAt = time.Now().UTC()
	}
	disposition.DisposedAt = request.DisposedAt.UTC()
	next := record.State
	next.State = StateEscalated
	next.ResultDispositions = append(append([]CompactResultDisposition{}, record.State.ResultDispositions...), disposition)
	// The already-captured refusal has to be atomic with the commit. A
	// capture-result publishes its artifact while holding the store lock and
	// never mutates the compact authority state, so it does not move the
	// revision the CAS compares: a capture landing between the early scan above
	// and this commit would otherwise be invisible, and the lineage would be
	// terminally escalated with a disposition for a lens that now holds a valid
	// captured result, orphaning that work. Re-scanning inside the locked commit
	// path closes the window, and the retained listing is recomputed from the
	// same authoritative snapshot so the audit record cannot under-report.
	retained := captured
	revision, err := store.replaceContextGuarded(ctx, request.ExpectedRevision, CompactResultDispositionOperation, next, func() error {
		locked, err := compactCapturedReviewerResults(store.Dir)
		if err != nil {
			return err
		}
		if err := compactRefuseCapturedLens(locked, disposedResult, request); err != nil {
			return err
		}
		retained = locked
		return nil
	})
	if err != nil {
		return CompactResultDispositionRecord{}, err
	}
	return CompactResultDispositionRecord{
		LineageID: request.LineageID, PreviousRevision: request.ExpectedRevision, Revision: revision,
		State: StateEscalated, ArtifactPath: path, Disposition: disposition, RetainedLensResults: retained,
	}, nil
}

// compactRefuseCapturedLens refuses a disposition of a lens and order that
// already holds a captured reviewer result. A valid capture is never
// dispositioned away, so the same check runs both as an early refusal and,
// authoritatively, inside the locked commit path.
func compactRefuseCapturedLens(captured []string, disposedResult string, request CompactResultDispositionRequest) error {
	for _, name := range captured {
		if name == disposedResult {
			return fmt.Errorf("review dispose-result refused: lens %q at order %d already holds a captured reviewer result; a valid capture is never dispositioned",
				request.Lens, request.SelectedOrder)
		}
	}
	return nil
}

// replayCommittedResultDisposition converges an exact replay of a committed
// disposition on the committed record instead of applying it twice or failing.
// The stored authorization binding embeds the predecessor revision, so an
// identical binding proves the replay targets the same committed transition.
func replayCommittedResultDisposition(record CompactRecord, path string, captured []string, request CompactResultDisposition) (CompactResultDispositionRecord, error) {
	for _, committed := range record.State.ResultDispositions {
		expected := committed
		expected.DisposedAt = time.Time{}
		if expected.Lens != request.Lens || expected.SelectedOrder != request.SelectedOrder ||
			expected.TargetIdentity != request.TargetIdentity || expected.ArtifactDigest != request.ArtifactDigest ||
			expected.Class != request.Class || expected.PayloadDecodable != request.PayloadDecodable ||
			expected.Diagnostic != request.Diagnostic ||
			expected.Reason != request.Reason || expected.Actor != request.Actor ||
			expected.MaintainerAuthorization != request.MaintainerAuthorization ||
			!equalStrings(expected.AbsentPaths, request.AbsentPaths) {
			continue
		}
		return CompactResultDispositionRecord{
			LineageID: record.State.LineageID, PreviousRevision: record.Revision, Revision: record.Revision,
			State: record.State.State, ArtifactPath: path, Disposition: committed,
			RetainedLensResults: captured, Replayed: true,
		}, nil
	}
	return CompactResultDispositionRecord{}, fmt.Errorf("review dispose-result refused: lineage %q is already terminally escalated and holds no committed disposition matching this request", record.State.LineageID)
}

// compactValidateResultInapplicability re-derives candidate inapplicability
// from the preserved bytes and the frozen target alone; the caller's claim is
// never trusted on its own. The two classes are mutually exclusive on the exact
// same evidence: a transport/syntax disposition must name a payload that is
// genuinely not decodable JSON, so an ordinary valid capture can never be
// routed here, and a wrong-target disposition must name a payload that did
// decode and must cite paths that the payload actually references and that the
// frozen candidate does not contain. Without the decodability check on both
// sides, a payload that never decoded could be permanently audited as the
// stronger semantic claim it does not support.
func compactValidateResultInapplicability(state CompactState, request CompactResultDispositionRequest, payload []byte) ([]string, error) {
	switch request.Class {
	case ResultDispositionTransportSyntax:
		if len(request.AbsentPaths) != 0 {
			return nil, fmt.Errorf("review dispose-result refused: class %q carries no wrong-target path evidence", ResultDispositionTransportSyntax)
		}
		if json.Valid(payload) {
			return nil, fmt.Errorf("review dispose-result refused: the preserved payload is syntactically valid JSON, so it is not a transport or syntax failure; replay it through capture-result, or disposition it as %q with absent-path evidence", ResultDispositionWrongTarget)
		}
		return nil, nil
	case ResultDispositionWrongTarget:
		absent, err := canonicalPaths(request.AbsentPaths)
		if err != nil || len(absent) == 0 {
			return nil, fmt.Errorf("review dispose-result refused: class %q requires at least one canonical repository-relative absent path", ResultDispositionWrongTarget)
		}
		if !json.Valid(payload) {
			return nil, fmt.Errorf("review dispose-result refused: class %q claims a decodable payload described a different candidate, but the preserved payload never decoded as JSON; disposition it as %q instead", ResultDispositionWrongTarget, ResultDispositionTransportSyntax)
		}
		for _, path := range absent {
			for _, candidate := range state.InitialSnapshot.Paths {
				if candidate == path {
					return nil, fmt.Errorf("review dispose-result refused: cited path %q is inside the frozen candidate, so it is not wrong-target evidence", path)
				}
			}
			if !bytes.Contains(payload, []byte(path)) {
				return nil, fmt.Errorf("review dispose-result refused: cited path %q does not appear in the preserved reviewer output", path)
			}
		}
		return absent, nil
	}
	return nil, fmt.Errorf("review dispose-result requires class %q or %q; got %q",
		ResultDispositionTransportSyntax, ResultDispositionWrongTarget, request.Class)
}

// compactCapturedReviewerResults lists the reviewer result artifacts one
// lineage already captured. The disposition retains every one of them; the
// listing is recorded in the audit output so the retention is auditable
// without reopening the store.
func compactCapturedReviewerResults(storeDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(storeDir, CompactReviewerResultsDir))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("inspect captured reviewer results: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
