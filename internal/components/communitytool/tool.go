package communitytool

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	piagent "github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

type Availability string

const (
	AvailabilityAvailable Availability = "available"
	AvailabilityMissing   Availability = "missing"
)

type AgentStatusKind string

const (
	AgentStatusUnavailable AgentStatusKind = "unavailable"
	AgentStatusConfigured  AgentStatusKind = "configured"
	AgentStatusMissing     AgentStatusKind = "missing"
)

type Definition struct {
	ID          model.CommunityToolID
	Name        string
	PackageName string
	CommandName string
	RepoURL     string
	Description string
}

type Result struct {
	Tool          model.CommunityToolID
	CommandsRun   []string
	ManualActions []string
	StatusBefore  *Status
	StatusAfter   *Status
	PiCodeGraph   *PiCodeGraphResult
}

type Status struct {
	Tool      model.CommunityToolID
	CLI       Availability
	CLIPath   string
	Agents    []AgentStatus
	FollowUps []string
}

type AgentStatus struct {
	Agent      model.AgentID
	Name       string
	Status     AgentStatusKind
	Detected   bool
	Configured bool
	Path       string
	Reason     string
	Children   []PiCodeGraphChild
}

type Detector interface {
	LookPath(name string) (string, error)
}

type DetectorFunc func(name string) (string, error)

func (fn DetectorFunc) LookPath(name string) (string, error) { return fn(name) }

type Runner interface {
	Run(name string, args ...string) error
}

type RunnerFunc func(name string, args ...string) error

func (fn RunnerFunc) Run(name string, args ...string) error { return fn(name, args...) }

var (
	codeGraphPackageLookPath = exec.LookPath
	codeGraphPnpmGlobalBin   = defaultPnpmGlobalBin
)

var definitions = []Definition{
	{
		ID:          model.CommunityToolCodeGraph,
		Name:        "CodeGraph",
		PackageName: "@colbymchenry/codegraph@latest",
		CommandName: "codegraph",
		RepoURL:     "https://github.com/colbymchenry/codegraph",
		Description: "Code graph indexing and MCP wiring for supported coding agents",
	},
}

func Definitions() []Definition {
	out := make([]Definition, len(definitions))
	copy(out, definitions)
	return out
}

func DefinitionFor(id model.CommunityToolID) (Definition, bool) {
	for _, def := range definitions {
		if def.ID == id {
			return def, true
		}
	}
	return Definition{}, false
}

func Install(id model.CommunityToolID, workspaceDir string, runner Runner) (Result, error) {
	return InstallWithHome(id, workspaceDir, defaultHomeDir(), runner, DetectorFunc(exec.LookPath))
}

