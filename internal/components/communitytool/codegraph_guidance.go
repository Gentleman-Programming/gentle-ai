package communitytool

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

const (
	codeGraphGuidanceSectionID   = "codegraph-guidance"
	legacyCodeGraphGuidanceStart = "<!-- CODEGRAPH_START -->"
	legacyCodeGraphGuidanceEnd   = "<!-- CODEGRAPH_END -->"
	upstreamCodeGraphSkipPhrase  = "If there is no `.codegraph/` directory, skip CodeGraph entirely"
	upstreamCodeGraphOrderPhrase = "BEFORE grep/find or reading files"
)

// GuidanceInjectionResult describes the managed agent-instruction updates made
// when the CodeGraph community tool is enabled.
type GuidanceInjectionResult struct {
	Changed bool
	Files   []string
}

// CodeGraphGuidanceMarkdown is the shared instruction block injected into every
// detected supported agent when the CodeGraph community tool is selected.
func CodeGraphGuidanceMarkdown() string {
	return strings.Join([]string{
		"Codegraph comes first",
		"",
		"Codegraph comes first: for every show, what, where, plan, refactor, code fix, analysis or investigation that needs symbol, structure or call-path discovery, run a codegraph query or explore BEFORE any read/fd/find/rg/grep search whenever the workspace has a .codegraph index. For every refactor, code fix, analysis or investigation, codegraph also precedes any rg/grep symbol search. rg/grep are for exact-string and filename lookups, or when codegraph fails or the index could not be initialized.",
		"",
		"Trust by calling, not by checking: never probe for .codegraph/ or daemon state before the first codegraph call — file tools are gitignore-blind there and liveness is unknowable in advance. The codegraph tool answers on its own (daemon, then CLI fallback) and is bounded; only its own unavailability message is the signal to fall back to read/grep/find.",
		"",
		"Search-tool gate: codegraph gates the search tools — the session's first fd/find/rg/grep search must come after a codegraph query or explore, and once codegraph has been used, fd/find/rg/grep may be used freely for any further search (exact-string, filename, or symbol).",
		"",
		"Memory ordering: persistent-memory reads (for example engram mem_context / mem_search) happen strictly after codegraph — codegraph calls of the same step may run in parallel with each other, but every planned codegraph query or explore must have returned before any memory read, and a memory call is never issued in the same parallel block as a codegraph call; then run mem_context and at least one task-scoped mem_search before proceeding further, even when codegraph failed or no index could be initialized.",
		"",
		"Post-compaction revalidation: after compaction or workspace changes, recover target, actor access, and preparation status before using graph evidence; history alone authorizes no init/sync.",
		"",
		"Index hygiene: keep .codegraph/ out of the change — the runtime maintains the local git exclude (including linked worktrees) and the index ships its own .gitignore; never commit index files or hand-edit the exclude.",
		"",
		"Fallback: if codegraph fails or the index could not be initialized, fall back to fd/find/rg/grep directly, and any search that ran without a preceding codegraph call names the skip reason in one line in the final report.",
		"",
		"Staleness: current source prevails over conflicting graph output — inspect the relevant stale files directly, or task-relevant source if the whole index is stale; graph output never replaces tests, compilers, linters, or runtime checks.",
		"",
		"Anti-redundancy: source returned by the graph counts as inspected; do not repeat reads or searches merely to confirm symbols when the required evidence is already present.",
		"",
		"Wrong-project guard: before trusting query/explore -p results for a path whose index you have not verified, run \"codegraph status <path>\" and assert its \"Project:\" line names that path — codegraph silently answers from the current project (exit 0) when the -p target has no .codegraph index, so the failure fallback never fires and wrong-project hits look identical to correct ones.",
		"",
		"Worktrees: codegraph is workspace-scoped; a linked worktree without an index is seeded from the main checkout by the runtime at session start (copy + sync, init as the fallback), so a session never initializes one by hand — it only states in its report when no symbol search was available. Worktree commands run through the CLI with explicit path args: init/index/sync/unlock/status take a positional path and query/explore take -p, so no cd is needed.",
		"",
		"Subagents: a subagent that can call codegraph does so itself, and for a worktree dispatch it does so via the CLI path args because session-bound codegraph tools answer from the subagent cwd and not the dispatched worktree; when it cannot, the parent session runs the query BEFORE dispatching the subagent and embeds the findings in the task handoff, and a subagent result that claims codegraph verification must quote the query it ran or cite the parent-provided findings, otherwise the claim is treated as not performed.",
		"",
		"Truncation fallback (MCP variant): a middle-cut mcp__codegraph__* result — one carrying \"Warning: truncated output\" or \"…N chars truncated…\" — is never worked from: re-run the same query through the native session codegraph tool (daemon path, unaffected by the 20 KB MCP cap) or read the result's [Full output: …] file with offset/limit; use the native tool when the whole report matters, the file when only a slice is missing.",
	}, "\n")
}

