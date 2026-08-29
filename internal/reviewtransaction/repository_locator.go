package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	ReviewRepositoryContextCapability = "review.opaque_repository_context"
	ReviewRepositoryContextSchema     = "gentle-ai.review-repository-context/v1"

	reviewRepositoryContextHandlePrefix = "rctx1_"
	reviewRepositoryLocatorMaxBytes     = 64 << 10

	reviewRepositoryContextV2Schema          = "gentle-ai.review-repository-context/v2"
	reviewRepositoryContextV2HandlePrefix    = "rctx2_"
	reviewRepositoryContextV2MaxDecodedBytes = 64 << 10
	// The v2 handle is a fixed-width digest, not a transported payload. Its
	// preimage names the repository identity and the capture binding; the
	// caller supplies both again at resolution, so the token never has to
	// carry a filesystem path to stay self-contained.
	reviewRepositoryContextV2DigestBytes     = 64
	reviewRepositoryContextV2MaxEncodedBytes = len(reviewRepositoryContextV2HandlePrefix) + reviewRepositoryContextV2DigestBytes
)

// ErrRepositoryIdentityChanged reports that a live repository no longer
// matches the exact filesystem identity captured by its lease.
var ErrRepositoryIdentityChanged = errors.New("repository identity changed")

// ReviewRepositoryContextBinding is the public, path-free portion of a
// provider-issued repository context. The handle is discovery only: current
// compact authority remains the sole authorization source.
type ReviewRepositoryContextBinding struct {
	LineageID      string `json:"lineage_id"`
	TargetIdentity string `json:"target_identity"`
	Revision       string `json:"revision"`
}

type reviewRepositoryIdentityRecord struct {
	RepositoryRoot     string `json:"repository_root"`
	GitCommonDir       string `json:"git_common_dir"`
	GitDir             string `json:"git_dir"`
	RepositoryIdentity string `json:"repository_identity"`
}

// RepositoryIdentity is the exact Git worktree identity bound by a
// RepositoryIdentityLease. RepositoryRef preserves the existing canonical
// sha256 reference used by review authority records.
type RepositoryIdentity struct {
	RepositoryRoot string
	GitCommonDir   string
	GitDir         string
	RepositoryRef  string
}

type reviewRepositoryDirectoryIdentity struct {
	path string
	info fs.FileInfo
}

type reviewRepositoryControlIdentity struct {
	path       string
	info       fs.FileInfo
	payload    []byte
	hasPayload bool
}

// RepositoryIdentityLease is an immutable, read-only binding to one exact Git
// worktree. Callers retain the pointer and validate it immediately around
// authority operations that must fail closed across Git metadata replacement.
type RepositoryIdentityLease struct {
	identity      RepositoryIdentity
	storageKey    string
	root          reviewRepositoryDirectoryIdentity
	commonDir     reviewRepositoryDirectoryIdentity
	gitDir        reviewRepositoryDirectoryIdentity
	gitControl    reviewRepositoryControlIdentity
	commonControl *reviewRepositoryControlIdentity
}

type reviewTargetedValidationContext struct {
	RequestHash             string `json:"request_hash"`
	CorrectionCandidateTree string `json:"correction_candidate_tree"`
}

type reviewRepositoryContextFile struct {
	Schema             string                           `json:"schema"`
	Handle             string                           `json:"handle"`
	LineageID          string                           `json:"lineage_id"`
	TargetIdentity     string                           `json:"target_identity"`
	Revision           string                           `json:"revision"`
	RepositoryIdentity string                           `json:"repository_identity"`
	RepositoryRoot     string                           `json:"repository_root"`
	GitCommonDir       string                           `json:"git_common_dir"`
	GitDir             string                           `json:"git_dir"`
	TargetedValidation *reviewTargetedValidationContext `json:"targeted_validation,omitempty"`
}

// reviewRepositoryContextV2Token is the canonical digest preimage for one v2
// handle. It is never transported: only its digest crosses a process boundary,
// so the paths below stay inside this package and out of every command line,
// log, and relayed host payload.
type reviewRepositoryContextV2Token struct {
	Schema               string `json:"schema"`
	RepositoryRoot       string `json:"repository_root"`
	GitCommonDir         string `json:"git_common_dir"`
	GitDir               string `json:"git_dir"`
	RepositoryRef        string `json:"repository_ref"`
	LineageID            string `json:"lineage_id"`
	TargetIdentity       string `json:"target_identity"`
	CapturePhaseRevision string `json:"capture_phase_revision"`
}

var errInvalidReviewRepositoryContextV2 = errors.New("invalid rctx2 repository context") // refusal:by-design operator-knowledge: callers must refresh the provider-issued repository context instead of attempting to repair an untrusted token

type reviewRepositoryContextV2ResolutionError struct{ cause error }

func (err *reviewRepositoryContextV2ResolutionError) Error() string {
	return errInvalidReviewRepositoryContextV2.Error()
}

func (err *reviewRepositoryContextV2ResolutionError) Unwrap() error { return err.cause }

func invalidReviewRepositoryContextV2Resolution(cause error) error {
	if cause == nil {
		return errInvalidReviewRepositoryContextV2
	}
	return &reviewRepositoryContextV2ResolutionError{cause: cause}
}

// OpenRepositoryIdentityLease resolves and captures one exact Git worktree
// without creating repository or authority storage.
func OpenRepositoryIdentityLease(ctx context.Context, repo string) (*RepositoryIdentityLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	return openRepositoryIdentityLeaseAtRoot(ctx, root)
}

