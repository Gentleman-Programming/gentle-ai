package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

const (
	FrozenCandidateDiffEncodingBase64 = "base64"
	MaxFrozenCandidateDiffBytes       = 4 << 20
	MaxFrozenCandidateDiffBase64Bytes = 5_592_408
	maxFrozenCandidateManifestBytes   = 4 << 20
)

// FrozenCandidateDiff is retained only for negotiated v1 compatibility.
// Provider-managed native-Git reviewers never receive this payload.
type FrozenCandidateDiff struct {
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
	SHA256   string `json:"sha256"`
	ByteSize int    `json:"byte_size"`
}

func NewFrozenCandidateDiff(payload []byte) (FrozenCandidateDiff, error) {
	if len(payload) > MaxFrozenCandidateDiffBytes {
		return FrozenCandidateDiff{}, &GitOutputLimitError{Args: []string{"diff"}, Limit: MaxFrozenCandidateDiffBytes, Actual: len(payload)}
	}
	digest := sha256.Sum256(payload)
	return FrozenCandidateDiff{
		Encoding: FrozenCandidateDiffEncodingBase64, Data: base64.StdEncoding.EncodeToString(payload),
		SHA256: fmt.Sprintf("sha256:%x", digest), ByteSize: len(payload),
	}, nil
}

func (diff FrozenCandidateDiff) Bytes() ([]byte, error) {
	if diff.Encoding != FrozenCandidateDiffEncodingBase64 || diff.ByteSize < 0 || diff.ByteSize > MaxFrozenCandidateDiffBytes ||
		len(diff.Data) > MaxFrozenCandidateDiffBase64Bytes || len(diff.Data) != base64.StdEncoding.EncodedLen(diff.ByteSize) {
		return nil, errors.New("invalid frozen candidate diff metadata")
	}
	payload, err := base64.StdEncoding.Strict().DecodeString(diff.Data)
	if err != nil || base64.StdEncoding.EncodeToString(payload) != diff.Data {
		return nil, errors.New("frozen candidate diff data is not canonical base64")
	}
	if len(payload) != diff.ByteSize {
		return nil, errors.New("frozen candidate diff byte size does not match data")
	}
	digest := sha256.Sum256(payload)
	if diff.SHA256 != fmt.Sprintf("sha256:%x", digest) {
		return nil, errors.New("frozen candidate diff digest does not match data")
	}
	return payload, nil
}

// CandidatePathStatus is the stable public status of one immutable tree-diff
// entry. Renames are deliberately disabled, so the canonical surface contains
// only additions, deletions, modifications, and type changes.
type CandidatePathStatus string

const (
	CandidatePathAdded       CandidatePathStatus = "A"
	CandidatePathDeleted     CandidatePathStatus = "D"
	CandidatePathModified    CandidatePathStatus = "M"
	CandidatePathTypeChanged CandidatePathStatus = "T"
)

// ChangedPathManifestEntry describes one path in a frozen candidate. Paths are
// repository-relative and entries retain the persisted Snapshot.Paths order.
type ChangedPathManifestEntry struct {
	Path              string              `json:"path"`
	Status            CandidatePathStatus `json:"status"`
	OldMode           string              `json:"old_mode"`
	NewMode           string              `json:"new_mode"`
	Deleted           bool                `json:"deleted"`
	TypeChanged       bool                `json:"type_changed"`
	ModeOnly          bool                `json:"mode_only"`
	IntendedUntracked bool                `json:"intended_untracked"`
}

