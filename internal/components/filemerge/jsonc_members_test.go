package filemerge

import (
	"slices"
	"testing"
)

func TestRemoveJSONCMembersEditsOnlyTheRemovedMembers(t *testing.T) {
	raw := `// keep: header
{
  "$schema": "https://opencode.ai/config.json", // keep: schema
  "agent": {
    "sdd-apply": {"mode": "subagent"},
    /* keep: mine */
    "mine": {"mode": "primary"},
    "sdd-verify": {
      "mode": "subagent"
    }
  },
  "agents": {
    "sdd-apply": {"system": "x"},
    "explore": {"system": "y"},
  },
  "theme": "dark"
}
`
	got, kept, err := RemoveJSONCMembers([]byte(raw), []string{"agent"}, []string{"sdd-apply", "sdd-verify", "absent"}, false)
	if err != nil || len(kept) != 0 {
		t.Fatalf("RemoveJSONCMembers() kept = %v, err = %v", kept, err)
	}
	want := `// keep: header
{
  "$schema": "https://opencode.ai/config.json", // keep: schema
  "agent": {
    /* keep: mine */
    "mine": {"mode": "primary"}
  },
  "agents": {
    "sdd-apply": {"system": "x"},
    "explore": {"system": "y"},
  },
  "theme": "dark"
}
`
	if string(got) != want {
		t.Fatalf("agent members:\n%s\nwant:\n%s", got, want)
	}

	got, kept, err = RemoveJSONCMembers(got, []string{"agents"}, []string{"sdd-apply", "explore"}, true)
	if err != nil || len(kept) != 0 {
		t.Fatalf("drop empty kept = %v, err = %v", kept, err)
	}
	want = `// keep: header
{
  "$schema": "https://opencode.ai/config.json", // keep: schema
  "agent": {
    /* keep: mine */
    "mine": {"mode": "primary"}
  },
  "theme": "dark"
}
`
	if string(got) != want {
		t.Fatalf("emptied agents:\n%s\nwant:\n%s", got, want)
	}
	if _, err := UnmarshalJSONObject(got); err != nil {
		t.Fatalf("result does not parse: %v", err)
	}
	again, kept, err := RemoveJSONCMembers(got, []string{"agents"}, []string{"sdd-apply"}, true)
	if err != nil || len(kept) != 0 || string(again) != string(got) {
		t.Fatalf("missing path changed the document: %v, %v", kept, err)
	}
}

func TestRemoveJSONCMembersNestedPathAndCompactJSON(t *testing.T) {
	raw := `{"agent":{"gentle-orchestrator":{"permission":{"task":{"*":"deny","sdd-apply":"allow","jd-judge-a":"allow","sdd-verify":"allow"}}}}}`
	got, kept, err := RemoveJSONCMembers([]byte(raw), []string{"agent", "gentle-orchestrator", "permission", "task"}, []string{"sdd-apply", "sdd-verify"}, false)
	if err != nil || len(kept) != 0 {
		t.Fatalf("kept = %v, err = %v", kept, err)
	}
	if want := `{"agent":{"gentle-orchestrator":{"permission":{"task":{"*":"deny","jd-judge-a":"allow"}}}}}`; string(got) != want {
		t.Fatalf("nested removal = %s", got)
	}
	got, _, err = RemoveJSONCMembers([]byte(`{"agents":{"only":{}}, "x": 1}`), []string{"agents"}, []string{"only"}, true)
	if err != nil || string(got) != `{"x": 1}` {
		t.Fatalf("drop first member = %s, %v", got, err)
	}
	got, _, err = RemoveJSONCMembers([]byte(`{"x": 1, "agents":{"only":{}}}`), []string{"agents"}, []string{"only"}, true)
	if err != nil || string(got) != `{"x": 1}` {
		t.Fatalf("drop last member = %s, %v", got, err)
	}
}

// A comment on its own line after the last member stays, and the preceding
// comma stays too, so the comment never ends up nested in another value.
func TestRemoveJSONCMembersLastMemberBeforeClosingComment(t *testing.T) {
	raw := "{\n  \"agent\": {\"mine\": {}},\n  \"agents\": {\n    \"sdd-apply\": {}\n  }\n  // closing note\n}\n"
	got, kept, err := RemoveJSONCMembers([]byte(raw), []string{"agents"}, []string{"sdd-apply"}, true)
	if err != nil || len(kept) != 0 {
		t.Fatalf("kept = %v, err = %v", kept, err)
	}
	if want := "{\n  \"agent\": {\"mine\": {}},\n  // closing note\n}\n"; string(got) != want {
		t.Fatalf("removal = %q, want %q", got, want)
	}
	if JSONCTopLevelValueHasComments(got, "agent") {
		t.Fatal("the closing comment became nested in agent")
	}
	if _, err := UnmarshalJSONObject(got); err != nil {
		t.Fatalf("result does not parse: %v", err)
	}
}

func TestRemoveJSONCMembersKeepsCommentedMembersAndRefusesDuplicates(t *testing.T) {
	raw := `{
  "agent": {
    // my notes about this agent
    "sdd-apply": {"mode": "subagent"},
    "sdd-spec": {"mode": "subagent" /* tuned */},
    "sdd-tasks": {"mode": "subagent"}, // why I keep it
    "sdd-\u0076erify": {"mode": "subagent"},
    "sdd-design": {"mode": "subagent"}
  }
}
`
	got, kept, err := RemoveJSONCMembers([]byte(raw), []string{"agent"}, []string{"sdd-apply", "sdd-spec", "sdd-tasks", "sdd-verify", "sdd-design"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"sdd-apply", "sdd-spec", "sdd-tasks", "sdd-verify"}; !slices.Equal(kept, want) {
		t.Fatalf("kept = %v, want %v", kept, want)
	}
	root, err := UnmarshalJSONObject(got)
	if err != nil {
		t.Fatal(err)
	}
	agent := root["agent"].(map[string]any)
	if _, ok := agent["sdd-design"]; ok || len(agent) != 4 {
		t.Fatalf("remaining agents = %v", agent)
	}

	duplicate := []byte(`{"agent":{"sdd-apply":{}},"agent":{}}`)
	if out, _, err := RemoveJSONCMembers(duplicate, []string{"agent"}, []string{"sdd-apply"}, false); err == nil || string(out) != string(duplicate) {
		t.Fatalf("duplicate keys accepted: %s, %v", out, err)
	}
}
