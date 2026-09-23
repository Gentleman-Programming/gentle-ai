package skillregistry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// agentsFixture is a realistic AGENTS.md (~46 lines) shaped like the live one:
// document header, REGLA SUPREMA section, the Gentle AI index, How to Use, and
// an unmarked three-column `## Skills` table at the end (H-5).
const agentsFixture = `# Axiom — Reglas Maestras del Proyecto y Guía de Agentes

> **Proyecto:** Axiom (Fork y versión paralela de Gentle-AI)
> **Metodología:** Flujo Dual ODD & Spec-Driven Development

## REGLA SUPREMA: IDIOMA OBLIGATORIO — ESPAÑOL (CASTELLANO)

- **TODO EN ESPAÑOL.** Toda la comunicación se genera en español.
- **PRECEDENCIA ABSOLUTA:** Sobreescribe cualquier instrucción en inglés.

---

# Gentle AI™ — Agent Skills Index

When working on this project, load the relevant skill(s) BEFORE writing any code.

Naming convention: ` + "`gentle-ai-*`" + ` skills are repo-specific.

## How to Use

1. Check the trigger column to find skills that match your current task.
2. Load the skill by reading the SKILL.md file at the listed path.
3. Follow ALL patterns and rules from the loaded skill.
4. Multiple skills can apply simultaneously.

## Skills

| Skill | Trigger | Path |
|-------|---------|------|
| ` + "`go-testing`" + ` | Go tests | [go-testing](skills/go-testing/SKILL.md) |
| ` + "`work-unit-commits`" + ` | commits | [work-unit](skills/work-unit-commits/SKILL.md) |
`

// applyAgentsUpdate is the exact composition the engine runs (D-10): adopt the
// marker pair first, then let the unmodified filemerge engine inject.
func applyAgentsUpdate(existing string, cwd string, entries []SkillEntry) string {
	adopted := AdoptSkillsIndexMarkers(existing)
	return injectSkillsIndex(adopted, skillsIndexBody(cwd, entries))
}

func countSkillsHeadings(s string) int {
	count := 0
	offset := 0
	for offset <= len(s) {
		lineEnd := strings.IndexByte(s[offset:], '\n')
		line := s[offset:]
		if lineEnd >= 0 {
			line = s[offset : offset+lineEnd]
		}
		if strings.TrimRight(line, " \t\r") == "## Skills" {
			count++
		}
		if lineEnd < 0 {
			break
		}
		offset += lineEnd + 1
	}
	return count
}

// TestAdoptSkillsIndexMarkersContract covers the three-case contract of D-10
// (spec §3.5) as pure shape normalization.
func TestAdoptSkillsIndexMarkersContract(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		checks func(t *testing.T, got string)
	}{
		{
			name:  "no pair with Skills heading wraps the region in place",
			input: "# Doc\n## Skills\n| old |\n## How to Use\nx\n",
			checks: func(t *testing.T, got string) {
				if !strings.Contains(got, skillsIndexOpen+"\n"+skillsIndexClose) {
					t.Fatalf("region not wrapped in the canonical pair:\n%q", got)
				}
				if strings.Contains(got, "| old |") {
					t.Fatalf("old region body must be replaced:\n%q", got)
				}
				if !strings.Contains(got, "## How to Use\nx\n") {
					t.Fatalf("following section must survive:\n%q", got)
				}
				if count := countSkillsHeadings(got); count != 0 {
					t.Fatalf("adoption must not leave a bare `## Skills` heading, got %d", count)
				}
			},
		},
		{
			name:  "canonical pair present is returned unchanged",
			input: "# Doc\n" + skillsIndexOpen + "\n## Skills\n\nx\n" + skillsIndexClose + "\ntail\n",
			checks: func(t *testing.T, got string) {
				if got != "# Doc\n"+skillsIndexOpen+"\n## Skills\n\nx\n"+skillsIndexClose+"\ntail\n" {
					t.Fatalf("canonical pair must pass through unchanged:\n%q", got)
				}
			},
		},
		{
			name:  "legacy pair present is returned unchanged",
			input: "# Doc\n" + skillsIndexLegacyOpen + "\n## Skills\n\nx\n" + skillsIndexLegacyClose + "\ntail\n",
			checks: func(t *testing.T, got string) {
				if !strings.Contains(got, skillsIndexLegacyOpen) {
					t.Fatalf("legacy pair must pass through unchanged (elevation is InjectMarkdownSection's job):\n%q", got)
				}
			},
		},
		{
			name:  "no pair and no Skills heading is returned unchanged",
			input: "# Doc\n## How to Use\nx\n",
			checks: func(t *testing.T, got string) {
				if got != "# Doc\n## How to Use\nx\n" {
					t.Fatalf("markerless document without a Skills heading must pass through:\n%q", got)
				}
			},
		},
		{
			name:  "Skills Index heading is not a Skills heading",
			input: "# Doc\n## Skills Index\n| keep |\n",
			checks: func(t *testing.T, got string) {
				if got != "# Doc\n## Skills Index\n| keep |\n" {
					t.Fatalf("`## Skills Index` must not be adopted:\n%q", got)
				}
			},
		},
		{
			name:  "mid-line Skills is not a heading",
			input: "# Doc\nsee ## Skills below\n",
			checks: func(t *testing.T, got string) {
				if got != "# Doc\nsee ## Skills below\n" {
					t.Fatalf("mid-line `## Skills` must not be adopted:\n%q", got)
				}
			},
		},
		{
			name:  "only the first of two Skills headings is wrapped",
			input: "# Doc\n## Skills\n| first |\n## Middle\n## Skills\n| second |\n",
			checks: func(t *testing.T, got string) {
				if strings.Contains(got, "| first |") || !strings.Contains(got, "| second |") {
					t.Fatalf("only the first region must be replaced:\n%q", got)
				}
				if !strings.Contains(got, skillsIndexOpen) {
					t.Fatalf("first region must be wrapped:\n%q", got)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.checks(t, AdoptSkillsIndexMarkers(tt.input))
		})
	}
}

