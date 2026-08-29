package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v2/internal/doctor"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/storage"
)

type CheckStatus = doctor.Status
type CheckResult = doctor.Result
type DoctorReport = doctor.Report

const (
	CheckStatusPass = doctor.StatusPass
	CheckStatusWarn = doctor.StatusWarn
	CheckStatusFail = doctor.StatusFail
)

// coreTools are ecosystem-level binaries that gentle-ai always requires
// regardless of which agents the user installed. Agent-specific binaries are
// derived from state.json's InstalledAgents field (see #709) so the doctor
// only reports missing agents the user actually selected.
var coreTools = []string{"gentle-ai", "gga", "engram"}

// agentToolBinaries maps an agent ID from state.json's InstalledAgents to the
// CLI binary name exec.LookPath should resolve. An empty string means "no CLI
// binary to check" — typically IDE-only agents like cursor, windsurf, or
// antigravity that do not ship a standalone executable on PATH.
//
// Keep this map in sync with the agentID → binary convention used by the
// adapters under internal/agents/*/adapter.go. Unknown agent IDs are ignored
// by the doctor so legacy or custom state files do not cause spurious
// failures.
var agentToolBinaries = map[string]string{
	"claude-code":    "claude",
	"opencode":       "opencode",
	"codex":          "codex",
	"pi":             "pi",
	"gemini-cli":     "gemini",
	"kilocode":       "kilo",
	"kiro-ide":       "kiro",
	"kimi":           "kimi",
	"qwen-code":      "qwen",
	"vscode-copilot": "code",
	"openclaw":       "openclaw",
	"hermes":         "hermes",
}

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
	osExecutableDoctor = os.Executable
	engramProbeStdioFn = engram.ProbeStdio
	httpGetFn          = func(url string, timeout time.Duration) (int, error) {
		resp, err := (&http.Client{Timeout: timeout}).Get(url) //nolint:noctx
		if err != nil {
			return 0, err
		}
		_ = resp.Body.Close()
		return resp.StatusCode, nil
	}
)

// RunDoctor runs all ecosystem health checks and renders a report to w.
func RunDoctor(ctx context.Context, w io.Writer) error {
	homeDir, err := osUserHomeDirDoctor()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	installedAgents, _ := readDoctorInstalledAgents(homeDir)
	// A state read failure (missing/malformed file) is surfaced separately by
	// checkStateJSON. Here we fall back to an empty list so the doctor only
	// reports the always-required core tools — preserving the first-time-install
	// behaviour where the user has not yet selected any agents (#709).

	pathDirs := pathDirsFn()
	requiredTools := requiredDoctorTools(installedAgents)
	checks := make([]doctor.Check, 0, len(requiredTools)+3)
	for _, tool := range requiredTools {
		tool := tool
		checks = append(checks, doctor.Check{ID: doctor.ToolCheckID(tool), Run: func(context.Context) doctor.Result {
			return checkOneTool(tool, pathDirs)
		}})
	}
	checks = append(checks,
		doctor.Check{ID: doctor.CheckStateJSON, Run: func(context.Context) doctor.Result { return checkStateJSON(homeDir) }},
		doctor.Check{ID: doctor.CheckInstalledAssetVersion, Run: func(context.Context) doctor.Result { return checkInstalledAssetVersion(homeDir) }},
		doctor.Check{ID: doctor.CheckEngramReachable, Run: func(ctx context.Context) doctor.Result { return checkEngramReachable(ctx, homeDir, installedAgents) }},
		doctor.Check{ID: doctor.CheckDiskSpace, Run: func(context.Context) doctor.Result { return checkDiskSpace(homeDir) }},
	)
	report := (doctor.Runner{Checks: checks}).Run(ctx)

	renderDoctorReport(w, report)
	return nil
}

