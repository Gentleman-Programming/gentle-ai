package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/state"
	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

// CheckStatus is the outcome of a doctor check: pass, warn, or fail.
type CheckStatus string

const (
	CheckStatusPass CheckStatus = "pass"
	CheckStatusWarn CheckStatus = "warn"
	CheckStatusFail CheckStatus = "fail"
)

// CheckResult is the result of one doctor check.
type CheckResult struct {
	Name   string
	Status CheckStatus
	Detail string
	Remedy string // optional fix suggestion
}

// DoctorReport aggregates all check results.
type DoctorReport struct {
	Checks []CheckResult
}

var knownTools = []string{"gentle-ai", "engram", "gga", "claude", "opencode"}

const (
	engramHealthEnvVar = "ENGRAM_BASE_URL"
	diskWarnThreshold  = int64(100 * 1024 * 1024) // 100 MB
	diskFailThreshold  = int64(10 * 1024 * 1024)  // 10 MB
)

// Overridable for testing.
var (
	lookPathFn          = exec.LookPath
	availableBytesFn    = storage.AvailableBytes
	osUserHomeDirDoctor = os.UserHomeDir
	doctorGOOS          = runtime.GOOS
	executableExtsFn    = executableExtensions
	pathDirsFn          = func() []string {
		return filepath.SplitList(os.Getenv("PATH"))
	}
	httpGetFn = func(url string, timeout time.Duration) (int, error) {
		resp, err := (&http.Client{Timeout: timeout}).Get(url) //nolint:noctx
		if err != nil {
			return 0, err
		}
		_ = resp.Body.Close()
		return resp.StatusCode, nil
	}
	newDoctorRegistry = agents.NewDefaultRegistry
)

// RunDoctor runs all ecosystem health checks and renders a report to w.
func RunDoctor(ctx context.Context, w io.Writer) error {
	homeDir, err := osUserHomeDirDoctor()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	report := DoctorReport{}
	report.Checks = append(report.Checks, checkToolBinaries(pathDirsFn())...)
	report.Checks = append(report.Checks, checkStateJSON(homeDir))
	report.Checks = append(report.Checks, checkEngramReachable())
	report.Checks = append(report.Checks, checkDiskSpace(homeDir))

	renderDoctorReport(w, report)
	return nil
}

// checkToolBinaries checks each known tool for PATH resolution and shadowing.
func checkToolBinaries(pathDirs []string) []CheckResult {
	results := make([]CheckResult, 0, len(knownTools))
	for _, tool := range knownTools {
		results = append(results, checkOneTool(tool, pathDirs))
	}
	return results
}

func checkOneTool(tool string, pathDirs []string) CheckResult {
	resolved, shim, err := resolveDoctorTool(tool)
	if err != nil {
		return CheckResult{
			Name:   "tool:" + tool,
			Status: CheckStatusFail,
			Detail: tool + " not found in PATH",
			Remedy: "Install " + tool + " or add its directory to PATH",
		}
	}

	copies := doctorToolCopies(tool, pathDirs)
	if len(copies) > 1 {
		return CheckResult{
			Name:   "tool:" + tool,
			Status: CheckStatusWarn,
			Detail: fmt.Sprintf("%s resolved to %s but %d copies found in PATH: %s", tool, resolved, len(copies), strings.Join(copies, ", ")),
			Remedy: "Remove duplicate binaries; keep only one copy of " + tool + " in PATH",
		}
	}

	detail := tool + " found at " + resolved
	if shim != "" {
		detail += " (" + shim + ")"
	}
	return CheckResult{
		Name:   "tool:" + tool,
		Status: CheckStatusPass,
		Detail: detail,
	}
}

func resolveDoctorTool(tool string) (string, string, error) {
	resolved, err := lookPathFn(tool)
	if err == nil {
		return resolved, "", nil
	}
	if doctorGOOS != "windows" {
		return "", "", err
	}
	resolved, ps1Err := lookPathFn(tool + ".ps1")
	if ps1Err != nil {
		return "", "", err
	}
	return resolved, "PowerShell shim", nil
}