// FrozenCandidateContext is the deterministic reviewer input derived only
// from the immutable Git trees and metadata persisted in a Snapshot.
// Its repositoryRoot is provider-only local state, not reviewer-visible input.
type FrozenCandidateContext struct {
	BaseTree            string
	CandidateTree       string
	LegacyCandidateDiff *FrozenCandidateDiff
	ChangedPathManifest []ChangedPathManifestEntry
	// RenamePairs is the rename pairing frozenRenamePairs computed once
	// during THIS inspector's own preparation (#4107/#3208): it is exactly
	// what this same inspector's rename-aware patch reads used, so a
	// reviewer or context builder inspecting this frozen candidate sees the
	// pairing its own patches were built from. It is not synchronized with
	// any other caller's separate invocation of the same derivation (for
	// example ChangedLines' rename-aware sizing, computed independently,
	// ordinarily at a different time for a different purpose) -- on the
	// ordinary path both agree because the derivation is deterministic over
	// the same immutable trees, but each has its own failure mode.
	RenamePairs map[string]renamePairInfo
	// RenamePairingDegraded is true when the rename pairing lookup itself
	// failed for this boundary; RenamePairs is then empty and every rename
	// pair fell back to the conservative --no-renames read.
	RenamePairingDegraded bool
	repositoryRoot        string
}

type PreparedCandidateInspector struct {
	frozen         FrozenCandidateContext
	isolation      []string
	attributesFile string
	cleanup        func() error
	closed         bool
	// renamePartners maps one manifest path to the other side of a
	// git-detected rename pair (#3208). The manifest itself stays
	// rename-disabled -- Status is never "R" -- so this is used only to size
	// the per-path "patch" read: a moved file's two manifest entries would
	// otherwise each materialize the entire file (a full delete, a full
	// insert) instead of the small change git's own diff reports for it.
	renamePartners map[string]string
	// inspectionCache memoizes each successful Inspect result by operation,
	// path index, and side. base_tree/candidate_tree are immutable for the
	// lifetime of one inspector, so a repeated identical read always returns
	// byte-identical output. STATUS's lens-context budget probe
	// (reviewLensContextBudgetProbe, issues #3733/#3871) reuses one inspector
	// across every selected lens and re-renders the complete candidate --
	// two discovery reads plus one patch read per changed path -- once per
	// lens, turning a bounded read-only STATUS into an O(lenses x paths)
	// subprocess cost that scales with candidate size. Serving a repeated
	// call from this cache instead of re-invoking Git collapses that back to
	// O(paths), the cost one full pass over the candidate always required.
	// Entries are private to the cache: Inspect returns a copy so a caller
	// that mutates its slice cannot corrupt a later lens's view, and
	// inspectionMu guards the map because one inspector is shared across
	// lenses that may run concurrently.
	inspectionCache map[string][]byte
	inspectionMu    sync.Mutex
}

// WithLegacyCandidateDiff adds the exact published v1 candidate transport.
// Callers use it only after explicitly negotiating the legacy contract.
func (builder SnapshotBuilder) WithLegacyCandidateDiff(ctx context.Context, snapshot Snapshot, frozen FrozenCandidateContext) (FrozenCandidateContext, error) {
	if frozen.BaseTree != snapshot.BaseTree || frozen.CandidateTree != snapshot.CandidateTree {
		return FrozenCandidateContext{}, errors.New("legacy candidate transport does not match frozen trees") // refusal:by-design world-action: provider code mixed immutable contexts and must be fixed before retry
	}
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return FrozenCandidateContext{}, err
	}
	isolation, cleanup, err := isolatedImmutableTreeGit(ctx, repo)
	if err != nil {
		return FrozenCandidateContext{}, err
	}
	defer cleanup()
	payload, err := runGitLimited(ctx, repo, isolation, nil, MaxFrozenCandidateDiffBytes,
		"diff", "--binary", "--full-index", "--no-color", "--no-renames", "--no-ext-diff", "--no-textconv",
		"--diff-algorithm=myers", "--no-indent-heuristic", "--unified=3", "--ignore-submodules=none",
		"--src-prefix=a/", "--dst-prefix=b/", snapshot.BaseTree, snapshot.CandidateTree, "--")
	if err != nil {
		return FrozenCandidateContext{}, fmt.Errorf("render legacy frozen candidate diff: %w", err)
	}
	diff, err := NewFrozenCandidateDiff(payload)
	if err != nil {
		return FrozenCandidateContext{}, err
	}
	frozen.LegacyCandidateDiff = &diff
	return frozen, nil
}

