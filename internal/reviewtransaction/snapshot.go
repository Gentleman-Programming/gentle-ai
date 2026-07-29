package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathidentity"
)

type TargetKind string

type Projection string

const (
	TargetCurrentChanges       TargetKind = "current-changes"
	TargetBaseDiff             TargetKind = "base-diff"
	TargetBaseWorkspaceOverlay TargetKind = "base-workspace-overlay"
	TargetExactRevision        TargetKind = "commit-range"
	TargetFixDiff              TargetKind = "fix-diff"

	ProjectionWorkspace Projection = "workspace"
	ProjectionStaged    Projection = "staged"
)

type Target struct {
	Kind              TargetKind `json:"kind"`
	Projection        Projection `json:"projection,omitempty"`
	BaseRef           string     `json:"base_ref,omitempty"`
	Revision          string     `json:"revision,omitempty"`
	IntendedUntracked []string   `json:"intended_untracked"`
	LedgerIDs         []string   `json:"ledger_ids,omitempty"`
}

type Snapshot struct {
	Kind                   TargetKind `json:"kind"`
	Projection             Projection `json:"projection,omitempty"`
	UnbornHead             bool       `json:"unborn_head,omitempty"`
	BaseTree               string     `json:"base_tree"`
	CandidateTree          string     `json:"candidate_tree"`
	PathsDigest            string     `json:"paths_digest"`
	IntendedUntracked      []string   `json:"intended_untracked"`
	IntendedUntrackedProof string     `json:"intended_untracked_proof"`
	LedgerIDs              []string   `json:"ledger_ids,omitempty"`
	Paths                  []string   `json:"paths"`
	Identity               string     `json:"identity"`
}

type SnapshotBuilder struct {
	Repo       string
	unbornHead bool
}

var exactObjectPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}(?:[0-9a-fA-F]{24})?$`)

func (builder SnapshotBuilder) Build(ctx context.Context, target Target) (Snapshot, error) {
	return builder.build(ctx, target, false)
}

// BuildStagedWorkspaceOverlayRecovery freezes the exact real index for the
// single recovery-only staged overlay transition. Ordinary START keeps using
// Build, so this representation cannot create fresh authority directly.
func (builder SnapshotBuilder) BuildStagedWorkspaceOverlayRecovery(ctx context.Context, target Target) (Snapshot, error) {
	if target.Kind != TargetBaseWorkspaceOverlay || target.Projection != ProjectionStaged ||
		target.IntendedUntracked == nil || len(target.IntendedUntracked) != 0 || len(target.LedgerIDs) != 0 {
		return Snapshot{}, errors.New("staged workspace-overlay recovery requires an explicit empty intended_untracked list and no ledger IDs")
	}
	return builder.build(ctx, target, true)
}

func (builder SnapshotBuilder) BuildStoredSnapshot(ctx context.Context, target Target) (Snapshot, error) {
	if target.Kind == TargetBaseWorkspaceOverlay && target.Projection == ProjectionStaged {
		return builder.BuildStagedWorkspaceOverlayRecovery(ctx, target)
	}
	return builder.Build(ctx, target)
}

func (builder SnapshotBuilder) build(ctx context.Context, target Target, allowStagedIntended bool) (Snapshot, error) {
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	builder.Repo = repo

	projection, err := canonicalProjection(target.Projection)
	if err != nil {
		return Snapshot{}, err
	}
	if projection == ProjectionStaged && target.Kind != TargetCurrentChanges && target.Kind != TargetBaseDiff && target.Kind != TargetFixDiff &&
		(target.Kind != TargetBaseWorkspaceOverlay || !allowStagedIntended) {
		return Snapshot{}, errors.New("staged projection is only supported for current-changes, base-diff, and fix-diff targets")
	}

	var baseTree, candidateTree, untrackedProof string
	intended := []string{}
	ledgerIDs, err := canonicalStrings(target.LedgerIDs, "ledger id")
	if err != nil {
		return Snapshot{}, err
	}

	switch target.Kind {
	case TargetCurrentChanges:
		if target.IntendedUntracked == nil {
			return Snapshot{}, errors.New("current-changes requires an explicit intended_untracked list")
		}
		intended, err = canonicalPaths(target.IntendedUntracked)
		if err != nil {
			return Snapshot{}, err
		}
		if projection == ProjectionStaged && len(intended) != 0 {
			return Snapshot{}, errors.New("staged projection does not accept intended-untracked paths")
		}
		baseTree, candidateTree, untrackedProof, err = builder.buildCurrentChanges(ctx, intended, allowStagedIntended, projection)
	case TargetBaseDiff:
		if strings.TrimSpace(target.BaseRef) == "" {
			return Snapshot{}, errors.New("base-diff requires base_ref")
		}
		if strings.Contains(target.BaseRef, "..") {
			return Snapshot{}, errors.New("base-diff base_ref must be one revision, not a range")
		}
		intended, err = canonicalPaths(target.IntendedUntracked)
		if err != nil {
			return Snapshot{}, err
		}
		if projection == ProjectionStaged && len(intended) != 0 {
			return Snapshot{}, errors.New("staged projection does not accept intended-untracked paths")
		}
		baseTree, err = builder.resolveTree(ctx, target.BaseRef)
		if err == nil && projection == ProjectionStaged {
			candidateTree, err = builder.resolveTree(ctx, "HEAD")
			if err == nil {
				untrackedProof, err = builder.untrackedProof(ctx, candidateTree, intended)
			}
		} else if err == nil {
			candidateTree, untrackedProof, err = builder.buildHeadWithIntended(ctx, intended)
		}
	case TargetBaseWorkspaceOverlay:
		if strings.TrimSpace(target.BaseRef) == "" || strings.Contains(target.BaseRef, "..") {
			return Snapshot{}, errors.New("base-workspace-overlay requires one base_ref revision")
		}
		if projection == ProjectionStaged && !allowStagedIntended || target.IntendedUntracked == nil {
			return Snapshot{}, errors.New("base-workspace-overlay requires workspace projection and explicit intended_untracked")
		}
		intended, err = canonicalPaths(target.IntendedUntracked)
		if err == nil {
			baseTree, err = builder.resolveTree(ctx, target.BaseRef)
		}
		if err == nil {
			_, candidateTree, untrackedProof, err = builder.buildCurrentChanges(ctx, intended, allowStagedIntended, projection)
		}
	case TargetExactRevision:
		baseTree, candidateTree, err = builder.resolveExactRevision(ctx, target.Revision)
		untrackedProof = hashCanonical("gentle-ai.intended-untracked/v1")
	case TargetFixDiff:
		if strings.TrimSpace(target.BaseRef) == "" || len(ledgerIDs) == 0 {
			return Snapshot{}, errors.New("fix-diff requires base_ref and ledger_ids")
		}
		if target.IntendedUntracked == nil {
			return Snapshot{}, errors.New("fix-diff requires an explicit intended_untracked list")
		}
		intended, err = canonicalPaths(target.IntendedUntracked)
		if err != nil {
			return Snapshot{}, err
		}
		if projection == ProjectionStaged && len(intended) != 0 {
			return Snapshot{}, errors.New("staged projection does not accept intended-untracked paths")
		}
		_, candidateTree, untrackedProof, err = builder.buildCurrentChanges(ctx, intended, false, projection)
		if err == nil {
			baseTree, err = builder.resolveTree(ctx, target.BaseRef)
		}
	default:
		return Snapshot{}, fmt.Errorf("unsupported target kind %q", target.Kind)
	}
	if err != nil {
		return Snapshot{}, err
	}

	paths, err := builder.changedPaths(ctx, baseTree, candidateTree)
	if err != nil {
		return Snapshot{}, err
	}
	pathsDigest := digestPaths(paths)
	identity := snapshotIdentityForProjection(target.Kind, projection, baseTree, candidateTree, pathsDigest, untrackedProof, intended, ledgerIDs)
	return Snapshot{
		Kind: target.Kind, Projection: projection, BaseTree: baseTree, CandidateTree: candidateTree,
		UnbornHead:  builder.unbornHead,
		PathsDigest: pathsDigest, IntendedUntracked: intended,
		IntendedUntrackedProof: untrackedProof, LedgerIDs: ledgerIDs,
		Paths: paths, Identity: identity,
	}, nil
}

func (builder SnapshotBuilder) buildHeadWithIntended(ctx context.Context, intended []string) (string, string, error) {
	tracked := 0
	if len(intended) > 0 {
		entries, err := listTreeEntries(ctx, builder.Repo, "HEAD")
		if err != nil {
			return "", "", err
		}
		for _, logicalPath := range intended {
			if _, present := entries[logicalPath]; present {
				tracked++
			}
		}
	}
	if tracked != 0 && tracked != len(intended) {
		return "", "", errors.New("intended-untracked paths must transition into HEAD all-or-none")
	}
	if tracked == 0 {
		if err := builder.rejectIgnoredIntended(ctx, intended); err != nil {
			return "", "", err
		}
	}

	temp, err := os.CreateTemp("", "gentle-ai-review-index-*")
	if err != nil {
		return "", "", err
	}
	tempIndex := temp.Name()
	defer os.Remove(tempIndex)
	if err := temp.Close(); err != nil {
		return "", "", err
	}
	env := []string{"GIT_INDEX_FILE=" + tempIndex}
	if _, err := runGit(ctx, builder.Repo, env, nil, "read-tree", "HEAD"); err != nil {
		return "", "", err
	}
	if len(intended) > 0 && tracked == 0 {
		if err := addIntendedPathspecs(ctx, builder.Repo, env, intended); err != nil {
			return "", "", err
		}
	}
	output, err := runGit(ctx, builder.Repo, env, nil, "write-tree")
	if err != nil {
		return "", "", err
	}
	candidateTree := strings.TrimSpace(string(output))
	proof, err := builder.untrackedProof(ctx, candidateTree, intended)
	return candidateTree, proof, err
}

// ValidateEvidence binds snapshot metadata to repository object evidence.
func (builder SnapshotBuilder) ValidateEvidence(ctx context.Context, snapshot Snapshot) error {
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return err
	}
	builder.Repo = repo
	paths, err := builder.changedPaths(ctx, snapshot.BaseTree, snapshot.CandidateTree)
	if err != nil {
		return err
	}
	proof, err := builder.untrackedProof(ctx, snapshot.CandidateTree, snapshot.IntendedUntracked)
	if err != nil {
		return err
	}
	digest := digestPaths(paths)
	projection, err := canonicalProjection(snapshot.Projection)
	if err != nil {
		return err
	}
	identity := snapshotIdentityForProjection(snapshot.Kind, projection, snapshot.BaseTree, snapshot.CandidateTree, digest, proof, snapshot.IntendedUntracked, snapshot.LedgerIDs)
	if !equalStrings(paths, snapshot.Paths) || digest != snapshot.PathsDigest || proof != snapshot.IntendedUntrackedProof || identity != snapshot.Identity {
		return errors.New("snapshot paths, digests, or identity do not match Git tree evidence")
	}
	return nil
}

// ValidateLiveSnapshot proves that a frozen snapshot still describes its exact live target.
func (builder SnapshotBuilder) ValidateLiveSnapshot(ctx context.Context, expected Snapshot) error {
	if err := builder.ValidateEvidence(ctx, expected); err != nil {
		return fmt.Errorf("validate frozen snapshot Git evidence: %w", err)
	}
	target := Target{
		Kind: expected.Kind, Projection: expected.Projection,
		IntendedUntracked: append([]string{}, expected.IntendedUntracked...),
		LedgerIDs:         append([]string(nil), expected.LedgerIDs...),
	}
	switch expected.Kind {
	case TargetCurrentChanges:
	case TargetBaseDiff, TargetBaseWorkspaceOverlay, TargetFixDiff:
		target.BaseRef = expected.BaseTree
	default:
		return fmt.Errorf("unsupported live snapshot target kind %q", expected.Kind)
	}
	live, err := builder.BuildStoredSnapshot(ctx, target)
	if err != nil {
		return fmt.Errorf("rebuild live snapshot target: %w", err)
	}
	if live.UnbornHead != expected.UnbornHead || !snapshotsEqual(live, expected) {
		return fmt.Errorf("live repository snapshot no longer matches frozen target: expected %s, got %s", expected.Identity, live.Identity)
	}
	return nil
}

func (builder SnapshotBuilder) CandidateLocationSupportsCausality(ctx context.Context, snapshot Snapshot, location string, causality CausalDisposition) (bool, error) {
	if err := builder.ValidateEvidence(ctx, snapshot); err != nil {
		return false, err
	}
	separator := strings.LastIndex(location, ":")
	if !findingLocationInGenesis(location, snapshot.Paths) {
		return false, nil
	}
	logicalPath := location[:separator]
	line, _ := strconv.Atoi(location[separator+1:])
	if causality == CausalBehaviorActivated {
		entry, err := runGit(ctx, builder.Repo, nil, nil, "ls-tree", "-z", snapshot.CandidateTree, "--", literalPathspec(logicalPath))
		if err != nil || len(entry) == 0 {
			return false, err
		}
		for _, tree := range []string{snapshot.CandidateTree} {
			blob, err := runGit(ctx, builder.Repo, nil, nil, "show", tree+":"+logicalPath)
			if err != nil {
				return false, err
			}
			lines := bytes.Count(blob, []byte{'\n'})
			if len(blob) > 0 && blob[len(blob)-1] != '\n' {
				lines++
			}
			if line <= lines {
				return true, nil
			}
		}
		return false, nil
	}
	if causality != CausalIntroduced && causality != CausalWorsened {
		return false, nil
	}
	output, err := runGit(ctx, builder.Repo, nil, nil, "diff", "--unified=0", "--no-renames", "--no-ext-diff", "--no-textconv", snapshot.BaseTree, snapshot.CandidateTree, "--", literalPathspec(logicalPath))
	if err != nil {
		return false, err
	}
	for _, match := range regexp.MustCompile(`(?m)^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`).FindAllSubmatch(output, -1) {
		offset := 3
		start, _ := strconv.Atoi(string(match[offset]))
		count := 1
		if len(match[offset+1]) > 0 {
			count, _ = strconv.Atoi(string(match[offset+1]))
		}
		if count > 0 && line >= start && line < start+count {
			return true, nil
		}
	}
	return false, nil
}

func rebuildCurrentSnapshotEvidence(ctx context.Context, repo string, snapshot Snapshot) error {
	if strings.TrimSpace(repo) == "" {
		return errors.New("repository evidence is required for invalidation")
	}
	target := Target{Kind: snapshot.Kind, Projection: snapshot.Projection, IntendedUntracked: append([]string(nil), snapshot.IntendedUntracked...)}
	if target.IntendedUntracked == nil {
		target.IntendedUntracked = []string{}
	}
	switch snapshot.Kind {
	case TargetCurrentChanges:
	case TargetBaseDiff, TargetBaseWorkspaceOverlay:
		target.BaseRef = snapshot.BaseTree
	default:
		return errors.New("invalidation supports only live current-changes or base-diff snapshots")
	}
	live, err := (SnapshotBuilder{Repo: repo}).BuildStoredSnapshot(ctx, target)
	if err != nil {
		return err
	}
	if !snapshotsEqual(live, snapshot) {
		return fmt.Errorf("live repository snapshot no longer matches the reviewing authority: expected %s, got %s", snapshot.Identity, live.Identity)
	}
	return nil
}

// DiffStats returns the canonical base-to-candidate numstat for a validated
// snapshot boundary. It rejects any mismatch with the snapshot path set.
func (builder SnapshotBuilder) DiffStats(ctx context.Context, snapshot Snapshot) ([]DiffStat, error) {
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	isolation, cleanup, err := isolatedImmutableTreeGit(ctx, repo)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	output, err := runGitIsolated(ctx, repo, isolation, nil, "diff", "--numstat", "-z", "--no-renames", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none", snapshot.BaseTree, snapshot.CandidateTree, "--")
	if err != nil {
		return nil, err
	}
	statsByPath := make(map[string]DiffStat, len(snapshot.Paths))
	for _, record := range bytes.Split(output, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		fields := bytes.SplitN(record, []byte{'\t'}, 3)
		if len(fields) != 3 {
			return nil, fmt.Errorf("unexpected immutable diff stat %q", record)
		}
		logicalPath, err := normalizeLogicalPath(string(fields[2]))
		if err != nil {
			return nil, err
		}
		if _, duplicate := statsByPath[logicalPath]; duplicate {
			return nil, fmt.Errorf("duplicate immutable diff stat path %q", logicalPath)
		}
		stat := DiffStat{Path: logicalPath, Generated: isGeneratedGoldenPath(logicalPath)}
		if bytes.Equal(fields[0], []byte{'-'}) && bytes.Equal(fields[1], []byte{'-'}) {
			stat.Binary = true
		} else {
			stat.Additions, err = strconv.Atoi(string(fields[0]))
			if err != nil {
				return nil, fmt.Errorf("parse additions for %q: %w", stat.Path, err)
			}
			stat.Deletions, err = strconv.Atoi(string(fields[1]))
			if err != nil {
				return nil, fmt.Errorf("parse deletions for %q: %w", stat.Path, err)
			}
		}
		statsByPath[stat.Path] = stat
	}
	rawOutput, err := runGitIsolated(ctx, repo, isolation, nil, "diff", "--raw", "-z", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=none", snapshot.BaseTree, snapshot.CandidateTree, "--")
	if err != nil {
		return nil, err
	}
	modesByPath, err := parseRawDiffModes(rawOutput)
	if err != nil {
		return nil, err
	}
	stats := make([]DiffStat, 0, len(snapshot.Paths))
	for _, path := range snapshot.Paths {
		stat, ok := statsByPath[path]
		if !ok {
			return nil, fmt.Errorf("immutable snapshot path %q is missing from tree diff stats", path)
		}
		modes, ok := modesByPath[path]
		if !ok {
			return nil, fmt.Errorf("immutable snapshot path %q is missing from raw tree diff", path)
		}
		stat.OldMode, stat.NewMode = modes.oldMode, modes.newMode
		stat.ModeOnly = modes.oldObject == modes.newObject && modes.oldMode != modes.newMode
		stats = append(stats, stat)
	}
	if len(statsByPath) != len(snapshot.Paths) || len(modesByPath) != len(snapshot.Paths) {
		return nil, errors.New("immutable tree diff contains paths outside the review snapshot")
	}
	return stats, nil
}

type rawDiffModes struct {
	status               CandidatePathStatus
	oldMode, newMode     string
	oldObject, newObject string
}

func parseRawDiffModes(payload []byte) (map[string]rawDiffModes, error) {
	records := bytes.Split(payload, []byte{0})
	modes := make(map[string]rawDiffModes, len(records)/2)
	for index := 0; index < len(records); index++ {
		header := records[index]
		if len(header) == 0 {
			continue
		}
		fields := bytes.Fields(header)
		if len(fields) != 5 || len(fields[0]) != 7 || fields[0][0] != ':' || index+1 >= len(records) || len(records[index+1]) == 0 {
			return nil, fmt.Errorf("unexpected immutable raw diff record %q", header)
		}
		if len(fields[4]) != 1 || !bytes.ContainsAny(fields[4], "ADMT") {
			return nil, fmt.Errorf("unexpected immutable raw diff status %q", fields[4])
		}
		oldMode, newMode := string(fields[0][1:]), string(fields[1])
		if !validRawGitMode(oldMode) || !validRawGitMode(newMode) {
			return nil, fmt.Errorf("unexpected immutable raw diff modes %q and %q", oldMode, newMode)
		}
		index++
		logicalPath, err := normalizeLogicalPath(string(records[index]))
		if err != nil {
			return nil, err
		}
		if _, duplicate := modes[logicalPath]; duplicate {
			return nil, fmt.Errorf("duplicate immutable raw diff path %q", logicalPath)
		}
		modes[logicalPath] = rawDiffModes{
			status: CandidatePathStatus(fields[4]), oldMode: oldMode, newMode: newMode,
			oldObject: string(fields[2]), newObject: string(fields[3]),
		}
	}
	return modes, nil
}

func validRawGitMode(mode string) bool {
	if len(mode) != 6 {
		return false
	}
	for _, digit := range mode {
		if digit < '0' || digit > '7' {
			return false
		}
	}
	return true
}

func isGeneratedGoldenPath(logicalPath string) bool {
	normalized := "/" + strings.TrimPrefix(filepath.ToSlash(logicalPath), "./")
	return strings.Contains(normalized, "/testdata/golden/") && strings.HasSuffix(normalized, ".golden")
}

func (builder SnapshotBuilder) repositoryRoot(ctx context.Context) (string, error) {
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		return "", err
	}
	abs, err := canonicalRepositoryPath(builder.Repo)
	if err != nil {
		return "", err
	}
	// Identity, not string equality. Git reports the toplevel in the spelling
	// the kernel gave it, which on a case-insensitive volume differs from the
	// spelling the caller typed even after filepath.EvalSymlinks resolved both.
	if !pathidentity.SameDirectory(root, abs) {
		return "", fmt.Errorf("snapshot repo %s is not the repository root %s", abs, root)
	}
	return root, nil
}

// ResolveRepositoryRoot resolves Repo through the hardened review Git boundary.
// Unlike Build, it accepts a path anywhere inside the requested repository.
func (builder SnapshotBuilder) ResolveRepositoryRoot(ctx context.Context) (string, error) {
	if strings.TrimSpace(builder.Repo) == "" {
		return "", errors.New("snapshot repository path is required")
	}
	abs, err := canonicalRepositoryPath(builder.Repo)
	if err != nil {
		return "", err
	}
	root, err := resolveGitDirectory(ctx, abs, "--show-toplevel")
	if err != nil {
		return "", err
	}
	// 1773 boundary 2: filepath.Rel decided containment by comparing strings,
	// so on a default case-insensitive APFS volume the requested path and the
	// toplevel Git reported for it -- same device, same inode, different
	// spelling -- were reported as different repositories. Containment is a
	// filesystem question and internal/pathidentity asks the filesystem.
	if !pathidentity.Contains(root, abs) {
		return "", errors.New("resolved repository root does not contain the requested path")
	}
	return root, nil
}

// DiscoverIntendedUntracked returns canonical untracked paths from the
// requested repository while ignoring inherited Git repository selectors.
func (builder SnapshotBuilder) DiscoverIntendedUntracked(ctx context.Context) ([]string, error) {
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	output, err := runGit(ctx, root, nil, nil, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(output, []byte{0})
	paths := make([]string, 0, len(parts))
	var nestedRepositories []string
	for _, item := range parts {
		if len(item) == 0 {
			continue
		}
		value := string(item)
		if strings.HasSuffix(value, "/") {
			// Without --directory, `git ls-files --others` recurses into every
			// ordinary untracked directory and lists its files one by one. The
			// only entries it reports as a bare directory with a trailing slash
			// are directories it refuses to look inside because they hold
			// another Git repository: a nested linked worktree's checkout or an
			// embedded foreign clone (issue #1881, reported as ".wt/test/").
			nestedRepositories = append(nestedRepositories, value)
			continue
		}
		paths = append(paths, value)
	}
	if len(nestedRepositories) != 0 {
		if err := excludeRegisteredNestedWorktrees(ctx, root, nestedRepositories); err != nil {
			return nil, err
		}
	}
	canonical, err := canonicalPaths(paths)
	if err != nil {
		return nil, &UntrackedScopeRefusalError{Cause: err}
	}
	return canonical, nil
}

// UntrackedScopeRefusalError marks a working-tree shape that untracked-scope
// discovery refuses as a NAMED, anticipated condition: an embedded foreign
// repository, or an untracked path Git reported that cannot be addressed as
// canonical review scope. Callers use the type to tell a policy refusal (the
// operator changes the repository layout) apart from an unanticipated internal
// fault (a product defect worth a defect report); the message is unchanged.
type UntrackedScopeRefusalError struct{ Cause error }

func (err *UntrackedScopeRefusalError) Error() string { return err.Cause.Error() }
func (err *UntrackedScopeRefusalError) Unwrap() error { return err.Cause }

// excludeRegisteredNestedWorktrees decides what happens to the opaque
// nested-repository directories `git ls-files --others` reported inside root.
//
// A directory that `git worktree list --porcelain` names as a linked worktree
// of this repository is excluded from the candidate the same way `.git` itself
// is: it is another checkout's working tree, not reviewable content of this
// one. The alternative — admitting it as ordinary untracked content — was
// considered and rejected: Git reports only the bare directory and refuses to
// enumerate the files inside it, so the snapshot could never hash or diff
// those bytes, and freezing a directory entry as if it were a reviewable file
// would produce a manifest the delivery gates can never re-verify. Exclusion
// is principled rather than pattern-based because the worktree list is Git's
// own authoritative registry of which directories are its linked checkouts.
//
// An opaque nested repository that is NOT a registered worktree (an embedded
// foreign clone) stays refused: silently dropping it would hide from the user
// that a directory they may believe is under review can never be, and Git
// itself warns rather than recurses when asked to add one. The refusal names
// the path and every honest way out.
func excludeRegisteredNestedWorktrees(ctx context.Context, root string, nestedRepositories []string) error {
	registered, err := linkedWorktreeDirectories(ctx, root)
	if err != nil {
		return err
	}
	for _, value := range nestedRepositories {
		logicalPath, err := normalizeLogicalPath(strings.TrimSuffix(value, "/"))
		if err != nil {
			return &UntrackedScopeRefusalError{Cause: fmt.Errorf("untracked nested repository directory %q is not addressable as review scope: %w; add it to .gitignore or move it outside this repository", value, err)}
		}
		absolute := filepath.Join(root, filepath.FromSlash(logicalPath))
		excluded := false
		for _, worktree := range registered {
			if pathidentity.SameDirectory(absolute, worktree) {
				excluded = true
				break
			}
		}
		// guard:population nested-worktree-scope too-tight: legitimate opaque nested repositories are Git-registered linked worktrees; unregistered embedded repositories remain excluded
		if !excluded {
			return &UntrackedScopeRefusalError{Cause: fmt.Errorf("untracked directory %q holds another Git repository that is not a linked worktree of this one, so it cannot enter the review candidate: add it to .gitignore, move it outside this repository, or register it as a linked worktree", logicalPath)} // refusal:by-design world-action: the exit is a repository-layout change (gitignore, move, or register the nested checkout), which no command of this product can decide or perform
		}
	}
	return nil
}

// linkedWorktreeDirectories returns the absolute working-tree directories Git
// registers for this repository, including the main one. Plain --porcelain
// (not -z) keeps the git floor low; a hypothetical registered path containing
// a newline would mis-parse into lines that match no opaque directory, which
// fails closed into the embedded-repository refusal instead of silently
// excluding content.
func linkedWorktreeDirectories(ctx context.Context, root string) ([]string, error) {
	output, err := runGit(ctx, root, nil, nil, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var directories []string
	for _, line := range strings.Split(string(output), "\n") {
		if directory, ok := strings.CutPrefix(line, "worktree "); ok && directory != "" {
			directories = append(directories, directory)
		}
	}
	return directories, nil
}

// DiscoverTrackedAndUnignoredPaths returns the canonical Git-owned workspace
// inventory: every cached path plus every unignored untracked path.
func (builder SnapshotBuilder) DiscoverTrackedAndUnignoredPaths(ctx context.Context) ([]string, error) {
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	output, err := runGitInventory(ctx, root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(output, []byte{0})
	paths := make([]string, 0, len(parts))
	for _, item := range parts {
		if len(item) > 0 {
			value := string(item)
			if strings.HasSuffix(value, "/") {
				value = strings.TrimSuffix(value, "/")
				if value == "" || strings.HasSuffix(value, "/") {
					return nil, fmt.Errorf("invalid opaque Git inventory path %q", item)
				}
			}
			paths = append(paths, value)
		}
	}
	return canonicalPaths(paths)
}

// HasDirtyTrackedChanges reports whether the worktree or index differs from
// HEAD, excluding untracked paths.
func (builder SnapshotBuilder) HasDirtyTrackedChanges(ctx context.Context) (bool, error) {
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		return false, err
	}
	output, err := runGit(ctx, root, nil, nil, "diff", "--name-only", "-z", "HEAD", "--")
	if err != nil {
		return false, err
	}
	return len(output) != 0, nil
}

func canonicalRepositoryPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func resolveGitDirectory(ctx context.Context, repo, selector string) (string, error) {
	switch selector {
	case "--show-toplevel", "--git-common-dir", "--git-dir":
	default:
		return "", fmt.Errorf("unsupported Git directory selector %q", selector)
	}
	output, err := runGit(ctx, repo, nil, nil, "rev-parse", selector)
	if err != nil {
		// Only --show-toplevel can fail for want of a working tree;
		// --git-dir and --git-common-dir answer normally in a bare repository.
		if selector == "--show-toplevel" {
			return "", bareRepositoryFailure(ctx, repo, err)
		}
		return "", err
	}
	return canonicalGitDirectory(repo, output)
}

// ErrBareRepositoryHasNoWorkingTree reports that the requested repository is
// bare. Refusing is correct: a review candidate is a working-tree diff, and a
// bare repository has no working tree for one to exist in.
var ErrBareRepositoryHasNoWorkingTree = errors.New("review needs a working tree")

// BareRepositoryError states that refusal in the product's own voice and names
// what the operator can run, which is normally what they wanted: they are in a
// bare clone or a server-side hook and the work they mean to review lives in a
// checkout somewhere else.
//
// It deliberately names no subcommand. This boundary serves every review verb,
// so naming one verb would hand a caller of a different verb a command that
// does not clear their block -- worse than naming nothing. The flag it names is
// the one every review verb accepts.
type BareRepositoryError struct {
	Path string
}

func (err *BareRepositoryError) Error() string {
	return fmt.Sprintf(
		"%v: %s is a bare repository, and a review candidate is a working-tree diff; "+
			"run the same command again from a checkout, or point it at one with `--cwd <path-to-a-checkout>`",
		ErrBareRepositoryHasNoWorkingTree, err.Path,
	)
}

func (err *BareRepositoryError) Unwrap() error { return ErrBareRepositoryHasNoWorkingTree }

// bareRepositoryFailure classifies a rev-parse failure by asking Git the single
// question that separates "this repository has no working tree" from every
// other cause. A probe that cannot answer, or that answers no, returns the
// original error untouched: an unexpected failure must keep the cause it came
// with, because destroying diagnostic information is its own defect.
func bareRepositoryFailure(ctx context.Context, repo string, err error) error {
	if err == nil {
		return nil
	}
	output, probeErr := runGit(ctx, repo, nil, nil, "rev-parse", "--is-bare-repository")
	if probeErr != nil || string(bytes.TrimSpace(output)) != "true" {
		return err
	}
	return &BareRepositoryError{Path: repo}
}

func canonicalGitDirectory(repo string, output []byte) (string, error) {
	if len(output) == 0 || bytes.IndexByte(output, 0) >= 0 {
		return "", errors.New("Git directory output is empty or contains NUL")
	}
	record := output
	if record[len(record)-1] == '\n' {
		record = record[:len(record)-1]
		if len(record) > 0 && record[len(record)-1] == '\r' {
			record = record[:len(record)-1]
		}
	}
	if len(record) == 0 || bytes.ContainsAny(record, "\r\n") || strings.TrimSpace(string(record)) == "" || bytes.HasPrefix(record, []byte("--")) {
		return "", errors.New("Git directory output is not exactly one valid path record")
	}
	root, err := canonicalRepositoryPath(repo)
	if err != nil {
		return "", err
	}
	directory := string(record)
	relative := !filepath.IsAbs(directory)
	if relative {
		directory = filepath.Join(root, directory)
		rel, relErr := filepath.Rel(root, directory)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return "", errors.New("relative Git directory escapes the repository root")
		}
	}
	directory, err = canonicalRepositoryPath(directory)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return "", errors.New("Git directory output is not a directory")
	}
	return filepath.Clean(directory), nil
}

func readSnapshotIndex(path string) ([]byte, time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return nil, time.Time{}, err
	}
	payload, err := io.ReadAll(file)
	if err != nil {
		return nil, time.Time{}, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, time.Time{}, err
	}
	if before.Size() != after.Size() || before.ModTime() != after.ModTime() || int64(len(payload)) != after.Size() {
		return nil, time.Time{}, errors.New("real index changed while being copied")
	}
	return payload, after.ModTime(), nil
}

func (builder *SnapshotBuilder) buildCurrentChanges(ctx context.Context, intended []string, allowStagedIntended bool, projection Projection) (string, string, string, error) {
	baseTree, unborn, err := builder.resolveCurrentChangesBase(ctx, projection)
	if err != nil {
		return "", "", "", err
	}
	builder.unbornHead = unborn
	indexPathOutput, err := runGit(ctx, builder.Repo, nil, nil, "rev-parse", "--git-path", "index")
	if err != nil {
		return "", "", "", fmt.Errorf("locate real index: %w", err)
	}
	indexPath := strings.TrimSpace(string(indexPathOutput))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(builder.Repo, indexPath)
	}
	indexContent, indexModTime, err := readSnapshotIndex(indexPath)
	missingIndex := errors.Is(err, os.ErrNotExist)
	if err != nil && !missingIndex {
		return "", "", "", fmt.Errorf("read real index: %w", err)
	}

	stagedIntended := 0
	if len(intended) > 0 {
		trackedOutput, err := runGitInventory(ctx, builder.Repo, "ls-files", "--cached", "-z", "--")
		if err != nil {
			return "", "", "", err
		}
		tracked := nulSeparatedPathSet(trackedOutput)
		for _, logicalPath := range intended {
			if _, isTracked := tracked[logicalPath]; isTracked {
				if !allowStagedIntended {
					return "", "", "", fmt.Errorf("intended-untracked path %q is already tracked", logicalPath)
				}
				stagedIntended++
			}
		}
	}
	if stagedIntended > 0 && stagedIntended != len(intended) {
		return "", "", "", errors.New("intended-untracked paths must be either all untracked or all staged")
	}
	if stagedIntended == 0 {
		if err := builder.rejectIgnoredIntended(ctx, intended); err != nil {
			return "", "", "", err
		}
	}
	for _, logicalPath := range intended {
		info, err := os.Lstat(filepath.Join(builder.Repo, filepath.FromSlash(logicalPath)))
		if err != nil {
			return "", "", "", fmt.Errorf("intended-untracked path %q: %w", logicalPath, err)
		}
		if info.IsDir() {
			return "", "", "", fmt.Errorf("intended-untracked path %q must name a file or symlink, not a directory", logicalPath)
		}
	}
	temp, err := os.CreateTemp("", "gentle-ai-review-index-*")
	if err != nil {
		return "", "", "", err
	}
	tempIndex := temp.Name()
	defer os.Remove(tempIndex)
	if err := temp.Close(); err != nil {
		return "", "", "", err
	}
	env := []string{"GIT_INDEX_FILE=" + tempIndex}
	if missingIndex {
		if err := os.Remove(tempIndex); err != nil {
			return "", "", "", err
		}
		if _, err := runGit(ctx, builder.Repo, env, nil, "read-tree", "--empty"); err != nil {
			return "", "", "", err
		}
	} else {
		if err := os.WriteFile(tempIndex, indexContent, 0o600); err != nil {
			return "", "", "", err
		}
		// Git's racily-clean check compares cached entry timestamps with the
		// index timestamp. Preserve the real index timestamp: leaving the copied
		// index freshly dated can make a rapid same-stat rewrite look safely old
		// and let `git add -u` reuse stale cached content.
		if err := os.Chtimes(tempIndex, indexModTime, indexModTime); err != nil {
			return "", "", "", err
		}
	}
	cachedEntries, err := runGitInventoryWithEnv(ctx, builder.Repo, env, "ls-files", "--cached", "-z")
	if err != nil {
		return "", "", "", err
	}
	if projection != ProjectionStaged {
		if len(cachedEntries) > 0 {
			if _, err := runGit(ctx, builder.Repo, env, nil, "add", "-u", "--", "."); err != nil {
				return "", "", "", err
			}
		}
		if len(intended) > 0 {
			if err := addIntendedPathspecs(ctx, builder.Repo, env, intended); err != nil {
				return "", "", "", err
			}
		}
	}
	candidateOutput, err := runGit(ctx, builder.Repo, env, nil, "write-tree")
	if err != nil {
		return "", "", "", err
	}
	candidateTree := strings.TrimSpace(string(candidateOutput))
	if unborn && candidateTree == baseTree {
		return "", "", "", errors.New("unborn repository has no staged changes; stage the review candidate with git add")
	}
	if allowStagedIntended && projection != ProjectionStaged {
		if _, err := runGit(ctx, builder.Repo, nil, nil, "diff", "--cached", "--quiet", candidateTree, "--"); err != nil {
			return "", "", "", errors.New("staged tree does not exactly match the complete reviewed candidate")
		}
	}
	proof, err := builder.untrackedProof(ctx, candidateTree, intended)
	if err != nil {
		return "", "", "", err
	}
	return baseTree, candidateTree, proof, nil
}

func (builder SnapshotBuilder) resolveCurrentChangesBase(ctx context.Context, projection Projection) (string, bool, error) {
	baseTree, headErr := builder.resolveTree(ctx, "HEAD")
	if headErr == nil {
		return baseTree, false, headErr
	}

	refOutput, err := runGit(ctx, builder.Repo, nil, nil, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return "", false, headErr
	}
	ref := strings.TrimSpace(string(refOutput))
	if !strings.HasPrefix(ref, "refs/heads/") || strings.TrimPrefix(ref, "refs/heads/") == "" {
		return "", false, headErr
	}
	if _, err := runGit(ctx, builder.Repo, nil, nil, "show-ref", "--verify", "--quiet", "--", ref); err == nil {
		return "", false, headErr
	} else {
		var commandErr *GitCommandError
		if !errors.As(err, &commandErr) || commandErr.ExitCode != 1 {
			return "", false, err
		}
	}
	emptyTree, err := builder.emptyTree(ctx)
	if err != nil {
		return "", false, err
	}
	return emptyTree, true, nil
}

func (builder SnapshotBuilder) emptyTree(ctx context.Context) (string, error) {
	output, err := runGit(ctx, builder.Repo, nil, []byte{}, "mktree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (builder SnapshotBuilder) resolveExactRevision(ctx context.Context, revision string) (string, string, error) {
	revision = strings.TrimSpace(revision)
	if revision == "" || strings.Contains(revision, "...") {
		return "", "", errors.New("commit-range requires one exact commit or A..B range")
	}
	if strings.Contains(revision, "..") {
		parts := strings.Split(revision, "..")
		if len(parts) != 2 || !exactObjectPattern.MatchString(parts[0]) || !exactObjectPattern.MatchString(parts[1]) {
			return "", "", errors.New("commit-range endpoints must be full hexadecimal commit IDs")
		}
		base, err := builder.resolveTree(ctx, parts[0])
		if err != nil {
			return "", "", err
		}
		candidate, err := builder.resolveTree(ctx, parts[1])
		return base, candidate, err
	}
	if !exactObjectPattern.MatchString(revision) {
		return "", "", errors.New("commit-range revision must be a full hexadecimal commit ID")
	}
	commitOutput, err := runGit(ctx, builder.Repo, nil, nil, "rev-parse", "--verify", revision+"^{commit}")
	if err != nil {
		return "", "", err
	}
	commit := strings.TrimSpace(string(commitOutput))
	candidate, err := builder.resolveTree(ctx, commit)
	if err != nil {
		return "", "", err
	}
	parentsOutput, err := runGit(ctx, builder.Repo, nil, nil, "rev-list", "--parents", "-n", "1", commit)
	if err != nil {
		return "", "", err
	}
	parents := strings.Fields(string(parentsOutput))
	if len(parents) > 1 {
		base, err := builder.resolveTree(ctx, parents[1])
		return base, candidate, err
	}
	emptyTreeOutput, err := runGit(ctx, builder.Repo, nil, []byte{}, "mktree")
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(emptyTreeOutput)), candidate, nil
}

func (builder SnapshotBuilder) resolveTree(ctx context.Context, revision string) (string, error) {
	output, err := runGit(ctx, builder.Repo, nil, nil, "rev-parse", "--verify", strings.TrimSpace(revision)+"^{tree}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (builder SnapshotBuilder) changedPaths(ctx context.Context, baseTree, candidateTree string) ([]string, error) {
	output, err := runGit(ctx, builder.Repo, nil, nil, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", "--no-renames", "--ignore-submodules=none", baseTree, candidateTree)
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(output, []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		logicalPath, err := normalizeLogicalPath(string(part))
		if err != nil {
			return nil, err
		}
		paths = append(paths, logicalPath)
	}
	sort.Strings(paths)
	return paths, nil
}

func (builder SnapshotBuilder) rejectIgnoredIntended(ctx context.Context, intended []string) error {
	if len(intended) == 0 {
		return nil
	}
	stdin := make([]byte, 0, len(intended)*32)
	for _, logicalPath := range intended {
		stdin = append(stdin, logicalPath...)
		stdin = append(stdin, 0)
	}
	output, err := runGit(ctx, builder.Repo, nil, stdin, "check-ignore", "-z", "--stdin", "--no-index")
	if err != nil {
		var commandErr *GitCommandError
		if errors.As(err, &commandErr) && commandErr.ExitCode == 1 {
			return nil
		}
		return err
	}
	ignored := nulSeparatedPathSet(output)
	for _, logicalPath := range intended {
		if _, isIgnored := ignored[logicalPath]; isIgnored {
			return fmt.Errorf("intended-untracked path %q is ignored", logicalPath)
		}
	}
	return nil
}

func (builder SnapshotBuilder) untrackedProof(ctx context.Context, candidateTree string, intended []string) (string, error) {
	hash := sha256.New()
	hash.Write([]byte("gentle-ai.intended-untracked/v1\x00"))
	if len(intended) == 0 {
		return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
	}
	entries, err := listTreeEntries(ctx, builder.Repo, candidateTree)
	if err != nil {
		return "", err
	}
	for _, logicalPath := range intended {
		entry, present := entries[logicalPath]
		if !present {
			return "", fmt.Errorf("intended-untracked path %q is absent from candidate tree", logicalPath)
		}
		writeLengthPrefixed(hash, []byte(logicalPath))
		writeLengthPrefixed(hash, entry)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func listTreeEntries(ctx context.Context, repo, tree string) (map[string][]byte, error) {
	output, err := runGitInventory(ctx, repo, "ls-tree", "-r", "-t", "-z", tree)
	if err != nil {
		return nil, err
	}
	return parseTreeEntries(output)
}

// parseTreeEntries preserves each complete ls-tree record, including its NUL,
// so untracked proof bytes remain identical to the former per-path command.
func parseTreeEntries(output []byte) (map[string][]byte, error) {
	if len(output) > 0 && output[len(output)-1] != 0 {
		return nil, errors.New("unexpected unterminated tree entry") // refusal:by-design world-action: truncated Git protocol output cannot be made trustworthy by a review command
	}
	entries := make(map[string][]byte)
	for _, record := range bytes.Split(output, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		tab := bytes.IndexByte(record, '\t')
		if tab < 0 {
			return nil, fmt.Errorf("unexpected tree entry %q", record) // refusal:by-design world-action: malformed Git protocol output cannot be made trustworthy by a review command
		}
		entry := append(append([]byte(nil), record...), 0)
		entries[string(record[tab+1:])] = entry
	}
	return entries, nil
}

func nulSeparatedPathSet(output []byte) map[string]struct{} {
	paths := make(map[string]struct{})
	for _, record := range bytes.Split(output, []byte{0}) {
		if len(record) > 0 {
			paths[string(record)] = struct{}{}
		}
	}
	return paths
}

func literalPathspec(logicalPath string) string {
	return ":(literal)" + logicalPath
}

func literalPathspecs(logicalPaths []string) []string {
	result := make([]string, len(logicalPaths))
	for index, logicalPath := range logicalPaths {
		result[index] = literalPathspec(logicalPath)
	}
	return result
}

// addIntendedPathspecs stages the intended-untracked paths by literal
// pathspec, feeding them to Git over stdin instead of argv. Expanding one
// ":(literal)<path>" pathspec per file into argv scales with the size of the
// intended-untracked set; Windows caps a process command line at 32767
// characters, so a large set (~1000+ paths) can exceed that limit and fail
// to launch the process at all (issue 1778). --pathspec-from-file=- with
// --pathspec-file-nul avoids argv entirely and needs no quoting, since
// entries are NUL-delimited; pathspec magic such as ":(literal)" is still
// honored per-entry.
func addIntendedPathspecs(ctx context.Context, repo string, env []string, intended []string) error {
	if len(intended) == 0 {
		return nil
	}
	stdin := nulJoinedPathspecs(intended)
	_, err := runGit(ctx, repo, env, stdin, "add", "--pathspec-from-file=-", "--pathspec-file-nul")
	return err
}

// nulJoinedPathspecs renders each intended-untracked path as a NUL-delimited
// literal pathspec suitable for `git add --pathspec-from-file=- --pathspec-file-nul`.
func nulJoinedPathspecs(logicalPaths []string) []byte {
	pathspecs := literalPathspecs(logicalPaths)
	var buffer bytes.Buffer
	for index, pathspec := range pathspecs {
		if index > 0 {
			buffer.WriteByte(0)
		}
		buffer.WriteString(pathspec)
	}
	return buffer.Bytes()
}

func canonicalPaths(values []string) ([]string, error) {
	normalized := make([]string, len(values))
	for index, value := range values {
		logicalPath, err := normalizeLogicalPath(value)
		if err != nil {
			return nil, err
		}
		normalized[index] = logicalPath
	}
	sort.Strings(normalized)
	for index := 1; index < len(normalized); index++ {
		if normalized[index] == normalized[index-1] {
			return nil, fmt.Errorf("duplicate intended-untracked path %q", normalized[index])
		}
	}
	return normalized, nil
}

// pathsAreSubset verifies that a correction can only touch paths that were
// present in the immutable genesis snapshot.
func pathsAreSubset(paths, genesis []string) error {
	canonicalCandidate, err := canonicalPaths(paths)
	if err != nil || !equalStrings(canonicalCandidate, paths) {
		return errors.New("snapshot paths must be canonical")
	}
	canonicalGenesis, err := canonicalPaths(genesis)
	if err != nil || !equalStrings(canonicalGenesis, genesis) {
		return errors.New("genesis snapshot paths must be canonical")
	}
	allowed := make(map[string]struct{}, len(genesis))
	for _, path := range genesis {
		allowed[path] = struct{}{}
	}
	for _, path := range paths {
		if _, ok := allowed[path]; !ok {
			return fmt.Errorf("correction path %q is outside immutable genesis scope", path)
		}
	}
	return nil
}

func canonicalStrings(values []string, label string) ([]string, error) {
	result := make([]string, len(values))
	for index, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("%s must be non-empty", label)
		}
		result[index] = value
	}
	sort.Strings(result)
	for index := 1; index < len(result); index++ {
		if result[index] == result[index-1] {
			return nil, fmt.Errorf("duplicate %s %q", label, result[index])
		}
	}
	return result, nil
}

func digestPaths(paths []string) string {
	hash := sha256.New()
	hash.Write([]byte("gentle-ai.paths/v1\x00"))
	for _, logicalPath := range paths {
		writeLengthPrefixed(hash, []byte(logicalPath))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func canonicalProjection(projection Projection) (Projection, error) {
	switch projection {
	case "", ProjectionWorkspace:
		return "", nil
	case ProjectionStaged:
		return ProjectionStaged, nil
	default:
		return "", fmt.Errorf("unsupported projection %q", projection)
	}
}

func snapshotIdentity(kind TargetKind, baseTree, candidateTree, pathsDigest, proof string, intended, ledgerIDs []string) string {
	return snapshotIdentityForProjection(kind, "", baseTree, candidateTree, pathsDigest, proof, intended, ledgerIDs)
}

func snapshotIdentityForProjection(kind TargetKind, projection Projection, baseTree, candidateTree, pathsDigest, proof string, intended, ledgerIDs []string) string {
	hash := sha256.New()
	if kind == TargetBaseWorkspaceOverlay {
		hash.Write([]byte("gentle-ai.review-snapshot/base-workspace-overlay/v1\x00"))
	} else if projection == ProjectionStaged {
		hash.Write([]byte("gentle-ai.review-snapshot/v2\x00"))
	} else {
		hash.Write([]byte("gentle-ai.review-snapshot/v1\x00"))
	}
	values := []string{string(kind), baseTree, candidateTree, pathsDigest, proof}
	if projection == ProjectionStaged {
		values = []string{string(kind), string(projection), baseTree, candidateTree, pathsDigest, proof}
	}
	for _, value := range values {
		writeLengthPrefixed(hash, []byte(value))
	}
	for _, value := range intended {
		writeLengthPrefixed(hash, []byte(value))
	}
	for _, value := range ledgerIDs {
		writeLengthPrefixed(hash, []byte(value))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func hashCanonical(domain string) string {
	sum := sha256.Sum256([]byte(domain + "\x00"))
	return "sha256:" + hex.EncodeToString(sum[:])
}

type byteWriter interface {
	Write([]byte) (int, error)
}

func writeLengthPrefixed(writer byteWriter, value []byte) {
	_, _ = writer.Write([]byte(strconv.Itoa(len(value))))
	_, _ = writer.Write([]byte{0})
	_, _ = writer.Write(value)
	_, _ = writer.Write([]byte{0})
}

var ErrGitCommandTimeout = errors.New("git command timed out")

type GitCommandTimeoutError struct {
	Args      []string
	Timeout   time.Duration
	Remote    bool
	Aggregate bool
	Cause     error
}

func (err *GitCommandTimeoutError) Error() string {
	scope := "local"
	if err.Remote {
		scope = "remote"
	}
	if err.Aggregate {
		scope = "aggregate"
	}
	return fmt.Sprintf("%v within %s %s budget", ErrGitCommandTimeout, err.Timeout, scope)
}

func (err *GitCommandTimeoutError) Unwrap() []error {
	causes := []error{ErrGitCommandTimeout}
	if err.Cause != nil {
		causes = append(causes, err.Cause)
	}
	return causes
}

type GitCommandError struct {
	Args     []string
	ExitCode int
	Remote   bool
	Cause    error
	Output   string
}

func (err *GitCommandError) Error() string {
	message := fmt.Sprintf("git %s failed with exit code %d", strings.Join(err.Args, " "), err.ExitCode)
	if err.Output != "" {
		message += ": " + err.Output
	}
	return message
}

func (err *GitCommandError) Unwrap() error { return err.Cause }

var ErrGitOutputLimit = errors.New("git output exceeded deterministic byte limit")

// GitOutputLimitError reports that a bounded Git capture produced more bytes
// than the caller permits. The capture retains at most Limit bytes while the
// child is drained, so oversized output cannot grow process memory without
// bound.
//
// Actual is the total number of bytes the capture observed while draining,
// which is what makes an overflow explainable rather than merely detectable: a
// caller told only "you exceeded four mebibytes" cannot tell whether it is over
// by a kilobyte or by a factor of three, and so cannot judge how much smaller a
// candidate has to become. It is zero only where the total is genuinely
// unknown, so a renderer must treat zero as "unmeasured", never as "empty".
type GitOutputLimitError struct {
	Args   []string
	Limit  int
	Actual int
}

func (err *GitOutputLimitError) Error() string {
	return fmt.Sprintf("git %s output exceeds deterministic %d-byte limit", strings.Join(err.Args, " "), err.Limit)
}

func (err *GitOutputLimitError) Unwrap() error { return ErrGitOutputLimit }

// GitProcessControlError reports that a git subprocess could not be started or
// its process tree could not be brought under control before it produced any
// result, e.g. Windows job-object or NtResumeProcess failures. It carries the
// underlying cause so failure envelopes stay diagnosable.
type GitProcessControlError struct {
	Args  []string
	Cause error
}

func (err *GitProcessControlError) Error() string {
	return fmt.Sprintf("git %s subprocess start or process-tree control failed: %v", strings.Join(err.Args, " "), err.Cause)
}

func (err *GitProcessControlError) Unwrap() error { return err.Cause }

var localGitCommandTimeout = 15 * time.Second
var remoteGitCommandTimeout = 20 * time.Second
var gitCommandWaitDelay = time.Second
var gitCommandContext = exec.CommandContext
var gitProcessTreeStarter = startGitProcessTree

func runGit(ctx context.Context, repo string, extraEnv []string, stdin []byte, args ...string) ([]byte, error) {
	return runGitCaptured(ctx, repo, extraEnv, stdin, 0, false, false, args...)
}

func runGitInventory(ctx context.Context, repo string, args ...string) ([]byte, error) {
	return runGitInventoryWithEnv(ctx, repo, nil, args...)
}

func runGitInventoryWithEnv(ctx context.Context, repo string, extraEnv []string, args ...string) ([]byte, error) {
	return runGitCaptured(ctx, repo, extraEnv, nil, 0, false, true, args...)
}

func runGitIsolated(ctx context.Context, repo string, extraEnv []string, stdin []byte, args ...string) ([]byte, error) {
	return runGitCaptured(ctx, repo, extraEnv, stdin, 0, true, false, args...)
}

func runGitLimited(ctx context.Context, repo string, extraEnv []string, stdin []byte, outputLimit int, args ...string) ([]byte, error) {
	if outputLimit <= 0 {
		return nil, &GitOutputLimitError{Args: append([]string{}, args...), Limit: outputLimit}
	}
	return runGitCaptured(ctx, repo, extraEnv, stdin, outputLimit, true, false, args...)
}

func runGitCaptured(ctx context.Context, repo string, extraEnv []string, stdin []byte, outputLimit int, isolateConfig, rejectStderr bool, args ...string) ([]byte, error) {
	output, _, err := runGitCapturedRange(ctx, repo, extraEnv, stdin, 0, outputLimit, isolateConfig, rejectStderr, true, args...)
	return output, err
}

func runGitCapturedRange(ctx context.Context, repo string, extraEnv []string, stdin []byte, outputOffset, outputLimit int, isolateConfig, rejectStderr, rejectOverflow bool, args ...string) ([]byte, int, error) {
	remote := len(args) > 0 && args[0] == "ls-remote"
	timeout := localGitCommandTimeout
	if remote {
		timeout = remoteGitCommandTimeout
	}
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := gitCommandContext(commandContext, "git", append([]string{"--no-replace-objects", "-C", repo}, args...)...)
	command.Cancel = nil
	command.WaitDelay = gitCommandWaitDelay
	command.Env = sanitizedGitEnvironmentForRun(os.Environ(), extraEnv, isolateConfig)
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	var combined, machineStdout, machineStderr bytes.Buffer
	var stdout, stderr *boundedGitOutput
	if outputLimit > 0 {
		stdout = &boundedGitOutput{offset: outputOffset, limit: outputLimit}
		stderr = &boundedGitOutput{limit: 64 << 10}
		command.Stdout, command.Stderr = stdout, stderr
	} else if rejectStderr {
		command.Stdout, command.Stderr = &machineStdout, &machineStderr
	} else {
		command.Stdout, command.Stderr = &combined, &combined
	}
	release, startErr := gitProcessTreeStarter(command)
	err := startErr
	if err == nil {
		released := make(chan struct{})
		stopRelease := context.AfterFunc(commandContext, func() { _ = release(); close(released) })
		err = command.Wait()
		if stopRelease() {
			_ = release()
		} else {
			<-released
		}
	}
	if err != nil && release != nil && command.ProcessState == nil {
		_ = release()
	}
	if err != nil && command.Process != nil && command.ProcessState == nil {
		_ = command.Process.Kill()
		_ = command.Wait()
	}
	output, diagnostic := combined.Bytes(), combined.Bytes()
	if stdout != nil {
		output, diagnostic = stdout.Bytes(), stderr.Bytes()
	} else if rejectStderr {
		output, diagnostic = machineStdout.Bytes(), machineStderr.Bytes()
	}
	if errors.Is(err, exec.ErrWaitDelay) && commandContext.Err() == nil {
		err = nil
	}
	if err != nil {
		if commandContext.Err() != nil {
			cause := commandContext.Err()
			aggregate := ctx.Err() != nil
			if aggregate {
				cause = ctx.Err()
			}
			return nil, 0, &GitCommandTimeoutError{
				Args: append([]string{}, args...), Timeout: timeout, Remote: remote, Aggregate: aggregate, Cause: cause,
			}
		}
		if startErr != nil {
			return nil, 0, &GitProcessControlError{Args: append([]string{}, args...), Cause: startErr}
		}
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return nil, 0, &GitCommandError{
			Args: append([]string{}, args...), ExitCode: exitCode, Remote: remote, Cause: err,
			Output: strings.TrimSpace(string(diagnostic)),
		}
	}
	if stdout != nil && stdout.exceeded && rejectOverflow {
		return nil, 0, &GitOutputLimitError{Args: append([]string{}, args...), Limit: outputLimit, Actual: stdout.total}
	}
	if rejectStderr && len(diagnostic) != 0 {
		return nil, 0, fmt.Errorf("git inventory produced diagnostics: %s", strings.TrimSpace(string(diagnostic)))
	}
	total := len(output)
	if stdout != nil {
		total = stdout.total
	}
	return output, total, nil
}

type boundedGitOutput struct {
	buffer   bytes.Buffer
	offset   int
	limit    int
	exceeded bool
	// total counts every byte the child produced, including the bytes past
	// the limit that are deliberately discarded. Counting is free -- the
	// child is drained regardless -- and it is the only place the true size
	// is ever visible, because nothing downstream retains the discarded tail.
	total int
}

func (output *boundedGitOutput) Write(payload []byte) (int, error) {
	written := len(payload)
	start := output.total
	output.total += written
	if output.total <= output.offset {
		return written, nil
	}
	payloadOffset := 0
	if start < output.offset {
		payloadOffset = output.offset - start
	}
	payload = payload[payloadOffset:]
	remaining := output.limit - output.buffer.Len()
	if remaining > 0 {
		if remaining > len(payload) {
			remaining = len(payload)
		}
		_, _ = output.buffer.Write(payload[:remaining])
	}
	if len(payload) > remaining {
		output.exceeded = true
	}
	return written, nil
}

func (output *boundedGitOutput) Bytes() []byte { return output.buffer.Bytes() }

func sanitizedGitEnvironment(environment, extra []string) []string {
	return sanitizedGitEnvironmentForRun(environment, extra, false)
}

func sanitizedGitEnvironmentForRun(environment, extra []string, isolateConfig bool) []string {
	unsafe := map[string]struct{}{
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": {},
		"GIT_CEILING_DIRECTORIES":          {},
		"GIT_COMMON_DIR":                   {},
		"GIT_DIR":                          {},
		"GIT_DISCOVERY_ACROSS_FILESYSTEM":  {},
		"GIT_GRAFT_FILE":                   {},
		"GIT_IMPLICIT_WORK_TREE":           {},
		"GIT_INDEX_FILE":                   {},
		"GIT_INTERNAL_SUPER_PREFIX":        {},
		"GIT_ICASE_PATHSPECS":              {},
		"GIT_NAMESPACE":                    {},
		"GIT_LITERAL_PATHSPECS":            {},
		"GIT_GLOB_PATHSPECS":               {},
		"GIT_NOGLOB_PATHSPECS":             {},
		"GIT_NO_REPLACE_OBJECTS":           {},
		"GIT_OBJECT_DIRECTORY":             {},
		"GIT_PREFIX":                       {},
		"GIT_QUARANTINE_PATH":              {},
		"GIT_REPLACE_REF_BASE":             {},
		"GIT_SHALLOW_FILE":                 {},
		"GIT_WORK_TREE":                    {},
	}
	processEssential := map[string]struct{}{
		"COMSPEC": {}, "PATH": {}, "PATHEXT": {}, "SYSTEMDRIVE": {},
		"SYSTEMROOT": {}, "TEMP": {}, "TMP": {}, "TMPDIR": {}, "WINDIR": {},
	}
	result := make([]string, 0, len(environment)+len(extra)+1)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		canonicalName := strings.ToUpper(name)
		_, remove := unsafe[canonicalName]
		trace := strings.HasPrefix(canonicalName, "GIT_TRACE")
		_, essential := processEssential[canonicalName]
		if isolateConfig {
			if essential && !strings.HasPrefix(canonicalName, "GIT_") {
				result = append(result, entry)
			}
			continue
		}
		if !remove && !trace && canonicalName != "LC_ALL" {
			result = append(result, entry)
		}
	}
	result = append(result, "LC_ALL=C")
	result = append(result, extra...)
	return result
}
