package assets

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are authored form/skill contracts, not runtime authorization tests.
func TestIssueTaxonomyForms(t *testing.T) {
	for _, form := range []struct {
		name, kind, ids, controlsHash string
		required                      int
	}{
		{"bug_report", "bug", "preflight description steps expected actual version os agent area logs context", "4aa09d0f80cf563290e049132e4f82021685e4470d287fcd883e4c5363d888f8", 10},
		{"feature_request", "feature", "preflight area problem solution alternatives context", "20a1357018c6013dcff2487f3f27dc7034af8273193df928eb601e6502587e37", 5},
	} {
		t.Run(form.name, func(t *testing.T) {
			text := taxonomyRead(t, ".github/ISSUE_TEMPLATE/"+form.name+".yml")
			taxonomyTerms(t, text, `labels: ["type:`+form.kind+`", "status:needs-review"]`, "current direct human instruction", "MAINTAIN", "ADMIN", "automatically rejected")
			for _, alias := range []string{`"bug"`, `"enhancement"`, `"documentation"`} {
				if strings.Contains(text, alias) {
					t.Errorf("form emits legacy label %s", alias)
				}
			}
			// Pin all baseline controls/options/requiredness, excluding markdown guidance.
			start := strings.Index(text, "  - type: checkboxes\n")
			if start < 0 || fmt.Sprintf("%x", sha256.Sum256([]byte(text[start:]))) != form.controlsHash {
				t.Error("baseline form controls changed")
			}
			last := -1
			for _, id := range strings.Fields(form.ids) {
				at := strings.Index(text, "    id: "+id+"\n")
				if at <= last {
					t.Errorf("missing or reordered control %s", id)
				}
				last = at
			}
			if got := strings.Count(text, "required: true"); got != form.required {
				t.Errorf("required controls changed: got %d want %d", got, form.required)
			}
			taxonomyTerms(t, text, "- CLI (commands, flags)\n        - TUI (terminal UI)\n        - Installation Pipeline\n        - Agent Detection\n        - System Detection\n        - Catalog/Steps\n        - Documentation\n        - Other", "I have searched [existing issues]", "I understand that PRs will be rejected")
			if form.kind == "bug" {
				taxonomyTerms(t, text, "render: shell", "- macOS\n        - Linux (Ubuntu/Debian)\n        - Linux (Arch/Manjaro)\n        - Linux (Fedora/RHEL)\n        - Linux (Other)\n        - Windows\n        - Windows (WSL)", "- Claude Code\n        - OpenCode\n        - Gemini CLI\n        - Cursor\n        - Windsurf\n        - Other")
			}
		})
	}
}

func TestIssueTaxonomyCatalog(t *testing.T) {
	text := taxonomyRead(t, "CONTRIBUTING.md")
	taxonomyTerms(t, text, "github.com/Gentleman-Programming/gentle-ai", "inventory is not permission", "untrusted data", "zero or one", "exactly one", "Preserve every existing", "human review", "separate label-creation authority", "Classification does not grant status or priority authority")
	for _, label := range []string{"type:bug", "type:feature", "type:docs", "type:refactor", "type:chore", "type:breaking-change", "status:needs-review", "status:approved", "status:needs-design", "status:needs-info", "priority:high", "priority:medium", "priority:low", "good first issue", "help wanted", "up-for-grabs", "size:exception", "no-merge", "duplicate", "wontfix", "gentle-report", "source:guided-report", "rc-feedback"} {
		taxonomyTerms(t, text, "`"+label+"`")
	}
	for _, stale := range []string{"status:in-progress", "status:blocked", "status:wont-fix", "priority:critical"} {
		if strings.Contains(text, "`"+stale+"`") {
			t.Errorf("catalog retains stale label %s", stale)
		}
	}
	taxonomyTerms(t, text, "input-only", "verified producer", "`invalid`", "`question`", "`slop`", "do not emit")
	taxonomyTerms(t, text, "`bug` → `type:bug`")
	taxonomyTerms(t, text, "`enhancement` → `type:feature`")
	taxonomyTerms(t, text, "`documentation` → `type:docs`")
}