func doctorToolCopies(tool string, pathDirs []string) []string {
	seenDirs := make(map[string]struct{}, len(pathDirs))
	copies := make([]string, 0, len(pathDirs))
	for _, dir := range pathDirs {
		cleanDir := filepath.Clean(dir)
		if _, seen := seenDirs[cleanDir]; seen {
			continue
		}
		if p := toolInDir(dir, tool); p != "" {
			seenDirs[cleanDir] = struct{}{}
			copies = append(copies, p)
		}
	}
	return copies
}

// executableExtensions returns the filename suffixes to probe when scanning a
// PATH directory for a tool binary. On Windows it mirrors exec.LookPath, which
// resolves a bare name like "gentle-ai" to "gentle-ai.exe"/".cmd" via PATHEXT;
// on other platforms the bare name is used as-is. Without this, the duplicate
// scan never matches real Windows binaries and PATH shadowing goes unreported.
func executableExtensions() []string {
	return executableExtensionsFor(doctorGOOS, os.Getenv("PATHEXT"))
}

func executableExtensionsFor(goos, pathext string) []string {
	if goos != "windows" {
		return []string{""}
	}
	if pathext == "" {
		return []string{".com", ".exe", ".bat", ".cmd"}
	}
	var exts []string
	for _, e := range strings.Split(pathext, ";") {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		exts = append(exts, e)
	}
	return exts
}

// toolInDir returns the path to tool's executable inside dir, or "" if absent.
// It honors Windows executable extensions so the duplicate scan agrees with
// exec.LookPath (used for the resolved path).
func toolInDir(dir, tool string) string {
	for _, ext := range doctorToolExecutableExts() {
		candidate := filepath.Join(dir, tool+ext)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func doctorToolExecutableExts() []string {
	exts := append([]string(nil), executableExtsFn()...)
	if doctorGOOS == "windows" {
		exts = appendUniqueExt(exts, "")
		exts = appendUniqueExt(exts, ".ps1")
	}
	return exts
}

func appendUniqueExt(exts []string, ext string) []string {
	for _, existing := range exts {
		if strings.EqualFold(existing, ext) {
			return exts
		}
	}
	return append(exts, ext)
}

// checkStateJSON validates ~/.gentle-ai/state.json and agent config dirs.
func checkStateJSON(homeDir string) CheckResult {
	const name = "state:json"
	statePath := state.Path(homeDir)

	s, err := state.Read(homeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:   name,
				Status: CheckStatusWarn,
				Detail: "state file not found at " + statePath + " (expected for first-time install)",
				Remedy: "Run 'gentle-ai install' to create initial state",
			}
		}
		return CheckResult{
			Name:   name,
			Status: CheckStatusFail,
			Detail: "failed to parse " + statePath + ": " + err.Error(),
			Remedy: "Delete or repair " + statePath + ", then re-run 'gentle-ai install'",
		}
	}

	if len(s.InstalledAgents) == 0 {
		return CheckResult{
			Name:   name,
			Status: CheckStatusWarn,
			Detail: "state file found at " + statePath + " with no installed agents",
			Remedy: "Run 'gentle-ai install' to configure agents",
		}
	}

	var missing []string
	for _, agentID := range s.InstalledAgents {
		if dir := agentConfigDir(homeDir, agentID); dir != "" {
			if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
				missing = append(missing, agentID)
			}
		}
	}

	if len(missing) > 0 {
		return CheckResult{
			Name:   name,
			Status: CheckStatusWarn,
			Detail: fmt.Sprintf("state lists %d agent(s) whose config dirs are missing: %s", len(missing), strings.Join(missing, ", ")),
			Remedy: "Run 'gentle-ai sync' to restore missing config files",
		}
	}

	return CheckResult{
		Name:   name,
		Status: CheckStatusPass,
		Detail: fmt.Sprintf("state file OK — %d agent(s) installed: %s", len(s.InstalledAgents), strings.Join(s.InstalledAgents, ", ")),
	}
}

