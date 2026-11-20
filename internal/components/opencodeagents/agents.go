// Package opencodeagents defines the OpenCode-compatible agents owned by gentle-ai.
package opencodeagents

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	if spec.Name == "gentle-ai-verify" {
		// Inline carriers must work even when no skills component is selected.
		method, err := assets.Read("skills/_shared/verify-method.md")
		if err != nil {
			return nil, fmt.Errorf("read shared verification method: %w", err)
		}
		prompt = strings.ReplaceAll(prompt, "{{GENTLE_AI_VERIFY_METHOD}}", method)
	}
	return map[string]any{"mode": "subagent", "hidden": true, "description": spec.Description, "prompt": prompt, "permission": spec.Permission}, nil
}

// releasedPromptEdits reverse, per agent, every asset edit made since v4.5.0
// (f2f7d7b16), current text first and released text second, so the exact
// prompt v4.5.0 installed is derived from the current one. An upgrading user's
// unmodified agent is then still recognized as ours instead of being kept as a
// user edit. Each current snippet must occur exactly once, or the derivation
// fails and nothing extra is owned. Add a pair here whenever a released prompt
// changes; never remove one while that release can still be installed.
var releasedPromptEdits = map[string][][2]string{
	"gentle-ai-explore": {
		{"file contents. Never claim evidence", "file contents. Any requested `## Key Learnings` block follows it. Never claim evidence"},
	},
	"gentle-ai-worker": {
		{"list. Never widen it yourself: a route beyond it is reported to the parent with the exact candidate paths and reasons, and only a parent-issued replacement of that section authorizes it, carrying every still-needed path because the replacement is not a union. That declaration never substitutes for repository or worktree authority and never authorizes an operation the existing rules prohibit.", "list."},
		{"choice the task does not delegate to you is ambiguous, stop with `status: interaction_required`; do not guess. Escalate in the answerable shape required by the Interaction contract below: a derived candidate set the parent can approve, narrow, or route to the human, never an open request for anyone", "human choice is ambiguous, stop with `status: interaction_required`; do not guess. Escalate in the answerable shape required by the Interaction contract below: a derived candidate set the human can approve or narrow, never an open request for the human"},
		{"is not yours to make must use the deterministic `interaction_required` payload below; the parent decides whether it is a technical extension inside the same authorized objective, which it may approve itself, or a real human choice.", "requires a human must use the deterministic `interaction_required` payload below."},
		{"any input you may not decide is", "any human input is"},
		{"decision owner can approve, decline, or select from, and never ask anyone", "human can approve, decline, or select from, and never ask the human"},
		{"each with its reason, and ask the parent to approve that list, narrow it, or route it to the human.", "and ask the human to approve that list or name which entries to drop."},
		{"text, or unchanged file contents.", "text, or unchanged file contents. Any requested `## Key Learnings` block follows it."},
		{"never looping. Name in `summary` the loop's diagnosed cause (for example a wrong test, a wrong hypothesis, missing context, a setup or environment error, or a real blocker). Exhaustion ends this cycle, not the unit: the parent decides the next step from your evidence. Making a new test fail for the expected reason is RED construction, not a fix correction: its own harness or setup errors get the setup bound (one self-fix), separate from the fix's limit; if the test still cannot reproduce the failure, return `status: partial` naming a reproduction blocker, not exhausted corrections.", "never looping."},
	},
	"gentle-ai-verify": {
		{"probe regression tests follow it.", "probe regression tests, then any requested `## Key Learnings` block, follow it."},
	},
}

// releasedPrompt derives the prompt v4.5.0 installed for name from the current
// rendered prompt; ok is false when name has no released derivation or the
// current prompt no longer carries every edited snippet exactly once.
func releasedPrompt(name, prompt string) (string, bool) {
	edits, ok := releasedPromptEdits[name]
	if !ok {
		return "", false
	}
	for _, edit := range edits {
		if strings.Count(prompt, edit[0]) != 1 {
			return "", false
		}
		prompt = strings.Replace(prompt, edit[0], edit[1], 1)
	}
	return prompt, true
}

// legacyVerifyPrompt derives the pre-refactor verifier prompt (3132b5bc6) from
// the v4.5.0 expanded prompt and the canonical method asset.
func legacyVerifyPrompt(prompt string) (string, error) {
	method, err := assets.Read("skills/_shared/verify-method.md")
	if err != nil {
		return "", err
	}
	prompt = strings.ReplaceAll(prompt, "plus the scratch-copy probes in the shared method.", "plus the scratch-copy probes below.")
	body := strings.Index(method, "For behavior changes")
	if body <= 0 {
		return "", fmt.Errorf("verification method lost its body marker")
	}
	return strings.ReplaceAll(prompt, method[:body], ""), nil
}

func Refuter() map[string]any {
	return map[string]any{"mode": "subagent", "hidden": true, "description": "Read-only refuter for provider-issued review findings", "prompt": "Evaluate only the Go-issued review refuter task. Inspect only the frozen candidate through the provided commands. Do not edit files or delegate. Return only the requested result.", "permission": map[string]any{"write": "deny", "edit": "deny", "task": "deny"}}
}
func Validator() map[string]any {
	return map[string]any{"mode": "subagent", "hidden": true, "description": "Targeted read-only validator for provider-issued review checks", "prompt": "Execute only the Go-issued targeted validation. Do not edit files or delegate. Inspect only the frozen candidate using the provided gentle-ai review inspect-candidate command, not the live worktree. Return exactly the requested JSON.", "permission": map[string]any{"write": "deny", "edit": "deny", "task": "deny", "bash": map[string]any{"gentle-ai review inspect-candidate --purpose targeted-validation *": "allow", "*": "deny"}}}
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
	if bytes.Equal(got, expected) {
		return true, nil
	}
	// A prompt an earlier release installed, unmodified, is still ours; every
	// other field must match exactly.
	released, ok := releasedPrompt(name, want["prompt"].(string))
	if !ok {
		return false, nil
	}
	priors := []string{released}
	if name == "gentle-ai-verify" {
		legacy, err := legacyVerifyPrompt(released)
		if err != nil {
			return false, err
		}
		priors = append(priors, legacy)
	}
	for _, prior := range priors {
		want["prompt"] = prior
		if expected, err = json.Marshal(want); err != nil {
			return false, err
		}
		if bytes.Equal(got, expected) {
			return true, nil
		}
	}
	return false, nil
}
