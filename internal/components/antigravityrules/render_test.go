package antigravityrules_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/antigravityrules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

const frontmatter = "---\ntrigger: always_on\n---\n"

var (
	markerPattern = regexp.MustCompile(`<!-- gentle-ai:([A-Za-z0-9_.-]+) -->`)
	// demotedPattern is the on-demand frontmatter of a rule over the budget.
	demotedPattern = regexp.MustCompile(`\A---\ntrigger: model_decision\ndescription: "([^"\n]+)"\n---\n\n`)
)

// ruleBody strips either managed frontmatter and reports whether the rule is
// always on.
func ruleBody(t *testing.T, rule antigravityrules.Rule) (string, bool) {
	t.Helper()
	if body, ok := strings.CutPrefix(rule.Content, frontmatter+"\n"); ok {
		return body, true
	}
	match := demotedPattern.FindStringSubmatch(rule.Content)
	if match == nil {
		t.Fatalf("%s starts with neither managed frontmatter: %q", rule.Name, rule.Content[:min(len(rule.Content), 120)])
	}
	return rule.Content[len(match[0]):], false
}

// renderRealGuidance runs the production writers for Antigravity into a fake
// home, exactly as install does, and returns the staging file content.
func renderRealGuidance(t *testing.T, home string) string {
	t.Helper()
	adapter := antigravity.NewAdapter()
	engram.SetLookPathForTest(t, "/opt/homebrew/bin/engram", "")

	if _, err := persona.Inject(home, adapter, model.PersonaGentleman); err != nil {
		t.Fatalf("persona.Inject() error = %v", err)
	}
	if _, err := engram.Inject(home, adapter); err != nil {
		t.Fatalf("engram.Inject() error = %v", err)
	}
	if _, err := agentguidance.InjectRoutingWithOptions(home, model.AgentAntigravity, agentguidance.RoutingOptions{}); err != nil {
		t.Fatalf("InjectRoutingWithOptions() error = %v", err)
	}
	if _, err := agentguidance.InjectStrictTDDWithOptions(home, model.AgentAntigravity, false, agentguidance.RoutingOptions{}); err != nil {
		t.Fatalf("InjectStrictTDDWithOptions() error = %v", err)
	}
	staging := adapter.SystemPromptFile(home)
	withCodeGraph := filemerge.InjectMarkdownSection(readFile(t, staging), "codegraph-guidance", communitytool.CodeGraphGuidanceMarkdown())
	if err := os.WriteFile(staging, []byte(withCodeGraph), 0o644); err != nil {
		t.Fatal(err)
	}
	return withCodeGraph
}

// sectionBody returns the bytes between a gentle-ai marker pair, excluding the
// newline that follows the opening marker.
func sectionBody(t *testing.T, content, id string) string {
	t.Helper()
	open := "<!-- gentle-ai:" + id + " -->\n"
	start := strings.Index(content, open)
	end := strings.Index(content, "<!-- /gentle-ai:"+id+" -->")
	if start < 0 || end < start {
		t.Fatalf("section %q not found", id)
	}
	return content[start+len(open) : end]
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}

