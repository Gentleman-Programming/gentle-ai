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

type reviewRepositoryContextFile struct {
	Schema             string `json:"schema"`
	Handle             string `json:"handle"`
	LineageID          string `json:"lineage_id"`
	TargetIdentity     string `json:"target_identity"`
	Revision           string `json:"revision"`
	RepositoryIdentity string `json:"repository_identity"`
	RepositoryRoot     string `json:"repository_root"`
	GitCommonDir       string `json:"git_common_dir"`
	GitDir             string `json:"git_dir"`
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

// DeriveReviewRepositoryContextHandle derives the path-free handle that START
// will publish after compact authority creation. It performs no filesystem
// publication and does not treat the handle as authorization.
func DeriveReviewRepositoryContextHandle(ctx context.Context, repo string, binding ReviewRepositoryContextBinding) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateReviewRepositoryContextBinding(binding); err != nil {
		return "", err
	}
	identity, err := reviewRepositoryIdentity(ctx, repo)
	if err != nil {
		return "", err
	}
	return reviewRepositoryContextHandle(binding, identity), nil
}

// ValidateReviewRepositoryContextHandle validates only the opaque transport
// shape. Resolution still performs repository and authority validation.
func ValidateReviewRepositoryContextHandle(handle string) error {
	if !validReviewRepositoryContextHandle(handle) {
		return errors.New("invalid review repository context handle")
	}
	return nil
}

// PublishReviewRepositoryContext publishes a private immutable locator and
// returns its opaque, deterministic handle. The record contains paths only in
// provider-private storage and is not authority.
func PublishReviewRepositoryContext(ctx context.Context, repo string, binding ReviewRepositoryContextBinding) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateReviewRepositoryContextBinding(binding); err != nil {
		return "", err
	}
	identity, err := reviewRepositoryIdentity(ctx, repo)
	if err != nil {
		return "", err
	}
	if err := validateLiveReviewRepositoryContext(ctx, identity.RepositoryRoot, binding); err != nil {
		return "", err
	}
	handle := reviewRepositoryContextHandle(binding, identity)
	path, err := reviewRepositoryContextPath(handle)
	if err != nil {
		return "", err
	}
	home, err := reviewRepositoryContextHome()
	if err != nil {
		return "", err
	}
	storageRoot, err := ensureReviewRepositoryContextStorageRoot(home, true)
	if err != nil {
		return "", err
	}
	if err := ensurePrivateLocatorDirectory(storageRoot, filepath.Dir(path)); err != nil {
		return "", err
	}
	record := reviewRepositoryContextFile{
		Schema: ReviewRepositoryContextSchema, Handle: handle,
		LineageID: binding.LineageID, TargetIdentity: binding.TargetIdentity, Revision: binding.Revision,
		RepositoryIdentity: identity.RepositoryIdentity, RepositoryRoot: identity.RepositoryRoot,
		GitCommonDir: identity.GitCommonDir, GitDir: identity.GitDir,
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := publishReviewRepositoryContext(path, append(payload, '\n')); err != nil {
		return "", err
	}
	return handle, nil
}

// ResolveReviewRepositoryContext resolves one provider-issued handle from any
// process cwd, then revalidates its repository and current compact authority.
func ResolveReviewRepositoryContext(ctx context.Context, handle string, binding ReviewRepositoryContextBinding) (string, error) {
	if err := validateReviewRepositoryContextBinding(binding); err != nil {
		return "", err
	}
	root, resolved, err := ResolveReviewRepositoryContextBinding(ctx, handle)
	if err != nil {
		return "", err
	}
	if resolved != binding {
		return "", errors.New("review repository context binding is invalid") // refusal:by-design operator-knowledge: the caller supplied a binding the provider-issued handle does not commit to, and only a fresh native transition can supply the exact one
	}
	return root, nil
}

// ResolveReviewRepositoryContextBinding resolves one provider-issued handle and
// returns both the repository root and the binding the handle itself commits
// to. The handle is a digest over the schema, lineage, target identity,
// revision, and repository identity (reviewRepositoryContextHandle), and the
// stored record is only accepted when it re-derives that exact handle. A caller
// therefore cannot influence which lineage, target, or revision it receives by
// supplying different fields: there are no fields to supply. That is what lets
// a reviewer-facing surface take one opaque token instead of six values a
// relaying orchestrator could mistype.
func ResolveReviewRepositoryContextBinding(ctx context.Context, handle string) (string, ReviewRepositoryContextBinding, error) {
	root, binding, err := resolveReviewRepositoryContext(ctx, handle)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, err
	}
	return root, binding, nil
}