// Identity returns the immutable value identity captured by the lease.
func (lease *RepositoryIdentityLease) Identity() RepositoryIdentity {
	if lease == nil {
		return RepositoryIdentity{}
	}
	return lease.identity
}

// StorageKey returns the repository reference digest as a path-safe,
// lowercase 64-hex segment without the sha256 prefix.
func (lease *RepositoryIdentityLease) StorageKey() string {
	if lease == nil {
		return ""
	}
	return lease.storageKey
}

// Validate proves that the exact root, Git directories, and Git control
// entries captured by the lease are still live and resolve to the same
// canonical repository reference.
func (lease *RepositoryIdentityLease) Validate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if lease == nil || !validRepositoryIdentityLease(lease) {
		return errors.New("repository identity lease is not initialized")
	}
	if err := lease.validateCapturedIdentity(); err != nil {
		return repositoryIdentityChanged(err)
	}
	live, err := openRepositoryIdentityLeaseAtRoot(ctx, lease.identity.RepositoryRoot)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return repositoryIdentityChanged(err)
	}
	if live.identity != lease.identity || live.storageKey != lease.storageKey {
		return repositoryIdentityChanged(errors.New("canonical repository reference changed"))
	}
	if err := lease.validateCapturedIdentity(); err != nil {
		return repositoryIdentityChanged(err)
	}
	return nil
}

// DeriveReviewRepositoryContextHandle derives the self-contained rctx2 handle
// that current lifecycle routes render from the active compact authority. It has
// no locator publication, readiness, or caller-owned replay side effect.
func DeriveReviewRepositoryContextHandle(ctx context.Context, repo string, binding ReviewRepositoryContextBinding) (string, error) {
	return deriveReviewRepositoryContextV2Token(ctx, repo, binding)
}

// deriveReviewRepositoryContextV2Token creates the self-contained rctx2 core
// without publishing a locator or wiring a public consumer. Its explicit
// authority check keeps token creation bounded to the currently active compact
// record; its resolver repeats the same check before returning any identity.
func deriveReviewRepositoryContextV2Token(ctx context.Context, repo string, binding ReviewRepositoryContextBinding) (string, error) {
	if ctx == nil || ctx.Err() != nil || validateReviewRepositoryContextBinding(binding) != nil {
		return "", errInvalidReviewRepositoryContextV2
	}
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil || lease.Validate(ctx) != nil {
		return "", errInvalidReviewRepositoryContextV2
	}
	identity := lease.Identity()
	// Derivation is pure so START can use the same canonical token while it
	// proves reviewer-context capacity before its first CAS write. Every active
	// resolver below revalidates the token against Compact authority before it
	// returns a repository root or allows a caller to continue.
	return encodeReviewRepositoryContextV2Token(reviewRepositoryContextV2Token{
		Schema:               reviewRepositoryContextV2Schema,
		RepositoryRoot:       identity.RepositoryRoot,
		GitCommonDir:         identity.GitCommonDir,
		GitDir:               identity.GitDir,
		RepositoryRef:        identity.RepositoryRef,
		LineageID:            binding.LineageID,
		TargetIdentity:       binding.TargetIdentity,
		CapturePhaseRevision: binding.Revision,
	})
}

// resolveReviewRepositoryContextV2Token decodes a self-contained rctx2 token
// and confirms its frozen facts against the active compact authority. It is
// intentionally read-only and normalizes every refusal to one path-free error.
func resolveReviewRepositoryContextV2Token(ctx context.Context, repo, handle string, binding ReviewRepositoryContextBinding) (string, ReviewRepositoryContextBinding, error) {
	if ctx == nil || ctx.Err() != nil || validateReviewRepositoryContextBinding(binding) != nil {
		return "", ReviewRepositoryContextBinding{}, errInvalidReviewRepositoryContextV2
	}
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV2Resolution(err)
	}
	if err := lease.Validate(ctx); err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV2Resolution(err)
	}
	identity := lease.Identity()
	if err := matchReviewRepositoryContextV2Handle(handle, identity, binding); err != nil {
		return "", ReviewRepositoryContextBinding{}, err
	}
	store, err := CompactAuthoritativeStore(ctx, identity.RepositoryRoot, binding.LineageID)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV2Resolution(err)
	}
	record, err := store.LoadContext(ctx)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV2Resolution(err)
	}
	if err := validateReviewRepositoryContextRecord(ctx, identity.RepositoryRoot, binding, record); err != nil {
		return "", ReviewRepositoryContextBinding{}, errInvalidReviewRepositoryContextV2
	}
	if err := lease.Validate(ctx); err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV2Resolution(err)
	}
	return identity.RepositoryRoot, binding, nil
}