// readDoctorInstalledAgents returns the agent IDs persisted in state.json.
// An unreadable or absent state file yields a nil slice — callers must treat
// nil/empty as "no agents selected" rather than a hard error so first-time
// installs do not surface phantom agent-missing failures.
func readDoctorInstalledAgents(homeDir string) ([]string, error) {
	s, err := state.Read(homeDir)
	if err != nil {
		return nil, err
	}
	return s.InstalledAgents, nil
}

// checkToolBinaries checks each required tool for PATH resolution and
// shadowing. The required set is coreTools plus one binary per installed
// agent ID (resolved via agentToolBinaries), so the doctor only flags agents
// the user actually selected (#709). Unknown agent IDs are skipped.
func requiredDoctorTools(installedAgents []string) []string {
	required := make([]string, 0, len(coreTools)+len(installedAgents))
	required = append(required, coreTools...)
	seen := make(map[string]struct{}, len(required))
	for _, tool := range required {
		seen[tool] = struct{}{}
	}
	for _, agentID := range installedAgents {
		bin, ok := agentToolBinaries[agentID]
		if !ok || bin == "" {
			continue
		}
		if _, duplicate := seen[bin]; duplicate {
			continue
		}
		seen[bin] = struct{}{}
		required = append(required, bin)
	}
	return required
}

func checkToolBinaries(pathDirs []string, installedAgents []string) []CheckResult {
	required := requiredDoctorTools(installedAgents)
	results := make([]CheckResult, 0, len(required))
	for _, tool := range required {
		results = append(results, checkOneTool(tool, pathDirs))
	}
	return results
}

func checkOneTool(tool string, pathDirs []string) CheckResult {
	resolved, shim, err := resolveDoctorTool(tool)
	if err != nil {
		// resolved is "" here: there is no PATH-resolved copy to name or to
		// compare against, but the executable running THIS check is still
		// independently derivable (osExecutableDoctor does not depend on
		// PATH lookup succeeding). doctorInvokedGentleAIClause("") names it
		// without fabricating a comparison that has nothing to compare
		// against (organic-dx recovery: the clause must render on every
		// derivable gentle-ai branch, not only the healthy one).
		detail := tool + " not found in PATH"
		if tool == "gentle-ai" {
			detail += doctorInvokedGentleAIClause(resolved)
		}
		return CheckResult{
			Name:   doctor.ToolCheckID(tool),
			Status: CheckStatusFail,
			Detail: detail,
			Remedy: doctor.NewRemedy(doctor.RemedyInstallTool, "Install "+tool+" or add its directory to PATH"),
		}
	}

	copies := doctorToolCopies(tool, pathDirs)
	if len(copies) > 1 {
		// The duplicate branch is exactly where ambiguity about which build
		// is running is guaranteed, so this is the branch that most needs
		// the invoked-executable clause -- it must not be dropped here.
		detail := fmt.Sprintf("%s resolved to %s but %d copies found in PATH: %s", tool, resolved, len(copies), strings.Join(copies, ", "))
		if tool == "gentle-ai" {
			detail += doctorInvokedGentleAIClause(resolved)
		}
		return CheckResult{
			Name:   doctor.ToolCheckID(tool),
			Status: CheckStatusWarn,
			Detail: detail,
			Remedy: doctor.NewRemedy(doctor.RemedyRemoveDuplicates, "Remove duplicate binaries; keep only one copy of "+tool+" in PATH"),
		}
	}

	detail := tool + " found at " + resolved
	if shim != "" {
		detail += " (" + shim + ")"
	}
	if tool == "gentle-ai" {
		detail += doctorInvokedGentleAIClause(resolved)
	}
	return CheckResult{
		Name:   doctor.ToolCheckID(tool),
		Status: CheckStatusPass,
		Detail: detail,
	}
}

