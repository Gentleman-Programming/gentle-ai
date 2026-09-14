package researchcapability

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// TestOpenCodeResearchAdmissionRequiresExactDeclaredGrants pins the intentional
// OpenCode denial (#4088): webfetch/websearch identities never become grants,
// and the existing exact-multiset admission remains intact for a runtime the
// canonical matrix admits.
func TestOpenCodeResearchAdmissionRequiresExactDeclaredGrants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		request     Request
		wantAllowed bool
		wantGrants  []Grant
	}{
		{
			name:    "opencode empty grants",
			request: Request{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassDocumentation},
		},
		{
			name:    "opencode mapped webfetch identity",
			request: Request{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassDocumentation, ObservedGrants: []Grant{"webfetch"}},
		},
		{
			name:    "opencode mapped websearch identity",
			request: Request{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassOpenWeb, ObservedGrants: []Grant{"websearch"}},
		},
		{
			name:    "opencode both mapped identities",
			request: Request{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassOpenWeb, ObservedGrants: []Grant{"webfetch", "websearch"}},
		},
		{
			name:    "opencode unknown identity",
			request: Request{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassDocumentation, ObservedGrants: []Grant{"Bash"}},
		},
		{
			name:    "opencode documentation class empty",
			request: Request{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassDocumentation, ObservedGrants: nil},
		},
		{
			name:    "opencode open web class empty",
			request: Request{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassOpenWeb, ObservedGrants: nil},
		},
		{
			name:        "claude documentation full set",
			request:     Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, Class: ClassDocumentation, ObservedGrants: []Grant{GrantWebFetch}},
			wantAllowed: true,
			wantGrants:  []Grant{GrantWebFetch},
		},
		{
			name:        "claude open web full set",
			request:     Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, Class: ClassOpenWeb, ObservedGrants: []Grant{GrantWebSearch, GrantWebFetch}},
			wantAllowed: true,
			wantGrants:  []Grant{GrantWebSearch, GrantWebFetch},
		},
		{
			name:        "claude open web reordered full set",
			request:     Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, Class: ClassOpenWeb, ObservedGrants: []Grant{GrantWebFetch, GrantWebSearch}},
			wantAllowed: true,
			wantGrants:  []Grant{GrantWebSearch, GrantWebFetch},
		},
		{
			name:    "claude documentation subset",
			request: Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, Class: ClassDocumentation},
		},
		{
			name:    "claude documentation superset",
			request: Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, Class: ClassDocumentation, ObservedGrants: []Grant{GrantWebFetch, GrantWebSearch}},
		},
		{
			name:    "claude open web subset",
			request: Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, Class: ClassOpenWeb, ObservedGrants: []Grant{GrantWebSearch}},
		},
		{
			name:    "claude open web superset",
			request: Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, Class: ClassOpenWeb, ObservedGrants: []Grant{GrantWebSearch, GrantWebFetch, GrantContext7}},
		},
		{
			name:    "claude unknown identity",
			request: Request{Schema: SchemaV1, AgentID: model.AgentClaudeCode, Class: ClassDocumentation, ObservedGrants: []Grant{"Bash"}},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := Admit(test.request)
			if got.Allowed != test.wantAllowed {
				t.Fatalf("Admit() allowed = %v, want %v", got.Allowed, test.wantAllowed)
			}
			if !reflect.DeepEqual(got.VerifiedGrants, test.wantGrants) {
				t.Fatalf("Admit() verified grants = %v, want %v", got.VerifiedGrants, test.wantGrants)
			}
		})
	}
}

// TestOpenCodeResearchDenialEmitsNoClaims proves every OpenCode request shape
// produces a closed denial: no verified grants and no source claims.
func TestOpenCodeResearchDenialEmitsNoClaims(t *testing.T) {
	t.Parallel()

	requests := []Request{
		{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassDocumentation},
		{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassDocumentation, ObservedGrants: []Grant{"webfetch"}},
		{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassOpenWeb, ObservedGrants: []Grant{"websearch"}},
		{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassOpenWeb, ObservedGrants: []Grant{"webfetch", "websearch"}},
		{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassDocumentation, ObservedGrants: []Grant{"Bash"}},
		{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassDocumentation, ObservedGrants: []Grant{"WebFetch"}},
		{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: "repository", ObservedGrants: []Grant{"webfetch"}},
		{Schema: SchemaV1, AgentID: model.AgentOpenCode, Class: ClassOpenWeb, ObservedGrants: []Grant{"Bash", "mcp", "inherited"}},
	}

	for _, request := range requests {
		got := Admit(request)
		if got.Allowed {
			t.Fatalf("Admit(%#v) allowed = true, want closed denial", request)
		}
		if len(got.VerifiedGrants) != 0 {
			t.Fatalf("Admit(%#v) verified grants = %v, want none", request, got.VerifiedGrants)
		}
		if len(got.Claims) != 0 {
			t.Fatalf("Admit(%#v) claims = %v, want none", request, got.Claims)
		}
	}
}

