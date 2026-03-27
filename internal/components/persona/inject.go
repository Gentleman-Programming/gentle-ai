package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

// outputStyleOverlayJSON is the settings.json overlay to enable the Gentleman output style.
var outputStyleOverlayJSON = []byte("{\n  \"outputStyle\": \"Gentleman\"\n}\n")

// openCodeAgentOverlayJSON defines Tab-switchable agents for OpenCode.
// "gentleman" is the primary agent, "sdd-orchestrator" is available via Tab.
// Both reference AGENTS.md via {file:./AGENTS.md} for their system prompt.
var openCodeAgentOverlayJSON = []byte("{\n  \"agent\": {\n    \"gentleman\": {\n      \"mode\": \"primary\",\n      \"description\": \"Senior Architect mentor - helpful first, challenging when it matters\",\n      \"prompt\": \"{file:./AGENTS.md}\",\n      \"tools\": {\n        \"write\": true,\n        \"edit\": true\n      }\n    },\n    \"sdd-orchestrator\": {\n      \"mode\": \"all\",\n      \"description\": \"Gentleman personality + SDD delegate-only orchestrator\",\n      \"prompt\": \"{file:./AGENTS.md}\",\n      \"tools\": {\n        \"read\": true,\n        \"write\": true,\n        \"edit\": true,\n        \"bash\": true\n      }\n    }\n  }\n}\n")

