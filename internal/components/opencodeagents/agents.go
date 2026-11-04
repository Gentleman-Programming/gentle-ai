// Package opencodeagents defines the OpenCode-compatible agents owned by gentle-ai.
package opencodeagents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

type Spec struct {
	Name        string
	Description string
	Permission  map[string]any
}

var parity = []Spec{
	{"gentle-ai-explore", "Read-only exploration and mapping for generic ODD work.", map[string]any{"write": "deny", "edit": "deny", "bash": "deny", "task": "deny"}},
	{"gentle-ai-verify", "Read-only technical verification for generic ODD work.", map[string]any{"write": "deny", "edit": "deny", "task": "deny"}},
	{"gentle-ai-worker", "Scoped package-owned implementation writer for bounded ODD work. Edits code, runs focused tests, and returns review-ready evidence without committing.", map[string]any{"task": "deny"}},
	{"jd-judge-a", "Judgment Day blind adversarial reviewer A. Read-only; reports findings and does not fix code.", map[string]any{"write": "deny", "edit": "deny", "task": "deny"}},
	{"jd-judge-b", "Judgment Day blind adversarial reviewer B. Read-only; independently reports findings and does not fix code.", map[string]any{"write": "deny", "edit": "deny", "task": "deny"}},
	{"jd-fix-agent", "Judgment Day surgical fix agent for confirmed findings. Can edit code and run focused tests.", map[string]any{"task": "deny"}},
	{"review-risk", "R1 Risk reviewer — security, privilege boundaries, data exposure, dependency risks, and merge-blocking vulnerabilities.", map[string]any{"write": "deny", "edit": "deny", "bash": "deny", "task": "deny"}},
	{"review-readability", "R2 Readability reviewer — naming, complexity, intention, maintainability, review size, and context clarity.", map[string]any{"write": "deny", "edit": "deny", "bash": "deny", "task": "deny"}},
	{"review-reliability", "R3 Reliability reviewer — behavior-first tests, coverage value, edge cases, determinism, contracts, and regressions.", map[string]any{"write": "deny", "edit": "deny", "bash": "deny", "task": "deny"}},
	{"review-resilience", "R4 Resilience reviewer — fallbacks, retry/backoff, graceful degradation, observability, load, rollback, and SLO risks.", map[string]any{"write": "deny", "edit": "deny", "bash": "deny", "task": "deny"}},
}

var reviewNames = []string{"review-risk", "review-readability", "review-reliability", "review-resilience", "review-refuter", "review-validator"}

func ReviewNames() []string { return append([]string(nil), reviewNames...) }
func IsReview(name string) bool {
	for _, candidate := range reviewNames {
		if candidate == name {
			return true
		}
	}
	return false
}

func Parity(agent model.AgentID) []Spec {
	if agent == model.AgentOpenCode {
		return append([]Spec(nil), parity...)
	}
	var specs []Spec
	for _, spec := range parity {
		if strings.HasPrefix(spec.Name, "gentle-ai-") || (!model.SupportsReceiptDrivenDevelopment(agent) && IsReview(spec.Name)) {
			continue
		}
		specs = append(specs, spec)
	}
	return specs
}

func Roles(agent model.AgentID) []string {
	var names []string
	if model.SupportsReceiptDrivenDevelopment(agent) {
		names = append(names, "review-refuter")
	}
	if agent == model.AgentOpenCode {
		names = append(names, "review-validator")
	}
	for _, spec := range Parity(agent) {
		names = append(names, spec.Name)
	}
	return names
}

// LegacyOwned reports the v3.7.0 marked roles the runtime installed or
// explicitly retired during migration. Unknown marked names remain user data.
func LegacyOwned(agent model.AgentID, name string) bool {
	if name == "gentle-orchestrator" || name == "general" || name == "explore" || strings.HasPrefix(name, "sdd-") {
		return true
	}
	for _, role := range Roles(agent) {
		if role == name {
			return true
		}
	}
	return !model.SupportsReceiptDrivenDevelopment(agent) && IsReview(name)
}