func InstallWithHome(id model.CommunityToolID, workspaceDir string, homeDir string, runner Runner, detector Detector) (Result, error) {
	if runner == nil {
		return Result{}, fmt.Errorf("community tool runner is not configured")
	}
	def, ok := DefinitionFor(id)
	if !ok {
		return Result{}, fmt.Errorf("unknown community tool %q", id)
	}
	if def.ID != model.CommunityToolCodeGraph {
		return Result{}, fmt.Errorf("community tool %q is not supported", id)
	}

	result := Result{Tool: id}
	before := DetectStatus(id, homeDir, detector)
	result.StatusBefore = &before
	snapshots, err := snapshotCodeGraphPaths(CodeGraphManagedPaths(homeDir))
	if err != nil {
		return result, err
	}
	rollback := func(cause error) (Result, error) {
		if restoreErr := restoreCodeGraphPaths(snapshots); restoreErr != nil {
			return result, fmt.Errorf("%w; rollback CodeGraph configuration: %v", cause, restoreErr)
		}
		return result, cause
	}
	if before.CodeGraphReconcileSatisfied() || codeGraphCanRepairWithoutFullInstall(homeDir, before) {
		if NeedsOpenCodeCodeGraphReconcile(homeDir) {
			result.CommandsRun = append(result.CommandsRun, "codegraph install --target opencode --location global --yes")
		}
		openCodeResult, err := ReconcileOpenCodeCodeGraph(homeDir, runner)
		if err != nil {
			return rollback(err)
		}
		guidanceResult, err := InjectCodeGraphGuidanceIfSelected(homeDir, []model.CommunityToolID{id})
		if err != nil {
			return rollback(err)
		}
		piResult, err := reconcileDetectedPiCodeGraph(homeDir, workspaceDir)
		if err != nil {
			return rollback(err)
		}
		result.PiCodeGraph = piResult
		if piResult != nil {
			result.ManualActions = append(result.ManualActions, piResult.ManualActions...)
		}
		after := DetectStatus(id, homeDir, detector)
		result.StatusAfter = &after
		if err := validateCodeGraphInstallStatus(after); err != nil {
			return rollback(err)
		}
		if openCodeResult.Changed || guidanceResult.Changed {
			result.ManualActions = append(result.ManualActions, "CodeGraph is already available and MCP-configured. Agent guidance was updated so enabled agents lazily initialize project indexes when needed.")
		} else {
			result.ManualActions = append(result.ManualActions, "CodeGraph is already available and configured for all detected supported agents. No changes were needed.")
		}
		return result, nil
	}

	targets := detectedCodeGraphTargets(homeDir)
	commands := make([][]string, 0, 2)
	if len(targets) > 0 {
		commands = append(commands, []string{"codegraph", "install", "--target", strings.Join(targets, ","), "--location", "global", "--yes"})
	}
	if before.CLI != AvailabilityAvailable {
		var err error
		commands, err = CodeGraphCommandsForDetectorAndTargets(DetectorFunc(codeGraphPackageLookPath), targets)
		if err != nil {
			return result, err
		}
		if len(targets) == 0 {
			commands = commands[:1]
		}
	}
	for _, command := range commands {
		if len(command) == 0 {
			continue
		}
		result.CommandsRun = append(result.CommandsRun, strings.Join(command, " "))
		if err := runner.Run(command[0], command[1:]...); err != nil {
			return rollback(fmt.Errorf("run %q: %w", strings.Join(command, " "), err))
		}
	}
	if _, err := ReconcileOpenCodeCodeGraph(homeDir, runner); err != nil {
		return rollback(err)
	}
	if _, err := InjectCodeGraphGuidanceIfSelected(homeDir, []model.CommunityToolID{id}); err != nil {
		return rollback(err)
	}
	piResult, err := reconcileDetectedPiCodeGraph(homeDir, workspaceDir)
	if err != nil {
		return rollback(err)
	}
	result.PiCodeGraph = piResult
	if piResult != nil {
		result.ManualActions = append(result.ManualActions, piResult.ManualActions...)
	}
	after := DetectStatus(id, homeDir, detector)
	result.StatusAfter = &after
	if err := validateCodeGraphInstallStatus(after); err != nil {
		return rollback(err)
	}
	result.ManualActions = append(result.ManualActions, "CodeGraph CLI was installed and supported agents were connected. Project indexes will be created automatically when an enabled agent opens inside a project.")
	return result, nil
}

func codeGraphCanRepairWithoutFullInstall(homeDir string, status Status) bool {
	if status.CLI != AvailabilityAvailable {
		return false
	}
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		return false
	}
	foundMissing := false
	for _, agent := range status.Agents {
		if !agent.Detected || agent.Configured || agent.Agent == model.AgentPi {
			continue
		}
		foundMissing = true
		if agent.Agent == model.AgentOpenCode {
			continue
		}
		adapter, ok := reg.Get(agent.Agent)
		if !ok {
			return false
		}
		if _, wired := hasCodeGraphToolWiring(homeDir, adapter); !wired {
			return false
		}
	}
	return foundMissing
}

func reconcileDetectedPiCodeGraph(homeDir, workspaceDir string) (*PiCodeGraphResult, error) {
	paths := piagent.CodeGraphPaths(homeDir)
	if _, err := os.Stat(paths.AgentDir); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	result, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: homeDir, WorkspaceDir: workspaceDir, Selected: true})
	result, err = PreservePiCodeGraphPending(result, err)
	return &result, err
}