// InjectCodeGraphGuidanceIfSelected is the central community-tool hook for
// agent guidance. It is a no-op unless CodeGraph is among the selected tools.
func InjectCodeGraphGuidanceIfSelected(homeDir string, selected []model.CommunityToolID) (GuidanceInjectionResult, error) {
	if !slices.Contains(selected, model.CommunityToolCodeGraph) {
		return GuidanceInjectionResult{}, nil
	}
	return InjectCodeGraphGuidance(homeDir)
}

// RefreshCodeGraphGuidanceIfConfigured refreshes CodeGraph guidance during
// managed sync flows without requiring persisted Community Tools selection.
//
// It is deliberately conservative: guidance is refreshed only when the
// CodeGraph CLI is available and at least one detected supported agent already
// has CodeGraph wiring or a managed guidance marker. This prevents normal sync
// from introducing CodeGraph prompts for users who never installed/enabled it.
func RefreshCodeGraphGuidanceIfConfigured(homeDir string, detector Detector) (GuidanceInjectionResult, bool, error) {
	if !HasConfiguredCodeGraph(homeDir, detector) && !hasAvailableCodeGraphGuidance(homeDir, detector) {
		return GuidanceInjectionResult{}, false, nil
	}

	result, err := InjectCodeGraphGuidance(homeDir)
	return result, true, err
}

func hasAvailableCodeGraphGuidance(homeDir string, detector Detector) bool {
	status := DetectStatus(model.CommunityToolCodeGraph, homeDir, detector)
	return status.CLI == AvailabilityAvailable && HasAnyCodeGraphGuidance(homeDir)
}

func HasConfiguredCodeGraph(homeDir string, detector Detector) bool {
	status := DetectStatus(model.CommunityToolCodeGraph, homeDir, detector)
	if status.CLI != AvailabilityAvailable {
		return false
	}
	for _, agent := range status.Agents {
		if agent.Detected && agent.Configured {
			return true
		}
	}
	return hasDetectedCodeGraphToolWiring(homeDir)
}

func HasLegacyCodeGraphGuidance(homeDir string) bool {
	for _, path := range CodeGraphGuidancePaths(homeDir) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if containsLegacyCodeGraphGuidance(string(data)) {
			return true
		}
	}
	return false
}

func HasAnyCodeGraphGuidance(homeDir string) bool {
	return HasManagedCodeGraphGuidance(homeDir) || HasLegacyCodeGraphGuidance(homeDir)
}

func HasManagedCodeGraphGuidance(homeDir string) bool {
	for _, path := range CodeGraphGuidancePaths(homeDir) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "<!-- gentle-ai:"+codeGraphGuidanceSectionID+" -->") {
			return true
		}
	}
	return false
}

func CleanLegacyCodeGraphGuidance(homeDir string) (GuidanceInjectionResult, error) {
	result := GuidanceInjectionResult{}
	for _, path := range CodeGraphGuidancePaths(homeDir) {
		existing, err := readTextFileOrEmpty(path)
		if err != nil {
			return result, err
		}
		cleaned := stripLegacyCodeGraphGuidance(existing)
		if cleaned == existing {
			continue
		}

		writeResult, err := filemerge.WriteFileAtomic(path, []byte(cleaned), 0o644)
		if err != nil {
			return result, err
		}
		result.Changed = result.Changed || writeResult.Changed
		if writeResult.Changed {
			result.Files = append(result.Files, path)
		}
	}
	return result, nil
}