// UninstallRole recognizes current runtime roles and retired Kilo review roles.
// Shape checks remain necessary before removing any unmarked entry.
func UninstallRole(agent model.AgentID, name string) bool {
	for _, role := range Roles(agent) {
		if role == name {
			return true
		}
	}
	return agent == model.AgentKilocode && IsReview(name) && name != "review-validator"
}

func Entry(spec Spec) (map[string]any, error) {
	prompt, err := assets.Read("opencode/agents/" + spec.Name + ".md")
	if err != nil {
		return nil, fmt.Errorf("read embedded prompt for %q: %w", spec.Name, err)
	}
	return map[string]any{"mode": "subagent", "hidden": true, "description": spec.Description, "prompt": prompt, "permission": spec.Permission}, nil
}

func Refuter() map[string]any {
	return map[string]any{"mode": "subagent", "hidden": true, "description": "Read-only refuter for provider-issued review findings", "prompt": "Evaluate only the Go-issued review refuter task. Inspect only the frozen candidate through the provided commands. Do not edit files or delegate. Return only the requested result.", "permission": map[string]any{"write": "deny", "edit": "deny", "task": "deny"}}
}
func Validator() map[string]any {
	return map[string]any{"mode": "subagent", "hidden": true, "description": "Targeted read-only validator for provider-issued review checks", "prompt": "Execute only the Go-issued targeted validation. Do not edit files or delegate. Inspect only the frozen candidate using the provided gentle-ai review inspect-candidate command, not the live worktree. Return exactly the requested JSON.", "permission": map[string]any{"write": "deny", "edit": "deny", "task": "deny", "bash": map[string]any{"gentle-ai review inspect-candidate --purpose targeted-validation *": "allow", "*": "deny"}}}
}

// nativeActions maps V1 permission actions to their native OpenCode V2 names.
// Unknown actions pass through unchanged.
var nativeActions = map[string]string{"bash": "shell", "task": "subagent", "write": "edit", "patch": "edit"}

// NativeRules converts a V1 permission object into the ordered native
// `permissions` rule list. OpenCode evaluates rule lists in order and the last
// matching rule wins, while the V1 object evaluator walks patterns in byte
// order ("*" sorts first), so sorting the emitted rules by resource preserves
// the exact same precedence.
func NativeRules(permission map[string]any) []map[string]any {
	type rule struct{ action, resource, effect string }
	var rules []rule
	add := func(action, resource, effect string) {
		if mapped, ok := nativeActions[action]; ok {
			action = mapped
		}
		rules = append(rules, rule{action, resource, effect})
	}
	for action, value := range permission {
		switch typed := value.(type) {
		case string:
			add(action, "*", typed)
		case bool:
			if typed {
				add(action, "*", "allow")
			} else {
				add(action, "*", "deny")
			}
		case map[string]any:
			for resource, effect := range typed {
				switch e := effect.(type) {
				case string:
					add(action, resource, e)
				case bool:
					if e {
						add(action, resource, "allow")
					} else {
						add(action, resource, "deny")
					}
				}
			}
		}
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].action != rules[j].action {
			return rules[i].action < rules[j].action
		}
		return rules[i].resource < rules[j].resource
	})
	out := make([]map[string]any, 0, len(rules))
	for _, r := range rules {
		out = append(out, map[string]any{"action": r.action, "resource": r.resource, "effect": r.effect})
	}
	return out
}

// NativeEntry renders spec as a native OpenCode V2 agent entry: the embedded
// prompt moves to `system` and the V1 permission object becomes an ordered
// native `permissions` rule list.
func NativeEntry(spec Spec) (map[string]any, error) {
	prompt, err := assets.Read("opencode/agents/" + spec.Name + ".md")
	if err != nil {
		return nil, fmt.Errorf("read embedded prompt for %q: %w", spec.Name, err)
	}
	return map[string]any{"mode": "subagent", "hidden": true, "description": spec.Description, "system": prompt, "permissions": NativeRules(spec.Permission)}, nil
}

