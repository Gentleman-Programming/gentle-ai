package communitytool

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

const (
	// rtkHTTPTimeout bounds a single pinned release request well below the
	// Community Tool verification budget while leaving enough time for a normal
	// archive download.
	rtkHTTPTimeout       = 30 * time.Second
	rtkMaxStatusFileSize = 64 << 10
)

var (
	rtkGOOS                               = runtime.GOOS
	rtkGOARCH                             = runtime.GOARCH
	rtkHTTPClientForInstall rtkHTTPClient = &http.Client{Timeout: rtkHTTPTimeout}
)

type rtkEnvironmentRunner interface {
	Runner
	RunWithEnv(environment map[string]string, name string, args ...string) error
}

func rtkPlatformForRuntime() rtkPlatform {
	return rtkPlatform(rtkGOOS + "/" + rtkGOARCH)
}

func rtkAssetForRuntime() (rtkCandidateAsset, bool) {
	platform := rtkPlatformForRuntime()
	if !slices.Contains(rtkEnabledPlatforms, platform) {
		return rtkCandidateAsset{}, false
	}
	for _, asset := range rtkCandidateAssets {
		if asset.Platform == platform {
			return asset, true
		}
	}
	return rtkCandidateAsset{}, false
}

func rtkInstallPath(homeDir string) string { return filepath.Join(homeDir, ".local", "bin", "rtk") }

func rtkPiAgentDir(homeDir string) string {
	if configured := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); configured != "" {
		return configured
	}
	return filepath.Join(homeDir, ".pi", "agent")
}

func RTKManagedPaths(homeDir string) []string {
	return rtkManagedPaths(homeDir, rtkDetectedAgents(homeDir))
}

func RTKManagedPathsForAgents(homeDir string, selected []model.AgentID) []string {
	return rtkManagedPaths(homeDir, rtkScopedDetectedAgents(homeDir, selected))
}

func rtkManagedPaths(homeDir string, agents []model.AgentID) []string {
	paths := []string{rtkInstallPath(homeDir)}
	for _, agent := range agents {
		switch agent {
		case model.AgentClaudeCode:
			paths = append(paths, filepath.Join(homeDir, ".claude", "RTK.md"), filepath.Join(homeDir, ".claude", "CLAUDE.md"), filepath.Join(homeDir, ".claude", "settings.json"))
		case model.AgentOpenCode:
			paths = append(paths, filepath.Join(homeDir, ".config", "opencode", "plugins", "rtk.ts"))
		case model.AgentCodex:
			paths = append(paths, filepath.Join(homeDir, ".codex", "RTK.md"), filepath.Join(homeDir, ".codex", "AGENTS.md"))
		case model.AgentPi:
			paths = append(paths, filepath.Join(rtkPiAgentDir(homeDir), "extensions", "rtk.ts"))
		}
	}
	return uniqueSortedPaths(paths)
}

func uniqueSortedPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, candidate := range paths {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	slices.Sort(out)
	return out
}

func rtkDetectedAgents(homeDir string) []model.AgentID {
	registry, err := agents.NewDefaultRegistry()
	if err != nil {
		return nil
	}
	present := make(map[model.AgentID]bool)
	for _, candidate := range agents.DiscoverSelected(registry, homeDir) {
		present[candidate.ID] = true
	}
	order := []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode, model.AgentCodex, model.AgentPi}
	out := make([]model.AgentID, 0, len(order))
	for _, agent := range order {
		if present[agent] {
			out = append(out, agent)
		}
	}
	return out
}

func rtkScopedDetectedAgents(homeDir string, selected []model.AgentID) []model.AgentID {
	out := make([]model.AgentID, 0, len(selected))
	for _, agent := range rtkDetectedAgents(homeDir) {
		if slices.Contains(selected, agent) {
			out = append(out, agent)
		}
	}
	return out
}

func rtkSetupArgs(agent model.AgentID) ([]string, bool) {
	for _, contract := range rtkSetupContracts {
		if contract.Agent == agent {
			return strings.Fields(contract.Invocation), true
		}
	}
	return nil, false
}

