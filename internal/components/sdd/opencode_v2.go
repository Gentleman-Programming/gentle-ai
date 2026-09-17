package sdd

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// nativeAgentOverlay converts only generated overlay entries, never user data.
// Other top-level V1 fields remain supported by OpenCode V2.
func nativeAgentOverlay(overlay []byte, base map[string]any) ([]byte, error) {
	root, err := filemerge.UnmarshalJSONObject(overlay)
	if err != nil {
		return nil, err
	}
	agents, ok := root["agent"].(map[string]any)
	if !ok {
		return overlay, nil
	}
	for name, raw := range agents {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid generated agent %q", name)
		}
		for old, next := range map[string]string{"prompt": "system", "disable": "disabled", "maxSteps": "steps"} {
			if value, exists := entry[old]; exists {
				entry[next] = value
				delete(entry, old)
			}
		}
		if reference, exists := entry["model"]; exists {
			assignment, valid := model.ParseModelReference(reference)
			if !valid {
				return nil, fmt.Errorf("invalid generated model for %q", name)
			}
			if effort, ok := entry["variant"].(string); ok {
				assignment.Effort = effort
			}
			selection := assignment.FullID()
			if assignment.Effort != "" {
				selection += "#" + assignment.Effort
			}
			entry["model"] = selection
		}
		delete(entry, "variant")
		if permission, exists := entry["permission"]; exists {
			rules, err := nativePermissionRules(permission)
			if err != nil {
				return nil, fmt.Errorf("agent %q: %w", name, err)
			}
			native, _ := base["agents"].(map[string]any)
			existing, _ := native[name].(map[string]any)
			previous, _ := existing["permissions"].([]any)

			combined, err := mergeNativePermissionRules(previous, rules, permission)
			if err != nil {
				return nil, fmt.Errorf("agent %q: %w", name, err)
			}

			entry["permissions"] = map[string]any{"__replace__": combined}
			delete(entry, "permission")
		}
		// OpenCode generated agents already express access through permissions.
		if _, exists := entry["tools"]; exists {
			return nil, fmt.Errorf("generated agent %q has unsupported legacy tools", name)
		}
	}
	root["agents"] = agents
	delete(root, "agent")
	return json.Marshal(root)
}

func nativePermissionRules(value any) ([]any, error) {
	permissions, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unsupported permission shape")
	}
	keys := make([]string, 0, len(permissions))
	for key := range permissions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rules := []any{}
	for _, key := range keys {
		action := nativePermissionAction(key)
		appendRule := func(resource string, effect any) error {
			e, ok := effect.(string)
			if !ok || (e != "allow" && e != "ask" && e != "deny") {
				return fmt.Errorf("unsupported permission effect")
			}
			rules = append(rules, map[string]any{"action": action, "resource": resource, "effect": e})
			return nil
		}
		switch value := permissions[key].(type) {
		case string:
			if err := appendRule("*", value); err != nil {
				return nil, err
			}
		case map[string]any:
			if replacement, ok := value["__replace__"].(map[string]any); ok {
				value = replacement
			}
			resources := make([]string, 0, len(value))
			for resource := range value {
				resources = append(resources, resource)
			}
			sort.Strings(resources)
			for _, resource := range resources {
				if err := appendRule(resource, value[resource]); err != nil {
					return nil, err
				}
			}
		default:
			return nil, fmt.Errorf("unsupported permission value")
		}
	}
	return rules, nil
}

// Match V1 merge ownership: generated defaults precede existing exceptions;
// explicit replacement/scalar-deny actions retire stale per-resource allowances.
// Universal deny/ask remains protected, like V1 scalar restrictions.
func mergeNativePermissionRules(previous, generated []any, permission any) ([]any, error) {
	owned, scalarDeny := map[string]bool{}, map[string]bool{}
	permissions, _ := permission.(map[string]any)
	for action, value := range permissions {
		native := nativePermissionAction(action)
		entry, _ := value.(map[string]any)
		_, replace := entry["__replace__"]
		scalarDeny[native] = scalarDeny[native] || value == "deny"
		owned[native] = owned[native] || replace || value == "deny"
	}
	counts := map[string]int{}
	for _, raw := range previous {
		rule, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid existing permission rule")
		}
		action, aok := rule["action"].(string)
		_, rok := rule["resource"].(string)
		effect, eok := rule["effect"].(string)
		if !aok || !rok || !eok || (effect != "allow" && effect != "ask" && effect != "deny") {
			return nil, fmt.Errorf("invalid existing permission rule")
		}
		counts[action]++
	}
	protected := map[string]bool{}
	for _, raw := range previous {
		rule := raw.(map[string]any)
		action := rule["action"].(string)
		if counts[action] == 1 && rule["resource"] == "*" && (rule["effect"] == "deny" || (rule["effect"] == "ask" && !scalarDeny[action])) {
			protected[action] = true
		}
	}
	// Preserve existing order; exact-key updates cannot loosen deny/ask.
	retained, leading := []any{}, []any{}
	existing := map[string]bool{}
	seenAction := map[string]bool{}
	for _, raw := range previous {
		rule := raw.(map[string]any)
		action := rule["action"].(string)
		resource := rule["resource"].(string)
		if owned[action] && !protected[action] && rule["effect"] == "allow" {
			// Ownership retires stale allowances, never user restrictions. Keep
			// matching generated exceptions in their original last-match order;
			// moving a wildcard deny after them would incorrectly revoke them.
			matched := false
			for _, rawNew := range generated {
				next := rawNew.(map[string]any)
				if next["action"] == action && next["resource"] == resource {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		copy := map[string]any{"action": action, "resource": resource, "effect": rule["effect"]}
		for _, rawNew := range generated {
			next := rawNew.(map[string]any)
			if next["action"] == action && next["resource"] == resource && (copy["effect"] == "allow" || (copy["effect"] == "ask" && next["effect"] == "deny")) {
				copy["effect"] = next["effect"]
			}
		}
		existing[action+"\x00"+resource] = true
		if !seenAction[action] && resource == "*" && copy["effect"] == "allow" {
			leading = append(leading, copy)
		} else {
			retained = append(retained, copy)
		}
		seenAction[action] = true
	}
	combined := leading
	for _, raw := range generated {
		rule := raw.(map[string]any)
		action := rule["action"].(string)
		resource := rule["resource"].(string)
		key := action + "\x00" + resource
		if !protected[action] && !existing[key] {
			combined = append(combined, raw)
			existing[key] = true
		}
	}
	return append(combined, retained...), nil
}

func nativePermissionAction(action string) string {
	switch action {
	case "bash":
		return "shell"
	case "task":
		return "subagent"
	case "write", "patch":
		return "edit"
	default:
		return action
	}
}