func TestRenderRealAntigravityGuidanceFitsAgyRuleLimit(t *testing.T) {
	home := t.TempDir()
	staging := renderRealGuidance(t, home)
	if len(staging) <= antigravityrules.MaxRuleFileBytes {
		t.Fatalf("rendered guidance is %d bytes; the test needs guidance larger than one rule file", len(staging))
	}

	bodies := map[string]string{}
	var order []string
	for _, rule := range antigravityrules.Render(staging) {
		name, content := rule.Name, rule.Content
		t.Logf("%s: %d bytes", name, len(content))
		if len(content) > antigravityrules.MaxRuleFileBytes {
			t.Errorf("%s is %d bytes, want <= %d", name, len(content), antigravityrules.MaxRuleFileBytes)
		}
		body, _ := ruleBody(t, rule)
		id := regexp.MustCompile(`^gentle-ai-\d{2}-(.+?)(-part\d+)?\.md$`).FindStringSubmatch(name)
		if id == nil {
			t.Fatalf("unexpected rule file name %q", name)
		}
		if _, seen := bodies[id[1]]; !seen {
			order = append(order, id[1])
		}
		bodies[id[1]] += body
	}

	// Nested sections (remote-authorization lives inside routing guidance)
	// travel inside their enclosing section's body.
	var wantOrder []string
	sectionEnd := 0
	for _, loc := range markerPattern.FindAllStringSubmatchIndex(staging, -1) {
		if loc[0] < sectionEnd {
			continue
		}
		id := staging[loc[2]:loc[3]]
		wantOrder = append(wantOrder, id)
		sectionEnd = strings.Index(staging, "<!-- /gentle-ai:"+id+" -->")
	}
	if strings.Join(order, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("rendered section order = %v, want %v", order, wantOrder)
	}
	for _, id := range []string{"persona", "engram-protocol", "agent-routing", "codegraph-guidance"} {
		if _, ok := bodies[id]; !ok {
			t.Errorf("section %q was not rendered; got %v", id, order)
		}
	}
	for id, body := range bodies {
		want := sectionBody(t, staging, id)
		if body != want || body == "" {
			t.Errorf("concatenated %q rule bodies differ from the staging section (%d vs %d bytes)", id, len(body), len(want))
		}
	}
}

func TestRenderSplitsOversizedSectionsAtHeadingBoundaries(t *testing.T) {
	block := func(heading string, n int) string {
		return heading + "\n" + strings.Repeat("line of guidance text that fills space\n", n)
	}
	headed := block("## One", 400) + block("### Two", 400) + "```\n## not a heading inside a fence\n```\n" + block("## Three", 400)
	headless := strings.Repeat("plain guidance without headings\n", 1600)

	tests := []struct {
		name         string
		body         string
		wantParts    int
		wantHeadings bool
	}{
		{name: "small section stays whole", body: "## Small\nshort\n", wantParts: 1},
		{name: "oversized section splits at headings", body: headed, wantParts: 3, wantHeadings: true},
		{name: "heading-free section falls back to lines", body: headless, wantParts: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			staging := filemerge.InjectMarkdownSection("", "agent-routing", tt.body)
			rules := antigravityrules.Render(staging)
			if len(rules) != tt.wantParts {
				t.Fatalf("Render() produced %d rules, want %d", len(rules), tt.wantParts)
			}
			var joined strings.Builder
			for i, rule := range rules {
				if len(rule.Content) > antigravityrules.MaxRuleFileBytes {
					t.Errorf("rule %s is %d bytes", rule.Name, len(rule.Content))
				}
				body := strings.TrimPrefix(rule.Content, frontmatter+"\n")
				if tt.wantHeadings && !strings.HasPrefix(body, "#") {
					t.Errorf("rule %d does not start at a heading: %q", i, body[:min(len(body), 30)])
				}
				if tt.wantParts > 1 && !strings.HasSuffix(rule.Name, "-part"+string(rune('1'+i))+".md") {
					t.Errorf("rule %d name = %q, want -part%d suffix", i, rule.Name, i+1)
				}
				joined.WriteString(body)
			}
			if joined.String() != tt.body {
				t.Fatalf("concatenated bodies differ from the section body")
			}
		})
	}
}

func TestRenderRealAntigravityGuidanceKeepsCoreInlineWithinBudget(t *testing.T) {
	rules := antigravityrules.Render(renderRealGuidance(t, t.TempDir()))
	core := map[string]bool{
		"gentle-ai-01-persona.md":             true,
		"gentle-ai-02-engram-protocol.md":     true,
		"gentle-ai-03-orchestrator-part1.md":  true,
		"gentle-ai-04-agent-routing-part1.md": true,
	}
	inline, demoted := 0, 0
	for _, rule := range rules {
		_, alwaysOn := ruleBody(t, rule)
		if core[rule.Name] && !alwaysOn {
			t.Errorf("core rule %s was demoted", rule.Name)
		}
		if alwaysOn {
			inline += len(rule.Content)
			delete(core, rule.Name)
			continue
		}
		demoted++
		t.Logf("%s demoted: %s", rule.Name, demotedPattern.FindStringSubmatch(rule.Content)[1])
	}
	if len(core) > 0 {
		t.Errorf("core rules not rendered: %v", core)
	}
	if inline > antigravityrules.AlwaysOnBudgetBytes {
		t.Errorf("always_on rules total %d bytes, want <= %d", inline, antigravityrules.AlwaysOnBudgetBytes)
	}
	if demoted == 0 {
		t.Fatalf("no rule was demoted; the test needs guidance larger than the budget")
	}
}