func Inject(homeDir string, adapter agents.Adapter, persona model.PersonaID) (InjectionResult, error) {
	if !adapter.SupportsSystemPrompt() {
		return InjectionResult{}, nil
	}

	// Custom persona does nothing — user keeps their own config.
	if persona == model.PersonaCustom {
		return InjectionResult{}, nil
	}

	files := make([]string, 0, 3)
	changed := false

	content := personaContent(adapter.Agent(), persona)
	if content == "" {
		return InjectionResult{}, nil
	}

	// 1. Inject persona content based on system prompt strategy.
	switch adapter.SystemPromptStrategy() {
	case model.StrategyMarkdownSections:
		promptPath := adapter.SystemPromptFile(homeDir)
		existing, err := readFileOrEmpty(promptPath)
		if err != nil {
			return InjectionResult{}, err
		}

		// Auto-heal: strip any legacy free-text Gentleman persona block that was
		// written before the marker-based injection system existed. This prevents
		// duplicate persona content when users re-run the installer after an old
		// install placed the persona as raw text above the <!-- gentle-ai: --> markers.
		healed := filemerge.StripLegacyPersonaBlock(existing)

		updated := filemerge.InjectMarkdownSection(healed, "persona", content)

		writeResult, err := filemerge.WriteFileAtomic(promptPath, []byte(updated), 0o644)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || writeResult.Changed
		files = append(files, promptPath)

	case model.StrategyFileReplace:
		promptPath := adapter.SystemPromptFile(homeDir)

		// For non-Gentleman personas (e.g. neutral), the content is just a short
		// one-liner. Writing ONLY that content would destroy any SDD/engram
		// sections that are injected later in the pipeline. Instead, we write the
		// persona content as the base and let subsequent inject steps (SDD, engram)
		// append their sections. For Gentleman, the content is the full persona
		// asset which is safe to write as-is.
		//
		// If the file already exists and has managed sections (SDD, engram), we
		// must preserve them — replace only the persona portion at the top.
		existing, readErr := readFileOrEmpty(promptPath)
		if readErr != nil {
			return InjectionResult{}, readErr
		}

		if preserved, ok := preserveManagedSections(existing, content, persona); ok {
			writeResult, err := filemerge.WriteFileAtomic(promptPath, []byte(preserved), 0o644)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || writeResult.Changed
			files = append(files, promptPath)
			break
		}

		writeResult, err := filemerge.WriteFileAtomic(promptPath, []byte(content), 0o644)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || writeResult.Changed
		files = append(files, promptPath)

	case model.StrategyInstructionsFile:
		promptPath := adapter.SystemPromptFile(homeDir)

		// Auto-heal: remove any stale Gentleman persona content left at the
		// old VSCode path (~/.github/copilot-instructions.md) that was written
		// by an older installer version.  VS Code still reads that path for
		// global instructions, so the two files would conflict.
		if cleaned, cleanErr := cleanLegacyVSCodePersona(homeDir); cleanErr == nil && cleaned {
			changed = true
		}

		// For non-Gentleman personas, preserve managed sections (same logic
		// as StrategyFileReplace above).
		existing, readErr := readFileOrEmpty(promptPath)
		if readErr != nil {
			return InjectionResult{}, readErr
		}

		if preserved, ok := preserveManagedSections(existing, wrapInstructionsFile(content), persona); ok {
			writeResult, err := filemerge.WriteFileAtomic(promptPath, []byte(preserved), 0o644)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || writeResult.Changed
			files = append(files, promptPath)
			break
		}

		// Write the new instructions file (with YAML frontmatter) to the current path.
		// WriteFileAtomic compares bytes, so it is naturally idempotent: it rewrites
		// whenever the on-disk content differs from instructionsContent, which covers
		// the case where an older install wrote persona content without frontmatter.
		instructionsContent := wrapInstructionsFile(content)
		writeResult, err := filemerge.WriteFileAtomic(promptPath, []byte(instructionsContent), 0o644)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || writeResult.Changed
		files = append(files, promptPath)

	case model.StrategyAppendToFile:
		promptPath := adapter.SystemPromptFile(homeDir)

		// Read existing content if file exists
		existing, err := readFileOrEmpty(promptPath)
		if err != nil {
			return InjectionResult{}, err
		}

		// Idempotency: skip if persona content is already present in the file.
		if strings.Contains(existing, strings.TrimSpace(content)) {
			return InjectionResult{Files: []string{promptPath}}, nil
		}

		// Do a real append: preserve existing content + add new content
		updated := existing
		if len(updated) > 0 && !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		if len(updated) > 0 {
			updated += "\n"
		}
		updated += content

		writeResult, err := filemerge.WriteFileAtomic(promptPath, []byte(updated), 0o644)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || writeResult.Changed
		files = append(files, promptPath)
	}

	// 2. OpenCode agent definitions — Tab-switchable agents in opencode.json.
	if adapter.Agent() == model.AgentOpenCode && persona != model.PersonaCustom {
		settingsPath := adapter.SettingsPath(homeDir)
		if settingsPath != "" {
			agentResult, err := mergeJSONFile(settingsPath, openCodeAgentOverlayJSON)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || agentResult.Changed
			files = append(files, settingsPath)
		}
	}

	// 3. Gentleman-only: write output style + merge into settings (if agent supports it).
	if persona == model.PersonaGentleman && adapter.SupportsOutputStyles() {
		outputStyleDir := adapter.OutputStyleDir(homeDir)
		if outputStyleDir != "" {
			outputStylePath := outputStyleDir + "/gentleman.md"
			outputStyleContent := assets.MustRead("claude/output-style-gentleman.md")

			styleResult, err := filemerge.WriteFileAtomic(outputStylePath, []byte(outputStyleContent), 0o644)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || styleResult.Changed
			files = append(files, outputStylePath)
		}

		// Merge "outputStyle": "Gentleman" into settings.
		settingsPath := adapter.SettingsPath(homeDir)
		if settingsPath != "" {
			settingsResult, err := mergeJSONFile(settingsPath, outputStyleOverlayJSON)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || settingsResult.Changed
			files = append(files, settingsPath)
		}
	}

	return InjectionResult{Changed: changed, Files: files}, nil
}

func personaContent(agent model.AgentID, persona model.PersonaID) string {
	switch persona {
	case model.PersonaNeutral:
		// Neutral persona: same teacher, same philosophy, no regional language.
		return assets.MustRead("generic/persona-neutral.md")
	case model.PersonaCustom:
		return ""
	default:
		// Gentleman persona — try agent-specific asset, then generic fallback.
		switch agent {
		case model.AgentClaudeCode:
			return assets.MustRead("claude/persona-gentleman.md")
		case model.AgentOpenCode:
			return assets.MustRead("opencode/persona-gentleman.md")
		default:
			// Generic persona includes Gentleman personality + skills table + SDD orchestrator.
			// Used by Gemini CLI, Cursor, VS Code Copilot, and any future agents.
			return assets.MustRead("generic/persona-gentleman.md")
		}
	}
}

