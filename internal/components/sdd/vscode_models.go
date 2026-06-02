package sdd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/opencode"
)

const githubCopilotProviderID = "github-copilot"

// vscodeModelAssignmentKeys defines the only VS Code agent keys that may receive
// explicit model frontmatter. Keeping this list closed prevents named-profile
// or arbitrary-file assignments from leaking into native Copilot agents.
func vscodeModelAssignmentKeys() []string {
	return []string{
		"sdd-orchestrator",
		"sdd-init",
		"sdd-explore",
		"sdd-propose",
		"sdd-spec",
		"sdd-design",
		"sdd-tasks",
		"sdd-apply",
		"sdd-verify",
		"sdd-archive",
		"sdd-onboard",
	}
}

// withDefaultVSCodeModelPaths resolves cache paths relative to the injected
// home directory, not the process user. That keeps tests, portable installs,
// and sync-with-selection from accidentally reading another user's cache.
func withDefaultVSCodeModelPaths(opts InjectOptions, homeDir string) InjectOptions {
	if opts.VSCodeModelCachePath == "" {
		opts.VSCodeModelCachePath = filepath.Join(homeDir, ".cache", "opencode", "models.json")
	}
	if opts.VSCodeModelVariantsPath == "" {
		opts.VSCodeModelVariantsPath = filepath.Join(homeDir, ".gentle-ai", "cache", "model-variants.json")
	}
	return opts
}

// renderVSCodeAgentModelAssignment applies an optional model line to a single
// native VS Code agent file. Unresolved assignments are intentionally rendered
// without a model line so Copilot safely inherits the parent session model.
func renderVSCodeAgentModelAssignment(content, fileName string, opts InjectOptions) (string, []string) {
	agentKey, ok := vscodeAgentKey(fileName)
	if !ok {
		return content, nil
	}
	if opts.VSCodeModelAssignments == nil {
		return injectVSCodeModelLine(content, ""), nil
	}

	modelLabel, warnings := resolveVSCodeModelAssignment(
		agentKey,
		opts.VSCodeModelAssignments,
		opts.VSCodeModelCachePath,
		opts.VSCodeModelVariantsPath,
	)
	return injectVSCodeModelLine(content, modelLabel), warnings
}

// vscodeAgentKey maps a managed `.agent.md` filename to its persisted assignment
// key. Non-native files are ignored so other adapter asset formats remain isolated.
func vscodeAgentKey(fileName string) (string, bool) {
	if !strings.HasSuffix(fileName, ".agent.md") {
		return "", false
	}
	key := strings.TrimSuffix(fileName, ".agent.md")
	if key == "" {
		return "", false
	}
	if !isVSCodeModelAssignmentKey(key) {
		return "", false
	}
	return key, true
}

// isVSCodeModelAssignmentKey enforces the closed assignment surface at runtime;
// tests alone are not enough because embedded assets can grow over time.
func isVSCodeModelAssignmentKey(key string) bool {
	for _, allowed := range vscodeModelAssignmentKeys() {
		if allowed == key {
			return true
		}
	}
	return false
}

// resolveVSCodeModelAssignment validates an assignment against the dynamic
// OpenCode model cache and returns the VS Code display label to write. Any
// invalid or stale assignment returns a warning plus an empty label; the caller
// must then omit `model:` instead of failing sync.
func resolveVSCodeModelAssignment(agentKey string, assignments map[string]model.ModelAssignment, cachePath, variantsPath string) (string, []string) {
	assignment, ok := assignments[agentKey]
	if !ok || assignment.ProviderID == "" || assignment.ModelID == "" {
		return "", nil
	}
	if assignment.ProviderID != githubCopilotProviderID {
		return "", []string{fmt.Sprintf("VS Code model assignment for %s skipped: provider %q is not %q", agentKey, assignment.ProviderID, githubCopilotProviderID)}
	}

	providers, warnings := loadVSCodeModelCatalog(agentKey, cachePath, variantsPath)
	if len(warnings) > 0 {
		return "", warnings
	}

	provider, ok := providers[githubCopilotProviderID]
	if !ok {
		return "", []string{fmt.Sprintf("VS Code model assignment for %s skipped: provider %q missing from models cache", agentKey, githubCopilotProviderID)}
	}
	cachedModel, ok := provider.Models[assignment.ModelID]
	if !ok {
		return "", []string{fmt.Sprintf("VS Code model assignment for %s skipped: model %q missing from %q models cache", agentKey, assignment.ModelID, githubCopilotProviderID)}
	}
	if !cachedModel.ToolCall {
		return "", []string{fmt.Sprintf("VS Code model assignment for %s skipped: model %q does not support tool calls", agentKey, assignment.ModelID)}
	}
	if warning := validateVSCodeEffort(agentKey, assignment, cachedModel); warning != "" {
		return "", []string{warning}
	}

	label := strings.TrimSpace(cachedModel.Name)
	if label == "" {
		label = strings.TrimSpace(cachedModel.ID)
	}
	if label == "" {
		return "", []string{fmt.Sprintf("VS Code model assignment for %s skipped: model %q has insufficient metadata", agentKey, assignment.ModelID)}
	}
	return label, nil
}

