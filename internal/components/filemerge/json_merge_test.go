package filemerge

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONCTouchedValueCommentGuard(t *testing.T) {
	for _, tc := range []struct {
		name, base, overlay string
		refuse              bool
	}{
		{"permission nested line", `{"permission":{"bash":{// note
"*":"deny"}}}`, `{"permission":{"read":{"*":"allow"}}}`, true},
		{"mcp nested block", `{"mcp":{"other":{/* keep */"type":"remote"}}}`, `{"mcp":{"context7":{"type":"remote"}}}`, true},
		{"untouched subtree", `{"agent":{/* keep */"custom":{}},"mcp":{}}`, `{"mcp":{"context7":{}}}`, false},
		{"comment in string", `{"mcp":{"other":{"url":"https://example.com"}}}`, `{"mcp":{"context7":{}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := MergeJSONObjectsForPath("opencode.jsonc", []byte(tc.base), []byte(tc.overlay))
			if (err != nil) != tc.refuse {
				t.Fatalf("merge error = %v, want refusal %t", err, tc.refuse)
			}
		})
	}
}

func TestJSONCMergesRefuseDuplicateKeysBeforeMapRewrite(t *testing.T) {
	for _, tc := range []struct {
		name, base, overlay string
	}{
		{"nested permission", `{"permission":{"bash":{"ssh":"deny","ssh":"allow"}}}`, `{"permission":{"bash":{"*":"ask"}}}`},
		{"nested mcp", `{"mcp":{"other":{"type":"local","type":"remote"}}}`, `{"mcp":{"context7":{"type":"remote"}}}`},
		{"escaped touched top-level duplicate", `{"mcp":{},"m\u0063p":{"other":true}}`, `{"mcp":{"context7":{}}}`},
		{"escaped touched top-level only", `{"m\u0063p":{"other":true}}`, `{"mcp":{"context7":{}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := []byte(tc.base)
			for _, merge := range []struct {
				name string
				fn   func(string, []byte, []byte) ([]byte, error)
			}{
				{"overlay", MergeJSONObjectsForPath},
				{"defaults", MergeOpenCodeJSONDefaultsForPath},
			} {
				t.Run(merge.name, func(t *testing.T) {
					got, err := merge.fn("opencode.jsonc", base, []byte(tc.overlay))
					if err == nil || !strings.Contains(err.Error(), "refuse") {
						t.Fatalf("want actionable refusal; got %q, %v", got, err)
					}
					if got != nil && string(got) != string(base) {
						t.Fatalf("refusal changed bytes: %q", got)
					}
				})
			}
		})
	}
}

func TestRemoveLegacyOpenCodeAgentMarkers(t *testing.T) {
	for _, path := range []string{"opencode.json", "opencode.jsonc"} {
		t.Run(path, func(t *testing.T) {
			raw := []byte(`{"agent":{"gentle-orchestrator":{"__managed_by":"gentle-ai/sdd","prompt":"keep"},"custom":{"__managed_by":"gentle-ai/sdd"},"sdd-apply":{"__managed_by":"other"}},"theme":"keep"}`)
			got, err := RemoveLegacyOpenCodeAgentMarkers(path, raw, []string{"gentle-orchestrator", "sdd-apply"})
			if err != nil {
				t.Fatal(err)
			}
			root, err := UnmarshalJSONObject(got)
			if err != nil {
				t.Fatal(err)
			}
			agents := root["agent"].(map[string]any)
			if _, ok := agents["gentle-orchestrator"].(map[string]any)["__managed_by"]; ok {
				t.Fatalf("marker retained: %s", got)
			}
			if agents["gentle-orchestrator"].(map[string]any)["prompt"] != "keep" || agents["custom"].(map[string]any)["__managed_by"] != "gentle-ai/sdd" || agents["sdd-apply"].(map[string]any)["__managed_by"] != "other" {
				t.Fatalf("user data changed: %s", got)
			}
			again, err := RemoveLegacyOpenCodeAgentMarkers(path, got, []string{"gentle-orchestrator", "sdd-apply"})
			if err != nil || string(again) != string(got) {
				t.Fatalf("not idempotent: %s %v", again, err)
			}
			if path == "opencode.jsonc" && !strings.Contains(string(got), `"theme":"keep"`) {
				t.Fatal("lost unrelated formatting")
			}
		})
	}
}

func TestRemoveLegacyOpenCodeAgentMarkersJSONCCommentsAndRefusals(t *testing.T) {
	raw := []byte(`{
 // outside
 "agent": {
   "gentle-orchestrator": {
     // retain prompt note
     "prompt": "keep",
     "__managed_by": "gentle-ai/sdd",
   },
   "custom": {"__managed_by": "gentle-ai/sdd"},
 },
}`)
	got, err := RemoveLegacyOpenCodeAgentMarkers("opencode.jsonc", raw, []string{"gentle-orchestrator"})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"// outside", "// retain prompt note", `"custom": {"__managed_by": "gentle-ai/sdd"}`} {
		if !strings.Contains(string(got), part) {
			t.Fatalf("lost %q: %s", part, got)
		}
	}
	if _, err := UnmarshalJSONObject(got); err != nil {
		t.Fatalf("invalid output: %v", err)
	}
	for _, bad := range []string{`{"agent":`, `{"agent":{"gentle-orchestrator":{"__managed_by":"gentle-ai/sdd"}},"agent":{"gentle-orchestrator":{"__managed_by":"gentle-ai/sdd"}}}`} {
		result, err := RemoveLegacyOpenCodeAgentMarkers("opencode.jsonc", []byte(bad), []string{"gentle-orchestrator"})
		if err == nil || string(result) != bad {
			t.Fatalf("unsafe rewrite of %q: %s %v", bad, result, err)
		}
	}
}