// InjectCodeGraphGuidance writes the shared CodeGraph lifecycle guidance to the
// supported agents the user selected at install time.
//
// agents.DiscoverSelected intersects the persisted selection with existing
// config directories, keeping the original property — standalone Community
// Tools setup never creates configs for tools the user does not use — while
// also honouring the selection, so guidance never lands in an IDE the user
// declined. Installs predating state persistence have no
// selection to honour, and there guidance still reaches every detected agent.
func InjectCodeGraphGuidance(homeDir string) (GuidanceInjectionResult, error) {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		return GuidanceInjectionResult{}, err
	}

	installed := agents.DiscoverSelected(reg, homeDir)
	result := GuidanceInjectionResult{}
	for _, installedAgent := range installed {
		adapter, ok := reg.Get(installedAgent.ID)
		if !ok || !isCodeGraphCompatibleAgent(installedAgent.ID) || !adapter.SupportsSystemPrompt() || installedAgent.ID == model.AgentPi {
			continue
		}

		file, changed, err := injectCodeGraphGuidanceForAgent(homeDir, adapter)
		if err != nil {
			return result, fmt.Errorf("inject CodeGraph guidance for %s: %w", installedAgent.ID, err)
		}
		if file == "" {
			continue
		}
		result.Changed = result.Changed || changed
		result.Files = append(result.Files, file)
	}

	return result, nil
}

// CodeGraphGuidancePaths returns the system prompt files that the CodeGraph
// guidance injector and the legacy-guidance cleaner may WRITE, scoped to the
// selected supported agents.
func CodeGraphGuidancePaths(homeDir string) []string {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		return nil
	}
	return codeGraphGuidancePathsFor(reg, homeDir, agents.DiscoverSelected(reg, homeDir))
}

// codeGraphGuidancePathsFor resolves guidance paths for a given agent set, so
// the write and backup scopes share one resolution.
func codeGraphGuidancePathsFor(reg *agents.Registry, homeDir string, installed []agents.InstalledAgent) []string {
	paths := make([]string, 0, len(installed))
	for _, installedAgent := range installed {
		adapter, ok := reg.Get(installedAgent.ID)
		if !ok || !isCodeGraphCompatibleAgent(installedAgent.ID) || !adapter.SupportsSystemPrompt() {
			continue
		}
		path := adapter.SystemPromptFile(homeDir)
		if strings.TrimSpace(path) != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

// CodeGraphManagedPaths returns every file CodeGraph setup or managed guidance
// may update, for every DETECTED agent. Sync and install use it purely as the
// backup and changed-file accounting set. It is deliberately broader than the
// write set: what we back up must cover what the third-party installer might
// touch, not only what we intend to write — its blind-install path passes no
// --target and picks its own agents.
func CodeGraphManagedPaths(homeDir string) []string {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		return nil
	}

	detected := agents.DiscoverInstalled(reg, homeDir)
	paths := codeGraphGuidancePathsFor(reg, homeDir, detected)
	for _, installedAgent := range detected {
		adapter, ok := reg.Get(installedAgent.ID)
		if !ok || !isCodeGraphCompatibleAgent(installedAgent.ID) {
			continue
		}
		paths = append(paths, codeGraphToolWiringPaths(homeDir, adapter)...)
	}

	seen := make(map[string]struct{}, len(paths))
	managed := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		managed = append(managed, path)
	}
	slices.Sort(managed)
	return managed
}

func NeedsOpenCodeCodeGraphReconcile(homeDir string) bool {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		return false
	}
	adapter, ok := reg.Get(model.AgentOpenCode)
	if !ok {
		return false
	}
	detected := slices.ContainsFunc(agents.DiscoverSelected(reg, homeDir), func(agent agents.InstalledAgent) bool {
		return agent.ID == model.AgentOpenCode
	})
	if !detected {
		return false
	}
	_, configured := hasCodeGraphToolWiring(homeDir, adapter)
	return !configured
}

