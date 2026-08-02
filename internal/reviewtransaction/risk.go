package reviewtransaction

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// LargeChangeLines is a review-composition boundary, not a tier input. Volume
// deliberately no longer selects a review tier: a five-line authorization edit
// outranks a five-thousand-line mechanical rename, so only evidence escalates.
const LargeChangeLines = 400
const MaxCorrectionChangedLines = 200

// processBoundaryScanByteLimit caps every content readback this classifier
// performs, so neither process-boundary detection nor passive-content proof can
// be turned into an unbounded repository read by a large candidate.
var processBoundaryScanByteLimit int64 = 8 << 20

var semanticSourceExtensions = map[string]struct{}{
	".c": {}, ".cc": {}, ".cpp": {}, ".cs": {}, ".go": {}, ".h": {}, ".hpp": {},
	".java": {}, ".js": {}, ".jsx": {}, ".kt": {}, ".kts": {}, ".php": {}, ".py": {},
	".rb": {}, ".rs": {}, ".sh": {}, ".bash": {}, ".zsh": {}, ".ts": {}, ".tsx": {},
}

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type RiskSignal string

const (
	SignalAuth         RiskSignal = "auth"
	SignalUpdate       RiskSignal = "update"
	SignalSecurity     RiskSignal = "security"
	SignalPayments     RiskSignal = "payments"
	SignalDataExposure RiskSignal = "data_exposure"
	SignalDataLoss     RiskSignal = "data_loss"
	SignalPermissions  RiskSignal = "permissions"
	SignalShellProcess RiskSignal = "shell_process"
)

type DiffStat struct {
	Path      string
	Additions int
	Deletions int
	Binary    bool
	Generated bool
	ModeOnly  bool
	OldMode   string
	NewMode   string
}

type RiskReasonCode string

const (
	RiskReasonHotPath          RiskReasonCode = "hot_path"
	RiskReasonServiceToken     RiskReasonCode = "service_token"
	RiskReasonShellSource      RiskReasonCode = "shell_source"
	RiskReasonProcessBoundary  RiskReasonCode = "process_boundary"
	RiskReasonProcessScanLimit RiskReasonCode = "process_scan_limit"
	RiskReasonExecutableMode   RiskReasonCode = "executable_mode"
	// RiskReasonLargeChange is never derived anymore; size stopped selecting a
	// tier. It stays declared so authorities frozen under the old size rule can
	// still be read back and replayed by the delivery gates.
	RiskReasonLargeChange         RiskReasonCode = "large_change"
	RiskReasonNonExecutableOnly   RiskReasonCode = "non_executable_only"
	RiskReasonConfigurationChange RiskReasonCode = "configuration_change"
	RiskReasonExecutableChange    RiskReasonCode = "executable_change"
	// RiskReasonEmptyContent reports a candidate that carries no bytes at all
	// on either side. It fails closed exactly like RiskReasonExecutableChange
	// -- a file whose type cannot be determined is never proven passive -- but
	// it is a separate code because the two state different facts. Reporting an
	// empty file as an executable change describes content that is not there.
	RiskReasonEmptyContent RiskReasonCode = "empty_content"
)

// RiskReason records only evidence derivable from the immutable snapshot.
// It is intentionally not part of compact authority or receipt identity.
type RiskReason struct {
	Code    RiskReasonCode `json:"code"`
	Signal  RiskSignal     `json:"signal,omitempty"`
	Path    string         `json:"path,omitempty"`
	OldMode string         `json:"old_mode,omitempty"`
	NewMode string         `json:"new_mode,omitempty"`
}

// RiskAssessment is the deterministic, repository-derived classification.
// Public exposure is negotiated separately; existing facade responses remain unchanged.
type RiskAssessment struct {
	Level        RiskLevel    `json:"level"`
	ChangedLines int          `json:"changed_lines"`
	Reasons      []RiskReason `json:"reasons"`
	// DominantLens is no longer derived: it only ever existed to steer the
	// size-based large-documentation exception. It stays declared so authorities
	// frozen before that exception was removed still replay unchanged.
	DominantLens string `json:"-"`
}

type RiskInput struct {
	Stats   []DiffStat
	Signals []RiskSignal
	// OnlyPassiveContentChanges is true only when every authored path is a
	// passive document proven by its frozen bytes and mode, never by extension.
	OnlyPassiveContentChanges bool
	TouchesConfiguration      bool
}

// SelectReviewLenses is the single RAR-owned 0/1/4 lens policy. The CLI only
// supplies the user's medium-risk focus; it does not derive review authority.
func SelectReviewLenses(assessment RiskAssessment, focus string) ([]string, error) {
	if assessment.DominantLens != "" {
		if assessment.Level != RiskMedium || assessment.DominantLens != LensReadability {
			return nil, fmt.Errorf("unsupported dominant review lens %q for risk %q", assessment.DominantLens, assessment.Level)
		}
		if _, ok := reviewFocusLens(focus); !ok {
			return nil, fmt.Errorf("unsupported review focus %q", focus)
		}
		return []string{assessment.DominantLens}, nil
	}
	switch assessment.Level {
	case RiskLow:
		return []string{}, nil
	case RiskMedium:
		lens, ok := reviewFocusLens(focus)
		if !ok {
			return nil, fmt.Errorf("unsupported review focus %q", focus)
		}
		return []string{lens}, nil
	case RiskHigh:
		return append([]string(nil), supportedLenses...), nil
	default:
		return nil, fmt.Errorf("unsupported review risk %q", assessment.Level)
	}
}

