package legacyassets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

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
// home or workspace directory releases resolved them from: a workspace-scoped
// install of v1.31.0 to v3.7.0 wrote the same files under the workspace
// (<ws>/.codex, <ws>/.kimi). The prompt edits are confined to the Codex home
// and the Kimi root, like the profiles and the module.
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
	// sddOrchestratorOpen and sddOrchestratorClose delimit the managed
	// section the SDD component wrote into system prompt files.
	sddOrchestratorOpen  = "<!-- gentle-ai:sdd-orchestrator -->"
	sddOrchestratorClose = "<!-- /gentle-ai:sdd-orchestrator -->"
)

// TextRetireResult reports whether retired SDD text was removed from a
// prompt file, or why a file holding it was kept: the file is not a regular
// file (for example a symlink), it sits behind a directory that is not a
// real directory, or its markers are ambiguous.
type TextRetireResult struct {
	Path                  string
	Removed, Unrewritable bool
	// UnsupportedDir is the directory, from the root down, that is not a
	// real directory.
	UnsupportedDir string
	// Ambiguous says why the markers have no trustworthy boundary.
	Ambiguous string
	// text names what was retired, for the manual action.
	text string
}

// RetireKimiSDDInclude removes from a legacy Kimi KIMI.md router under root
// every line that is exactly the include releases wrote for the retired SDD
// module. Every other byte, including any other include of that module, is
// kept.
func RetireKimiSDDInclude(root, hub string) (TextRetireResult, error) {
	return retirePromptText(root, hub, "the line `"+kimiSDDIncludeLine+"`", func(content string) (string, string) {
		lines := strings.SplitAfter(content, "\n")
		kept := lines[:0]
		for _, line := range lines {
			if strings.TrimSuffix(line, "\n") != kimiSDDIncludeLine {
				kept = append(kept, line)
			}
		}
		return strings.Join(kept, ""), ""
	})
}

// RetireSDDOrchestratorBlock removes the `<!-- gentle-ai:sdd-orchestrator -->`
// managed section from a system prompt file under root that routing guidance
// does not migrate. A file that is active (the same file, as on a
// case-insensitive filesystem) is left to the routing guidance, which
// converts the block in place. Only one exactly delimited block is removed;
// any other use of the markers is ambiguous, so the file is kept and
// reported.
func RetireSDDOrchestratorBlock(root, path, active string) (TextRetireResult, error) {
	if active != "" {
		if a, err := os.Stat(path); err == nil {
			if b, err := os.Stat(active); err == nil && os.SameFile(a, b) {
				return TextRetireResult{Path: path}, nil
			}
		}
	}
	return retirePromptText(root, path, "the block from `"+sddOrchestratorOpen+"` to `"+sddOrchestratorClose+"`", removeSDDOrchestratorBlock)
}

// removeSDDOrchestratorBlock removes the one block whose open and close
// markers each fill a line, with the close line's ending and one blank line
// before the block, the separator the section writer added. It returns why
// the markers are ambiguous instead when they appear in any other shape.
func removeSDDOrchestratorBlock(content string) (string, string) {
	opens, closes := strings.Count(content, sddOrchestratorOpen), strings.Count(content, sddOrchestratorClose)
	switch {
	case opens == 0 && closes == 0:
		return content, ""
	case opens != 1 || closes != 1:
		return content, fmt.Sprintf("it has %d opening and %d closing markers instead of one block", opens, closes)
	}
	start, end := strings.Index(content, sddOrchestratorOpen), strings.Index(content, sddOrchestratorClose)
	if end < start {
		return content, "its closing marker comes before its opening marker"
	}
	after := content[end+len(sddOrchestratorClose):]
	ending := ""
	for _, candidate := range []string{"\r\n", "\n"} {
		if strings.HasPrefix(after, candidate) {
			ending = candidate
			break
		}
	}
	opensLine := start == 0 || content[start-1] == '\n'
	closesLine := content[end-1] == '\n' && (ending != "" || after == "")
	body := content[start+len(sddOrchestratorOpen):]
	if !opensLine || !closesLine || !strings.HasPrefix(body, "\n") && !strings.HasPrefix(body, "\r\n") {
		return content, "its markers are not one block whose markers each fill a line"
	}
	before := content[:start]
	for _, separator := range []string{"\r\n\r\n", "\n\n"} {
		if strings.HasSuffix(before, separator) {
			before = before[:len(before)-len(separator)/2]
			break
		}
	}
	return before + after[len(ending):], ""
}

// RetirablePromptFile reports whether retirement may rewrite path: a regular
// file whose directories, from root down, are real directories. Snapshots
// declare exactly these prompt files.
func RetirablePromptFile(root, path string) bool {
	if unsupported, err := unsupportedAncestor(root, path); err != nil || unsupported != "" {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// retirePromptText rewrites path with edit when that changes it. Only a
// regular file under real directories is ever read for rewriting. A symlink
// to a regular file, or a regular file behind a directory that is not a real
// directory, is read through the link only to report retired text it holds;
// anything else (a FIFO, a device) is never opened.
func retirePromptText(root, path, text string, edit func(string) (string, string)) (TextRetireResult, error) {
	result := TextRetireResult{Path: path, text: text}
	unsupported, err := unsupportedAncestor(root, path)
	if err != nil {
		return result, err
	}
	info, err := os.Lstat(path)
	if unsupported != "" {
		info, err = os.Stat(path)
	}
	// A runtime home that is a regular file (ENOTDIR) holds no prompt either.
	if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("inspect %s: %w", path, err)
	}
	linked := info.Mode()&os.ModeSymlink != 0
	if linked {
		if info, err = os.Stat(path); err != nil {
			return result, nil // a dangling link holds nothing to retire
		}
	}
	if !info.Mode().IsRegular() {
		return result, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("read %s: %w", path, err)
	}
	updated, ambiguous := edit(string(raw))
	if updated == string(raw) && ambiguous == "" {
		return result, nil
	}
	switch {
	case unsupported != "":
		result.UnsupportedDir = unsupported
	case linked:
		result.Unrewritable = true
	case ambiguous != "":
		result.Ambiguous = ambiguous
	default:
		if _, err := filemerge.WriteFileAtomic(path, []byte(updated), info.Mode().Perm()); err != nil {
			return result, err
		}
		result.Removed = true
	}
	return result, nil
}

// ManualActions tells the user what to do with retired text that was kept.
func (r TextRetireResult) ManualActions() []string {
	const retired = "retired SDD instructions Gentle AI installed before v4.0.0"
	switch {
	case r.UnsupportedDir != "":
		return []string{fmt.Sprintf("%s still holds %s, %s. Gentle AI did not edit it because %s is not a real directory (for example a symlink); remove that text yourself.", r.Path, r.text, retired, r.UnsupportedDir)}
	case r.Unrewritable:
		return []string{fmt.Sprintf("%s still holds %s, %s. Gentle AI did not edit the file because it is not a regular file (for example a symlink); remove that text yourself.", r.Path, r.text, retired)}
	case r.Ambiguous != "":
		return []string{fmt.Sprintf("%s holds %s, %s, but Gentle AI did not edit it because %s, so it cannot tell which text is yours. Remove the retired block yourself; if you no longer need the file, move or delete it.", r.Path, r.text, retired, r.Ambiguous)}
	}
	return nil
}