// resolveReviewRepositoryContextV2TokenForCorrectedInspection validates every
// token and authority fact that is available before the provider-owned targeted
// request is decoded. The request then proves the correction target/tree/hash;
// this preserves immutable inspection after later workspace drift without adding
// a locator-backed request sidecar.
func resolveReviewRepositoryContextV2TokenForCorrectedInspection(ctx context.Context, repo, handle string, binding ReviewRepositoryContextBinding) (string, ReviewRepositoryContextBinding, error) {
	if ctx == nil || ctx.Err() != nil || validateReviewRepositoryContextBinding(binding) != nil {
		return "", ReviewRepositoryContextBinding{}, errInvalidReviewRepositoryContextV2
	}
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV2Resolution(err)
	}
	if err := lease.Validate(ctx); err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV2Resolution(err)
	}
	identity := lease.Identity()
	if err := matchReviewRepositoryContextV2Handle(handle, identity, binding); err != nil {
		return "", ReviewRepositoryContextBinding{}, err
	}
	store, err := CompactAuthoritativeStore(ctx, identity.RepositoryRoot, binding.LineageID)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV2Resolution(err)
	}
	record, err := store.LoadContext(ctx)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV2Resolution(err)
	}
	if record.State.LineageID != binding.LineageID || record.State.CapturePhaseRevision != binding.Revision ||
		record.State.State != StateCorrectionRequired || record.State.ProposedCorrectionLines == nil || record.State.CorrectionAttemptConsumed() {
		return "", ReviewRepositoryContextBinding{}, errInvalidReviewRepositoryContextV2
	}
	if err := lease.Validate(ctx); err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV2Resolution(err)
	}
	return identity.RepositoryRoot, binding, nil
}

func encodeReviewRepositoryContextV2Token(token reviewRepositoryContextV2Token) (string, error) {
	payload, err := canonicalReviewRepositoryContextV2Payload(token)
	if err != nil {
		return "", errInvalidReviewRepositoryContextV2
	}
	return reviewRepositoryContextV2HandlePrefix + identityHash(string(payload)), nil
}

// validReviewRepositoryContextV2Handle validates only the opaque transport
// shape: the v2 prefix followed by a lowercase hex digest of fixed width.
func validReviewRepositoryContextV2Handle(handle string) bool {
	if !strings.HasPrefix(handle, reviewRepositoryContextV2HandlePrefix) || len(handle) != reviewRepositoryContextV2MaxEncodedBytes {
		return false
	}
	suffix := strings.TrimPrefix(handle, reviewRepositoryContextV2HandlePrefix)
	if suffix != strings.ToLower(suffix) {
		return false
	}
	_, err := hex.DecodeString(suffix)
	return err == nil
}

// matchReviewRepositoryContextV2Handle re-derives the handle from the live
// repository identity and the caller-supplied binding, then compares it to the
// handle the caller presented. The repository root arrives from the caller
// rather than from the token, so this is a real check: a handle that names a
// different repository, lineage, target, or revision cannot be made to match by
// pointing the resolver somewhere else.
func matchReviewRepositoryContextV2Handle(handle string, identity RepositoryIdentity, binding ReviewRepositoryContextBinding) error {
	if !validReviewRepositoryContextV2Handle(handle) {
		return errInvalidReviewRepositoryContextV2
	}
	derived, err := encodeReviewRepositoryContextV2Token(reviewRepositoryContextV2Token{
		Schema:               reviewRepositoryContextV2Schema,
		RepositoryRoot:       identity.RepositoryRoot,
		GitCommonDir:         identity.GitCommonDir,
		GitDir:               identity.GitDir,
		RepositoryRef:        identity.RepositoryRef,
		LineageID:            binding.LineageID,
		TargetIdentity:       binding.TargetIdentity,
		CapturePhaseRevision: binding.Revision,
	})
	if err != nil {
		return errInvalidReviewRepositoryContextV2
	}
	if subtle.ConstantTimeCompare([]byte(derived), []byte(handle)) != 1 {
		return errInvalidReviewRepositoryContextV2
	}
	return nil
}

func canonicalReviewRepositoryContextV2Payload(token reviewRepositoryContextV2Token) ([]byte, error) {
	if token.Schema != reviewRepositoryContextV2Schema || !validReviewRepositoryContextV2Path(token.RepositoryRoot) ||
		!validReviewRepositoryContextV2Path(token.GitCommonDir) || !validReviewRepositoryContextV2Path(token.GitDir) ||
		!validSHA256(token.RepositoryRef) || validateReviewRepositoryContextBinding(ReviewRepositoryContextBinding{
		LineageID: token.LineageID, TargetIdentity: token.TargetIdentity, Revision: token.CapturePhaseRevision,
	}) != nil {
		return nil, errInvalidReviewRepositoryContextV2
	}
	identity := reviewRepositoryIdentityRecord{
		RepositoryRoot: token.RepositoryRoot, GitCommonDir: token.GitCommonDir, GitDir: token.GitDir,
	}
	if token.RepositoryRef != reviewRepositoryIdentityHash(identity) {
		return nil, errInvalidReviewRepositoryContextV2
	}
	payload, err := json.Marshal(token)
	if err != nil || len(payload) > reviewRepositoryContextV2MaxDecodedBytes {
		return nil, errInvalidReviewRepositoryContextV2
	}
	return payload, nil
}

func validReviewRepositoryContextV2Path(path string) bool {
	return path != "" && !strings.ContainsRune(path, 0) && filepath.IsAbs(path) && filepath.Clean(path) == path
}

// ValidateReviewRepositoryContextHandle validates only the opaque transport
// shape. Resolution still performs repository and authority validation.
func ValidateReviewRepositoryContextHandle(handle string) error {
	if validReviewRepositoryContextHandle(handle) {
		return nil
	}
	if validReviewRepositoryContextV2Handle(handle) {
		return nil
	}
	return errors.New("invalid review repository context handle")
}

