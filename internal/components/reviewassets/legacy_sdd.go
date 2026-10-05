package reviewassets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/mutationjournal"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// LegacySDDNativeAgentManifest lists the SDD phase agents Gentle AI v3 wrote
// into a runtime's native agents directory. SDD was retired in v4, which no
// longer renders these agents, so they cannot share RetiredNativeAgentManifest:
// ownership is proven against the known v3 renders in legacySDDAgentHashes
// instead of the current managed render or the ownership ledger.
var LegacySDDNativeAgentManifest = map[model.AgentID][]string{
	model.AgentClaudeCode: {
		"sdd-apply.md", "sdd-archive.md", "sdd-design.md", "sdd-explore.md", "sdd-init.md", "sdd-onboard.md",
		"sdd-propose.md", "sdd-research.md", "sdd-spec.md", "sdd-tasks.md", "sdd-verify.md",
	},
}

// IsLegacySDDNativeAgent reports whether path names a legacy SDD agent the
// installer manages for agent.
func IsLegacySDDNativeAgent(agent model.AgentID, path string) bool {
	return containsString(LegacySDDNativeAgentManifest[agent], filepath.Base(path))
}

// legacySDDManagedSections are the managed Markdown sections v3 appended to
// every rendered SDD agent. Their bodies varied with release and options.
var legacySDDManagedSections = []string{"codegraph-guidance", "agent-language-contract", "remote-authorization"}

const legacySDDCodeGraphToolGrant = ", mcp__codegraph__codegraph_explore"

// normalizeLegacySDDAgent removes every part of a v3 SDD agent render that
// depended on install options or release: the model and effort frontmatter
// lines, the CodeGraph tool grant, and the managed sections. What remains is
// the template body, so a single hash per template proves ownership.
func normalizeLegacySDDAgent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	for _, section := range legacySDDManagedSections {
		content = filemerge.InjectMarkdownSection(content, section, "")
	}
	if strings.HasPrefix(content, "---\n") {
		if end := strings.Index(content[len("---\n"):], "\n---\n"); end >= 0 {
			end += len("---\n")
			lines := strings.Split(content[:end], "\n")
			kept := lines[:0]
			for _, line := range lines {
				if strings.HasPrefix(line, "model:") || strings.HasPrefix(line, "effort:") {
					continue
				}
				if strings.HasPrefix(line, "tools:") {
					line = strings.TrimSuffix(line, legacySDDCodeGraphToolGrant)
				}
				kept = append(kept, line)
			}
			content = strings.Join(kept, "\n") + content[end:]
		}
	}
	return strings.TrimRight(content, " \t\n") + "\n"
}

// legacySDDAgentOwned reports whether data is a v3 render of the legacy SDD
// agent name, modulo the option-dependent parts normalizeLegacySDDAgent drops.
func legacySDDAgentOwned(name string, data []byte) bool {
	_, ok := legacySDDAgentHashes[name][installedHash([]byte(normalizeLegacySDDAgent(string(data))))]
	return ok
}

// removeLegacySDDAgent removes one legacy SDD agent when its bytes match a
// known v3 render. It returns the removed path, or the preserved path when a
// file under that name exists but ownership cannot be proven. A symlink or
// other non-regular file is never ours to delete.
func removeLegacySDDAgent(journal *mutationjournal.Journal, dir, name string) (removed, preserved string, err error) {
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("stat legacy SDD agent %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", path, nil
	}
	// Capture before hashing: Remove then refuses bytes changed after the check.
	if err := journal.Capture(path); err != nil {
		return "", "", fmt.Errorf("capture legacy SDD agent %s: %w", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read legacy SDD agent %s: %w", path, err)
	}
	if !legacySDDAgentOwned(name, data) {
		return "", path, nil
	}
	ok, err := journal.Remove(path)
	if err != nil {
		return "", "", fmt.Errorf("remove legacy SDD agent %s: %w", path, err)
	}
	if !ok {
		return "", "", nil
	}
	return path, "", nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
