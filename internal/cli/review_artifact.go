package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	reviewResultArtifactSchema     = "gentle-ai.review-result-artifact/v2"
	reviewResultArtifactCapability = "review.native_result_artifact"
	reviewAdmittedResultSchema     = reviewtransaction.AdmittedReviewerResultSchema
	reviewResultReferencePrefix    = "rart1_"
	reviewResultArtifactLimit      = 4 << 20
)

// reviewerResultSlotConflictError is the deliberately unwrapped identity for
// the byte-literal-conflict guard inside captureReviewerArtifact: the exact
// same SHA-256 already read back matches artifact.SHA256 yet its bytes
// differ (a hash-collision-only branch in practice, since
// readVerifiedReviewerArtifact already rejects a mismatched SHA-256 before
// this point is ever reached). Keeping it as a named sentinel rather than an
// inline errors.New lets the discoverability guidance below wrap it with
// %w instead of duplicating the base message.
var reviewerResultSlotConflictError = errors.New("captured reviewer result already exists with different canonical bytes")

// errCapturedFinalEvidenceMissing has the historical explicit-selector error
// text, but a distinct identity so lineage-only discovery can distinguish an
// absent capture from an unsafe or invalid persisted capture.
var errCapturedFinalEvidenceMissing = errors.New("captured final evidence is unavailable or unsafe")

func RunReviewCaptureEvidence(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review capture-evidence", stdout, "Capture outcome-bearing verification evidence bound to one compact authority and candidate.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "exact review lineage identifier")
	target := flags.String("target", "", "exact frozen target identity")
	revision := flags.String("expected-revision", "", "exact validating or correction authority revision")
	outcome := flags.String("outcome", "", "closed verification outcome: passed, verification_failed, or procedural_tooling_failed")
	input := flags.String("input", "", "final verification evidence file or - for stdin")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*target) == "" || strings.TrimSpace(*revision) == "" || strings.TrimSpace(*outcome) == "" || strings.TrimSpace(*input) == "" {
		return reviewPreflightError(errors.New("review capture-evidence requires exact --cwd, --lineage, --target, --expected-revision, --outcome, and --input")) // refusal:by-design operator-knowledge: the current STATUS transition supplies the authority-bound values and the verifier supplies the outcome and input
	}
	ctx := context.Background()
	root, err := resolveReviewMutationRoot(ctx, *cwd)
	if err != nil {
		return err
	}
	store, record, err := discoverCompactFacadeReview(ctx, root, *lineage, false)
	if err != nil {
		return reviewPreflightError(err)
	}
	state := record.State
	if record.Revision != *revision {
		return reviewPreflightError(errors.New("verification evidence binding does not match the current authority revision")) // refusal:by-design operator-knowledge: only a fresh STATUS transition can identify the current immutable revision
	}
	evidenceTarget, err := facadeVerificationEvidenceTarget(ctx, root, state, record.Revision)
	if err != nil || evidenceTarget.Identity != *target {
		return reviewPreflightError(errors.New("verification evidence binding does not match the current validating or correction authority")) // refusal:by-design operator-knowledge: the evidence producer must use the exact target emitted by the current STATUS transition
	}
	payload, err := readFacadeBytes(*input)
	if err != nil || len(payload) == 0 || len(payload) > reviewResultArtifactLimit {
		return reviewPreflightError(errors.New("final verification evidence is required"))
	}
	captured, err := reviewtransaction.PublishCapturedVerificationEvidence(reviewtransaction.CaptureVerificationEvidenceRequest{
		StoreDir: store.Dir, LineageID: state.LineageID, AuthorityRevision: record.Revision,
		Target: evidenceTarget, Payload: payload, Outcome: reviewtransaction.VerificationOutcome(*outcome),
	})
	if err != nil {
		if errors.Is(err, reviewtransaction.ErrCapturedVerificationEvidenceConflict) {
			baseMessage := "captured verification evidence already exists with different bytes or outcome"
			clause := reviewGenerateToolFaultDefectReport(ctx, root, reviewDefectReportInput{
				Operation:            "review capture-evidence --lineage --target --expected-revision --outcome --input",
				ReasonCode:           "captured_final_evidence_conflict",
				ErrorMessage:         baseMessage,
				TerminalPrecondition: "verification evidence was already captured for this authority and candidate with different immutable content",
				StateIdentifiers:     map[string]string{"state": string(state.State), "target": *target, "revision": *revision},
			})
			return reviewPreflightError(fmt.Errorf("%s.%s", baseMessage, clause))
		}
		return reviewPreflightError(err)
	}
	return encodeReviewJSON(stdout, captured.Record)
}

func facadeVerificationEvidenceTarget(ctx context.Context, repo string, state reviewtransaction.CompactState, revision string) (reviewtransaction.Snapshot, error) {
	switch state.State {
	case reviewtransaction.StateValidating:
		return state.CurrentSnapshot, nil
	case reviewtransaction.StateCorrectionRequired:
		if state.ProposedCorrectionLines == nil {
			return reviewtransaction.Snapshot{}, errors.New("verification evidence requires a forecasted correction") // refusal:by-design operator-knowledge: correction planning is an external prerequisite whose exact line forecast must be finalized first
		}
		if err := rejectFacadeCorrectionUntracked(ctx, repo, state); err != nil {
			return reviewtransaction.Snapshot{}, err
		}
		projection := state.InitialSnapshot.Projection
		if projection == "" {
			projection = reviewtransaction.ProjectionWorkspace
		}
		fix, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(ctx, reviewtransaction.Target{
			Kind: reviewtransaction.TargetFixDiff, Projection: projection, BaseRef: state.CurrentSnapshot.CandidateTree,
			IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs,
		})
		if err != nil {
			return reviewtransaction.Snapshot{}, err
		}
		request, err := reviewtransaction.BuildTargetedValidationRequest(ctx, repo, state, revision)
		if err != nil || request.CorrectionTargetIdentity != fix.Identity || request.CorrectionCandidateTree != fix.CandidateTree ||
			request.CorrectionPathsDigest != fix.PathsDigest {
			return reviewtransaction.Snapshot{}, errors.New("correction candidate changed while binding verification evidence") // refusal:by-design world-action: concurrent candidate mutation invalidated the snapshot and a stable candidate is required before capture
		}
		return fix, nil
	default:
		return reviewtransaction.Snapshot{}, fmt.Errorf("cannot capture verification evidence from compact state %q", state.State) // refusal:by-design world-action: this persisted lifecycle state has no verification-capture transition
	}
}

