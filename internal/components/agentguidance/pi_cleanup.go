package agentguidance

import (
	"fmt"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
)

// legacyPiSystemPromptSectionIDs are the only marker pairs this cleanup owns.
// Pi's package owns APPEND_SYSTEM.md, so unmarked and non-allowlisted content
// must never be interpreted as Gentle AI content.
var legacyPiSystemPromptSectionIDs = []string{
	"persona", "engram-protocol", "sdd-orchestrator", "strict-tdd-mode",
	"agent-routing", "trigger-rules", "codegraph-guidance",
}

// RetirePiSystemPromptBlocks removes only complete legacy managed sections from
// Pi's APPEND_SYSTEM.md. It refuses a direct file symlink but permits symlinked
// parent directories, preserving regular-file mode and every unowned byte.
func RetirePiSystemPromptBlocks(homeDir string, adapter agents.Adapter) (Result, error) {
	promptPath := adapter.SystemPromptFile(homeDir)
	info, err := os.Lstat(promptPath)
	if os.IsNotExist(err) {
		return Result{}, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("stat Pi system prompt %q: %w", promptPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Result{}, fmt.Errorf("Pi system prompt %q is not a regular file", promptPath)
	}

	existing, err := os.ReadFile(promptPath)
	if err != nil {
		return Result{}, fmt.Errorf("read Pi system prompt %q: %w", promptPath, err)
	}
	updated, removed := removePiManagedSections(string(existing))
	if !removed {
		return Result{}, nil
	}
	if strings.TrimSpace(updated) == "" {
		if err := os.Remove(promptPath); err != nil {
			return Result{}, fmt.Errorf("remove empty Pi system prompt %q: %w", promptPath, err)
		}
		return Result{Changed: true, Files: []string{promptPath}}, nil
	}
	writeResult, err := filemerge.WriteFileAtomic(promptPath, []byte(updated), info.Mode().Perm())
	if err != nil {
		return Result{}, err
	}
	return Result{Changed: writeResult.Changed, Files: []string{promptPath}}, nil
}

func removePiManagedSections(content string) (string, bool) {
	removed := false
	for _, sectionID := range legacyPiSystemPromptSectionIDs {
		open := "<!-- gentle-ai:" + sectionID + " -->"
		close := "<!-- /gentle-ai:" + sectionID + " -->"
		for search := 0; ; {
			start := strings.Index(content[search:], open)
			if start < 0 {
				break
			}
			start += search
			end := strings.Index(content[start+len(open):], close)
			if end < 0 {
				search = start + len(open)
				continue
			}
			end += start + len(open) + len(close)
			content = content[:start] + content[end:]
			removed = true
			search = start
		}
	}
	return content, removed
}
