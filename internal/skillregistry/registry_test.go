package skillregistry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/antigravity"
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

func TestAntigravitySkillDiscoveryMatchesActiveVariant(t *testing.T) {
	type fixtureSkill struct {
		scope       string
		path        string
		name        string
		description string
	}
	type expectedSkill struct {
		scope       string
		path        string
		name        string
		description string
	}
	tests := []struct {
		name          string
		skills        []fixtureSkill
		want          []expectedSkill
		activeVariant string
	}{
		{
			name: "CLI only",
			skills: []fixtureSkill{
				{scope: "user", path: ".gemini/antigravity-cli/skills/cli-only/SKILL.md", name: "cli-only", description: "CLI only"},
			},
			want: []expectedSkill{
				{scope: "user", path: ".gemini/antigravity-cli/skills/cli-only/SKILL.md", name: "cli-only", description: "CLI only"},
			},
			activeVariant: ".gemini/antigravity-cli/skills",
		},
		{
			name: "Desktop only",
			skills: []fixtureSkill{
				{scope: "user", path: ".gemini/antigravity-desktop/skills/desktop-only/SKILL.md", name: "desktop-only", description: "Desktop only"},
			},
			want: []expectedSkill{
				{scope: "user", path: ".gemini/antigravity-desktop/skills/desktop-only/SKILL.md", name: "desktop-only", description: "Desktop only"},
			},
			activeVariant: ".gemini/antigravity-desktop/skills",
		},
		{
			name: "both variants preserve compatibility roots and precedence",
			skills: []fixtureSkill{
				{scope: "user", path: ".gemini/antigravity-cli/skills/cli-only/SKILL.md", name: "cli-only", description: "CLI only"},
				{scope: "user", path: ".gemini/antigravity-cli/skills/duplicate/SKILL.md", name: "duplicate", description: "CLI duplicate"},
				{scope: "user", path: ".gemini/antigravity-desktop/skills/desktop-only/SKILL.md", name: "desktop-only", description: "Desktop only"},
				{scope: "user", path: ".gemini/antigravity-desktop/skills/duplicate/SKILL.md", name: "duplicate", description: "Desktop duplicate"},
				{scope: "user", path: ".gemini/antigravity-desktop/skills/project-wins/SKILL.md", name: "project-wins", description: "Desktop user copy"},
				{scope: "user", path: ".gemini/antigravity/skills/legacy-only/SKILL.md", name: "legacy-only", description: "Legacy only"},
				{scope: "user", path: ".gemini/skills/shared-only/SKILL.md", name: "shared-only", description: "Shared only"},
				{scope: "project", path: "skills/project-wins/SKILL.md", name: "project-wins", description: "Project copy"},
			},
			want: []expectedSkill{
				{scope: "user", path: ".gemini/antigravity-cli/skills/cli-only/SKILL.md", name: "cli-only", description: "CLI only"},
				{scope: "user", path: ".gemini/antigravity-desktop/skills/desktop-only/SKILL.md", name: "desktop-only", description: "Desktop only"},
				{scope: "user", path: ".gemini/antigravity-desktop/skills/duplicate/SKILL.md", name: "duplicate", description: "Desktop duplicate"},
				{scope: "user", path: ".gemini/antigravity/skills/legacy-only/SKILL.md", name: "legacy-only", description: "Legacy only"},
				{scope: "project", path: "skills/project-wins/SKILL.md", name: "project-wins", description: "Project copy"},
				{scope: "user", path: ".gemini/skills/shared-only/SKILL.md", name: "shared-only", description: "Shared only"},
			},
			activeVariant: ".gemini/antigravity-desktop/skills",
		},
		{
			name: "missing variant directories preserve shared and legacy roots",
			skills: []fixtureSkill{
				{scope: "user", path: ".gemini/antigravity/skills/legacy-only/SKILL.md", name: "legacy-only", description: "Legacy only"},
				{scope: "user", path: ".gemini/skills/shared-only/SKILL.md", name: "shared-only", description: "Shared only"},
			},
			want: []expectedSkill{
				{scope: "user", path: ".gemini/antigravity/skills/legacy-only/SKILL.md", name: "legacy-only", description: "Legacy only"},
				{scope: "user", path: ".gemini/skills/shared-only/SKILL.md", name: "shared-only", description: "Shared only"},
			},
			activeVariant: ".gemini/antigravity-cli/skills",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cwd := t.TempDir()
			home := t.TempDir()
			pathFor := func(scope, path string) string {
				if scope == "project" {
					return filepath.Join(cwd, filepath.FromSlash(path))
				}
				return filepath.Join(home, filepath.FromSlash(path))
			}

			all := make([]SkillEntry, len(tt.skills))
			for i, skill := range tt.skills {
				writeSkill(t, pathFor(skill.scope, skill.path), "---\nname: "+skill.name+"\ndescription: "+skill.description+"\n---\n")
				all[i] = SkillEntry{
					Name:        skill.name,
					Path:        pathFor(skill.scope, skill.path),
					Description: skill.description,
				}
			}

			activeSkillsDir := antigravity.NewAdapter().SkillsDir(home)
			if want := filepath.Join(home, filepath.FromSlash(tt.activeVariant)); activeSkillsDir != want {
				t.Fatalf("active Antigravity skills dir = %q, want %q", activeSkillsDir, want)
			}

			want := make([]SkillEntry, len(tt.want))
			for i, skill := range tt.want {
				want[i] = SkillEntry{
					Name:        skill.name,
					Path:        pathFor(skill.scope, skill.path),
					Description: skill.description,
				}
			}
			t.Run("List", func(t *testing.T) {
				assertSkillEntries(t, List(cwd, home), want)
				assertSkillEntries(t, List(cwd, home), want)
			})

			t.Run("Regenerate", func(t *testing.T) {
				first, err := Regenerate(cwd, home, false)
				if err != nil {
					t.Fatalf("Regenerate() error = %v", err)
				}
				if !first.Regenerated || first.SkillCount != len(want) || first.Reason != "fingerprint-changed" {
					t.Fatalf("first result = %#v, want %d regenerated skills", first, len(want))
				}
				registry := readFile(t, filepath.Join(cwd, RegistryRelPath))
				assertRegistrySkills(t, registry, want, all)

				second, err := Regenerate(cwd, home, false)
				if err != nil {
					t.Fatalf("second Regenerate() error = %v", err)
				}
				if second.Regenerated || second.Reason != "cache-hit" {
					t.Fatalf("second result = %#v, want unchanged cache hit", second)
				}
				if got := readFile(t, filepath.Join(cwd, RegistryRelPath)); got != registry {
					t.Fatal("registry changed on deterministic cache-hit run")
				}

				forced, err := Regenerate(cwd, home, true)
				if err != nil {
					t.Fatalf("forced Regenerate() error = %v", err)
				}
				if !forced.Regenerated || forced.SkillCount != len(want) || forced.Reason != "forced" {
					t.Fatalf("forced result = %#v, want %d regenerated skills", forced, len(want))
				}
				if got := readFile(t, filepath.Join(cwd, RegistryRelPath)); got != registry {
					t.Fatal("registry changed on deterministic forced regeneration")
				}
			})
		})
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
		filepath.Join(home, ".gemini", "antigravity-cli", "skills"),
		filepath.Join(home, ".gemini", "antigravity-desktop", "skills"),
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
			gotName, gotDesc := parseFrontmatter(tc.source)
			if gotName != tc.wantName {
				t.Errorf("name = %q, want %q", gotName, tc.wantName)
			}
			if gotDesc != tc.wantDesc {
				t.Errorf("description = %q, want %q", gotDesc, tc.wantDesc)
			}
		})
	}
}

