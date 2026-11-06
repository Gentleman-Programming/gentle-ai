package opencodeagents

import (
	"testing"
)

// resolve walks the rule list in order and returns the effect of the last
// rule matching the command, mirroring OpenCode's last-matching-rule
// semantics. A resource matches when it equals the command or ends with "*"
// acting as a trailing wildcard.
func resolve(rules []map[string]any, action, command string) string {
	effect := ""
	for _, rule := range rules {
		if rule["action"] != action {
			continue
		}
		resource := rule["resource"].(string)
		if matches(resource, command) {
			effect = rule["effect"].(string)
		}
	}
	return effect
}

func matches(resource, command string) bool {
	if len(resource) > 0 && resource[len(resource)-1] == '*' {
		return len(command) >= len(resource)-1 && command[:len(resource)-1] == resource[:len(resource)-1]
	}
	return resource == command
}

// TestMergeRulesByPrecedencePreservesEffectiveDecisions pins the reviewer
// scenarios: user rules keep their effective decisions under the
// last-matching-rule semantics after the safety overlay is merged, a
// duplicate-triple user sequence never collapses, and repeated merges are
// idempotent.
func TestMergeRulesByPrecedencePreservesEffectiveDecisions(t *testing.T) {
	user := []map[string]any{
		{"action": "shell", "resource": "git push *", "effect": "ask"},
	}
	overlay := []map[string]any{
		{"action": "shell", "resource": "*", "effect": "allow"},
		{"action": "shell", "resource": "chmod -R 777*", "effect": "deny"},
		{"action": "shell", "resource": "git push", "effect": "ask"},
		{"action": "shell", "resource": "git push *", "effect": "ask"},
	}
	merged := MergeRulesByPrecedence(user, overlay)

	if got := resolve(merged, "shell", "git push origin"); got != "ask" {
		t.Fatalf("effective decision for 'git push origin' = %q, want ask (the overlay wildcard allow must not override the user ask)", got)
	}
	if got := resolve(merged, "shell", "chmod -R 777 /"); got != "deny" {
		t.Fatalf("effective decision for 'chmod -R 777 /' = %q, want deny", got)
	}
	if got := resolve(merged, "shell", "ls -la"); got != "allow" {
		t.Fatalf("effective decision for 'ls -la' = %q, want allow", got)
	}

	// A user duplicate-triple sequence keeps its final decision.
	withDupes := []map[string]any{
		{"action": "shell", "resource": "x", "effect": "allow"},
		{"action": "shell", "resource": "x", "effect": "deny"},
		{"action": "shell", "resource": "x", "effect": "allow"},
	}
	merged = MergeRulesByPrecedence(withDupes, nil)
	if got := resolve(merged, "shell", "x"); got != "allow" {
		t.Fatalf("duplicate-triple sequence collapsed: effective = %q, want allow", got)
	}
	if len(merged) != 3 {
		t.Fatalf("existing rules were dropped: %d rules, want 3", len(merged))
	}

	// Repeated merges are idempotent: identical triples are skipped, and the
	// effective decisions do not drift.
	once := MergeRulesByPrecedence(user, overlay)
	twice := MergeRulesByPrecedence(once, overlay)
	if len(twice) != len(once) {
		t.Fatalf("second merge changed the rule count: %d -> %d", len(once), len(twice))
	}
	for i := range once {
		if resolve(twice, "shell", "git push origin") != resolve(once, "shell", "git push origin") {
			t.Fatalf("second merge drifted at rule %d", i)
		}
	}
}