// agentConfigDir returns the expected config directory for a known agent ID.
func agentConfigDir(homeDir, agentID string) string {
	cfgBase := filepath.Join(homeDir, ".config")
	switch agentID {
	case "claude-code":
		return filepath.Join(homeDir, ".claude")
	case "opencode":
		return filepath.Join(cfgBase, "opencode")
	case "cursor":
		return filepath.Join(homeDir, ".cursor")
	case "windsurf":
		return filepath.Join(homeDir, ".codeium", "windsurf")
	case "vscode":
		return filepath.Join(cfgBase, "Code")
	case "codex":
		return filepath.Join(homeDir, ".codex")
	case "kiro":
		return filepath.Join(homeDir, ".kiro")
	default:
		return ""
	}
}

// checkEngramReachable checks whether the engram HTTP health endpoint responds.
func checkEngramReachable() CheckResult {
	const name = "engram:reachable"

	baseURL := os.Getenv(engramHealthEnvVar)
	if baseURL == "" {
		baseURL = "http://localhost:7437"
	}
	healthURL := strings.TrimRight(baseURL, "/") + "/health"

	statusCode, err := httpGetFn(healthURL, 3*time.Second)
	if err != nil {
		return CheckResult{
			Name:   name,
			Status: CheckStatusFail,
			Detail: "engram health endpoint unreachable at " + healthURL + ": " + err.Error(),
			Remedy: "Start engram or check that it is configured as an MCP server",
		}
	}
	if statusCode < 200 || statusCode >= 300 {
		return CheckResult{
			Name:   name,
			Status: CheckStatusWarn,
			Detail: fmt.Sprintf("engram health endpoint %s returned HTTP %d", healthURL, statusCode),
			Remedy: "Check engram logs for errors",
		}
	}
	return CheckResult{
		Name:   name,
		Status: CheckStatusPass,
		Detail: fmt.Sprintf("engram health endpoint OK at %s (HTTP %d)", healthURL, statusCode),
	}
}

// checkDiskSpace reports free space on the ~/.gentle-ai filesystem.
func checkDiskSpace(homeDir string) CheckResult {
	const name = "disk:space"
	dir := filepath.Join(homeDir, ".gentle-ai")

	free, err := availableBytesFn(dir)
	if err != nil {
		return CheckResult{Name: name, Status: CheckStatusWarn, Detail: "could not determine free disk space for " + dir + ": " + err.Error()}
	}

	freeMB := free / (1024 * 1024)
	switch {
	case free < diskFailThreshold:
		return CheckResult{
			Name:   name,
			Status: CheckStatusFail,
			Detail: fmt.Sprintf("critically low disk space: %d MB free on %s filesystem", freeMB, dir),
			Remedy: "Free up disk space before running install or sync operations",
		}
	case free < diskWarnThreshold:
		return CheckResult{
			Name:   name,
			Status: CheckStatusWarn,
			Detail: fmt.Sprintf("low disk space: %d MB free on %s filesystem", freeMB, dir),
			Remedy: "Consider freeing disk space",
		}
	default:
		return CheckResult{
			Name:   name,
			Status: CheckStatusPass,
			Detail: fmt.Sprintf("%d MB free on %s filesystem", freeMB, dir),
		}
	}
}

// estimateTokens returns a rough token count using the ~4-chars/token
// heuristic. Intentionally coarse: doctor labels every token figure a "rough
// estimate" and no model-specific tokenizer is a dependency (out of scope).
func estimateTokens(chars int) int {
	return chars / 4
}