func installRTKForAgents(homeDir string, runner Runner, detector Detector, selected []model.AgentID, scoped bool) (Result, error) {
	result := Result{Tool: model.CommunityToolRTK}
	targets := rtkDetectedAgents(homeDir)
	if scoped {
		targets = rtkScopedDetectedAgents(homeDir, selected)
		if len(targets) == 0 {
			return result, fmt.Errorf("RTK has no selected supported detected agents; select an installed supported agent before setup")
		}
	}
	asset, admitted := rtkAssetForRuntime()
	if !admitted {
		return result, fmt.Errorf("RTK is unavailable on %s/%s; Windows and unsupported platforms are not admitted", rtkGOOS, rtkGOARCH)
	}
	if runner == nil {
		return result, fmt.Errorf("community tool runner is not configured")
	}
	if _, ok := runner.(rtkEnvironmentRunner); !ok {
		return result, fmt.Errorf("RTK runner does not support per-process telemetry-disabled environment")
	}
	before := detectRTKStatus(homeDir, detector)
	result.StatusBefore = &before
	managedPaths := RTKManagedPaths(homeDir)
	if scoped {
		managedPaths = RTKManagedPathsForAgents(homeDir, selected)
	}
	snapshots, err := snapshotCodeGraphPaths(managedPaths)
	if err != nil {
		return result, err
	}
	rollback := func(cause error) (Result, error) {
		if restoreErr := restoreCodeGraphPaths(snapshots); restoreErr != nil {
			return result, fmt.Errorf("%w; rollback RTK configuration: %v", cause, restoreErr)
		}
		return result, cause
	}
	installed := rtkInstallPath(homeDir)
	if _, err := os.Lstat(installed); os.IsNotExist(err) {
		stage, stageErr := os.MkdirTemp("", "gentle-ai-rtk-*")
		if stageErr != nil {
			return result, fmt.Errorf("create RTK staging directory: %w", stageErr)
		}
		defer os.RemoveAll(stage)
		binary, acquireErr := acquireRTKCandidate(rtkHTTPClientForInstall, stage, asset)
		if acquireErr != nil {
			return rollback(acquireErr)
		}
		if err := os.MkdirAll(filepath.Dir(installed), 0o755); err != nil {
			return rollback(fmt.Errorf("create RTK install directory: %w", err))
		}
		partial := installed + ".tmp"
		if err := os.Remove(partial); err != nil && !os.IsNotExist(err) {
			return rollback(fmt.Errorf("clear RTK partial output: %w", err))
		}
		binaryBytes, err := os.ReadFile(binary)
		if err != nil {
			return rollback(fmt.Errorf("read extracted RTK executable: %w", err))
		}
		if err := os.WriteFile(partial, binaryBytes, 0o700); err != nil {
			return rollback(fmt.Errorf("write RTK executable: %w", err))
		}
		if err := os.Rename(partial, installed); err != nil {
			_ = os.Remove(partial)
			return rollback(fmt.Errorf("install RTK executable: %w", err))
		}
		if err := verifyExistingRTKBinary(installed, asset); err != nil {
			return rollback(fmt.Errorf("verify installed RTK executable: %w", err))
		}
		result.CommandsRun = append(result.CommandsRun, "download "+asset.URL)
	} else if err != nil {
		return rollback(fmt.Errorf("inspect RTK install path: %w", err))
	} else if err := verifyExistingRTKBinary(installed, asset); err != nil {
		return rollback(err)
	}

	for _, agent := range targets {
		args, ok := rtkSetupArgs(agent)
		if !ok {
			continue
		}
		result.CommandsRun = append(result.CommandsRun, "rtk "+strings.Join(args, " "))
		if err := runRTKWithTelemetryDisabled(homeDir, runner, installed, args...); err != nil {
			return rollback(fmt.Errorf("configure RTK for %s: %w", agentDisplayName(agent), err))
		}
	}
	after := detectRTKStatus(homeDir, detector)
	result.StatusAfter = &after
	if after.CLI != AvailabilityAvailable {
		result.ManualActions = append(result.ManualActions, `RTK was installed to ~/.local/bin/rtk. Add it to PATH, then restart your shell: export PATH="$HOME/.local/bin:$PATH"`)
	}
	return result, nil
}

func runRTKWithTelemetryDisabled(homeDir string, runner Runner, name string, args ...string) error {
	envRunner := runner.(rtkEnvironmentRunner)
	return envRunner.RunWithEnv(map[string]string{
		"HOME":                   homeDir,
		"XDG_CONFIG_HOME":        filepath.Join(homeDir, ".config"),
		"RTK_TELEMETRY_DISABLED": "1",
	}, name, args...)
}

func verifyExistingRTKBinary(path string, asset rtkCandidateAsset) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect existing RTK binary %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0) {
		return fmt.Errorf("cannot verify existing RTK at %q: it must be a regular executable pinned to %s; remove it and rerun Gentle AI", path, rtkReleaseTag)
	}
	if asset.ExecutableSizeBytes <= 0 || len(asset.ExecutableSHA256) != 64 || info.Size() != asset.ExecutableSizeBytes {
		return fmt.Errorf("cannot verify existing RTK at %q is pinned to %s: remove it and rerun Gentle AI to install the verified release", path, rtkReleaseTag)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot verify existing RTK at %q: remove it and rerun Gentle AI: %w", path, err)
	}
	defer file.Close()
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, asset.ExecutableSizeBytes+1))
	if err != nil || written != asset.ExecutableSizeBytes || hex.EncodeToString(hasher.Sum(nil)) != asset.ExecutableSHA256 {
		return fmt.Errorf("cannot verify existing RTK at %q is pinned to %s: remove it and rerun Gentle AI to install the verified release", path, rtkReleaseTag)
	}
	return nil
}