func TestFindAllSkillFilesFollowsSymlinkedSkillDir(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	// A real skill living outside the project, linked in as a project skill.
	// filepath.WalkDir would silently skip this; the one-level scan must not.
	realDir := filepath.Join(t.TempDir(), "linked-skill")
	writeSkill(t, filepath.Join(realDir, "SKILL.md"), `---
name: linked
description: linked via symlink
---

## Hard Rules

- ok
`)
	linkParent := filepath.Join(cwd, "skills")
	if err := os.MkdirAll(linkParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, filepath.Join(linkParent, "linked")); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	result, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1 (symlinked skill dir must be indexed)", result.SkillCount)
	}
	registry := readFile(t, filepath.Join(cwd, RegistryRelPath))
	if !strings.Contains(registry, "`linked`") || !strings.Contains(registry, "linked via symlink") {
		t.Fatalf("registry missing symlinked skill:\n%s", registry)
	}
}

func TestRegenerateIgnoresNestedSkillMd(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "parent", "SKILL.md"), `---
name: parent
description: top-level skill
---

## Hard Rules

- ok
`)
	// A SKILL.md bundled as an example inside the skill must not be indexed.
	writeSkill(t, filepath.Join(cwd, "skills", "parent", "examples", "SKILL.md"), `---
name: nested-example
description: should not be indexed
---

## Hard Rules

- no
`)

	result, err := Regenerate(cwd, home, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1 (nested SKILL.md must be ignored)", result.SkillCount)
	}
	registry := readFile(t, filepath.Join(cwd, RegistryRelPath))
	if strings.Contains(registry, "nested-example") || strings.Contains(registry, "should not be indexed") {
		t.Fatalf("nested SKILL.md leaked into registry:\n%s", registry)
	}
}