func resolveReviewRepositoryContext(ctx context.Context, handle string) (string, ReviewRepositoryContextBinding, error) {
	if err := ctx.Err(); err != nil {
		return "", ReviewRepositoryContextBinding{}, err
	}
	if err := ValidateReviewRepositoryContextHandle(handle); err != nil {
		return "", ReviewRepositoryContextBinding{}, err
	}
	empty := ReviewRepositoryContextBinding{}
	path, err := reviewRepositoryContextPath(handle)
	if err != nil {
		return "", empty, err
	}
	home, err := reviewRepositoryContextHome()
	if err != nil {
		return "", empty, err
	}
	storageRoot, err := ensureReviewRepositoryContextStorageRoot(home, false)
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
	if err := validateLiveReviewRepositoryContext(ctx, live.RepositoryRoot, binding); err != nil {
		return "", empty, err
	}
	return live.RepositoryRoot, binding, nil
}

// reviewRepositoryContextIdentityError reports that the repository bound by a
// provider-issued context could not be re-identified. Its message is exactly
// the historical flattened one, so the public failure surface is unchanged,
// while Unwrap keeps the real cause reachable through errors.As.
type reviewRepositoryContextIdentityError struct{ cause error }

func (err *reviewRepositoryContextIdentityError) Error() string {
	return "review repository context identity changed"
}

func (err *reviewRepositoryContextIdentityError) Unwrap() error { return err.cause }

func validateLiveReviewRepositoryContext(ctx context.Context, repo string, binding ReviewRepositoryContextBinding) error {
	store, err := CompactAuthoritativeStore(ctx, repo, binding.LineageID)
	if err != nil {
		return err
	}
	record, err := store.Load()
	if err != nil {
		return err
	}
	if record.Revision != binding.Revision || record.State.LineageID != binding.LineageID {
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
		correction, err := BuildTargetedValidationRequest(ctx, repo, record.State, record.Revision)
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
	if err := ValidateReviewRepositoryContextHandle(handle); err != nil {
		return "", err
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

func ensureReviewRepositoryContextStorageRoot(home string, create bool) (string, error) {
	root := filepath.Join(home, ".gentle-ai")
	if !locatorPathWithin(home, root) {
		return "", errors.New("review repository context storage root escapes HOME")
	}
	info, err := os.Lstat(root)
	if os.IsNotExist(err) && create {
		if err := os.Mkdir(root, 0o700); err != nil && !os.IsExist(err) {
			return "", err
		}
		info, err = os.Lstat(root)
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("review repository context storage root is unsafe")
	}
	if create {
		if err := SyncReviewDirectory(home); err != nil {
			return "", err
		}
	}
	return root, nil
}

func publishReviewRepositoryContext(path string, payload []byte) error {
	existing, err := readReviewRepositoryContext(path)
	if err == nil {
		if !reviewRepositoryContextPayloadEqual(existing, payload) {
			return errors.New("existing review repository context differs")
		}
		return SyncReviewDirectory(filepath.Dir(path))
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".repository-context-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0o600); err == nil {
		_, err = temp.Write(payload)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = PublishFileNoReplace(temp.Name(), path); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	existing, err = readReviewRepositoryContext(path)
	if err != nil || !reviewRepositoryContextPayloadEqual(existing, payload) {
		return errors.New("concurrent review repository context differs")
	}
	return SyncReviewDirectory(filepath.Dir(path))
}

func reviewRepositoryContextPayloadEqual(left, right []byte) bool {
	leftHash, rightHash := sha256.Sum256(left), sha256.Sum256(right)
	return leftHash == rightHash && bytes.Equal(left, right)
}

func readReviewRepositoryContext(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !privateLocatorFileModeSafe(info.Mode()) {
		return nil, errors.New("review repository context is not a private regular file")
	}
	file, err := os.Open(path)
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

func ensurePrivateLocatorDirectory(base, dir string) error {
	return walkPrivateLocatorDirectory(base, dir, true)
}

func validatePrivateLocatorDirectory(base, dir string) error {
	return walkPrivateLocatorDirectory(base, dir, false)
}

func walkPrivateLocatorDirectory(base, dir string, create bool) error {
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
		parent := current
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) && create {
			if err := os.Mkdir(current, 0o700); err != nil && !os.IsExist(err) {
				return err
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !privateLocatorDirectoryModeSafe(info.Mode()) {
			return errors.New("review repository context directory is unsafe")
		}
		if create {
			if err := SyncReviewDirectory(parent); err != nil {
				return err
			}
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