// ReconcileOpenCodeCodeGraph delegates JSON/JSONC editing to CodeGraph's own
// installer so comments and unrelated user configuration remain untouched.
func ReconcileOpenCodeCodeGraph(homeDir string, runner Runner) (GuidanceInjectionResult, error) {
	if !NeedsOpenCodeCodeGraphReconcile(homeDir) {
		return GuidanceInjectionResult{}, nil
	}
	if err := opencode.NewAdapter().NativeCodeGraphWiringError(homeDir); err != nil {
		return GuidanceInjectionResult{}, err
	}
	if runner == nil {
		return GuidanceInjectionResult{}, fmt.Errorf("OpenCode CodeGraph reconciliation runner is not configured")
	}

	paths := CodeGraphManagedPaths(homeDir)
	before := readCodeGraphManagedFiles(paths)
	if err := runner.Run("codegraph", "install", "--target", "opencode", "--location", "global", "--yes"); err != nil {
		return GuidanceInjectionResult{}, fmt.Errorf("reconcile OpenCode CodeGraph MCP wiring: %w", err)
	}
	if NeedsOpenCodeCodeGraphReconcile(homeDir) {
		return GuidanceInjectionResult{}, fmt.Errorf("CodeGraph installer did not create effective OpenCode MCP wiring")
	}

	result := GuidanceInjectionResult{}
	for _, path := range paths {
		after, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("read reconciled CodeGraph file %q: %w", path, err)
		}
		if string(after) == before[path] {
			continue
		}
		result.Changed = true
		result.Files = append(result.Files, path)
	}
	return result, nil
}

func readCodeGraphManagedFiles(paths []string) map[string]string {
	files := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			files[path] = string(data)
		}
	}
	return files
}

func injectCodeGraphGuidanceForAgent(homeDir string, adapter agents.Adapter) (string, bool, error) {
	promptPath := adapter.SystemPromptFile(homeDir)
	if strings.TrimSpace(promptPath) == "" {
		return "", false, nil
	}

	existing, err := readTextFileOrEmpty(promptPath)
	if err != nil {
		return "", false, err
	}
	cleaned := stripLegacyCodeGraphGuidance(existing)
	updated := filemerge.InjectMarkdownSection(cleaned, codeGraphGuidanceSectionID, CodeGraphGuidanceMarkdown())

	writeResult, err := filemerge.WriteFileAtomic(promptPath, []byte(updated), 0o644)
	if err != nil {
		return "", false, err
	}
	return promptPath, writeResult.Changed, nil
}

func readTextFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return string(data), nil
	}
	if os.IsNotExist(err) {
		return "", nil
	}
	return "", fmt.Errorf("read %q: %w", path, err)
}

func containsLegacyCodeGraphGuidance(content string) bool {
	return strings.Contains(content, legacyCodeGraphGuidanceStart) ||
		strings.Contains(content, legacyCodeGraphGuidanceEnd) ||
		containsUnmarkedUpstreamCodeGraphGuidance(content)
}

func stripLegacyCodeGraphGuidance(content string) string {
	for {
		startIdx := strings.Index(content, legacyCodeGraphGuidanceStart)
		if startIdx < 0 {
			break
		}

		searchFrom := startIdx + len(legacyCodeGraphGuidanceStart)
		relEndIdx := strings.Index(content[searchFrom:], legacyCodeGraphGuidanceEnd)
		if relEndIdx < 0 {
			content = content[:startIdx] + content[searchFrom:]
			continue
		}

		endIdx := searchFrom + relEndIdx
		before := strings.TrimRight(content[:startIdx], "\r\n")
		after := strings.TrimLeft(content[endIdx+len(legacyCodeGraphGuidanceEnd):], "\r\n")

		switch {
		case before == "":
			content = after
		case after == "":
			content = before
		default:
			content = before + "\n\n" + after
		}
	}

	content = strings.ReplaceAll(content, legacyCodeGraphGuidanceStart, "")
	content = strings.ReplaceAll(content, legacyCodeGraphGuidanceEnd, "")
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}
	return stripUnmarkedUpstreamCodeGraphGuidance(content)
}

func containsUnmarkedUpstreamCodeGraphGuidance(content string) bool {
	return findUnmarkedUpstreamCodeGraphGuidanceStart(content) >= 0
}

func stripUnmarkedUpstreamCodeGraphGuidance(content string) string {
	for {
		startIdx := findUnmarkedUpstreamCodeGraphGuidanceStart(content)
		if startIdx < 0 {
			return collapseBlankLines(content)
		}

		endIdx := findNextMarkdownHeading(content, startIdx+len("## CodeGraph"))
		if endIdx < 0 {
			endIdx = len(content)
		}
		cleanedSection, changed := stripKnownUpstreamCodeGraphLines(content[startIdx:endIdx])
		if !changed {
			return collapseBlankLines(content)
		}
		if cleanedSection != "" {
			after := content[endIdx:]
			if after != "" && !strings.HasSuffix(cleanedSection, "\n") && !strings.HasPrefix(after, "\n") {
				cleanedSection += "\n"
			}
			content = content[:startIdx] + cleanedSection + after
			continue
		}

		before := strings.TrimRight(content[:startIdx], "\r\n")
		after := strings.TrimLeft(content[endIdx:], "\r\n")
		switch {
		case before == "":
			content = after
		case after == "":
			content = before
		default:
			content = before + "\n\n" + after
		}
	}
}

