package researchcapability

import (
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestAdmitFailsClosedAndReturnsOnlyVerifiedGrants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		request     Request
		wantAllowed bool
		wantGrants  []Grant
	}{
		{
			name: "Claude documentation",
			request: Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode,
				Class: ClassDocumentation, ObservedGrants: []Grant{GrantWebFetch}},
			wantAllowed: true,
			wantGrants:  []Grant{GrantWebFetch},
		},
		{
			name: "Claude open web",
			request: Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode,
				Class: ClassOpenWeb, ObservedGrants: []Grant{GrantWebSearch, GrantWebFetch}},
			wantAllowed: true,
			wantGrants:  []Grant{GrantWebSearch, GrantWebFetch},
		},
		{
			name: "Kiro documentation",
			request: Request{Schema: SchemaV1, AgentID: model.AgentKiroIDE,
				Class: ClassDocumentation, ObservedGrants: []Grant{GrantContext7}},
			wantAllowed: true,
			wantGrants:  []Grant{GrantContext7},
		},
		{name: "missing schema", request: Request{AgentID: model.AgentClaudeCode, Class: ClassDocumentation, ObservedGrants: []Grant{GrantWebFetch}}},
		{name: "unknown schema", request: Request{Schema: "gentle-ai.sdd-research-capability/v2", AgentID: model.AgentClaudeCode, Class: ClassDocumentation, ObservedGrants: []Grant{GrantWebFetch}}},
		{name: "missing class", request: Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, ObservedGrants: []Grant{GrantWebFetch}}},
		{name: "unknown class", request: Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, Class: "repository", ObservedGrants: []Grant{GrantWebFetch}}},
		{name: "unknown agent", request: Request{Schema: SchemaV1, AgentID: "future-agent", Class: ClassDocumentation, ObservedGrants: []Grant{GrantWebFetch}}},
		{name: "unsupported known agent", request: Request{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassDocumentation, ObservedGrants: []Grant{GrantWebFetch}}},
		{name: "Bash is not evidence", request: Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, Class: ClassDocumentation, ObservedGrants: []Grant{"Bash"}}},
		{name: "generic MCP is not evidence", request: Request{Schema: SchemaV1, AgentID: model.AgentKiroIDE, Class: ClassDocumentation, ObservedGrants: []Grant{"mcp"}}},
		{name: "unnamed inheritance is not evidence", request: Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, Class: ClassDocumentation, ObservedGrants: []Grant{"inherited"}}},
		{name: "extra grant breaks exact match", request: Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, Class: ClassDocumentation, ObservedGrants: []Grant{GrantWebFetch, "Bash"}}},
		{name: "Kiro has no open-web grant", request: Request{Schema: SchemaV1, AgentID: model.AgentKiroIDE, Class: ClassOpenWeb, ObservedGrants: []Grant{GrantContext7}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := Admit(test.request)
			if got.Allowed != test.wantAllowed {
				t.Fatalf("Admit() allowed = %v, want %v", got.Allowed, test.wantAllowed)
			}
			if !reflect.DeepEqual(got.VerifiedGrants, test.wantGrants) {
				t.Fatalf("Admit() verified grants = %v, want %v", got.VerifiedGrants, test.wantGrants)
			}
			if len(got.Claims) != 0 {
				t.Fatalf("Admit() claims = %v, want none; admission cannot create source claims", got.Claims)
			}
		})
	}
}

func TestDocumentationLikeInputsNeverBecomeGrants(t *testing.T) {
	t.Parallel()

	for _, input := range []Grant{
		"requirements.txt",
		"CMakeLists.txt",
		"guide.md --execute",
		"component.mdx --run",
		"README.sh",
	} {
		input := input
		t.Run(string(input), func(t *testing.T) {
			t.Parallel()
			got := Admit(Request{
				Schema: SchemaV1, AgentID: model.AgentClaudeCode,
				Class: ClassDocumentation, ObservedGrants: []Grant{input},
			})
			if got.Allowed || len(got.VerifiedGrants) != 0 || len(got.Claims) != 0 {
				t.Fatalf("Admit(%q) = %#v, want closed denial without grants or claims", input, got)
			}
		})
	}
}

func TestForAgentDeclaresOnlyClaudeAndKiro(t *testing.T) {
	t.Parallel()

	want := map[model.AgentID]map[Class][]Grant{
		model.AgentClaudeCode: {
			ClassDocumentation: {GrantWebFetch},
			ClassOpenWeb:       {GrantWebSearch, GrantWebFetch},
		},
		model.AgentKiroIDE: {ClassDocumentation: {GrantContext7}},
	}
	for _, agent := range []model.AgentID{
		model.AgentClaudeCode, model.AgentOpenCode, model.AgentKilocode, model.AgentGeminiCLI,
		model.AgentCursor, model.AgentVSCodeCopilot, model.AgentCodex, model.AgentAntigravity,
		model.AgentWindsurf, model.AgentKimi, model.AgentQwenCode, model.AgentKiroIDE,
		model.AgentOpenClaw, model.AgentPi, model.AgentTrae, model.AgentHermes,
	} {
		got, ok := ForAgent(agent)
		if expected, supported := want[agent]; supported {
			if !ok || got.Schema != SchemaV1 || !reflect.DeepEqual(got.Grants, expected) {
				t.Errorf("ForAgent(%q) = %#v, %v; want schema v1 grants %v", agent, got, ok, expected)
			}
		} else if ok {
			t.Errorf("ForAgent(%q) unexpectedly declared capability %#v", agent, got)
		}
	}
}
