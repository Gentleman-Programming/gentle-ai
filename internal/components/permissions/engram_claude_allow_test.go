package permissions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func engramClaudeSettingsPath(home string) string {
	return filepath.Join(home, ".claude", "settings.json")
}

func writeEngramClaudeSettings(t *testing.T, home, content string) string {
	t.Helper()
	path := engramClaudeSettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readAllowList(t *testing.T, path string) []string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Ask   []string `json:"ask"`
			Deny  []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("settings not valid JSON: %v\n%s", err, content)
	}
	return settings.Permissions.Allow
}

func TestEnsureEngramClaudeAutoAllowRulesFreshInstall(t *testing.T) {
	home := t.TempDir()

	changed, err := EnsureEngramClaudeAutoAllowRules(home)
	if err != nil {
		t.Fatalf("EnsureEngramClaudeAutoAllowRules() error = %v", err)
	}
	if !changed {
		t.Error("changed = false, want true: a fresh install has every rule missing")
	}

	allow := readAllowList(t, engramClaudeSettingsPath(home))
	allowSet := make(map[string]bool, len(allow))
	for _, rule := range allow {
		allowSet[rule] = true
	}

	want := engramClaudeAutoAllowRules()
	if len(want) != 24 {
		t.Fatalf("expected 24 candidate rules, got %d", len(want))
	}
	for _, rule := range want {
		if !allowSet[rule] {
			t.Errorf("fresh install missing rule %q; got allow = %v", rule, allow)
		}
	}
}

func TestEnsureEngramClaudeAutoAllowRulesNoPermissionsKey(t *testing.T) {
	home := t.TempDir()
	writeEngramClaudeSettings(t, home, `{"unrelated":"keep"}`)

	if _, err := EnsureEngramClaudeAutoAllowRules(home); err != nil {
		t.Fatalf("EnsureEngramClaudeAutoAllowRules() error = %v", err)
	}

	content, err := os.ReadFile(engramClaudeSettingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["unrelated"] != "keep" {
		t.Errorf("unrelated top-level key lost: %#v", settings)
	}
	allow := readAllowList(t, engramClaudeSettingsPath(home))
	if len(allow) != 24 {
		t.Fatalf("allow = %v, want 24 rules", allow)
	}
}

func TestEnsureEngramClaudeAutoAllowRulesSkipsCoveredRules(t *testing.T) {
	home := t.TempDir()
	// mem_save is already allowed verbatim under the plain engram server.
	// mem_update is covered by an MCP server-level wildcard for the plugin
	// server. mem_review is covered by an unrelated glob a user might have
	// written for a totally different purpose. Everything else is missing.
	seed := `{
  "permissions": {
    "allow": ["mcp__engram__mem_save", "mcp__plugin_engram_engram"],
    "ask": ["mcp__*__mem_review"],
    "deny": ["Read(.env)"]
  },
  "unrelated": "keep"
}`
	path := writeEngramClaudeSettings(t, home, seed)

	if _, err := EnsureEngramClaudeAutoAllowRules(home); err != nil {
		t.Fatalf("EnsureEngramClaudeAutoAllowRules() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["unrelated"] != "keep" {
		t.Errorf("unrelated top-level key lost: %#v", settings)
	}
	perms := settings["permissions"].(map[string]any)
	if got := perms["ask"].([]any); len(got) != 1 || got[0] != "mcp__*__mem_review" {
		t.Errorf("ask list changed: %#v", got)
	}
	if got := perms["deny"].([]any); len(got) != 1 || got[0] != "Read(.env)" {
		t.Errorf("deny list changed: %#v", got)
	}

	allow := readAllowList(t, path)
	allowSet := make(map[string]bool, len(allow))
	for _, rule := range allow {
		allowSet[rule] = true
	}

	// Pre-existing entries remain.
	if !allowSet["mcp__engram__mem_save"] {
		t.Error("pre-existing exact allow entry mcp__engram__mem_save was lost")
	}
	if !allowSet["mcp__plugin_engram_engram"] {
		t.Error("pre-existing server-level allow entry mcp__plugin_engram_engram was lost")
	}

	// Already-covered candidates were not duplicated into allow.
	if allowSet["mcp__plugin_engram_engram__mem_update"] {
		t.Error("mcp__plugin_engram_engram__mem_update should be covered by the server-level allow entry, not duplicated")
	}
	if allowSet["mcp__engram__mem_review"] || allowSet["mcp__plugin_engram_engram__mem_review"] {
		t.Error("mem_review should be covered by the ask glob mcp__*__mem_review, not duplicated into allow")
	}

	// Genuinely missing rules were added.
	if !allowSet["mcp__engram__mem_update"] {
		t.Error("mcp__engram__mem_update should have been added (not covered by any existing rule)")
	}
	if !allowSet["mcp__engram__mem_judge"] {
		t.Error("mcp__engram__mem_judge should have been added")
	}
}

func TestEnsureEngramClaudeAutoAllowRulesNeverAddsDestructiveTools(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureEngramClaudeAutoAllowRules(home); err != nil {
		t.Fatalf("EnsureEngramClaudeAutoAllowRules() error = %v", err)
	}
	allow := readAllowList(t, engramClaudeSettingsPath(home))
	for _, rule := range allow {
		if rule == "mcp__engram__mem_delete" || rule == "mcp__plugin_engram_engram__mem_delete" ||
			rule == "mcp__engram__mem_merge_projects" || rule == "mcp__plugin_engram_engram__mem_merge_projects" {
			t.Errorf("destructive tool rule %q must never be auto-allowed", rule)
		}
	}
}

func TestEnsureEngramClaudeAutoAllowRulesIsIdempotent(t *testing.T) {
	home := t.TempDir()
	firstChanged, err := EnsureEngramClaudeAutoAllowRules(home)
	if err != nil {
		t.Fatalf("first call error = %v", err)
	}
	if !firstChanged {
		t.Error("first call changed = false, want true: a fresh install has every rule missing")
	}
	path := engramClaudeSettingsPath(home)
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	secondChanged, err := EnsureEngramClaudeAutoAllowRules(home)
	if err != nil {
		t.Fatalf("second call error = %v", err)
	}
	if secondChanged {
		t.Error("second call changed = true, want false: every rule is already present")
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("second call changed settings.json:\nfirst:  %s\nsecond: %s", first, second)
	}

	allow := readAllowList(t, path)
	seen := map[string]int{}
	for _, rule := range allow {
		seen[rule]++
	}
	for rule, count := range seen {
		if count > 1 {
			t.Errorf("rule %q appears %d times after two calls, want at most once", rule, count)
		}
	}
}

// TestEnsureEngramClaudeAutoAllowRulesRefusesLockedSettings mirrors
// TestSelectedPermissionsNeverWidenLockedMode: a mode-0 settings file is
// normally unreadable, so the function returns an error and leaves it
// byte-for-byte untouched (the same refusal the base writer already gives
// every non-OpenCode agent's injection path in this package). If a sandboxed
// test environment can read a mode-0 file anyway, the write must still never
// corrupt the content or widen the mode past owner-only (0600).
func TestEnsureEngramClaudeAutoAllowRulesRefusesLockedSettings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not supported on Windows")
	}
	home := t.TempDir()
	const original = `{"permissions":{"allow":[]}}`
	path := writeEngramClaudeSettings(t, home, original)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}

	_, callErr := EnsureEngramClaudeAutoAllowRules(home)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0 && mode != 0o600 {
		t.Fatalf("locked settings mode widened past owner-only: %04o", mode)
	}

	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if callErr != nil {
		// The common case: the file was genuinely unreadable, so nothing
		// was written at all.
		if string(content) != original {
			t.Fatalf("locked settings file was modified despite a refusal error: %s", content)
		}
		return
	}
	// The sandboxed fallback: the call succeeded, so it must have produced
	// valid JSON with the original content preserved, not a corrupted file.
	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("settings corrupted after a successful call on a locked file: %v\n%s", err, content)
	}
}