// AgentFootprint is one agent's measured managed-block footprint.
type AgentFootprint struct {
	AgentID    string
	Path       string // "" when the agent id had no adapter
	Unresolved bool   // state listed an id with no registered adapter
	Present    bool   // instruction file existed and was read
	Sections   []filemerge.Section
	Anomalies  []filemerge.Anomaly
	CharCount  int // sum of section CharCounts
	LineCount  int // sum of section LineCounts
	TokenEst   int // estimateTokens(CharCount)
}

// FootprintSummary aggregates managed-block footprint measurements across all
// agents listed in the persisted install state.
type FootprintSummary struct {
	Agents        []AgentFootprint
	TotalBlocks   int
	AgentsCovered int // agents with >=1 measured block
	TotalChars    int
	TotalTokenEst int
	HasFail       bool // any AnomalyOrphanOpen or AnomalyMismatch anywhere
	HasWarn       bool // any AnomalyOrphanClose anywhere
	StateMissing  bool // state.json absent (first-time install)
	NoAgents      bool // state present and readable, but InstalledAgents is genuinely empty
	// StateUnreadable is true when state.Read failed for a reason OTHER than
	// "file does not exist" — i.e. a malformed/corrupt state.json. This is
	// deliberately distinct from NoAgents: telling a user with a corrupt
	// state file to "run install" (the NoAgents remedy) contradicts the
	// state:json check, which correctly diagnoses the same file as needing
	// repair.
	StateUnreadable bool
	// RegistryUnavailable is true when newDoctorRegistry() itself failed.
	// This is a defensive, not user-actionable, path — it does not mean
	// "no agents installed" and must not carry that remedy.
	RegistryUnavailable bool
}

// collectFootprint reads the install state and every installed agent's
// instruction file, scanning each for managed marker blocks.
func collectFootprint(homeDir string) FootprintSummary {
	var sum FootprintSummary

	reg, err := newDoctorRegistry()
	if err != nil {
		sum.RegistryUnavailable = true
		return sum
	}

	s, err := state.Read(homeDir)
	if err != nil {
		if os.IsNotExist(err) {
			sum.StateMissing = true
		} else {
			sum.StateUnreadable = true
		}
		return sum
	}

	if len(s.InstalledAgents) == 0 {
		sum.NoAgents = true
		return sum
	}

	for _, id := range s.InstalledAgents {
		sum.Agents = append(sum.Agents, collectAgentFootprint(reg, homeDir, id))
	}

	for _, af := range sum.Agents {
		sum.TotalBlocks += len(af.Sections)
		sum.TotalChars += af.CharCount
		sum.TotalTokenEst += af.TokenEst
		if len(af.Sections) > 0 {
			sum.AgentsCovered++
		}
		for _, an := range af.Anomalies {
			switch an.Kind {
			case filemerge.AnomalyOrphanOpen, filemerge.AnomalyMismatch:
				sum.HasFail = true
			case filemerge.AnomalyOrphanClose:
				sum.HasWarn = true
			}
		}
	}

	return sum
}

// collectAgentFootprint resolves and scans a single agent's instruction file.
func collectAgentFootprint(reg *agents.Registry, homeDir, agentID string) AgentFootprint {
	af := AgentFootprint{AgentID: agentID}

	adapter, ok := reg.Get(model.AgentID(agentID))
	if !ok {
		af.Unresolved = true
		return af
	}

	af.Path = adapter.SystemPromptFile(homeDir)
	data, err := os.ReadFile(af.Path)
	if err != nil {
		return af
	}
	af.Present = true

	res := filemerge.ScanSections(string(data))
	af.Sections = res.Sections
	af.Anomalies = res.Anomalies
	for _, sec := range res.Sections {
		af.CharCount += sec.CharCount
		af.LineCount += sec.LineCount
	}
	af.TokenEst = estimateTokens(af.CharCount)

	return af
}

