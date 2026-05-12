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
