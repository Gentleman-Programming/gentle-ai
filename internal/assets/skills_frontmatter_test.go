package assets

import (
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// TestSkillFrontmatterIsLintClean walks every embedded SKILL.md asset and
// asserts the YAML frontmatter follows the structural rules defined by
// design.md decision 3 for the contextual-skill-loading change:
//
//  1. Frontmatter is delimited by a leading `---` and a closing `---`.
//  2. `name:` value equals the parent directory basename.
//  3. `description:` raw value is a quoted scalar on the same line, not `>`
//     or `|` (rejects YAML-unsafe plain scalars and block scalars).
//  4. The parsed `description` value is a single line (no embedded newline)
//     with <=160 chars and contains the substring `Trigger:` (skipped only
//     for `_shared`, which is documented as not a real invokable skill).
//  5. No top-level frontmatter keys outside the whitelist
//     {name, description, license, metadata, version}. This catches
//     non-standard fields like `allowed-tools:`.
//
// The test deliberately uses a tiny manual parser instead of pulling in a
// YAML dependency — the rules above only require line-level inspection.
func TestSkillFrontmatterIsLintClean(t *testing.T) {
	skillPaths := embeddedSkillPaths(t)

	allowedKeys := map[string]bool{
		"name":                     true,
		"description":              true,
		"license":                  true,
		"metadata":                 true,
		"version":                  true,
		"user-invocable":           true,
		"disable-model-invocation": true,
	}

	for _, path := range skillPaths {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)

			fm, err := extractSkillFrontmatter(content)
			if err != nil {
				t.Fatalf("extract frontmatter: %v", err)
			}

			// Rule 2: name == parent directory basename.
			expectedName := skillDirBasename(path)
			if fm.name != expectedName {
				t.Errorf("name = %q, want %q (must match directory basename)", fm.name, expectedName)
			}

			// Rule 3: description raw line is a quoted YAML-safe scalar (no `>` or `|`).
			if strings.HasPrefix(fm.descriptionRawAfterColon, ">") || strings.HasPrefix(fm.descriptionRawAfterColon, "|") {
				t.Errorf("description uses block scalar (`%s`); must be a quoted single-line scalar", string(fm.descriptionRawAfterColon[0]))
			}
			if !isQuotedScalar(fm.descriptionRawAfterColon) {
				t.Errorf("description must be quoted to stay YAML-safe; got raw value: %q", fm.descriptionRawAfterColon)
			}

			// Rule 4: parsed description is single-line, within budget, and contains Trigger.
			if strings.Contains(fm.description, "\n") {
				t.Errorf("description spans multiple lines; must be a single line. got: %q", fm.description)
			}
			if got := len([]rune(fm.description)); got > 160 {
				t.Errorf("description length = %d chars, want <=160 for Claude Code budget: %q", got, fm.description)
			}
			if path != "skills/_shared/SKILL.md" {
				if !strings.Contains(fm.description, "Trigger:") {
					t.Errorf("description must contain `Trigger:` substring; got: %q", fm.description)
				}
			}

			// Rule 5: only allowed top-level keys.
			for _, key := range fm.topLevelKeys {
				if !allowedKeys[key] {
					t.Errorf("non-standard top-level frontmatter key %q (allowed: name, description, license, metadata, version, user-invocable, disable-model-invocation)", key)
				}
			}
		})
	}
}

func embeddedSkillPaths(t *testing.T) []string {
	t.Helper()

	var paths []string
	if err := fs.WalkDir(FS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "/SKILL.md") {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		t.Fatalf("walk embedded skills: %v", err)
	}

	if len(paths) == 0 {
		t.Fatal("no embedded SKILL.md files found")
	}
	sort.Strings(paths)
	return paths
}

type skillFrontmatter struct {
	name                     string
	description              string // logical, single-line representation
	descriptionRawAfterColon string // first non-space token after `description:` on the raw line
	topLevelKeys             []string
}

// skillDirBasename returns the directory name immediately containing the
// SKILL.md file (e.g. "skills/foo/SKILL.md" → "foo").
func skillDirBasename(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

// extractSkillFrontmatter parses the leading `---` ... `---` block of a
// SKILL.md file and returns the rules-relevant fields. It intentionally
// supports only the simple key forms used by gentle-ai's SKILL.md files:
//
//   - `key: value`                  — plain scalar on the same line
//   - `key: > / key: |` + indented continuation lines (block scalars)
//   - `key:` with indented mapping  — treated as a parent map (e.g. metadata)
//
// Anything outside that envelope returns a clear error so the test fails loudly.
func extractSkillFrontmatter(content string) (skillFrontmatter, error) {
	var fm skillFrontmatter

	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return fm, errFrontmatter("file does not start with `---`")
	}

	// Strip leading `---\n` then split out the frontmatter block.
	rest := strings.TrimPrefix(content, "---\n")
	rest = strings.TrimPrefix(rest, "---\r\n")
	closeIdx := strings.Index(rest, "\n---")
	if closeIdx == -1 {
		return fm, errFrontmatter("missing closing `---`")
	}
	block := rest[:closeIdx]

	lines := strings.Split(block, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		// Top-level keys live at column 0 — anything indented is a continuation/child.
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}

		colon := strings.Index(line, ":")
		if colon == -1 {
			return fm, errFrontmatter("malformed line (no colon): " + line)
		}
		key := line[:colon]
		valueRaw := strings.TrimSpace(line[colon+1:])
		fm.topLevelKeys = append(fm.topLevelKeys, key)

		switch key {
		case "name":
			fm.name = unquote(valueRaw)
		case "description":
			fm.descriptionRawAfterColon = valueRaw
			if strings.HasPrefix(valueRaw, ">") || strings.HasPrefix(valueRaw, "|") {
				// Block scalar — concatenate following indented lines.
				var parts []string
				for j := i + 1; j < len(lines); j++ {
					next := lines[j]
					if next == "" || next[0] == ' ' || next[0] == '\t' {
						parts = append(parts, strings.TrimSpace(next))
						continue
					}
					break
				}
				fm.description = strings.TrimSpace(strings.Join(parts, " "))
				// Folded scalars yield a single logical line; we still report
				// it as multi-line via descriptionRawAfterColon for rule 3.
				// For rule 4 we keep the joined form so it satisfies the
				// "no embedded newline" check — rule 3 already failed by then.
			} else {
				fm.description = unquote(valueRaw)
			}
		}
	}

	if fm.name == "" {
		return fm, errFrontmatter("missing required `name:` field")
	}
	if fm.descriptionRawAfterColon == "" && fm.description == "" {
		return fm, errFrontmatter("missing required `description:` field")
	}
	return fm, nil
}

// unquote strips surrounding "..." or '...' if present.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func isQuotedScalar(s string) bool {
	if len(s) < 2 {
		return false
	}
	quote := s[0]
	if quote != '\'' && quote != '"' {
		return false
	}
	return s[len(s)-1] == quote
}

type frontmatterError struct{ msg string }

func (e *frontmatterError) Error() string { return e.msg }

func errFrontmatter(msg string) error { return &frontmatterError{msg: msg} }