func TestListReturnsDedupedEntriesWithoutWriting(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".claude", "skills", "dup", "SKILL.md"), "---\nname: dup\ndescription: user copy\n---\n")
	writeSkill(t, filepath.Join(cwd, "skills", "dup", "SKILL.md"), "---\nname: dup\ndescription: project copy\n---\n")
	writeSkill(t, filepath.Join(cwd, "skills", "solo", "SKILL.md"), "---\nname: solo\ndescription: solo\n---\n")

	entries := List(cwd, home)
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	// Sorted by name: dup, solo. The project copy of dup must win.
	if entries[0].Name != "dup" || entries[0].Description != "project copy" {
		t.Fatalf("entries[0] = %#v, want project dup", entries[0])
	}
	if got := ScopeForPath(cwd, entries[0].Path); got != "project" {
		t.Fatalf("dup scope = %q, want project", got)
	}
	// List is read-only: it must not write the registry or cache.
	if _, err := os.Stat(filepath.Join(cwd, RegistryRelPath)); !os.IsNotExist(err) {
		t.Fatalf("List wrote registry (stat err = %v), want it absent", err)
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

func assertSkillEntries(t *testing.T, got, want []SkillEntry) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(entries) = %d, want %d\nentries = %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entries[%d] = %#v, want %#v\nall entries = %#v", i, got[i], want[i], got)
		}
	}
}

func assertRegistrySkills(
	t *testing.T,
	registry string,
	want []SkillEntry,
	all []SkillEntry,
) {
	t.Helper()
	lastIndex := -1
	selected := make(map[string]bool, len(want))
	for _, skill := range want {
		selected[skill.Path] = true
		index := strings.Index(registry, "`"+skill.Path+"`")
		if index == -1 {
			t.Fatalf("registry missing selected skill %q:\n%s", skill.Path, registry)
		}
		if index <= lastIndex {
			t.Fatalf("registry skills are not in deterministic name order:\n%s", registry)
		}
		lastIndex = index
	}
	for _, skill := range all {
		if !selected[skill.Path] && strings.Contains(registry, "`"+skill.Path+"`") {
			t.Fatalf("registry includes lower-precedence skill %q:\n%s", skill.Path, registry)
		}
	}
}
