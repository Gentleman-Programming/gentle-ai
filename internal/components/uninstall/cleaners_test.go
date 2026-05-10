package uninstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzNormalizeJSON_NoPanic(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"mcpServers":{"engram":{"command":"engram"}}}`),
		[]byte("{\n  // comment\n  \"mcpServers\": {\n    \"engram\": {\n      \"command\": \"engram\",\n    },\n  },\n}\n"),
		[]byte(`{"url":"https://example.com/x//y","arr":[1,2,],}`),
		[]byte(`{"quote":"escaped \" // not a comment"}`),
		[]byte(`{"unterminated": /* comment`),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		normalized := normalizeJSON(input)
		_, _ = unmarshalJSONObject(normalized)
	})
}

func TestRemoveMarkdownSections_RemovesOnlyManagedBlock(t *testing.T) {
	input := strings.Join([]string{
		"# User Intro",
		"",
		"Keep this.",
		"",
		"<!-- gentle-ai:engram-protocol -->",
		"Managed content.",
		"<!-- /gentle-ai:engram-protocol -->",
		"",
		"# User Footer",
		"",
		"Must stay.",
	}, "\n") + "\n"

	updated, changed := removeMarkdownSections(input, "engram-protocol")
	if !changed {
		t.Fatal("removeMarkdownSections() changed = false, want true")
	}
	if strings.Contains(updated, "gentle-ai:engram-protocol") {
		t.Fatalf("managed marker block still present:\n%s", updated)
	}
	if !strings.Contains(updated, "# User Intro") || !strings.Contains(updated, "# User Footer") {
		t.Fatalf("user content was lost:\n%s", updated)
	}
}

func TestRemoveManagedPersonaPreamble_PreservesManagedSuffix(t *testing.T) {
	input := strings.Join([]string{
		"---",
		"name: Gentle AI Persona",
		"description: Teaching-oriented persona with SDD orchestration and Engram protocol",
		"applyTo: \"**\"",
		"---",
		"",
		"## Personality",
		"Senior Architect mentor persona.",
		"",
		"## Rules",
		"Be direct.",
		"",
		"<!-- gentle-ai:sdd-orchestrator -->",
		"SDD stays.",
		"<!-- /gentle-ai:sdd-orchestrator -->",
	}, "\n") + "\n"

	updated, changed := removeManagedPersonaPreamble(input)
	if !changed {
		t.Fatal("removeManagedPersonaPreamble() changed = false, want true")
	}
	if strings.Contains(updated, "name: Gentle AI Persona") || strings.Contains(updated, "## Personality") {
		t.Fatalf("managed persona preamble still present:\n%s", updated)
	}
	if !strings.HasPrefix(updated, "<!-- gentle-ai:sdd-orchestrator -->") {
		t.Fatalf("managed suffix was not preserved at file start:\n%s", updated)
	}
}

func TestRemoveManagedPersonaPreamble_WithoutMarkerDoesNotDeleteContent(t *testing.T) {
	input := strings.Join([]string{
		"---",
		"name: Gentle AI Persona",
		"description: Teaching-oriented persona with SDD orchestration and Engram protocol",
		"---",
		"",
		"## Personality",
		"Senior Architect mentor persona.",
	}, "\n")

	updated, changed := removeManagedPersonaPreamble(input)
	if changed {
		t.Fatal("removeManagedPersonaPreamble() changed = true, want false without explicit marker")
	}
	if updated != input {
		t.Fatalf("content was modified without explicit marker:\n%s", updated)
	}
}

func TestRemoveJSONPaths_RemovesOnlyManagedKeys(t *testing.T) {
	input := []byte(`{
  "theme": "gentleman-kanagawa",
  "permission": {
    "bash": {
      "*": "allow"
    }
  },
  "mcpServers": {
    "context7": {
      "command": "npx"
    },
    "custom": {
      "command": "python"
    }
  },
  "userSetting": true
}
`)

	updated, changed, err := removeJSONPaths(input,
		jsonPath{"theme"},
		jsonPath{"permission"},
		jsonPath{"mcpServers", "context7"},
	)
	if err != nil {
		t.Fatalf("removeJSONPaths() error = %v", err)
	}
	if !changed {
		t.Fatal("removeJSONPaths() changed = false, want true")
	}

	var got map[string]any
	if err := json.Unmarshal(updated, &got); err != nil {
		t.Fatalf("json.Unmarshal(updated) error = %v", err)
	}

	if _, exists := got["theme"]; exists {
		t.Fatalf("theme key should be removed: %#v", got)
	}
	if _, exists := got["permission"]; exists {
		t.Fatalf("permission key should be removed: %#v", got)
	}
	if got["userSetting"] != true {
		t.Fatalf("userSetting = %#v, want true", got["userSetting"])
	}

	mcpServers, ok := got["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers missing or invalid: %#v", got["mcpServers"])
	}
	if _, exists := mcpServers["context7"]; exists {
		t.Fatalf("context7 should be removed from mcpServers: %#v", mcpServers)
	}
	if _, exists := mcpServers["custom"]; !exists {
		t.Fatalf("custom server should be preserved: %#v", mcpServers)
	}
}

func TestRemoveJSONPaths_SupportsCommentsAndTrailingCommas(t *testing.T) {
	input := []byte(`{
  // user comment
  "mcpServers": {
    "engram": {
      "command": "engram",
    },
    "custom": {
      "command": "python"
    },
  },
}
`)

	updated, changed, err := removeJSONPaths(input, jsonPath{"mcpServers", "engram"})
	if err != nil {
		t.Fatalf("removeJSONPaths() error = %v", err)
	}
	if !changed {
		t.Fatal("removeJSONPaths() changed = false, want true")
	}

	var got map[string]any
	if err := json.Unmarshal(updated, &got); err != nil {
		t.Fatalf("json.Unmarshal(updated) error = %v", err)
	}
	mcpServers := got["mcpServers"].(map[string]any)
	if _, exists := mcpServers["engram"]; exists {
		t.Fatalf("engram should be removed: %#v", mcpServers)
	}
	if _, exists := mcpServers["custom"]; !exists {
		t.Fatalf("custom server should remain: %#v", mcpServers)
	}
}

func TestUnmarshalJSONObject_PreservesLargeIntegersAsJSONNumber(t *testing.T) {
	root, err := unmarshalJSONObject([]byte(`{"big":9223372036854775807}`))
	if err != nil {
		t.Fatalf("unmarshalJSONObject() error = %v", err)
	}

	number, ok := root["big"].(json.Number)
	if !ok {
		t.Fatalf("big value type = %T, want json.Number", root["big"])
	}
	if string(number) != "9223372036854775807" {
		t.Fatalf("json.Number = %q, want exact integer", number)
	}
}

func TestUnmarshalJSONObject_RejectsTrailingJSONPayload(t *testing.T) {
	_, err := unmarshalJSONObject([]byte(`{"one":1}{"two":2}`))
	if err == nil {
		t.Fatal("unmarshalJSONObject() error = nil, want rejection for trailing JSON payload")
	}
}

func TestRemoveJSONPaths_PreservesCRLF(t *testing.T) {
	input := []byte("{\r\n  \"outputStyle\": \"Gentleman\",\r\n  \"userSetting\": true\r\n}\r\n")

	updated, changed, err := removeJSONPaths(input, jsonPath{"outputStyle"})
	if err != nil {
		t.Fatalf("removeJSONPaths() error = %v", err)
	}
	if !changed {
		t.Fatal("removeJSONPaths() changed = false, want true")
	}
	if !strings.Contains(string(updated), "\r\n") {
		t.Fatalf("expected CRLF to be preserved, got: %q", string(updated))
	}
}

func TestReadManagedFile_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	_, err := readManagedFile(link)
	if err == nil || !strings.Contains(err.Error(), "refusing to read symlink") {
		t.Fatalf("readManagedFile(symlink) error = %v, want symlink rejection", err)
	}
}

func TestReadManagedFile_RejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.md")
	data := make([]byte, maxManagedFileSize+1)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(huge) error = %v", err)
	}

	_, err := readManagedFile(path)
	if err == nil || !strings.Contains(err.Error(), "exceeds max managed size") {
		t.Fatalf("readManagedFile(huge) error = %v, want max-size rejection", err)
	}
}

func TestCleanCodexTOML_DoesNotDoubleRestoreCRLF(t *testing.T) {
	input := strings.Join([]string{
		"model_instructions_file = \"/tmp/managed.md\"",
		"custom_top = \"keep\"",
		"",
		"[other]",
		"value = \"keep\"",
	}, "\r\n") + "\r\n"

	updated, changed := cleanCodexTOML(input)
	if !changed {
		t.Fatal("cleanCodexTOML() changed = false, want true")
	}
	if strings.Contains(updated, "\r\r\n") {
		t.Fatalf("cleanCodexTOML() produced doubled CRLF: %q", updated)
	}
	if strings.Contains(updated, "\r\n") {
		t.Fatalf("cleanCodexTOML() should return normalized LF content, got: %q", updated)
	}
	if !strings.Contains(updated, "custom_top = \"keep\"") {
		t.Fatalf("top-level user content lost: %q", updated)
	}
	if !strings.Contains(updated, "[other]\nvalue = \"keep\"") {
		t.Fatalf("table content lost: %q", updated)
	}
}

func TestRemoveTopLevelTOMLKeys_PreservesNestedKeys(t *testing.T) {
	input := strings.Join([]string{
		"custom_top = \"keep\"",
		"model_instructions_file = \"/tmp/managed.md\"",
		"",
		"[nested]",
		"model_instructions_file = \"user-value\"",
	}, "\n") + "\n"

	updated := removeTopLevelTOMLKeys(input, "model_instructions_file")
	if strings.Contains(updated, "model_instructions_file = \"/tmp/managed.md\"") {
		t.Fatalf("top-level managed key was not removed: %q", updated)
	}
	if !strings.Contains(updated, "model_instructions_file = \"user-value\"") {
		t.Fatalf("nested key should be preserved: %q", updated)
	}
}

func TestCleanCodexTOML_RemovesOnlyManagedEntries(t *testing.T) {
	input := strings.Join([]string{
		"model_instructions_file = \"/home/me/.codex/engram-instructions.md\"",
		"experimental_compact_prompt_file = \"/home/me/.codex/engram-compact-prompt.md\"",
		"custom_top = \"keep\"",
		"",
		"[mcp_servers.engram]",
		"command = \"engram\"",
		"args = [\"mcp\", \"--tools=agent\"]",
		"",
		"[other]",
		"value = \"keep\"",
	}, "\n") + "\n"

	updated, changed := cleanCodexTOML(input)
	if !changed {
		t.Fatal("cleanCodexTOML() changed = false, want true")
	}
	if strings.Contains(updated, "model_instructions_file") || strings.Contains(updated, "experimental_compact_prompt_file") {
		t.Fatalf("managed top-level keys were not removed:\n%s", updated)
	}
	if strings.Contains(updated, "[mcp_servers.engram]") {
		t.Fatalf("managed engram TOML block was not removed:\n%s", updated)
	}
	if !strings.Contains(updated, "custom_top = \"keep\"") || !strings.Contains(updated, "[other]") {
		t.Fatalf("user TOML content was lost:\n%s", updated)
	}
}

func TestMarkdownCleanup_OnRealFileWithTempDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	input := strings.Join([]string{
		"# User Heading",
		"",
		"Hand-written intro.",
		"",
		"<!-- gentle-ai:engram-protocol -->",
		"Managed content.",
		"<!-- /gentle-ai:engram-protocol -->",
		"",
		"# Footer",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	updated, changed := removeMarkdownSections(string(raw), "engram-protocol")
	if !changed {
		t.Fatal("removeMarkdownSections() changed = false, want true")
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("WriteFile(updated) error = %v", err)
	}

	finalRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(final) error = %v", err)
	}
	final := string(finalRaw)
	if strings.Contains(final, "gentle-ai:engram-protocol") {
		t.Fatalf("managed markdown block still present in file:\n%s", final)
	}
	if !strings.Contains(final, "Hand-written intro.") || !strings.Contains(final, "# Footer") {
		t.Fatalf("user markdown content was lost:\n%s", final)
	}
}

func TestJSONCleanup_OnRealFileWithTempDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	input := []byte(`{
  "outputStyle": "Gentleman",
  "mcpServers": {
    "engram": {
      "command": "engram"
    },
    "custom": {
      "command": "python"
    }
  },
  "userSetting": "keep"
}
`)
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	updated, changed, err := removeJSONPaths(raw, jsonPath{"outputStyle"}, jsonPath{"mcpServers", "engram"})
	if err != nil {
		t.Fatalf("removeJSONPaths() error = %v", err)
	}
	if !changed {
		t.Fatal("removeJSONPaths() changed = false, want true")
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("WriteFile(updated) error = %v", err)
	}

	finalRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(final) error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(finalRaw, &got); err != nil {
		t.Fatalf("json.Unmarshal(final) error = %v", err)
	}
	if _, exists := got["outputStyle"]; exists {
		t.Fatalf("outputStyle should be removed: %#v", got)
	}
	if got["userSetting"] != "keep" {
		t.Fatalf("userSetting = %#v, want %q", got["userSetting"], "keep")
	}
	mcpServers := got["mcpServers"].(map[string]any)
	if _, exists := mcpServers["engram"]; exists {
		t.Fatalf("engram should be removed from file JSON: %#v", mcpServers)
	}
	if _, exists := mcpServers["custom"]; !exists {
		t.Fatalf("custom server should remain in file JSON: %#v", mcpServers)
	}
}