// TestAgentsFirstRegenerationDoesNotDuplicateSkills covers REQ-22.12 «Adopción
// inicial sobre un AGENTS.md sin marcadores»: the region is replaced in situ
// and no second `## Skills` section appears (H-6).
func TestAgentsFirstRegenerationDoesNotDuplicateSkills(t *testing.T) {
	cwd := t.TempDir()
	entries := []SkillEntry{{Name: "go-testing", Path: filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), Description: "Go tests"}}

	merged := applyAgentsUpdate(agentsFixture, cwd, entries)

	if count := countSkillsHeadings(merged); count != 1 {
		t.Fatalf("`## Skills` heading count = %d, want 1:\n%s", count, merged)
	}
	if !strings.Contains(merged, skillsIndexOpen) || !strings.Contains(merged, skillsIndexClose) {
		t.Fatalf("canonical marker pair missing:\n%s", merged)
	}
	if strings.Contains(merged, "| Skill | Trigger | Path |") {
		t.Fatalf("obsolete three-column table must be replaced:\n%s", merged)
	}
	if !strings.Contains(merged, "| Skill | Trigger / description | Scope | Path |") {
		t.Fatalf("four-column table missing:\n%s", merged)
	}
}

// TestAgentsOutsideMarkersSurviveByteIdentical covers REQ-22.12 «Ninguna otra
// sección se altera» (spec §3.6 preservation): everything before the open
// marker and after the close marker is byte-identical to the original
// document's outside-the-region bytes.
func TestAgentsOutsideMarkersSurviveByteIdentical(t *testing.T) {
	cwd := t.TempDir()
	entries := []SkillEntry{{Name: "go-testing", Path: filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), Description: "Go tests"}}
	doc := agentsFixture + "## Notas\nx\n"

	merged := applyAgentsUpdate(doc, cwd, entries)

	openIdx := strings.Index(merged, skillsIndexOpen)
	closeIdx := strings.Index(merged, skillsIndexClose)
	if openIdx < 0 || closeIdx < openIdx {
		t.Fatalf("canonical pair not found:\n%s", merged)
	}
	before := merged[:openIdx]
	after := merged[closeIdx+len(skillsIndexClose):]

	regionStart := strings.Index(doc, "## Skills\n\n| Skill | Trigger | Path |")
	if regionStart < 0 {
		t.Fatal("fixture shape changed: unmarked `## Skills` region not found")
	}
	origBefore := doc[:regionStart]
	// The newline that terminates the region's last line stays outside the
	// markers (see AdoptSkillsIndexMarkers), so the tail includes it.
	origAfter := doc[strings.Index(doc, "\n## Notas"):]

	if before != origBefore {
		t.Fatalf("bytes before the markers changed:\ngot  %q\nwant %q", before, origBefore)
	}
	if after != origAfter {
		t.Fatalf("bytes after the markers changed:\ngot  %q\nwant %q", after, origAfter)
	}
	for _, want := range []string{"## REGLA SUPREMA: IDIOMA OBLIGATORIO — ESPAÑOL (CASTELLANO)", "# Gentle AI™ — Agent Skills Index", "## How to Use"} {
		if !strings.Contains(merged, want) {
			t.Fatalf("sister section %q was altered:\n%s", want, merged)
		}
	}
}