// doctorInvokedGentleAIClause names the exact executable and version that is
// running THIS doctor check, alongside the PATH-resolved gentle-ai reported
// above. An RC tester who invokes gentle-ai by an absolute path may have a
// different gentle-ai earlier on PATH; without this, doctor would report only
// that other, unexercised copy as healthy, leaving the report ambiguous about
// which build was actually under test (organic-dx Phase 3f task 3f.5).
//
// It must render on every gentle-ai branch where it is derivable -- not only
// the healthy one -- since PATH duplicates are exactly the situation where
// knowing which build is actually running matters most. pathResolved may be
// "" when the tool check has no PATH-resolved copy to name (e.g. gentle-ai
// itself is not found on PATH); in that case the clause still names the
// invoked executable but skips the comparison, since there is honestly
// nothing to compare it against.
func doctorInvokedGentleAIClause(pathResolved string) string {
	invoked, err := osExecutableDoctor()
	if err != nil {
		return ""
	}
	version, _ := reviewGentleAIVersionAndCommit()
	clause := fmt.Sprintf("; invoked executable: %s (version %s)", invoked, version)
	if pathResolved != "" && !doctorSameExecutable(invoked, pathResolved) {
		clause += " -- this differs from the PATH-resolved copy above; the PATH copy's health does not describe the build actually running"
	}
	return clause
}

// doctorSameExecutable reports whether two paths resolve to the same file,
// tolerating symlinks and path formatting differences. It fails closed to
// "different" when either path cannot be resolved, so a resolution error
// never silently suppresses the mismatch warning.
func doctorSameExecutable(a, b string) bool {
	resolvedA, errA := filepath.EvalSymlinks(a)
	if errA != nil {
		resolvedA = filepath.Clean(a)
	}
	resolvedB, errB := filepath.EvalSymlinks(b)
	if errB != nil {
		resolvedB = filepath.Clean(b)
	}
	return resolvedA == resolvedB
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
	seenCopies := make(map[string]struct{}, len(pathDirs))
	copies := make([]string, 0, len(pathDirs))
	for _, dir := range pathDirs {
		if p := toolInDir(dir, tool); p != "" {
			resolved, err := filepath.EvalSymlinks(p)
			if err != nil {
				resolved = filepath.Clean(p)
			}
			if _, seen := seenCopies[resolved]; seen {
				continue
			}
			seenCopies[resolved] = struct{}{}
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
// exec.LookPath (used for the resolved path). On non-Windows platforms the
// candidate must also have at least one execute bit set — files without the
// execute bit (or directories whose name happens to match a tool, e.g. a
// PATH entry named "gentle-ai") are not counted as binaries (#709).
//
// Windows executable resolution (#177, PATHEXT gaps) is intentionally out of
// scope here; an extension match is treated as sufficient on Windows because
// LookPath already enforces PATHEXT when reporting the resolved path.
func toolInDir(dir, tool string) string {
	for _, ext := range doctorToolExecutableExts() {
		candidate := filepath.Join(dir, tool+ext)
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if doctorGOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			continue
		}
		return candidate
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
	const id = doctor.CheckStateJSON
	statePath := state.Path(homeDir)

	s, err := state.Read(homeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:   id,
				Status: CheckStatusWarn,
				Detail: "state file not found at " + statePath + " (expected for first-time install)",
				Remedy: doctor.NewRemedy(doctor.RemedyInstall, "Run 'gentle-ai install' to create initial state"),
			}
		}
		return CheckResult{
			Name:   id,
			Status: CheckStatusFail,
			Detail: "failed to parse " + statePath + ": " + err.Error(),
			Remedy: doctor.NewRemedy(doctor.RemedyRepairState, "Delete or repair "+statePath+", then re-run 'gentle-ai install'"),
		}
	}

	if len(s.InstalledAgents) == 0 {
		return CheckResult{
			Name:   id,
			Status: CheckStatusWarn,
			Detail: "state file found at " + statePath + " with no installed agents",
			Remedy: doctor.NewRemedy(doctor.RemedyInstall, "Run 'gentle-ai install' to configure agents"),
		}
	}

	var missing []string
	var dangling []string
	for _, agentID := range s.InstalledAgents {
		if dir := agentConfigDir(homeDir, agentID); dir != "" {
			info, lstatErr := os.Lstat(dir)
			if os.IsNotExist(lstatErr) {
				// A missing final path is only genuinely missing when its
				// ancestors resolve: a dangling ancestor symlink (e.g.
				// ~/.config pointing at a removed target) also yields ENOENT
				// here, and sync cannot restore a path behind a broken link.
				ancestor, ancestorErr := danglingAncestor(homeDir, dir)
				if ancestorErr != nil {
					return CheckResult{
						Name:   id,
						Status: CheckStatusWarn,
						Detail: fmt.Sprintf("managed config path %s could not be inspected: %v; inspect or repair it manually, then re-run 'gentle-ai doctor'", dir, ancestorErr),
					}
				}
				if ancestor != "" {
					dangling = append(dangling, fmt.Sprintf("%s (dangling ancestor symlink %s)", dir, ancestor))
					continue
				}
				missing = append(missing, agentID)
				continue
			}
			if lstatErr != nil {
				return CheckResult{
					Name:   id,
					Status: CheckStatusWarn,
					Detail: fmt.Sprintf("managed config path %s could not be inspected: %v; inspect or repair it manually, then re-run 'gentle-ai doctor'", dir, lstatErr),
				}
			}
			if info.Mode()&os.ModeSymlink != 0 {
				if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
					dangling = append(dangling, dir)
				} else if statErr != nil {
					return CheckResult{Name: id, Status: CheckStatusWarn, Detail: fmt.Sprintf("managed config symlink target %s could not be inspected: %v; inspect or repair it manually, then re-run 'gentle-ai doctor'", dir, statErr)}
				}
			}
		}
	}

	if len(dangling) > 0 {
		detail := fmt.Sprintf("state lists %d agent(s) whose managed config paths are dangling symlinks: %s; inspect or repair these paths manually, then re-run 'gentle-ai doctor'", len(dangling), strings.Join(dangling, ", "))
		if len(missing) > 0 {
			detail += "; genuinely absent config dirs: " + strings.Join(missing, ", ")
		}
		return CheckResult{Name: id, Status: CheckStatusWarn, Detail: detail}
	}

	if len(missing) > 0 {
		return CheckResult{
			Name:   id,
			Status: CheckStatusWarn,
			Detail: fmt.Sprintf("state lists %d agent(s) whose config dirs are missing: %s", len(missing), strings.Join(missing, ", ")),
			Remedy: doctor.NewRemedy(doctor.RemedySync, "Run 'gentle-ai sync' to restore missing config files"),
		}
	}

	return CheckResult{
		Name:   id,
		Status: CheckStatusPass,
		Detail: fmt.Sprintf("state file OK — %d agent(s) installed: %s", len(s.InstalledAgents), strings.Join(s.InstalledAgents, ", ")),
	}
}