// ResolveReviewRepositoryContext resolves one provider-issued handle from any
// process cwd, then revalidates its repository and current compact authority.
func ResolveReviewRepositoryContext(ctx context.Context, repo, handle string, binding ReviewRepositoryContextBinding) (string, error) {
	if err := validateReviewRepositoryContextBinding(binding); err != nil {
		return "", err
	}
	root, resolved, err := ResolveReviewRepositoryContextBinding(ctx, repo, handle, binding)
	if err != nil {
		return "", err
	}
	if resolved != binding {
		return "", errors.New("review repository context binding is invalid") // refusal:by-design operator-knowledge: the caller supplied a binding the provider-issued handle does not commit to, and only a fresh native transition can supply the exact one
	}
	return root, nil
}

// ResolveReviewRepositoryContextBinding resolves one provider-issued handle
// against a caller-supplied repository and binding, and returns the canonical
// repository root. The handle is a digest over the schema, lineage, target
// identity, revision, and repository identity, so it commits to exactly one
// tuple and proves nothing on its own: the caller must present the same
// repository and the same binding for it to match. A mistyped lineage, target,
// or revision fails the digest instead of silently resolving something else,
// which is the tamper evidence a self-describing token cannot give.
func ResolveReviewRepositoryContextBinding(ctx context.Context, repo, handle string, binding ReviewRepositoryContextBinding) (string, ReviewRepositoryContextBinding, error) {
	root, resolved, err := resolveReviewRepositoryContext(ctx, repo, handle, binding)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, err
	}
	return root, resolved, nil
}

func resolveReviewRepositoryContext(ctx context.Context, repo, handle string, binding ReviewRepositoryContextBinding) (string, ReviewRepositoryContextBinding, error) {
	if !strings.HasPrefix(handle, reviewRepositoryContextV2HandlePrefix) {
		return "", ReviewRepositoryContextBinding{}, errInvalidReviewRepositoryContextV2
	}
	return resolveReviewRepositoryContextV2Token(ctx, repo, handle, binding)
}

// ResolveHistoricalReviewRepositoryContextBinding retains a read-only decoder for
// archived rctx1 locators. Current lifecycle operations intentionally use the
// rctx2-only resolver above, so an old handle cannot activate replacement
// authority or authorize mutation.
func ResolveHistoricalReviewRepositoryContextBinding(ctx context.Context, handle string) (string, ReviewRepositoryContextBinding, error) {
	if !validReviewRepositoryContextHandle(handle) {
		return "", ReviewRepositoryContextBinding{}, errInvalidReviewRepositoryContextV2
	}
	return resolveOpaqueReviewRepositoryContext(ctx, handle)
}

// resolveReviewRepositoryContextLoadedHook is a test-only observation hook for
// the post-load window. Tests replacing this mutable package variable must not
// run in parallel.
var resolveReviewRepositoryContextLoadedHook = func() {}

// resolveOpaqueReviewRepositoryContext proves the private locator still names
// its original Git worktree without reading compact authority or Git content.
func resolveOpaqueReviewRepositoryContext(ctx context.Context, handle string) (string, ReviewRepositoryContextBinding, error) {
	root, record, err := resolveOpaqueReviewRepositoryContextRecord(ctx, handle)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, err
	}
	return root, ReviewRepositoryContextBinding{
		LineageID: record.LineageID, TargetIdentity: record.TargetIdentity, Revision: record.Revision,
	}, nil
}

func resolveTargetedValidationReviewRepositoryContext(ctx context.Context, repo, handle string, requested ReviewRepositoryContextBinding) (string, ReviewRepositoryContextBinding, reviewTargetedValidationContext, error) {
	root, binding, err := resolveReviewRepositoryContextV2Token(ctx, repo, handle, requested)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, reviewTargetedValidationContext{}, err
	}
	store, err := CompactAuthoritativeStore(ctx, root, binding.LineageID)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, reviewTargetedValidationContext{}, errInvalidReviewRepositoryContextV2
	}
	record, err := store.LoadContext(ctx)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, reviewTargetedValidationContext{}, errInvalidReviewRepositoryContextV2
	}
	request, err := BuildTargetedValidationRequest(ctx, root, record.State, binding.Revision)
	if err != nil || request.CorrectionTargetIdentity != binding.TargetIdentity {
		return "", ReviewRepositoryContextBinding{}, reviewTargetedValidationContext{}, errInvalidReviewRepositoryContextV2
	}
	return root, binding, reviewTargetedValidationContext{
		RequestHash: request.RequestHash, CorrectionCandidateTree: request.CorrectionCandidateTree,
	}, nil
}

