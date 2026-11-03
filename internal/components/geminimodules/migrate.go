package geminimodules

import (
	"fmt"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/antigravityrules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
)

// MigrateLegacyRoot moves the Gentle AI content older releases wrote into
// GEMINI.md (the persona body before the first marker and every marker
// section) into the staging file, so the writers that run next update it as
// they updated GEMINI.md. The persona body is recognized with the persona
// fingerprints StripLegacyPersonaBlock uses; any other text stays in
// GEMINI.md. The import block stays, and content an older release put beside
// it after a downgrade replaces its staged copy.
func MigrateLegacyRoot(baseDir string) (Result, error) {
	path := RootPath(baseDir)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Result{}, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("read %q: %w", path, err)
	}
	content := string(raw)

	user, preamble := content, ""
	if filemerge.StripLegacyPersonaBlock(content) != content {
		cut := strings.Index(content, "<!-- gentle-ai:")
		if cut < 0 {
			cut = len(content)
		}
		preamble, user = content[:cut], content[cut:]
	}
	var moved []antigravityrules.Section
	for _, section := range antigravityrules.Sections(user) {
		if section.ID != ImportSectionID {
			moved = append(moved, section)
		}
	}
	if preamble == "" && len(moved) == 0 {
		return Result{}, nil
	}

	staged, err := os.ReadFile(StagingPath(baseDir))
	if err != nil && !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("read Gemini CLI guidance staging file: %w", err)
	}
	stagedSections := antigravityrules.Sections(string(staged))
	if preamble == "" {
		preamble = string(staged)
		if len(stagedSections) > 0 {
			preamble = preamble[:stagedSections[0].Start]
		}
	}
	managed := ""
	if strings.TrimSpace(preamble) != "" {
		managed = strings.TrimRight(preamble, "\n") + "\n"
	}
	for _, section := range append(stagedSections, moved...) {
		if section.Body != "" {
			managed = filemerge.InjectMarkdownSection(managed, section.ID, section.Body)
		}
	}
	for _, section := range moved {
		user = filemerge.InjectMarkdownSection(user, section.ID, "")
	}

	result := Result{}
	if _, err := filemerge.WriteFileAtomic(StagingPath(baseDir), []byte(managed), 0o644); err != nil {
		return result, fmt.Errorf("stage Gemini CLI guidance from %q: %w", path, err)
	}
	result.add(StagingPath(baseDir))
	if err := writeOrRemove(path, user); err != nil {
		return result, err
	}
	result.add(path)
	return result, nil
}
