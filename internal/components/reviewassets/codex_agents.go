package reviewassets

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// codexAgentsSubdir is the directory Codex scans for agent-role definitions,
// relative to the Codex config root (`~/.codex`). Directory discovery is always
// on, so writing `<role>.toml` here needs no `~/.codex/config.toml` change.
const codexAgentsSubdir = "agents"

// codexRuntimeProfileHeader is prepended to every role's developer
// instructions so the prompt names the Codex runtime primitives instead of
// Claude Code's.
const codexRuntimeProfileHeader = "Codex runtime profile: you run as a named Codex agent role. Delegate with the `spawn_agent` tool, plan with `update_plan`, and run independent tool calls in parallel. Gentle AI skill and agent roots live under `~/.codex/`."

// CodexAgentRoleNames derives the managed Codex role files from the shared
// native agent manifest. Codex is deliberately absent from NativeAgentManifest
// because it has no file-based sub-agents directory (SupportsSubAgents stays
// false); the managed role set is exactly the Claude Code set, rendered as
// `<role>.toml`.
func CodexAgentRoleNames() []string {
	sources := NativeAgentManifest[model.AgentClaudeCode]
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, strings.TrimSuffix(source, ".md")+".toml")
	}
	return names
}

// CodexAgentRoleDir is the owned Codex agent-role directory for a home.
//
// Codex discovers agent roles only under CODEX_HOME, i.e. `~/.codex/agents`.
// A project-local `.codex/agents/` is NOT scanned (a project-local
// `.codex/config.toml` is read, but role files there never load); verified
// against Codex CLI 0.151.0 on this repository. The writer therefore
// intentionally ignores the install scope and stays home-based under both
// `--scope global` and `--scope workspace`, exactly like the sibling
// home-based `~/.codex/hooks.json` backup entry in backupTargets. The `--scope`
// contract documented in docs/agents.md and docs/non-interactive.md is why this
// exception has to be stated rather than inferred.
func CodexAgentRoleDir(home string) string {
	return filepath.Join(home, ".codex", codexAgentsSubdir)
}

// RetiredCodexAgentRoleManifest is the extension point for Codex role files
// earlier releases installed that the managed set no longer includes. A role
// removed from CodexAgentRoleNames() must be listed here: ownership
// reconciliation then removes an untouched, ledger-owned file and drops its
// ledger key instead of the install hard-failing on a ledger entry outside the
// allowed set. A retired role has no live asset; reconciliation matches it by
// recorded ledger hash. Empty today.
var RetiredCodexAgentRoleManifest []string

// codexAgentRoleTOML is the exact Codex role-file schema. Codex parses each
// role file with deny_unknown_fields, so this struct must stay at these four
// keys and every one must be emitted.
type codexAgentRoleTOML struct {
	Name                  string   `toml:"name"`
	Description           string   `toml:"description"`
	DeveloperInstructions string   `toml:"developer_instructions"`
	NicknameCandidates    []string `toml:"nickname_candidates"`
}

// codexAgentFrontmatter is the authored source shape shared with the Claude
// agent assets; only name and description flow into the Codex role TOML.
type codexAgentFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// InstallCodexAgentRoles writes the managed Codex agent-role TOMLs under
// `~/.codex/agents`. It reuses the shared native agent ownership machinery, so
// a pre-existing role file Gentle AI does not own is preserved untouched. The
// target is CODEX_HOME-based and ignores the install scope by design; see
// CodexAgentRoleDir for the Codex CLI 0.151.0 evidence.
func InstallCodexAgentRoles(home string) (InstallResult, error) {
	return installCodexAgentRoles(home, RetiredCodexAgentRoleManifest)
}

// installCodexAgentRoles is the testable core of InstallCodexAgentRoles: the
// retired list is a parameter so reconciliation can be exercised without
// mutating the exported manifest.
func installCodexAgentRoles(home string, retired []string) (InstallResult, error) {
	names := CodexAgentRoleNames()
	rendered := make(map[string]string, len(names))
	for _, name := range names {
		role := strings.TrimSuffix(name, ".toml")
		content, err := RenderCodexAgentRole(role)
		if err != nil {
			return InstallResult{}, err
		}
		rendered[name] = content
	}
	// A retired role has no live embedded asset to render. An empty managed
	// render means reconciliation removes the file only when its bytes match the
	// recorded ledger hash (an untouched, ledger-owned file).
	retiredRendered := make(map[string]string, len(retired))
	for _, name := range retired {
		retiredRendered[name] = ""
	}
	return installNativeAgentFiles(nativeAgentInstallPlan{
		dir:             CodexAgentRoleDir(home),
		names:           names,
		retired:         retired,
		rendered:        rendered,
		retiredRendered: retiredRendered,
	})
}