func resolveOpaqueReviewRepositoryContextRecord(ctx context.Context, handle string) (string, reviewRepositoryContextFile, error) {
	if err := ctx.Err(); err != nil {
		return "", reviewRepositoryContextFile{}, err
	}
	if err := ValidateReviewRepositoryContextHandle(handle); err != nil {
		return "", reviewRepositoryContextFile{}, err
	}
	empty := reviewRepositoryContextFile{}
	path, err := reviewRepositoryContextPath(handle)
	if err != nil {
		return "", empty, err
	}
	home, err := reviewRepositoryContextHome()
	if err != nil {
		return "", empty, err
	}
	storageRoot, err := ensureReviewRepositoryContextStorageRoot(home)
	if err != nil {
		return "", empty, err
	}
	if err := validatePrivateLocatorDirectory(storageRoot, filepath.Dir(path)); err != nil {
		return "", empty, err
	}
	payload, err := readReviewRepositoryContext(path)
	if err != nil {
		return "", empty, err
	}
	var record reviewRepositoryContextFile
	if err := decodeReviewRepositoryContext(payload, &record); err != nil {
		return "", empty, err
	}
	binding := ReviewRepositoryContextBinding{
		LineageID: record.LineageID, TargetIdentity: record.TargetIdentity, Revision: record.Revision,
	}
	if record.Handle != handle {
		return "", empty, errors.New("review repository context binding is invalid") // refusal:by-design world-action: the stored provider-private locator does not name the requested handle, which is provider storage corruption rather than an operator-fixable state
	}
	stored := reviewRepositoryIdentityRecord{
		RepositoryRoot: record.RepositoryRoot, GitCommonDir: record.GitCommonDir,
		GitDir: record.GitDir, RepositoryIdentity: record.RepositoryIdentity,
	}
	// The handle is a digest over the binding and the repository identity, so
	// re-deriving it from the record's own fields is what proves the record was
	// not substituted: a record carrying any other lineage, target, revision, or
	// repository cannot reproduce this handle.
	if reviewRepositoryIdentityHash(stored) != stored.RepositoryIdentity ||
		reviewRepositoryContextHandle(binding, stored) != handle {
		return "", empty, errors.New("review repository context identity is invalid") // refusal:by-design world-action: the provider-private locator no longer re-derives its own handle, which is provider storage corruption rather than an operator-fixable state
	}
	live, err := reviewRepositoryIdentity(ctx, stored.RepositoryRoot)
	if err != nil {
		// Preserve the exact underlying cause behind the unchanged public
		// message. A caller cannot distinguish an environmental refusal that
		// no review action can repair (Git declining the repository outright,
		// for example for ownership reasons) from a genuine identity change
		// once the cause has been flattened into prose.
		return "", empty, &reviewRepositoryContextIdentityError{cause: err}
	}
	if !sameLocatorDirectory(stored.RepositoryRoot, live.RepositoryRoot) ||
		!sameLocatorDirectory(stored.GitCommonDir, live.GitCommonDir) ||
		!sameLocatorDirectory(stored.GitDir, live.GitDir) || live.RepositoryIdentity != stored.RepositoryIdentity {
		return "", empty, errors.New("review repository context identity changed") // refusal:by-design world-action: the bound Git worktree was replaced outside this product and only restoring or re-creating that exact repository resolves it
	}
	return live.RepositoryRoot, record, nil
}

// reviewRepositoryContextIdentityError reports that the repository bound by a
// provider-issued context could not be re-identified. Its message is exactly
// the historical flattened one, so the public failure surface is unchanged,
// while Unwrap keeps the real cause reachable through errors.As.
type reviewRepositoryContextIdentityError struct{ cause error }

func (err *reviewRepositoryContextIdentityError) Error() string {
	// refusal:by-design world-action: same claim as the errors.New twin above -- the bound Git worktree was replaced outside this product and only restoring or re-creating that exact repository resolves it.
	return "review repository context identity changed"
}

func (err *reviewRepositoryContextIdentityError) Unwrap() error { return err.cause }

func validateReviewRepositoryContextRecord(ctx context.Context, repo string, binding ReviewRepositoryContextBinding, record CompactRecord) error {
	if record.State.LineageID != binding.LineageID {
		return errors.New("review repository context is stale or has no live matching authority")
	}
	if record.State.CapturePhaseRevision != binding.Revision {
		return errors.New("review repository context is stale or has no live matching authority")
	}
	switch record.State.State {
	case StateReviewing:
		if record.State.InitialSnapshot.Identity != binding.TargetIdentity {
			return errors.New("review repository context is stale or has no live matching authority")
		}
	case StateValidating:
		if record.State.CurrentSnapshot.Identity != binding.TargetIdentity {
			return errors.New("review repository context is stale or has no live matching authority")
		}
	case StateCorrectionRequired:
		if record.State.ProposedCorrectionLines == nil {
			if record.State.CurrentSnapshot.Identity != binding.TargetIdentity {
				return errors.New("review repository context is stale or has no live matching authority")
			}
			return nil
		}
		correction, err := BuildTargetedValidationRequest(ctx, repo, record.State, record.State.CapturePhaseRevision)
		if err != nil {
			return &reviewRepositoryContextTargetedValidationError{cause: err}
		}
		if correction.CorrectionTargetIdentity != binding.TargetIdentity {
			return errors.New("review repository context is stale or has no live matching authority")
		}
	default:
		return errors.New("review repository context is stale or has no live matching authority")
	}
	return nil
}

// reviewRepositoryContextTargetedValidationError keeps a derivation failure
// inspectable without exposing repository details to provider-facing callers.
type reviewRepositoryContextTargetedValidationError struct{ cause error }

func (err *reviewRepositoryContextTargetedValidationError) Error() string {
	return "review repository context is stale or has no live matching authority"
}

func (err *reviewRepositoryContextTargetedValidationError) Unwrap() error { return err.cause }

func validateReviewRepositoryContextBinding(binding ReviewRepositoryContextBinding) error {
	if validateLineageID(binding.LineageID) != nil || !validSHA256(binding.TargetIdentity) || !validSHA256(binding.Revision) {
		return errors.New("invalid review repository context binding")
	}
	return nil
}

