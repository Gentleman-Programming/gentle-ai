package assets

import (
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// TestClaudeAgentFrontmatterIsLintClean walks every embedded
// internal/assets/claude/agents/*.md file and asserts the YAML frontmatter
// follows the structural rules defined by design.md decision D6 for the
// dev-agents-p0-remediation change:
//
//  1. (A1) Frontmatter is delimited by a leading `---` and a closing `---`.
//  2. (A2) `name:` value equals the file basename minus `.md` (NOT the
//     parent directory basename — these are flat files, not skill dirs).
//  3. (A3) `description:` is required; both `>` block scalars and unquoted
//     plain scalars are allowed. The folded/parsed value must be a single
//     logical line (no embedded newline) and <=500 runes. There is
//     deliberately no `Trigger:` substring requirement — that convention is
//     specific to skills/SKILL.md, not Claude agent prompts.
//  4. (A4) No top-level frontmatter keys outside the whitelist
//     {name, description, model, tools}.
//  5. (A5) A colon-less top-level line is tolerated ONLY when it is exactly
//     a `{{...}}` placeholder (e.g. the standalone `{{CLAUDE_EFFORT_FRONTMATTER}}`
//     line these files use); any other colon-less line is still a loud error.
//  6. (A6) Placeholder tokens (whether standalone or as a key's value) must
//     belong to the known set {{CLAUDE_MODEL}}, {{CLAUDE_EFFORT_FRONTMATTER}}
//     — this catches typo'd templates.
//
// This is a sibling test to TestSkillFrontmatterIsLintClean, not a widened
// version of it: reusing skillFrontmatter/extractSkillFrontmatter would not
// compile with distinct semantics (rule 2 differs, the whitelist differs,
// and the placeholder tolerance does not exist over there), so this file
// declares its own agentFrontmatter/extractAgentFrontmatter types.
func TestClaudeAgentFrontmatterIsLintClean(t *testing.T) {
	agentPaths := embeddedClaudeAgentPaths(t)

	if len(agentPaths) != 30 {
		t.Fatalf("expected 30 embedded claude agent files, got %d: %v", len(agentPaths), agentPaths)
	}

	for _, path := range agentPaths {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)

			fm, err := extractAgentFrontmatter(content)
			if err != nil {
				t.Fatalf("extract frontmatter: %v", err)
			}

			// Rule A2: name == file basename minus ".md".
			expectedName := agentFileBasename(path)
			if fm.name != expectedName {
				t.Errorf("name = %q, want %q (must match file basename minus .md)", fm.name, expectedName)
			}

			// Rule A3: description required, single logical line, budget.
			if fm.description == "" {
				t.Fatalf("missing required description")
			}
			if strings.Contains(fm.description, "\n") {
				t.Errorf("description spans multiple lines; must be a single line. got: %q", fm.description)
			}
			if got := len([]rune(fm.description)); got > 500 {
				t.Errorf("description length = %d chars, want <=500: %q", got, fm.description)
			}

			// Rules A4/A6 are enforced inside extractAgentFrontmatter itself
			// (unknown key / unknown placeholder both fail parsing), so a
			// successful parse above already guarantees them. allowedKeys is
			// still referenced here as a documentation cross-check.
			for _, key := range fm.topLevelKeys {
				if !agentFrontmatterAllowedKeys[key] {
					t.Errorf("non-standard top-level frontmatter key %q slipped past extractAgentFrontmatter (allowed: name, description, model, tools)", key)
				}
			}
		})
	}
}

// TestExtractAgentFrontmatterRejectsMalformedInput is a table-driven unit
// test on extractAgentFrontmatter covering the deliberate-malformation
// scenario from the spec, without mutating any embedded asset.
func TestExtractAgentFrontmatterRejectsMalformedInput(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name: "missing closing ---",
			content: "---\n" +
				"name: foo\n" +
				"description: bar\n",
		},
		{
			name: "missing name",
			content: "---\n" +
				"description: bar\n" +
				"---\n" +
				"body\n",
		},
		{
			name: "unknown top-level key",
			content: "---\n" +
				"name: foo\n" +
				"description: bar\n" +
				"allowed-tools: baz\n" +
				"---\n" +
				"body\n",
		},
		{
			name: "unknown placeholder",
			content: "---\n" +
				"name: foo\n" +
				"description: bar\n" +
				"model: {{CLAUDE_MYSTERY_TOKEN}}\n" +
				"---\n" +
				"body\n",
		},
		{
			name: "stray colon-less line that is not a placeholder",
			content: "---\n" +
				"name: foo\n" +
				"description: bar\n" +
				"this is not a placeholder\n" +
				"---\n" +
				"body\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm, err := extractAgentFrontmatter(tc.content)
			if err == nil {
				t.Fatalf("expected error for malformed input, got none (parsed: %+v)", fm)
			}
		})
	}
}

