package filemerge

import (
	"strings"
	"testing"
)

// ─── UpsertCodexEngramBlock ───────────────────────────────────────────────────

func TestUpsertCodexEngramBlock_Empty(t *testing.T) {
	result := UpsertCodexEngramBlock("", "")

	if !strings.Contains(result, "[mcp_servers.engram]") {
		t.Fatalf("result missing [mcp_servers.engram]; got:\n%s", result)
	}
	if !strings.Contains(result, `command = "engram"`) {
		t.Fatalf("result missing command = \"engram\"; got:\n%s", result)
	}
	if !strings.Contains(result, `"--tools=agent"`) {
		t.Fatalf("result missing --tools=agent; got:\n%s", result)
	}
	if !strings.Contains(result, `args = ["mcp", "--tools=agent"]`) {
		t.Fatalf("result has wrong args format; got:\n%s", result)
	}
}

func TestRemoveTOMLTable(t *testing.T) {
	input := "model = \"gpt-5.5\"\n\n[mcp_servers.engram]\ncommand = \"engram\"\nargs = [\"mcp\"]\n\n[other]\nvalue = true\n"
	got := RemoveTOMLTable(input, "mcp_servers.engram")
	if strings.Contains(got, "[mcp_servers.engram]") || strings.Contains(got, "command = \"engram\"") {
		t.Fatalf("RemoveTOMLTable() kept removed table; got:\n%s", got)
	}
	if !strings.Contains(got, "model = \"gpt-5.5\"") || !strings.Contains(got, "[other]") {
		t.Fatalf("RemoveTOMLTable() did not preserve unrelated content; got:\n%s", got)
	}
}

