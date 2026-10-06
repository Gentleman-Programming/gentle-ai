package legacyassets

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// SDDRuntimeFiles locates what releases wrote for one runtime outside its
// skill, command, and workflow directories.
type SDDRuntimeFiles struct {
	// Dirs holds the Codex home (SDD profiles) or the legacy Kimi root (SDD
	// module); install, sync, and snapshots list them with PresentRetiredSDDAssetPaths.
	Dirs SDDAssetDirs
	// Prompt is the lowercase agents.md v1.7.10 to v1.31.0 wrote Codex's SDD
	// block to; Active is the AGENTS.md routing guidance migrates.
	Prompt, Active string
	// Hub is the legacy Kimi router that included the SDD module.
	Hub string
}

// RetiredSDDRuntimeFiles returns the SDDRuntimeFiles of agent under root, the
// home or workspace directory releases resolved them from.
func RetiredSDDRuntimeFiles(agent model.AgentID, root string) SDDRuntimeFiles {
	switch agent {
	case model.AgentCodex:
		home := filepath.Join(root, ".codex")
		return SDDRuntimeFiles{Dirs: SDDAssetDirs{CodexHome: home}, Prompt: filepath.Join(home, "agents.md"), Active: filepath.Join(home, "AGENTS.md")}
	case model.AgentKimi:
		home := filepath.Join(root, ".kimi")
		return SDDRuntimeFiles{Dirs: SDDAssetDirs{KimiHome: home}, Hub: filepath.Join(home, "KIMI.md")}
	}
	return SDDRuntimeFiles{}
}

// codexProfileLine is a line the retired Codex profile writer rendered: one
// of the two keys v1.36.0 to v3.7.0 upserted, with a Go-quoted value.
var codexProfileLine = regexp.MustCompile(`^(model|model_reasoning_effort) = "(?:[^"\\]|\\.)*"$`)

// NormalizeCodexProfile maps every render of a retired sdd-*.config.toml
// profile to the same bytes. It erases exactly what the writer substituted:
// the quoted value of each line that is `model = "<value>"` or
// `model_reasoning_effort = "<value>"`, the user's model choice. Every other
// byte, including blank lines and line endings, must equal a release's
// render.
func NormalizeCodexProfile(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if match := codexProfileLine.FindStringSubmatch(line); match != nil {
			lines[i] = match[1] + " = {{VALUE}}"
		}
	}
	return strings.Join(lines, "\n")
}

const (
	// kimiSDDIncludeLine is the line every release's KIMI.md router loaded
	// the retired SDD orchestrator module with.
	kimiSDDIncludeLine = `{% include "sdd-orchestrator.md" ignore missing %}`
	// sddOrchestratorSection is the managed section the SDD component
	// wrote into system prompt files.
	sddOrchestratorSection = "sdd-orchestrator"
)

// TextRetireResult reports whether retired SDD text was removed from a
// prompt file, or left because the file is not a regular file (for example a
// symlink) and is never rewritten.
type TextRetireResult struct {
	Path                  string
	Removed, Unrewritable bool
	// text names what was retired, for the manual action.
	text string
}

// RetireKimiSDDInclude removes from a legacy Kimi KIMI.md router every line
// that is exactly the include releases wrote for the retired SDD module.
// Every other byte, including any other include of that module, is kept.
func RetireKimiSDDInclude(hub string) (TextRetireResult, error) {
	return retirePromptText(hub, "the line `"+kimiSDDIncludeLine+"`", func(content string) string {
		lines := strings.SplitAfter(content, "\n")
		kept := lines[:0]
		for _, line := range lines {
			if strings.TrimSuffix(line, "\n") != kimiSDDIncludeLine {
				kept = append(kept, line)
			}
		}
		return strings.Join(kept, "")
	})
}

// RetireSDDOrchestratorBlock removes every complete `<!-- gentle-ai:
// sdd-orchestrator -->` managed section from a system prompt file routing
// guidance does not migrate. A file that is active (the same file, as on a
// case-insensitive filesystem) is left to the routing guidance, which
// converts the block in place. An unterminated marker has no ownership
// boundary and is left alone.
func RetireSDDOrchestratorBlock(path, active string) (TextRetireResult, error) {
	if active != "" {
		if a, err := os.Stat(path); err == nil {
			if b, err := os.Stat(active); err == nil && os.SameFile(a, b) {
				return TextRetireResult{Path: path}, nil
			}
		}
	}
	open := "<!-- gentle-ai:" + sddOrchestratorSection + " -->"
	end := "<!-- /gentle-ai:" + sddOrchestratorSection + " -->"
	return retirePromptText(path, "the block from `"+open+"` to `"+end+"`", func(content string) string {
		start := strings.Index(content, open)
		if start < 0 || !strings.Contains(content[start:], end) {
			return content
		}
		return filemerge.InjectMarkdownSection(content, sddOrchestratorSection, "")
	})
}

// retirePromptText rewrites path with edit when that changes it. A path that
// is not a regular file is never rewritten; it is reported when edit would
// have changed it.
func retirePromptText(path, text string, edit func(string) string) (TextRetireResult, error) {
	result := TextRetireResult{Path: path, text: text}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("inspect %s: %w", path, err)
	}
	if info.IsDir() {
		return result, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if !info.Mode().IsRegular() {
			return result, nil // a dangling link holds nothing to retire
		}
		return result, fmt.Errorf("read %s: %w", path, err)
	}
	updated := edit(string(raw))
	if updated == string(raw) {
		return result, nil
	}
	if !info.Mode().IsRegular() {
		result.Unrewritable = true
		return result, nil
	}
	if _, err := filemerge.WriteFileAtomic(path, []byte(updated), info.Mode().Perm()); err != nil {
		return result, err
	}
	result.Removed = true
	return result, nil
}

// ManualActions tells the user what to do with retired text that was kept.
func (r TextRetireResult) ManualActions() []string {
	if !r.Unrewritable {
		return nil
	}
	return []string{fmt.Sprintf("%s still holds %s, retired SDD instructions Gentle AI installed before v4.0.0. Gentle AI did not edit the file because it is not a regular file (for example a symlink); remove that text yourself.", r.Path, r.text)}
}