func TestRemoveLegacyOpenCodeAgentMarkersPreservesInlineCommentByRefusal(t *testing.T) {
	for _, raw := range []string{
		`{"agent":{"gentle-orchestrator":{"__managed_by" /* user note */ : "gentle-ai/sdd","prompt":"keep"}}}`,
		`{"agent":{"gentle-orchestrator":{"__managed_by" // user note
 : "gentle-ai/sdd","prompt":"keep"}}}`,
		`{"agent":{"gentle-orchestrator":{/* user note */ "__managed_by":"gentle-ai/sdd","prompt":"keep"}}}`,
		`{"agent":{"gentle-orchestrator":{"__managed_by":"gentle-ai/sdd" /* user's note */,"prompt":"keep"}}}`,
		`{"agent":{"gentle-orchestrator":{"__managed_by":"gentle-ai/sdd" // user's note
  ,"prompt":"keep"}}}`,
	} {
		got, err := RemoveLegacyOpenCodeAgentMarkers("opencode.jsonc", []byte(raw), []string{"gentle-orchestrator"})
		if err == nil || string(got) != raw {
			t.Fatalf("inline comment must be preserved by refusal: %s, %v", got, err)
		}
	}
}

func TestRemoveLegacyOpenCodeAgentMarkersAcceptsEmptySettings(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		raw  []byte
	}{
		{name: "zero bytes", path: "opencode.json", raw: nil},
		{name: "whitespace", path: "opencode.json", raw: []byte(" \t\r\n ")},
		{name: "whitespace JSONC", path: "opencode.jsonc", raw: []byte(" \n ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RemoveLegacyOpenCodeAgentMarkers(tc.path, tc.raw, []string{"gentle-orchestrator"})
			if err != nil || string(got) != string(tc.raw) {
				t.Fatalf("empty settings must remain unchanged without error: got %q, err %v", got, err)
			}
		})
	}
}

func TestRemoveLegacyOpenCodeAgentMarkersRejectsCommentOnlyJSONC(t *testing.T) {
	raw := []byte("// user note\n")
	got, err := RemoveLegacyOpenCodeAgentMarkers("opencode.jsonc", raw, []string{"gentle-orchestrator"})
	if err == nil || string(got) != string(raw) {
		t.Fatalf("comment-only settings must be refused unchanged: got %q, err %v", got, err)
	}
}

func TestRemoveLegacyOpenCodeAgentMarkersRejectsDuplicateKeys(t *testing.T) {
	cases := []string{
		`{"agent":{"gentle-orchestrator":{"__managed_by":"gentle-ai/sdd"}},"agent":{"custom":true}}`,
		`{"agent":{"gentle-orchestrator":{"__managed_by":"gentle-ai/sdd"},"gentle-orchestrator":{"prompt":"user"}}}`,
		`{"agent":{"gentle-orchestrator":{"__managed_by":"gentle-ai/sdd","__managed_by":"other"}}}`,
		`{"theme":1,"theme":2,"agent":{"gentle-orchestrator":{"__managed_by":"gentle-ai/sdd"}}}`,
	}
	for _, path := range []string{"opencode.json", "opencode.jsonc"} {
		for _, raw := range cases {
			t.Run(path+raw, func(t *testing.T) {
				got, err := RemoveLegacyOpenCodeAgentMarkers(path, []byte(raw), []string{"gentle-orchestrator"})
				if err == nil || string(got) != raw {
					t.Fatalf("duplicate key must fail closed: %s %v", got, err)
				}
			})
		}
	}
}