// footprintCheckResult classifies a FootprintSummary into the always-on
// "managed:footprint" CheckResult.
func footprintCheckResult(sum FootprintSummary) CheckResult {
	const name = "managed:footprint"

	switch {
	case sum.StateMissing:
		return CheckResult{
			Name:   name,
			Status: CheckStatusWarn,
			Detail: "state file not found — managed block footprint unavailable (expected for first-time install)",
			Remedy: "Run 'gentle-ai install' to create initial state",
		}
	case sum.StateUnreadable:
		return CheckResult{
			Name:   name,
			Status: CheckStatusFail,
			Detail: "state file could not be read — managed block footprint unavailable (see the state:json check above for details)",
			Remedy: "Delete or repair the state file, then re-run 'gentle-ai install'",
		}
	case sum.NoAgents:
		return CheckResult{
			Name:   name,
			Status: CheckStatusWarn,
			Detail: "no installed agents — managed block footprint unavailable",
			Remedy: "Run 'gentle-ai install' to configure agents",
		}
	case sum.RegistryUnavailable:
		return CheckResult{
			Name:   name,
			Status: CheckStatusWarn,
			Detail: "agent registry unavailable — managed block footprint could not be computed",
			Remedy: "Re-run 'gentle-ai doctor'; if this persists, reinstall gentle-ai",
		}
	case sum.HasFail:
		return CheckResult{
			Name:   name,
			Status: CheckStatusFail,
			Detail: "broken managed block marker(s): " + strings.Join(footprintFailLocations(sum), ", "),
			Remedy: "Run 'gentle-ai sync' to repair marker boundaries",
		}
	case sum.HasWarn:
		return CheckResult{
			Name:   name,
			Status: CheckStatusWarn,
			Detail: "stray closing marker found in managed block(s)",
			Remedy: "Run 'gentle-ai sync' to repair marker boundaries",
		}
	default:
		return CheckResult{
			Name:   name,
			Status: CheckStatusPass,
			Detail: fmt.Sprintf("%d block(s) across %d agent(s), ~%d tokens (rough estimate)", sum.TotalBlocks, sum.AgentsCovered, sum.TotalTokenEst),
		}
	}
}

// footprintFailLocations returns "agentID:sectionID" for every orphan-open or
// mismatch anomaly, for the Fail detail message.
func footprintFailLocations(sum FootprintSummary) []string {
	var locations []string
	for _, af := range sum.Agents {
		for _, an := range af.Anomalies {
			if an.Kind == filemerge.AnomalyOrphanOpen || an.Kind == filemerge.AnomalyMismatch {
				locations = append(locations, af.AgentID+":"+an.ID)
			}
		}
	}
	return locations
}

// renderDoctorReport writes a human-readable report to w.
func renderDoctorReport(w io.Writer, report DoctorReport) {
	var passed, warned, failed int
	for _, c := range report.Checks {
		switch c.Status {
		case CheckStatusPass:
			passed++
		case CheckStatusWarn:
			warned++
		case CheckStatusFail:
			failed++
		}
	}

	fmt.Fprintln(w, "gentle-ai doctor — system health check")
	fmt.Fprintln(w, "=======================================")
	fmt.Fprintln(w)

	for _, c := range report.Checks {
		fmt.Fprintf(w, "  %s  %-30s %s\n", statusIcon(c.Status), c.Name, c.Detail)
		if c.Remedy != "" {
			fmt.Fprintf(w, "       Remedy: %s\n", c.Remedy)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Summary: %d passed, %d failed, %d warnings\n", passed, failed, warned)

	status := "healthy"
	if failed > 0 {
		status = "unhealthy"
	} else if warned > 0 {
		status = "degraded"
	}
	fmt.Fprintf(w, "Status:  %s\n", status)
}

func statusIcon(s CheckStatus) string {
	switch s {
	case CheckStatusPass:
		return "[ok]"
	case CheckStatusWarn:
		return "[!!]"
	case CheckStatusFail:
		return "[xx]"
	default:
		return "[??]"
	}
}
