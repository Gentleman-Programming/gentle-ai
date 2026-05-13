package skillregistry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegenerateWritesRegistryAndCacheThenHitsCache(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "react", "SKILL.md"), `---
name: react
description: React patterns
---

## Compact Rules

- Prefer composition.
- Keep state local.
`)

	if err := EnsureATLIgnored(cwd); err != nil {
		t.Fatalf("EnsureATLIgnored() error = %v", err)
	}
	first, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatalf("Regenerate() error = %v", err)
	}
	if !first.Regenerated || first.SkillCount != 1 || first.Reason != "fingerprint-changed" {
		t.Fatalf("first result = %#v", first)
	}
	registry, err := os.ReadFile(filepath.Join(cwd, RegistryRelPath))
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	for _, want := range []string{"### react", "- Trigger: React patterns", "  - Prefer composition."} {
		if !strings.Contains(string(registry), want) {
			t.Fatalf("registry missing %q:\n%s", want, registry)
		}
	}
	if _, err := os.Stat(filepath.Join(cwd, CacheRelPath)); err != nil {
		t.Fatalf("cache missing: %v", err)
	}

	second, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatalf("second Regenerate() error = %v", err)
	}
	if second.Regenerated || second.Reason != "cache-hit" {
		t.Fatalf("second result = %#v", second)
	}
}

func TestRegenerateForceBypassesCacheAndProjectSkillWins(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".claude", "skills", "dup", "SKILL.md"), `---
name: dup
description: user copy
---

## Compact Rules

- User rule.
`)
	writeSkill(t, filepath.Join(cwd, "skills", "dup", "SKILL.md"), `---
name: dup
description: project copy
---

## Compact Rules

- Project rule.
`)

	first, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1", first.SkillCount)
	}
	forced, err := Regenerate(cwd, home, true)
	if err != nil {
		t.Fatal(err)
	}
	if !forced.Regenerated || forced.Reason != "forced" {
		t.Fatalf("forced result = %#v", forced)
	}
	registry := readFile(t, filepath.Join(cwd, RegistryRelPath))
	if !strings.Contains(registry, "Project rule") || strings.Contains(registry, "User rule") {
		t.Fatalf("project skill should win over user duplicate:\n%s", registry)
	}
}

func TestRegenerateScansProjectOpenCodeSkillsBeforeGlobalOpenCode(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".config", "opencode", "skills", "dup", "SKILL.md"), `---
name: dup
description: global OpenCode copy
---

## Compact Rules

- Global OpenCode rule.
`)
	writeSkill(t, filepath.Join(cwd, ".opencode", "skills", "dup", "SKILL.md"), `---
name: dup
description: project OpenCode copy
---

## Compact Rules

- Project OpenCode rule.
`)

	result, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1", result.SkillCount)
	}
	registry := readFile(t, filepath.Join(cwd, RegistryRelPath))
	for _, want := range []string{"- .opencode/skills", "Project OpenCode rule"} {
		if !strings.Contains(registry, want) {
			t.Fatalf("registry missing %q:\n%s", want, registry)
		}
	}
	if strings.Contains(registry, "Global OpenCode rule") {
		t.Fatalf("project .opencode skill should win over global duplicate:\n%s", registry)
	}
}

func TestRegenerateKeepsUserSkillSourceOrderForGlobalDuplicates(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".claude", "skills", "dup", "SKILL.md"), `---
name: dup
description: Claude copy
---

## Compact Rules

- Claude rule.
`)
	writeSkill(t, filepath.Join(home, ".config", "opencode", "skills", "dup", "SKILL.md"), `---
name: dup
description: OpenCode copy
---

## Compact Rules

- OpenCode rule.
`)

	result, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1", result.SkillCount)
	}
	registry := readFile(t, filepath.Join(cwd, RegistryRelPath))
	if !strings.Contains(registry, "OpenCode rule") || strings.Contains(registry, "Claude rule") {
		t.Fatalf("user duplicate should respect UserSkillDirs source order:\n%s", registry)
	}
}

func TestUserSkillDirsIncludesSupportedAgentSkillLocations(t *testing.T) {
	home := t.TempDir()
	dirs := UserSkillDirs(home)

	for _, want := range []string{
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(home, ".config", "kilo", "skills"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".gemini", "skills"),
		filepath.Join(home, ".gemini", "antigravity", "skills"),
		filepath.Join(home, ".cursor", "skills"),
		filepath.Join(home, ".copilot", "skills"),
		filepath.Join(home, ".codex", "skills"),
		filepath.Join(home, ".codeium", "windsurf", "skills"),
		filepath.Join(home, ".config", "agents", "skills"),
		filepath.Join(home, ".kimi", "skills"),
		filepath.Join(home, ".qwen", "skills"),
		filepath.Join(home, ".kiro", "skills"),
		filepath.Join(home, ".openclaw", "skills"),
		filepath.Join(home, ".pi", "agent", "skills"),
		filepath.Join(home, ".agents", "skills"),
	} {
		if !containsPath(dirs, want) {
			t.Fatalf("UserSkillDirs() missing %q in %#v", want, dirs)
		}
	}
}