func TestDeclarationRoundTripsThroughParseDeclaration(t *testing.T) {
	t.Parallel()

	for _, agent := range []model.AgentID{
		model.AgentClaudeCode, model.AgentKiroIDE, model.AgentPi,
		model.AgentOpenCode, model.AgentKilocode,
	} {
		agent := agent
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			parsed, ok := ParseDeclaration(Declaration(agent))
			if !ok {
				t.Fatalf("ParseDeclaration(Declaration(%q)) refused the canonical declaration %q", agent, Declaration(agent))
			}
			if len(parsed) != 2 {
				t.Fatalf("ParseDeclaration(Declaration(%q)) classes = %v, want both classes always present", agent, parsed)
			}
			canonical, declared := ForAgent(agent)
			for _, class := range []Class{ClassDocumentation, ClassOpenWeb} {
				var want []Grant
				if declared {
					want = canonical.Grants[class]
				}
				if !sameGrants(parsed[class], want) {
					t.Fatalf("ParseDeclaration(Declaration(%q))[%s] = %v, want %v", agent, class, parsed[class], want)
				}
			}
		})
	}
}

func TestDeclarationMatchesShippedRuntimeAssets(t *testing.T) {
	t.Parallel()

	exact := map[model.AgentID]string{
		model.AgentClaudeCode: "Evidence grants: documentation=[WebFetch]; open-web=[WebSearch,WebFetch].",
		model.AgentKiroIDE:    "Evidence grants: documentation=[@context7]; open-web=[].",
		model.AgentCursor:     "Evidence grants: documentation=[]; open-web=[].",
		model.AgentKimi:       "Evidence grants: documentation=[]; open-web=[].",
	}
	for agent, want := range exact {
		if got := Declaration(agent); got != want {
			t.Fatalf("Declaration(%q) = %q, want %q", agent, got, want)
		}
	}

	for path, agent := range map[string]model.AgentID{
		"claude/agents/sdd-research.md": model.AgentClaudeCode,
		"kiro/agents/sdd-research.md":   model.AgentKiroIDE,
		"cursor/agents/sdd-research.md": model.AgentCursor,
		"kimi/agents/sdd-research.md":   model.AgentKimi,
	} {
		if content := assets.MustRead(path); !strings.Contains(content, Declaration(agent)) {
			t.Fatalf("%s does not carry the canonical declaration %q", path, Declaration(agent))
		}
	}

	for _, path := range []string{"opencode/sdd-overlay-single.json", "opencode/sdd-overlay-multi.json"} {
		var overlay struct {
			Agent map[string]struct {
				Prompt string `json:"prompt"`
			} `json:"agent"`
		}
		if err := json.Unmarshal([]byte(assets.MustRead(path)), &overlay); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		if got := overlay.Agent["sdd-research"].Prompt; !strings.Contains(got, Declaration(model.AgentOpenCode)) {
			t.Fatalf("%s research prompt does not carry the canonical declaration %q", path, Declaration(model.AgentOpenCode))
		}
	}
}

// TestMarkdownProjectionAcceptsShippedResearchAssets runs every shipped
// markdown research asset through the production extraction plus verification
// boundary, and pins the Claude/Kiro tools line to the authority decisions.
func TestMarkdownProjectionAcceptsShippedResearchAssets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path  string
		agent model.AgentID
	}{
		{"claude/agents/sdd-research.md", model.AgentClaudeCode},
		{"kiro/agents/sdd-research.md", model.AgentKiroIDE},
		{"cursor/agents/sdd-research.md", model.AgentCursor},
		{"kimi/agents/sdd-research.md", model.AgentKimi},
	}
	for _, test := range tests {
		test := test
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()

			projection, err := MarkdownProjection(test.agent, assets.MustRead(test.path))
			if err != nil {
				t.Fatalf("MarkdownProjection(%q) error = %v", test.agent, err)
			}
			if err := VerifyProjection(projection); err != nil {
				t.Fatalf("VerifyProjection(%q) error = %v", test.agent, err)
			}
			var wantAllowed []string
			for _, decision := range EvidenceToolDecisions(test.agent) {
				if decision.Allowed {
					wantAllowed = append(wantAllowed, decision.Tool)
				}
			}
			if !reflect.DeepEqual(projection.AllowedTools, wantAllowed) {
				t.Fatalf("allowed tools = %v, want authority decisions %v", projection.AllowedTools, wantAllowed)
			}
		})
	}
}

