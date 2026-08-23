// Package researchcapability defines the closed admission contract for
// source-backed SDD research. It is deliberately separate from the immutable
// AgentCapabilityManifest v1 projection.
package researchcapability

import "github.com/gentleman-programming/gentle-ai/v2/internal/model"

type SchemaVersion string

const SchemaV1 SchemaVersion = "gentle-ai.sdd-research-capability/v1"

type Class string

const (
	ClassDocumentation Class = "documentation"
	ClassOpenWeb       Class = "open-web"
)

type Grant string

const (
	GrantWebFetch  Grant = "WebFetch"
	GrantWebSearch Grant = "WebSearch"
	GrantContext7  Grant = "@context7"
)

// Capability is one runtime's maximum declared evidence capability.
type Capability struct {
	Schema SchemaVersion
	Grants map[Class][]Grant
}

// Request carries observed runtime grants. Admission accepts only an exact
// match; filenames, Bash, generic MCP access, and inherited tools have no
// special meaning.
type Request struct {
	Schema         SchemaVersion
	AgentID        model.AgentID
	Class          Class
	ObservedGrants []Grant
}

type Result struct {
	Allowed        bool
	VerifiedGrants []Grant
	Claims         []string
}

var capabilities = map[model.AgentID]Capability{
	model.AgentClaudeCode: {
		Schema: SchemaV1,
		Grants: map[Class][]Grant{
			ClassDocumentation: {GrantWebFetch},
			ClassOpenWeb:       {GrantWebSearch, GrantWebFetch},
		},
	},
	model.AgentKiroIDE: {
		Schema: SchemaV1,
		Grants: map[Class][]Grant{
			ClassDocumentation: {GrantContext7},
		},
	},
}

// ForAgent returns a defensive copy of a declared capability. Every absent
// AgentID is denied rather than inheriting a runtime-wide default.
func ForAgent(agent model.AgentID) (Capability, bool) {
	capability, ok := capabilities[agent]
	if !ok {
		return Capability{}, false
	}
	copyOf := Capability{Schema: capability.Schema, Grants: make(map[Class][]Grant, len(capability.Grants))}
	for class, grants := range capability.Grants {
		copyOf.Grants[class] = append([]Grant(nil), grants...)
	}
	return copyOf, true
}

func Admit(request Request) Result {
	capability, ok := ForAgent(request.AgentID)
	if !ok || request.Schema != SchemaV1 || capability.Schema != request.Schema {
		return Result{}
	}
	want, ok := capability.Grants[request.Class]
	if !ok || !sameGrants(request.ObservedGrants, want) {
		return Result{}
	}
	return Result{Allowed: true, VerifiedGrants: append([]Grant(nil), want...)}
}

func sameGrants(got, want []Grant) bool {
	if len(got) != len(want) {
		return false
	}
	counts := make(map[Grant]int, len(want))
	for _, grant := range want {
		counts[grant]++
	}
	for _, grant := range got {
		counts[grant]--
		if counts[grant] < 0 {
			return false
		}
	}
	return true
}