func readCapturedFinalEvidence(storeDir string, state reviewtransaction.CompactState, revision string) (reviewtransaction.CapturedVerificationEvidence, error) {
	captured, err := reviewtransaction.ReadCapturedVerificationEvidence(storeDir, state.LineageID, revision, state.CurrentSnapshot)
	if errors.Is(err, reviewtransaction.ErrCapturedVerificationEvidenceMissing) {
		return reviewtransaction.CapturedVerificationEvidence{}, errCapturedFinalEvidenceMissing
	}
	return captured, err
}

type reviewResultArtifact struct {
	Schema            string                                      `json:"schema"`
	Capability        string                                      `json:"capability"`
	Path              string                                      `json:"path,omitempty"`
	Reference         string                                      `json:"reference,omitempty"`
	SHA256            string                                      `json:"sha256"`
	LineageID         string                                      `json:"lineage_id"`
	TargetIdentity    string                                      `json:"target_identity"`
	Lens              string                                      `json:"lens"`
	SelectedOrder     int                                         `json:"selected_order"`
	SubjectHash       string                                      `json:"subject_hash"`
	AdmissionDecision reviewtransaction.ArtifactAdmissionDecision `json:"admission_decision"`
}

// admittedReviewerResult is the durable provider-owned envelope. Historical
// v1 files contained only model JSON; those bytes intentionally fail closed
// because they carry neither a subject nor an admission decision.
type admittedReviewerResult struct {
	Schema    string                              `json:"schema"`
	Subject   reviewtransaction.ArtifactSubject   `json:"subject"`
	Admission reviewtransaction.ArtifactAdmission `json:"admission"`
	Result    facadeReviewerResult                `json:"result"`
}

type capturedArtifactBinding struct {
	Subject   reviewtransaction.ArtifactSubject
	Admission reviewtransaction.ArtifactAdmission
}

// ReviewerResultPayloadError is returned when a raw reviewer result payload is
// structurally invalid before JSON decoding is attempted. Code is
// machine-readable: "empty_result" for empty/whitespace-only payloads and
// "nested_envelope" for payloads that still contain an XML task envelope.
type ReviewerResultPayloadError struct {
	Code    string
	Message string
}

func (e *ReviewerResultPayloadError) Error() string { return e.Message }

// validateReviewerResultPayload inspects the raw bytes of a reviewer result
// before JSON decoding. It rejects two structurally distinct failure modes
// that require separate diagnostics:
//  1. empty_result: the task completed but produced no reviewer output.
//  2. nested_envelope: the reviewer output was not extracted from its XML
//     task wrapper before being passed as the strict JSON payload.
func validateReviewerResultPayload(payload []byte) error {
	if len(bytes.TrimSpace(payload)) == 0 {
		return &ReviewerResultPayloadError{
			Code:    "empty_result",
			Message: "reviewer result payload is empty or whitespace-only: the task may have completed without producing output",
		}
	}
	if bytes.Contains(payload, []byte("<task_result>")) || bytes.Contains(payload, []byte("</task_result>")) {
		if !json.Valid(payload) {
			return &ReviewerResultPayloadError{
				Code:    "nested_envelope",
				Message: "reviewer result payload contains a raw XML task envelope: extract the strict JSON reviewer output from <task_result> before capture",
			}
		}
	}
	return nil
}

var reviewArtifactAfterLstat = func() {}
var reviewArtifactRuntimeGOOS = func() string { return runtime.GOOS }
var syncReviewerArtifactDirectory = func(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}