func detectRTKStatus(homeDir string, detector Detector) Status {
	status := Status{Tool: model.CommunityToolRTK, CLI: AvailabilityMissing}
	asset, admitted := rtkAssetForRuntime()
	if !admitted {
		status.FollowUps = append(status.FollowUps, fmt.Sprintf("RTK is unavailable on %s/%s; Windows remains disabled.", rtkGOOS, rtkGOARCH))
		return status
	}
	if detector == nil {
		detector = DetectorFunc(defaultRTKLookPath)
	}
	if path, err := detector.LookPath("rtk"); err == nil && rtkResolvedPathMatchesManaged(homeDir, path) && verifyExistingRTKBinary(rtkInstallPath(homeDir), asset) == nil {
		status.CLI, status.CLIPath = AvailabilityAvailable, rtkInstallPath(homeDir)
	}
	for _, agent := range rtkDetectedAgents(homeDir) {
		configured, path, reason := rtkAgentConfigured(homeDir, agent)
		state := AgentStatus{Agent: agent, Name: agentDisplayName(agent), Detected: true, Configured: configured, Path: path, Reason: reason, Status: AgentStatusMissing}
		if status.CLI != AvailabilityAvailable {
			state.Configured = false
			state.Reason = "RTK is not effective on PATH"
		} else if configured {
			state.Status = AgentStatusConfigured
		}
		status.Agents = append(status.Agents, state)
	}
	return status
}

func defaultRTKLookPath(name string) (string, error) { return exec.LookPath(name) }

func rtkResolvedPathMatchesManaged(homeDir, resolved string) bool {
	resolved = strings.TrimSpace(resolved)
	if resolved == "" {
		return false
	}
	managed := filepath.Clean(rtkInstallPath(homeDir))
	if filepath.IsAbs(resolved) {
		return filepath.Clean(resolved) == managed
	}
	// A detector seam may return a home-relative path. Anchor it to the managed
	// home rather than the ambient process directory, so relative shadows cannot
	// gain effective-status acceptance.
	return filepath.Clean(filepath.Join(homeDir, resolved)) == managed
}

func rtkAgentConfigured(homeDir string, agent model.AgentID) (bool, string, string) {
	var paths []string
	switch agent {
	case model.AgentClaudeCode:
		rtk, claude, settings := filepath.Join(homeDir, ".claude", "RTK.md"), filepath.Join(homeDir, ".claude", "CLAUDE.md"), filepath.Join(homeDir, ".claude", "settings.json")
		if containsFile(rtk, "") && containsFile(claude, "@RTK.md") && containsAllFile(settings, "PreToolUse", "rtk hook claude") {
			return true, rtk, "found RTK instruction and Claude hook"
		}
		paths = []string{rtk, claude, settings}
	case model.AgentOpenCode:
		path := filepath.Join(homeDir, ".config", "opencode", "plugins", "rtk.ts")
		if containsFile(path, "tool.execute.before") && containsFile(path, "rtk rewrite") {
			return true, path, "found RTK OpenCode plugin marker"
		}
		paths = []string{path}
	case model.AgentCodex:
		rtk, agentsPath := filepath.Join(homeDir, ".codex", "RTK.md"), filepath.Join(homeDir, ".codex", "AGENTS.md")
		if containsFile(rtk, "") && containsFile(agentsPath, rtk) {
			return true, rtk, "found RTK global instruction reference"
		}
		paths = []string{rtk, agentsPath}
	case model.AgentPi:
		path := filepath.Join(rtkPiAgentDir(homeDir), "extensions", "rtk.ts")
		if containsFile(path, `exec("rtk", ["rewrite"`) {
			return true, path, "found RTK Pi extension marker"
		}
		paths = []string{path}
	}
	return false, strings.Join(paths, ", "), "RTK setup marker was not found"
}

func containsFile(path, fragment string) bool {
	data, ok := readRTKStatusFile(path)
	return ok && (fragment == "" || strings.Contains(string(data), fragment))
}

func containsAllFile(path string, fragments ...string) bool {
	data, ok := readRTKStatusFile(path)
	if !ok {
		return false
	}
	for _, fragment := range fragments {
		if !strings.Contains(string(data), fragment) {
			return false
		}
	}
	return true
}

func readRTKStatusFile(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > rtkMaxStatusFileSize {
		return nil, false
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, rtkMaxStatusFileSize+1))
	if err != nil || int64(len(data)) > rtkMaxStatusFileSize {
		return nil, false
	}
	return data, true
}