func reviewFocusLens(focus string) (string, bool) {
	lens, ok := map[string]string{
		"risk": LensRisk, "resilience": LensResilience,
		"readability": LensReadability, "reliability": LensReliability,
	}[strings.TrimSpace(focus)]
	return lens, ok
}

// ClassifyRisk selects the review tier from evidence alone. Focused 4R requires
// a named risk signal or a hot-path touch; structural readback with zero
// reviewers requires proof that every authored byte is passive; everything else
// is one consolidated review. Change volume, file count, model, provider,
// profile, and effort are intentionally not classifier inputs.
func ClassifyRisk(input RiskInput) (RiskLevel, error) {
	// Counting still runs to reject malformed or duplicated stats before any
	// tier is published; its result deliberately does not reach the decision.
	if _, err := CountChangedLines(input.Stats); err != nil {
		return "", err
	}
	for _, signal := range input.Signals {
		if !validRiskSignal(signal) {
			return "", fmt.Errorf("unknown risk signal %q", signal)
		}
	}

	if hasHighSignal(input.Signals) || touchesHotPath(input.Stats) {
		return RiskHigh, nil
	}
	if input.OnlyPassiveContentChanges && !input.TouchesConfiguration {
		return RiskLow, nil
	}
	return RiskMedium, nil
}

// riskLevelFromReasons mirrors the tier a consumer derives from the published
// reasons alone. Keeping both derivations equal is what lets the consent prompt
// tell a human why a tier was chosen without re-deriving review authority.
func riskLevelFromReasons(reasons []RiskReason) RiskLevel {
	for _, reason := range reasons {
		if reason.Signal != "" {
			return RiskHigh
		}
	}
	if len(reasons) == 1 && reasons[0].Code == RiskReasonNonExecutableOnly {
		return RiskLow
	}
	return RiskMedium
}

// CountChangedLines is the cross-adapter counting contract. Callers provide the
// canonical base-to-candidate union with one entry per repository-relative path.
// Authored text additions and deletions count once, including complete
// untracked/deleted text. Recognized generated goldens, binary, mode-only, and
// unchanged rename entries count as zero while remaining in snapshot identity.
func CountChangedLines(stats []DiffStat) (int, error) {
	total := 0
	seen := make(map[string]struct{}, len(stats))
	for _, stat := range stats {
		logicalPath, err := normalizeLogicalPath(stat.Path)
		if err != nil {
			return 0, err
		}
		if _, duplicate := seen[logicalPath]; duplicate {
			return 0, fmt.Errorf("duplicate diff stat path %q", logicalPath)
		}
		seen[logicalPath] = struct{}{}
		if stat.Additions < 0 || stat.Deletions < 0 {
			return 0, fmt.Errorf("negative diff stat for %q", logicalPath)
		}
		if isGeneratedGoldenPath(logicalPath) || stat.Binary || stat.ModeOnly {
			continue
		}
		total += stat.Additions + stat.Deletions
	}
	return total, nil
}

// CorrectionBudget freezes the maximum correction size from the original
// authored candidate. Odd line counts round up and the budget is capped at 200.
func CorrectionBudget(originalChangedLines int) (int, error) {
	if originalChangedLines < 0 {
		return 0, errors.New("original changed lines cannot be negative")
	}
	return min(MaxCorrectionChangedLines, originalChangedLines/2+originalChangedLines%2), nil
}

// ClassifySnapshotRisk derives both risk and changed lines from one immutable
// repository tree boundary and the canonical CountChangedLines contract.
func (builder SnapshotBuilder) ClassifySnapshotRisk(ctx context.Context, snapshot Snapshot) (RiskLevel, int, error) {
	assessment, err := builder.AssessSnapshotRisk(ctx, snapshot)
	return assessment.Level, assessment.ChangedLines, err
}

// AssessSnapshotRisk derives the tier, authored size, and canonical reasons
// from one immutable Git tree boundary.
func (builder SnapshotBuilder) AssessSnapshotRisk(ctx context.Context, snapshot Snapshot) (RiskAssessment, error) {
	stats, err := builder.DiffStats(ctx, snapshot)
	if err != nil {
		return RiskAssessment{}, err
	}
	changedLines, err := CountChangedLines(stats)
	if err != nil {
		return RiskAssessment{}, err
	}
	activeContent, err := builder.activePassiveContentPaths(ctx, snapshot, stats)
	if err != nil {
		return RiskAssessment{}, err
	}
	reasons := deriveSnapshotRiskReasons(stats, activeContent)
	processReasons, err := builder.processBoundaryRiskReasons(ctx, snapshot, stats)
	if err != nil {
		return RiskAssessment{}, err
	}
	if len(processReasons) > 0 {
		reasons = removeFallbackRiskReasons(reasons)
		reasons = canonicalRiskReasons(append(reasons, processReasons...))
	}
	onlyPassiveContent := true
	touchesConfiguration := false
	for _, stat := range stats {
		if isGeneratedGoldenPath(stat.Path) {
			continue
		}
		_, contradicted := activeContent[stat.Path]
		onlyPassiveContent = onlyPassiveContent && !contradicted && isPassiveContentCandidateStat(stat)
		touchesConfiguration = touchesConfiguration || isConfigurationReviewPath(stat.Path)
	}
	risk, err := ClassifyRisk(RiskInput{
		Stats: stats, Signals: riskSignalsFromReasons(reasons),
		OnlyPassiveContentChanges: onlyPassiveContent, TouchesConfiguration: touchesConfiguration,
	})
	if err != nil {
		return RiskAssessment{}, err
	}
	// Fail closed on an unexplained tier: a decision whose own reasons imply a
	// different tier could never be shown honestly to the user who has to
	// consent to its cost.
	if explained := riskLevelFromReasons(reasons); explained != risk {
		return RiskAssessment{}, fmt.Errorf("review tier %q is not explained by its reasons (%q)", risk, explained)
	}
	return RiskAssessment{Level: risk, ChangedLines: changedLines, Reasons: reasons}, nil
}