func TestProjectSkillDirsIncludesWorkspaceSkillLocations(t *testing.T) {
	cwd := t.TempDir()
	dirs := ProjectSkillDirs(cwd)

	for _, want := range []string{
		filepath.Join(cwd, "skills"),
		filepath.Join(cwd, ".opencode", "skills"),
		filepath.Join(cwd, ".claude", "skills"),
		filepath.Join(cwd, ".gemini", "skills"),
		filepath.Join(cwd, ".cursor", "skills"),
		filepath.Join(cwd, ".github", "skills"),
		filepath.Join(cwd, ".codex", "skills"),
		filepath.Join(cwd, ".qwen", "skills"),
		filepath.Join(cwd, ".kiro", "skills"),
		filepath.Join(cwd, ".openclaw", "skills"),
		filepath.Join(cwd, ".pi", "skills"),
		filepath.Join(cwd, ".agent", "skills"),
		filepath.Join(cwd, ".agents", "skills"),
		filepath.Join(cwd, ".atl", "skills"),
	} {
		if !containsPath(dirs, want) {
			t.Fatalf("ProjectSkillDirs() missing %q in %#v", want, dirs)
		}
	}
}

func TestRegenerateExtractsHardRulesWhenCompactRulesAreAbsent(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), `---
name: go-testing
description: Go testing patterns
---

## Activation Contract

Use this for Go tests.

## Hard Rules

- Run focused tests before broad tests.
- Keep table tests readable.

## Execution Steps

- This should not be copied.
`)

	result, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1", result.SkillCount)
	}
	registry := readFile(t, filepath.Join(cwd, RegistryRelPath))
	for _, want := range []string{"Run focused tests before broad tests.", "Keep table tests readable."} {
		if !strings.Contains(registry, want) {
			t.Fatalf("registry missing %q:\n%s", want, registry)
		}
	}
	for _, dontWant := range []string{fallbackCompactRules, "This should not be copied."} {
		if strings.Contains(registry, dontWant) {
			t.Fatalf("registry should not contain %q:\n%s", dontWant, registry)
		}
	}
}

func TestRegenerateExtractsLegacyRuleSectionsWhenCompactRulesAreAbsent(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "comment-writer", "SKILL.md"), `---
name: comment-writer
description: Comment writing
---

## Voice Rules

- Be warm and direct.
- Keep it short.

## Critical Rules

1. Link an approved issue.
2. Keep PRs within the review budget.

## Critical Patterns

- Start with the actionable point.
`)

	result, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1", result.SkillCount)
	}
	registry := readFile(t, filepath.Join(cwd, RegistryRelPath))
	for _, want := range []string{"Be warm and direct.", "Keep it short.", "Link an approved issue.", "Keep PRs within the review budget.", "Start with the actionable point."} {
		if !strings.Contains(registry, want) {
			t.Fatalf("registry missing %q:\n%s", want, registry)
		}
	}
	if strings.Contains(registry, fallbackCompactRules) {
		t.Fatalf("registry should not use fallback for legacy rule sections:\n%s", registry)
	}
}

func TestRegeneratePrefersCompactRulesOverFallbackSections(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "explicit", "SKILL.md"), `---
name: explicit
---

## Compact Rules

- Explicit compact rule.

## Hard Rules

- Hard rule should not be copied.
`)

	result, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1", result.SkillCount)
	}
	registry := readFile(t, filepath.Join(cwd, RegistryRelPath))
	if !strings.Contains(registry, "Explicit compact rule.") || strings.Contains(registry, "Hard rule should not be copied.") {
		t.Fatalf("Compact Rules should be preferred over fallback sections:\n%s", registry)
	}
}

func TestRegenerateCapsExtractedFallbackRules(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "many", "SKILL.md"), `---
name: many
---

## Hard Rules

- Rule 01.
- Rule 02.
- Rule 03.
- Rule 04.
- Rule 05.
- Rule 06.
- Rule 07.
- Rule 08.
- Rule 09.
- Rule 10.
- Rule 11.
- Rule 12.
- Rule 13.
- Rule 14.
- Rule 15.
- Rule 16.
`)

	result, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1", result.SkillCount)
	}
	registry := readFile(t, filepath.Join(cwd, RegistryRelPath))
	if !strings.Contains(registry, "Rule 15.") || strings.Contains(registry, "Rule 16.") {
		t.Fatalf("fallback extracted rules should be capped at 15:\n%s", registry)
	}
}

func TestRegenerateExcludesSkillRegistrySharedAndSDD(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "_shared", "SKILL.md"), `---
name: _shared
---

## Compact Rules
- no
`)
	writeSkill(t, filepath.Join(cwd, "skills", "skill-registry", "SKILL.md"), `---
name: skill-registry
---

## Compact Rules
- no
`)
	writeSkill(t, filepath.Join(cwd, "skills", "sdd-apply", "SKILL.md"), `---
name: sdd-apply
---

## Compact Rules
- no
`)
	writeSkill(t, filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), `---
name: go-testing
---

## Compact Rules
- yes
`)
	result, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1", result.SkillCount)
	}
	registry := readFile(t, filepath.Join(cwd, RegistryRelPath))
	if !strings.Contains(registry, "go-testing") || strings.Contains(registry, "### sdd-apply") || strings.Contains(registry, "### skill-registry") {
		t.Fatalf("unexpected registry content:\n%s", registry)
	}
}

func writeSkill(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func containsPath(paths []string, want string) bool {
	want = filepath.Clean(want)
	for _, path := range paths {
		if filepath.Clean(path) == want {
			return true
		}
	}
	return false
}
