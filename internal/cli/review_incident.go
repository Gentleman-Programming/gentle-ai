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
	"strings"
	"syscall"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	reviewCapturePreflightSchema     = "gentle-ai.review-capture-preflight/v1"
	reviewCapturePreflightCapability = "review.native_capture_preflight"
	reviewIncidentArtifactSchema     = "gentle-ai.review-incident-artifact/v1"
	reviewIncidentArtifactCapability = "review.native_incident_artifact"
	reviewIncidentReferencePrefix    = "rinc1_"
)

// reviewCapturePreflightResult confirms that one capture binding matches the
// reviewing authority reachable from the resolved repository root, before a
// bound reviewer consumes its exactly-once invocation.
type reviewCapturePreflightResult struct {
	Schema              string                                       `json:"schema"`
	Capability          string                                       `json:"capability"`
	RepositoryRoot      string                                       `json:"repository_root,omitempty"`
	LineageID           string                                       `json:"lineage_id"`
	TargetIdentity      string                                       `json:"target_identity"`
	Lens                string                                       `json:"lens"`
	SelectedOrder       int                                          `json:"selected_order"`
	ArtifactSubject     reviewtransaction.ArtifactSubject            `json:"artifact_subject"`
	BaseTree            string                                       `json:"base_tree"`
	CandidateTree       string                                       `json:"candidate_tree"`
	ChangedPathManifest []reviewtransaction.ChangedPathManifestEntry `json:"changed_path_manifest"`
}

// reviewIncidentArtifact references one durably preserved raw reviewer result.
// Its schema is distinct from the captured-result artifact schema on purpose:
// finalize rejects it, so a preserved incident can never masquerade as a
// verified lens capture.
type reviewIncidentArtifact struct {
	Schema         string                                `json:"schema"`
	Capability     string                                `json:"capability"`
	Path           string                                `json:"path,omitempty"`
	Reference      string                                `json:"reference,omitempty"`
	SHA256         string                                `json:"sha256"`
	LineageID      string                                `json:"lineage_id"`
	TargetIdentity string                                `json:"target_identity"`
	Lens           string                                `json:"lens"`
	SelectedOrder  int                                   `json:"selected_order"`
	Class          reviewtransaction.ResultIncidentClass `json:"class,omitempty"`
}

type reviewOpaqueContextOperationError struct {
	Code   string
	Action string
	// Cause is the scrubbed one-line reason this operation failed, empty when
	// the code alone already identifies the failure exactly (a Git trust
	// refusal, for example, has exactly one cause).
	Cause string
}

func (err *reviewOpaqueContextOperationError) Error() string {
	if err.Cause == "" {
		return fmt.Sprintf("%s: provider-issued review repository context operation failed; %s", err.Code, err.Action)
	}
	return fmt.Sprintf("%s: provider-issued review repository context operation failed; cause: %s; %s", err.Code, err.Cause, err.Action)
}

func reviewOpaqueContextFailure(code, action string) error {
	return reviewPreflightError(&reviewOpaqueContextOperationError{Code: code, Action: action})
}

// reviewOpaqueContextCause reports the same typed code and instruction as
// reviewOpaqueContextFailure and additionally names the scrubbed cause.
//
// Why this exists: the opaque path used to answer every distinct failure behind
// one code with one constant sentence, so a missing authority record, an
// unparsable one, a Git setup refusal, and a stale binding all reached the
// reporter as the same string. That is the mechanism behind community reports
// #2227, #2411 and #2461: different roots, one message, and no way for the
// reporter or the maintainer to tell them apart. The opacity itself is not
// the defect -- an absolute path must never reach a session transcript -- so
// the cause is forwarded through the SAME privacy gate the defect reporter
// already uses (reviewScrubDefectReportField) rather than dropped.
func reviewOpaqueContextCause(code, action string, cause error) error {
	return reviewPreflightError(&reviewOpaqueContextOperationError{
		Code: code, Action: action, Cause: reviewScrubOpaqueContextCause(cause),
	})
}

// reviewOpaqueContextCauseLimit bounds a forwarded cause. Native errors can
// quote reviewer payload fragments, and the destination is a session
// transcript, not a log file.
const reviewOpaqueContextCauseLimit = 512