// RenderCodexAgentRole renders one managed role exactly as
// InstallCodexAgentRoles writes it.
func RenderCodexAgentRole(role string) (string, error) {
	source, err := assets.Read("codex/agents/" + role + ".md")
	if err != nil {
		return "", fmt.Errorf("read codex agent role %s: %w", role, err)
	}
	frontmatter, body, err := splitAgentFrontmatter(source)
	if err != nil {
		return "", fmt.Errorf("codex agent role %s: %w", role, err)
	}
	body = codexAgentRoleBody(role, body)
	instructions := strings.TrimSpace(codexRuntimeProfileHeader) + "\n\n" + strings.TrimSpace(body)
	instructions = filemerge.InjectMarkdownSection(instructions, "agent-language-contract", strings.TrimSpace(assets.MustRead("generic/agent-language-contract.md")))
	instructions = agentguidance.InjectRemoteAuthorization(instructions)
	return renderCodexAgentRoleTOML(frontmatter.Name, frontmatter.Description, instructions, codexNicknameCandidates(frontmatter.Name))
}

// codexAgentRoleBody materializes the shared body the native projection installs
// for a role, so the Codex role TOML never ships authored text no runtime
// installs. A lens reviewer gets the tool-free Claude lens transport through
// the same function the claude path ends with; a jd-judge-* role gets the
// shared Judgment Day contract in place of its authored ledger section. Every
// other role keeps its authored body. This mirrors the order in
// renderNativeAgent (reviewer replacement, then the Judgment Day section).
func codexAgentRoleBody(role, authoredBody string) string {
	if prompt, reviewer := ClaudeReviewerPrompt(role); reviewer {
		return prompt
	}
	if strings.HasPrefix(role, "jd-judge-") {
		return replaceJudgmentSection(authoredBody, JudgmentDayReviewerContract())
	}
	return authoredBody
}

// renderCodexAgentRoleTOML encodes the four-key role record with the repository
// TOML encoder. Round-tripping is asserted by the package tests, including
// multiline instructions that contain backslashes, double quotes, and a triple
// single-quote sequence.
func renderCodexAgentRoleTOML(name, description, instructions string, nicknames []string) (string, error) {
	var builder strings.Builder
	encoder := toml.NewEncoder(&builder)
	if err := encoder.Encode(codexAgentRoleTOML{
		Name:                  name,
		Description:           description,
		DeveloperInstructions: instructions,
		NicknameCandidates:    nicknames,
	}); err != nil {
		return "", fmt.Errorf("encode codex agent role TOML: %w", err)
	}
	return builder.String(), nil
}

// codexNicknameCandidates returns the spaced and literal forms of a role name,
// both of which satisfy Codex's `[A-Za-z0-9 _-]+` nickname rule.
func codexNicknameCandidates(name string) []string {
	spaced := strings.NewReplacer("-", " ", "_", " ").Replace(name)
	spaced = strings.Join(strings.Fields(spaced), " ")
	if spaced == name || spaced == "" {
		return []string{name}
	}
	return []string{spaced, name}
}

// splitAgentFrontmatter parses the YAML frontmatter of an authored agent asset
// and returns it with the remaining body.
func splitAgentFrontmatter(source string) (codexAgentFrontmatter, string, error) {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	if !strings.HasPrefix(source, "---\n") {
		return codexAgentFrontmatter{}, "", fmt.Errorf("missing frontmatter opener")
	}
	rest := source[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return codexAgentFrontmatter{}, "", fmt.Errorf("unterminated frontmatter")
	}
	frontmatter := rest[:end]
	body := strings.TrimPrefix(rest[end+4:], "\n")
	parsed := codexAgentFrontmatter{}
	if err := yaml.Unmarshal([]byte(frontmatter), &parsed); err != nil {
		return codexAgentFrontmatter{}, "", fmt.Errorf("parse frontmatter: %w", err)
	}
	parsed.Name = strings.TrimSpace(parsed.Name)
	parsed.Description = strings.TrimSpace(parsed.Description)
	if parsed.Name == "" {
		return codexAgentFrontmatter{}, "", fmt.Errorf("frontmatter name is empty")
	}
	if parsed.Description == "" {
		return codexAgentFrontmatter{}, "", fmt.Errorf("frontmatter description is empty")
	}
	return parsed, body, nil
}