func TestRenderCoreKeepsOneCompleteExplorationRule(t *testing.T) {
	staging := renderRealGuidance(t, t.TempDir())
	mapping := "more than approximately 5 sequential lookups requires one read-only explorer; with its handoff, re-evaluate task size. Return at most approximately 2k tokens with path:line evidence and one parent spot check. Do not reread the entire mapped evidence."
	if count := strings.Count(staging, mapping); count != 1 {
		t.Errorf("complete exploration rule rendered %d times, want one canonical copy", count)
	}
	var inline strings.Builder
	inlineBytes := 0
	for _, rule := range antigravityrules.Render(staging) {
		if body, alwaysOn := ruleBody(t, rule); alwaysOn {
			inline.WriteString(body)
			inlineBytes += len(rule.Content)
		}
	}
	for _, retained := range []string{
		"Understanding that needs more evidence or " + mapping,
		"Inline evidence uses one parallel batch, at most 3 calls and approximately 10k tokens.",
		"Keep parent bash output bounded to counts, --stat, tail, or summaries.",
		"On a large task, delegate long suites and builds; return concise observed results, including failures.",
		"The approximately 150k parent-context backstop is advisory guidance, not mechanically observed or enforced; pause and delegate the next bounded unit without claiming runtime telemetry or enforcement.",
	} {
		if !strings.Contains(inline.String(), retained) {
			t.Errorf("required exploration clause missing from always-on rules: %q", retained)
		}
	}
	t.Logf("always-on bytes after exploration rendering: %d", inlineBytes)
}

func TestRenderKeepsSmallGuidanceAlwaysOn(t *testing.T) {
	staging := filemerge.InjectMarkdownSection("", "persona", "Be kind.\n")
	staging = filemerge.InjectMarkdownSection(staging, "agent-routing", "## Routing\nDelegate.\n")
	staging = filemerge.InjectMarkdownSection(staging, "codegraph-guidance", "## CodeGraph\nExplore.\n")
	rules := antigravityrules.Render(staging)
	if len(rules) != 3 {
		t.Fatalf("Render() produced %d rules, want 3", len(rules))
	}
	for _, rule := range rules {
		if _, alwaysOn := ruleBody(t, rule); !alwaysOn {
			t.Errorf("%s was demoted; small guidance must stay always_on", rule.Name)
		}
	}
}

func TestRenderDescribesDemotedRulesByFirstHeading(t *testing.T) {
	filler := strings.Repeat("guidance line that fills the always-on budget\n", 250)
	staging := ""
	for _, id := range []string{"persona", "engram-protocol", "orchestrator", "agent-routing", "strict-tdd"} {
		staging = filemerge.InjectMarkdownSection(staging, id, "## \"Quoted\" "+id+" rules\n"+filler)
	}
	var demoted []string
	for _, rule := range antigravityrules.Render(staging) {
		if _, alwaysOn := ruleBody(t, rule); !alwaysOn {
			description := demotedPattern.FindStringSubmatch(rule.Content)[1]
			if !strings.Contains(description, "Quoted strict-tdd rules") {
				t.Errorf("%s description = %q, want its first heading", rule.Name, description)
			}
			demoted = append(demoted, rule.Name)
		}
	}
	if strings.Join(demoted, ",") != "gentle-ai-05-strict-tdd.md" {
		t.Fatalf("demoted rules = %v, want only the lowest-priority section", demoted)
	}
}
