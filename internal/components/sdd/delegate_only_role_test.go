package sdd

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"gopkg.in/yaml.v3"
)

// delegateOnlyFrontmatter captures only the metadata field this ratchet
// cares about. Deriving the skill list from this frontmatter (rather than a
// hardcoded name list) means a new delegate_only skill is covered
// automatically, with no test update required.
type delegateOnlyFrontmatter struct {
	Metadata struct {
		DelegateOnly bool `yaml:"delegate_only"`
	} `yaml:"metadata"`
}

var (
	// roleHeadingRE matches a markdown heading (any level) that declares a
	// role split, e.g. "## Execution Role" or the retired "## Executor
	// Override". It intentionally matches on the concept (role/override/
	// orchestrator/executor), not a literal fixed heading string, so a
	// future rewording of the compliant heading still counts as one role
	// block instead of silently falling to zero. "gate" is deliberately
	// excluded here: these skill files also use unrelated headings such as
	// "Hard Gate" or "Task Completion Gate" for other review/delivery
	// concepts, and including it produces false positives unrelated to the
	// executor/orchestrator role split this ratchet targets.
	roleHeadingRE = regexp.MustCompile(`(?im)^#{1,6}[ \t]+.*\b(role|override|orchestrator|executor)\b`)

	// headingLineRE matches any markdown heading line, used to find where a
	// role-declaring heading's own block ends (the next heading, of any
	// level, or end of body).
	headingLineRE = regexp.MustCompile(`(?m)^#{1,6}[ \t]+.*$`)

	// retractionPhraseRE matches wording that undoes an earlier imperative
	// for one of the two readers instead of gating it with a leading
	// condition. These are the exact phrases the #721 gate+override pair
	// used to retract the orchestrator gate for the executor reader.
	retractionPhraseRE = regexp.MustCompile(`(?i)(does\s+not\s+apply\s+to\s+you|the\s+gate\s+above|the\s+gate\s+below|gate\s+above|gate\s+below)`)
)

// extractNamedSection returns the text found between the HTML comment
// markers "<!-- section:NAME -->" and "<!-- /section:NAME -->" in content.
// It reports false if the opening marker is absent, so callers can fall back
// to treating the whole file as a single (unsectioned) body.
func extractNamedSection(content, name string) (string, bool) {
	open := "<!-- section:" + name + " -->"
	close := "<!-- /section:" + name + " -->"

	start := strings.Index(content, open)
	if start == -1 {
		return "", false
	}
	start += len(open)

	if end := strings.Index(content[start:], close); end != -1 {
		return content[start : start+end], true
	}
	return content[start:], true
}

// splitFrontmatter parses the leading "---\n...\n---" YAML block out of a
// section's raw text and returns the decoded frontmatter alongside the
// remaining body (everything after the closing delimiter).
func splitFrontmatter(t *testing.T, skillName, section string) (delegateOnlyFrontmatter, string) {
	t.Helper()

	trimmed := strings.TrimLeft(section, "\n")
	const openDelim = "---\n"
	if !strings.HasPrefix(trimmed, openDelim) {
		t.Fatalf("%s: section does not open with YAML frontmatter", skillName)
	}

	rest := trimmed[len(openDelim):]
	end := strings.Index(rest, "\n---")
	if end == -1 {
		t.Fatalf("%s: could not find closing frontmatter delimiter", skillName)
	}

	var fm delegateOnlyFrontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		t.Fatalf("%s: invalid frontmatter YAML: %v", skillName, err)
	}

	body := rest[end+len("\n---"):]
	return fm, body
}