func stripKnownUpstreamCodeGraphLines(section string) (string, bool) {
	lines := strings.Split(section, "\n")
	remove := make([]bool, len(lines))
	changed := false
	for idx, line := range lines {
		if isKnownUpstreamCodeGraphLine(line) {
			remove[idx] = true
			changed = true
		}
	}
	if !changed {
		return section, false
	}
	for idx, line := range lines {
		if strings.TrimSpace(line) != "" {
			continue
		}
		if (idx > 0 && remove[idx-1]) || (idx+1 < len(remove) && remove[idx+1]) {
			remove[idx] = true
		}
	}

	kept := make([]string, 0, len(lines))
	for idx, line := range lines {
		if remove[idx] {
			continue
		}
		kept = append(kept, line)
	}

	manualLines := trimBlankLines(kept[1:])
	if len(manualLines) == 0 {
		return "", true
	}
	return "## CodeGraph\n\n" + strings.Join(manualLines, "\n"), true
}

func trimBlankLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

func isKnownUpstreamCodeGraphLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return isKnownUpstreamCodeGraphIntroLine(trimmed) ||
		strings.HasPrefix(trimmed, "- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call") ||
		strings.HasPrefix(trimmed, "- **Shell** (always works): `codegraph explore ") ||
		isKnownUpstreamCodeGraphSkipLine(trimmed)
}

func isKnownUpstreamCodeGraphIntroLine(line string) bool {
	return strings.HasPrefix(line, "In repositories indexed by CodeGraph") &&
		strings.Contains(line, upstreamCodeGraphOrderPhrase)
}

func isKnownUpstreamCodeGraphSkipLine(line string) bool {
	return strings.HasPrefix(line, upstreamCodeGraphSkipPhrase)
}

func findUnmarkedUpstreamCodeGraphGuidanceStart(content string) int {
	searchFrom := 0
	for {
		relStart := strings.Index(content[searchFrom:], "## CodeGraph")
		if relStart < 0 {
			return -1
		}
		startIdx := searchFrom + relStart
		endIdx := findNextMarkdownHeading(content, startIdx+len("## CodeGraph"))
		if endIdx < 0 {
			endIdx = len(content)
		}
		section := content[startIdx:endIdx]
		if containsKnownUpstreamCodeGraphDuplicate(section) {
			return startIdx
		}
		searchFrom = startIdx + len("## CodeGraph")
	}
}

func containsKnownUpstreamCodeGraphDuplicate(section string) bool {
	hasIntro := false
	hasSkip := false
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		hasIntro = hasIntro || isKnownUpstreamCodeGraphIntroLine(trimmed)
		hasSkip = hasSkip || isKnownUpstreamCodeGraphSkipLine(trimmed)
	}
	return hasIntro && hasSkip
}

func findNextMarkdownHeading(content string, from int) int {
	for from < len(content) {
		relNewline := strings.IndexByte(content[from:], '\n')
		if relNewline < 0 {
			return -1
		}
		lineStart := from + relNewline + 1
		lineEnd := len(content)
		if relNext := strings.IndexByte(content[lineStart:], '\n'); relNext >= 0 {
			lineEnd = lineStart + relNext
		}
		if strings.HasPrefix(strings.TrimSpace(content[lineStart:lineEnd]), "#") {
			return lineStart
		}
		from = lineStart
	}
	return -1
}

func collapseBlankLines(content string) string {
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}
	return content
}

func hasCodeGraphGuidance(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	content := strings.ToLower(string(data))
	return strings.Contains(content, "gentle-ai:"+codeGraphGuidanceSectionID) ||
		(strings.Contains(content, "codegraph") && strings.Contains(content, "gentle-ai codegraph init --cwd <project-root>"))
}

func codeGraphGuidancePath(homeDir string, adapter agents.Adapter) string {
	path := adapter.SystemPromptFile(homeDir)
	if strings.TrimSpace(path) != "" {
		return path
	}
	return filepath.Join(adapter.GlobalConfigDir(homeDir), "AGENTS.md")
}
