package legacyassets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
)

// releasedOpenCodeAgents reads the agent map of a real v3.7.0 OpenCode render
// (multi mode plus a "fallback" profile; see testdata/v3.7.0): "v1" is the
// `agent` map, "v2" the native `agents` map. Orchestrator and reviewer prompts
// are trimmed to their opening lines; sdd-* phase entries are as rendered.
func releasedOpenCodeAgents(t *testing.T, shape string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "v3.7.0", "opencode-"+shape+".json"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := filemerge.UnmarshalJSONObject(data)
	if err != nil {
		t.Fatal(err)
	}
	if shape == "v2" {
		return root["agents"].(map[string]any)
	}
	return root["agent"].(map[string]any)
}

func withoutMarker(entry map[string]any) map[string]any {
	copied := map[string]any{}
	for key, value := range entry {
		if key != "__managed_by" {
			copied[key] = value
		}
	}
	return copied
}

func TestOwnsOpenCodeSDDAgentProvesEveryReleasedShape(t *testing.T) {
	for _, shape := range []string{"v1", "v2"} {
		retired := 0
		for name, raw := range releasedOpenCodeAgents(t, shape) {
			entry := raw.(map[string]any)
			if _, _, ok := RetiredOpenCodeSDDAgent(name); !ok {
				if strings.HasPrefix(name, "sdd-") {
					t.Errorf("%s: %s not recognized as a retired SDD agent", shape, name)
				}
				continue
			}
			retired++
			if !OwnsOpenCodeSDDAgent(name, entry) {
				t.Errorf("%s: released %s not owned", shape, name)
			}
			// Profile entries never carried the marker; overlay entries
			// must be provable once a user (or main) dropped it.
			if !OwnsOpenCodeSDDAgent(name, withoutMarker(entry)) {
				t.Errorf("%s: released %s not owned by its shape alone", shape, name)
			}
		}
		if retired != 23 {
			t.Errorf("%s: %d retired SDD entries in the render, want 23", shape, retired)
		}
	}
}

func TestOwnsOpenCodeSDDAgentRejectsUserShapes(t *testing.T) {
	agents := releasedOpenCodeAgents(t, "v1")
	edited := func(name string, change func(map[string]any)) map[string]any {
		entry := withoutMarker(agents[name].(map[string]any))
		change(entry)
		return entry
	}
	for _, tc := range []struct {
		name, agent string
		entry       map[string]any
	}{
		{"own description", "sdd-init", map[string]any{"mode": "subagent", "hidden": true, "description": "My own init", "prompt": "{file:./prompts/sdd/sdd-init.md}"}},
		{"edited inline prompt", "sdd-research", edited("sdd-research", func(e map[string]any) { e["prompt"] = e["prompt"].(string) + "\nAlso check my notes." })},
		{"prompt file of another phase", "sdd-apply", edited("sdd-apply", func(e map[string]any) { e["prompt"] = "{file:./prompts/sdd/sdd-verify.md}" })},
		{"extra field", "sdd-apply", edited("sdd-apply", func(e map[string]any) { e["steps"] = 40 })},
		{"visible phase", "sdd-apply", edited("sdd-apply", func(e map[string]any) { delete(e, "hidden") })},
		{"primary phase", "sdd-apply-fallback", edited("sdd-apply-fallback", func(e map[string]any) { e["mode"] = "primary" })},
		{"other profile in description", "sdd-orchestrator-fallback", edited("sdd-orchestrator-fallback", func(e map[string]any) {
			e["description"] = "SDD Orchestrator (premium profile) - coordinates sub-agents, never does work inline"
		})},
		{"own orchestrator prompt", "sdd-orchestrator-fallback", edited("sdd-orchestrator-fallback", func(e map[string]any) { e["prompt"] = "# My orchestrator\n" })},
		{"widened delegation", "sdd-orchestrator-fallback", edited("sdd-orchestrator-fallback", func(e map[string]any) {
			e["permission"] = map[string]any{"question": "allow", "task": map[string]any{"my-agent": "allow"}}
		})},
		{"other marker", "sdd-apply", edited("sdd-apply", func(e map[string]any) { e["__managed_by"] = "someone-else"; e["description"] = "mine" })},
	} {
		if OwnsOpenCodeSDDAgent(tc.agent, tc.entry) {
			t.Errorf("%s: %s owned", tc.name, tc.agent)
		}
	}
	for _, name := range []string{"sdd-applyx", "sdd-apply-Bad", "sdd-apply-", "sdd-custom", "gentle-orchestrator"} {
		if _, _, ok := RetiredOpenCodeSDDAgent(name); ok {
			t.Errorf("%s recognized as a retired SDD agent", name)
		}
	}
	// v1.17.0's inline prompt file content and v1.5.0's sdd-orchestrator.
	legacy := map[string]any{"mode": "subagent", "hidden": true, "description": "Bootstrap SDD context and project configuration", "prompt": "You are an SDD sub-agent for the init phase. Your skill file is at ~/.config/opencode/skills/sdd-init/SKILL.md — read it and follow its instructions.", "tools": map[string]any{"read": true, "write": true, "edit": true, "bash": true}}
	if !OwnsOpenCodeSDDAgent("sdd-init", legacy) {
		t.Error("v1.5.0 sdd-init not owned")
	}
	orchestrator := map[string]any{"mode": "all", "description": "Gentleman personality + SDD delegate-only orchestrator", "prompt": "{file:./AGENTS.md}", "tools": map[string]any{"read": true, "write": true, "edit": true, "bash": true}}
	if !OwnsOpenCodeSDDAgent("sdd-orchestrator", orchestrator) {
		t.Error("v1.5.0 sdd-orchestrator not owned")
	}
}