// NativeRefuter renders the review refuter as a native OpenCode V2 entry.
func NativeRefuter() map[string]any {
	refuter := Refuter()
	rules, _ := refuter["permission"].(map[string]any)
	return map[string]any{"mode": "subagent", "hidden": true, "description": refuter["description"], "system": refuter["prompt"], "permissions": NativeRules(rules)}
}

// NativeValidator renders the review validator as a native OpenCode V2 entry.
func NativeValidator() map[string]any {
	validator := Validator()
	rules, _ := validator["permission"].(map[string]any)
	return map[string]any{"mode": "subagent", "hidden": true, "description": validator["description"], "system": validator["prompt"], "permissions": NativeRules(rules)}
}

// MergeRuleLists appends incoming rules to existing rules, skipping rules that
// already exist verbatim (action, resource, effect). Existing rules keep their
// order so user-authored precedence decisions are preserved.
func MergeRuleLists(existing, incoming []map[string]any) []map[string]any {
	key := func(r map[string]any) string {
		action, _ := r["action"].(string)
		resource, _ := r["resource"].(string)
		effect, _ := r["effect"].(string)
		return action + "\x00" + resource + "\x00" + effect
	}
	seen := make(map[string]bool, len(existing)+len(incoming))
	merged := make([]map[string]any, 0, len(existing)+len(incoming))
	for _, rule := range existing {
		if rule == nil {
			continue
		}
		k := key(rule)
		if seen[k] {
			continue
		}
		seen[k] = true
		merged = append(merged, rule)
	}
	for _, rule := range incoming {
		k := key(rule)
		if seen[k] {
			continue
		}
		seen[k] = true
		merged = append(merged, rule)
	}
	return merged
}

// GentlemanShape recognizes the persona overlay for this runtime, without
// claiming a user-modified agent under the same name.
func GentlemanShape(agent model.AgentID, entry map[string]any) (bool, error) {
	want := map[string]any{"mode": "primary", "description": "Senior Architect mentor - helpful first, challenging when it matters", "prompt": "{file:./AGENTS.md}"}
	if agent == model.AgentKilocode {
		want["tools"] = map[string]any{"write": true, "edit": true}
	}
	comparable := make(map[string]any, len(entry))
	for k, v := range entry {
		if k != "model" && k != "variant" {
			comparable[k] = v
		}
	}
	got, err := json.Marshal(comparable)
	if err != nil {
		return false, err
	}
	expected, err := json.Marshal(want)
	if err != nil {
		return false, err
	}
	return bytes.Equal(got, expected), nil
}

// ReviewShape returns the entry installed for a managed review role.
func ReviewShape(name string) (map[string]any, bool) {
	if !IsReview(name) {
		return nil, false
	}
	switch name {
	case "review-refuter":
		return Refuter(), true
	case "review-validator":
		return Validator(), true
	default:
		for _, spec := range parity {
			if spec.Name == name {
				entry, err := Entry(spec)
				return entry, err == nil
			}
		}
	}
	return nil, false
}

// Shape accepts user-selected model and variant, but no other deviations.
func Shape(name string, entry map[string]any) (bool, error) {
	var want map[string]any
	switch name {
	case "review-refuter", "review-validator":
		want, _ = ReviewShape(name)
	default:
		for _, spec := range parity {
			if spec.Name == name {
				var err error
				want, err = Entry(spec)
				if err != nil {
					return false, err
				}
				break
			}
		}
	}
	if want == nil {
		return false, nil
	}
	comparable := make(map[string]any, len(entry))
	for k, v := range entry {
		if k != "model" && k != "variant" {
			comparable[k] = v
		}
	}
	got, err := json.Marshal(comparable)
	if err != nil {
		return false, err
	}
	expected, err := json.Marshal(want)
	if err != nil {
		return false, err
	}
	return bytes.Equal(got, expected), nil
}