func RunReviewCaptureResult(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review capture-result", stdout, "Capture one strict reviewer result in native authority and emit its bound manifest.")
	cwd := flags.String("cwd", ".", "repository path")
	repositoryContext := flags.String("repository-context", "", "opaque provider-issued repository context; supplied by the collect transition and mutually exclusive with --cwd, pass one or the other and not both")
	lineage := flags.String("lineage", "", "exact review lineage identifier")
	target := flags.String("target", "", "exact frozen target identity")
	lens := flags.String("lens", "", "exact selected lens")
	order := flags.Int("order", -1, "zero-based selected lens order")
	revision := flags.String("expected-revision", "", "exact reviewing authority revision")
	subjectHash := flags.String("subject-hash", "", "provider-issued artifact subject hash for native-Git context")
	input := flags.String("input", "", "raw reviewer result JSON file or - for stdin; `gentle-ai review schema reviewer` emits the schema and a working example")
	preflight := flags.Bool("preflight", false, "verify the capture binding against the current reviewing authority without reading or persisting any result")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*target) == "" ||
		strings.TrimSpace(*lens) == "" || *order < 0 || (!*preflight && strings.TrimSpace(*input) == "") {
		return reviewPreflightError(errors.New("review capture-result requires an exact repository context, --lineage, --target, --lens, --order, and --input (or --preflight); `gentle-ai review status --contract gentle-ai.review-integration/v1 --next-transition` prints the exact bindings and `gentle-ai review schema reviewer` emits the result schema with a working example"))
	}
	if *preflight && strings.TrimSpace(*input) != "" {
		return reviewPreflightError(errors.New("review capture-result --preflight verifies the binding only and does not accept --input"))
	}
	contextHandle := strings.TrimSpace(*repositoryContext)
	if contextHandle != "" && reviewFlagWasProvided(flags, "cwd") {
		return reviewPreflightError(errors.New("review capture-result accepts either --repository-context or --cwd, not both"))
	}
	if contextHandle != "" && strings.TrimSpace(*revision) == "" {
		return reviewPreflightError(errors.New("review capture-result with --repository-context requires --expected-revision"))
	}
	ctx := context.Background()
	var root string
	var err error
	if contextHandle != "" {
		root, err = resolveOpaqueReviewRepositoryRoot(ctx, contextHandle, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: *lineage, TargetIdentity: *target, Revision: *revision,
		})
		if err != nil {
			return err
		}
	} else {
		root, err = (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
		if err != nil {
			return fmt.Errorf("resolve review repository root: %w", err)
		}
	}
	// --preflight verifies the capture binding without reading or persisting
	// any result, so it stays reachable under a frozen switch. Everything else
	// here publishes a reviewer result into the store.
	if !*preflight {
		if err := authorizeReviewAuthorityMutation(ctx, root); err != nil {
			return err
		}
	}
	store, record, err := discoverCompactFacadeReview(ctx, root, *lineage, false)
	repositoryDescription := fmt.Sprintf("repository %q", root)
	if contextHandle != "" {
		repositoryDescription = "the provider-issued repository context"
	}
	if err != nil {
		if contextHandle != "" {
			return reviewOpaqueContextFailure("repository_context_authority_unavailable", "refresh the exact native next_transition before retrying")
		}
		return reviewPreflightError(fmt.Errorf("resolve reviewing authority for lineage %q under %s: %w; if the review was started in a different repository (for example a nested one), re-run with --cwd set to that repository", *lineage, repositoryDescription, err))
	}
	state := record.State
	if state.State != reviewtransaction.StateReviewing || state.LineageID != *lineage || state.InitialSnapshot.Identity != *target ||
		(strings.TrimSpace(*revision) != "" && record.Revision != *revision) || *order >= len(state.SelectedLenses) || state.SelectedLenses[*order] != *lens {
		if contextHandle != "" {
			return reviewPreflightError(fmt.Errorf("capture binding does not match the current reviewing authority under the provider-issued repository context; ask the parent orchestrator to refresh the exact native next transition by running %s", reviewNextTransitionRefreshCommandV2))
		}
		return reviewPreflightError(fmt.Errorf("capture binding does not match the current reviewing authority under repository %q; verify the frozen lineage, target, lens, and order for that repository, or re-run with --cwd set to the repository where the review was started", root))
	}
	frozen, err := (reviewtransaction.SnapshotBuilder{Repo: root}).FrozenCandidateContext(ctx, state.InitialSnapshot)
	if err != nil {
		return reviewPreflightError(fmt.Errorf("derive reviewer artifact subject: %w", err))
	}
	subject, err := reviewtransaction.NewArtifactSubject(state, record.Revision, frozen, *lens, *order, "")
	if err != nil {
		return reviewPreflightError(fmt.Errorf("derive reviewer artifact subject: %w", err))
	}
	if *preflight {
		if *subjectHash != "" && *subjectHash != subject.SubjectHash {
			legacyFrozen, legacyErr := (reviewtransaction.SnapshotBuilder{Repo: root}).WithLegacyCandidateDiff(ctx, state.InitialSnapshot, frozen)
			if legacyErr != nil {
				return reviewPreflightError(errors.New("review capture preflight subject hash does not match the provider-owned authority; refresh the binding with gentle-ai review status --cwd <repo> --contract <same-contract> --next-transition"))
			}
			legacySubject, legacyErr := reviewtransaction.NewLegacyArtifactSubject(state, record.Revision, legacyFrozen, *lens, *order, "")
			if legacyErr != nil || *subjectHash != legacySubject.SubjectHash {
				return reviewPreflightError(errors.New("review capture preflight subject hash does not match the provider-owned authority; refresh the binding with gentle-ai review status --cwd <repo> --contract <same-contract> --next-transition"))
			}
			frozen, subject = legacyFrozen, legacySubject
		}
		publicRoot := root
		if contextHandle != "" {
			publicRoot = ""
		}
		return encodeReviewJSON(stdout, reviewCapturePreflightResult{
			Schema: reviewCapturePreflightSchema, Capability: reviewCapturePreflightCapability, RepositoryRoot: publicRoot,
			LineageID: state.LineageID, TargetIdentity: state.InitialSnapshot.Identity, Lens: *lens, SelectedOrder: *order,
			ArtifactSubject: subject, BaseTree: frozen.BaseTree, CandidateTree: frozen.CandidateTree,
			ChangedPathManifest: append([]reviewtransaction.ChangedPathManifestEntry{}, frozen.ChangedPathManifest...),
		})
	}
	rawPayload, err := readFacadeBytes(*input)
	if err != nil {
		return reviewPreflightError(fmt.Errorf("read reviewer result: %w", err))
	}
	if err := validateReviewerResultPayload(rawPayload); err != nil {
		return reviewPreflightError(err)
	}
	payload, decision, err := reviewtransaction.ExtractBoundedSingleJSONObject(rawPayload, reviewResultArtifactLimit)
	if err != nil {
		return reviewPreflightError(fmt.Errorf("reviewer artifact admission %s: %w", decision, err))
	}
	var result facadeReviewerResult
	if err := decodeFacadeJSONBytes(payload, &result); err != nil {
		return reviewPreflightError(fmt.Errorf("decode reviewer result: %w", err))
	}
	if result.Findings == nil || result.Evidence == nil {
		return reviewPreflightError(errors.New("reviewer result requires explicit findings and evidence arrays"))
	}
	if result.SubjectHash != subject.SubjectHash {
		legacyFrozen, legacyErr := (reviewtransaction.SnapshotBuilder{Repo: root}).WithLegacyCandidateDiff(ctx, state.InitialSnapshot, frozen)
		if legacyErr == nil {
			legacySubject, subjectErr := reviewtransaction.NewLegacyArtifactSubject(state, record.Revision, legacyFrozen, *lens, *order, "")
			if subjectErr == nil && result.SubjectHash == legacySubject.SubjectHash {
				frozen, subject = legacyFrozen, legacySubject
			}
		}
	}
	if _, err := prepareCompactReviewerResults(reviewtransaction.CompactState{SelectedLenses: []string{*lens}}, []facadeReviewerResult{result}, facadeRefuterResult{}); err != nil {
		return reviewPreflightError(err)
	}
	canonicalResult, err := json.Marshal(result)
	if err != nil {
		return err
	}
	canonicalResult = append(canonicalResult, '\n')
	nativeResult := result.nativeLensResult()
	nativeResult.Lens = *lens
	// Derive verified candidate-causal IDs from the CANONICALIZED result, not
	// the raw one: CanonicalCompactLensResult assigns a fallback ID
	// (`<prefix>-NNN`) to any severe finding submitted without an explicit
	// "id", and AdmitArtifact below canonicalizes independently before
	// comparing against this set. Verifying against the raw result left a
	// severe candidate-causal finding with no "id" permanently unmatchable
	// against admission's canonicalized fallback ID (community report,
	// issue-1699, confirmed unresolved by the earlier Group A fix).
	canonicalForCausality, err := reviewtransaction.CanonicalCompactLensResult(nativeResult)
	if err != nil {
		return reviewPreflightError(fmt.Errorf("canonicalize reviewer result: %w", err))
	}
	candidateCausalIDs, err := verifiedCandidateCausalFindingIDs(ctx, root, state.InitialSnapshot, canonicalForCausality)
	if err != nil {
		return reviewPreflightError(err)
	}
	_, admission, err := reviewtransaction.AdmitArtifact(reviewtransaction.ArtifactAdmissionRequest{
		ExpectedSubject: subject, FrozenContext: frozen, EchoedSubjectHash: result.SubjectHash,
		Inspection: result.Inspection, Result: nativeResult, CandidateCausalFindingIDs: candidateCausalIDs,
		RawPayload: rawPayload, CanonicalPayload: canonicalResult,
	})
	if err != nil {
		return reviewPreflightError(err)
	}
	envelopeSchema := reviewAdmittedResultSchema
	if subject.Schema == reviewtransaction.ArtifactSubjectSchemaV1 {
		envelopeSchema = reviewtransaction.AdmittedReviewerResultSchemaV1
	}
	envelope, err := json.Marshal(admittedReviewerResult{
		Schema: envelopeSchema, Subject: subject, Admission: admission, Result: result,
	})
	if err != nil {
		return err
	}
	envelope = append(envelope, '\n')
	var artifact reviewResultArtifact
	err = store.CaptureReviewerResult(record.Revision, *target, *lens, *order, func(current reviewtransaction.CompactState) error {
		var captureErr error
		artifact, captureErr = captureReviewerArtifact(store.Dir, current, *order, envelope, capturedArtifactBinding{Subject: subject, Admission: admission})
		return captureErr
	})
	if err != nil {
		if contextHandle != "" {
			return reviewOpaqueContextFailure("repository_context_capture_failed", "retry capture-result with the same exact binding or refresh status")
		}
		return reviewPreflightError(err)
	}
	if contextHandle != "" {
		artifact.Reference = reviewResultReference(artifact)
		artifact.Path = ""
	}
	return encodeReviewJSON(stdout, artifact)
}