func reviewRepositoryContextHandle(binding ReviewRepositoryContextBinding, identity reviewRepositoryIdentityRecord) string {
	preimage := struct {
		Schema             string `json:"schema"`
		LineageID          string `json:"lineage_id"`
		TargetIdentity     string `json:"target_identity"`
		Revision           string `json:"revision"`
		RepositoryIdentity string `json:"repository_identity"`
	}{
		Schema: ReviewRepositoryContextSchema, LineageID: binding.LineageID,
		TargetIdentity: binding.TargetIdentity, Revision: binding.Revision,
		RepositoryIdentity: identity.RepositoryIdentity,
	}
	payload, _ := json.Marshal(preimage)
	return reviewRepositoryContextHandlePrefix + identityHash(string(payload))
}

func validReviewRepositoryContextHandle(handle string) bool {
	if !strings.HasPrefix(handle, reviewRepositoryContextHandlePrefix) || len(handle) != len(reviewRepositoryContextHandlePrefix)+64 {
		return false
	}
	suffix := strings.TrimPrefix(handle, reviewRepositoryContextHandlePrefix)
	if suffix != strings.ToLower(suffix) {
		return false
	}
	_, err := hex.DecodeString(suffix)
	return err == nil
}

func reviewRepositoryContextPath(handle string) (string, error) {
	if !validReviewRepositoryContextHandle(handle) {
		return "", errors.New("invalid historical review repository context handle") // refusal:by-design operator-knowledge: only archived provider-issued rctx1 handles can be read through this compatibility path
	}
	home, err := reviewRepositoryContextHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gentle-ai", "review-contexts", "v1", handle+".json"), nil
}

func reviewRepositoryContextHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return canonicalLocatorDirectory(home)
}

func ensureReviewRepositoryContextStorageRoot(home string) (string, error) {
	root := filepath.Join(home, ".gentle-ai")
	if !locatorPathWithin(home, root) {
		return "", errors.New("review repository context storage root escapes HOME")
	}
	info, err := os.Lstat(root)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("review repository context storage root is unavailable") // refusal:by-design world-action: restore the archived private locator root before using the read-only compatibility decoder
	}
	return root, nil
}

func readReviewRepositoryContext(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !privateLocatorFileModeSafe(info.Mode()) {
		return nil, errors.New("review repository context is not a private regular file")
	}
	file, err := openReviewRepositoryContext(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !privateLocatorFileModeSafe(opened.Mode()) || !os.SameFile(info, opened) {
		return nil, errors.New("review repository context changed while opening")
	}
	payload, err := io.ReadAll(io.LimitReader(file, reviewRepositoryLocatorMaxBytes+1))
	if err != nil || len(payload) > reviewRepositoryLocatorMaxBytes {
		return nil, errors.New("review repository context is oversized")
	}
	var record reviewRepositoryContextFile
	if err := decodeReviewRepositoryContext(payload, &record); err != nil {
		return nil, err
	}
	return payload, nil
}

func decodeReviewRepositoryContext(payload []byte, target *reviewRepositoryContextFile) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) ||
		target.Schema != ReviewRepositoryContextSchema || !validReviewRepositoryContextHandle(target.Handle) ||
		validateReviewRepositoryContextBinding(ReviewRepositoryContextBinding{
			LineageID: target.LineageID, TargetIdentity: target.TargetIdentity, Revision: target.Revision,
		}) != nil || !validSHA256(target.RepositoryIdentity) || !filepath.IsAbs(target.RepositoryRoot) ||
		!filepath.IsAbs(target.GitCommonDir) || !filepath.IsAbs(target.GitDir) {
		return fs.ErrInvalid
	}
	if targeted := target.TargetedValidation; targeted != nil &&
		(!validSHA256(targeted.RequestHash) || !validGitTree(targeted.CorrectionCandidateTree)) {
		return fs.ErrInvalid
	}
	return nil
}

func reviewRepositoryIdentity(ctx context.Context, repo string) (reviewRepositoryIdentityRecord, error) {
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		return reviewRepositoryIdentityRecord{}, err
	}
	return reviewRepositoryIdentityRecordFromLease(lease), nil
}

func reviewRepositoryIdentityAtRoot(ctx context.Context, root string) (reviewRepositoryIdentityRecord, error) {
	lease, err := openRepositoryIdentityLeaseAtRoot(ctx, root)
	if err != nil {
		return reviewRepositoryIdentityRecord{}, err
	}
	return reviewRepositoryIdentityRecordFromLease(lease), nil
}

func openRepositoryIdentityLeaseAtRoot(ctx context.Context, root string) (*RepositoryIdentityLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rootIdentity, err := captureReviewRepositoryDirectory(root)
	if err != nil {
		return nil, err
	}
	gitControl, err := captureReviewRepositoryControl(
		filepath.Join(rootIdentity.path, ".git"),
		true,
		true,
	)
	if err != nil {
		return nil, err
	}
	directories, err := resolveReviewRepositoryDirectories(ctx, rootIdentity.path)
	if err != nil {
		return nil, err
	}
	top, commonDir, gitDir := directories[0], directories[1], directories[2]
	if !os.SameFile(rootIdentity.info, top.info) {
		return nil, errors.New("repository root identity changed")
	}
	commonControl, err := captureReviewGitCommonDirectoryControl(gitDir, commonDir)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, directory := range []reviewRepositoryDirectoryIdentity{rootIdentity, top, commonDir, gitDir} {
		current, statErr := os.Stat(directory.path)
		if statErr != nil || !current.IsDir() || !os.SameFile(directory.info, current) {
			return nil, errors.New("repository directory identity changed during resolution")
		}
	}
	if err := validateReviewRepositoryControl(gitControl); err != nil {
		return nil, errors.New("repository Git control entry changed during resolution")
	}
	if commonControl != nil {
		if err := validateReviewRepositoryControl(*commonControl); err != nil {
			return nil, errors.New("repository common-directory control entry changed during resolution")
		}
	}
	record := reviewRepositoryIdentityRecord{
		RepositoryRoot: rootIdentity.path,
		GitCommonDir:   commonDir.path,
		GitDir:         gitDir.path,
	}
	record.RepositoryIdentity = reviewRepositoryIdentityHash(record)
	storageKey := strings.TrimPrefix(record.RepositoryIdentity, "sha256:")
	return &RepositoryIdentityLease{
		identity: RepositoryIdentity{
			RepositoryRoot: record.RepositoryRoot,
			GitCommonDir:   record.GitCommonDir,
			GitDir:         record.GitDir,
			RepositoryRef:  record.RepositoryIdentity,
		},
		storageKey:    storageKey,
		root:          rootIdentity,
		commonDir:     commonDir,
		gitDir:        gitDir,
		gitControl:    gitControl,
		commonControl: commonControl,
	}, nil
}