// FrozenCandidateContext returns immutable Git tree references and their typed
// path manifest. Reviewers read only path-scoped diffs between these trees.
func (builder SnapshotBuilder) FrozenCandidateContext(ctx context.Context, snapshot Snapshot) (FrozenCandidateContext, error) {
	inspector, err := builder.PrepareCandidateInspector(ctx, snapshot)
	if err != nil {
		return FrozenCandidateContext{}, err
	}
	frozen := inspector.FrozenCandidateContext()
	if err := inspector.Close(); err != nil {
		return FrozenCandidateContext{}, err
	}
	return frozen, nil
}

func (builder SnapshotBuilder) PrepareCandidateInspector(ctx context.Context, snapshot Snapshot) (*PreparedCandidateInspector, error) {
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	builder.Repo = repo
	if err := builder.ValidateEvidence(ctx, snapshot); err != nil {
		return nil, fmt.Errorf("validate frozen candidate snapshot: %w", err)
	}
	isolation, attributesFile, cleanup, err := isolatedImmutableTreeGitWithAttributesFile(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("prepare isolated frozen candidate Git view: %w", err)
	}
	fail := func(err error) (*PreparedCandidateInspector, error) {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return nil, errors.Join(err, fmt.Errorf("clean up prepared frozen candidate Git view: %w", cleanupErr))
		}
		return nil, err
	}

	raw, err := runGitLimited(ctx, repo, isolation, nil, maxFrozenCandidateManifestBytes,
		"diff",
		"--raw",
		"-z",
		"--full-index",
		"--no-renames",
		"--no-ext-diff",
		"--no-textconv",
		"--ignore-submodules=none",
		snapshot.BaseTree,
		snapshot.CandidateTree,
		"--",
	)
	if err != nil {
		return fail(fmt.Errorf("render frozen candidate manifest: %w", err))
	}
	modesByPath, err := parseRawDiffModes(raw)
	if err != nil {
		return fail(err)
	}
	if len(modesByPath) != len(snapshot.Paths) {
		return fail(errors.New("immutable raw tree diff does not exactly match snapshot paths"))
	}

	intended := make(map[string]struct{}, len(snapshot.IntendedUntracked))
	for _, path := range snapshot.IntendedUntracked {
		intended[path] = struct{}{}
	}
	manifest := make([]ChangedPathManifestEntry, 0, len(snapshot.Paths))
	for _, path := range snapshot.Paths {
		modes, ok := modesByPath[path]
		if !ok {
			return fail(fmt.Errorf("immutable snapshot path %q is missing from raw tree diff", path))
		}
		_, wasIntendedUntracked := intended[path]
		entry := ChangedPathManifestEntry{
			Path: path, Status: modes.status, OldMode: modes.oldMode, NewMode: modes.newMode,
			Deleted: modes.status == CandidatePathDeleted, TypeChanged: modes.status == CandidatePathTypeChanged,
			ModeOnly:          modes.status == CandidatePathModified && modes.oldObject == modes.newObject && modes.oldMode != modes.newMode,
			IntendedUntracked: wasIntendedUntracked,
		}
		if err := validateChangedPathManifestEntry(entry); err != nil {
			return fail(err)
		}
		manifest = append(manifest, entry)
	}
	// Rename pairing is computed exactly once for THIS inspector's own
	// preparation, via the single canonical derivation (frozenRenamePairs)
	// ChangedLines' rename-aware sizing also calls -- so the two can never
	// disagree on the pairing LOGIC, though each still runs its own git
	// invocation and does not observe the other's result (#4107/#3208). A
	// failed lookup here degrades this inspector's own pairing to empty --
	// recorded observably as RenamePairingDegraded so a reviewer/context
	// builder can see it -- with every path in THIS inspector falling back
	// to the plain --no-renames read, rather than aborting preparation.
	renamePairs, renameDegraded := frozenRenamePairs(ctx, repo, isolation, snapshot.BaseTree, snapshot.CandidateTree)
	renamePartners := renamePartnersFromManifest(renamePairs, manifest)
	return &PreparedCandidateInspector{
		frozen: FrozenCandidateContext{
			BaseTree: snapshot.BaseTree, CandidateTree: snapshot.CandidateTree,
			ChangedPathManifest: manifest, repositoryRoot: repo,
			RenamePairs: renamePairs, RenamePairingDegraded: renameDegraded,
		},
		isolation: isolation, attributesFile: attributesFile, cleanup: cleanup,
		renamePartners: renamePartners,
	}, nil
}

