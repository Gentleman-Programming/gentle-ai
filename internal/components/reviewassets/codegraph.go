package reviewassets

import (
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"strings"
)

// InjectCodeGraphToolGrant preserves native Markdown frontmatter and tool-free reviewers.
func InjectCodeGraphToolGrant(prompt string, agentID model.AgentID, guidance string) string {
	if strings.TrimSpace(guidance) == "" {
		return prompt
	}
	grant := ""
	switch agentID {
	case model.AgentClaudeCode:
		grant = "mcp__codegraph__codegraph_explore"
	case model.AgentKiroIDE:
		grant = "@codegraph"
	default:
		return prompt
	}
	end := strings.Index(prompt, "\n---\n")
	if end < 0 {
		return prompt
	}
	lines := strings.Split(prompt[:end], "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "tools:") {
			continue
		}
		if strings.Contains(line, grant) {
			return prompt
		}
		if value := strings.TrimSpace(strings.TrimPrefix(line, "tools:")); value == "" || value == "[]" {
			return prompt
		}
		if agentID == model.AgentClaudeCode {
			lines[i] = line + ", " + grant
		} else if strings.HasSuffix(line, "]") {
			lines[i] = strings.TrimSuffix(line, "]") + `, "` + grant + `"]`
		} else {
			return prompt
		}
		return strings.Join(lines, "\n") + prompt[end:]
	}
	return prompt
}