func reviewScrubOpaqueContextCause(cause error) string {
	scrubbed := reviewScrubDefectReportField(reviewOpaqueContextCauseChain(cause))
	if runes := []rune(scrubbed); len(runes) > reviewOpaqueContextCauseLimit {
		return string(runes[:reviewOpaqueContextCauseLimit]) + " (truncated)"
	}
	return scrubbed
}

// reviewOpaqueContextCauseChain flattens an error chain into one line.
//
// Walking the chain instead of reading only the outermost Error() is
// load-bearing, not defensive: reviewtransaction deliberately flattens some
// failures behind a fixed public sentence and keeps the real cause reachable
// only through Unwrap (reviewRepositoryContextIdentityError and
// reviewRepositoryContextTargetedValidationError both do this). Reading the
// surface alone would re-collapse exactly the failures this change exists to
// separate. A link whose message is already contained in what has been
// collected is skipped, so ordinary %w wrapping does not repeat itself.
func reviewOpaqueContextCauseChain(cause error) string {
	message := ""
	for err := cause; err != nil; err = errors.Unwrap(err) {
		next := strings.TrimSpace(err.Error())
		if next == "" || strings.Contains(message, next) {
			continue
		}
		if message == "" {
			message = next
			continue
		}
		message += ": " + next
	}
	return message
}

const (
	// reviewGitTrustRefusalCode is the typed, path-free code for "Git itself
	// declined to open the bound repository in this process because the
	// repository is owned by another account". It is deliberately distinct
	// from repository_context_unavailable: no review action can repair it,
	// because a running process cannot change its own inherited Git trust
	// configuration.
	reviewGitTrustRefusalCode = "git_repository_untrusted"
	// reviewGitTrustRefusalAction is the instruction for that code. gentle-ai
	// never provisions safe.directory and never bypasses Git's ownership
	// protection, so the only thing the caller can actually do is relaunch
	// the host process under a Git context that already trusts the
	// repository. The wording contains no slash, backslash, newline, address,
	// or KEY=VALUE token, so reviewScrubDefectReportField leaves it byte
	// identical: this string can never become a path leak.
	reviewGitTrustRefusalAction = "Git declined to open the bound repository in this process because it is owned by a different account; " +
		"gentle-ai never provisions a safe.directory exception and never bypasses that protection. " +
		"Restart the host process under a Git context that already trusts that repository, then retry the same exact binding"
	// gitSafeDirectoryHint is the second half of Git's ownership refusal:
	// every version that emits the refusal also emits this remediation hint
	// in the same die() call.
	gitSafeDirectoryHint = "git config --global --add safe.directory "
	// gitDieExitCode is the exit status Git's die() always uses, and the one
	// a real staged ownership refusal was observed to return.
	gitDieExitCode = 128
)

// resolveOpaqueReviewRepositoryRoot resolves one provider-issued repository
// context and converts any failure into a typed, path-free operation failure.
func resolveOpaqueReviewRepositoryRoot(ctx context.Context, handle string, binding reviewtransaction.ReviewRepositoryContextBinding) (string, error) {
	root, err := reviewtransaction.ResolveReviewRepositoryContext(ctx, handle, binding)
	if err != nil {
		return "", reviewRepositoryContextResolutionFailure(err)
	}
	return root, nil
}

// reviewRepositoryContextResolutionFailure classifies why an opaque repository
// context could not be resolved. A Git ownership refusal gets its own code and
// its own carry-outable instruction; everything else keeps the historical
// generic code, so an unrelated failure is never mislabelled as a trust
// problem.
func reviewRepositoryContextResolutionFailure(err error) error {
	if reviewGitOwnershipRefusal(err) {
		return reviewOpaqueContextFailure(reviewGitTrustRefusalCode, reviewGitTrustRefusalAction)
	}
	// Authority written by a newer release is not a stale context, and the
	// generic instruction is actively wrong for it: refreshing a transition
	// cannot make this binary parse those bytes. #2461's reporter followed
	// that instruction across four reoffered reviewer slots and got an
	// identical failure every time.
	if errors.Is(err, reviewtransaction.ErrCompactAuthorityFromNewerRelease) {
		return reviewOpaqueContextCause(reviewAuthorityNewerReleaseCode, reviewAuthorityNewerReleaseAction, err)
	}
	return reviewOpaqueContextCause("repository_context_unavailable", "refresh the exact native next_transition before retrying", err)
}