// renamePartnersFromManifest narrows an already-computed rename pairing (see
// frozenRenamePairs) to the pairs this exact manifest recognizes as a clean
// delete/add pair (issue #3208's counterpart to #4107): it performs no Git
// invocation of its own, and the manifest's paths and rename-disabled Status
// enum are never touched.
func renamePartnersFromManifest(pairs map[string]renamePairInfo, manifest []ChangedPathManifestEntry) map[string]string {
	statusByPath := make(map[string]CandidatePathStatus, len(manifest))
	for _, entry := range manifest {
		statusByPath[entry.Path] = entry.Status
	}
	partners := make(map[string]string, len(pairs))
	for path, info := range pairs {
		if statusByPath[path] != CandidatePathDeleted || statusByPath[info.partner] != CandidatePathAdded {
			continue
		}
		partners[path] = info.partner
		partners[info.partner] = path
	}
	return partners
}

// FrozenCandidateContext returns a copy that cannot mutate later inspection scope.
func (inspector *PreparedCandidateInspector) FrozenCandidateContext() FrozenCandidateContext {
	frozen := inspector.frozen
	frozen.ChangedPathManifest = append(frozen.ChangedPathManifest[:0:0], frozen.ChangedPathManifest...)
	// RenamePairs is a map: the struct copy above shares it with
	// inspector.frozen unless cloned here, which would let a caller mutating
	// the returned value corrupt this inspector's own recorded pairing.
	frozen.RenamePairs = maps.Clone(frozen.RenamePairs)
	return frozen
}