// activePassiveContentPaths reads the frozen bytes behind every passive
// candidate and returns the paths whose content contradicts the passive shape
// their extension advertises. Sizes are probed before any blob is read so tier
// classification stays bounded; an over-budget candidate set is never proven
// passive and therefore fails closed into the higher tier.
func (builder SnapshotBuilder) activePassiveContentPaths(ctx context.Context, snapshot Snapshot, stats []DiffStat) (map[string]struct{}, error) {
	candidates := make([]DiffStat, 0, len(stats))
	paths := make([]string, 0, len(stats))
	for _, stat := range stats {
		if isGeneratedGoldenPath(stat.Path) || !isPassiveContentCandidateStat(stat) {
			continue
		}
		candidates = append(candidates, stat)
		paths = append(paths, stat.Path)
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	scanned := int64(0)
	treeBlobs := make(map[string]map[string]treeBlob, 2)
	for _, tree := range []string{snapshot.BaseTree, snapshot.CandidateTree} {
		blobs, err := treeBlobSizes(ctx, repo, tree, paths)
		if err != nil {
			return nil, err
		}
		byPath := make(map[string]treeBlob, len(blobs))
		for _, blob := range blobs {
			scanned += blob.size
			byPath[blob.path] = blob
		}
		treeBlobs[tree] = byPath
	}
	active := make(map[string]struct{}, len(candidates))
	if scanned > processBoundaryScanByteLimit {
		for _, logicalPath := range paths {
			active[logicalPath] = struct{}{}
		}
		return active, nil
	}
	oids := make([]string, 0, len(candidates)*2)
	for _, stat := range candidates {
		for _, version := range []struct {
			tree string
			mode string
		}{{tree: snapshot.BaseTree, mode: stat.OldMode}, {tree: snapshot.CandidateTree, mode: stat.NewMode}} {
			if version.mode == "" || version.mode == "000000" {
				continue
			}
			blob, present := treeBlobs[version.tree][stat.Path]
			if !present {
				return nil, fmt.Errorf("read immutable passive candidate %q: blob is absent from tree inventory", stat.Path) // refusal:by-design world-action: contradictory immutable Git evidence cannot be repaired by a review command
			}
			oids = append(oids, blob.oid)
		}
	}
	contents, err := batchBlobContents(ctx, repo, oids)
	if err != nil {
		return nil, err
	}
	for _, stat := range candidates {
		for _, version := range []struct {
			tree string
			mode string
		}{{tree: snapshot.BaseTree, mode: stat.OldMode}, {tree: snapshot.CandidateTree, mode: stat.NewMode}} {
			if version.mode == "" || version.mode == "000000" {
				continue
			}
			blob := treeBlobs[version.tree][stat.Path]
			if !isPassiveDocumentContent(stat.Path, contents[blob.oid]) {
				active[stat.Path] = struct{}{}
				break
			}
		}
	}
	return active, nil
}

// batchBlobContents reads exact OIDs rather than rev:path expressions, keeping
// path bytes out of cat-file's line-delimited batch protocol.
func batchBlobContents(ctx context.Context, repo string, oids []string) (map[string][]byte, error) {
	contents := make(map[string][]byte, len(oids))
	if len(oids) == 0 {
		return contents, nil
	}
	stdin := make([]byte, 0, len(oids)*65)
	for _, oid := range oids {
		stdin = append(stdin, oid...)
		stdin = append(stdin, '\n')
	}
	output, err := runGitCaptured(ctx, repo, nil, stdin, 0, false, true, "cat-file", "--batch")
	if err != nil {
		return nil, fmt.Errorf("read immutable passive candidate blobs: %w", err)
	}
	rest := output
	for _, oid := range oids {
		newline := bytes.IndexByte(rest, '\n')
		if newline < 0 {
			return nil, fmt.Errorf("read immutable passive candidate blob %s: truncated cat-file batch header", oid) // refusal:by-design world-action: malformed Git protocol output cannot be made trustworthy by a review command
		}
		fields := bytes.Fields(rest[:newline])
		rest = rest[newline+1:]
		if len(fields) != 3 || string(fields[0]) != oid || string(fields[1]) != "blob" {
			return nil, fmt.Errorf("read immutable passive candidate blob %s: unexpected cat-file batch header %q", oid, fields) // refusal:by-design world-action: malformed Git protocol output cannot be made trustworthy by a review command
		}
		size, parseErr := strconv.ParseInt(string(fields[2]), 10, 64)
		if parseErr != nil || size < 0 || size >= int64(len(rest)) || rest[int(size)] != '\n' {
			return nil, fmt.Errorf("read immutable passive candidate blob %s: invalid cat-file batch content", oid) // refusal:by-design world-action: malformed Git protocol output cannot be made trustworthy by a review command
		}
		contents[oid] = rest[:int(size)]
		rest = rest[int(size)+1:]
	}
	if len(rest) != 0 {
		return nil, errors.New("read immutable passive candidate blobs: unexpected trailing cat-file batch output") // refusal:by-design world-action: malformed Git protocol output cannot be made trustworthy by a review command
	}
	return contents, nil
}

// isPassiveDocumentContent proves a document is inert from its own bytes.
// Extensions only nominate a candidate; an interpreter directive, runtime MDX
// syntax, or bytes that are not decodable text all withdraw the nomination.
func isPassiveDocumentContent(logicalPath string, content []byte) bool {
	if bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
		return false
	}
	if hasInterpreterDirective(content) {
		return false
	}
	if strings.EqualFold(path.Ext(logicalPath), ".mdx") {
		return isStaticMDXDocument(content)
	}
	return true
}

