package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
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
}

type RiskInput struct {
	Stats                    []DiffStat
	Signals                  []RiskSignal
	OnlyNonExecutableChanges bool
	TouchesConfiguration     bool
}

// ClassifyRisk evaluates high, low, then medium. The first matching tier wins.
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

	if hasHighSignal(input.Signals) || touchesHotPath(input.Stats) || changedLines > LargeChangeLines {
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
	signals := riskSignalsFromReasons(reasons)
	onlyNonExecutable := true
	touchesConfiguration := false
	for _, stat := range stats {
		if isGeneratedGoldenPath(stat.Path) {
			continue
		}
		onlyNonExecutable = onlyNonExecutable && isNonExecutableReviewPath(stat.Path)
		touchesConfiguration = touchesConfiguration || isConfigurationReviewPath(stat.Path)
	}
	risk, err := ClassifyRisk(RiskInput{
		Stats: stats, Signals: signals, OnlyNonExecutableChanges: onlyNonExecutable, TouchesConfiguration: touchesConfiguration,
	})
	if err != nil {
		return RiskAssessment{}, err
	}
	return RiskAssessment{Level: risk, ChangedLines: changedLines, Reasons: reasons}, nil
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
	switch asciiLower(filepath.Ext(logicalPath)) {
	case ".sh", ".bash", ".zsh":
		return true
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
		if !isNonExecutableReviewPath(stat.Path) {
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
	base := asciiLower(filepath.Base(stat.Path))
	if strings.HasPrefix(base, "readme") {
		return false
	}
	if _, ok := semanticSourceExtensions[asciiLower(filepath.Ext(stat.Path))]; ok {
		return true
	}
	return isConfigurationReviewPath(stat.Path)
}

func stripSemanticPathExtension(segment string) string {
	extension := asciiLower(filepath.Ext(segment))
	if _, source := semanticSourceExtensions[extension]; source || isConfigurationReviewPath(segment) {
		return strings.TrimSuffix(segment, filepath.Ext(segment))
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

func isNonExecutableReviewPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".mdx", ".rst", ".adoc", ".png", ".jpg", ".jpeg", ".gif", ".svg":
		return true
	default:
		return false
	}
}

func isConfigurationReviewPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	switch base {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "dockerfile", "makefile":
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
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