func reviewResultReference(artifact reviewResultArtifact) string {
	preimage := struct {
		Schema, Capability, SHA256, LineageID, TargetIdentity, Lens, SubjectHash string
		SelectedOrder                                                            int
		AdmissionDecision                                                        reviewtransaction.ArtifactAdmissionDecision
	}{
		Schema: artifact.Schema, Capability: artifact.Capability, SHA256: artifact.SHA256,
		LineageID: artifact.LineageID, TargetIdentity: artifact.TargetIdentity,
		Lens: artifact.Lens, SelectedOrder: artifact.SelectedOrder, SubjectHash: artifact.SubjectHash,
		AdmissionDecision: artifact.AdmissionDecision,
	}
	payload, _ := json.Marshal(preimage)
	return reviewResultReferencePrefix + strings.TrimPrefix(facadePayloadHash(payload), "sha256:")
}
func captureReviewerArtifact(storeDir string, state reviewtransaction.CompactState, order int, payload []byte, bindings ...capturedArtifactBinding) (reviewResultArtifact, error) {
	dir := filepath.Join(storeDir, reviewtransaction.CompactReviewerResultsDir)
	if err := ensureReviewerArtifactDir(dir); err != nil {
		return reviewResultArtifact{}, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%02d-%s.json", order, state.SelectedLenses[order]))
	if err := archiveQuarantinedReviewerArtifact(storeDir, state, order, path); err != nil {
		return reviewResultArtifact{}, err
	}
	artifact := reviewResultArtifact{
		Schema: reviewResultArtifactSchema, Capability: reviewResultArtifactCapability, Path: path,
		SHA256: facadePayloadHash(payload), LineageID: state.LineageID,
		TargetIdentity: state.InitialSnapshot.Identity, Lens: state.SelectedLenses[order], SelectedOrder: order,
	}
	if len(bindings) > 0 {
		artifact.SubjectHash = bindings[0].Subject.SubjectHash
		artifact.AdmissionDecision = bindings[0].Admission.Decision
	}
	if existing, err := readVerifiedReviewerArtifact(artifact, storeDir, state); err == nil {
		if !bytes.Equal(existing, payload) {
			return reviewResultArtifact{}, fmt.Errorf("%w; a different reviewer result already occupies this slot — decide with `review dispose-result` (discard it) or `review preserve-result` (keep it and quarantine this new submission)", reviewerResultSlotConflictError)
		}
		return artifact, persistReviewerArtifactDigest(path, artifact.SHA256)
	} else if !os.IsNotExist(err) {
		return reviewResultArtifact{}, fmt.Errorf("%w; a different reviewer result already occupies this slot — decide with `review dispose-result` (discard it) or `review preserve-result` (keep it and quarantine this new submission)", err)
	}
	temp, err := os.CreateTemp(dir, ".capture-*")
	if err != nil {
		return reviewResultArtifact{}, fmt.Errorf("create reviewer result temporary file: %w", err)
	}
	owned, _ := temp.Stat()
	defer removeOwnedArtifact(temp.Name(), owned)
	if err := temp.Chmod(0o600); err != nil {
		return reviewResultArtifact{}, err
	}
	if _, err := temp.Write(payload); err != nil {
		return reviewResultArtifact{}, err
	}
	if err := temp.Sync(); err != nil {
		return reviewResultArtifact{}, err
	}
	if err := temp.Close(); err != nil {
		return reviewResultArtifact{}, err
	}
	if err := reviewtransaction.PublishFileNoReplace(temp.Name(), path); err != nil {
		if existing, readErr := readVerifiedReviewerArtifact(artifact, storeDir, state); readErr == nil && bytes.Equal(existing, payload) {
			return artifact, persistReviewerArtifactDigest(path, artifact.SHA256)
		}
		return reviewResultArtifact{}, fmt.Errorf("publish reviewer result atomically: %w", err)
	}
	if err := syncReviewerArtifactDirectoryCompatible(dir); err != nil {
		removeOwnedArtifact(path, owned)
		return reviewResultArtifact{}, fmt.Errorf("sync reviewer result directory: %w", err)
	}
	if _, err := readVerifiedReviewerArtifact(artifact, storeDir, state); err != nil {
		removeOwnedArtifact(path, owned)
		return reviewResultArtifact{}, fmt.Errorf("read back reviewer result: %w", err)
	}
	return artifact, persistReviewerArtifactDigest(path, artifact.SHA256)
}

// archiveQuarantinedReviewerArtifact removes only an artifact digest that the
// native reopen transition already classified and bound in authority. It
// publishes immutable archive copies before removing the canonical slot, so a
// crash at any point converges on retry without losing the rejected bytes.
func archiveQuarantinedReviewerArtifact(storeDir string, state reviewtransaction.CompactState, order int, path string) error {
	digestPath := path + ".sha256"
	payload, payloadInfo, payloadErr := readPrivateReviewerFile(path, reviewResultArtifactLimit)
	if payloadErr != nil && !os.IsNotExist(payloadErr) {
		return fmt.Errorf("read quarantined reviewer result: %w", payloadErr)
	}
	digestPayload, digestInfo, digestErr := readPrivateReviewerFile(digestPath, 256)
	if digestErr != nil && !os.IsNotExist(digestErr) {
		return fmt.Errorf("read quarantined reviewer result digest: %w", digestErr)
	}
	if os.IsNotExist(payloadErr) && os.IsNotExist(digestErr) {
		return nil
	}
	digest := strings.TrimSpace(string(digestPayload))
	if !os.IsNotExist(payloadErr) {
		actual := facadePayloadHash(payload)
		if digest != "" && digest != actual {
			return errors.New("quarantined reviewer result digest does not match its bytes")
		}
		digest = actual
	}
	if !validReviewCapabilitySHA256(digest) {
		return errors.New("quarantined reviewer result has no valid digest")
	}
	archivePath, quarantined := reviewtransaction.ReviewerResultQuarantinePath(storeDir, state, order, digest)
	if !quarantined {
		return nil
	}
	archiveRoot := filepath.Join(storeDir, reviewtransaction.CompactQuarantinedReviewerResultsDir)
	if err := ensureReviewerArtifactDir(archiveRoot); err != nil {
		return err
	}
	archiveDir := filepath.Dir(archivePath)
	if err := ensureReviewerArtifactDir(archiveDir); err != nil {
		return err
	}
	if os.IsNotExist(payloadErr) {
		archived, _, err := readPrivateReviewerFile(archivePath, reviewResultArtifactLimit)
		if err != nil || facadePayloadHash(archived) != digest {
			return errors.New("quarantined reviewer result archive is incomplete")
		}
	} else if err := publishImmutableReviewerFile(archivePath, payload); err != nil {
		return fmt.Errorf("archive quarantined reviewer result: %w", err)
	}
	canonicalDigest := []byte(digest + "\n")
	if err := publishImmutableReviewerFile(archivePath+".sha256", canonicalDigest); err != nil {
		return fmt.Errorf("archive quarantined reviewer result digest: %w", err)
	}
	if payloadInfo != nil {
		removeOwnedArtifact(path, payloadInfo)
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return errors.New("quarantined reviewer result changed before removal")
		}
		if err := syncReviewerArtifactDirectoryCompatible(filepath.Dir(path)); err != nil {
			return err
		}
	}
	if digestInfo != nil {
		removeOwnedArtifact(digestPath, digestInfo)
		if _, err := os.Lstat(digestPath); !os.IsNotExist(err) {
			return errors.New("quarantined reviewer result digest changed before removal")
		}
		if err := syncReviewerArtifactDirectoryCompatible(filepath.Dir(path)); err != nil {
			return err
		}
	}
	return nil
}