func TestRemoveTOMLTable_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		tableName string
		want      string
	}{
		{
			name:      "table at EOF without trailing newline",
			input:     "model = \"gpt-5.5\"\n\n[mcp_servers.engram]\ncommand = \"engram\"",
			tableName: "mcp_servers.engram",
			want:      "model = \"gpt-5.5\"\n",
		},
		{
			name:      "absent table preserves LF content unchanged",
			input:     "model = \"gpt-5.5\"\n\n[other]\nvalue = true\n",
			tableName: "mcp_servers.engram",
			want:      "model = \"gpt-5.5\"\n\n[other]\nvalue = true\n",
		},
		{
			name:      "preserves top-level and next table boundaries",
			input:     "model = \"gpt-5.5\"\n\n[mcp_servers.engram]\ncommand = \"engram\"\n\n[other]\nvalue = true\n",
			tableName: "mcp_servers.engram",
			want:      "model = \"gpt-5.5\"\n\n[other]\nvalue = true\n",
		},
		{
			name:      "adjacent tables keep following table",
			input:     "[mcp_servers.engram]\ncommand = \"engram\"\n[other]\nvalue = true\n",
			tableName: "mcp_servers.engram",
			want:      "[other]\nvalue = true\n",
		},
		{
			name:      "CRLF input is normalized to LF",
			input:     "model = \"gpt-5.5\"\r\n\r\n[mcp_servers.engram]\r\ncommand = \"engram\"\r\n[other]\r\nvalue = true\r\n",
			tableName: "mcp_servers.engram",
			want:      "model = \"gpt-5.5\"\n\n[other]\nvalue = true\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveTOMLTable(tt.input, tt.tableName); got != tt.want {
				t.Fatalf("RemoveTOMLTable() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpsertCodexEngramBlock_ExistingBlock(t *testing.T) {
	input := `[other_section]
key = "value"

[mcp_servers.engram]
command = "engram"
args = ["mcp"]

[another_section]
foo = "bar"
`
	result := UpsertCodexEngramBlock(input, "")

	// Must have exactly one [mcp_servers.engram] block.
	count := strings.Count(result, "[mcp_servers.engram]")
	if count != 1 {
		t.Fatalf("expected 1 [mcp_servers.engram] block, got %d; result:\n%s", count, result)
	}

	// Must preserve unrelated sections.
	if !strings.Contains(result, "[other_section]") {
		t.Fatalf("result missing [other_section]; got:\n%s", result)
	}
	if !strings.Contains(result, "[another_section]") {
		t.Fatalf("result missing [another_section]; got:\n%s", result)
	}

	// Must use the updated args with --tools=agent.
	if !strings.Contains(result, `"--tools=agent"`) {
		t.Fatalf("result missing --tools=agent; got:\n%s", result)
	}
}

func TestUpsertCodexEngramBlock_PreservesOtherSections(t *testing.T) {
	input := `model = "gpt-4o"

[settings]
timeout = 30
`
	result := UpsertCodexEngramBlock(input, "")

	if !strings.Contains(result, `model = "gpt-4o"`) {
		t.Fatalf("result missing top-level model key; got:\n%s", result)
	}
	if !strings.Contains(result, "[settings]") {
		t.Fatalf("result missing [settings] section; got:\n%s", result)
	}
	if !strings.Contains(result, "[mcp_servers.engram]") {
		t.Fatalf("result missing [mcp_servers.engram]; got:\n%s", result)
	}
}

func TestUpsertCodexEngramBlock_AbsolutePath(t *testing.T) {
	result := UpsertCodexEngramBlock("", "/usr/local/bin/engram")

	if !strings.Contains(result, "[mcp_servers.engram]") {
		t.Fatalf("result missing [mcp_servers.engram]; got:\n%s", result)
	}
	if !strings.Contains(result, `command = "/usr/local/bin/engram"`) {
		t.Fatalf("result missing absolute command path; got:\n%s", result)
	}
	if strings.Contains(result, `command = "engram"`) {
		t.Fatalf("result should NOT have relative command when absolute path given; got:\n%s", result)
	}
}

func TestUpsertCodexEngramBlock_Idempotent(t *testing.T) {
	input := `[other]
key = "val"
`
	first := UpsertCodexEngramBlock(input, "")
	second := UpsertCodexEngramBlock(first, "")

	if first != second {
		t.Fatalf("UpsertCodexEngramBlock is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	count := strings.Count(second, "[mcp_servers.engram]")
	if count != 1 {
		t.Fatalf("after two runs: expected 1 [mcp_servers.engram] block, got %d; result:\n%s", count, second)
	}
}

func TestUpsertCodexEngramBlockWindowsPath(t *testing.T) {
	// Windows paths contain backslashes which must be escaped in TOML double-quoted strings.
	// \U would be interpreted as a Unicode escape sequence → parse error.
	windowsCmd := `C:\Users\PERC\AppData\Local\engram\bin\engram.exe`
	result := UpsertCodexEngramBlock("", windowsCmd)

	// TOML double-quoted string must have double backslashes.
	want := `command = "C:\\Users\\PERC\\AppData\\Local\\engram\\bin\\engram.exe"`
	if !strings.Contains(result, want) {
		t.Fatalf("result missing properly escaped Windows path;\nwant substring: %s\ngot:\n%s", want, result)
	}
}

// ─── UpsertTopLevelTOMLString ─────────────────────────────────────────────────

func TestUpsertTopLevelTOMLString_NewKey(t *testing.T) {
	input := `[mcp_servers.engram]
command = "engram"
`
	result := UpsertTopLevelTOMLString(input, "model_instructions_file", "/home/user/.codex/instructions.md")

	if !strings.Contains(result, `model_instructions_file = "/home/user/.codex/instructions.md"`) {
		t.Fatalf("result missing model_instructions_file key; got:\n%s", result)
	}
	// Must appear before the first [section].
	idx := strings.Index(result, "model_instructions_file")
	sectionIdx := strings.Index(result, "[mcp_servers.engram]")
	if idx > sectionIdx {
		t.Fatalf("model_instructions_file should appear before [mcp_servers.engram]; got:\n%s", result)
	}
}

func TestUpsertTopLevelTOMLString_ReplaceKey(t *testing.T) {
	input := `model_instructions_file = "/old/path.md"

[mcp_servers.engram]
command = "engram"
`
	result := UpsertTopLevelTOMLString(input, "model_instructions_file", "/new/path.md")

	if !strings.Contains(result, `model_instructions_file = "/new/path.md"`) {
		t.Fatalf("result missing updated value; got:\n%s", result)
	}
	if strings.Contains(result, "/old/path.md") {
		t.Fatalf("result still has old value; got:\n%s", result)
	}
	count := strings.Count(result, "model_instructions_file")
	if count != 1 {
		t.Fatalf("expected 1 model_instructions_file, got %d; result:\n%s", count, result)
	}
}

func TestUpsertTopLevelTOMLString_Idempotent(t *testing.T) {
	input := `[mcp_servers.engram]
command = "engram"
`
	first := UpsertTopLevelTOMLString(input, "model_instructions_file", "/path/instructions.md")
	second := UpsertTopLevelTOMLString(first, "model_instructions_file", "/path/instructions.md")

	if first != second {
		t.Fatalf("UpsertTopLevelTOMLString is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// ─── UpsertCodexMCPServerBlock ────────────────────────────────────────────────

func TestUpsertCodexMCPServerBlock_Empty(t *testing.T) {
	result := UpsertCodexMCPServerBlock("", "context7", "npx", []string{"-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"})

	if !strings.Contains(result, "[mcp_servers.context7]") {
		t.Fatalf("result missing [mcp_servers.context7]; got:\n%s", result)
	}
	if !strings.Contains(result, `command = "npx"`) {
		t.Fatalf("result missing command = \"npx\"; got:\n%s", result)
	}
	if !strings.Contains(result, `"--package=@upstash/context7-mcp@2.2.5"`) {
		t.Fatalf("result missing pinned version arg; got:\n%s", result)
	}
	if !strings.HasSuffix(result, "\n") {
		t.Fatalf("result does not end with newline; got:\n%q", result)
	}
}

func TestUpsertCodexMCPServerBlock_ReplacesExisting(t *testing.T) {
	input := `[other_section]
key = "value"

[mcp_servers.context7]
command = "npx"
args = ["-y", "@upstash/context7-mcp@1.0.0"]

[another_section]
foo = "bar"
`
	result := UpsertCodexMCPServerBlock(input, "context7", "npx", []string{"-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"})

	count := strings.Count(result, "[mcp_servers.context7]")
	if count != 1 {
		t.Fatalf("expected 1 [mcp_servers.context7] block, got %d; result:\n%s", count, result)
	}

	if strings.Contains(result, "@upstash/context7-mcp@1.0.0") {
		t.Fatalf("result still contains stale args; got:\n%s", result)
	}
	if !strings.Contains(result, "[other_section]") {
		t.Fatalf("result missing [other_section]; got:\n%s", result)
	}
	if !strings.Contains(result, "[another_section]") {
		t.Fatalf("result missing [another_section]; got:\n%s", result)
	}
}

func TestUpsertCodexMCPServerBlock_Idempotent(t *testing.T) {
	input := `[other]
key = "val"
`
	args := []string{"-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"}
	first := UpsertCodexMCPServerBlock(input, "context7", "npx", args)
	second := UpsertCodexMCPServerBlock(first, "context7", "npx", args)

	if first != second {
		t.Fatalf("UpsertCodexMCPServerBlock is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	count := strings.Count(second, "[mcp_servers.context7]")
	if count != 1 {
		t.Fatalf("after two runs: expected 1 [mcp_servers.context7] block, got %d; result:\n%s", count, second)
	}
}

func TestUpsertCodexMCPServerBlock_PreservesEngramBlock(t *testing.T) {
	input := `[mcp_servers.engram]
command = "engram"
args = ["mcp", "--tools=agent"]
`
	result := UpsertCodexMCPServerBlock(input, "context7", "npx", []string{"-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"})

	if !strings.Contains(result, "[mcp_servers.engram]") {
		t.Fatalf("result missing [mcp_servers.engram] after context7 upsert; got:\n%s", result)
	}
	if !strings.Contains(result, `command = "engram"`) {
		t.Fatalf("result missing engram command after context7 upsert; got:\n%s", result)
	}
	if !strings.Contains(result, "[mcp_servers.context7]") {
		t.Fatalf("result missing [mcp_servers.context7]; got:\n%s", result)
	}

	engramCount := strings.Count(result, "[mcp_servers.engram]")
	if engramCount != 1 {
		t.Fatalf("expected 1 [mcp_servers.engram] block, got %d; result:\n%s", engramCount, result)
	}
}

func TestUpsertCodexMCPServerBlock_EscapesBackslashes(t *testing.T) {
	// Windows-style path in command must have backslashes doubled in TOML double-quoted strings.
	winCmd := `C:\Users\PERC\AppData\Roaming\npm\npx.cmd`
	result := UpsertCodexMCPServerBlock("", "context7", winCmd, []string{`C:\some\arg\path`})

	wantCmd := `command = "C:\\Users\\PERC\\AppData\\Roaming\\npm\\npx.cmd"`
	if !strings.Contains(result, wantCmd) {
		t.Fatalf("result missing properly escaped Windows command;\nwant substring: %s\ngot:\n%s", wantCmd, result)
	}

	wantArg := `"C:\\some\\arg\\path"`
	if !strings.Contains(result, wantArg) {
		t.Fatalf("result missing properly escaped Windows arg;\nwant substring: %s\ngot:\n%s", wantArg, result)
	}
}

func TestUpsertCodexRemoteMCPServerBlock_ReplacesLegacyLocalBlock(t *testing.T) {
	input := `[mcp_servers.context7]
command = "npx"
args = ["-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"]

[mcp_servers.engram]
command = "engram"
args = ["mcp", "--tools=agent"]
`
	result := UpsertCodexRemoteMCPServerBlock(input, "context7", "https://mcp.context7.com/mcp")

	if count := strings.Count(result, "[mcp_servers.context7]"); count != 1 {
		t.Fatalf("expected 1 [mcp_servers.context7], got %d; result:\n%s", count, result)
	}
	if !strings.Contains(result, `url = "https://mcp.context7.com/mcp"`) {
		t.Fatalf("result missing remote Context7 URL; got:\n%s", result)
	}
	if strings.Contains(result, `command = "npx"`) || strings.Contains(result, "context7-mcp") {
		t.Fatalf("legacy local Context7 config survived migration; got:\n%s", result)
	}
	if !strings.Contains(result, "[mcp_servers.engram]") {
		t.Fatalf("result missing [mcp_servers.engram]; got:\n%s", result)
	}
}

func TestUpsertCodexRemoteMCPServerBlock_Idempotent(t *testing.T) {
	input := `[other]
key = "value"
`
	first := UpsertCodexRemoteMCPServerBlock(input, "context7", "https://mcp.context7.com/mcp")
	second := UpsertCodexRemoteMCPServerBlock(first, "context7", "https://mcp.context7.com/mcp")

	if first != second {
		t.Fatalf("UpsertCodexRemoteMCPServerBlock is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// ─── UpsertTOMLTableKey ───────────────────────────────────────────────────────

func TestUpsertTOMLTableKey_CreatesSection(t *testing.T) {
	// When the [features] section does not exist, it must be appended with the key.
	result := UpsertTOMLTableKey("", "features", "multi_agent", "false")

	count := strings.Count(result, "[features]")
	if count != 1 {
		t.Fatalf("expected 1 [features] header, got %d; result:\n%s", count, result)
	}
	if !strings.Contains(result, "multi_agent = false") {
		t.Fatalf("result missing multi_agent = false; got:\n%s", result)
	}
	if !strings.HasSuffix(result, "\n") {
		t.Fatalf("result does not end with newline; got:\n%q", result)
	}
}

func TestUpsertTOMLTableKey_ReplacesKeyInSection(t *testing.T) {
	input := "[features]\nmulti_agent = true\n"
	result := UpsertTOMLTableKey(input, "features", "multi_agent", "false")

	count := strings.Count(result, "[features]")
	if count != 1 {
		t.Fatalf("expected 1 [features] header, got %d; result:\n%s", count, result)
	}
	if !strings.Contains(result, "multi_agent = false") {
		t.Fatalf("result missing multi_agent = false; got:\n%s", result)
	}
	if strings.Contains(result, "multi_agent = true") {
		t.Fatalf("result still has old value multi_agent = true; got:\n%s", result)
	}
	count2 := strings.Count(result, "multi_agent")
	if count2 != 1 {
		t.Fatalf("expected 1 multi_agent key, got %d; result:\n%s", count2, result)
	}
}

func TestUpsertTOMLTableKey_PreservesOtherTables(t *testing.T) {
	// Upserting [features].multi_agent must not disturb [agents] or top-level keys.
	input := `model = "gpt-4o"

[agents]
max_threads = 4
max_depth = 2
`
	result := UpsertTOMLTableKey(input, "features", "multi_agent", "false")

	if !strings.Contains(result, `model = "gpt-4o"`) {
		t.Fatalf("result missing top-level model key; got:\n%s", result)
	}
	if !strings.Contains(result, "[agents]") {
		t.Fatalf("result missing [agents] section; got:\n%s", result)
	}
	if !strings.Contains(result, "max_threads = 4") {
		t.Fatalf("result missing max_threads; got:\n%s", result)
	}
	if !strings.Contains(result, "[features]") {
		t.Fatalf("result missing new [features] section; got:\n%s", result)
	}
	if !strings.Contains(result, "multi_agent = false") {
		t.Fatalf("result missing multi_agent; got:\n%s", result)
	}
}

func TestUpsertTOMLTableKey_Idempotent(t *testing.T) {
	input := "[agents]\nmax_threads = 2\n"
	first := UpsertTOMLTableKey(input, "agents", "max_threads", "4")
	second := UpsertTOMLTableKey(first, "agents", "max_threads", "4")

	if first != second {
		t.Fatalf("UpsertTOMLTableKey is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	count := strings.Count(second, "max_threads")
	if count != 1 {
		t.Fatalf("after two runs: expected 1 max_threads key, got %d; result:\n%s", count, second)
	}
}

func TestUpsertTOMLTableKey_BareValues(t *testing.T) {
	// Boolean and integer rawValues must be written unquoted.
	r1 := UpsertTOMLTableKey("", "features", "multi_agent", "false")
	if !strings.Contains(r1, "multi_agent = false") {
		t.Fatalf("bool false must be bare (no quotes); got:\n%s", r1)
	}
	if strings.Contains(r1, `"false"`) {
		t.Fatalf("bool false must NOT be quoted; got:\n%s", r1)
	}

	r2 := UpsertTOMLTableKey("", "agents", "max_threads", "4")
	if !strings.Contains(r2, "max_threads = 4") {
		t.Fatalf("integer 4 must be bare (no quotes); got:\n%s", r2)
	}
	if strings.Contains(r2, `"4"`) {
		t.Fatalf("integer 4 must NOT be quoted; got:\n%s", r2)
	}
}

func TestUpsertTOMLTableKey_MultipleKeysInSection(t *testing.T) {
	// Upserting a second key to an existing section keeps the first key intact.
	input := "[agents]\nmax_threads = 4\n"
	result := UpsertTOMLTableKey(input, "agents", "max_depth", "2")

	if !strings.Contains(result, "max_threads = 4") {
		t.Fatalf("result missing original max_threads key; got:\n%s", result)
	}
	if !strings.Contains(result, "max_depth = 2") {
		t.Fatalf("result missing new max_depth key; got:\n%s", result)
	}
	count := strings.Count(result, "[agents]")
	if count != 1 {
		t.Fatalf("expected 1 [agents] header, got %d; result:\n%s", count, result)
	}
}

func TestUpsertTOMLTableKey_ScopedToTargetSection(t *testing.T) {
	// A same-named key in [other] must NOT be touched when upserting into [agents].
	input := `[other]
max_threads = 99

[agents]
max_threads = 2
`
	result := UpsertTOMLTableKey(input, "agents", "max_threads", "4")

	// [agents].max_threads updated; [other].max_threads untouched.
	if strings.Count(result, "max_threads = 99") != 1 {
		t.Fatalf("[other].max_threads=99 must be preserved exactly once; got:\n%s", result)
	}
	if strings.Count(result, "max_threads = 4") != 1 {
		t.Fatalf("[agents].max_threads=4 must appear exactly once; got:\n%s", result)
	}
	if strings.Contains(result, "max_threads = 2") {
		t.Fatalf("old [agents].max_threads=2 must be removed; got:\n%s", result)
	}
}

// ─── RemoveTOMLTableKeys ─────────────────────────────────────────────────────

func TestRemoveTOMLTableKeys_RemovesOnlyTargetSectionKeys(t *testing.T) {
	input := `model = "gpt-5"

[permissions.gentle-dev.filesystem.":workspace_roots"]
"**/.git" = "write"
"**/.git/**" = "write"
".git/**" = "write"
"**/.env" = "deny"

[other]
"**/.git" = "write"
`
	result := RemoveTOMLTableKeys(input, `permissions.gentle-dev.filesystem.":workspace_roots"`, []string{
		`"**/.git"`,
		`"**/.git/**"`,
	})

	if strings.Count(result, `"**/.git" = "write"`) != 1 {
		t.Fatalf("same key outside target section should be preserved once; got:\n%s", result)
	}
	if strings.Contains(result, `"**/.git/**" = "write"`) {
		t.Fatalf("result still has invalid target section key; got:\n%s", result)
	}
	if !strings.Contains(result, `".git/**" = "write"`) {
		t.Fatalf("result removed valid git rule; got:\n%s", result)
	}
	if !strings.Contains(result, `"**/.env" = "deny"`) {
		t.Fatalf("result removed env deny rule; got:\n%s", result)
	}
	if !strings.Contains(result, `[other]`) {
		t.Fatalf("result removed other section; got:\n%s", result)
	}
}

func TestRemoveTOMLTableKeys_Idempotent(t *testing.T) {
	input := `[permissions.gentle-dev.filesystem.":workspace_roots"]
"**/.git" = "write"
".git/**" = "write"
`
	first := RemoveTOMLTableKeys(input, `permissions.gentle-dev.filesystem.":workspace_roots"`, []string{`"**/.git"`})
	second := RemoveTOMLTableKeys(first, `permissions.gentle-dev.filesystem.":workspace_roots"`, []string{`"**/.git"`})

	if first != second {
		t.Fatalf("RemoveTOMLTableKeys is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestRemoveTOMLTableTreeRemovesTableAndSubtables(t *testing.T) {
	input := `model = "gpt-5.5"

[permissions.gentle-dev]
description = "injected"

[permissions.gentle-dev.network]
enabled = true

[permissions.gentle-dev.filesystem.":workspace_roots"]
"." = "write"

[mcp_servers.engram]
command = "engram"
`
	want := `model = "gpt-5.5"

[mcp_servers.engram]
command = "engram"
`
	if got := RemoveTOMLTableTree(input, "permissions.gentle-dev"); got != want {
		t.Fatalf("RemoveTOMLTableTree() = %q, want %q", got, want)
	}
}

func TestRemoveTOMLTableTreePreservesSimilarlyNamedTables(t *testing.T) {
	input := `[permissions.gentle-dev]
a = 1

[permissions.gentle-devtools]
b = 2

[permissions.custom]
c = 3
`
	want := `[permissions.gentle-devtools]
b = 2

[permissions.custom]
c = 3
`
	if got := RemoveTOMLTableTree(input, "permissions.gentle-dev"); got != want {
		t.Fatalf("RemoveTOMLTableTree() = %q, want %q", got, want)
	}
}

func TestRemoveTOMLTableTreeWithoutTargetIsByteIdentical(t *testing.T) {
	input := "custom = true\n\n[other] # keep\nkey = \"value\""
	if got := RemoveTOMLTableTree(input, "permissions.gentle-dev"); got != input {
		t.Fatalf("RemoveTOMLTableTree() = %q, want untouched %q", got, input)
	}
}

func TestRemoveTOMLTableTreeRemovesTopLevelDottedKeys(t *testing.T) {
	input := `permissions.gentle-dev.workspace_roots."~" = true
permissions.custom.workspace_roots."~" = true

[other]
permissions.gentle-dev.x = 1
`
	want := `permissions.custom.workspace_roots."~" = true

[other]
permissions.gentle-dev.x = 1
`
	if got := RemoveTOMLTableTree(input, "permissions.gentle-dev"); got != want {
		t.Fatalf("RemoveTOMLTableTree() = %q, want %q", got, want)
	}
}

func TestRemoveTopLevelTOMLKeyIfValueRemovesExactMatch(t *testing.T) {
	input := `model = "gpt-5.5"
approval_policy = "on-request"

[table]
approval_policy = "on-request"
`
	want := `model = "gpt-5.5"

[table]
approval_policy = "on-request"
`
	if got := RemoveTopLevelTOMLKeyIfValue(input, "approval_policy", "\"on-request\""); got != want {
		t.Fatalf("RemoveTopLevelTOMLKeyIfValue() = %q, want %q", got, want)
	}
}

func TestRemoveTopLevelTOMLKeyIfValueKeepsCustomizedValue(t *testing.T) {
	input := "approval_policy = \"never\"\ndefault_permissions = \"other\"\n"
	got := RemoveTopLevelTOMLKeyIfValue(input, "approval_policy", "\"on-request\"")
	got = RemoveTopLevelTOMLKeyIfValue(got, "default_permissions", "\"gentle-dev\"")
	if got != input {
		t.Fatalf("RemoveTopLevelTOMLKeyIfValue() = %q, want untouched %q", got, input)
	}
}

func TestRemoveTOMLTableTreeRecognizesQuotedHeaderSegments(t *testing.T) {
	input := `[permissions."gentle-dev".workspace_roots]
"~" = true

[permissions.'gentle-dev'.filesystem]
":minimal" = "read"

[permissions."custom".workspace_roots]
"~" = true
`
	want := `[permissions."custom".workspace_roots]
"~" = true
`
	if got := RemoveTOMLTableTree(input, "permissions.gentle-dev"); got != want {
		t.Fatalf("RemoveTOMLTableTree() = %q, want %q", got, want)
	}
}

func TestRemoveTOMLTableTreeRemovesQuotedDottedKeys(t *testing.T) {
	input := `permissions."gentle-dev".workspace_roots."~" = true
permissions.gentle-dev.workspace_roots."~/project" = true
permissions."custom".workspace_roots."~" = true

[other]
"gentle-dev" = true
`
	want := `permissions."custom".workspace_roots."~" = true

[other]
"gentle-dev" = true
`
	if got := RemoveTOMLTableTree(input, "permissions.gentle-dev"); got != want {
		t.Fatalf("RemoveTOMLTableTree() = %q, want %q", got, want)
	}
}

func TestRemoveTOMLTableTreeKeepsUserContentAfterMalformedHeader(t *testing.T) {
	input := "[permissions.gentle-dev]\na = 1\n\n[bad header no closing bracket\nb = 2\n"
	want := "[bad header no closing bracket\nb = 2\n"
	if got := RemoveTOMLTableTree(input, "permissions.gentle-dev"); got != want {
		t.Fatalf("RemoveTOMLTableTree() = %q, want %q", got, want)
	}
}

func TestRemoveTOMLTableTreeMalformedHeaderBeforeMatchIsByteIdentical(t *testing.T) {
	input := "[broken header\npermissions.gentle-dev.x = 1\n"
	if got := RemoveTOMLTableTree(input, "permissions.gentle-dev"); got != input {
		t.Fatalf("RemoveTOMLTableTree() = %q, want untouched %q", got, input)
	}
}

func TestRemoveTopLevelTOMLKeyIfValueRecognizesQuotedKey(t *testing.T) {
	input := `"approval_policy" = "on-request"
model = "gpt-5.5"
`
	want := `model = "gpt-5.5"
`
	if got := RemoveTopLevelTOMLKeyIfValue(input, "approval_policy", "\"on-request\""); got != want {
		t.Fatalf("RemoveTopLevelTOMLKeyIfValue() = %q, want %q", got, want)
	}
}

func TestRemoveTOMLTableTreeIgnoresHeadersInsideMultilineBasicStrings(t *testing.T) {
	input := `message = """
[permissions.gentle-dev]
user content
"""
permissions.gentle-dev.workspace_roots."~" = true
[other]
keep = true
`
	want := `message = """
[permissions.gentle-dev]
user content
"""
[other]
keep = true
`
	if got := RemoveTOMLTableTree(input, "permissions.gentle-dev"); got != want {
		t.Fatalf("RemoveTOMLTableTree() = %q, want %q", got, want)
	}
}

func TestRemoveTopLevelTOMLKeyIfValueIgnoresHeadersInsideMultilineLiteralStrings(t *testing.T) {
	input := `message = '''
[table]
user content
'''
approval_policy = "on-request"
`
	want := `message = '''
[table]
user content
'''
`
	if got := RemoveTopLevelTOMLKeyIfValue(input, "approval_policy", "\"on-request\""); got != want {
		t.Fatalf("RemoveTopLevelTOMLKeyIfValue() = %q, want %q", got, want)
	}
}

func TestTOMLRemovalRejectsGoOnlyEscapesInQuotedKeys(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		remove func(string) string
	}{
		{
			name:  "top-level key with hexadecimal escape",
			input: "\"approval\\x5fpolicy\" = \"on-request\"\n",
			remove: func(input string) string {
				return RemoveTopLevelTOMLKeyIfValue(input, "approval_policy", "\"on-request\"")
			},
		},
		{
			name:  "table key part with hexadecimal escape",
			input: "[permissions.\"gentle\\x2ddev\"]\nkeep = true\n",
			remove: func(input string) string {
				return RemoveTOMLTableTree(input, "permissions.gentle-dev")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.remove(tt.input); got != tt.input {
				t.Fatalf("removal changed malformed TOML to %q, want untouched %q", got, tt.input)
			}
		})
	}
}

func TestRemoveTOMLTableTreeRemovesMultilineDottedAssignments(t *testing.T) {
	tests := []struct {
		name, assignment string
	}{
		{"basic", "permissions.gentle-dev.note = \"\"\"\n[target content]\n\"\"\"\n"},
		{"literal", "permissions.gentle-dev.note = '''\n[target content]\n'''\n"},
	}
	want := `message = """
[not-a-header]
"""
[other]
keep = true
`
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveTOMLTableTree(tt.assignment+want, "permissions.gentle-dev"); got != want {
				t.Fatalf("RemoveTOMLTableTree() = %q, want %q", got, want)
			}
		})
	}
}

func TestRemoveTOMLTableTreeRemovesMultilineArrayAssignment(t *testing.T) {
	input := `permissions.gentle-dev.rules = ["""
  ] multiline string bracket
  """,
  "[string bracket]",
  # ] comment bracket
  [
    "nested",
  ],
]
unrelated = [
  "]",
  # [ comment bracket
  "keep",
]
[other]
keep = true
`
	want := `unrelated = [
  "]",
  # [ comment bracket
  "keep",
]
[other]
keep = true
`
	if got := RemoveTOMLTableTree(input, "permissions.gentle-dev"); got != want {
		t.Fatalf("RemoveTOMLTableTree() = %q, want %q", got, want)
	}
}

func TestRemoveTOMLTableTreeRemovesNestedArrayInsideTargetTable(t *testing.T) {
	input := `[permissions.gentle-dev]
rules = [
  ["nested"],
]
[other]
keep = true
`
	want := `[other]
keep = true
`
	if got := RemoveTOMLTableTree(input, "permissions.gentle-dev"); got != want {
		t.Fatalf("RemoveTOMLTableTree() = %q, want %q", got, want)
	}
}

func TestRemoveTopLevelTOMLKeysAfterNestedArray(t *testing.T) {
	input := `matrix = [
  ["[string bracket]"],
  # ] comment bracket
  [
    "nested",
  ],
]
approval_policy = "on-request"
default_permissions = "gentle-dev"
[other]
approval_policy = "on-request"
default_permissions = "gentle-dev"
`
	want := `matrix = [
  ["[string bracket]"],
  # ] comment bracket
  [
    "nested",
  ],
]
[other]
approval_policy = "on-request"
default_permissions = "gentle-dev"
`
	got := RemoveTopLevelTOMLKeyIfValue(input, "approval_policy", "\"on-request\"")
	got = RemoveTopLevelTOMLKeyIfValue(got, "default_permissions", "\"gentle-dev\"")
	if got != want {
		t.Fatalf("top-level cleanup = %q, want %q", got, want)
	}
}