func TestIssueTaxonomyShippedContract(t *testing.T) {
	canonical := MustRead("skills/issue-creation/SKILL.md")
	taxonomyTerms(t, canonical, `version: "1.6"`, "reviewed policy requires a catalog", "missing required catalog", "unknown label", "inventory is not permission", "untrusted data", "other repositories", "Classification does not grant status or priority authority", "runtime version/digest enforcement", "external control center")
	reference := MustRead("skills/issue-creation/references/delegated-workflow-actions.md")
	taxonomyTerms(t, reference, "approved catalog", "Preserve every existing", "multiple type labels", "human", "Before any generic `$LABEL`", "MAINTAIN", "ADMIN")
	for _, path := range []string{"skills/systemic-issue-triage/SKILL.md", "internal/assets/skills/systemic-issue-triage/SKILL.md", "skills/issue-root-resolution/SKILL.md", "skills/gentle-ai-collab-perfect/SKILL.md", "skills/branch-pr/SKILL.md", "internal/assets/skills/branch-pr/SKILL.md"} {
		t.Run(path, func(t *testing.T) {
			taxonomyTerms(t, taxonomyRead(t, path), "CONTRIBUTING.md", "internal/assets/skills/issue-creation/SKILL.md", "inventory is not permission")
		})
	}
	public := taxonomyRead(t, "skills/systemic-issue-triage/SKILL.md")
	if public != MustRead("skills/systemic-issue-triage/SKILL.md") {
		t.Error("public triage and shipped asset differ")
	}
	pr := taxonomyRead(t, ".github/PULL_REQUEST_TEMPLATE.md")
	taxonomyTerms(t, pr, "current direct human instruction", "MAINTAIN", "ADMIN", "read-back confirms exactly one")
	for _, kind := range []string{"bug", "feature", "docs", "refactor", "chore", "breaking-change"} {
		taxonomyTerms(t, pr, "- [ ] `type:"+kind+"`")
	}
	for _, section := range []string{"Linked Issue", "PR Type", "Summary", "Changes", "AI Assistance", "Test Plan", "Automated Checks", "Contributor Checklist", "Notes for Reviewers"} {
		taxonomyTerms(t, pr, section)
	}
	for _, obsolete := range []string{"maintainer-applied", "I have added the appropriate"} {
		if strings.Contains(pr, obsolete) {
			t.Errorf("PR template retains unsupported claim %q", obsolete)
		}
	}
}

// Pins authored decision examples and gates; this is not a label mutation engine.
func TestIssueTaxonomyHumanReclassification(t *testing.T) {
	canonical := MustRead("skills/issue-creation/SKILL.md")
	taxonomyTerms(t, canonical, "Automatic classification", "Human-authorized type correction", "Preserve every existing", "historical migration")
	reference := MustRead("skills/issue-creation/references/delegated-workflow-actions.md")
	taxonomyTerms(t, reference, "exact existing catalog names to remove and add", "fresh concrete target-host capability", "fresh complete pre-state", "unexpected types", "No wildcard or inferred all-type removal", "expected final type set", "every unrelated label", "no server-atomic guarantee", "unknown", "no retry")
	for _, row := range []string{
		"| One wrong type; automatic classification | DEFER; preserve it, do not add a second type |",
		"| One wrong type; no direct human instruction | DEFER; no mutation |",
		"| One wrong type; explicit named authorized replacement | ALLOW one bounded attempt; preserve unrelated labels |",
		"| Multiple types; automatic classification | DEFER; preserve all types |",
		"| Multiple types; complete human-named resolution | ALLOW one bounded attempt to the exact chosen final type set |",
		"| Multiple types; incomplete human choice | STOP; no mutation |",
		"| Unknown label, wrong target, unavailable capability, or unexpected pre-state types | STOP; no mutation |",
		"| Post-attempt readback unknown, unavailable, or mismatched | UNKNOWN; STOP, no retry or further mutation |",
	} {
		taxonomyTerms(t, reference, row)
	}
	for _, kind := range []string{"issue", "pr"} {
		taxonomyTerms(t, reference, "gh "+kind+" edit \"$NUMBER\" --repo \"$TARGET\" --add-label \"$ADD_TYPES\" --remove-label \"$REMOVE_TYPES\"")
	}
	for _, path := range []string{"CONTRIBUTING.md", "skills/systemic-issue-triage/SKILL.md", "internal/assets/skills/systemic-issue-triage/SKILL.md", "skills/issue-root-resolution/SKILL.md", "skills/gentle-ai-collab-perfect/SKILL.md", "skills/branch-pr/SKILL.md", "internal/assets/skills/branch-pr/SKILL.md"} {
		t.Run(path, func(t *testing.T) {
			taxonomyTerms(t, taxonomyRead(t, path), "automatic classification", "human-authorized type correction")
		})
	}
}

func taxonomyRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func taxonomyTerms(t *testing.T, text string, terms ...string) {
	t.Helper()
	for _, term := range terms {
		if !strings.Contains(text, term) {
			t.Errorf("missing authored contract %q", term)
		}
	}
}