func TestRetireOpenCodeSDDSettingsBothShapes(t *testing.T) {
	v1, v2 := releasedOpenCodeAgents(t, "v1"), releasedOpenCodeAgents(t, "v2")
	agent := map[string]any{
		"sdd-apply":                 v1["sdd-apply"],
		"sdd-research-fallback":     v1["sdd-research-fallback"],
		"sdd-orchestrator-fallback": v1["sdd-orchestrator-fallback"],
		"sdd-init":                  map[string]any{"mode": "subagent", "prompt": "Mine."},
		// main stripped this marker before; its shape still proves it.
		"sdd-verify": withoutMarker(v1["sdd-verify"].(map[string]any)),
		"gentle-orchestrator": map[string]any{"mode": "primary", "permission": map[string]any{"task": map[string]any{
			"*": "deny", "sdd-apply": "allow", "sdd-verify": "allow", "sdd-init": "allow", "sdd-*": "allow", "jd-judge-a": "allow",
		}}},
	}
	agents := map[string]any{
		"gentle-orchestrator": v2["gentle-orchestrator"],
		"sdd-apply":           v2["sdd-apply"],
		"sdd-apply-fallback":  v2["sdd-apply-fallback"],
	}
	encode := func(value any) string {
		data, err := json.MarshalIndent(value, "  ", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	// Not Gentle-shaped: sdd-spec never had this description.
	userSpec := `{"mode": "subagent", "hidden": true, "description": "Validate implementation against specs", "prompt": "{file:./prompts/sdd/sdd-spec.md}"}`
	document := "// my settings\n{\n  \"theme\": \"dark\", // keep\n  \"agent\": " + encode(agent)[:len(encode(agent))-1] +
		"  // I still read this one\n    ,\"sdd-spec\": " + userSpec + "\n  },\n  \"agents\": " + encode(agents) + "\n  /* closing note */\n}\n"
	path := filepath.Join(t.TempDir(), "opencode.jsonc")
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := RetireOpenCodeSDDSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	wantRemoved := []string{"agent.sdd-apply", "agent.sdd-orchestrator-fallback", "agent.sdd-research-fallback", "agent.sdd-verify", "agents.gentle-orchestrator", "agents.sdd-apply", "agents.sdd-apply-fallback"}
	if !result.Changed || !slices.Equal(result.Removed, wantRemoved) {
		t.Fatalf("removed = %v (changed %v), want %v", result.Removed, result.Changed, wantRemoved)
	}
	if !slices.Equal(result.Preserved, []string{"agent.sdd-init", "agent.sdd-spec"}) || len(result.Commented) != 0 {
		t.Fatalf("preserved = %v, commented = %v", result.Preserved, result.Commented)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, comment := range []string{"// my settings", "// keep", "// I still read this one", "/* closing note */"} {
		if !strings.Contains(string(data), comment) {
			t.Errorf("comment %q lost:\n%s", comment, data)
		}
	}
	root, err := filemerge.UnmarshalJSONObject(data)
	if err != nil {
		t.Fatalf("result does not parse: %v\n%s", err, data)
	}
	if _, ok := root["agents"]; ok {
		t.Error("emptied native agents map kept")
	}
	got := root["agent"].(map[string]any)
	if _, ok := got["sdd-spec"]; !ok {
		t.Error("user-shaped agent.sdd-spec removed")
	}
	task := got["gentle-orchestrator"].(map[string]any)["permission"].(map[string]any)["task"].(map[string]any)
	// Allowlist entries go only with the retired agent they named.
	for target, want := range map[string]bool{"sdd-apply": false, "sdd-verify": false, "sdd-*": false, "sdd-init": true, "jd-judge-a": true, "*": true} {
		if _, ok := task[target]; ok != want {
			t.Errorf("task[%s] present = %v, want %v", target, ok, want)
		}
	}
	if !hasAction(result.ManualActions(), path, "agent.sdd-init", "move or delete it") {
		t.Errorf("preserved agent not reported: %v", result.ManualActions())
	}

	again, err := RetireOpenCodeSDDSettings(path)
	if err != nil || again.Changed || len(again.Removed) != 0 {
		t.Fatalf("second run = %+v, %v", again, err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(data) {
		t.Fatal("second run changed the settings")
	}
}

// An owned entry with comments attached is kept with its marker and reported.
func TestRetireOpenCodeSDDSettingsKeepsCommentedOwnedEntry(t *testing.T) {
	entry, err := json.Marshal(releasedOpenCodeAgents(t, "v1")["sdd-apply"])
	if err != nil {
		t.Fatal(err)
	}
	document := "{\n  \"agent\": {\n    // tuned for my repo\n    \"sdd-apply\": " + string(entry) + "\n  }\n}\n"
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := RetireOpenCodeSDDSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed || !slices.Equal(result.Commented, []string{"agent.sdd-apply"}) {
		t.Fatalf("result = %+v", result)
	}
	if !hasAction(result.ManualActions(), "agent.sdd-apply", "comments are attached", "move or delete it") {
		t.Fatalf("commented entry not reported: %v", result.ManualActions())
	}
	if data, _ := os.ReadFile(path); string(data) != document {
		t.Fatal("commented entry was rewritten")
	}
}

func TestRetireOpenCodeSDDSettingsMissingAndNonRegular(t *testing.T) {
	dir := t.TempDir()
	if result, err := RetireOpenCodeSDDSettings(filepath.Join(dir, "absent.json")); err != nil || result.Changed || len(result.ManualActions()) != 0 {
		t.Fatalf("missing settings = %+v, %v", result, err)
	}
	target := filepath.Join(dir, "real.json")
	document := []byte(`{"agent":{"sdd-apply":{"__managed_by":"gentle-ai/sdd"}}}`)
	if err := os.WriteFile(target, document, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "opencode.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	result, err := RetireOpenCodeSDDSettings(link)
	if err != nil || result.Changed || !hasAction(result.ManualActions(), link, "not a regular file") {
		t.Fatalf("symlinked settings = %+v, %v", result, err)
	}
	if data, _ := os.ReadFile(target); string(data) != string(document) {
		t.Fatal("symlink target changed")
	}
}

func TestRetireOpenCodeSDDPromptsByShippedBytes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "prompts", "sdd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	render := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join("testdata", "v3.7.0", "opencode-prompt-"+name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	apply := write("sdd-apply.md", render("sdd-apply.md"))
	// CRLF and a different CodeGraph guidance are render differences.
	research := write("sdd-research.md", []byte(strings.ReplaceAll(strings.Replace(string(render("sdd-research.md")), "Use CodeGraph first.", "Prefer CodeGraph.", 1), "\n", "\r\n")))
	// v1.17.0 rendered prompt files from inline Go strings.
	legacy := write("sdd-init.md", []byte("You are an SDD executor for the init phase, not the orchestrator. Do this phase's work yourself. Do NOT delegate, Do NOT call task/delegate, and Do NOT launch sub-agents. Read your skill file at ~/.config/opencode/skills/sdd-init/SKILL.md and follow it exactly."))
	edited := write("sdd-verify.md", append(render("sdd-apply.md"), "My extra rule.\n"...))
	unrelated := write("notes.md", []byte("mine"))

	result, err := RetireOpenCodeSDDPrompts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{legacy, research, apply}; !slices.Equal(sortedCopy(result.Removed), sortedCopy(want)) {
		t.Fatalf("removed = %v, want %v", result.Removed, want)
	}
	if !slices.Equal(result.Preserved, []string{edited}) || !hasAction(result.ManualActions(), edited, "move or delete it") {
		t.Fatalf("preserved = %v, actions = %v", result.Preserved, result.ManualActions())
	}
	for _, path := range []string{edited, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s removed: %v", path, err)
		}
	}

	only := filepath.Join(t.TempDir(), "sdd")
	if err := os.MkdirAll(only, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(only, "sdd-apply.md"), render("sdd-apply.md"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RetireOpenCodeSDDPrompts(only); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(only); !os.IsNotExist(err) {
		t.Fatalf("emptied prompt directory kept: %v", err)
	}
}

func sortedCopy(values []string) []string {
	copied := append([]string(nil), values...)
	slices.Sort(copied)
	return copied
}

func hasAction(actions []string, parts ...string) bool {
	return slices.ContainsFunc(actions, func(action string) bool {
		for _, part := range parts {
			if !strings.Contains(action, part) {
				return false
			}
		}
		return true
	})
}