func readPrivateReviewerFile(path string, limit int64) ([]byte, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !reviewArtifactModeSafe(info.Mode(), false) {
		return nil, nil, errors.New("reviewer artifact is not an owner-only regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, nil, errors.New("reviewer artifact path changed before read")
	}
	payload, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(payload)) > limit {
		return nil, nil, errors.New("reviewer artifact exceeds its native size limit")
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, after) {
		return nil, nil, errors.New("reviewer artifact path changed during read")
	}
	return payload, after, nil
}

func publishImmutableReviewerFile(path string, payload []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".archive-*")
	if err != nil {
		return err
	}
	owned, _ := temp.Stat()
	defer removeOwnedArtifact(temp.Name(), owned)
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(payload); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := reviewtransaction.PublishFileNoReplace(temp.Name(), path); err != nil {
		existing, _, readErr := readPrivateReviewerFile(path, int64(len(payload)))
		if readErr != nil || !bytes.Equal(existing, payload) {
			return err
		}
	}
	return syncReviewerArtifactDirectoryCompatible(dir)
}

func persistReviewerArtifactDigest(path, digest string) error {
	digestPath := path + ".sha256"
	if existing, err := os.ReadFile(digestPath); err == nil {
		if strings.TrimSpace(string(existing)) == digest {
			return nil
		}
		return errors.New("captured reviewer result digest already exists with different bytes; decide with `review dispose-result` (discard it) or `review preserve-result` (keep it and quarantine this new submission)")
	} else if !os.IsNotExist(err) {
		return err
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".capture-digest-*")
	if err != nil {
		return err
	}
	defer temp.Close()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.WriteString(digest + "\n"); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := reviewtransaction.PublishFileNoReplace(temp.Name(), digestPath); err != nil {
		if existing, readErr := os.ReadFile(digestPath); readErr == nil && strings.TrimSpace(string(existing)) == digest {
			return nil
		}
		return err
	}
	return syncReviewerArtifactDirectoryCompatible(dir)
}

