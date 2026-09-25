package reviewassets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/mutationjournal"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// NativeAgentManifest is an explicit allowlist: never enumerate the embedded agents directory.
var NativeAgentManifest = map[model.AgentID][]string{
	model.AgentClaudeCode: {"jd-fix-agent.md", "jd-judge-a.md", "jd-judge-b.md", "review-readability.md", "review-refuter.md", "review-reliability.md", "review-resilience.md", "review-risk.md"},
	model.AgentCursor:     {"review-readability.md", "review-refuter.md", "review-reliability.md", "review-resilience.md", "review-risk.md"},
	model.AgentKiroIDE:    {"jd-fix-agent.md", "jd-judge-a.md", "jd-judge-b.md", "review-readability.md", "review-refuter.md", "review-reliability.md", "review-resilience.md", "review-risk.md"},
	model.AgentKimi:       {"gentleman.yaml", "review-readability.md", "review-readability.yaml", "review-refuter.md", "review-refuter.yaml", "review-reliability.md", "review-reliability.yaml", "review-resilience.md", "review-resilience.yaml", "review-risk.md", "review-risk.yaml"},
}

type InstallOptions struct {
	ClaudeModelAssignments    map[string]model.ClaudeModelAlias
	ClaudePhaseAssignments    map[string]model.ClaudePhaseAssignment
	KiroModelAssignments      map[string]model.KiroModelAlias
	CodeGraphGuidanceMarkdown string
}
type InstallResult struct {
	Changed bool
	Files   []string
	Skipped []string
}

var engramToolPlaceholder = regexp.MustCompile(`\{\{ENGRAM_TOOL_PREFIX\}\}([A-Za-z0-9_]+)`)

type kiroModelResolver interface {
	KiroModelID(model.KiroModelAlias) string
}
type claudeModelResolver interface {
	ClaudeModelID(model.ClaudeModelAlias) string
}

