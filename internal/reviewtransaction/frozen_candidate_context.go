package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	repositoryRoot      string
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
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return FrozenCandidateContext{}, err
	}
	builder.Repo = repo
	if err := builder.ValidateEvidence(ctx, snapshot); err != nil {
		return FrozenCandidateContext{}, fmt.Errorf("validate frozen candidate snapshot: %w", err)
	}
	isolation, cleanup, err := isolatedImmutableTreeGit(ctx, repo)
	if err != nil {
		return FrozenCandidateContext{}, fmt.Errorf("prepare isolated frozen candidate Git view: %w", err)
	}
	defer cleanup()

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
		return FrozenCandidateContext{}, fmt.Errorf("render frozen candidate manifest: %w", err)
	}
	modesByPath, err := parseRawDiffModes(raw)
	if err != nil {
		return FrozenCandidateContext{}, err
	}
	if len(modesByPath) != len(snapshot.Paths) {
		return FrozenCandidateContext{}, errors.New("immutable raw tree diff does not exactly match snapshot paths")
	}

	intended := make(map[string]struct{}, len(snapshot.IntendedUntracked))
	for _, path := range snapshot.IntendedUntracked {
		intended[path] = struct{}{}
	}
	manifest := make([]ChangedPathManifestEntry, 0, len(snapshot.Paths))
	for _, path := range snapshot.Paths {
		modes, ok := modesByPath[path]
		if !ok {
			return FrozenCandidateContext{}, fmt.Errorf("immutable snapshot path %q is missing from raw tree diff", path)
		}
		_, wasIntendedUntracked := intended[path]
		entry := ChangedPathManifestEntry{
			Path: path, Status: modes.status, OldMode: modes.oldMode, NewMode: modes.newMode,
			Deleted: modes.status == CandidatePathDeleted, TypeChanged: modes.status == CandidatePathTypeChanged,
			ModeOnly:          modes.status == CandidatePathModified && modes.oldObject == modes.newObject && modes.oldMode != modes.newMode,
			IntendedUntracked: wasIntendedUntracked,
		}
		if err := validateChangedPathManifestEntry(entry); err != nil {
			return FrozenCandidateContext{}, err
		}
		manifest = append(manifest, entry)
	}
	return FrozenCandidateContext{
		BaseTree: snapshot.BaseTree, CandidateTree: snapshot.CandidateTree,
		ChangedPathManifest: manifest, repositoryRoot: repo,
	}, nil
}

// InspectCandidate renders one bounded, read-only frozen-candidate view; path operations select only by canonical manifest index.
func (builder SnapshotBuilder) InspectCandidate(ctx context.Context, snapshot Snapshot, operation string, pathIndex int, side string) ([]byte, error) {
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	frozen, err := builder.FrozenCandidateContext(ctx, snapshot)
	if err != nil {
		return nil, err
	}
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

	isolation, cleanup, err := isolatedImmutableTreeGit(ctx, repo)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	common := []string{"--no-pager", "-c", "color.ui=false", "-c", "core.attributesFile=" + os.DevNull, "-c", "diff.external="}
	var args []string
	switch operation {
	case "name-status", "numstat":
		args = append(common, "diff", "--"+operation, "--text", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=none", frozen.BaseTree, frozen.CandidateTree, "--")
	case "stat":
		path := ":(literal)" + frozen.ChangedPathManifest[pathIndex].Path
		args = append(common, "diff", "--stat", "--text", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=none", frozen.BaseTree, frozen.CandidateTree, "--", path)
	case "patch":
		path := ":(literal)" + frozen.ChangedPathManifest[pathIndex].Path
		args = append(common, "diff", "--patch", "--text", "--full-index", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames", "--diff-algorithm=myers", "--no-indent-heuristic", "--unified=3", "--ignore-submodules=none", frozen.BaseTree, frozen.CandidateTree, "--", path)
	case "object":
		tree := frozen.CandidateTree
		if side == "base" {
			tree = frozen.BaseTree
		}
		args = append(common, "cat-file", "-p", tree+":"+frozen.ChangedPathManifest[pathIndex].Path)
	default:
		return nil, fmt.Errorf("unknown candidate inspection operation %q", operation) // refusal:by-design operator-knowledge: the native CLI validates the closed operation enum before calling this boundary
	}
	return runGitLimited(ctx, repo, isolation, nil, MaxFrozenCandidateDiffBytes, args...)
}

func isolatedImmutableTreeGit(ctx context.Context, repo string) ([]string, func(), error) {
	identity, err := reviewRepositoryIdentity(ctx, repo)
	if err != nil {
		return nil, func() {}, err
	}
	objectFormatOutput, err := runGit(ctx, identity.RepositoryRoot, nil, nil, "rev-parse", "--show-object-format")
	if err != nil {
		return nil, func() {}, err
	}
	objectFormat := strings.TrimSpace(string(objectFormatOutput))
	if objectFormat != "sha1" && objectFormat != "sha256" {
		return nil, func() {}, fmt.Errorf("unsupported Git object format %q", objectFormat)
	}
	// The repository Git directory is the reliable writable location when a
	// sandboxed caller does not expose an accessible process temp directory.
	gitDir, err := os.MkdirTemp(identity.GitDir, ".gentle-ai-frozen-git-*")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(gitDir) }
	for _, dir := range []string{"objects", "refs"} {
		if err := os.Mkdir(filepath.Join(gitDir, dir), 0o700); err != nil {
			cleanup()
			return nil, func() {}, err
		}
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/frozen-context\n"), 0o600); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	repositoryFormatVersion := "0"
	extensions := ""
	if objectFormat == "sha256" {
		repositoryFormatVersion = "1"
		extensions = "[extensions]\n\tobjectFormat = sha256\n"
	}
	config := "[core]\n\trepositoryFormatVersion = " + repositoryFormatVersion + "\n\tbare = true\n" + extensions
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o600); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return []string{
		"GIT_DIR=" + gitDir,
		"GIT_OBJECT_DIRECTORY=" + filepath.Join(identity.GitCommonDir, "objects"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=" + os.DevNull,
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_COUNT=0",
		"GIT_ATTR_NOSYSTEM=1",
		"LANG=C",
	}, cleanup, nil
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