func syncReviewerArtifactDirectoryCompatible(dir string) error {
	err := syncReviewerArtifactDirectory(dir)
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, errors.ErrUnsupported) || reviewArtifactRuntimeGOOS() == "windows" && errors.Is(err, os.ErrPermission) {
		return nil
	}
	return err
}
func ensureReviewerArtifactDir(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create reviewer result directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !reviewArtifactModeSafe(info.Mode(), true) {
		return errors.New("reviewer result directory is not a private native directory")
	}
	return nil
}
func readFacadeReviewerArtifacts(ctx context.Context, repo string, raw []string, storeDir string, state reviewtransaction.CompactState, revision string) ([]facadeReviewerResult, error) {
	if len(raw) != len(state.SelectedLenses) {
		return nil, fmt.Errorf("review finalize requires all %d original reviewer artifact(s); capture each missing one with `%s` (see `%s` for the exact lineage/target/lens/order bindings)", len(state.SelectedLenses), reviewCaptureResultCommandName(), reviewNextTransitionRefreshCommand)
	}
	frozen, err := reviewerArtifactFrozenContext(ctx, repo, state)
	if err != nil {
		return nil, err
	}
	results := make([]facadeReviewerResult, len(raw))
	for index := range raw {
		var artifact reviewResultArtifact
		if err := decodeFacadeJSONBytes([]byte(raw[index]), &artifact); err != nil {
			return nil, fmt.Errorf("decode reviewer artifact %d: %w", index+1, err)
		}
		if artifact.SelectedOrder != index {
			return nil, fmt.Errorf("reviewer artifact %d is out of selected-lens order", index+1)
		}
		payload, err := readVerifiedReviewerArtifact(artifact, storeDir, state)
		if err != nil {
			return nil, fmt.Errorf("verify reviewer artifact %d: %w", index+1, err)
		}
		result, subject, err := decodeBoundAdmittedReviewerResult(ctx, repo, payload, artifact.SHA256, state, revision, index, frozen)
		if err != nil {
			return nil, fmt.Errorf("parse reviewer artifact %d: %w", index+1, err)
		}
		if artifact.SubjectHash != subject.SubjectHash || artifact.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted {
			return nil, fmt.Errorf("verify reviewer artifact %d: artifact manifest does not match the provider-owned subject", index+1)
		}
		results[index] = result
	}
	return results, nil
}

// discoverCapturedReviewerArtifacts reads only the canonical native capture
// locations. It makes status restart-safe without exposing provider paths or
// asking a consumer to reconstruct result manifests.
func discoverCapturedReviewerArtifacts(ctx context.Context, repo, storeDir string, state reviewtransaction.CompactState, revision string) ([]ReviewTransitionArtifact, error) {
	frozen, err := reviewerArtifactFrozenContext(ctx, repo, state)
	if err != nil {
		return nil, err
	}
	artifacts := make([]ReviewTransitionArtifact, 0, len(state.SelectedLenses))
	for order, lens := range state.SelectedLenses {
		path := filepath.Join(storeDir, reviewtransaction.CompactReviewerResultsDir, fmt.Sprintf("%02d-%s.json", order, lens))
		digest, err := os.ReadFile(path + ".sha256")
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read captured reviewer result digest %d: %w", order, err)
		}
		digestValue := strings.TrimSpace(string(digest))
		if reviewtransaction.ReviewerResultDigestIsQuarantined(state, order, digestValue) {
			continue
		}
		artifact := reviewResultArtifact{
			Schema: reviewResultArtifactSchema, Capability: reviewResultArtifactCapability, Path: path, SHA256: digestValue,
			LineageID: state.LineageID, TargetIdentity: state.InitialSnapshot.Identity, Lens: lens, SelectedOrder: order,
		}
		payload, err := readVerifiedReviewerArtifact(artifact, storeDir, state)
		if err != nil {
			return nil, fmt.Errorf("verify captured reviewer result %d: %w", order, err)
		}
		_, subject, err := decodeBoundAdmittedReviewerResult(ctx, repo, payload, artifact.SHA256, state, revision, order, frozen)
		if err != nil {
			return nil, fmt.Errorf("verify captured reviewer admission %d: %w", order, err)
		}
		artifact.SubjectHash = subject.SubjectHash
		artifact.AdmissionDecision = reviewtransaction.ArtifactAdmissionCompleted
		artifacts = append(artifacts, ReviewTransitionArtifact{
			Schema: artifact.Schema, Capability: artifact.Capability, SHA256: artifact.SHA256, LineageID: artifact.LineageID,
			TargetIdentity: artifact.TargetIdentity, Lens: artifact.Lens, SelectedOrder: artifact.SelectedOrder,
			SubjectHash: artifact.SubjectHash, AdmissionDecision: artifact.AdmissionDecision,
		})
	}
	return artifacts, nil
}