// hasInterpreterDirective reports the shebang that makes a file executable no
// matter which passive extension it wears.
func hasInterpreterDirective(content []byte) bool {
	return bytes.HasPrefix(bytes.TrimPrefix(content, []byte("\uFEFF")), []byte("#!"))
}

type treeBlob struct {
	path string
	oid  string
	size int64
}

// gitPathspecArgvBudget bounds the combined character length of the literal
// pathspec arguments (":(literal)<path>", one per changed file) that
// batchLiteralPathspecs packs into a single Git invocation's argv. Windows
// caps a whole process command line -- argv AND the inherited environment
// block together -- at 32767 characters (issue 1778), so this budget leaves
// headroom for: the fixed argv prefix each call site builds ("git",
// "--no-replace-objects", "-C", the repository path, the subcommand, its
// flags, and the tree-ish/pattern), the sanitized environment block the
// child inherits, and general OS bookkeeping. 26000 leaves roughly 6700
// characters of headroom under the hard limit -- comfortably more than any
// fixed prefix or realistic environment block this package constructs --
// while still fitting several hundred literal pathspecs in a single batch
// for typical path lengths, so an ordinary review never batches at all. It
// is a var, not a const, so tests can force small batches deterministically
// without needing tens of thousands of real fixture paths.
var gitPathspecArgvBudget = 26000

// gitArgvPrefixLength approximates the argv length of the fixed arguments
// that precede a batch of literal pathspecs in one Git invocation: the
// repository selector runGit always adds ("--no-replace-objects", "-C",
// repo) plus the caller's own fixed subcommand arguments (subcommand,
// flags, tree-ish/pattern, "--"). batchLiteralPathspecs subtracts this from
// gitPathspecArgvBudget instead of assuming the fixed prefix is negligible.
func gitArgvPrefixLength(repo string, fixedArgs ...string) int {
	length := len("--no-replace-objects") + 1 + len("-C") + 1 + len(repo) + 1
	for _, arg := range fixedArgs {
		length += len(arg) + 1
	}
	return length
}

// batchLiteralPathspecs splits logicalPaths into ordered batches of literal
// pathspec arguments (":(literal)<path>") so that a single Git invocation's
// combined pathspec-argument length, plus prefixLength (the caller's fixed
// argv that precedes the pathspecs), stays within gitPathspecArgvBudget.
// Every input path appears in exactly one batch, in its original order,
// with no duplication and no loss: a small set that already fits produces
// exactly one batch, and a single pathspec whose own length already exceeds
// the remaining budget still gets a batch of its own -- passed to Git,
// which reports its own failure if the platform genuinely cannot accept it
// -- rather than being silently dropped from a risk scan, which would
// understate risk (the dangerous direction for this classifier).
func batchLiteralPathspecs(logicalPaths []string, prefixLength int) [][]string {
	if len(logicalPaths) == 0 {
		return nil
	}
	budget := gitPathspecArgvBudget - prefixLength
	if budget < 1 {
		budget = 1
	}
	batches := make([][]string, 0, 1)
	var current []string
	currentLength := 0
	for _, logicalPath := range logicalPaths {
		pathspec := literalPathspec(logicalPath)
		length := len(pathspec) + 1
		if len(current) > 0 && currentLength+length > budget {
			batches = append(batches, current)
			current, currentLength = nil, 0
		}
		current = append(current, pathspec)
		currentLength += length
	}
	batches = append(batches, current)
	return batches
}

// treeBlobSizes lists the blob sizes of the given logical paths inside one
// immutable tree, so a caller can refuse an oversized scan before reading
// it. The pathspec set is batched (see batchLiteralPathspecs) instead of
// expanded into one argv entry per path, since `git ls-tree` does not
// support --pathspec-from-file and a large changed-file set can otherwise
// exceed the Windows process command-line limit (issue 1778).
func treeBlobSizes(ctx context.Context, repo, tree string, paths []string) ([]treeBlob, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	fixedArgs := []string{"ls-tree", "-r", "-l", "-z", tree, "--"}
	prefixLength := gitArgvPrefixLength(repo, fixedArgs...)
	blobs := make([]treeBlob, 0, len(paths))
	for _, batch := range batchLiteralPathspecs(paths, prefixLength) {
		args := append(append([]string{}, fixedArgs...), batch...)
		entries, err := runGit(ctx, repo, nil, nil, args...)
		if err != nil {
			return nil, err
		}
		for _, entry := range bytes.Split(entries, []byte{0}) {
			tab := bytes.IndexByte(entry, '\t')
			fields := strings.Fields(string(entry[:max(tab, 0)]))
			if tab < 0 || len(fields) != 4 {
				continue
			}
			size, parseErr := strconv.ParseInt(fields[3], 10, 64)
			if parseErr != nil {
				return nil, fmt.Errorf("parse source blob size: %w", parseErr)
			}
			blobs = append(blobs, treeBlob{path: string(entry[tab+1:]), oid: fields[2], size: size})
		}
	}
	return blobs, nil
}