func TestMarkdownProjectionRejectsTamperedAssets(t *testing.T) {
	t.Parallel()

	claude := assets.MustRead("claude/agents/sdd-research.md")
	kiro := assets.MustRead("kiro/agents/sdd-research.md")
	cursor := assets.MustRead("cursor/agents/sdd-research.md")

	tests := []struct {
		name    string
		agent   model.AgentID
		content string
	}{
		{
			name:    "unknown token",
			agent:   model.AgentClaudeCode,
			content: strings.Replace(claude, "tools: WebFetch, WebSearch", "tools: WebFetch, curl", 1),
		},
		{
			name:    "duplicate token",
			agent:   model.AgentClaudeCode,
			content: strings.Replace(claude, "tools: WebFetch, WebSearch", "tools: WebFetch, WebFetch", 1),
		},
		{
			name:    "comma without space",
			agent:   model.AgentClaudeCode,
			content: strings.Replace(claude, "tools: WebFetch, WebSearch", "tools: WebFetch,WebSearch", 1),
		},
		{
			name:    "two tools lines",
			agent:   model.AgentClaudeCode,
			content: strings.Replace(claude, "tools: WebFetch, WebSearch", "tools: WebFetch, WebSearch\ntools: WebFetch", 1),
		},
		{
			name:    "kiro malformed array",
			agent:   model.AgentKiroIDE,
			content: strings.Replace(kiro, `tools: ["@context7"]`, `tools: ["@context7"`, 1),
		},
		{
			name:    "kiro duplicate array entry",
			agent:   model.AgentKiroIDE,
			content: strings.Replace(kiro, `tools: ["@context7"]`, `tools: ["@context7", "@context7"]`, 1),
		},
		{
			name:    "kiro object instead of array",
			agent:   model.AgentKiroIDE,
			content: strings.Replace(kiro, `tools: ["@context7"]`, `tools: "@context7"`, 1),
		},
		{
			name:    "cursor tools line",
			agent:   model.AgentCursor,
			content: strings.Replace(cursor, "readonly: true", "tools: WebFetch\nreadonly: true", 1),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := MarkdownProjection(test.agent, test.content); err == nil {
				t.Fatalf("MarkdownProjection(%q) = nil error, want refusal", test.agent)
			}
		})
	}

	// `exclude_tools:` must not be mistaken for the evidence tools line. The
	// projection then has an empty tool surface, which verification refuses
	// for Claude because the canonical matrix declares WebFetch/WebSearch.
	t.Run("exclude_tools is not a tools line", func(t *testing.T) {
		t.Parallel()

		tampered := strings.Replace(claude, "tools: WebFetch, WebSearch", "exclude_tools: WebFetch, WebSearch", 1)
		projection, err := MarkdownProjection(model.AgentClaudeCode, tampered)
		if err != nil {
			t.Fatalf("MarkdownProjection() error = %v", err)
		}
		if len(projection.AllowedTools) != 0 {
			t.Fatalf("AllowedTools = %v, want empty for a line that is not a tools line", projection.AllowedTools)
		}
		if err := VerifyProjection(projection); err == nil {
			t.Fatal("VerifyProjection() = nil, want refusal for a missing canonical tool surface")
		}
	})
}

