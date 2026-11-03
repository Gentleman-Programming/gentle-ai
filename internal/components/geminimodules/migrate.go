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
// GEMINI.md. A GEMINI.md that already carries the import block is the
// current layout and is left alone.
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
	if strings.Contains(content, "<!-- gentle-ai:"+ImportSectionID+" -->") {
		return Result{}, nil
	}

	user, managed := content, ""
	if filemerge.StripLegacyPersonaBlock(content) != content {
		cut := strings.Index(content, "<!-- gentle-ai:")
		if cut < 0 {
			cut = len(content)
		}
		managed, user = content[:cut], content[cut:]
	}
	for _, section := range antigravityrules.Sections(user) {
		if managed != "" && !strings.HasSuffix(managed, "\n\n") {
			managed = strings.TrimRight(managed, "\n") + "\n\n"
		}
		managed += user[section.Start:section.End] + "\n"
	}
	if managed == "" {
		return Result{}, nil
	}
	user = stripSections(user)

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