func (builder SnapshotBuilder) processBoundaryRiskReasons(ctx context.Context, snapshot Snapshot, stats []DiffStat) ([]RiskReason, error) {
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(stats))
	for _, stat := range stats {
		if isContentScanEligible(stat) {
			paths = append(paths, stat.Path)
		}
	}
	if len(paths) == 0 {
		return nil, nil
	}
	reasons, scanned := make([]RiskReason, 0), int64(0)
	for _, tree := range []string{snapshot.BaseTree, snapshot.CandidateTree} {
		blobs, err := treeBlobSizes(ctx, repo, tree, paths)
		if err != nil {
			return nil, err
		}
		treePaths := make([]string, 0, len(blobs))
		for _, blob := range blobs {
			scanned += blob.size
			if scanned > processBoundaryScanByteLimit {
				return []RiskReason{{Code: RiskReasonProcessScanLimit, Signal: SignalShellProcess, Path: blob.path}}, nil
			}
			treePaths = append(treePaths, blob.path)
		}
		if len(treePaths) == 0 {
			continue
		}
		// An interpreter directive and a spawn construct are both process
		// boundaries the file's extension can neither promise nor deny, so the
		// same bounded pass looks for either inside the frozen bytes.
		fixedArgs := []string{
			"grep", "-I", "-l", "-z", "-i", "-E",
			`(^#!)|((^|[^[:alnum:]_])(subprocess|execute_process|exec)([^[:alnum:]_]|$))`, tree, "--",
		}
		prefixLength := gitArgvPrefixLength(repo, fixedArgs...)
		for _, batch := range batchLiteralPathspecs(treePaths, prefixLength) {
			args := append(append([]string{}, fixedArgs...), batch...)
			matches, err := runGit(ctx, repo, nil, nil, args...)
			var gitErr *GitCommandError
			// git grep exits 1 when a batch has no matches; that is not an
			// error and must not abort the scan or be mistaken for a real
			// command failure across the remaining batches.
			if err != nil && (!errors.As(err, &gitErr) || gitErr.ExitCode != 1) {
				return nil, err
			}
			for _, match := range bytes.Split(bytes.TrimSuffix(matches, []byte{0}), []byte{0}) {
				logicalPath := strings.TrimPrefix(string(match), tree+":")
				if logicalPath != "" {
					reasons = append(reasons, RiskReason{Code: RiskReasonProcessBoundary, Signal: SignalShellProcess, Path: logicalPath})
				}
			}
		}
	}
	return canonicalRiskReasons(reasons), nil
}

func removeFallbackRiskReasons(reasons []RiskReason) []RiskReason {
	filtered := make([]RiskReason, 0, len(reasons))
	for _, reason := range reasons {
		switch reason.Code {
		case RiskReasonNonExecutableOnly, RiskReasonConfigurationChange, RiskReasonExecutableChange, RiskReasonEmptyContent:
			continue
		default:
			filtered = append(filtered, reason)
		}
	}
	return filtered
}

func deriveSemanticRiskSignals(stats []DiffStat) []RiskSignal {
	reasons := deriveSnapshotRiskReasons(stats, nil)
	signals := make([]RiskSignal, 0, len(reasons))
	for _, reason := range reasons {
		switch reason.Code {
		case RiskReasonServiceToken, RiskReasonShellSource, RiskReasonExecutableMode:
			signals = append(signals, reason.Signal)
		}
	}
	return canonicalRiskSignals(signals)
}

// deriveSnapshotRiskReasons names the evidence behind a tier. activeContent
// carries the paths whose frozen bytes contradicted their passive extension, so
// the fallback reason reports what the content proved rather than what the file
// name suggested.
func deriveSnapshotRiskReasons(stats []DiffStat, activeContent map[string]struct{}) []RiskReason {
	candidates := make([]RiskReason, 0, len(stats)+1)
	for _, stat := range stats {
		if isSemanticRiskEligible(stat) {
			if isServiceTokenReviewPath(stat.Path) {
				candidates = append(candidates, RiskReason{Code: RiskReasonServiceToken, Signal: SignalAuth, Path: stat.Path})
			}
			if isShellReviewPath(stat.Path) {
				candidates = append(candidates, RiskReason{Code: RiskReasonShellSource, Signal: SignalShellProcess, Path: stat.Path})
			}
		}
		if executableModeChanged(stat.OldMode, stat.NewMode) {
			candidates = append(candidates, RiskReason{
				Code: RiskReasonExecutableMode, Signal: SignalPermissions, Path: stat.Path,
				OldMode: stat.OldMode, NewMode: stat.NewMode,
			})
		}
		if !isGeneratedGoldenPath(stat.Path) {
			for _, signal := range hotPathRiskSignals(stat.Path) {
				candidates = append(candidates, RiskReason{Code: RiskReasonHotPath, Signal: signal, Path: stat.Path})
			}
		}
	}
	if len(candidates) == 0 {
		candidates = append(candidates, fallbackRiskReason(stats, activeContent))
	}
	return canonicalRiskReasons(candidates)
}

