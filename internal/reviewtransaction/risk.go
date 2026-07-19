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
)

const LargeChangeLines = 400
const MaxCorrectionChangedLines = 200

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
	RiskReasonHotPath             RiskReasonCode = "hot_path"
	RiskReasonServiceToken        RiskReasonCode = "service_token"
	RiskReasonShellSource         RiskReasonCode = "shell_source"
	RiskReasonExecutableMode      RiskReasonCode = "executable_mode"
	RiskReasonLargeChange         RiskReasonCode = "large_change"
	RiskReasonNonExecutableOnly   RiskReasonCode = "non_executable_only"
	RiskReasonConfigurationChange RiskReasonCode = "configuration_change"
	RiskReasonExecutableChange    RiskReasonCode = "executable_change"
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
	DominantLens string       `json:"-"`
}

type RiskInput struct {
	Stats                        []DiffStat
	Signals                      []RiskSignal
	OnlyNonExecutableChanges     bool
	OnlyPureDocumentationChanges bool
	TouchesConfiguration         bool
}

// ClassifyRisk evaluates semantic high risk before the size-based documentation
// exception, then the remaining large, low, and medium tiers.
// Model, provider, profile, and effort are intentionally not classifier inputs.
func ClassifyRisk(input RiskInput) (RiskLevel, error) {
	changedLines, err := CountChangedLines(input.Stats)
	if err != nil {
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
	if changedLines > LargeChangeLines {
		if isLargePureDocumentation(input, changedLines) {
			return RiskMedium, nil
		}
		return RiskHigh, nil
	}
	if input.OnlyNonExecutableChanges && !input.TouchesConfiguration {
		return RiskLow, nil
	}
	return RiskMedium, nil
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
	reasons := deriveSnapshotRiskReasons(stats, changedLines)
	onlyNonExecutable := true
	onlyPureDocumentation := true
	touchesConfiguration := false
	for _, stat := range stats {
		if isGeneratedGoldenPath(stat.Path) {
			continue
		}
		onlyNonExecutable = onlyNonExecutable && isLowRiskNonExecutableStat(stat)
		onlyPureDocumentation = onlyPureDocumentation && isPureDocumentationReviewStat(stat)
		touchesConfiguration = touchesConfiguration || isConfigurationReviewPath(stat.Path)
	}
	if onlyPureDocumentation {
		onlyPureDocumentation, err = builder.hasOnlyStaticMDX(ctx, snapshot, stats)
		if err != nil {
			return RiskAssessment{}, err
		}
	}
	input := RiskInput{
		Stats: stats, OnlyNonExecutableChanges: onlyNonExecutable,
		OnlyPureDocumentationChanges: onlyPureDocumentation, TouchesConfiguration: touchesConfiguration,
	}
	if isLargePureDocumentation(input, changedLines) {
		reasons = canonicalRiskReasons(append(reasons, RiskReason{Code: RiskReasonNonExecutableOnly}))
	}
	input.Signals = riskSignalsFromReasons(reasons)
	risk, err := ClassifyRisk(input)
	if err != nil {
		return RiskAssessment{}, err
	}
	dominantLens := ""
	if risk == RiskMedium && isLargePureDocumentation(input, changedLines) {
		dominantLens = LensReadability
	}
	return RiskAssessment{Level: risk, ChangedLines: changedLines, Reasons: reasons, DominantLens: dominantLens}, nil
}

func isLargePureDocumentation(input RiskInput, changedLines int) bool {
	return changedLines > LargeChangeLines && input.OnlyPureDocumentationChanges && !input.TouchesConfiguration
}

func deriveSemanticRiskSignals(stats []DiffStat) []RiskSignal {
	reasons := deriveSnapshotRiskReasons(stats, 0)
	signals := make([]RiskSignal, 0, len(reasons))
	for _, reason := range reasons {
		switch reason.Code {
		case RiskReasonServiceToken, RiskReasonShellSource, RiskReasonExecutableMode:
			signals = append(signals, reason.Signal)
		}
	}
	return canonicalRiskSignals(signals)
}

func deriveSnapshotRiskReasons(stats []DiffStat, changedLines int) []RiskReason {
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
	if changedLines > LargeChangeLines {
		candidates = append(candidates, RiskReason{Code: RiskReasonLargeChange})
	}
	if len(candidates) == 0 {
		candidates = append(candidates, fallbackRiskReason(stats))
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

func fallbackRiskReason(stats []DiffStat) RiskReason {
	for _, stat := range stats {
		if isGeneratedGoldenPath(stat.Path) {
			continue
		}
		if isConfigurationReviewPath(stat.Path) {
			return RiskReason{Code: RiskReasonConfigurationChange, Path: stat.Path}
		}
		if !isLowRiskNonExecutableStat(stat) {
			return RiskReason{Code: RiskReasonExecutableChange, Path: stat.Path}
		}
	}
	return RiskReason{Code: RiskReasonNonExecutableOnly}
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

func isSemanticRiskEligible(stat DiffStat) bool {
	if stat.Additions+stat.Deletions == 0 || stat.Binary || stat.ModeOnly || stat.Generated || isGeneratedGoldenPath(stat.Path) {
		return false
	}
	segments := strings.Split(stat.Path, "/")
	for _, segment := range segments {
		lower := asciiLower(segment)
		if lower == "docs" || lower == "testdata" || lower == "fixture" || lower == "fixtures" || lower == "__fixtures__" {
			return false
		}
	}
	base := asciiLower(path.Base(stat.Path))
	if strings.HasPrefix(base, "readme") {
		return false
	}
	if _, ok := semanticSourceExtensions[asciiLower(path.Ext(stat.Path))]; ok {
		return true
	}
	return isConfigurationReviewPath(stat.Path)
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

func isLowRiskNonExecutableStat(stat DiffStat) bool {
	if stat.Binary || stat.ModeOnly || !isRegularNonExecutableGitMode(stat.OldMode) || !isRegularNonExecutableGitMode(stat.NewMode) {
		return false
	}
	return isNonExecutableReviewPath(stat.Path)
}

func isRegularNonExecutableGitMode(mode string) bool {
	switch mode {
	case "", "000000", "100644":
		return true
	default:
		return false
	}
}

func isNonExecutableReviewPath(logicalPath string) bool {
	extension := strings.ToLower(path.Ext(logicalPath))
	switch extension {
	case ".md":
		return !isOperationalMarkdownPath(logicalPath)
	case ".rst", ".adoc", ".png", ".jpg", ".jpeg", ".gif":
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

func isPureDocumentationReviewPath(logicalPath string) bool {
	if (path.Ext(strings.ToLower(logicalPath)) != ".mdx" && !isNonExecutableReviewPath(logicalPath)) || isConfigurationReviewPath(logicalPath) {
		return false
	}
	if strings.EqualFold(path.Ext(logicalPath), ".svg") {
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

func isPureDocumentationReviewStat(stat DiffStat) bool {
	if !isPureDocumentationReviewPath(stat.Path) {
		return false
	}
	for _, mode := range []string{stat.OldMode, stat.NewMode} {
		if mode != "" && mode != "000000" && mode != "100644" {
			return false
		}
	}
	return true
}

func (builder SnapshotBuilder) hasOnlyStaticMDX(ctx context.Context, snapshot Snapshot, stats []DiffStat) (bool, error) {
	for _, stat := range stats {
		if !strings.EqualFold(path.Ext(stat.Path), ".mdx") {
			continue
		}
		for _, version := range []struct {
			tree string
			mode string
		}{{tree: snapshot.BaseTree, mode: stat.OldMode}, {tree: snapshot.CandidateTree, mode: stat.NewMode}} {
			if version.mode == "" || version.mode == "000000" {
				continue
			}
			content, err := runGit(ctx, builder.Repo, nil, nil, "cat-file", "blob", version.tree+":"+stat.Path)
			if err != nil {
				return false, fmt.Errorf("read immutable MDX %q: %w", stat.Path, err)
			}
			if !isStaticMDXDocument(content) {
				return false, nil
			}
		}
	}
	return true, nil
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