func captureReviewRepositoryDirectory(path string) (reviewRepositoryDirectoryIdentity, error) {
	canonical, err := canonicalLocatorDirectory(path)
	if err != nil {
		return reviewRepositoryDirectoryIdentity{}, err
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return reviewRepositoryDirectoryIdentity{}, errors.New("review repository identity path is not a directory")
	}
	return reviewRepositoryDirectoryIdentity{path: canonical, info: info}, nil
}

func captureReviewRepositoryControl(path string, allowDirectory, allowSymlink bool) (reviewRepositoryControlIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return reviewRepositoryControlIdentity{}, err
	}
	switch {
	case info.Mode().IsRegular():
	case allowDirectory && info.IsDir():
	case allowSymlink && info.Mode()&os.ModeSymlink != 0:
	default:
		return reviewRepositoryControlIdentity{}, errors.New("repository Git control entry has an unsupported type")
	}
	control := reviewRepositoryControlIdentity{path: filepath.Clean(path), info: info}
	if !info.IsDir() {
		control.payload, err = readReviewRepositoryControlPayload(control.path, info)
		if err != nil {
			return reviewRepositoryControlIdentity{}, err
		}
		control.hasPayload = true
	}
	if err := validateReviewRepositoryControl(control); err != nil {
		return reviewRepositoryControlIdentity{}, err
	}
	return control, nil
}