const (
	// reviewAuthorityNewerReleaseCode is the typed code for "the authority
	// exists and is intact, but this build predates the release that wrote
	// it". It is deliberately distinct from repository_context_unavailable:
	// that code's whole instruction is to refresh the transition, which is
	// unreachable here.
	reviewAuthorityNewerReleaseCode = "review_authority_newer_release"
	// reviewAuthorityNewerReleaseAction names the only two things that
	// resolve it, and the PATH shape, because a bare-name spawn from an
	// editor plugin is how the stale binary gets invoked: the caller
	// installed the newer build but an older one answers first.
	reviewAuthorityNewerReleaseAction = "upgrade this gentle-ai, or invoke the newer build directly; " +
		"an editor plugin resolves gentle-ai from PATH, so run `which -a gentle-ai` and make the newer build the one it finds first"
)

// reviewGitOwnershipRefusal reports whether err was caused by Git refusing a
// repository for ownership reasons.
//
// Detection requires three independent signals from the same failure, so it
// cannot fire on an unrelated error: the cause must be a Git subprocess
// failure (*reviewtransaction.GitCommandError), it must carry Git's die() exit
// status, and its diagnostic must carry BOTH halves of the single die() call
// that Git emits for this refusal — an anchored `fatal:` line naming the
// refusal, and the safe.directory remediation hint. Matching text is
// unavoidable because an ownership refusal is not signalled any other way, but
// every Git invocation in this codebase runs under a forced LC_ALL=C, which
// pins the diagnostic to Git's untranslated English wording.
func reviewGitOwnershipRefusal(err error) bool {
	var commandErr *reviewtransaction.GitCommandError
	if !errors.As(err, &commandErr) || commandErr.ExitCode != gitDieExitCode {
		return false
	}
	if !strings.Contains(commandErr.Output, gitSafeDirectoryHint) {
		return false
	}
	for _, line := range strings.Split(commandErr.Output, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if !strings.HasPrefix(line, "fatal: ") {
			continue
		}
		// Current wording (Git 2.35.2 and newer).
		if strings.Contains(line, "detected dubious ownership in repository at '") {
			return true
		}
		// The wording Git 2.35.2 itself shipped, kept so an older installed
		// Git still produces the actionable diagnostic.
		if strings.Contains(line, "unsafe repository ('") && strings.Contains(line, "' is owned by someone else)") {
			return true
		}
	}
	return false
}