// TestVerifyProjectionRejectsEveryAmbiguity covers the strict declaration
// grammar and the projection refusal rules. Every failure must name the agent
// so a production error identifies which generated surface drifted.
func TestVerifyProjectionRejectsEveryAmbiguity(t *testing.T) {
	type parseCase struct {
		name string
		text string
	}
	parseCases := []parseCase{
		{name: "declaration with list spacing", text: "Evidence grants: documentation=[A, B]; open-web=[]. "},
		{name: "duplicate class", text: "Evidence grants: documentation=[]; documentation=[]; open-web=[]. "},
		{name: "missing class", text: "Evidence grants: documentation=[]. "},
		{name: "missing period", text: "Evidence grants: documentation=[]; open-web=[] "},
		{name: "two occurrences", text: "Evidence grants: documentation=[]; open-web=[]. Evidence grants: documentation=[]; open-web=[]. "},
		{name: "zero occurrences", text: "documentation=[]; open-web=[]. "},
		{name: "trailing comma", text: "Evidence grants: documentation=[A,]; open-web=[]. "},
		{name: "empty token", text: "Evidence grants: documentation=[A,,B]; open-web=[]. "},
		{name: "duplicate grant in list", text: "Evidence grants: documentation=[A,A]; open-web=[]. "},
		{name: "extra punctuation", text: "Evidence grants: documentation=[];; open-web=[]. "},
		{name: "unknown class order", text: "Evidence grants: open-web=[]; documentation=[]. "},
	}
	for _, test := range parseCases {
		test := test
		t.Run("parse: "+test.name, func(t *testing.T) {
			if _, ok := ParseDeclaration(test.text); ok {
				t.Fatalf("ParseDeclaration(%q) = ok, want refusal", test.text)
			}
		})
	}

	claudeDeclared := func(documentation, openWeb []Grant) map[Class][]Grant {
		return map[Class][]Grant{ClassDocumentation: documentation, ClassOpenWeb: openWeb}
	}
	canonicalClaude := claudeDeclared([]Grant{GrantWebFetch}, []Grant{GrantWebSearch, GrantWebFetch})

	projectionCases := []struct {
		name       string
		projection RuntimeProjection
		wantReason string
	}{
		{
			name: "wrong grant",
			projection: RuntimeProjection{
				Agent: model.AgentClaudeCode, HasDeclaration: true, Allowlist: true,
				Declared:     claudeDeclared([]Grant{GrantWebSearch}, []Grant{GrantWebSearch, GrantWebFetch}),
				AllowedTools: []string{"WebSearch", "WebFetch"},
			},
			wantReason: "declared",
		},
		{
			name: "extra grant",
			projection: RuntimeProjection{
				Agent: model.AgentClaudeCode, HasDeclaration: true, Allowlist: true,
				Declared:     claudeDeclared([]Grant{GrantWebFetch, "Bash"}, []Grant{GrantWebSearch, GrantWebFetch}),
				AllowedTools: []string{"WebFetch", "WebSearch"},
			},
			wantReason: "declared",
		},
		{
			name: "missing explicit deny",
			projection: RuntimeProjection{
				Agent: model.AgentOpenCode, HasDeclaration: true, Allowlist: false,
				Declared: claudeDeclared([]Grant{}, []Grant{}),
			},
			wantReason: "explicitly deny",
		},
		{
			name: "allow and deny same identity",
			projection: RuntimeProjection{
				Agent: model.AgentClaudeCode, HasDeclaration: true, Allowlist: false,
				Declared:     canonicalClaude,
				AllowedTools: []string{"WebFetch", "WebSearch"},
				DeniedTools:  []string{"WebFetch"},
			},
			wantReason: "both allowed and denied",
		},
		{
			name: "duplicate allowed identity",
			projection: RuntimeProjection{
				Agent: model.AgentClaudeCode, HasDeclaration: true, Allowlist: true,
				Declared:     canonicalClaude,
				AllowedTools: []string{"WebFetch", "WebSearch", "WebFetch"},
			},
			wantReason: "duplicate allowed",
		},
		{
			name: "unknown allowed identity",
			projection: RuntimeProjection{
				Agent: model.AgentClaudeCode, HasDeclaration: true, Allowlist: true,
				Declared:     canonicalClaude,
				AllowedTools: []string{"WebFetch", "WebSearch", "curl"},
			},
			wantReason: "no adapter binding",
		},
		{
			name: "missing canonical tool",
			projection: RuntimeProjection{
				Agent: model.AgentClaudeCode, HasDeclaration: true, Allowlist: true,
				Declared:     canonicalClaude,
				AllowedTools: []string{"WebFetch"},
			},
			wantReason: "missing",
		},
		{
			name: "allowlist format with deny entries",
			projection: RuntimeProjection{
				Agent: model.AgentClaudeCode, HasDeclaration: true, Allowlist: true,
				Declared:     canonicalClaude,
				AllowedTools: []string{"WebFetch", "WebSearch"},
				DeniedTools:  []string{"WebFetch"},
			},
			wantReason: "deny channel",
		},
		{
			name: "unknown declared class",
			projection: RuntimeProjection{
				Agent: model.AgentClaudeCode, HasDeclaration: true, Allowlist: true,
				Declared: map[Class][]Grant{
					ClassDocumentation:  {GrantWebFetch},
					ClassOpenWeb:        {GrantWebSearch, GrantWebFetch},
					Class("repository"): []Grant{},
				},
				AllowedTools: []string{"WebFetch", "WebSearch"},
			},
			wantReason: "unknown declared research class",
		},
		{
			name: "missing declaration with canonical grants",
			projection: RuntimeProjection{
				Agent: model.AgentClaudeCode, HasDeclaration: false, Allowlist: true,
			},
			wantReason: "no declaration",
		},
		{
			name: "declared grants without declaration flag",
			projection: RuntimeProjection{
				Agent: model.AgentOpenCode, HasDeclaration: false, Allowlist: false,
				Declared: map[Class][]Grant{ClassDocumentation: {}},
			},
			wantReason: "must not carry declared grants",
		},
	}
	for _, test := range projectionCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := VerifyProjection(test.projection)
			if err == nil {
				t.Fatalf("VerifyProjection(%#v) = nil, want error", test.projection)
			}
			if !strings.Contains(err.Error(), string(test.projection.Agent)) {
				t.Fatalf("VerifyProjection() error %q does not name agent %q", err, test.projection.Agent)
			}
			if !strings.Contains(err.Error(), test.wantReason) {
				t.Fatalf("VerifyProjection() error = %q, want reason containing %q", err, test.wantReason)
			}
		})
	}

	// The unmappable-grant case needs a canonical grant without a binding. The
	// capability table is package state, so this subtest mutates and restores it
	// and never runs in parallel.
	t.Run("unmappable canonical grant", func(t *testing.T) {
		original, existed := capabilities[model.AgentClaudeCode]
		capabilities[model.AgentClaudeCode] = Capability{
			Schema: SchemaV1,
			Grants: map[Class][]Grant{ClassDocumentation: {"unmappable-tool"}},
		}
		defer func() {
			if existed {
				capabilities[model.AgentClaudeCode] = original
				return
			}
			delete(capabilities, model.AgentClaudeCode)
		}()

		projection := RuntimeProjection{
			Agent: model.AgentClaudeCode, HasDeclaration: true, Allowlist: true,
			Declared: claudeDeclared([]Grant{"unmappable-tool"}, []Grant{}),
		}
		err := VerifyProjection(projection)
		if err == nil {
			t.Fatal("VerifyProjection() = nil, want unmappable-grant refusal")
		}
		if !strings.Contains(err.Error(), string(model.AgentClaudeCode)) || !strings.Contains(err.Error(), "unmappable-tool") {
			t.Fatalf("VerifyProjection() error = %q, want agent and unmappable grant named", err)
		}
	})
}

func TestEvidenceToolDecisionsFollowBindingOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		agent model.AgentID
		want  []ToolDecision
	}{
		{
			agent: model.AgentClaudeCode,
			want:  []ToolDecision{{Tool: "WebFetch", Allowed: true}, {Tool: "WebSearch", Allowed: true}},
		},
		{
			agent: model.AgentKiroIDE,
			want:  []ToolDecision{{Tool: "@context7", Allowed: true}},
		},
		{
			agent: model.AgentPi,
			want: []ToolDecision{
				{Tool: "fetch_content", Allowed: true},
				{Tool: "web_search", Allowed: true},
				{Tool: "source_check", Allowed: true},
				{Tool: "get_search_content", Allowed: true},
			},
		},
		{
			agent: model.AgentOpenCode,
			want:  []ToolDecision{{Tool: "webfetch", Allowed: false}, {Tool: "websearch", Allowed: false}},
		},
		{
			agent: model.AgentKilocode,
			want:  []ToolDecision{{Tool: "webfetch", Allowed: false}, {Tool: "websearch", Allowed: false}},
		},
	}
	for _, test := range tests {
		if got := EvidenceToolDecisions(test.agent); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("EvidenceToolDecisions(%q) = %v, want %v", test.agent, got, test.want)
		}
	}
	if got := EvidenceToolDecisions(model.AgentCursor); got != nil {
		t.Fatalf("EvidenceToolDecisions(cursor) = %v, want no bindings", got)
	}
}

func TestAdapterToolBindingsAreDefensiveCopies(t *testing.T) {
	t.Parallel()

	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentKiroIDE, model.AgentOpenCode} {
		bindings := AdapterToolBindings(agent)
		if len(bindings) == 0 {
			t.Fatalf("AdapterToolBindings(%q) is empty", agent)
		}
		original := bindings[0]
		bindings[0] = ToolBinding{Grant: "mutated", Tool: "mutated"}
		fresh := AdapterToolBindings(agent)
		if fresh[0] != original {
			t.Fatalf("AdapterToolBindings(%q) aliases caller mutation", agent)
		}
	}
}