func embeddedClaudeAgentPaths(t *testing.T) []string {
	t.Helper()

	var paths []string
	if err := fs.WalkDir(FS, "claude/agents", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		t.Fatalf("walk embedded claude agents: %v", err)
	}

	if len(paths) == 0 {
		t.Fatal("no embedded claude/agents/*.md files found")
	}
	sort.Strings(paths)
	return paths
}

var knownAgentPlaceholders = map[string]bool{
	"{{CLAUDE_MODEL}}":              true,
	"{{CLAUDE_EFFORT_FRONTMATTER}}": true,
}

var agentFrontmatterAllowedKeys = map[string]bool{
	"name":        true,
	"description": true,
	"model":       true,
	"tools":       true,
}

type agentFrontmatter struct {
	name         string
	description  string // logical, single-line representation
	topLevelKeys []string
	placeholders []string // every {{...}} token seen, standalone or as a value
}

// agentFileBasename returns the file name minus its ".md" extension (e.g.
// "claude/agents/foo.md" -> "foo"). Unlike skillDirBasename this is NOT the
// parent directory name — claude/agents/*.md are flat files, not one dir per
// agent, so the "name == parent dir basename" rule does not apply here.
func agentFileBasename(path string) string {
	base := path
	if idx := strings.LastIndex(base, "/"); idx != -1 {
		base = base[idx+1:]
	}
	return strings.TrimSuffix(base, ".md")
}

// isPlaceholderLine reports whether the trimmed line is exactly a `{{...}}`
// token and nothing else.
func isPlaceholderLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") && len(trimmed) > 4 {
		return trimmed, true
	}
	return "", false
}

// extractAgentFrontmatter parses the leading `---` ... `---` block of a
// claude/agents/*.md file and returns the rules-relevant fields (A1-A6). It
// is a distinct sibling to extractSkillFrontmatter: agent files are flat
// (no per-agent directory), allow model/tools keys, and tolerate standalone
// `{{...}}` placeholder lines that break the generic "every top-level line
// has a colon" assumption used for SKILL.md.
func extractAgentFrontmatter(content string) (agentFrontmatter, error) {
	var fm agentFrontmatter

	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return fm, errFrontmatter("file does not start with `---`")
	}

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
		if line == "" {
			continue
		}
		// Indented lines are continuations of a block scalar handled below;
		// skip them here (they get consumed when we hit the owning key).
		if line[0] == ' ' || line[0] == '\t' {
			continue
		}

		colon := strings.Index(line, ":")
		if colon == -1 {
			// Rule A5: tolerate a colon-less line only if it is exactly a
			// standalone `{{...}}` placeholder.
			ph, ok := isPlaceholderLine(line)
			if !ok {
				return fm, errFrontmatter("malformed line (no colon, not a placeholder): " + line)
			}
			// Rule A6: the placeholder must be from the known set.
			if !knownAgentPlaceholders[ph] {
				return fm, errFrontmatter("unknown placeholder: " + ph)
			}
			fm.placeholders = append(fm.placeholders, ph)
			continue
		}
		key := line[:colon]
		valueRaw := strings.TrimSpace(line[colon+1:])

		// Rule A4: only allowed top-level keys.
		if !agentFrontmatterAllowedKeys[key] {
			return fm, errFrontmatter("non-standard top-level frontmatter key: " + key)
		}
		fm.topLevelKeys = append(fm.topLevelKeys, key)

		// Rule A6 (partial): collect placeholder tokens used as values too
		// (e.g. `model: {{CLAUDE_MODEL}}`), and reject unknown ones.
		if ph, ok := isPlaceholderLine(valueRaw); ok {
			if !knownAgentPlaceholders[ph] {
				return fm, errFrontmatter("unknown placeholder: " + ph)
			}
			fm.placeholders = append(fm.placeholders, ph)
		}

		switch key {
		case "name":
			fm.name = unquote(valueRaw)
		case "description":
			if strings.HasPrefix(valueRaw, ">") || strings.HasPrefix(valueRaw, "|") {
				var parts []string
				for j := i + 1; j < len(lines); j++ {
					next := lines[j]
					if next == "" || next[0] == ' ' || next[0] == '\t' {
						if strings.TrimSpace(next) != "" {
							parts = append(parts, strings.TrimSpace(next))
						}
						continue
					}
					break
				}
				fm.description = strings.TrimSpace(strings.Join(parts, " "))
			} else {
				fm.description = unquote(valueRaw)
			}
		}
	}

	if fm.name == "" {
		return fm, errFrontmatter("missing required `name:` field")
	}
	if fm.description == "" {
		return fm, errFrontmatter("missing required `description:` field")
	}
	return fm, nil
}