func readReviewRepositoryControlPayload(path string, info fs.FileInfo) ([]byte, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil || len(target) > reviewRepositoryLocatorMaxBytes {
			return nil, errors.New("repository Git control link is invalid")
		}
		current, statErr := os.Lstat(path)
		if statErr != nil || !os.SameFile(info, current) {
			return nil, errors.New("repository Git control link changed while reading")
		}
		return []byte(target), nil
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("repository Git control entry has no bounded payload")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, statErr := file.Stat()
	if statErr != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		_ = file.Close()
		return nil, errors.New("repository Git control file changed while opening")
	}
	payload, readErr := io.ReadAll(io.LimitReader(file, reviewRepositoryLocatorMaxBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(payload) > reviewRepositoryLocatorMaxBytes {
		return nil, errors.New("repository Git control file is unreadable or oversized")
	}
	current, statErr := os.Lstat(path)
	if statErr != nil || !os.SameFile(opened, current) {
		return nil, errors.New("repository Git control file changed while reading")
	}
	return payload, nil
}

func validateReviewRepositoryControl(control reviewRepositoryControlIdentity) error {
	current, err := os.Lstat(control.path)
	if err != nil || current.Mode().Type() != control.info.Mode().Type() ||
		!os.SameFile(control.info, current) {
		return errors.New("repository Git control entry identity changed")
	}
	if control.hasPayload {
		payload, err := readReviewRepositoryControlPayload(control.path, current)
		if err != nil || !bytes.Equal(payload, control.payload) {
			return errors.New("repository Git control entry payload changed")
		}
	}
	return nil
}

func resolveReviewRepositoryDirectory(ctx context.Context, root, selector string) (reviewRepositoryDirectoryIdentity, error) {
	path, err := resolveGitDirectory(ctx, root, selector)
	if err != nil {
		return reviewRepositoryDirectoryIdentity{}, err
	}
	return captureReviewRepositoryDirectory(path)
}

func resolveReviewRepositoryDirectories(ctx context.Context, root string) ([]reviewRepositoryDirectoryIdentity, error) {
	output, err := runGit(ctx, root, nil, nil, "rev-parse", "--show-toplevel", "--git-common-dir", "--git-dir")
	if err != nil {
		// This combined selection includes --show-toplevel, so it is the other
		// boundary a bare repository reaches first.
		return nil, bareRepositoryFailure(ctx, root, err)
	}
	records := bytes.Split(bytes.TrimSuffix(output, []byte{'\n'}), []byte{'\n'})
	if len(records) != 3 {
		return nil, errors.New("Git repository selection must return exactly three path records")
	}
	directories := make([]reviewRepositoryDirectoryIdentity, 3)
	for index, record := range records {
		path, pathErr := canonicalGitDirectory(root, bytes.TrimSuffix(record, []byte{'\r'}))
		if pathErr != nil {
			return nil, pathErr
		}
		directories[index], pathErr = captureReviewRepositoryDirectory(path)
		if pathErr != nil {
			return nil, pathErr
		}
	}
	return directories, nil
}

func captureReviewGitCommonDirectoryControl(
	gitDir,
	commonDir reviewRepositoryDirectoryIdentity,
) (*reviewRepositoryControlIdentity, error) {
	if os.SameFile(gitDir.info, commonDir.info) {
		return nil, nil
	}
	control, err := captureReviewRepositoryControl(
		filepath.Join(gitDir.path, "commondir"),
		false,
		false,
	)
	if err != nil {
		return nil, errors.New("Git common directory relationship is invalid")
	}
	record := append([]byte(nil), control.payload...)
	record = bytes.TrimSuffix(bytes.TrimSuffix(record, []byte{'\n'}), []byte{'\r'})
	if len(record) == 0 || bytes.IndexByte(record, 0) >= 0 || bytes.ContainsAny(record, "\r\n") ||
		strings.TrimSpace(string(record)) == "" || bytes.HasPrefix(record, []byte("--")) {
		return nil, errors.New("Git common directory relationship is invalid")
	}
	path := string(record)
	if !filepath.IsAbs(path) {
		path = filepath.Join(gitDir.path, path)
	}
	resolved, err := captureReviewRepositoryDirectory(path)
	if err != nil || !os.SameFile(resolved.info, commonDir.info) {
		return nil, errors.New("Git common directory relationship is invalid")
	}
	return &control, nil
}

func reviewRepositoryIdentityHash(identity reviewRepositoryIdentityRecord) string {
	preimage := struct {
		RepositoryRoot string `json:"repository_root"`
		GitCommonDir   string `json:"git_common_dir"`
		GitDir         string `json:"git_dir"`
	}{RepositoryRoot: filepath.Clean(identity.RepositoryRoot), GitCommonDir: filepath.Clean(identity.GitCommonDir), GitDir: filepath.Clean(identity.GitDir)}
	payload, _ := json.Marshal(preimage)
	return "sha256:" + identityHash(string(payload))
}

func reviewRepositoryIdentityRecordFromLease(lease *RepositoryIdentityLease) reviewRepositoryIdentityRecord {
	identity := lease.Identity()
	return reviewRepositoryIdentityRecord{
		RepositoryRoot:     identity.RepositoryRoot,
		GitCommonDir:       identity.GitCommonDir,
		GitDir:             identity.GitDir,
		RepositoryIdentity: identity.RepositoryRef,
	}
}

func validRepositoryIdentityLease(lease *RepositoryIdentityLease) bool {
	if lease == nil || !filepath.IsAbs(lease.identity.RepositoryRoot) ||
		!filepath.IsAbs(lease.identity.GitCommonDir) || !filepath.IsAbs(lease.identity.GitDir) ||
		!validSHA256(lease.identity.RepositoryRef) ||
		len(lease.storageKey) != 64 || lease.storageKey != strings.ToLower(lease.storageKey) ||
		lease.identity.RepositoryRef != "sha256:"+lease.storageKey {
		return false
	}
	_, err := hex.DecodeString(lease.storageKey)
	return err == nil
}

func (lease *RepositoryIdentityLease) validateCapturedIdentity() error {
	for _, directory := range []reviewRepositoryDirectoryIdentity{
		lease.root,
		lease.commonDir,
		lease.gitDir,
	} {
		current, err := os.Stat(directory.path)
		if err != nil || !current.IsDir() || !os.SameFile(directory.info, current) {
			return errors.New("repository directory identity changed")
		}
	}
	if err := validateReviewRepositoryControl(lease.gitControl); err != nil {
		return err
	}
	if lease.commonControl != nil {
		if err := validateReviewRepositoryControl(*lease.commonControl); err != nil {
			return err
		}
	}
	return nil
}

func repositoryIdentityChanged(cause error) error {
	if cause == nil {
		return ErrRepositoryIdentityChanged
	}
	return fmt.Errorf("%w: %v", ErrRepositoryIdentityChanged, cause)
}

func canonicalLocatorDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("review repository identity path is not a directory")
	}
	return filepath.Clean(resolved), nil
}

func validatePrivateLocatorDirectory(base, dir string) error {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(filepath.Clean(baseAbs), filepath.Clean(dirAbs))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return errors.New("review repository context directory escapes its private root")
	}
	baseInfo, err := os.Lstat(baseAbs)
	if err != nil || baseInfo.Mode()&os.ModeSymlink != 0 || !baseInfo.IsDir() {
		return errors.New("review repository context private root is unsafe")
	}
	current := filepath.Clean(baseAbs)
	if relative == "." {
		return nil
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" || part == "." || part == ".." {
			return errors.New("review repository context directory is invalid")
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !privateLocatorDirectoryModeSafe(info.Mode()) {
			return errors.New("review repository context directory is unsafe")
		}
	}
	return nil
}

func privateLocatorDirectoryModeSafe(mode fs.FileMode) bool {
	return runtime.GOOS == "windows" || mode.Perm()&0o077 == 0
}

func privateLocatorFileModeSafe(mode fs.FileMode) bool {
	return runtime.GOOS == "windows" || mode.Perm() == 0o600
}

func locatorPathWithin(base, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(base), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func locatorPathStrictlyWithin(base, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(base), filepath.Clean(candidate))
	return err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func sameLocatorDirectory(left, right string) bool {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && leftInfo.IsDir() && rightInfo.IsDir() && os.SameFile(leftInfo, rightInfo)
}

func identityHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