func readCapturedReviewerResults(ctx context.Context, repo, storeDir string, state reviewtransaction.CompactState, revision string) ([]facadeReviewerResult, error) {
	artifacts, err := discoverCapturedReviewerArtifacts(ctx, repo, storeDir, state, revision)
	if err != nil {
		return nil, err
	}
	if len(artifacts) != len(state.SelectedLenses) {
		return nil, fmt.Errorf("review finalize requires all %d captured reviewer result(s); capture each missing one with `%s` (see `%s` for the exact lineage/target/lens/order bindings)", len(state.SelectedLenses), reviewCaptureResultCommandName(), reviewNextTransitionRefreshCommand)
	}
	results := make([]facadeReviewerResult, len(artifacts))
	frozen, err := reviewerArtifactFrozenContext(ctx, repo, state)
	if err != nil {
		return nil, err
	}
	for index, published := range artifacts {
		artifact := reviewResultArtifact{
			Schema: published.Schema, Capability: published.Capability,
			Path:   filepath.Join(storeDir, reviewtransaction.CompactReviewerResultsDir, fmt.Sprintf("%02d-%s.json", published.SelectedOrder, published.Lens)),
			SHA256: published.SHA256, LineageID: published.LineageID, TargetIdentity: published.TargetIdentity,
			Lens: published.Lens, SelectedOrder: published.SelectedOrder, SubjectHash: published.SubjectHash,
			AdmissionDecision: published.AdmissionDecision,
		}
		payload, err := readVerifiedReviewerArtifact(artifact, storeDir, state)
		if err != nil {
			return nil, err
		}
		result, subject, err := decodeBoundAdmittedReviewerResult(ctx, repo, payload, artifact.SHA256, state, revision, index, frozen)
		if err != nil {
			return nil, err
		}
		if published.SubjectHash != subject.SubjectHash || published.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted {
			return nil, errors.New("captured reviewer artifact does not match its admitted subject")
		}
		results[index] = result
	}
	return results, nil
}

func reviewerArtifactFrozenContext(ctx context.Context, repo string, state reviewtransaction.CompactState) (reviewtransaction.FrozenCandidateContext, error) {
	frozen, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(ctx, state.InitialSnapshot)
	if err != nil {
		return reviewtransaction.FrozenCandidateContext{}, fmt.Errorf("derive frozen reviewer artifact context: %w", err)
	}
	return frozen, nil
}

func decodeBoundAdmittedReviewerResult(ctx context.Context, repo string, payload []byte, artifactDigest string, state reviewtransaction.CompactState, currentRevision string, order int, frozen reviewtransaction.FrozenCandidateContext) (facadeReviewerResult, reviewtransaction.ArtifactSubject, error) {
	var envelope admittedReviewerResult
	if err := decodeFacadeJSONBytes(payload, &envelope); err != nil {
		return facadeReviewerResult{}, reviewtransaction.ArtifactSubject{}, err
	}
	if order < 0 || order >= len(state.SelectedLenses) || reviewtransaction.ReviewerResultDigestIsQuarantined(state, order, artifactDigest) {
		return facadeReviewerResult{}, reviewtransaction.ArtifactSubject{}, errors.New("captured reviewer result does not bind the active authority revision")
	}
	if state.State == reviewtransaction.StateReviewing && envelope.Subject.AuthorityRevision != currentRevision &&
		!reviewtransaction.ReviewerResultAdmissionWasRetained(
			state, order, artifactDigest, envelope.Subject.SubjectHash, envelope.Subject.AuthorityRevision, envelope.Admission.ResultHash,
		) {
		return facadeReviewerResult{}, reviewtransaction.ArtifactSubject{}, errors.New("captured reviewer result does not bind the active authority revision")
	}
	subjectFrozen := frozen
	var expected reviewtransaction.ArtifactSubject
	var err error
	if envelope.Subject.Schema == reviewtransaction.ArtifactSubjectSchemaV1 {
		subjectFrozen, err = (reviewtransaction.SnapshotBuilder{Repo: repo}).WithLegacyCandidateDiff(ctx, state.InitialSnapshot, frozen)
		if err == nil {
			expected, err = reviewtransaction.NewLegacyArtifactSubject(
				state, envelope.Subject.AuthorityRevision, subjectFrozen, state.SelectedLenses[order], order, envelope.Subject.CorrectionTargetIdentity,
			)
		}
	} else {
		expected, err = reviewtransaction.NewArtifactSubject(
			state, envelope.Subject.AuthorityRevision, frozen, state.SelectedLenses[order], order, envelope.Subject.CorrectionTargetIdentity,
		)
	}
	if err != nil {
		return facadeReviewerResult{}, reviewtransaction.ArtifactSubject{}, err
	}
	result, err := decodeAdmittedReviewerResult(payload, expected, subjectFrozen)
	if err != nil {
		return facadeReviewerResult{}, reviewtransaction.ArtifactSubject{}, err
	}
	if state.State != reviewtransaction.StateReviewing && len(state.LensResults) > 0 {
		native := result.nativeLensResult()
		native.Lens = expected.Lens
		canonical, canonicalErr := reviewtransaction.CanonicalCompactLensResult(native)
		if canonicalErr != nil || order >= len(state.LensResults) || state.LensResults[order].ResultHash != canonical.ResultHash {
			return facadeReviewerResult{}, reviewtransaction.ArtifactSubject{}, errors.New("captured reviewer result does not match the completed authority")
		}
	}
	return result, expected, nil
}