// Inspect renders one bounded view selected only by canonical manifest index.
func (inspector *PreparedCandidateInspector) Inspect(ctx context.Context, operation string, pathIndex int, side string) ([]byte, error) {
	if inspector == nil || inspector.closed {
		return nil, errors.New("prepared candidate inspector is closed") // refusal:by-design operator-knowledge: provider code must retain and use its invocation-owned inspector before closing it
	}
	frozen := inspector.frozen
	pathOperation := operation == "stat" || operation == "patch" || operation == "object"
	if pathOperation != (pathIndex >= 0) || pathIndex >= len(frozen.ChangedPathManifest) {
		return nil, errors.New("candidate inspection operation requires its exact canonical path index") // refusal:by-design operator-knowledge: the native CLI validates this closed combination before calling the provider boundary
	}
	if operation == "object" {
		if side != "base" && side != "candidate" {
			return nil, errors.New("candidate object inspection requires side base or candidate") // refusal:by-design operator-knowledge: the native CLI validates this closed enum before calling the provider boundary
		}
	} else if side != "" {
		return nil, errors.New("candidate inspection side is valid only for object content") // refusal:by-design operator-knowledge: reaching this means provider code bypassed the validated native CLI contract
	}

	cacheKey := operation + "\x00" + strconv.Itoa(pathIndex) + "\x00" + side
	inspector.inspectionMu.Lock()
	cached, hit := inspector.inspectionCache[cacheKey]
	inspector.inspectionMu.Unlock()
	if hit {
		return bytes.Clone(cached), nil
	}

	common := []string{"--no-pager", "-c", "color.ui=false", "-c", "core.attributesFile=" + inspector.attributesFile, "-c", "diff.external="}
	var args []string
	switch operation {
	case "name-status", "numstat":
		args = append(common, "diff", "--"+operation, "--text", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=none", frozen.BaseTree, frozen.CandidateTree, "--")
	case "stat":
		path := ":(literal)" + frozen.ChangedPathManifest[pathIndex].Path
		args = append(common, "diff", "--stat", "--text", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=none", frozen.BaseTree, frozen.CandidateTree, "--", path)
	case "patch":
		// This is the one operation that emits candidate content, and it
		// deliberately does NOT force --text. The discovery operations above do,
		// because they emit counts and names whose determinism is worth pinning;
		// here the same flag would override Git's binary classification and
		// spill a blob's raw bytes into a reviewer prompt.
		//
		// That costs twice. A lens holds no tools and cannot skip what its
		// prompt contains, so one candidate carrying a PDF filled a lens prompt
		// with roughly 114K tokens of bytes no text reviewer can act on. And
		// arbitrary content bytes are arbitrary control material downstream: an
		// `@\` sequence inside one blob made a host's file-mention resolver
		// staple a drive-root attachment onto every lens launch (issue #3193).
		//
		// Without it Git reports "Binary files a/... and b/... differ", which
		// still names the path and still proves it changed, so the empty-patch
		// refusal in the lens-context surface stays satisfied. Determinism is
		// preserved by the isolation this inspector already installs: an empty
		// attributes file plus GIT_ATTR_NOSYSTEM, so neither the repository's
		// .gitattributes nor the machine's can move the classification.
		entryPath := frozen.ChangedPathManifest[pathIndex].Path
		renameFlag, paths := "--no-renames", []string{":(literal)" + entryPath}
		// A git-detected rename charges this read as one small change-inside
		// -the-moved-file patch instead of a full delete or full insert, so a
		// candidate dominated by pure moves stays inside the reviewer context
		// byte budget (#3208). Both manifest entries in the pair render the
		// same rename patch; each is still tagged with its own path above.
		if partner, ok := inspector.renamePartners[entryPath]; ok {
			renameFlag, paths = "-M", []string{":(literal)" + entryPath, ":(literal)" + partner}
		}
		args = append(common, "diff", "--patch", "--full-index", "--no-color", "--no-ext-diff", "--no-textconv", renameFlag, "--diff-algorithm=myers", "--no-indent-heuristic", "--unified=3", "--ignore-submodules=none", frozen.BaseTree, frozen.CandidateTree, "--")
		args = append(args, paths...)
	case "object":
		tree := frozen.CandidateTree
		if side == "base" {
			tree = frozen.BaseTree
		}
		args = append(common, "cat-file", "-p", tree+":"+frozen.ChangedPathManifest[pathIndex].Path)
	default:
		return nil, fmt.Errorf("unknown candidate inspection operation %q", operation) // refusal:by-design operator-knowledge: the native CLI validates the closed operation enum before calling this boundary
	}
	payload, err := runGitLimited(ctx, frozen.repositoryRoot, inspector.isolation, nil, MaxFrozenCandidateDiffBytes, args...)
	if err != nil {
		return nil, err
	}
	inspector.inspectionMu.Lock()
	if inspector.inspectionCache == nil {
		inspector.inspectionCache = make(map[string][]byte, len(frozen.ChangedPathManifest))
	}
	inspector.inspectionCache[cacheKey] = bytes.Clone(payload)
	inspector.inspectionMu.Unlock()
	return payload, nil
}

func (inspector *PreparedCandidateInspector) Close() error {
	if inspector == nil || inspector.closed {
		return nil
	}
	inspector.closed = true
	if inspector.cleanup == nil {
		return nil
	}
	return inspector.cleanup()
}

// InspectCandidate preserves the one-shot inspection boundary.
func (builder SnapshotBuilder) InspectCandidate(ctx context.Context, snapshot Snapshot, operation string, pathIndex int, side string) ([]byte, error) {
	inspector, err := builder.PrepareCandidateInspector(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	payload, inspectErr := inspector.Inspect(ctx, operation, pathIndex, side)
	if cleanupErr := inspector.Close(); cleanupErr != nil {
		if inspectErr != nil {
			return nil, errors.Join(inspectErr, cleanupErr)
		}
		return nil, cleanupErr
	}
	return payload, inspectErr
}

// gitShowObjectFormatUnsupported caches, for the rest of this process,
// whether the installed git predates 2.38's `rev-parse --show-object-format`
// (#3541): that git echoes the unrecognized flag back instead of failing,
// which would otherwise be misread as the object format on every call.
var (
	gitShowObjectFormatMu          sync.Mutex
	gitShowObjectFormatUnsupported bool
)

// gitObjectFormat determines a repository's object hash algorithm across the
// git version gap #3541 reports. git >= 2.38 answers directly. Older git
// echoes the unrecognized flag back verbatim (the same "unsupported option
// echo" shape canonicalGitDirectory already guards against for
// --path-format=absolute), recognized here by its leading "--", and degrades
// to the fallback below, caching the negative result.
func gitObjectFormat(ctx context.Context, repo string) (string, error) {
	gitShowObjectFormatMu.Lock()
	unsupported := gitShowObjectFormatUnsupported
	gitShowObjectFormatMu.Unlock()
	if !unsupported {
		output, err := runGit(ctx, repo, nil, nil, "rev-parse", "--show-object-format")
		if err != nil {
			return "", err
		}
		format := strings.TrimSpace(string(output))
		switch {
		case format == "sha1" || format == "sha256":
			return format, nil
		case strings.HasPrefix(format, "--"):
			gitShowObjectFormatMu.Lock()
			gitShowObjectFormatUnsupported = true
			gitShowObjectFormatMu.Unlock()
		default:
			return "", fmt.Errorf("unsupported Git object format %q", format)
		}
	}
	return legacyGitObjectFormat(ctx, repo)
}

// legacyGitObjectFormat determines the object format for git < 2.38, which
// has no --show-object-format flag. A SHA-256 repository predates that flag
// too (git init --object-format=sha256, supported since git 2.29) and
// records its choice in extensions.objectformat; every other repository is
// sha1, the only format that existed before that extension did.
func legacyGitObjectFormat(ctx context.Context, repo string) (string, error) {
	output, err := runGit(ctx, repo, nil, nil, "config", "--get", "extensions.objectformat")
	if err != nil {
		var commandErr *GitCommandError
		if errors.As(err, &commandErr) && commandErr.ExitCode == 1 {
			// `git config --get` exits 1 when the key is unset; on git that
			// predates extensions.objectformat entirely, unset IS sha1.
			return "sha1", nil
		}
		return "", err
	}
	format := strings.ToLower(strings.TrimSpace(string(output)))
	if format == "" {
		format = "sha1"
	}
	if format != "sha1" && format != "sha256" {
		return "", fmt.Errorf("unsupported Git object format %q", format)
	}
	return format, nil
}

func isolatedImmutableTreeGit(ctx context.Context, repo string) ([]string, func() error, error) {
	isolation, _, cleanup, err := isolatedImmutableTreeGitWithAttributesFile(ctx, repo)
	return isolation, cleanup, err
}

func isolatedImmutableTreeGitWithAttributesFile(ctx context.Context, repo string) ([]string, string, func() error, error) {
	identity, err := reviewRepositoryIdentity(ctx, repo)
	if err != nil {
		return nil, "", func() error { return nil }, err
	}
	objectFormat, err := gitObjectFormat(ctx, identity.RepositoryRoot)
	if err != nil {
		return nil, "", func() error { return nil }, err
	}
	// The repository Git directory is the reliable writable location when a
	// sandboxed caller does not expose an accessible process temp directory.
	gitDir, err := os.MkdirTemp(identity.GitDir, ".gentle-ai-frozen-git-*")
	if err != nil {
		return nil, "", func() error { return nil }, err
	}
	cleanup := func() error { return os.RemoveAll(gitDir) }
	fail := func(err error) ([]string, string, func() error, error) {
		if cleanupErr := cleanup(); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("clean up isolated frozen candidate Git view: %w", cleanupErr))
		}
		return nil, "", func() error { return nil }, err
	}
	for _, dir := range []string{"objects", "refs"} {
		if err := os.Mkdir(filepath.Join(gitDir, dir), 0o700); err != nil {
			return fail(err)
		}
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/frozen-context\n"), 0o600); err != nil {
		return fail(err)
	}
	repositoryFormatVersion := "0"
	extensions := ""
	if objectFormat == "sha256" {
		repositoryFormatVersion = "1"
		extensions = "[extensions]\n\tobjectFormat = sha256\n"
	}
	config := "[core]\n\trepositoryFormatVersion = " + repositoryFormatVersion + "\n\tbare = true\n" + extensions
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o600); err != nil {
		return fail(err)
	}
	emptyFiles := make([]string, 0, 3)
	for _, prefix := range []string{"git-iso-system-*", "git-iso-global-*", "git-iso-attrs-*"} {
		file, err := os.CreateTemp(gitDir, prefix)
		if err != nil {
			return fail(err)
		}
		if err := file.Close(); err != nil {
			return fail(err)
		}
		emptyFiles = append(emptyFiles, file.Name())
	}
	return []string{
		"GIT_DIR=" + gitDir,
		"GIT_OBJECT_DIRECTORY=" + filepath.Join(identity.GitCommonDir, "objects"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=" + emptyFiles[0],
		"GIT_CONFIG_GLOBAL=" + emptyFiles[1],
		"GIT_CONFIG_COUNT=0",
		"GIT_ATTR_NOSYSTEM=1",
		"LANG=C",
	}, emptyFiles[2], cleanup, nil
}

func validateChangedPathManifestEntry(entry ChangedPathManifestEntry) error {
	if entry.Path == "" || !validRawGitMode(entry.OldMode) || !validRawGitMode(entry.NewMode) {
		return fmt.Errorf("invalid frozen candidate manifest entry for %q", entry.Path)
	}
	switch entry.Status {
	case CandidatePathAdded:
		if entry.OldMode != "000000" || entry.NewMode == "000000" || entry.Deleted || entry.TypeChanged || entry.ModeOnly {
			return fmt.Errorf("invalid added frozen candidate manifest entry for %q", entry.Path)
		}
	case CandidatePathDeleted:
		if entry.OldMode == "000000" || entry.NewMode != "000000" || !entry.Deleted || entry.TypeChanged || entry.ModeOnly {
			return fmt.Errorf("invalid deleted frozen candidate manifest entry for %q", entry.Path)
		}
	case CandidatePathModified:
		if entry.OldMode == "000000" || entry.NewMode == "000000" || entry.Deleted || entry.TypeChanged {
			return fmt.Errorf("invalid modified frozen candidate manifest entry for %q", entry.Path)
		}
	case CandidatePathTypeChanged:
		if entry.OldMode == "000000" || entry.NewMode == "000000" || entry.Deleted || !entry.TypeChanged || entry.ModeOnly {
			return fmt.Errorf("invalid type-changed frozen candidate manifest entry for %q", entry.Path)
		}
	default:
		return fmt.Errorf("unsupported frozen candidate status %q for %q", entry.Status, entry.Path)
	}
	return nil
}

// ValidateChangedPathManifest verifies the public manifest shape and its
// canonical repository-relative ordering independently of any live workspace.
func ValidateChangedPathManifest(entries []ChangedPathManifestEntry) error {
	if entries == nil {
		return errors.New("frozen candidate changed-path manifest is missing")
	}
	paths := make([]string, len(entries))
	for index, entry := range entries {
		if err := validateChangedPathManifestEntry(entry); err != nil {
			return err
		}
		paths[index] = entry.Path
	}
	canonical, err := canonicalPaths(paths)
	if err != nil || !reflect.DeepEqual(canonical, paths) {
		return errors.New("frozen candidate changed-path manifest is not canonical")
	}
	return nil
}