func isServiceTokenReviewPath(logicalPath string) bool {
	for _, segment := range strings.Split(logicalPath, "/") {
		if hasAdjacentServiceTokenTokens(stripSemanticPathExtension(segment)) {
			return true
		}
	}
	return false
}

func isShellReviewPath(logicalPath string) bool {
	switch asciiLower(path.Ext(logicalPath)) {
	case ".sh", ".bash", ".zsh":
		return true
	case ".yml", ".yaml":
		return strings.HasPrefix(logicalPath, ".github/workflows/")
	default:
		return false
	}
}

func executableModeChanged(oldMode, newMode string) bool {
	if oldMode == "" || newMode == "" {
		return false
	}
	oldValue, oldErr := strconv.ParseUint(oldMode, 8, 32)
	newValue, newErr := strconv.ParseUint(newMode, 8, 32)
	return oldErr == nil && newErr == nil && oldValue&0o111 != newValue&0o111
}

func fallbackRiskReason(stats []DiffStat, activeContent map[string]struct{}) RiskReason {
	for _, stat := range stats {
		if isGeneratedGoldenPath(stat.Path) {
			continue
		}
		if isConfigurationReviewPath(stat.Path) {
			return RiskReason{Code: RiskReasonConfigurationChange, Path: stat.Path}
		}
		if _, contradicted := activeContent[stat.Path]; contradicted || !isPassiveContentCandidateStat(stat) {
			if isEmptyContentStat(stat) {
				return RiskReason{Code: RiskReasonEmptyContent, Path: stat.Path}
			}
			return RiskReason{Code: RiskReasonExecutableChange, Path: stat.Path}
		}
	}
	return RiskReason{Code: RiskReasonNonExecutableOnly}
}

// isEmptyContentStat reports a candidate with no bytes on either side: a
// zero-byte file added or removed. Diff stats are collected with --no-renames,
// so a non-binary entry that is not mode-only and counts no added or deleted
// line can only be an empty blob opposite an absent side.
//
// Such an entry is unclassifiable because there is nothing to classify, which
// is why it stays in the fail-closed branch. It gets its own reason so the
// evidence can say what is true -- the file is empty -- instead of asserting
// something about content it does not have.
func isEmptyContentStat(stat DiffStat) bool {
	return !stat.Binary && !stat.ModeOnly && stat.Additions+stat.Deletions == 0
}

func canonicalRiskReasons(reasons []RiskReason) []RiskReason {
	sorted := append([]RiskReason(nil), reasons...)
	sort.Slice(sorted, func(left, right int) bool {
		if sorted[left].Code != sorted[right].Code {
			return sorted[left].Code < sorted[right].Code
		}
		if sorted[left].Signal != sorted[right].Signal {
			return sorted[left].Signal < sorted[right].Signal
		}
		if sorted[left].Path != sorted[right].Path {
			return sorted[left].Path < sorted[right].Path
		}
		if sorted[left].OldMode != sorted[right].OldMode {
			return sorted[left].OldMode < sorted[right].OldMode
		}
		return sorted[left].NewMode < sorted[right].NewMode
	})
	canonical := make([]RiskReason, 0, len(sorted))
	seen := make(map[string]struct{}, len(sorted))
	for _, reason := range sorted {
		key := string(reason.Code) + "\x00" + string(reason.Signal)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		canonical = append(canonical, reason)
	}
	return canonical
}

func riskSignalsFromReasons(reasons []RiskReason) []RiskSignal {
	signals := make([]RiskSignal, 0, len(reasons))
	for _, reason := range reasons {
		if reason.Signal != "" {
			signals = append(signals, reason.Signal)
		}
	}
	return canonicalRiskSignals(signals)
}

func canonicalRiskSignals(signals []RiskSignal) []RiskSignal {
	sorted := append([]RiskSignal(nil), signals...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })
	canonical := make([]RiskSignal, 0, len(sorted))
	for _, signal := range sorted {
		if len(canonical) == 0 || canonical[len(canonical)-1] != signal {
			canonical = append(canonical, signal)
		}
	}
	return canonical
}

// isSemanticRiskEligible gates the signals derived from a path NAME. Names are
// weak evidence, so documentation, fixtures, and README-shaped files are kept
// out: a file called service-token must never escalate on its name alone.
func isSemanticRiskEligible(stat DiffStat) bool {
	if !isInspectableStat(stat) || hasExcludedRiskSegment(stat.Path) {
		return false
	}
	if strings.HasPrefix(asciiLower(path.Base(stat.Path)), "readme") {
		return false
	}
	if _, ok := semanticSourceExtensions[asciiLower(path.Ext(stat.Path))]; ok {
		return true
	}
	return isConfigurationReviewPath(stat.Path)
}

