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

var markerPattern = regexp.MustCompile(`<!-- gentle-ai:([A-Za-z0-9_.-]+) -->`)

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
		if !strings.HasPrefix(content, frontmatter) {
			t.Errorf("%s does not start with the always_on frontmatter", name)
		}
		id := regexp.MustCompile(`^gentle-ai-\d{2}-(.+?)(-part\d+)?\.md$`).FindStringSubmatch(name)
		if id == nil {
			t.Fatalf("unexpected rule file name %q", name)
		}
		if _, seen := bodies[id[1]]; !seen {
			order = append(order, id[1])
		}
		bodies[id[1]] += strings.TrimPrefix(content, frontmatter+"\n")
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
