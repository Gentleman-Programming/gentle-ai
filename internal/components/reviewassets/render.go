package reviewassets

import "strings"

// RenderReviewerAsset replaces a lens agent's body while preserving its frontmatter.
// Non-review agents remain the caller's responsibility (including Judgment Day).
func RenderReviewerAsset(path, content string) (string, bool) {
	name := path
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	name = strings.TrimSuffix(name, ".md")
	prompt, reviewer := ReviewerPrompt(name)
	if !reviewer {
		return content, false
	}
	if strings.HasPrefix(path, "claude/agents/") {
		prompt, _ = ClaudeReviewerPrompt(name)
	}
	end := strings.Index(content, "\n---\n")
	if end < 0 {
		return prompt, true
	}
	return strings.TrimRight(content[:end+5], "\n") + "\n\n" + prompt + "\n", true
}