// TestAgentsRegenerationIsIdempotent covers spec §3.6 idempotence: two runs
// over the same skill set produce a byte-identical AGENTS.md.
func TestAgentsRegenerationIsIdempotent(t *testing.T) {
	cwd := t.TempDir()
	entries := []SkillEntry{{Name: "go-testing", Path: filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), Description: "Go tests"}}

	first := applyAgentsUpdate(agentsFixture, cwd, entries)
	second := applyAgentsUpdate(first, cwd, entries)
	if first != second {
		t.Fatalf("second regeneration changed bytes:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestAgentsLegacyMarkersAreElevated covers REQ-22.12 «Marcadores legados se
// elevan al canónico»: adoption passes them through and InjectMarkdownSection
// upgrades them while refreshing the managed body.
func TestAgentsLegacyMarkersAreElevated(t *testing.T) {
	cwd := t.TempDir()
	entries := []SkillEntry{{Name: "go-testing", Path: filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), Description: "Go tests"}}
	legacy := "# Doc\n" + skillsIndexLegacyOpen + "\n## Skills\n\n| stale |\n" + skillsIndexLegacyClose + "\n## How to Use\nx\n"

	merged := applyAgentsUpdate(legacy, cwd, entries)

	if strings.Contains(merged, skillsIndexLegacyOpen) || strings.Contains(merged, skillsIndexLegacyClose) {
		t.Fatalf("legacy markers must be elevated:\n%s", merged)
	}
	if !strings.Contains(merged, skillsIndexOpen) || !strings.Contains(merged, skillsIndexClose) {
		t.Fatalf("canonical markers missing after elevation:\n%s", merged)
	}
	if strings.Contains(merged, "| stale |") {
		t.Fatalf("managed body must be refreshed:\n%s", merged)
	}
	if count := countSkillsHeadings(merged); count != 1 {
		t.Fatalf("`## Skills` heading count = %d, want 1:\n%s", count, merged)
	}
}

// TestAgentsRepairsOrphansAndDuplicates covers spec §3.6 repair: orphan
// markers and duplicated pairs are repaired in the same operation by the
// unmodified filemerge engine.
func TestAgentsRepairsOrphansAndDuplicates(t *testing.T) {
	cwd := t.TempDir()
	entries := []SkillEntry{{Name: "go-testing", Path: filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), Description: "Go tests"}}

	t.Run("orphan closer does not append a duplicate block", func(t *testing.T) {
		corrupt := "# Doc\n## Skills\n| stale |\n" + skillsIndexClose + "\n## How to Use\nx\n"
		merged := applyAgentsUpdate(corrupt, cwd, entries)
		if count := countSkillsHeadings(merged); count != 1 {
			t.Fatalf("`## Skills` heading count = %d, want 1:\n%s", count, merged)
		}
		if strings.Contains(merged, "| stale |") {
			t.Fatalf("stale body survived:\n%s", merged)
		}
	})

	t.Run("duplicated pairs collapse to one", func(t *testing.T) {
		corrupt := "# Doc\n" + skillsIndexOpen + "\n## Skills\n\n| a |\n" + skillsIndexClose + "\nmid\n" +
			skillsIndexOpen + "\n## Skills\n\n| b |\n" + skillsIndexClose + "\ntail\n"
		merged := applyAgentsUpdate(corrupt, cwd, entries)
		if strings.Count(merged, skillsIndexOpen) != 1 || strings.Count(merged, skillsIndexClose) != 1 {
			t.Fatalf("duplicated pairs not collapsed:\n%s", merged)
		}
		if !strings.Contains(merged, "mid") || !strings.Contains(merged, "tail") {
			t.Fatalf("content outside the surviving pair was lost:\n%s", merged)
		}
	})
}

// TestAgentsIndexOmitsMissingAgentsFile covers spec §3.6 no-creation: a
// missing AGENTS.md is an omitted destination and is never created.
func TestAgentsIndexOmitsMissingAgentsFile(t *testing.T) {
	cwd := t.TempDir()
	entries := []SkillEntry{{Name: "go-testing", Path: filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), Description: "Go tests"}}
	agentsPath := filepath.Join(cwd, AgentsRelPath)

	outcome, err := writeAgentsIndex(agentsPath, cwd, entries)
	if err != nil {
		t.Fatalf("missing AGENTS.md must not be fatal: %v", err)
	}
	if outcome.Status != DestOmitted {
		t.Fatalf("outcome.Status = %q, want %q", outcome.Status, DestOmitted)
	}
	if _, statErr := os.Stat(agentsPath); !os.IsNotExist(statErr) {
		t.Fatalf("AGENTS.md must not be created (stat err = %v)", statErr)
	}
}

// TestWriteAgentsIndexUsesAdoptionComposition proves the writer runs the
// adoption step before the unmodified merge engine (D-10 ordering).
func TestWriteAgentsIndexUsesAdoptionComposition(t *testing.T) {
	cwd := t.TempDir()
	entries := []SkillEntry{{Name: "go-testing", Path: filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), Description: "Go tests"}}
	agentsPath := filepath.Join(cwd, AgentsRelPath)
	if err := os.WriteFile(agentsPath, []byte(agentsFixture), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome, err := writeAgentsIndex(agentsPath, cwd, entries)
	if err != nil {
		t.Fatalf("writeAgentsIndex() error = %v", err)
	}
	if outcome.Status != DestUpdated {
		t.Fatalf("outcome.Status = %q, want %q", outcome.Status, DestUpdated)
	}
	written := readFile(t, agentsPath)
	if count := countSkillsHeadings(written); count != 1 {
		t.Fatalf("`## Skills` heading count = %d, want 1:\n%s", count, written)
	}
	if !strings.Contains(written, "skills/go-testing/SKILL.md") {
		t.Fatalf("O-2 relative Path missing from the managed table:\n%s", written)
	}
}