// TestDelegateOnlySkillsStateRoleWithoutRetraction is the #721 ratchet.
//
// Issue #721: the model-capable section of every delegate_only skill opened
// with an unconditional "ORCHESTRATOR GATE" imperative immediately followed
// by an "Executor Override" that retracted it for the executor reader.
// Autoregressive readers commit to the first imperative before reaching its
// own retraction, so the intended delegate_only reader (the dedicated
// sub-agent) regularly self-identified as the orchestrator and refused to do
// the phase work. Simply reordering the two blocks (PR #1105's fix) does not
// remove the contradiction, it only moves it to the other reader.
//
// This test asserts the property the fix must hold, not the literal
// replacement wording, so a future rewrite of the compliant text still
// passes as long as it keeps the property, and a future regression that
// reintroduces the old pattern under different wording still fails:
//
//  1. Exactly one role-declaring block in the model-capable section.
//  2. That block names both the executor (sub-agent) and orchestrator
//     roles, so neither reader is left unaddressed.
//  3. No retraction phrase that undoes an earlier imperative for either
//     reader.
//
// The skill list is derived from each asset's own frontmatter
// (metadata.delegate_only), never hardcoded, so a new delegate_only skill is
// covered automatically without a test change.
func TestDelegateOnlySkillsStateRoleWithoutRetraction(t *testing.T) {
	entries, err := assets.FS.ReadDir("skills")
	if err != nil {
		t.Fatalf("ReadDir(skills) error = %v", err)
	}

	checked := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()

		data, err := assets.FS.ReadFile("skills/" + skillName + "/SKILL.md")
		if err != nil {
			// Not every skill directory ships a SKILL.md (e.g. _shared
			// holds only fragment includes); nothing to check here.
			continue
		}
		content := string(data)

		// Only the model-capable section carries the paired role
		// declaration; model-small variants intentionally keep a single-
		// sentence gate with no paired override and are out of scope.
		section, sectioned := extractNamedSection(content, "model-capable")
		if !sectioned {
			section = content
		}

		fm, body := splitFrontmatter(t, skillName, section)
		if !fm.Metadata.DelegateOnly {
			continue
		}
		checked++

		// Property 1: exactly one role-declaring heading in the whole
		// section. This is checked over the full body because a regression
		// could reintroduce a second role heading anywhere in the file, not
		// just adjacent to the first one.
		roleMatches := roleHeadingRE.FindAllStringIndex(body, -1)
		if len(roleMatches) != 1 {
			found := roleHeadingRE.FindAllString(body, -1)
			t.Errorf("%s: model-capable section has %d role-declaring blocks, want exactly 1 (found: %q)", skillName, len(roleMatches), found)
			continue
		}

		// Bound the role block to the text between the role heading and the
		// next heading of any level (or end of body), so properties 2 and 3
		// are checked against the role declaration itself, not unrelated
		// later content that happens to share a keyword (these skills also
		// define other, unrelated "gates" such as a Task Completion Gate).
		roleStart := roleMatches[0][0]
		blockEnd := len(body)
		for _, hs := range headingLineRE.FindAllStringIndex(body, -1) {
			if hs[0] > roleStart {
				blockEnd = hs[0]
				break
			}
		}
		roleBlock := body[roleStart:blockEnd]
		lowerBlock := strings.ToLower(roleBlock)

		// Property 2: the block must name both roles, so neither reader is
		// left unaddressed by the single block.
		if !strings.Contains(lowerBlock, "orchestrator") {
			t.Errorf("%s: role block never names the orchestrator role", skillName)
		}
		if !strings.Contains(lowerBlock, "executor") && !strings.Contains(lowerBlock, "sub-agent") {
			t.Errorf("%s: role block never names the executor (sub-agent) role", skillName)
		}

		// Property 3: no retraction phrase undoing an earlier imperative.
		if m := retractionPhraseRE.FindString(roleBlock); m != "" {
			t.Errorf("%s: role block contains retraction phrase %q, which undoes an earlier imperative instead of gating it with a leading condition", skillName, m)
		}
	}

	if checked == 0 {
		t.Fatal("no delegate_only skill assets were found; this ratchet is not exercising anything")
	}
}