func validateCodeGraphInstallStatus(status Status) error {
	if status.Tool != model.CommunityToolCodeGraph {
		return nil
	}
	if status.CLI != AvailabilityAvailable {
		return fmt.Errorf("CodeGraph install did not leave the codegraph CLI available")
	}
	missing := make([]string, 0)
	for _, agent := range status.Agents {
		if agent.Agent == model.AgentPi {
			continue
		}
		if agent.Detected && !agent.Configured {
			missing = append(missing, agent.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("CodeGraph install did not configure detected supported agents: %s", strings.Join(missing, ", "))
	}
	return nil
}

func DetectStatus(id model.CommunityToolID, homeDir string, detector Detector) Status {
	status := Status{Tool: id, CLI: AvailabilityMissing}
	def, ok := DefinitionFor(id)
	if !ok || id != model.CommunityToolCodeGraph {
		status.FollowUps = append(status.FollowUps, fmt.Sprintf("status detection for %q is not implemented", id))
		return status
	}
	if detector == nil {
		detector = DetectorFunc(exec.LookPath)
	}
	if path, err := detector.LookPath(def.CommandName); err == nil && strings.TrimSpace(path) != "" {
		status.CLI = AvailabilityAvailable
		status.CLIPath = path
	}
	status.Agents = detectCodeGraphAgents(homeDir)
	status.FollowUps = append(status.FollowUps, "CodeGraph markers can vary by upstream version; detection currently checks conservative MCP entries and instruction markers containing codegraph.")
	return status
}

func (s Status) CodeGraphReconcileSatisfied() bool {
	if s.Tool != model.CommunityToolCodeGraph || s.CLI != AvailabilityAvailable {
		return false
	}
	for _, agent := range s.Agents {
		if agent.Detected && !agent.Configured {
			return false
		}
	}
	return true
}

func (s Status) DetectedConfiguredMissingCounts() (detected, configured, missing int) {
	for _, agent := range s.Agents {
		if !agent.Detected {
			continue
		}
		detected++
		if agent.Configured {
			configured++
		} else {
			missing++
		}
	}
	return detected, configured, missing
}

func detectCodeGraphAgents(homeDir string) []AgentStatus {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		return nil
	}
	supported := codeGraphSupportedAgents()
	installed := agents.DiscoverInstalled(reg, homeDir)
	installedByID := make(map[model.AgentID]string, len(installed))
	for _, agent := range installed {
		installedByID[agent.ID] = agent.ConfigDir
	}

	result := make([]AgentStatus, 0, len(supported))
	for _, id := range supported {
		adapter, ok := reg.Get(id)
		if !ok {
			continue
		}
		name := agentDisplayName(id)
		configDir, detected := installedByID[id]
		state := AgentStatus{
			Agent:    id,
			Name:     name,
			Detected: detected,
			Path:     configDir,
			Status:   AgentStatusUnavailable,
			Reason:   "agent config directory was not detected",
		}
		if detected {
			configured, markerPath, reason := hasCodeGraphWiring(homeDir, adapter)
			if id == model.AgentPi {
				configured, reason, state.Children = inspectPiCodeGraph(homeDir, "")
			}
			state.Configured = configured
			state.Path = markerPath
			state.Reason = reason
			if configured {
				state.Status = AgentStatusConfigured
			} else {
				state.Status = AgentStatusMissing
			}
		}
		result = append(result, state)
	}
	return result
}

func codeGraphSupportedAgents() []model.AgentID {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		return nil
	}
	ids := reg.SupportedAgents()
	ids = slices.DeleteFunc(ids, func(id model.AgentID) bool { return !isCodeGraphCompatibleAgent(id) })
	return ids
}