// isContentScanEligible gates the signals derived from the frozen BYTES. It is
// deliberately more permissive than name eligibility: a README-prefixed shell
// script and an inert-looking dependency or build manifest still get read,
// because only content can prove or disprove that a file executes.
func isContentScanEligible(stat DiffStat) bool {
	if !isInspectableStat(stat) || hasExcludedRiskSegment(stat.Path) {
		return false
	}
	extension := asciiLower(path.Ext(stat.Path))
	if _, ok := semanticSourceExtensions[extension]; ok {
		return true
	}
	if extension == ".txt" {
		return true
	}
	return isConfigurationReviewPath(stat.Path)
}

// isInspectableStat rejects entries whose bytes cannot carry authored evidence.
func isInspectableStat(stat DiffStat) bool {
	return stat.Additions+stat.Deletions != 0 && !stat.Binary && !stat.ModeOnly &&
		!stat.Generated && !isGeneratedGoldenPath(stat.Path)
}

// hasExcludedRiskSegment keeps documentation and test material outside source
// inspection; their bytes describe behavior instead of performing it.
func hasExcludedRiskSegment(logicalPath string) bool {
	for _, segment := range strings.Split(logicalPath, "/") {
		switch asciiLower(segment) {
		case "docs", "testdata", "fixture", "fixtures", "__fixtures__":
			return true
		}
	}
	return false
}

func stripSemanticPathExtension(segment string) string {
	extension := asciiLower(path.Ext(segment))
	if _, source := semanticSourceExtensions[extension]; source || isConfigurationReviewPath(segment) {
		return strings.TrimSuffix(segment, path.Ext(segment))
	}
	return segment
}

func hasAdjacentServiceTokenTokens(segment string) bool {
	tokens := strings.FieldsFunc(asciiLower(segment), func(r rune) bool { return r == '-' || r == '_' })
	for index := 0; index+1 < len(tokens); index++ {
		if tokens[index] == "service" && tokens[index+1] == "token" {
			return true
		}
	}
	return false
}

func asciiLower(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, value)
}

func (builder SnapshotBuilder) ChangedLines(ctx context.Context, snapshot Snapshot) (int, error) {
	stats, err := builder.DiffStats(ctx, snapshot)
	if err != nil {
		return 0, err
	}
	return CountChangedLines(stats)
}

func isRegularNonExecutableGitMode(mode string) bool {
	switch mode {
	case "", "000000", "100644":
		return true
	default:
		return false
	}
}

// isPassiveDocumentExtension nominates the extensions whose content can still be
// passive. Membership is only a nomination: mode and bytes decide the tier.
func isPassiveDocumentExtension(logicalPath string) bool {
	switch strings.ToLower(path.Ext(logicalPath)) {
	case ".md":
		return !isOperationalMarkdownPath(logicalPath)
	case ".mdx", ".rst", ".adoc", ".png", ".jpg", ".jpeg", ".gif":
		return true
	default:
		return false
	}
}

func isOperationalMarkdownPath(logicalPath string) bool {
	lower := asciiLower(logicalPath)
	segments := strings.Split(lower, "/")
	base := path.Base(lower)
	switch base {
	case "agents.md", "claude.md", "gemini.md", "kimi.md", "skill.md", "copilot-instructions.md":
		return true
	}
	for index, segment := range segments {
		switch segment {
		case ".agent", ".agents", ".claude", ".codex", ".cursor", ".opencode", "openspec", "runtime":
			return true
		}
		if index == 1 && segments[0] == "internal" {
			switch segment {
			case "runtime", "assets", "templates":
				return true
			}
		}
		name := strings.TrimSuffix(segment, path.Ext(segment))
		for _, token := range strings.FieldsFunc(name, func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < '0' || r > '9')
		}) {
			switch token {
			case "agent", "agents", "skill", "skills", "prompt", "prompts", "instruction", "instructions", "orchestrator", "orchestrators", "workflow", "workflows":
				return true
			}
		}
	}
	return false
}

// isPassiveContentCandidatePath nominates a path for tier 0. Operational,
// agent-facing, and configuration material is excluded because those documents
// are consumed as instructions rather than read as prose.
func isPassiveContentCandidatePath(logicalPath string) bool {
	if !isPassiveDocumentExtension(logicalPath) || isConfigurationReviewPath(logicalPath) {
		return false
	}
	if strings.HasPrefix(strings.ToLower(logicalPath), "internal/assets/") {
		return false
	}
	base := strings.ToLower(path.Base(logicalPath))
	switch base {
	case "agents.md", "claude.md", "gemini.md", "kimi.md", "soul.md", "tools.md", "skill.md", "copilot-instructions.md":
		return false
	}
	stem := strings.TrimSuffix(base, strings.ToLower(path.Ext(base)))
	for _, token := range strings.FieldsFunc(stem, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ' '
	}) {
		switch token {
		case "agent", "agents", "command", "commands", "prompt", "prompts", "rule", "rules", "runtime", "runtimes", "skill", "skills", "workflow", "workflows":
			return false
		}
	}
	for _, segment := range strings.Split(strings.ToLower(logicalPath), "/") {
		switch segment {
		case ".github", ".claude", ".codex", ".cursor", ".gemini", ".kiro", ".kilo", ".opencode", ".windsurf",
			"agent", "agents", "command", "commands", "prompt", "prompts", "rule", "rules", "runtime", "runtimes", "skill", "skills", "workflow", "workflows":
			return false
		}
	}
	return true
}