// RunReviewPreserveResult durably preserves one raw reviewer result as an
// incident artifact beside the compact review authority root after a failed
// capture. It binds the incident to the exact live selected lens position but
// never validates the payload contents or counts it as a captured lens result; recovery re-runs
// `review capture-result` with the preserved payload from the reviewing
// repository, which performs full native verification.
//
// Incidents are append-only audit history: each distinct raw payload for a
// slot is preserved under its own digest-suffixed name, so repeated failures
// accumulate instead of replacing earlier evidence, and a later successful
// capture of the same lens never removes them. Because a rejected admission
// never consumes the immutable lens slot, the recovery for a malformed
// payload (for example one missing the top-level subject_hash/inspection
// envelope) is to re-run the lens and capture a corrected result on the same
// lineage.
func RunReviewPreserveResult(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review preserve-result", stdout, "Durably preserve one raw reviewer result as an incident artifact after a failed capture; never a captured lens result.")
	cwd := flags.String("cwd", ".", "repository path")
	repositoryContext := flags.String("repository-context", "", "opaque provider-issued repository context")
	lineage := flags.String("lineage", "", "exact review lineage identifier from the capture binding")
	target := flags.String("target", "", "exact frozen target identity from the capture binding")
	lens := flags.String("lens", "", "exact selected lens from the capture binding")
	order := flags.Int("order", -1, "zero-based selected lens order from the capture binding")
	revision := flags.String("expected-revision", "", "exact reviewing authority revision")
	input := flags.String("input", "", "raw reviewer result file or - for stdin")
	class := flags.String("class", "", "extraction-failure classification: empty_result or nested_envelope")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 || strings.TrimSpace(*input) == "" {
		return reviewPreflightError(errors.New("review preserve-result requires an exact repository context, --lineage, --target, --lens, --order, and --input"))
	}
	contextHandle := strings.TrimSpace(*repositoryContext)
	if contextHandle != "" && reviewFlagWasProvided(flags, "cwd") {
		return reviewPreflightError(errors.New("review preserve-result accepts either --repository-context or --cwd, not both"))
	}
	if contextHandle != "" && strings.TrimSpace(*revision) == "" {
		return reviewPreflightError(errors.New("review preserve-result with --repository-context requires --expected-revision"))
	}
	if contextHandle == "" && strings.TrimSpace(*revision) != "" {
		return reviewPreflightError(errors.New("review preserve-result accepts --expected-revision only with --repository-context"))
	}
	switch *lens {
	case reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability:
	default:
		return reviewPreflightError(fmt.Errorf("review preserve-result requires one exact canonical lens; got %q", *lens))
	}
	if !validReviewCapabilitySHA256(*target) || *order < 0 || *order >= 4 {
		return reviewPreflightError(errors.New("review preserve-result requires the exact frozen target identity and selected lens order"))
	}
	if !reviewtransaction.ValidResultIncidentClass(reviewtransaction.ResultIncidentClass(*class)) {
		return reviewPreflightError(fmt.Errorf("review preserve-result requires --class to be empty or one exact canonical incident class; got %q", *class))
	}
	ctx := context.Background()
	repo := *cwd
	if contextHandle != "" {
		resolved, err := resolveOpaqueReviewRepositoryRoot(ctx, contextHandle, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: *lineage, TargetIdentity: *target, Revision: *revision,
		})
		if err != nil {
			return err
		}
		repo = resolved
	}
	if err := authorizeReviewAuthorityMutation(ctx, repo); err != nil {
		return err
	}
	_, record, err := discoverCompactFacadeReview(ctx, repo, *lineage, false)
	if err != nil {
		if contextHandle != "" {
			return reviewOpaqueContextCause("repository_context_authority_unavailable", "refresh the exact native next_transition before retrying", err)
		}
		return reviewPreflightError(fmt.Errorf("resolve reviewing authority for preserve-result: %w", err))
	}
	state := record.State
	if state.State != reviewtransaction.StateReviewing || state.LineageID != *lineage ||
		state.InitialSnapshot.Identity != *target || contextHandle != "" && record.Revision != *revision ||
		*order >= len(state.SelectedLenses) || state.SelectedLenses[*order] != *lens {
		if contextHandle != "" {
			return reviewOpaqueContextFailure("repository_context_binding_mismatch", "refresh the exact native next_transition before retrying")
		}
		return reviewPreflightError(errors.New("preserve-result binding does not match the current reviewing authority"))
	}
	if _, err := reviewtransaction.CompactIncidentsDir(ctx, repo, *lineage); err != nil {
		if contextHandle != "" {
			return reviewOpaqueContextCause("repository_context_preserve_unavailable", "retry preserve-result with the same exact binding", err)
		}
		return reviewPreflightError(fmt.Errorf("resolve incident preservation directory: %w", err))
	}
	payload, err := readFacadeBytes(*input)
	if err != nil {
		return reviewPreflightError(fmt.Errorf("read raw reviewer result: %w", err))
	}
	if len(payload) == 0 || len(payload) > reviewResultArtifactLimit {
		return reviewPreflightError(errors.New("raw reviewer result must be non-empty and within the native result size limit"))
	}
	artifact, err := preserveIncidentArtifact(ctx, repo, *lineage, *target, *lens, *order, payload, reviewtransaction.ResultIncidentClass(*class))
	if err != nil {
		if contextHandle != "" {
			return reviewOpaqueContextCause("repository_context_preserve_failed", "retry preserve-result with the same exact binding", err)
		}
		return reviewPreflightError(err)
	}
	if contextHandle != "" {
		artifact.Reference = reviewIncidentReference(artifact)
		artifact.Path = ""
	}
	return encodeReviewJSON(stdout, artifact)
}