// loadVSCodeModelCatalog reads the current model cache and enriches it with
// optional effort variants. A missing cache is a recoverable warning because the
// assignment must stay persisted for a future cache refresh.
func loadVSCodeModelCatalog(agentKey, cachePath, variantsPath string) (map[string]opencode.Provider, []string) {
	path := cachePath
	if path == "" {
		path = opencode.DefaultCachePath()
	}
	if path == "" {
		return nil, []string{fmt.Sprintf("VS Code model assignment for %s skipped: models cache path unavailable", agentKey)}
	}

	providers, err := opencode.LoadModels(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, []string{fmt.Sprintf("VS Code model assignment for %s skipped: models cache %q not found", agentKey, path)}
		}
		return nil, []string{fmt.Sprintf("VS Code model assignment for %s skipped: read models cache %q: %v", agentKey, path, err)}
	}

	variantPath := variantsPath
	if variantPath == "" {
		variantPath = opencode.DefaultVariantsCachePath()
	}
	if variantPath != "" {
		opencode.EnrichWithVariants(providers, variantPath)
	}
	return providers, nil
}

// validateVSCodeEffort rejects effort-specific assignments unless the cache can
// prove that the chosen model supports that effort. This avoids writing a model
// frontmatter value that looks precise but cannot represent the requested variant.
func validateVSCodeEffort(agentKey string, assignment model.ModelAssignment, cachedModel opencode.Model) string {
	if assignment.Effort == "" {
		return ""
	}
	if len(cachedModel.Variants) == 0 {
		return fmt.Sprintf("VS Code model assignment for %s skipped: model %q has no effort metadata", agentKey, assignment.ModelID)
	}
	for _, variant := range cachedModel.Variants {
		if variant == assignment.Effort {
			return ""
		}
	}
	return fmt.Sprintf("VS Code model assignment for %s skipped: effort %q is not supported by model %q", agentKey, assignment.Effort, assignment.ModelID)
}

// injectVSCodeModelLine rewrites only the YAML frontmatter model entry. It
// removes stale model lines first, preserves the asset line-ending style, and
// quotes labels so spaces or punctuation remain valid YAML.
func injectVSCodeModelLine(content, modelLabel string) string {
	lineBreak := "\n"
	if strings.Contains(content, "\r\n") {
		lineBreak = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	closing := frontmatterClosingLine(lines)
	if closing == -1 {
		return content
	}

	updated := make([]string, 0, len(lines)+1)
	updated = append(updated, lines[0])
	for i := 1; i < closing; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "model:") {
			continue
		}
		updated = append(updated, lines[i])
	}
	if modelLabel != "" {
		updated = append(updated, "model: "+strconv.Quote(modelLabel))
	}
	updated = append(updated, lines[closing:]...)
	return strings.Join(updated, lineBreak)
}

// frontmatterClosingLine finds the second YAML delimiter that ends frontmatter.
// Files without valid frontmatter are left untouched by the renderer.
func frontmatterClosingLine(lines []string) int {
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return -1
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return i
		}
	}
	return -1
}

// dedupWarnings keeps warning output stable when multiple generated files hit
// the same recoverable cache or metadata problem during one sync.
func dedupWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(warnings))
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if strings.TrimSpace(warning) == "" {
			continue
		}
		if _, ok := seen[warning]; ok {
			continue
		}
		seen[warning] = struct{}{}
		out = append(out, warning)
	}
	return out
}