// danglingAncestor reports the nearest existing ancestor of path — walking
// upward but staying strictly below homeDir — that is a symlink whose target
// is missing. It returns "" when every existing ancestor resolves (the path is
// then genuinely missing) and an error when an ancestor exists but cannot be
// inspected, mirroring the unreadable treatment of the final path. Ancestors
// at or above homeDir are never inspected: they are not managed by gentle-ai,
// and a broken home directory would have failed the state read already.
func danglingAncestor(homeDir, path string) (string, error) {
	home := filepath.Clean(homeDir)
	boundary := home + string(filepath.Separator)
	for ancestor := filepath.Dir(filepath.Clean(path)); ancestor != home && strings.HasPrefix(ancestor, boundary); ancestor = filepath.Dir(ancestor) {
		info, err := os.Lstat(ancestor)
		if os.IsNotExist(err) {
			continue // ancestor missing too; keep walking up
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			if info.IsDir() {
				return "", nil // nearest existing ancestor is real; path is genuinely missing
			}
			// A non-directory ancestor blocks both inspection and restoration:
			// sync cannot mkdir below a regular file. POSIX surfaces this as
			// ENOTDIR at the final lstat, but Windows reports it as not-exist,
			// which is how the walk gets here.
			return "", fmt.Errorf("ancestor %s is not a directory", ancestor) // refusal:by-design world-action: the caller embeds this cause in a warn that already names the continuation (inspect or repair the path, re-run 'gentle-ai doctor'); the repair itself happens on the filesystem, not through a command
		}
		if _, err := os.Stat(ancestor); os.IsNotExist(err) {
			return ancestor, nil
		} else if err != nil {
			return "", err
		}
		return "", nil // symlink resolves; path is genuinely missing below it
	}
	return "", nil
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

// checkEngramReachable probes the configured Engram transport. An explicit
// ENGRAM_BASE_URL selects HTTP; otherwise the doctor reads the stdio command
// and arguments persisted in installed agent configurations. It never guesses
// an HTTP address or replaces a persisted command with its current PATH.
func checkEngramReachable(ctx context.Context, homeDir string, installedAgents []string) CheckResult {
	const id = doctor.CheckEngramReachable

	if baseURL := strings.TrimSpace(os.Getenv(engramHealthEnvVar)); baseURL != "" {
		return checkEngramHTTP(id, baseURL)
	}

	commands, err := engram.ReadPersistedStdioCommands(homeDir, installedAgents)
	if err != nil {
		return CheckResult{
			Name:   id,
			Status: CheckStatusFail,
			Detail: "engram MCP persisted configuration is invalid: " + err.Error(),
			Remedy: doctor.NewRemedy(doctor.RemedyInspectEngram, "Repair the persisted Engram MCP configuration, then run 'gentle-ai sync'"),
		}
	}
	if len(commands) == 0 {
		return CheckResult{
			Name:   id,
			Status: CheckStatusWarn,
			Detail: "engram MCP not probed: no persisted MCP configuration found for installed agents",
			Remedy: doctor.NewRemedy(doctor.RemedySync, "Run 'gentle-ai sync' to restore the Engram MCP configuration"),
		}
	}

	sources := make([]string, 0, len(commands))
	for _, command := range commands {
		err := engramProbeStdioFn(ctx, command.Timeout, command.Command, command.Args...)
		switch {
		case errors.Is(err, engram.ErrNotInstalled):
			return CheckResult{
				Name:   id,
				Status: CheckStatusWarn,
				Detail: "engram MCP not probed: persisted command in " + command.Source + " is not found on PATH (see the tool:engram check)",
			}
		// A deadline that elapsed is not evidence that the transport is
		// broken, and #3068 showed what the old wording cost: the reporter's
		// arguments were correct and their store answered in about five
		// seconds, but they were told the handshake failed and sent to inspect
		// the command and arguments, which the evidence did not implicate.
		// Failing stays, because a probe that never answered is not a pass.
		case errors.Is(err, context.DeadlineExceeded):
			return CheckResult{
				Name:   id,
				Status: CheckStatusFail,
				Detail: "engram MCP (stdio) did not answer within " + engram.StdioProbeDeadline(command.Timeout).String() +
					" for persisted configuration " + command.Source,
				Remedy: doctor.NewRemedy(doctor.RemedyInspectEngram,
					"Raise the Engram MCP server's timeout in "+command.Source+" if the store is simply slow to start, or check whether the process is hanging"),
			}
		case err != nil:
			return CheckResult{
				Name:   id,
				Status: CheckStatusFail,
				Detail: "engram MCP (stdio) initialize handshake failed for persisted configuration " + command.Source + ": " + err.Error(),
				Remedy: doctor.NewRemedy(doctor.RemedyInspectEngram, "Check the Engram MCP command and arguments in "+command.Source),
			}
		}
		sources = append(sources, command.Source)
	}

	return CheckResult{
		Name:   id,
		Status: CheckStatusPass,
		Detail: "engram MCP (stdio) answered the initialize handshake for persisted configuration: " + strings.Join(sources, ", "),
	}
}

// checkEngramHTTP probes the HTTP deployment the user declared via
// ENGRAM_BASE_URL. It never invents a URL of its own.
func checkEngramHTTP(id doctor.CheckID, baseURL string) CheckResult {
	baseURL = strings.TrimSpace(baseURL)
	healthURL := strings.TrimRight(baseURL, "/") + "/health"

	statusCode, err := httpGetFn(healthURL, 3*time.Second)
	if err != nil {
		return CheckResult{
			Name:   id,
			Status: CheckStatusFail,
			Detail: "engram health endpoint unreachable at " + healthURL + " (from " + engramHealthEnvVar + "): " + err.Error(),
			Remedy: doctor.NewRemedy(doctor.RemedyStartEngram, "Start 'engram serve' or fix "+engramHealthEnvVar),
		}
	}
	if statusCode < 200 || statusCode >= 300 {
		return CheckResult{
			Name:   id,
			Status: CheckStatusWarn,
			Detail: fmt.Sprintf("engram health endpoint %s returned HTTP %d", healthURL, statusCode),
			Remedy: doctor.NewRemedy(doctor.RemedyInspectEngram, "Check engram logs for errors"),
		}
	}
	return CheckResult{
		Name:   id,
		Status: CheckStatusPass,
		Detail: fmt.Sprintf("engram health endpoint OK at %s (HTTP %d)", healthURL, statusCode),
	}
}

// checkDiskSpace reports free space on the ~/.gentle-ai filesystem.
func checkDiskSpace(homeDir string) CheckResult {
	const id = doctor.CheckDiskSpace
	dir := filepath.Join(homeDir, ".gentle-ai")

	free, err := availableBytesFn(dir)
	if err != nil {
		return CheckResult{Name: id, Status: CheckStatusWarn, Detail: "could not determine free disk space for " + dir + ": " + err.Error()}
	}

	freeMB := free / (1024 * 1024)
	switch {
	case free < diskFailThreshold:
		return CheckResult{
			Name:   id,
			Status: CheckStatusFail,
			Detail: fmt.Sprintf("critically low disk space: %d MB free on %s filesystem", freeMB, dir),
			Remedy: doctor.NewRemedy(doctor.RemedyFreeDiskSpace, "Free up disk space before running install or sync operations"),
		}
	case free < diskWarnThreshold:
		return CheckResult{
			Name:   id,
			Status: CheckStatusWarn,
			Detail: fmt.Sprintf("low disk space: %d MB free on %s filesystem", freeMB, dir),
			Remedy: doctor.NewRemedy(doctor.RemedyFreeDiskSpace, "Consider freeing disk space"),
		}
	default:
		return CheckResult{
			Name:   id,
			Status: CheckStatusPass,
			Detail: fmt.Sprintf("%d MB free on %s filesystem", freeMB, dir),
		}
	}
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
		if c.Remedy != nil {
			fmt.Fprintf(w, "       Remedy: %s\n", c.Remedy.Description)
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

func checkInstalledAssetVersion(homeDir string) CheckResult {
	s, err := state.Read(homeDir)
	if err != nil {
		return CheckResult{
			Status: CheckStatusPass,
			Detail: "no state file found — asset version check skipped",
		}
	}
	if s.InstalledBinaryVersion == "" {
		return CheckResult{
			Status: CheckStatusPass,
			Detail: "no installed binary version recorded in state file — check skipped",
		}
	}
	if s.InstalledBinaryVersion != AppVersion {
		return CheckResult{
			Status: CheckStatusWarn,
			Detail: fmt.Sprintf("installed assets were configured by gentle-ai %s, but running binary is %s — run 'gentle-ai sync' to update installed assets", s.InstalledBinaryVersion, AppVersion),
		}
	}
	return CheckResult{
		Status: CheckStatusPass,
		Detail: fmt.Sprintf("installed assets match running binary version (%s)", AppVersion),
	}
}