func reviewIncidentReference(artifact reviewIncidentArtifact) string {
	preimage := struct {
		Schema, Capability, SHA256, LineageID, TargetIdentity, Lens string
		SelectedOrder                                               int
	}{
		Schema: artifact.Schema, Capability: artifact.Capability, SHA256: artifact.SHA256,
		LineageID: artifact.LineageID, TargetIdentity: artifact.TargetIdentity,
		Lens: artifact.Lens, SelectedOrder: artifact.SelectedOrder,
	}
	payload, _ := json.Marshal(preimage)
	return reviewIncidentReferencePrefix + strings.TrimPrefix(facadePayloadHash(payload), "sha256:")
}

func preserveIncidentArtifact(ctx context.Context, repo, lineage, target, lens string, order int, payload []byte, class reviewtransaction.ResultIncidentClass) (reviewIncidentArtifact, error) {
	dir, err := reviewtransaction.EnsureCompactIncidentsDir(ctx, repo, lineage)
	if err != nil {
		return reviewIncidentArtifact{}, fmt.Errorf("create incident preservation directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !reviewArtifactModeSafe(info.Mode(), true) {
		return reviewIncidentArtifact{}, errors.New("incident preservation directory is not a private native directory")
	}
	hash := facadePayloadHash(payload)
	digest12 := strings.TrimPrefix(hash, "sha256:")[:12]
	name := fmt.Sprintf("%02d-%s-%s.raw", order, lens, digest12)
	if class != "" {
		name = fmt.Sprintf("%02d-%s-%s-%s.raw", order, lens, class, digest12)
	}
	path := filepath.Join(dir, name)
	artifact := reviewIncidentArtifact{
		Schema: reviewIncidentArtifactSchema, Capability: reviewIncidentArtifactCapability, Path: path,
		SHA256: hash, LineageID: lineage, TargetIdentity: target, Lens: lens, SelectedOrder: order, Class: class,
	}
	if existing, readErr := os.ReadFile(path); readErr == nil {
		if !bytes.Equal(existing, payload) {
			return reviewIncidentArtifact{}, errors.New("incident artifact already exists with different bytes")
		}
		return artifact, nil
	} else if !os.IsNotExist(readErr) {
		return reviewIncidentArtifact{}, readErr
	}
	temp, err := os.CreateTemp(dir, ".incident-*")
	if err != nil {
		return reviewIncidentArtifact{}, fmt.Errorf("create incident temporary file: %w", err)
	}
	owned, _ := temp.Stat()
	defer removeOwnedArtifact(temp.Name(), owned)
	if err := temp.Chmod(0o600); err != nil {
		return reviewIncidentArtifact{}, err
	}
	if _, err := temp.Write(payload); err != nil {
		return reviewIncidentArtifact{}, err
	}
	if err := temp.Sync(); err != nil {
		return reviewIncidentArtifact{}, err
	}
	if err := temp.Close(); err != nil {
		return reviewIncidentArtifact{}, err
	}
	if err := reviewtransaction.PublishFileNoReplace(temp.Name(), path); err != nil {
		if existing, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(existing, payload) {
			return artifact, nil
		}
		return reviewIncidentArtifact{}, fmt.Errorf("publish incident artifact atomically: %w", err)
	}
	if err := syncReviewerArtifactDirectory(dir); err != nil {
		unsupported := errors.Is(err, syscall.EINVAL) || errors.Is(err, errors.ErrUnsupported) ||
			reviewArtifactRuntimeGOOS() == "windows" && errors.Is(err, os.ErrPermission)
		if !unsupported {
			removeOwnedArtifact(path, owned)
			return reviewIncidentArtifact{}, fmt.Errorf("sync incident preservation directory: %w", err)
		}
	}
	if preserved, err := os.ReadFile(path); err != nil || !bytes.Equal(preserved, payload) {
		removeOwnedArtifact(path, owned)
		return reviewIncidentArtifact{}, fmt.Errorf("read back incident artifact: %w", err)
	}
	return artifact, nil
}