func TestPermissionOverlayNewWildcardCannotOverrideExistingDeny(t *testing.T) {
	got, err := MergeJSONObjects([]byte(`{"permission":{"bash":{"ssh*":"deny"}}}`), []byte(`{"permission":{"bash":{"*":"allow"}}}`))
	if err != nil || !strings.Contains(strings.Join(strings.Fields(string(got)), ""), `"bash":{"*":"allow","ssh*":"deny"}`) {
		t.Fatalf("new wildcard changed the effective SSH deny: %s, %v", got, err)
	}
}

func TestPermissionOverlayCannotReplaceRestrictionsWithScalarAllow(t *testing.T) {
	for _, base := range []string{`{"permission":{"bash":{"ssh *":"deny"}}}`, `{"agent":{"custom":{"permission":{"bash":"deny"}}}}`} {
		got, err := MergeJSONObjects([]byte(base), []byte(`{"permission":"allow","agent":{"custom":{"permission":"allow"}}}`))
		if err != nil || !strings.Contains(string(got), `"deny"`) {
			t.Fatalf("scalar overlay erased a restriction: %s, %v", got, err)
		}
	}
}

func TestPermissionDefaultsJSONCAndRestrictiveAgent(t *testing.T) {
	base := []byte("// personal settings\n" + `{"permission":{"bash":{"*":"allow","ssh*":"deny"}},"agent":{"custom":{"permission":"deny"}}}`)
	got, err := MergeJSONDefaultsForPath("opencode.jsonc", base, []byte(`{"permission":{"bash":{"*":"allow","ssh *":"ask"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "// personal settings") {
		t.Fatal("defaults lost JSONC comment")
	}
	repeated, err := MergeJSONDefaultsForPath("opencode.jsonc", got, []byte(`{"permission":{"bash":{"*":"allow","ssh *":"ask"}}}`))
	if err != nil || string(repeated) != string(got) {
		t.Fatalf("JSONC defaults are not idempotent: %v", err)
	}
	got, err = MergeJSONObjectsForPath("opencode.jsonc", got, []byte(`{"agent":{"custom":{"permission":{"bash":"allow"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	compact := strings.Join(strings.Fields(string(got)), "")
	if !strings.Contains(compact, `"custom":{"permission":"deny"}`) || !strings.Contains(compact, `"bash":{"*":"allow","ssh*":"ask","ssh*":"deny"}`) {
		t.Fatalf("cross-writer merge weakened restrictive permission: %s", got)
	}
}

func TestPermissionOrderSurvivesCrossWriterMerge(t *testing.T) {
	for _, path := range []string{"opencode.json", "opencode.jsonc"} {
		t.Run(path, func(t *testing.T) {
			base := []byte(`{"permission":{"bash":{"ssh *":"allow","*":"deny"}},"agent":{"custom":{"permission":{"bash":{"ssh *":"allow","*":"deny"}}}}}`)
			// Agent-tools retirement is another settings writer outside the
			// overlay merge; it must use the same serialization authority.
			base = []byte(strings.Replace(string(base), `"custom":{`, `"custom":{"tools":{"bash":true},`, 1))
			base, err := RemoveJSONAgentTools(base, "custom")
			if err != nil {
				t.Fatal(err)
			}
			for _, overlay := range []string{`{"agent":{"gentle-orchestrator":{"prompt":"guidance"}}}`, `{"agent":{"sdd-apply":{"permission":{}}}}`} {
				got, err := MergeJSONObjectsForPath(path, base, []byte(overlay))
				if err != nil {
					t.Fatal(err)
				}
				// Both patterns match ssh example.invalid: the final serialized
				// rule must remain deny, globally and in the custom agent.
				compact := strings.Join(strings.Fields(string(got)), "")
				if strings.Count(compact, `"bash":{"ssh*":"allow","*":"deny"}`) != 2 {
					t.Fatalf("last-match deny was reordered by %s: %s", overlay, got)
				}
				base = got
			}
		})
	}
}

func TestMergeJSONObjectsRecursively(t *testing.T) {
	base := []byte(`{"plugins":["a"],"settings":{"theme":"default","flags":{"x":true}}}`)
	overlay := []byte(`{"settings":{"theme":"gentleman","flags":{"y":true}},"extra":1}`)

	merged, err := MergeJSONObjects(base, overlay)
	if err != nil {
		t.Fatalf("MergeJSONObjects() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("Unmarshal merged json error = %v", err)
	}

	settings := got["settings"].(map[string]any)
	flags := settings["flags"].(map[string]any)

	if settings["theme"] != "gentleman" {
		t.Fatalf("theme = %v", settings["theme"])
	}

	if flags["x"] != true || flags["y"] != true {
		t.Fatalf("flags = %#v", flags)
	}

	plugins := got["plugins"].([]any)
	if len(plugins) != 1 || plugins[0] != "a" {
		t.Fatalf("plugins = %#v", plugins)
	}
}

func TestMergeJSONObjectsSupportsJSONCBase(t *testing.T) {
	base := []byte(`{
	  // VS Code-style comments and trailing commas
	  "editor.fontSize": 14,
	  "files.exclude": {
	    "**/.git": true,
	  },
	}`)
	overlay := []byte(`{"chat.tools.autoApprove": true}`)

	merged, err := MergeJSONObjects(base, overlay)
	if err != nil {
		t.Fatalf("MergeJSONObjects() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("Unmarshal merged json error = %v", err)
	}

	autoApprove, ok := got["chat.tools.autoApprove"].(bool)
	if !ok || !autoApprove {
		t.Fatalf("chat.tools.autoApprove = %#v", got["chat.tools.autoApprove"])
	}

	if got["editor.fontSize"] != float64(14) {
		t.Fatalf("editor.fontSize = %v", got["editor.fontSize"])
	}
}

func TestMergeOpenCodeJSONCObjectsKeepsCommentsAndTrailingCommas(t *testing.T) {
	base := []byte(`{
  // user provider note
  "provider": {
    "local": {"models": {"m": {},},},
  },
  // managed server note
  "mcp": {
    "engram": {"command": ["engram", "mcp"],},
  },
}`)
	overlay := []byte(`{"mcp":{"context7":{"type":"remote","enabled":true}}}`)

	merged, err := MergeOpenCodeJSONCObjects(base, overlay)
	if err != nil {
		t.Fatalf("MergeOpenCodeJSONCObjects() error = %v", err)
	}
	text := string(merged)
	for _, want := range []string{"// user provider note", `"m": {},`, "// managed server note"} {
		if !strings.Contains(text, want) {
			t.Fatalf("merged JSONC missing preserved text %q:\n%s", want, text)
		}
	}
	parsed, err := UnmarshalJSONObject(merged)
	if err != nil {
		t.Fatalf("merged JSONC no longer parses: %v\n%s", err, text)
	}
	mcp := parsed["mcp"].(map[string]any)
	if _, ok := mcp["engram"]; !ok {
		t.Fatalf("existing MCP entry was lost: %#v", mcp)
	}
	if _, ok := mcp["context7"]; !ok {
		t.Fatalf("overlay MCP entry was not added: %#v", mcp)
	}
}

func TestMergeOpenCodeJSONCObjectsInsertsMissingKeyAndUnwrapsSentinel(t *testing.T) {
	base := []byte(`{
  "provider": {"local": {}} // keep provider note
}`)
	overlay := []byte(`{"mcp":{"engram":{"__replace__":{"command":["engram","mcp"],"type":"local"}}}}`)

	merged, err := MergeOpenCodeJSONCObjects(base, overlay)
	if err != nil {
		t.Fatalf("MergeOpenCodeJSONCObjects() error = %v", err)
	}
	text := string(merged)
	if strings.Contains(text, "__replace__") {
		t.Fatalf("sentinel leaked into inserted JSONC output:\n%s", text)
	}
	if !strings.Contains(text, "// keep provider note") {
		t.Fatalf("existing trailing comment was not preserved:\n%s", text)
	}
	parsed, err := UnmarshalJSONObject(merged)
	if err != nil {
		t.Fatalf("merged JSONC no longer parses: %v\n%s", err, text)
	}
	mcp := parsed["mcp"].(map[string]any)
	engram := mcp["engram"].(map[string]any)
	if _, ok := engram["command"].([]any); !ok {
		t.Fatalf("engram command was not unwrapped into an array: %#v", engram)
	}
}

func TestMergeOpenCodeJSONCObjectsInsertionIgnoresClosingBraceComments(t *testing.T) {
	base := []byte(`{
  "provider": {"local": {}} // keep provider note
}
// comment with } after document
`)
	overlay := []byte(`{"mcp":{"context7":{"type":"remote"}}}`)

	merged, err := MergeOpenCodeJSONCObjects(base, overlay)
	if err != nil {
		t.Fatalf("MergeOpenCodeJSONCObjects() error = %v", err)
	}
	if _, err := UnmarshalJSONObject(merged); err != nil {
		t.Fatalf("merged JSONC no longer parses: %v\n%s", err, string(merged))
	}
	if !strings.Contains(string(merged), "// comment with } after document") {
		t.Fatalf("closing-brace comment was not preserved:\n%s", string(merged))
	}
}

func TestMergeOpenCodeJSONCObjectsExistingFinalMemberIdempotent(t *testing.T) {
	base := []byte("{\n  \"theme\": \"default\"  \n}\n")
	overlay := []byte(`{"theme":"gentleman"}`)

	merged, err := MergeOpenCodeJSONCObjects(base, overlay)
	if err != nil {
		t.Fatalf("MergeOpenCodeJSONCObjects() error = %v", err)
	}
	mergedAgain, err := MergeOpenCodeJSONCObjects(merged, overlay)
	if err != nil {
		t.Fatalf("MergeOpenCodeJSONCObjects() second merge error = %v", err)
	}
	if string(mergedAgain) != string(merged) {
		t.Fatalf("repeated merge changed bytes:\nfirst:\n%s\nsecond:\n%s", string(merged), string(mergedAgain))
	}
}

func TestMergeOpenCodeJSONCObjectsRejectsMalformedInputWithoutReplacingBytes(t *testing.T) {
	base := []byte("// interrupted user edit\n{\n  \"mcp\": {\n")
	overlay := []byte(`{"mcp":{"context7":{"type":"remote"}}}`)

	merged, err := MergeOpenCodeJSONCObjects(base, overlay)
	if err == nil {
		t.Fatal("MergeOpenCodeJSONCObjects() error = nil, want refusal for malformed JSONC")
	}
	if string(merged) != string(base) {
		t.Fatalf("malformed JSONC was replaced:\n got: %q\nwant: %q", merged, base)
	}
}

func TestMergeOpenCodeJSONCObjectsRejectsDuplicateTouchedTopLevelKey(t *testing.T) {
	base := []byte(`{
  "agent": {"effective": "stale"},
  "agent": {"effective": "current"}
}
`)
	overlay := []byte(`{"agent":{"managed":true}}`)

	merged, err := MergeOpenCodeJSONCObjects(base, overlay)
	if err == nil {
		t.Fatal("MergeOpenCodeJSONCObjects() error = nil, want refusal for duplicate touched top-level key")
	}
	if string(merged) != string(base) {
		t.Fatalf("duplicate-key JSONC was modified on refusal:\n got: %q\nwant: %q", merged, base)
	}
}

func TestMergeJSONObjectsMalformedBaseReturnsOverlayOnly(t *testing.T) {
	// Real user machines (e.g. Windows) may have a malformed ~/.cursor/mcp.json.
	// The installer should recover by treating the broken base as {} and continuing.
	tests := []struct {
		name    string
		base    []byte
		overlay []byte
		wantKey string
	}{
		{
			name:    "base starting with letter",
			base:    []byte(`allow: all`),
			overlay: []byte(`{"mcpServers": {"context7": {"type": "remote"}}}`),
			wantKey: "mcpServers",
		},
		{
			name:    "unclosed json object",
			base:    []byte(`{"ok": true`),
			overlay: []byte(`{"chat.tools.autoApprove": true}`),
			wantKey: "chat.tools.autoApprove",
		},
		{
			name:    "arbitrary text",
			base:    []byte(`a`),
			overlay: []byte(`{"servers": {"engram": {"command": "engram"}}}`),
			wantKey: "servers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, err := MergeJSONObjects(tt.base, tt.overlay)
			if err != nil {
				t.Fatalf("MergeJSONObjects() error = %v; want nil (malformed base should be treated as {})", err)
			}

			var got map[string]any
			if err := json.Unmarshal(merged, &got); err != nil {
				t.Fatalf("merged result is not valid JSON: %v", err)
			}

			if _, ok := got[tt.wantKey]; !ok {
				t.Fatalf("merged result missing key %q from overlay; got keys: %v", tt.wantKey, got)
			}
		})
	}
}

// ─── __replace__ sentinel tests ───────────────────────────────────────────────

func TestMergeJSONObjectsReplaceSentinelErasesBaseKeys(t *testing.T) {
	base := []byte(`{"mcp":{"engram":{"command":"/opt/homebrew/bin/engram","args":["mcp","--tools=agent"],"type":"local"}}}`)
	overlay := []byte(`{"mcp":{"engram":{"__replace__":{"command":["/opt/homebrew/bin/engram","mcp","--tools=agent"],"type":"local"}}}}`)

	merged, err := MergeJSONObjects(base, overlay)
	if err != nil {
		t.Fatalf("MergeJSONObjects() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("Unmarshal merged error = %v", err)
	}

	mcp := got["mcp"].(map[string]any)
	eng := mcp["engram"].(map[string]any)

	// args must be gone
	if _, ok := eng["args"]; ok {
		t.Fatalf("engram still has 'args' after __replace__; got: %v", eng)
	}
	// command must be an array
	cmd, ok := eng["command"].([]any)
	if !ok {
		t.Fatalf("engram command is not an array; got: %T = %v", eng["command"], eng["command"])
	}
	if len(cmd) != 3 {
		t.Fatalf("engram command has %d elements, want 3", len(cmd))
	}
	// __replace__ must not appear in output
	if _, ok := eng["__replace__"]; ok {
		t.Fatal("__replace__ sentinel leaked into output")
	}
}

func TestMergeJSONObjectsReplaceSentinelNoBaseKey(t *testing.T) {
	base := []byte(`{}`)
	overlay := []byte(`{"a":{"__replace__":{"z":3}}}`)

	merged, err := MergeJSONObjects(base, overlay)
	if err != nil {
		t.Fatalf("MergeJSONObjects() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("Unmarshal merged error = %v", err)
	}

	a, ok := got["a"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'a' to be a map; got: %T = %v", got["a"], got["a"])
	}
	if a["z"] != float64(3) {
		t.Fatalf("a.z = %v, want 3", a["z"])
	}
	if _, ok := a["__replace__"]; ok {
		t.Fatal("__replace__ sentinel leaked into output")
	}
}

func TestMergeJSONObjectsReplaceSentinelPreservesOtherKeys(t *testing.T) {
	base := []byte(`{"a":{"old":1},"b":"keep"}`)
	overlay := []byte(`{"a":{"__replace__":{"new":2}}}`)

	merged, err := MergeJSONObjects(base, overlay)
	if err != nil {
		t.Fatalf("MergeJSONObjects() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("Unmarshal merged error = %v", err)
	}

	// "b" must survive untouched
	if got["b"] != "keep" {
		t.Fatalf("sibling key 'b' lost; got: %v", got)
	}

	a := got["a"].(map[string]any)
	// "old" must be gone (replaced atomically)
	if _, ok := a["old"]; ok {
		t.Fatalf("'a.old' survived __replace__; got: %v", a)
	}
	if a["new"] != float64(2) {
		t.Fatalf("a.new = %v, want 2", a["new"])
	}
}

func TestMergeJSONObjectsReplaceSentinelNotInOutput(t *testing.T) {
	base := []byte(`{"x":{"y":1}}`)
	overlay := []byte(`{"x":{"__replace__":{"z":2}}}`)

	merged, err := MergeJSONObjects(base, overlay)
	if err != nil {
		t.Fatalf("MergeJSONObjects() error = %v", err)
	}

	if !json.Valid(merged) {
		t.Fatalf("merged output is not valid JSON: %s", string(merged))
	}

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}

	// Walk all nested maps and ensure __replace__ never appears as a key
	var walk func(m map[string]any)
	walk = func(m map[string]any) {
		for k, v := range m {
			if k == "__replace__" {
				t.Fatalf("sentinel '__replace__' leaked into output: %v", m)
			}
			if sub, ok := v.(map[string]any); ok {
				walk(sub)
			}
		}
	}
	walk(got)
}

// ─── Issue #278: deep merge preserves stale wildcard permissions ──────────────

// TestMergeJSONObjects_Issue278_WildcardSurvivesDeepMerge proves that when an
// existing opencode.json contains the old "sdd-*": "allow" wildcard and the
// overlay supplies an explicit allowlist, deep merge keeps BOTH — the wildcard
// is never removed. This is the core of issue #278.
func TestMergeJSONObjects_Issue278_WildcardSurvivesDeepMerge(t *testing.T) {
	// Simulates an existing user's opencode.json (installed before the fix).
	base := []byte(`{
  "agent": {
    "sdd-orchestrator": {
      "permission": {
        "task": {
          "*": "deny",
          "sdd-*": "allow"
        }
      }
    }
  }
}`)

	// Simulates the NEW overlay with explicit allowlist (the fix).
	overlay := []byte(`{
  "agent": {
    "sdd-orchestrator": {
      "permission": {
        "task": {
          "*": "deny",
          "sdd-init": "allow",
          "sdd-explore": "allow",
          "sdd-propose": "allow",
          "sdd-spec": "allow",
          "sdd-design": "allow",
          "sdd-tasks": "allow",
          "sdd-apply": "allow",
          "sdd-verify": "allow",
          "sdd-archive": "allow",
          "sdd-onboard": "allow"
        }
      }
    }
  }
}`)

	merged, err := MergeJSONObjects(base, overlay)
	if err != nil {
		t.Fatalf("MergeJSONObjects() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("Unmarshal merged error = %v", err)
	}

	agent := got["agent"].(map[string]any)
	orch := agent["sdd-orchestrator"].(map[string]any)
	perm := orch["permission"].(map[string]any)
	task := perm["task"].(map[string]any)

	// The critical assertion: "sdd-*" SURVIVES the deep merge.
	// This proves existing users keep the wildcard even after syncing
	// with the new explicit overlay — the bug persists without __replace__.
	if _, hasWildcard := task["sdd-*"]; !hasWildcard {
		t.Fatal("UNEXPECTED: 'sdd-*' was removed by deep merge — this contradicts the merge algorithm")
	}

	// Verify the new explicit entries are also present (merge adds them).
	for _, phase := range []string{"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"} {
		if _, ok := task[phase]; !ok {
			t.Fatalf("explicit permission %q missing from merged result", phase)
		}
	}

	t.Logf("CONFIRMED: deep merge produces %d task keys (wildcard + explicit coexist)", len(task))
	t.Logf("Merged task block: %v", task)
}

// TestMergeJSONObjects_Issue278_ReplaceSentinelFixesWildcard proves that
// wrapping the task block in __replace__ DOES remove the old wildcard,
// which is the proposed fix for issue #278.
func TestMergeJSONObjects_Issue278_ReplaceSentinelFixesWildcard(t *testing.T) {
	// Same base: existing user with old wildcard.
	base := []byte(`{
  "agent": {
    "sdd-orchestrator": {
      "permission": {
        "task": {
          "*": "deny",
          "sdd-*": "allow"
        }
      }
    }
  }
}`)

	// Overlay using __replace__ sentinel on the task block.
	overlay := []byte(`{
  "agent": {
    "sdd-orchestrator": {
      "permission": {
        "task": {
          "__replace__": {
            "*": "deny",
            "sdd-init": "allow",
            "sdd-explore": "allow",
            "sdd-propose": "allow",
            "sdd-spec": "allow",
            "sdd-design": "allow",
            "sdd-tasks": "allow",
            "sdd-apply": "allow",
            "sdd-verify": "allow",
            "sdd-archive": "allow",
            "sdd-onboard": "allow"
          }
        }
      }
    }
  }
}`)

	merged, err := MergeJSONObjects(base, overlay)
	if err != nil {
		t.Fatalf("MergeJSONObjects() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("Unmarshal merged error = %v", err)
	}

	agent := got["agent"].(map[string]any)
	orch := agent["sdd-orchestrator"].(map[string]any)
	perm := orch["permission"].(map[string]any)
	task := perm["task"].(map[string]any)

	// The wildcard MUST be gone.
	if _, hasWildcard := task["sdd-*"]; hasWildcard {
		t.Fatal("'sdd-*' survived __replace__ — sentinel is broken")
	}

	// __replace__ must NOT leak into output.
	if _, leaked := task["__replace__"]; leaked {
		t.Fatal("__replace__ sentinel leaked into output")
	}

	// All explicit entries must be present.
	expected := []string{"*", "sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"}
	for _, key := range expected {
		if _, ok := task[key]; !ok {
			t.Fatalf("expected key %q missing from task block after __replace__", key)
		}
	}

	if len(task) != len(expected) {
		t.Fatalf("task block has %d keys, want %d; got: %v", len(task), len(expected), task)
	}

	t.Logf("CONFIRMED: __replace__ produces exactly %d task keys (no wildcard)", len(task))
}

func TestSharedJSONMergesKeepBaseBehaviorWithoutOpenCodeOptIn(t *testing.T) {
	kilocodeDefaults := func(base, overlay []byte) ([]byte, error) {
		return MergeJSONDefaultsForPath("opencode.json", base, overlay)
	}
	kilocodeOverlay := func(base, overlay []byte) ([]byte, error) {
		return MergeJSONObjectsForPath("opencode.json", base, overlay)
	}
	for _, tc := range []struct {
		name, base, overlay string
		merge               func([]byte, []byte) ([]byte, error)
	}{
		{"defaults over duplicate keys", `{"permission":{"bash":{"ssh":"deny","ssh":"allow"}}}`, `{"permission":{"read":{"*":"allow"}}}`, kilocodeDefaults},
		{"json overlay over duplicate keys", "{\n  // note\n  \"mcp\":{\"a\":1,\"a\":2}\n}\n", `{"theme":"x"}`, kilocodeOverlay},
		{"json overlay over nested comments", "{\n  \"agent\":{/* keep */\"custom\":{}}\n}\n", `{"agent":{"gentleman":{}}}`, kilocodeOverlay},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.merge([]byte(tc.base), []byte(tc.overlay))
			if err != nil {
				t.Fatalf("shared merge must keep base behavior without the OpenCode opt-in; got %v", err)
			}
			if _, err := UnmarshalJSONObject(got); err != nil {
				t.Fatalf("merged output is not a JSON object: %v\n%s", err, got)
			}
		})
	}
}

func TestJSONCTopLevelKeyIsEscaped(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      bool
	}{
		{"escaped touched key", "{\n  // note\n  \"\\u0061gent\": {}\n}\n", true},
		{"plain key", "{\n  // note\n  \"agent\": {}\n}\n", false},
		{"absent key", `{"theme":"x"}`, false},
		{"escaped nested key only", `{"agent":{"\u0067entleman":{}}}`, false},
		{"unparseable document", `{"\u0061gent":`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := JSONCTopLevelKeyIsEscaped([]byte(tc.raw), "agent"); got != tc.want {
				t.Fatalf("JSONCTopLevelKeyIsEscaped() = %v, want %v", got, tc.want)
			}
		})
	}
}

// exactNumberSettings holds number tokens that float64 decoding would round,
// reformat or reject (#1672); every rewrite must keep them verbatim.
const exactNumberSettings = `{"agent":{"gentleman":{"tools":{},"__managed_by":"gentle-ai/sdd"}},` +
	`"big":9007199254740993,"neg":-9007199254740995,"max":9223372036854775807,` +
	`"nested":{"list":[1.10,6.02214076e23,1E+2,{"min":-9223372036854775808}]},"huge":1e999}`

// assertExactNumbers re-decodes encoded independently of the code under test
// and checks every exactNumberSettings token at its own JSON path.
func assertExactNumbers(t *testing.T, encoded []byte) {
	t.Helper()
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(normalizeJSON(encoded)))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		t.Fatalf("decode rewritten JSON error = %v:\n%s", err, encoded)
	}
	nested, _ := root["nested"].(map[string]any)
	list, _ := nested["list"].([]any)
	if len(list) != 4 {
		t.Fatalf("nested.list = %#v, want 4 entries:\n%s", nested["list"], encoded)
	}
	last, _ := list[3].(map[string]any)
	got := []any{root["big"], root["neg"], root["max"], list[0], list[1], list[2], last["min"], root["huge"]}
	want := []string{"9007199254740993", "-9007199254740995", "9223372036854775807",
		"1.10", "6.02214076e23", "1E+2", "-9223372036854775808", "1e999"}
	for i := range want {
		if number, ok := got[i].(json.Number); !ok || number.String() != want[i] {
			t.Fatalf("number %d = %#v, want exact %s:\n%s", i, got[i], want[i], encoded)
		}
	}
}

func TestJSONRewritesPreserveExactNumberTokens(t *testing.T) {
	base := []byte(exactNumberSettings)
	jsonc := []byte("// user note\n" + strings.TrimSuffix(exactNumberSettings, "}") + ",}")
	for _, tc := range []struct {
		name    string
		rewrite func() ([]byte, error)
	}{
		{"merge", func() ([]byte, error) { return MergeJSONObjects(base, []byte(`{"added":true}`)) }},
		{"merge normalized jsonc", func() ([]byte, error) { return MergeJSONObjects(jsonc, []byte(`{"added":true}`)) }},
		{"merge jsonc touched value", func() ([]byte, error) {
			return MergeJSONObjectsForPath("opencode.jsonc", base, []byte(`{"nested":{"added":true}}`))
		}},
		{"defaults", func() ([]byte, error) { return MergeJSONDefaultsForPath("x.json", base, []byte(`{"added":true}`)) }},
		{"defaults jsonc touched value", func() ([]byte, error) {
			return MergeOpenCodeJSONDefaultsForPath("x.jsonc", base, []byte(`{"nested":{"added":true}}`))
		}},
		{"remove agent tools", func() ([]byte, error) { return RemoveJSONAgentTools(base, "gentleman") }},
		{"remove legacy markers", func() ([]byte, error) {
			return RemoveLegacyOpenCodeAgentMarkers("x.json", base, []string{"gentleman"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.rewrite()
			if err != nil {
				t.Fatalf("rewrite error = %v", err)
			}
			if string(got) == string(base) {
				t.Fatalf("rewrite left the document unchanged; the test must exercise a re-encode")
			}
			assertExactNumbers(t, got)
		})
	}

	root, err := UnmarshalJSONObject(base)
	if err != nil || root["big"] != json.Number("9007199254740993") {
		t.Fatalf("UnmarshalJSONObject() big = %#v (err %v), want exact json.Number", root["big"], err)
	}
}

func TestUnmarshalJSONObjectKeepsStrictSingleDocument(t *testing.T) {
	for _, tc := range []struct{ name, raw, wantErr string }{
		{"second document", `{"a":1}{"b":2}`, "after top-level value"},
		{"trailing text", `{"a":1} trailing`, "after top-level value"},
		{"truncated", `{"a":`, "unexpected end of JSON input"},
		{"non-object", `[1]`, "cannot unmarshal array"},
		{"blank", "  \n", ""},
		{"jsonc", "// note\n{\"a\":1,}", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := UnmarshalJSONObject([]byte(tc.raw))
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("UnmarshalJSONObject(%q) error = %v, want %q", tc.raw, err, tc.wantErr)
			}
		})
	}
}
