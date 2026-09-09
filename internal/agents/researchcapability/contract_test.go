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

func TestPiAdmissionRequiresExactGrants(t *testing.T) {
	t.Parallel()

	openWeb := []Grant{"web_search", "source_check", "fetch_content", "get_search_content"}
	type admissionCase struct {
		name   string
		schema SchemaVersion
		class  Class
		grants []Grant
		want   []Grant
	}
	tests := []admissionCase{
		{"documentation", SchemaV1, ClassDocumentation, []Grant{"fetch_content"}, []Grant{"fetch_content"}},
		{"open web", SchemaV1, ClassOpenWeb, openWeb, openWeb},
		{"reordered open web", SchemaV1, ClassOpenWeb, []Grant{"get_search_content", "fetch_content", "source_check", "web_search"}, openWeb},
		{"missing documentation tool", SchemaV1, ClassDocumentation, nil, nil},
		{"renamed documentation tool", SchemaV1, ClassDocumentation, []Grant{"Fetch_Content"}, nil},
		{"duplicate documentation tool", SchemaV1, ClassDocumentation, []Grant{"fetch_content", "fetch_content"}, nil},
		{"unexpected documentation grant", SchemaV1, ClassDocumentation, []Grant{"fetch_content", "web_search"}, nil},
		{"missing schema", "", ClassOpenWeb, openWeb, nil},
		{"invalid schema", "gentle-ai.sdd-research-capability/v2", ClassOpenWeb, openWeb, nil},
		{"missing class", SchemaV1, "", openWeb, nil},
		{"invalid class", SchemaV1, "repository", openWeb, nil},
		{"Bash is not evidence", SchemaV1, ClassDocumentation, []Grant{"Bash"}, nil},
		{"generic MCP is not evidence", SchemaV1, ClassDocumentation, []Grant{"mcp"}, nil},
		{"inheritance is not evidence", SchemaV1, ClassDocumentation, []Grant{"inherited"}, nil},
	}
	for i, grant := range openWeb {
		missing := append([]Grant(nil), openWeb[:i]...)
		missing = append(missing, openWeb[i+1:]...)
		renamed := append([]Grant(nil), openWeb...)
		renamed[i] = grant + "_renamed"
		duplicate := append([]Grant(nil), openWeb...)
		duplicate[i] = openWeb[(i+1)%len(openWeb)]
		for _, variant := range []struct {
			name   string
			grants []Grant
		}{
			{"missing " + string(grant), missing},
			{"renamed " + string(grant), renamed},
			{"duplicate replacing " + string(grant), duplicate},
			{"extra " + string(grant), append(append([]Grant(nil), openWeb...), grant)},
		} {
			tests = append(tests, admissionCase{variant.name, SchemaV1, ClassOpenWeb, variant.grants, nil})
		}
	}
	for _, extra := range []Grant{"Bash", "mcp", "unexpected"} {
		tests = append(tests, admissionCase{"unexpected " + string(extra), SchemaV1, ClassOpenWeb, append(append([]Grant(nil), openWeb...), extra), nil})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := Admit(Request{Schema: test.schema, AgentID: model.AgentPi, Class: test.class, ObservedGrants: test.grants})
			if got.Allowed != (test.want != nil) || !reflect.DeepEqual(got.VerifiedGrants, test.want) || len(got.Claims) != 0 {
				t.Fatalf("Admit() = %#v, want grants %v and no claims", got, test.want)
			}
		})
	}
}

func TestCapabilitiesAndAdmissionReturnDefensiveCopies(t *testing.T) {
	t.Parallel()

	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentKiroIDE, model.AgentPi} {
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()
			original, ok := ForAgent(agent)
			if !ok {
				t.Fatal("expected declared capability")
			}
			for class, grants := range original.Grants {
				copyOf, _ := ForAgent(agent)
				copyOf.Grants[class][0] = "mutated"
				delete(copyOf.Grants, class)
				observed := append([]Grant(nil), grants...)
				result := Admit(Request{Schema: SchemaV1, AgentID: agent, Class: class, ObservedGrants: observed})
				if !result.Allowed {
					t.Fatal("copy mutation changed admission")
				}
				observed[0] = "mutated input"
				if !reflect.DeepEqual(result.VerifiedGrants, grants) {
					t.Fatal("verified grants alias request")
				}
				result.VerifiedGrants[0] = "mutated result"
				fresh, _ := ForAgent(agent)
				if !reflect.DeepEqual(fresh, original) {
					t.Fatal("caller mutation changed canonical capability")
				}
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

func TestForAgentDeclaresOnlyClaudeKiroAndPi(t *testing.T) {
	t.Parallel()

	want := map[model.AgentID]map[Class][]Grant{
		model.AgentClaudeCode: {
			ClassDocumentation: {GrantWebFetch},
			ClassOpenWeb:       {GrantWebSearch, GrantWebFetch},
		},
		model.AgentKiroIDE: {ClassDocumentation: {GrantContext7}},
		model.AgentPi: {
			ClassDocumentation: {"fetch_content"},
			ClassOpenWeb:       {"web_search", "source_check", "fetch_content", "get_search_content"},
		},
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