func TestEnsureEngramClaudeAutoAllowRulesLeavesMalformedSettingsUntouched(t *testing.T) {
	home := t.TempDir()
	seed := `not valid json at all`
	path := writeEngramClaudeSettings(t, home, seed)

	changed, err := EnsureEngramClaudeAutoAllowRules(home)
	if err != nil {
		t.Fatalf("EnsureEngramClaudeAutoAllowRules() error = %v, want nil (malformed settings are left alone)", err)
	}
	if changed {
		t.Error("changed = true, want false: malformed settings are left alone")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != seed {
		t.Fatalf("malformed settings.json was modified: %q, want unchanged %q", content, seed)
	}
}

func TestEnsureEngramClaudeAutoAllowRulesLeavesNullSettingsUntouched(t *testing.T) {
	home := t.TempDir()
	seed := `null`
	path := writeEngramClaudeSettings(t, home, seed)

	changed, err := EnsureEngramClaudeAutoAllowRules(home)
	if err != nil {
		t.Fatalf("EnsureEngramClaudeAutoAllowRules() error = %v, want nil (a null document is left alone)", err)
	}
	if changed {
		t.Error("changed = true, want false: a null document is left alone")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != seed {
		t.Fatalf("null settings.json was modified: %q, want unchanged %q", content, seed)
	}
}

func TestEnsureEngramClaudeAutoAllowRulesLeavesNonObjectPermissionsUntouched(t *testing.T) {
	home := t.TempDir()
	seed := `{"permissions":"not an object","unrelated":"keep"}`
	path := writeEngramClaudeSettings(t, home, seed)

	changed, err := EnsureEngramClaudeAutoAllowRules(home)
	if err != nil {
		t.Fatalf("EnsureEngramClaudeAutoAllowRules() error = %v, want nil", err)
	}
	if changed {
		t.Error("changed = true, want false: a non-object permissions value must not be coerced")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != seed {
		t.Fatalf("settings.json with a non-object permissions value was modified: %q, want unchanged %q", content, seed)
	}
}

func TestEnsureEngramClaudeAutoAllowRulesLeavesNonArrayAllowUntouched(t *testing.T) {
	home := t.TempDir()
	seed := `{"permissions":{"allow":"not an array"},"unrelated":"keep"}`
	path := writeEngramClaudeSettings(t, home, seed)

	changed, err := EnsureEngramClaudeAutoAllowRules(home)
	if err != nil {
		t.Fatalf("EnsureEngramClaudeAutoAllowRules() error = %v, want nil", err)
	}
	if changed {
		t.Error("changed = true, want false: a non-array permissions.allow value must not be coerced")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != seed {
		t.Fatalf("settings.json with a non-array permissions.allow value was modified: %q, want unchanged %q", content, seed)
	}
}

func TestEngramClaudeAutoAllowRuleGaps(t *testing.T) {
	if gaps := EngramClaudeAutoAllowRuleGaps(nil); len(gaps) != 24 {
		t.Fatalf("gaps for empty settings = %d, want 24", len(gaps))
	}

	covered := []byte(`{"permissions":{"allow":["mcp__engram","mcp__plugin_engram_engram"]}}`)
	if gaps := EngramClaudeAutoAllowRuleGaps(covered); len(gaps) != 0 {
		t.Fatalf("gaps with both servers wildcarded = %v, want none", gaps)
	}

	malformed := []byte(`not json`)
	if gaps := EngramClaudeAutoAllowRuleGaps(malformed); len(gaps) != 24 {
		t.Fatalf("gaps for unparseable settings = %d, want 24 (fail closed)", len(gaps))
	}

	// A wildcard SERVER segment ("mcp__*__mem_save") is not honored by Claude
	// Code as covering a specific server's tool, so it must not count as a
	// covered gap either: the real literal rule is still missing.
	wildcardServer := []byte(`{"permissions":{"allow":["mcp__*__mem_save"]}}`)
	gaps := EngramClaudeAutoAllowRuleGaps(wildcardServer)
	foundMissing := false
	for _, gap := range gaps {
		if gap == "mcp__engram__mem_save" {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Fatalf("gaps with only a wildcard-server allow entry = %v, want mcp__engram__mem_save still reported missing", gaps)
	}
}

func TestEngramClaudeAutoAllowStatus(t *testing.T) {
	malformed := EngramClaudeAutoAllowStatus([]byte(`not json`))
	if malformed.Parsed {
		t.Fatalf("Parsed = true for unparseable content, want false")
	}
	if len(malformed.Missing) != 0 || len(malformed.Restricted) != 0 {
		t.Fatalf("unparseable content report = %+v, want empty Missing and Restricted", malformed)
	}

	fullyAllowed := EngramClaudeAutoAllowStatus([]byte(`{"permissions":{"allow":["mcp__engram","mcp__plugin_engram_engram"]}}`))
	if !fullyAllowed.Parsed || len(fullyAllowed.Missing) != 0 || len(fullyAllowed.Restricted) != 0 {
		t.Fatalf("fully-allowed report = %+v, want Parsed=true and both slices empty", fullyAllowed)
	}

	// A deny rule restricts mem_save for the plain engram server; nothing
	// else is covered at all.
	restricted := EngramClaudeAutoAllowStatus([]byte(`{"permissions":{"deny":["mcp__engram__mem_save"]}}`))
	if !restricted.Parsed {
		t.Fatal("Parsed = false, want true")
	}
	if len(restricted.Restricted) != 1 || restricted.Restricted[0] != "mcp__engram__mem_save" {
		t.Fatalf("Restricted = %v, want exactly [mcp__engram__mem_save]", restricted.Restricted)
	}
	for _, rule := range restricted.Missing {
		if rule == "mcp__engram__mem_save" {
			t.Fatal("mcp__engram__mem_save reported Missing, want it reported only in Restricted")
		}
	}
	if len(restricted.Missing) != 23 {
		t.Fatalf("len(Missing) = %d, want 23 (every other candidate rule)", len(restricted.Missing))
	}

	// An ask rule with a wildcard server segment still restricts, even though
	// the same shape would not count as allow coverage.
	restrictedWildcard := EngramClaudeAutoAllowStatus([]byte(`{"permissions":{"ask":["mcp__*__mem_judge"]}}`))
	foundRestricted := false
	for _, rule := range restrictedWildcard.Restricted {
		if rule == "mcp__engram__mem_judge" {
			foundRestricted = true
		}
	}
	if !foundRestricted {
		t.Fatalf("Restricted = %v, want mcp__engram__mem_judge restricted by the wildcard-server ask entry", restrictedWildcard.Restricted)
	}
}
