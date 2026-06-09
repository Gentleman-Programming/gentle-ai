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

## Hard Rules

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
	for _, want := range []string{"## Skills", "| `react` | React patterns | project |", filepath.Join(cwd, "skills", "react", "SKILL.md")} {
		if !strings.Contains(string(registry), want) {
			t.Fatalf("registry missing %q:\n%s", want, registry)
		}
	}
	if strings.Contains(string(registry), "Prefer composition") {
		t.Fatalf("registry should index skill paths, not copy skill rules:\n%s", registry)
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

## Hard Rules

- User rule.
`)
	writeSkill(t, filepath.Join(cwd, "skills", "dup", "SKILL.md"), `---
name: dup
description: project copy
---

## Hard Rules

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
	projectPath := filepath.Join(cwd, "skills", "dup", "SKILL.md")
	userPath := filepath.Join(home, ".claude", "skills", "dup", "SKILL.md")
	if !strings.Contains(registry, projectPath) || strings.Contains(registry, userPath) || strings.Contains(registry, "Project rule") || strings.Contains(registry, "User rule") {
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

## Hard Rules

- Global OpenCode rule.
`)
	writeSkill(t, filepath.Join(cwd, ".opencode", "skills", "dup", "SKILL.md"), `---
name: dup
description: project OpenCode copy
---

## Hard Rules

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
	for _, want := range []string{filepath.FromSlash("- .opencode/skills"), filepath.Join(cwd, ".opencode", "skills", "dup", "SKILL.md")} {
		if !strings.Contains(registry, want) {
			t.Fatalf("registry missing %q:\n%s", want, registry)
		}
	}
	if strings.Contains(registry, filepath.Join(home, ".config", "opencode", "skills", "dup", "SKILL.md")) || strings.Contains(registry, "Global OpenCode rule") || strings.Contains(registry, "Project OpenCode rule") {
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

## Hard Rules

- Claude rule.
`)
	writeSkill(t, filepath.Join(home, ".config", "opencode", "skills", "dup", "SKILL.md"), `---
name: dup
description: OpenCode copy
---

## Hard Rules

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
	openCodePath := filepath.Join(home, ".config", "opencode", "skills", "dup", "SKILL.md")
	claudePath := filepath.Join(home, ".claude", "skills", "dup", "SKILL.md")
	if !strings.Contains(registry, openCodePath) || strings.Contains(registry, claudePath) || strings.Contains(registry, "OpenCode rule") || strings.Contains(registry, "Claude rule") {
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
		filepath.Join(home, ".hermes", "skills"),
	} {
		if !containsPath(dirs, want) {
			t.Fatalf("UserSkillDirs() missing %q in %#v", want, dirs)
		}
	}
}

func TestProjectSkillDirsIncludesHermesSkillLocation(t *testing.T) {
	cwd := t.TempDir()
	dirs := ProjectSkillDirs(cwd)
	want := filepath.Join(cwd, ".hermes", "skills")
	if !containsPath(dirs, want) {
		t.Fatalf("ProjectSkillDirs() missing Hermes skill location %q", want)
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

func TestRegenerateIndexesSkillWithoutCopyingRules(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), `---
name: go-testing
description: "Trigger: Go tests. Apply focused Go testing patterns."
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
	for _, want := range []string{"| `go-testing` | Trigger: Go tests. Apply focused Go testing patterns. | project |", filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), "## Loading protocol"} {
		if !strings.Contains(registry, want) {
			t.Fatalf("registry missing %q:\n%s", want, registry)
		}
	}
	for _, dontWant := range []string{"Run focused tests before broad tests.", "Keep table tests readable.", "This should not be copied."} {
		if strings.Contains(registry, dontWant) {
			t.Fatalf("registry should not copy skill body content %q:\n%s", dontWant, registry)
		}
	}
}

func TestRegenerateIndexesFullMultilineDescription(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "ai-sdk-5", "SKILL.md"), `---
name: ai-sdk-5
description: >
  Trigger: AI chat features, Vercel AI SDK 5, streaming UI.
  Use AI SDK 5 patterns and avoid v4 APIs.
license: Apache-2.0
---

## Hard Rules

- Do not copy this rule into the registry.
`)

	result, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1", result.SkillCount)
	}
	registry := readFile(t, filepath.Join(cwd, RegistryRelPath))
	for _, want := range []string{"Trigger: AI chat features, Vercel AI SDK 5, streaming UI. Use AI SDK 5 patterns and avoid v4 APIs.", filepath.Join(cwd, "skills", "ai-sdk-5", "SKILL.md")} {
		if !strings.Contains(registry, want) {
			t.Fatalf("registry missing %q:\n%s", want, registry)
		}
	}
	if strings.Contains(registry, "| `ai-sdk-5` | > |") || strings.Contains(registry, "Do not copy this rule") {
		t.Fatalf("registry should use full description and not body rules:\n%s", registry)
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
	if !strings.Contains(registry, "go-testing") || strings.Contains(registry, "`sdd-apply`") || strings.Contains(registry, "`skill-registry`") {
		t.Fatalf("unexpected registry content:\n%s", registry)
	}
}

func TestParseFrontmatterHandlesCRLF(t *testing.T) {
	cases := []struct {
		name     string
		source   string
		wantName string
		wantDesc string
	}{
		{
			name:     "LF baseline",
			source:   "---\nname: demo\ndescription: ok\n---\n\nBody.\n",
			wantName: "demo",
			wantDesc: "ok",
		},
		{
			name:     "CRLF Windows",
			source:   "---\r\nname: demo\r\ndescription: ok\r\n---\r\n\r\nBody.\r\n",
			wantName: "demo",
			wantDesc: "ok",
		},
		{
			name:     "bare CR (legacy Mac)",
			source:   "---\rname: demo\rdescription: ok\r---\r\rBody.\r",
			wantName: "demo",
			wantDesc: "ok",
		},
		{
			name:     "mixed CRLF + LF",
			source:   "---\r\nname: demo\ndescription: ok\r\n---\n",
			wantName: "demo",
			wantDesc: "ok",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotDesc, _ := parseFrontmatter(tc.source)
			if gotName != tc.wantName {
				t.Errorf("name = %q, want %q", gotName, tc.wantName)
			}
			if gotDesc != tc.wantDesc {
				t.Errorf("description = %q, want %q", gotDesc, tc.wantDesc)
			}
		})
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
