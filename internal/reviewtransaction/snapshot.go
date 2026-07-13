package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type TargetKind string

const (
	TargetCurrentChanges TargetKind = "current-changes"
	TargetBaseDiff       TargetKind = "base-diff"
	TargetExactRevision  TargetKind = "commit-range"
	TargetFixDiff        TargetKind = "fix-diff"
)

type Target struct {
	Kind              TargetKind `json:"kind"`
	BaseRef           string     `json:"base_ref,omitempty"`
	Revision          string     `json:"revision,omitempty"`
	IntendedUntracked []string   `json:"intended_untracked"`
	LedgerIDs         []string   `json:"ledger_ids,omitempty"`
}

type Snapshot struct {
	Kind                   TargetKind `json:"kind"`
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
	Repo string
}

var exactObjectPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}(?:[0-9a-fA-F]{24})?$`)

func (builder SnapshotBuilder) Build(ctx context.Context, target Target) (Snapshot, error) {
	return builder.build(ctx, target, false)
}

func (builder SnapshotBuilder) build(ctx context.Context, target Target, allowStagedIntended bool) (Snapshot, error) {
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	builder.Repo = repo

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
		baseTree, candidateTree, untrackedProof, err = builder.buildCurrentChanges(ctx, intended, allowStagedIntended)
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
		baseTree, err = builder.resolveTree(ctx, target.BaseRef)
		if err == nil {
			candidateTree, untrackedProof, err = builder.buildHeadWithIntended(ctx, intended)
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
		_, candidateTree, untrackedProof, err = builder.buildCurrentChanges(ctx, intended, false)
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
	identity := snapshotIdentity(target.Kind, baseTree, candidateTree, pathsDigest, untrackedProof, intended, ledgerIDs)
	return Snapshot{
		Kind: target.Kind, BaseTree: baseTree, CandidateTree: candidateTree,
		PathsDigest: pathsDigest, IntendedUntracked: intended,
		IntendedUntrackedProof: untrackedProof, LedgerIDs: ledgerIDs,
		Paths: paths, Identity: identity,
	}, nil
}

func (builder SnapshotBuilder) buildHeadWithIntended(ctx context.Context, intended []string) (string, string, error) {
	tracked := 0
	for _, logicalPath := range intended {
		output, err := runGit(ctx, builder.Repo, nil, nil, "ls-tree", "-z", "HEAD", "--", literalPathspec(logicalPath))
		if err != nil {
			return "", "", err
		}
		if len(output) > 0 {
			tracked++
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
		args := append([]string{"add", "--"}, literalPathspecs(intended)...)
		if _, err := runGit(ctx, builder.Repo, env, nil, args...); err != nil {
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
	identity := snapshotIdentity(snapshot.Kind, snapshot.BaseTree, snapshot.CandidateTree, digest, proof, snapshot.IntendedUntracked, snapshot.LedgerIDs)
	if !equalStrings(paths, snapshot.Paths) || digest != snapshot.PathsDigest || proof != snapshot.IntendedUntrackedProof || identity != snapshot.Identity {
		return errors.New("snapshot paths, digests, or identity do not match Git tree evidence")
	}
	return nil
}

func rebuildCurrentSnapshotEvidence(ctx context.Context, repo string, snapshot Snapshot) error {
	if strings.TrimSpace(repo) == "" {
		return errors.New("repository evidence is required for invalidation")
	}
	target := Target{Kind: snapshot.Kind, IntendedUntracked: append([]string(nil), snapshot.IntendedUntracked...)}
	if target.IntendedUntracked == nil {
		target.IntendedUntracked = []string{}
	}
	switch snapshot.Kind {
	case TargetCurrentChanges:
	case TargetBaseDiff:
		target.BaseRef = snapshot.BaseTree
	default:
		return errors.New("invalidation supports only live current-changes or base-diff snapshots")
	}
	live, err := (SnapshotBuilder{Repo: repo}).Build(ctx, target)
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
	output, err := runGit(ctx, repo, nil, nil, "diff", "--numstat", "--no-renames", snapshot.BaseTree, snapshot.CandidateTree, "--")
	if err != nil {
		return nil, err
	}
	stats := make([]DiffStat, 0, len(snapshot.Paths))
	seenPaths := make(map[string]struct{}, len(snapshot.Paths))
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return nil, fmt.Errorf("unexpected immutable diff stat %q", line)
		}
		logicalPath, err := normalizeLogicalPath(fields[2])
		if err != nil {
			return nil, err
		}
		stat := DiffStat{Path: logicalPath, Generated: isGeneratedGoldenPath(logicalPath)}
		if fields[0] == "-" && fields[1] == "-" {
			stat.Binary = true
		} else {
			stat.Additions, err = strconv.Atoi(fields[0])
			if err != nil {
				return nil, fmt.Errorf("parse additions for %q: %w", stat.Path, err)
			}
			stat.Deletions, err = strconv.Atoi(fields[1])
			if err != nil {
				return nil, fmt.Errorf("parse deletions for %q: %w", stat.Path, err)
			}
		}
		stats = append(stats, stat)
		seenPaths[stat.Path] = struct{}{}
	}
	for _, path := range snapshot.Paths {
		if _, ok := seenPaths[path]; !ok {
			return nil, fmt.Errorf("immutable snapshot path %q is missing from tree diff stats", path)
		}
	}
	if len(seenPaths) != len(snapshot.Paths) {
		return nil, errors.New("immutable tree diff contains paths outside the review snapshot")
	}
	return stats, nil
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
	if filepath.Clean(root) != filepath.Clean(abs) {
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
	output, err := runGit(ctx, abs, nil, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root, err := canonicalRepositoryPath(strings.TrimSpace(string(output)))
	if err != nil {
		return "", err
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
	for _, item := range parts {
		if len(item) > 0 {
			paths = append(paths, string(item))
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

func (builder SnapshotBuilder) buildCurrentChanges(ctx context.Context, intended []string, allowStagedIntended bool) (string, string, string, error) {
	baseTree, err := builder.resolveTree(ctx, "HEAD")
	if err != nil {
		return "", "", "", err
	}
	indexPathOutput, err := runGit(ctx, builder.Repo, nil, nil, "rev-parse", "--git-path", "index")
	if err != nil {
		return "", "", "", fmt.Errorf("locate real index: %w", err)
	}
	indexPath := strings.TrimSpace(string(indexPathOutput))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(builder.Repo, indexPath)
	}
	indexContent, err := os.ReadFile(indexPath)
	if err != nil {
		return "", "", "", fmt.Errorf("read real index: %w", err)
	}

	stagedIntended := 0
	for _, logicalPath := range intended {
		if _, err := runGit(ctx, builder.Repo, nil, nil, "ls-files", "--error-unmatch", "--", literalPathspec(logicalPath)); err == nil {
			if !allowStagedIntended {
				return "", "", "", fmt.Errorf("intended-untracked path %q is already tracked", logicalPath)
			}
			stagedIntended++
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
	if _, err := temp.Write(indexContent); err != nil {
		return "", "", "", err
	}
	if err := temp.Close(); err != nil {
		return "", "", "", err
	}
	env := []string{"GIT_INDEX_FILE=" + tempIndex}
	if _, err := runGit(ctx, builder.Repo, env, nil, "add", "-u", "--", "."); err != nil {
		return "", "", "", err
	}
	if len(intended) > 0 {
		args := append([]string{"add", "--"}, literalPathspecs(intended)...)
		if _, err := runGit(ctx, builder.Repo, env, nil, args...); err != nil {
			return "", "", "", err
		}
	}
	candidateOutput, err := runGit(ctx, builder.Repo, env, nil, "write-tree")
	if err != nil {
		return "", "", "", err
	}
	candidateTree := strings.TrimSpace(string(candidateOutput))
	if allowStagedIntended {
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
	output, err := runGit(ctx, builder.Repo, nil, nil, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", baseTree, candidateTree)
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
	for _, logicalPath := range intended {
		_, err := runGit(ctx, builder.Repo, nil, nil, "check-ignore", "--quiet", "--no-index", "--", logicalPath)
		if err == nil {
			return fmt.Errorf("intended-untracked path %q is ignored", logicalPath)
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return err
		}
	}
	return nil
}

func (builder SnapshotBuilder) untrackedProof(ctx context.Context, candidateTree string, intended []string) (string, error) {
	hash := sha256.New()
	hash.Write([]byte("gentle-ai.intended-untracked/v1\x00"))
	for _, logicalPath := range intended {
		output, err := runGit(ctx, builder.Repo, nil, nil, "ls-tree", "-z", candidateTree, "--", literalPathspec(logicalPath))
		if err != nil {
			return "", err
		}
		if len(output) == 0 {
			return "", fmt.Errorf("intended-untracked path %q is absent from candidate tree", logicalPath)
		}
		writeLengthPrefixed(hash, []byte(logicalPath))
		writeLengthPrefixed(hash, output)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
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

func snapshotIdentity(kind TargetKind, baseTree, candidateTree, pathsDigest, proof string, intended, ledgerIDs []string) string {
	hash := sha256.New()
	hash.Write([]byte("gentle-ai.review-snapshot/v1\x00"))
	for _, value := range []string{string(kind), baseTree, candidateTree, pathsDigest, proof} {
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

func runGit(ctx context.Context, repo string, extraEnv []string, stdin []byte, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"--no-replace-objects", "-C", repo}, args...)...)
	command.Env = sanitizedGitEnvironment(os.Environ(), extraEnv)
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func sanitizedGitEnvironment(environment, extra []string) []string {
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
		"GIT_NAMESPACE":                    {},
		"GIT_NO_REPLACE_OBJECTS":           {},
		"GIT_OBJECT_DIRECTORY":             {},
		"GIT_PREFIX":                       {},
		"GIT_QUARANTINE_PATH":              {},
		"GIT_REPLACE_REF_BASE":             {},
		"GIT_SHALLOW_FILE":                 {},
		"GIT_WORK_TREE":                    {},
	}
	result := make([]string, 0, len(environment)+len(extra)+1)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if _, remove := unsafe[name]; !remove && name != "LC_ALL" {
			result = append(result, entry)
		}
	}
	result = append(result, "LC_ALL=C")
	result = append(result, extra...)
	return result
}
