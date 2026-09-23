package skillregistry

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderSkillsTableShape pins the four-column format of spec §3.4: fixed
// header, rows sorted by Skill ascending, and one recognizable block for every
// destination (D-09).
func TestRenderSkillsTableShape(t *testing.T) {
	cwd := t.TempDir()
	entries := []SkillEntry{
		{Name: "zeta", Path: cwd + "/skills/zeta/SKILL.md", Description: "last"},
		{Name: "alpha", Path: cwd + "/skills/alpha/SKILL.md", Description: "first"},
	}

	got := renderSkillsTable(cwd, entries, PathDiscovered)
	wantHeader := "| Skill | Trigger / description | Scope | Path |\n| --- | --- | --- | --- |"
	if !strings.HasPrefix(got, wantHeader) {
		t.Fatalf("table header = %q, want prefix %q", got, wantHeader)
	}
	alpha := strings.Index(got, "| `alpha` |")
	zeta := strings.Index(got, "| `zeta` |")
	if alpha < 0 || zeta < 0 || alpha > zeta {
		t.Fatalf("rows not sorted by Skill ascending:\n%s", got)
	}
}

// TestMarkdownCell covers the three cell rules of spec §3.4.
func TestMarkdownCell(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty becomes em dash", value: "", want: "—"},
		{name: "blank becomes em dash", value: "   ", want: "—"},
		{name: "newlines collapse to spaces", value: "line1\nline2", want: "line1 line2"},
		{name: "pipe is escaped", value: "a|b", want: `a\|b`},
		{name: "plain text is trimmed", value: " hello ", want: "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := markdownCell(tt.value); got != tt.want {
				t.Fatalf("markdownCell(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestRenderSkillsTablePathFormat pins decision O-2 (2026-09-23): the AGENTS.md
// Path column is repository-relative for project skills and discovered for
// user skills; .atl/skill-registry.md keeps the discovered path. Both come from
// the same renderer, parameterized by path format.
func TestRenderSkillsTablePathFormat(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	projectSkill := filepath.Join(cwd, "skills", "go-testing", "SKILL.md")
	userSkill := filepath.Join(home, "skills", "branch-pr", "SKILL.md")
	entries := []SkillEntry{
		{Name: "branch-pr", Path: userSkill, Description: "user skill"},
		{Name: "go-testing", Path: projectSkill, Description: "project skill"},
	}

	tests := []struct {
		name     string
		format   PathFormat
		wantPath string
		dontWant string
	}{
		{
			name:     "discovered keeps exact project path",
			format:   PathDiscovered,
			wantPath: projectSkill,
			dontWant: "skills/go-testing/SKILL.md",
		},
		{
			name:     "repo relative renders project path with slashes",
			format:   PathRepoRelative,
			wantPath: "skills/go-testing/SKILL.md",
			dontWant: projectSkill,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderSkillsTable(cwd, entries, tt.format)
			if !strings.Contains(got, "`"+tt.wantPath+"`") {
				t.Fatalf("table missing Path cell %q:\n%s", tt.wantPath, got)
			}
			if strings.Contains(got, tt.dontWant) {
				t.Fatalf("table unexpectedly contains %q:\n%s", tt.dontWant, got)
			}
			// User-scope skills always keep the discovered path: they live
			// outside the repository and have no relative form (spec §3.4).
			if !strings.Contains(got, "`"+userSkill+"`") {
				t.Fatalf("table missing user-scope Path cell %q:\n%s", userSkill, got)
			}
		})
	}
}

// TestRenderSkillsTableIsTheOnlyRenderer proves there is one renderer: the
// block RenderRegistry embeds and the managed AGENTS.md body use the same
// table output (modulo the O-2 path format parameter).
func TestRenderSkillsTableIsTheOnlyRenderer(t *testing.T) {
	cwd := t.TempDir()
	entries := []SkillEntry{
		{Name: "go-testing", Path: filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), Description: "Go tests"},
	}

	registry := RenderRegistry(cwd, []string{"skills"}, entries)
	sameBlock := renderSkillsTable(cwd, entries, PathDiscovered)
	if !strings.Contains(registry, sameBlock) {
		t.Fatalf("RenderRegistry does not embed renderSkillsTable output:\n%s", registry)
	}

	body := skillsIndexBody(cwd, entries)
	if !strings.HasPrefix(body, "## Skills\n\n") {
		t.Fatalf("managed body must open with the title and a blank line, got %q", body)
	}
	if !strings.HasSuffix(body, "\n") {
		t.Fatalf("managed body must end with one final newline, got %q", body)
	}
	if !strings.Contains(body, renderSkillsTable(cwd, entries, PathRepoRelative)) {
		t.Fatalf("managed body does not embed renderSkillsTable output:\n%s", body)
	}
}