func decodeAdmittedReviewerResult(payload []byte, expected reviewtransaction.ArtifactSubject, frozen reviewtransaction.FrozenCandidateContext) (facadeReviewerResult, error) {
	var envelope admittedReviewerResult
	if err := decodeFacadeJSONBytes(payload, &envelope); err != nil {
		return facadeReviewerResult{}, err
	}
	wantSchema := reviewAdmittedResultSchema
	if expected.Schema == reviewtransaction.ArtifactSubjectSchemaV1 {
		wantSchema = reviewtransaction.AdmittedReviewerResultSchemaV1
	}
	if envelope.Schema != wantSchema || !reflect.DeepEqual(envelope.Subject, expected) {
		return facadeReviewerResult{}, errors.New("captured reviewer result does not contain the exact provider-owned subject")
	}
	if err := envelope.Admission.Validate(expected); err != nil {
		return facadeReviewerResult{}, err
	}
	canonical, err := json.Marshal(envelope.Result)
	if err != nil {
		return facadeReviewerResult{}, err
	}
	canonical = append(canonical, '\n')
	native := envelope.Result.nativeLensResult()
	native.Lens = expected.Lens
	result, revalidated, err := reviewtransaction.AdmitArtifact(reviewtransaction.ArtifactAdmissionRequest{
		ExpectedSubject: expected, FrozenContext: frozen, EchoedSubjectHash: envelope.Result.SubjectHash,
		Inspection: envelope.Result.Inspection, Result: native,
		CandidateCausalFindingIDs: envelope.Admission.CandidateCausalFindingIDs,
		RawPayload:                canonical, CanonicalPayload: canonical,
	})
	if err != nil || revalidated.Decision != reviewtransaction.ArtifactAdmissionCompleted ||
		revalidated.CanonicalSHA256 != envelope.Admission.CanonicalSHA256 || result.ResultHash != envelope.Admission.ResultHash {
		return facadeReviewerResult{}, errors.New("captured reviewer result no longer satisfies its admission record")
	}
	return envelope.Result, nil
}

func verifiedCandidateCausalFindingIDs(ctx context.Context, repo string, snapshot reviewtransaction.Snapshot, result reviewtransaction.LensResult) ([]string, error) {
	ids := make([]string, 0)
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	for _, finding := range result.Findings {
		if !facadeSevere(finding.Severity) {
			continue
		}
		switch finding.CausalDisposition {
		case reviewtransaction.CausalIntroduced, reviewtransaction.CausalBehaviorActivated, reviewtransaction.CausalWorsened:
			changed, err := builder.CandidateLocationSupportsCausality(ctx, snapshot, finding.Location, finding.CausalDisposition)
			if err != nil {
				return nil, fmt.Errorf("verify candidate causality for finding %q: %w", finding.ID, err)
			}
			if changed {
				ids = append(ids, finding.ID)
			}
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func readVerifiedReviewerArtifact(artifact reviewResultArtifact, storeDir string, state reviewtransaction.CompactState, subjects ...reviewtransaction.ArtifactSubject) ([]byte, error) {
	if artifact.Schema != reviewResultArtifactSchema || artifact.Capability != reviewResultArtifactCapability ||
		artifact.LineageID != state.LineageID || artifact.TargetIdentity != state.InitialSnapshot.Identity ||
		artifact.SelectedOrder < 0 || artifact.SelectedOrder >= len(state.SelectedLenses) ||
		artifact.Lens != state.SelectedLenses[artifact.SelectedOrder] || !validReviewCapabilitySHA256(artifact.SHA256) {
		return nil, errors.New("artifact manifest does not match frozen lineage, target, lens, and order")
	}
	if len(subjects) > 0 {
		if !validReviewCapabilitySHA256(artifact.SubjectHash) || artifact.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted ||
			artifact.SubjectHash != subjects[0].SubjectHash || subjects[0].Lens != artifact.Lens || subjects[0].SelectedOrder != artifact.SelectedOrder {
			return nil, errors.New("artifact manifest does not match the provider-owned subject")
		}
	}
	wantPath := filepath.Join(storeDir, reviewtransaction.CompactReviewerResultsDir, fmt.Sprintf("%02d-%s.json", artifact.SelectedOrder, artifact.Lens))
	path := artifact.Path
	switch {
	case artifact.Path != "" && artifact.Reference != "":
		return nil, errors.New("artifact manifest mixes legacy path and opaque reference")
	case artifact.Reference != "":
		if artifact.Reference != reviewResultReference(artifact) {
			return nil, errors.New("artifact opaque reference does not match its frozen binding")
		}
		path = wantPath
	case artifact.Path == "":
		return nil, errors.New("artifact manifest has no provider-owned locator")
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path != wantPath {
		return nil, errors.New("artifact path is outside native transaction ownership")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 || !reviewArtifactModeSafe(pathInfo.Mode(), false) {
		return nil, errors.New("artifact is not an owner-only regular file")
	}
	// Windows resolves file IDs lazily from the path; snapshot it before the path can be replaced.
	if !os.SameFile(pathInfo, pathInfo) {
		return nil, errors.New("artifact identity is unavailable")
	}
	reviewArtifactAfterLstat()
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(pathInfo, opened) {
		return nil, errors.New("artifact path changed before read")
	}
	payload, err := io.ReadAll(io.LimitReader(file, reviewResultArtifactLimit+1))
	if err != nil || len(payload) > reviewResultArtifactLimit {
		return nil, errors.New("artifact exceeds the native result size limit")
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, after) {
		return nil, errors.New("artifact path changed during read")
	}
	if facadePayloadHash(payload) != artifact.SHA256 {
		return nil, errors.New("artifact SHA-256 mismatch")
	}
	return payload, nil
}
func decodeFacadeJSONBytes(payload []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("input contains multiple JSON values")
	}
	return nil
}
func reviewArtifactModeSafe(mode os.FileMode, directory bool) bool {
	return reviewArtifactModeSafeForOS(mode, directory, runtime.GOOS)
}
func reviewArtifactModeSafeForOS(mode os.FileMode, directory bool, goos string) bool {
	return goos == "windows" || mode.Perm()&0o077 == 0 && (!directory || mode.Perm()&0o700 == 0o700)
}
func removeOwnedArtifact(path string, owned os.FileInfo) {
	if owned == nil {
		return
	}
	if current, err := os.Lstat(path); err == nil && os.SameFile(current, owned) {
		_ = os.Remove(path)
	}
}