// InstallNativeAgents installs only retained review, Judgment Day, and Kimi native agents.
// It never removes existing files, including legacy SDD files or user-owned agents.
func InstallNativeAgents(home string, adapter agents.Adapter, opts InstallOptions) (InstallResult, error) {
	names, supported := NativeAgentManifest[adapter.Agent()]
	if !supported {
		return InstallResult{}, fmt.Errorf("unsupported native agent runtime: %s", adapter.Agent())
	}
	dir := adapter.SubAgentsDir(home)
	if dir == "" {
		return InstallResult{}, fmt.Errorf("empty native agents directory")
	}
	// Render all inputs before touching the target so missing embedded assets do not partially install.
	rendered := make(map[string]string, len(names))
	for _, name := range names {
		path := adapter.EmbeddedSubAgentsDir() + "/" + name
		source, err := assets.Read(path)
		if err != nil {
			return InstallResult{}, fmt.Errorf("read native agent %s: %w", path, err)
		}
		content := source
		if prompt, reviewer := RenderReviewerAsset(path, content); reviewer {
			content = prompt
		}
		if strings.HasPrefix(name, "jd-judge-") {
			content = replaceJudgmentSection(content, JudgmentDayReviewerContract())
		}
		phase := strings.TrimSuffix(name, ".md")
		if kmr, ok := adapter.(kiroModelResolver); ok {
			alias := model.KiroModelAuto
			if selected, found := opts.KiroModelAssignments[phase]; found {
				alias = selected
			} else if selected, found := opts.KiroModelAssignments["default"]; found {
				alias = selected
			} else if opts.KiroModelAssignments == nil {
				if selected, found := opts.ClaudeModelAssignments[phase]; found {
					alias = model.KiroModelAlias(selected)
				} else if selected, found := opts.ClaudeModelAssignments["default"]; found {
					alias = model.KiroModelAlias(selected)
				}
			}
			content = strings.ReplaceAll(content, "{{KIRO_MODEL}}", kmr.KiroModelID(alias))
		}
		if cmr, ok := adapter.(claudeModelResolver); ok {
			assignment := resolveClaudeAssignment(opts.ClaudeModelAssignments, opts.ClaudePhaseAssignments, phase)
			content = strings.ReplaceAll(content, "{{CLAUDE_MODEL}}", cmr.ClaudeModelID(assignment.Model))
			effort := ""
			if assignment.Effort != model.ClaudeEffortDefault && model.ClaudeEffortAllowedForModel(assignment.Model, assignment.Effort) {
				effort = "effort: " + string(assignment.Effort)
			}
			if effort == "" {
				content = strings.ReplaceAll(content, "{{CLAUDE_EFFORT_FRONTMATTER}}\r\n", "")
				content = strings.ReplaceAll(content, "{{CLAUDE_EFFORT_FRONTMATTER}}\n", "")
			}
			content = strings.ReplaceAll(content, "{{CLAUDE_EFFORT_FRONTMATTER}}", effort)
		}
		content = engramToolPlaceholder.ReplaceAllString(content, "mcp__engram__$1, mcp__plugin_engram_engram__$1")
		if filepath.Ext(name) == ".md" {
			content = InjectCodeGraphToolGrant(content, adapter.Agent(), opts.CodeGraphGuidanceMarkdown)
			if strings.TrimSpace(opts.CodeGraphGuidanceMarkdown) != "" {
				content = filemerge.InjectMarkdownSection(content, "codegraph-guidance", opts.CodeGraphGuidanceMarkdown)
			}
			content = filemerge.InjectMarkdownSection(content, "agent-language-contract", strings.TrimSpace(assets.MustRead("generic/agent-language-contract.md")))
			content = agentguidance.InjectRemoteAuthorization(content)
		}
		rendered[name] = content
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create native agents directory: %w", err)
	}
	ledgerFile := ledgerPath(dir)
	journal := mutationjournal.New(dir)
	// The ledger is captured first; no agent mutation may precede its snapshot.
	if err := journal.Capture(ledgerFile); err != nil {
		return InstallResult{}, fmt.Errorf("capture ownership ledger: %w", err)
	}
	ledger, _, err := readOwnership(ledgerFile, names)
	if err != nil {
		return InstallResult{}, err
	}
	type candidate struct {
		name, path    string
		data          []byte
		exists, owned bool
	}
	candidates := make([]candidate, 0, len(names))
	result := InstallResult{}
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := journal.Validate(path); err != nil {
			return result, err
		}
		if err := journal.Capture(path); err != nil {
			return result, fmt.Errorf("capture native agent %s: %w", name, err)
		}
		data, exists, err := nativeFile(path)
		if err != nil {
			return result, err
		}
		expected, known := ledger.Files[name]
		owned := exists && known && installedHash(data) == expected
		if exists && !owned {
			result.Skipped = append(result.Skipped, path)
		}
		candidates = append(candidates, candidate{name: name, path: path, data: data, exists: exists, owned: owned})
	}
	rollback := func(err error) (InstallResult, error) {
		return InstallResult{Skipped: result.Skipped}, errors.Join(err, journal.Restore())
	}
	// A legacy installation with every candidate skipped has no owned bytes
	// to record. Do not create a ledger merely because it was absent.
	ledgerChanged := false
	for _, c := range candidates {
		if c.exists && !c.owned {
			continue
		}
		desired := []byte(rendered[c.name])
		if !c.exists || string(c.data) != string(desired) {
			if _, err := journal.WriteWithMode(c.path, desired, 0o644); err != nil {
				return rollback(fmt.Errorf("write native agent %s: %w", c.name, err))
			}
			result.Changed = true
			result.Files = append(result.Files, c.path)
		}
		// Hash the bytes at the installed destination rather than the template.
		actual, err := os.ReadFile(c.path)
		if err != nil {
			return rollback(fmt.Errorf("read installed agent %s: %w", c.name, err))
		}
		if string(actual) != string(desired) {
			return rollback(fmt.Errorf("native agent changed after write: %s", c.path))
		}
		hash := installedHash(actual)
		if ledger.Files[c.name] != hash {
			ledger.Files[c.name] = hash
			ledgerChanged = true
		}
	}
	if ledgerChanged {
		encoded, err := json.MarshalIndent(ledger, "", "  ")
		if err != nil {
			return rollback(fmt.Errorf("encode ownership ledger: %w", err))
		}
		encoded = append(encoded, '\n')
		if _, err := journal.WriteWithMode(ledgerFile, encoded, 0o644); err != nil {
			return rollback(fmt.Errorf("write ownership ledger: %w", err))
		}
		result.Changed = true
		result.Files = append(result.Files, ledgerFile)
	}
	return result, nil
}

func resolveClaudeAssignment(legacy map[string]model.ClaudeModelAlias, phases map[string]model.ClaudePhaseAssignment, phase string) model.ClaudePhaseAssignment {
	merged := model.ClaudePhaseAssignmentsFromLegacy(model.ClaudeModelPresetBalanced())
	for key, value := range model.ClaudePhaseAssignmentsFromLegacy(legacy) {
		merged[key] = value
	}
	for key, value := range phases {
		if value.Valid() {
			merged[key] = value
		}
	}
	if value, ok := merged[phase]; ok && value.Valid() {
		return value
	}
	if value, ok := merged["default"]; ok && value.Valid() {
		return value
	}
	return model.ClaudePhaseAssignment{Model: model.ClaudeModelSonnet}
}

func replaceJudgmentSection(content, body string) string {
	const heading = "## Review ledger contract"
	start := strings.Index(content, heading)
	if start < 0 {
		return content
	}
	return strings.TrimRight(content[:start], "\n") + "\n\n" + heading + "\n\n" + body + "\n\n"
}