// isPassiveContentCandidateStat adds the mode half of the tier-0 nomination: a
// binary blob, a symlink, a gitlink, a mode-only entry, or an executable bit
// cannot be reviewed as prose whatever its extension claims.
func isPassiveContentCandidateStat(stat DiffStat) bool {
	if stat.Binary || stat.ModeOnly || !isPassiveContentCandidatePath(stat.Path) {
		return false
	}
	return isRegularNonExecutableGitMode(stat.OldMode) && isRegularNonExecutableGitMode(stat.NewMode)
}

func isStaticMDXDocument(content []byte) bool {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	fence := ""
	for scanner.Scan() {
		trimmed := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\uFEFF"))
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			fence = "```"
			continue
		}
		if strings.HasPrefix(trimmed, "~~~") {
			fence = "~~~"
			continue
		}
		prose := trimLeadingJavaScriptBlockComments(stripMarkdownCodeSpans(trimmed))
		if startsJavaScriptKeyword(prose, "import") || startsJavaScriptKeyword(prose, "export") ||
			strings.ContainsAny(prose, "{}") || strings.Contains(prose, "<") {
			return false
		}
	}
	return scanner.Err() == nil
}

func trimLeadingJavaScriptBlockComments(value string) string {
	for strings.HasPrefix(value, "/*") {
		end := strings.Index(value[2:], "*/")
		if end < 0 {
			break
		}
		value = strings.TrimSpace(value[end+4:])
	}
	return value
}

func startsJavaScriptKeyword(value, keyword string) bool {
	if !strings.HasPrefix(value, keyword) {
		return false
	}
	if len(value) == len(keyword) {
		return true
	}
	next := value[len(keyword)]
	return !((next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') ||
		(next >= '0' && next <= '9') || next == '_' || next == '$')
}

func stripMarkdownCodeSpans(value string) string {
	var visible strings.Builder
	for len(value) > 0 {
		start := strings.IndexByte(value, '`')
		if start < 0 {
			visible.WriteString(value)
			break
		}
		if escapedBacktick(value, start) {
			visible.WriteString(value[:start+1])
			value = value[start+1:]
			continue
		}
		visible.WriteString(value[:start])
		run := 1
		for start+run < len(value) && value[start+run] == '`' {
			run++
		}
		marker := value[start : start+run]
		remainder := value[start+run:]
		end := exactBacktickRunIndex(remainder, marker)
		if end < 0 {
			visible.WriteString(value[start:])
			break
		}
		value = remainder[end+run:]
	}
	return strings.TrimSpace(visible.String())
}

func exactBacktickRunIndex(value, marker string) int {
	for offset := 0; offset < len(value); {
		index := strings.Index(value[offset:], marker)
		if index < 0 {
			return -1
		}
		index += offset
		before := index > 0 && value[index-1] == '`'
		after := index+len(marker) < len(value) && value[index+len(marker)] == '`'
		if !before && !after && !escapedBacktick(value, index) {
			return index
		}
		offset = index + 1
	}
	return -1
}

func escapedBacktick(value string, index int) bool {
	backslashes := 0
	for index--; index >= 0 && value[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func isConfigurationReviewPath(logicalPath string) bool {
	base := strings.ToLower(path.Base(logicalPath))
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	switch base {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "dockerfile", "makefile":
		return true
	}
	switch strings.ToLower(path.Ext(logicalPath)) {
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".env":
		return true
	default:
		return false
	}
}

func hasHighSignal(signals []RiskSignal) bool {
	return len(signals) > 0
}

func validRiskSignal(signal RiskSignal) bool {
	switch signal {
	case SignalAuth, SignalUpdate, SignalSecurity, SignalPayments,
		SignalDataExposure, SignalDataLoss, SignalPermissions, SignalShellProcess:
		return true
	default:
		return false
	}
}

func touchesHotPath(stats []DiffStat) bool {
	for _, stat := range stats {
		if isGeneratedGoldenPath(stat.Path) {
			continue
		}
		if len(hotPathRiskSignals(stat.Path)) != 0 {
			return true
		}
	}
	return false
}

func hotPathRiskSignals(logicalPath string) []RiskSignal {
	signals := make([]RiskSignal, 0, 4)
	if isNativeReviewAuthorityPath(logicalPath) {
		signals = append(signals, SignalAuth)
	}
	for _, token := range strings.FieldsFunc(strings.ToLower(logicalPath), func(r rune) bool {
		return r == '/' || r == '\\' || r == '.' || r == '-' || r == '_'
	}) {
		switch token {
		case "auth":
			signals = append(signals, SignalAuth)
		case "update":
			signals = append(signals, SignalUpdate)
		case "security":
			signals = append(signals, SignalSecurity)
		case "payments":
			signals = append(signals, SignalPayments)
		}
	}
	return canonicalRiskSignals(signals)
}

func isNativeReviewAuthorityPath(logicalPath string) bool {
	switch logicalPath {
	case "internal/reviewtransaction/compact.go", "internal/reviewtransaction/transaction.go",
		"internal/reviewtransaction/compact_store.go", "internal/reviewtransaction/store.go",
		"internal/reviewtransaction/compact_gate.go", "internal/reviewtransaction/gate.go":
		return true
	default:
		return false
	}
}

func normalizeLogicalPath(value string) (string, error) {
	if value == "" || strings.ContainsRune(value, '\x00') || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("invalid logical path %q", value)
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != value {
		return "", fmt.Errorf("logical path is not canonical: %q", value)
	}
	return cleaned, nil
}
