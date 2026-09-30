package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

const (
	reviewCapturePreflightSchema     = "gentle-ai.review-capture-preflight/v1"
	reviewCapturePreflightCapability = "review.native_capture_preflight"
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

// resolveOpaqueReviewRepositoryRoot verifies one provider-issued repository
// context against the repository the caller named, and converts any failure
// into a typed, path-free operation failure. The handle commits to a digest, so
// the repository has to come from the caller for the check to mean anything.
func resolveOpaqueReviewRepositoryRoot(ctx context.Context, repo, handle string, binding reviewtransaction.ReviewRepositoryContextBinding) (string, error) {
	root, err := reviewtransaction.ResolveReviewRepositoryContext(ctx, repo, handle, binding)
	if err != nil {
		return "", reviewRepositoryContextResolutionFailure(err)
	}
	return root, nil
}

// reviewRepositoryContextResolutionFailure classifies why an opaque repository
// context could not be resolved. A Git ownership refusal gets its own code and
// its own carry-outable instruction; structurally invalid rctx2 bindings get
// rctx2_binding_unusable, a resolution failure against a changed worktree gets
// rctx2_resolution_failed, and an identity mismatch gets its own code. A newer
// authority and everything else keep their existing codes.
func reviewRepositoryContextResolutionFailure(err error) error {
	if reviewGitOwnershipRefusal(err) {
		return reviewOpaqueContextFailure(reviewGitTrustRefusalCode, reviewGitTrustRefusalAction)
	}
	if errors.Is(err, reviewtransaction.ErrCompactAuthorityFromNewerRelease) {
		return reviewOpaqueContextCause(reviewAuthorityNewerReleaseCode, reviewAuthorityNewerReleaseAction, err)
	}
	// Structurally invalid binding — malformed digest, expired handle, missing
	// fields. No manual repair is possible; the maintainer must restart from
	// the exact native next_transition.
	if errors.Is(err, reviewtransaction.ErrInvalidReviewRepositoryContextV2) {
		return reviewOpaqueContextFailure(reviewRctx2UnusableCode, reviewRctx2UnusableAction)
	}
	// rctx2 resolution failed because the underlying worktree or repository
	// identity changed after the binding was issued.
	var resolutionErr *reviewtransaction.ReviewRepositoryContextV2ResolutionError
	if errors.As(err, &resolutionErr) {
		return reviewOpaqueContextCause(reviewRctx2ResolutionFailureCode, reviewRctx2ResolutionFailureAction, err)
	}
	// Identity mismatch — binding commits to a different repository than the
	// one named by --cwd.
	var identityErr *reviewtransaction.ReviewRepositoryContextIdentityError
	if errors.As(err, &identityErr) {
		return reviewOpaqueContextCause(reviewRctx2IdentityMismatchCode, reviewRctx2IdentityMismatchAction, err)
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

	// reviewRctx2UnusableCode is the code when the opaque rctx2 binding is
	// structurally invalid (missing fields, malformed digest, expired handle).
	// The only resolution is a fresh native next_transition — no manual
	// repair is possible because the provider owns the token lifecycle.
	reviewRctx2UnusableCode = "rctx2_binding_unusable"
	// reviewRctx2UnusableAction is the instruction for that code. The
	// binding is a provider-issued digest that commits to schema, lineage,
	// target identity, and repository identity; a malformed one proves the
	// host did not forward the exact transition the reviewer expects.
	reviewRctx2UnusableAction = "the provider-issued rctx2 binding is structurally invalid or has expired; " +
		"restart from the exact native next_transition that produced it — manual repair of the binding is impossible because the provider owns the token lifecycle"

	// reviewRctx2ResolutionFailureCode is the code when the rctx2 binding is
	// structurally valid but the underlying repository has changed (different
	// identity, moved directory, or repository destroyed). The binding was
	// correct when issued but no longer matches the live worktree.
	reviewRctx2ResolutionFailureCode = "rctx2_resolution_failed"
	// reviewRctx2ResolutionFailureAction is the instruction for that code.
	// The maintainer must start a fresh native review to get a binding that
	// matches the current worktree identity.
	reviewRctx2ResolutionFailureAction = "the rctx2 binding was valid when issued but the underlying repository has changed; " +
		"start a fresh native review to obtain a binding that matches the current worktree identity"

	// reviewRctx2IdentityMismatchCode is the code when the rctx2 binding's
	// repository identity does not match the caller-supplied repository root.
	// This typically happens when the caller points to a different worktree
	// or cloned copy than the one the provider issued the binding for.
	reviewRctx2IdentityMismatchCode = "rctx2_identity_mismatch"
	// reviewRctx2IdentityMismatchAction is the instruction for that code.
	reviewRctx2IdentityMismatchAction = "the rctx2 binding commits to a different repository identity than the one named by --cwd; " +
		"use the --cwd that matches the provider's worktree, or start a fresh native review"
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