func hasCodeGraphWiring(homeDir string, adapter agents.Adapter) (bool, string, string) {
	guidancePath := codeGraphGuidancePath(homeDir, adapter)
	if adapter.Agent() == model.AgentPi {
		configured, reason := PiCodeGraphConfigured(homeDir, "")
		return configured, piCodeGraphStatusPath(homeDir), reason
	}

	toolPath, hasToolWiring := hasCodeGraphToolWiring(homeDir, adapter)
	if adapter.SupportsSystemPrompt() {
		if !hasCodeGraphGuidance(guidancePath) {
			return false, guidancePath, "detected agent but no CodeGraph guidance marker was found in the system prompt file"
		}
		if !hasToolWiring {
			return false, guidancePath, "found CodeGraph guidance but no effective MCP/tool wiring"
		}
		return true, toolPath, "found CodeGraph guidance and MCP/tool wiring"
	}
	if hasToolWiring {
		return true, toolPath, "found CodeGraph tool wiring marker"
	}
	return false, adapter.GlobalConfigDir(homeDir), "detected agent but no CodeGraph MCP or instruction marker was found"
}

func piCodeGraphStatusPath(homeDir string) string {
	return piagent.CodeGraphPaths(homeDir).Manifest
}

func hasDetectedCodeGraphToolWiring(homeDir string) bool {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		return false
	}
	for _, installedAgent := range agents.DiscoverInstalled(reg, homeDir) {
		adapter, ok := reg.Get(installedAgent.ID)
		if !ok || !isCodeGraphCompatibleAgent(installedAgent.ID) {
			continue
		}
		if _, ok := hasCodeGraphToolWiring(homeDir, adapter); ok {
			return true
		}
	}
	return false
}

func agentDisplayName(id model.AgentID) string {
	for _, agent := range catalog.AllAgents() {
		if agent.ID == id {
			return agent.Name
		}
	}
	return string(id)
}

func defaultHomeDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return os.Getenv("HOME")
}

func CodeGraphCommands() [][]string {
	return codeGraphCommands("npm", nil)
}

func CodeGraphCommandsForDetector(detector Detector) ([][]string, error) {
	return CodeGraphCommandsForDetectorAndTargets(detector, nil)
}

func CodeGraphCommandsForDetectorAndTargets(detector Detector, targets []string) ([][]string, error) {
	packageManager, err := detectCodeGraphPackageManager(detector)
	if err != nil {
		return nil, err
	}
	return codeGraphCommands(packageManager, targets), nil
}

func defaultPnpmGlobalBin() (string, error) {
	output, err := exec.Command("pnpm", "bin", "-g").CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("pnpm global binary directory is not usable: %s", message)
	}
	return strings.TrimSpace(string(output)), nil
}

func detectCodeGraphPackageManager(detector Detector) (string, error) {
	if detector == nil {
		detector = DetectorFunc(exec.LookPath)
	}
	if _, err := detector.LookPath("npm"); err == nil {
		return "npm", nil
	}
	if _, err := detector.LookPath("pnpm"); err == nil {
		globalBin, binErr := codeGraphPnpmGlobalBin()
		if binErr != nil {
			return "", fmt.Errorf("CodeGraph installation found pnpm, but pnpm global installs are not ready. Run `pnpm setup`, restart your shell, then rerun Gentle AI: %w", binErr)
		}
		if globalBin == "" {
			return "", fmt.Errorf("CodeGraph installation found pnpm, but `pnpm bin -g` returned an empty global binary directory. Run `pnpm setup`, restart your shell, then rerun Gentle AI")
		}
		return "pnpm", nil
	}
	return "", fmt.Errorf("CodeGraph installation requires either `npm` or `pnpm` in PATH")
}

func codeGraphCommands(packageManager string, targets []string) [][]string {
	installCommand := []string{"npm", "install", "-g", "@colbymchenry/codegraph@latest"}
	if packageManager == "pnpm" {
		installCommand = []string{"pnpm", "add", "-g", "@colbymchenry/codegraph@latest"}
	}
	install := []string{"codegraph", "install", "--yes"}
	if len(targets) > 0 {
		install = []string{"codegraph", "install", "--target", strings.Join(targets, ","), "--location", "global", "--yes"}
	}
	return [][]string{installCommand, install}
}