func mergeJSONFile(path string, overlay []byte) (filemerge.WriteResult, error) {
	baseJSON, err := osReadFile(path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	merged, err := filemerge.MergeJSONObjects(baseJSON, overlay)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	return filemerge.WriteFileAtomic(path, merged, 0o644)
}

var osReadFile = func(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read json file %q: %w", path, err)
	}

	return content, nil
}

// preserveManagedSections checks whether the existing file content has
// gentle-ai managed sections (SDD orchestrator, engram protocol, etc.) and
// returns new content that preserves those sections while replacing only the
// persona text before them. Returns ("", false) when no preservation is needed
// (empty file, Gentleman persona, or no managed markers found).
func preserveManagedSections(existing, newPersona string, persona model.PersonaID) (string, bool) {
	if existing == "" || persona == model.PersonaGentleman {
		return "", false
	}

	idx := strings.Index(existing, "<!-- gentle-ai:")
	if idx < 0 {
		return "", false
	}

	managedSuffix := existing[idx:]
	updated := newPersona
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	if idx > 0 {
		// There was persona content before the markers — add a blank line separator.
		updated += "\n"
	}
	updated += managedSuffix

	return updated, true
}

func readFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read file %q: %w", path, err)
	}
	return string(data), nil
}

func wrapInstructionsFile(content string) string {
	frontmatter := "---\n" +
		"name: Gentle AI Persona\n" +
		"description: Teaching-oriented persona with SDD orchestration and Engram protocol\n" +
		"applyTo: \"**\"\n" +
		"---\n\n"

	return frontmatter + content
}

// isLegacyUnwrappedPersona reports whether content looks like a Gentleman persona
// file that was written without YAML frontmatter by an older installer version.
// It returns true when the content carries known persona fingerprints but does NOT
// start with the YAML front-matter block ("---\n").
func isLegacyUnwrappedPersona(content string) bool {
	if strings.HasPrefix(content, "---\n") {
		// Already has YAML frontmatter — not a legacy file.
		return false
	}
	// Must contain at least one characteristic persona fingerprint.
	personaFingerprints := []string{
		"## Personality",
		"Senior Architect",
	}
	for _, fp := range personaFingerprints {
		if strings.Contains(content, fp) {
			return true
		}
	}
	return false
}

// legacyVSCodePersonaPaths returns the old VS Code persona file paths that may
// contain stale Gentleman persona content from older installer versions.
// These paths are no longer written by the current installer but may still
// be read by VS Code, causing conflicting instructions.
func legacyVSCodePersonaPaths(homeDir string) []string {
	return []string{
		// v1 path: wrote raw persona to ~/.github/copilot-instructions.md
		filepath.Join(homeDir, ".github", "copilot-instructions.md"),
	}
}

// cleanLegacyVSCodePersona removes Gentleman persona content from any old VS Code
// persona file paths that are no longer written by the current installer.
// Only files that contain clear Gentleman persona fingerprints are removed —
// files with user-written content are left untouched.
// Returns true if at least one file was cleaned.
func cleanLegacyVSCodePersona(homeDir string) (bool, error) {
	cleaned := false
	for _, oldPath := range legacyVSCodePersonaPaths(homeDir) {
		data, err := os.ReadFile(oldPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return cleaned, fmt.Errorf("read legacy vscode persona %q: %w", oldPath, err)
		}

		if !isLegacyUnwrappedPersona(string(data)) {
			// File exists but doesn't look like a Gentleman persona — leave it alone.
			continue
		}

		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			return cleaned, fmt.Errorf("remove legacy vscode persona %q: %w", oldPath, err)
		}
		cleaned = true
	}
	return cleaned, nil
}
