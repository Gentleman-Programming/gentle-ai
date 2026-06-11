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
