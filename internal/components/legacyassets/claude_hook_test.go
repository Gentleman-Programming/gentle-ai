package legacyassets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetireClaudeSDDPreflightHookRemovesOnlyTheReleasedEntry(t *testing.T) {
	for _, tc := range []struct {
		name, settings string
		want           any
		removed        bool
		edited         bool
	}{
		{
			name:     "released group",
			settings: `{"a": 1, "hooks": {"PreToolUse": [{"matcher": "Agent", "hooks": [{"type": "command", "command": "gentle-ai sdd-preflight-hook --agent claude-code", "timeout": 30}]}], "Stop": []}}`,
			want:     map[string]any{"a": 1.0, "hooks": map[string]any{"Stop": []any{}}},
			removed:  true,
		},
		{
			name:     "shared group keeps the user hook",
			settings: `{"hooks": {"PreToolUse": [{"matcher": "Agent", "hooks": [{"type": "command", "command": "echo mine"}, {"type": "command", "command": "gentle-ai sdd-preflight-hook --agent claude-code", "timeout": 30}]}]}}`,
			want:     map[string]any{"hooks": map[string]any{"PreToolUse": []any{map[string]any{"matcher": "Agent", "hooks": []any{map[string]any{"type": "command", "command": "echo mine"}}}}}},
			removed:  true,
		},
		{
			name:     "edited hook is kept",
			settings: `{"hooks": {"PreToolUse": [{"matcher": "Agent", "hooks": [{"type": "command", "command": "gentle-ai sdd-preflight-hook --agent claude-code", "timeout": 90}]}]}}`,
			edited:   true,
		},
		{
			name:     "no hook",
			settings: `{"hooks": {"Stop": [{"matcher": "", "hooks": [{"type": "command", "command": "echo  &&  keep"}]}]}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			writeFixture(t, path, []byte(tc.settings))
			res, err := RetireClaudeSDDPreflightHook(path)
			if err != nil {
				t.Fatal(err)
			}
			if res.Removed != tc.removed || res.Edited != tc.edited {
				t.Fatalf("result = %+v", res)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.removed {
				if string(raw) != tc.settings {
					t.Fatalf("settings rewritten without a removal:\n%s", raw)
				}
				if tc.edited != (len(res.ManualActions()) == 1) {
					t.Fatalf("ManualActions = %v", res.ManualActions())
				}
				return
			}
			var got any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			wantJSON, _ := json.Marshal(tc.want)
			gotJSON, _ := json.Marshal(got)
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("settings = %s, want %s", gotJSON, wantJSON)
			}
			again, err := RetireClaudeSDDPreflightHook(path)
			if err != nil || again.Removed {
				t.Fatalf("second retirement = %+v, %v", again, err)
			}
			if after, _ := os.ReadFile(path); string(after) != string(raw) {
				t.Fatal("second retirement rewrote the settings")
			}
		})
	}
}

func TestRetireClaudeSDDPreflightHookKeepsUserBytesVerbatim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFixture(t, path, []byte(`{"hooks": {"PreToolUse": [{"matcher": "Agent", "hooks": [{"type": "command", "command": "gentle-ai sdd-preflight-hook --agent claude-code", "timeout": 30}]}], "Stop": [{"matcher": "", "hooks": [{"type": "command", "command": "a && b <x>", "timeout": 1e3}]}]}}`))
	if _, err := RetireClaudeSDDPreflightHook(path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"a && b <x>"`) || !strings.Contains(string(raw), `1e3`) {
		t.Fatalf("user strings or numbers re-encoded:\n%s", raw)
	}
}
