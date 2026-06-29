package communitytool

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
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
		"## CodeGraph",
		"",
		"When answering structural or codebase questions, use CodeGraph before broad filesystem searches. This is a hard ordering rule for repo maps, architecture, call flow, dependencies, symbol references, impact analysis, and “how does X work” questions.",
		"",
		"Required order for structural/codebase questions:",
		"",
		"1. Resolve the project root with `git rev-parse --show-toplevel || pwd`.",
		"2. Confirm the root is a real project/workspace. Do not initialize CodeGraph in `$HOME`, temporary directories, or non-project folders.",
		"3. Check for `<project-root>/.codegraph/` before any broad Read/Glob/Grep filesystem exploration.",
		"4. If `.codegraph/` is missing and CodeGraph is enabled/available, immediately run `codegraph init <project-root>` once, then use the `codegraph_explore` MCP tool or `codegraph explore \"...\"`.",
		"5. Do not fall back just because `.codegraph/` is missing; a missing index is the trigger to lazy-initialize, not a reason to skip CodeGraph.",
		"6. Only fall back to normal filesystem tools after CodeGraph init or CodeGraph use fails, and briefly explain the fallback.",
		"",
		"Broad Read/Glob/Grep exploration before this CodeGraph check is explicitly discouraged for structural/codebase questions.",
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
	if !HasConfiguredCodeGraph(homeDir, detector) {
		return GuidanceInjectionResult{}, false, nil
	}

	result, err := InjectCodeGraphGuidance(homeDir)
	return result, true, err
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
	return false
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

// InjectCodeGraphGuidance writes the shared CodeGraph lifecycle guidance to all
// detected supported agents. Detection is intentionally based on existing agent
// config directories so standalone Community Tools setup does not create agent
// configs for tools the user does not use.
func InjectCodeGraphGuidance(homeDir string) (GuidanceInjectionResult, error) {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		return GuidanceInjectionResult{}, err
	}

	installed := agents.DiscoverInstalled(reg, homeDir)
	result := GuidanceInjectionResult{}
	for _, installedAgent := range installed {
		adapter, ok := reg.Get(installedAgent.ID)
		if !ok || !isCodeGraphSupportedAgent(installedAgent.ID) || !adapter.SupportsSystemPrompt() {
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
// guidance injector may touch for currently detected supported agents.
func CodeGraphGuidancePaths(homeDir string) []string {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		return nil
	}

	installed := agents.DiscoverInstalled(reg, homeDir)
	paths := make([]string, 0, len(installed))
	for _, installedAgent := range installed {
		adapter, ok := reg.Get(installedAgent.ID)
		if !ok || !isCodeGraphSupportedAgent(installedAgent.ID) || !adapter.SupportsSystemPrompt() {
			continue
		}
		path := adapter.SystemPromptFile(homeDir)
		if strings.TrimSpace(path) != "" {
			paths = append(paths, path)
		}
	}
	return paths
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
		(strings.Contains(content, "codegraph") && strings.Contains(content, "codegraph init <project-root>"))
}

func codeGraphGuidancePath(homeDir string, adapter agents.Adapter) string {
	path := adapter.SystemPromptFile(homeDir)
	if strings.TrimSpace(path) != "" {
		return path
	}
	return filepath.Join(adapter.GlobalConfigDir(homeDir), "AGENTS.md")
}
