package filemerge

import (
	"strings"
	"testing"
)

// ─── UpsertYAMLMCPServerBlock ─────────────────────────────────────────────────

// TestUpsertYAMLMCPServerBlock covers the full golden matrix required by the spec
// and design (scenarios #1–#9). Scenario #10 (context7 wrapper) is in
// TestUpsertHermesContext7Block below.
func TestUpsertYAMLMCPServerBlock(t *testing.T) {
	t.Parallel()

	engram := func(content string) string {
		return UpsertYAMLMCPServerBlock(content, "engram", "engram", []string{"mcp", "--tools=agent"}, nil)
	}

	// fn receives the subtest *testing.T so that closures that perform inline
	// assertions (e.g. idempotency checks) report failures on the correct subtest
	// rather than on the parent t (which would be a data-race in t.Parallel() tests).
	tests := []struct {
		name   string
		input  string
		fn     func(*testing.T, string) string
		checks []string // substrings that must be present
		absent []string // substrings that must be absent
		suffix string   // if non-empty, result must end with this
		exact  string   // if non-empty, result must equal this exactly
	}{
		{
			// #1: empty/absent content → engram block created from scratch
			name:  "empty_content_creates_mcp_servers",
			input: "",
			fn:    func(_ *testing.T, s string) string { return engram(s) },
			checks: []string{
				"mcp_servers:\n",
				"  engram:\n",
				"    command: engram\n",
				"    args:\n",
				"      - mcp\n",
				"      - --tools=agent\n",
			},
			suffix: "\n",
		},
		{
			// #2: mcp_servers: absent, other top-level keys present → keys/comments preserved; section appended
			name: "other_top_level_keys_preserved",
			input: `# user config
model: gpt-4o
temperature: 0.7
`,
			fn: func(_ *testing.T, s string) string { return engram(s) },
			checks: []string{
				"# user config\n",
				"model: gpt-4o\n",
				"temperature: 0.7\n",
				"mcp_servers:\n",
				"  engram:\n",
				"    command: engram\n",
			},
			suffix: "\n",
		},
		{
			// #3: mcp_servers: present, no engram entry → user server preserved, engram appended as sibling
			name: "user_server_preserved_engram_appended",
			input: `mcp_servers:
  myserver:
    command: myserver
    args:
      - --flag
`,
			fn: func(_ *testing.T, s string) string { return engram(s) },
			checks: []string{
				"  myserver:\n",
				"    command: myserver\n",
				"    args:\n",
				"      - --flag\n",
				"  engram:\n",
				"    command: engram\n",
			},
		},
		{
			// #4: idempotency — output of #3 fed back in → byte-identical result.
			// The closure receives the subtest t so failures are attributed correctly
			// even when the subtest runs in parallel.
			name: "idempotent_rerun",
			input: `mcp_servers:
  myserver:
    command: myserver
    args:
      - --flag
`,
			fn: func(st *testing.T, content string) string {
				first := engram(content)
				second := engram(first)
				if first != second {
					st.Errorf("idempotency violated:\nfirst:\n%s\nsecond:\n%s", first, second)
				}
				return second
			},
			checks: []string{
				"  myserver:\n",
				"  engram:\n",
			},
			absent: []string{
				// Must not duplicate engram
				"  engram:\n  engram:\n",
			},
		},
		{
			// #5: upsert replaces stale engram block (old args) → old block removed, fresh block appended, siblings intact
			name: "stale_block_replaced",
			input: `mcp_servers:
  myserver:
    command: myserver
  engram:
    command: engram
    args:
      - mcp
`,
			fn: func(_ *testing.T, s string) string { return engram(s) },
			checks: []string{
				"  myserver:\n",
				"  engram:\n",
				"      - --tools=agent\n",
			},
			absent: []string{
				// Old engram args-only entry must be gone; replaced with full block
			},
		},
		{
			// #6: user comments outside managed block → all comments preserved verbatim
			name: "comments_outside_block_preserved",
			input: `# top-level comment
model: gpt-4o

# comment before mcp
mcp_servers:
  # comment inside mcp
  myserver:
    command: myserver

# trailing comment
`,
			fn: func(_ *testing.T, s string) string { return engram(s) },
			checks: []string{
				"# top-level comment\n",
				"# comment before mcp\n",
				"# trailing comment\n",
				"  # comment inside mcp\n",
				"  myserver:\n",
				"  engram:\n",
			},
		},
		{
			// #7: two managed servers coexist (engram then context7) → both present, both at 2-space indent, idempotent.
			// The closure receives the subtest t so the idempotency assertion is attributed correctly.
			name:  "two_managed_servers_coexist",
			input: "",
			fn: func(st *testing.T, content string) string {
				withEngram := engram(content)
				withBoth := UpsertYAMLMCPServerBlock(withEngram, "context7", "npx",
					[]string{"-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"}, nil)
				// idempotency check
				withBothAgain := UpsertYAMLMCPServerBlock(withBoth, "context7", "npx",
					[]string{"-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"}, nil)
				if withBoth != withBothAgain {
					st.Errorf("two-server idempotency violated:\nfirst:\n%s\nsecond:\n%s", withBoth, withBothAgain)
				}
				return withBoth
			},
			checks: []string{
				"  engram:\n",
				"    command: engram\n",
				"  context7:\n",
				"    command: npx\n",
			},
		},
		{
			// #8: CRLF input → normalized to \n, single trailing \n
			name:  "crlf_normalized",
			input: "model: gpt-4o\r\n",
			fn:    func(_ *testing.T, s string) string { return engram(s) },
			checks: []string{
				"model: gpt-4o\n",
				"mcp_servers:\n",
			},
			absent: []string{"\r\n"},
			suffix: "\n",
		},
		{
			// #9: env map rendered → env: sub-block with 2-space-deeper KV pairs
			name:  "env_block_rendered",
			input: "",
			fn: func(_ *testing.T, content string) string {
				return UpsertYAMLMCPServerBlock(content, "engram", "engram",
					[]string{"mcp", "--tools=agent"},
					map[string]string{"ENGRAM_HOME": "/data/engram"})
			},
			checks: []string{
				"    env:\n",
				"      ENGRAM_HOME: /data/engram\n",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.fn(t, tt.input)
			for _, want := range tt.checks {
				if !strings.Contains(got, want) {
					t.Errorf("missing expected content %q in:\n%s", want, got)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(got, absent) {
					t.Errorf("unexpected content %q found in:\n%s", absent, got)
				}
			}
			if tt.suffix != "" && !strings.HasSuffix(got, tt.suffix) {
				t.Errorf("result does not end with %q; got:\n%q", tt.suffix, got)
			}
			if tt.exact != "" && got != tt.exact {
				t.Errorf("result mismatch:\nwant:\n%s\ngot:\n%s", tt.exact, got)
			}
		})
	}
}

// #10: UpsertHermesContext7Block on empty → pinned versions.Context7MCP args emitted
func TestUpsertHermesContext7Block(t *testing.T) {
	t.Parallel()

	got := UpsertHermesContext7Block("")

	if !strings.Contains(got, "  context7:\n") {
		t.Fatalf("missing context7 server key; got:\n%s", got)
	}
	if !strings.Contains(got, "    command: npx\n") {
		t.Fatalf("missing command: npx; got:\n%s", got)
	}
	// Must contain the pinned context7 package arg.
	if !strings.Contains(got, "--package=@upstash/context7-mcp@") {
		t.Fatalf("missing pinned context7-mcp package arg; got:\n%s", got)
	}
	if !strings.Contains(got, "      - context7-mcp\n") {
		t.Fatalf("missing context7-mcp arg; got:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("result does not end with newline; got:\n%q", got)
	}

	// Idempotency.
	second := UpsertHermesContext7Block(got)
	if got != second {
		t.Fatalf("UpsertHermesContext7Block not idempotent:\nfirst:\n%s\nsecond:\n%s", got, second)
	}
}

// TestUpsertHermesEngramBlock verifies the engram-specific convenience wrapper.
func TestUpsertHermesEngramBlock(t *testing.T) {
	t.Parallel()

	t.Run("empty_content_default_command", func(t *testing.T) {
		t.Parallel()
		got := UpsertHermesEngramBlock("", "")
		if !strings.Contains(got, "  engram:\n") {
			t.Fatalf("missing engram server key; got:\n%s", got)
		}
		if !strings.Contains(got, "    command: engram\n") {
			t.Fatalf("missing default command 'engram'; got:\n%s", got)
		}
		if !strings.Contains(got, "      - --tools=agent\n") {
			t.Fatalf("missing --tools=agent arg; got:\n%s", got)
		}
	})

	t.Run("custom_command_used", func(t *testing.T) {
		t.Parallel()
		got := UpsertHermesEngramBlock("", "/usr/local/bin/engram")
		if !strings.Contains(got, "    command: /usr/local/bin/engram\n") {
			t.Fatalf("missing custom command; got:\n%s", got)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		t.Parallel()
		first := UpsertHermesEngramBlock("", "engram")
		second := UpsertHermesEngramBlock(first, "engram")
		if first != second {
			t.Fatalf("not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
		}
		if strings.Count(second, "  engram:\n") != 1 {
			t.Fatalf("expected exactly 1 engram block; got:\n%s", second)
		}
	})
}

// ─── ReadYAMLMCPServerCommand ─────────────────────────────────────────────────

// TestReadYAMLMCPServerCommand covers T-04: all recovery shapes.
func TestReadYAMLMCPServerCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		serverID string
		wantCmd  string
		wantOK   bool
	}{
		{
			// scalar command: command: engram → ("engram", true)
			name: "scalar_command",
			content: `mcp_servers:
  engram:
    command: engram
    args:
      - mcp
      - --tools=agent
`,
			serverID: "engram",
			wantCmd:  "engram",
			wantOK:   true,
		},
		{
			// list command: command:\n  - /path/engram items → first element ("/path/engram", true)
			name: "list_command_first_element",
			content: `mcp_servers:
  engram:
    command:
      - /path/to/engram
      - mcp
      - --tools=agent
`,
			serverID: "engram",
			wantCmd:  "/path/to/engram",
			wantOK:   true,
		},
		{
			// server absent under mcp_servers: → ("", false)
			name: "server_absent_under_mcp_servers",
			content: `mcp_servers:
  context7:
    command: npx
`,
			serverID: "engram",
			wantCmd:  "",
			wantOK:   false,
		},
		{
			// mcp_servers: key absent entirely → ("", false)
			name: "mcp_servers_key_absent",
			content: `model: gpt-4o
temperature: 0.7
`,
			serverID: "engram",
			wantCmd:  "",
			wantOK:   false,
		},
		{
			// comment lines inside/around block ignored without breaking recovery
			name: "comment_lines_ignored",
			content: `# top comment
mcp_servers:
  # comment about servers
  engram:
    # comment inside server
    command: engram
    args:
      - mcp
`,
			serverID: "engram",
			wantCmd:  "engram",
			wantOK:   true,
		},
		{
			// absolute path command preserved
			name: "absolute_path_command",
			content: `mcp_servers:
  engram:
    command: /custom/path/engram
`,
			serverID: "engram",
			wantCmd:  "/custom/path/engram",
			wantOK:   true,
		},
		{
			// empty content → ("", false)
			name:     "empty_content",
			content:  "",
			serverID: "engram",
			wantCmd:  "",
			wantOK:   false,
		},
		{
			// versioned cellar path (ensure it is recoverable)
			name: "versioned_cellar_path",
			content: `mcp_servers:
  engram:
    command: /opt/homebrew/Cellar/engram/1.2.3/bin/engram
`,
			serverID: "engram",
			wantCmd:  "/opt/homebrew/Cellar/engram/1.2.3/bin/engram",
			wantOK:   true,
		},
		// FIX 1: double-quoted command value must be stripped of quotes
		{
			name: "double_quoted_command_stripped",
			content: `mcp_servers:
  engram:
    command: "engram"
`,
			serverID: "engram",
			wantCmd:  "engram",
			wantOK:   true,
		},
		// FIX 1: single-quoted command value must be stripped of quotes
		{
			name: "single_quoted_command_stripped",
			content: `mcp_servers:
  engram:
    command: 'engram'
`,
			serverID: "engram",
			wantCmd:  "engram",
			wantOK:   true,
		},
		// FIX 2: inline trailing comment must be stripped
		{
			name: "inline_trailing_comment_stripped",
			content: `mcp_servers:
  engram:
    command: engram  # installed via brew
`,
			serverID: "engram",
			wantCmd:  "engram",
			wantOK:   true,
		},
		// FIX 1+2 combined: quoted value with hash inside must NOT be truncated
		{
			name: "quoted_value_with_hash_not_truncated",
			content: `mcp_servers:
  engram:
    command: "engram#x"
`,
			serverID: "engram",
			wantCmd:  "engram#x",
			wantOK:   true,
		},
		// BUG FIX: double-quoted scalar WITH trailing inline comment must strip both
		// quotes AND comment — currently returns `"engram"` (quotes intact).
		{
			name: "double_quoted_with_trailing_comment",
			content: `mcp_servers:
  engram:
    command: "engram" # installed via brew
`,
			serverID: "engram",
			wantCmd:  "engram",
			wantOK:   true,
		},
		// BUG FIX: single-quoted scalar WITH trailing inline comment must strip both
		// quotes AND comment — currently returns `'engram'` (quotes intact).
		{
			name: "single_quoted_with_trailing_comment",
			content: `mcp_servers:
  engram:
    command: 'engram' # note
`,
			serverID: "engram",
			wantCmd:  "engram",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotCmd, gotOK := ReadYAMLMCPServerCommand(tt.content, tt.serverID)
			if gotCmd != tt.wantCmd {
				t.Errorf("command: got %q, want %q", gotCmd, tt.wantCmd)
			}
			if gotOK != tt.wantOK {
				t.Errorf("ok: got %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

// TestReadYAMLMCPServerCommandNitA covers NIT FIX A: a comment with no space after # must
// be skipped when scanning for mcp_servers: (asymmetry fix at the top-level scanner).
func TestReadYAMLMCPServerCommandNitA(t *testing.T) {
	t.Parallel()

	content := "#nospacecomment\nmcp_servers:\n  engram:\n    command: engram\n"
	got, ok := ReadYAMLMCPServerCommand(content, "engram")
	if !ok {
		t.Fatalf("expected ok=true, got false; result %q", got)
	}
	if got != "engram" {
		t.Errorf("command: got %q, want %q", got, "engram")
	}
}

// TestUpsertYAMLMCPServerBlockInlineMCPServers covers FIX 3: when mcp_servers: has an
// inline value (e.g. "mcp_servers: {}"), the function must NOT produce a duplicate
// top-level mcp_servers: key. Output must contain exactly one mcp_servers: key,
// valid nested engram block, and preserve other top-level keys.
func TestUpsertYAMLMCPServerBlockInlineMCPServers(t *testing.T) {
	t.Parallel()

	input := "mcp_servers: {}\nmodel: gpt-4o\n"
	got := UpsertYAMLMCPServerBlock(input, "engram", "engram", []string{"mcp", "--tools=agent"}, nil)

	// Must contain exactly one top-level mcp_servers: key.
	count := strings.Count(got, "\nmcp_servers:")
	// Account for the case where mcp_servers: is at the very beginning of the string.
	if strings.HasPrefix(got, "mcp_servers:") {
		count++
	}
	if count != 1 {
		t.Errorf("expected exactly 1 top-level mcp_servers: key, got %d in:\n%s", count, got)
	}

	// Must contain valid nested engram block.
	if !strings.Contains(got, "  engram:\n") {
		t.Errorf("missing nested engram block in:\n%s", got)
	}
	if !strings.Contains(got, "    command: engram\n") {
		t.Errorf("missing command: engram in:\n%s", got)
	}

	// model: gpt-4o must be preserved.
	if !strings.Contains(got, "model: gpt-4o") {
		t.Errorf("model: gpt-4o not preserved in:\n%s", got)
	}
}

// TestUpsertYAMLMCPServerBlockInlineCommentedKey covers Group A item 1:
// when the existing server key has a trailing inline comment (e.g. "  engram: # managed"),
// UpsertYAMLMCPServerBlock must recognise and replace it — not append a duplicate block.
func TestUpsertYAMLMCPServerBlockInlineCommentedKey(t *testing.T) {
	t.Parallel()

	input := "mcp_servers:\n  engram: # managed\n    command: old-engram\n    args:\n      - mcp\n"
	got := UpsertYAMLMCPServerBlock(input, "engram", "engram", []string{"mcp", "--tools=agent"}, nil)

	count := strings.Count(got, "  engram:")
	if count != 1 {
		t.Errorf("expected exactly 1 engram: key, got %d in:\n%s", count, got)
	}
	if !strings.Contains(got, "    command: engram\n") {
		t.Errorf("missing updated command: engram in:\n%s", got)
	}
	if strings.Contains(got, "old-engram") {
		t.Errorf("old command must be removed; got:\n%s", got)
	}
}

// TestUpsertYAMLMCPServerBlockPreserveMCPServersInlineComment covers Group A item 2:
// when normalizing an inline mcp_servers: value (e.g. "mcp_servers: {}  # note"),
// the trailing inline comment must be preserved in the output.
func TestUpsertYAMLMCPServerBlockPreserveMCPServersInlineComment(t *testing.T) {
	t.Parallel()

	input := "mcp_servers: {}  # note\n"
	got := UpsertYAMLMCPServerBlock(input, "engram", "engram", []string{"mcp", "--tools=agent"}, nil)

	if !strings.Contains(got, "# note") {
		t.Errorf("inline comment on mcp_servers: must be preserved; got:\n%s", got)
	}
	if !strings.Contains(got, "  engram:\n") {
		t.Errorf("missing engram block; got:\n%s", got)
	}
}

// TestUpsertYAMLMCPServerBlockCommentBetweenSiblings covers Group A item 3:
// comment lines between sibling servers must survive after an upsert.
func TestUpsertYAMLMCPServerBlockCommentBetweenSiblings(t *testing.T) {
	t.Parallel()

	input := "mcp_servers:\n  engram:\n    command: old-engram\n  # sibling comment\n  myserver:\n    command: myserver\n"
	got := UpsertYAMLMCPServerBlock(input, "engram", "engram", []string{"mcp", "--tools=agent"}, nil)

	if !strings.Contains(got, "  # sibling comment\n") {
		t.Errorf("comment between siblings must be preserved; got:\n%s", got)
	}
	if !strings.Contains(got, "  myserver:\n") {
		t.Errorf("sibling server must be preserved; got:\n%s", got)
	}
	if !strings.Contains(got, "    command: engram\n") {
		t.Errorf("updated engram block must be present; got:\n%s", got)
	}
}

// TestReadYAMLMCPServerCommandInlineCommentedMCPServersKey covers Group A item 4 (first):
// mcp_servers: with a trailing comment (e.g. "mcp_servers: # note") must be detected.
func TestReadYAMLMCPServerCommandInlineCommentedMCPServersKey(t *testing.T) {
	t.Parallel()

	content := "mcp_servers: # note\n  engram:\n    command: engram\n"
	got, ok := ReadYAMLMCPServerCommand(content, "engram")
	if !ok {
		t.Fatalf("expected ok=true with commented mcp_servers: key; got false, cmd=%q", got)
	}
	if got != "engram" {
		t.Errorf("command: got %q, want %q", got, "engram")
	}
}

// TestReadYAMLMCPServerCommandInlineCommentedServerKey covers Group A item 4 (second):
// a server key with a trailing inline comment (e.g. "  engram: # note") must be detected.
func TestReadYAMLMCPServerCommandInlineCommentedServerKey(t *testing.T) {
	t.Parallel()

	content := "mcp_servers:\n  engram: # note\n    command: engram\n"
	got, ok := ReadYAMLMCPServerCommand(content, "engram")
	if !ok {
		t.Fatalf("expected ok=true with inline-commented server key; got false, cmd=%q", got)
	}
	if got != "engram" {
		t.Errorf("command: got %q, want %q", got, "engram")
	}
}

// TestReadYAMLMCPServerCommandListFormQuotedItem covers Group A item 5 (first):
// list-form command where the first item is double-quoted must strip the quotes.
func TestReadYAMLMCPServerCommandListFormQuotedItem(t *testing.T) {
	t.Parallel()

	content := "mcp_servers:\n  engram:\n    command:\n      - \"engram\"\n      - mcp\n"
	got, ok := ReadYAMLMCPServerCommand(content, "engram")
	if !ok {
		t.Fatalf("expected ok=true; got false, cmd=%q", got)
	}
	if got != "engram" {
		t.Errorf("command: got %q, want %q (quotes must be stripped)", got, "engram")
	}
}

// TestReadYAMLMCPServerCommandListFormInlineComment covers Group A item 5 (second):
// list-form command where the first item has a trailing inline comment must strip the comment.
func TestReadYAMLMCPServerCommandListFormInlineComment(t *testing.T) {
	t.Parallel()

	content := "mcp_servers:\n  engram:\n    command:\n      - engram  # note\n      - mcp\n"
	got, ok := ReadYAMLMCPServerCommand(content, "engram")
	if !ok {
		t.Fatalf("expected ok=true; got false, cmd=%q", got)
	}
	if got != "engram" {
		t.Errorf("command: got %q, want %q (inline comment must be stripped)", got, "engram")
	}
}

// TestNormalizeYAMLScalarQuotedWithComment covers the BUG: when a YAML scalar
// is both quoted AND has a trailing inline comment, normalizeYAMLScalar must
// strip the quotes AND discard the comment — not leave the quotes intact.
// These cases correspond to list-form items like `- "engram"  # note`.
func TestNormalizeYAMLScalarQuotedWithComment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// BUG: double-quoted value with trailing comment — currently returns `"engram"`
		{
			name:  "double_quoted_with_comment",
			input: `"engram"  # note`,
			want:  "engram",
		},
		// BUG: single-quoted value with trailing comment — currently returns `'engram'`
		{
			name:  "single_quoted_with_comment",
			input: `'engram' # x`,
			want:  "engram",
		},
		// REGRESSION: double-quoted, no comment — must still strip quotes
		{
			name:  "double_quoted_no_comment",
			input: `"engram"`,
			want:  "engram",
		},
		// REGRESSION: hash INSIDE quotes (no space-hash comment) — must preserve hash
		{
			name:  "hash_inside_double_quotes",
			input: `"engram#x"`,
			want:  "engram#x",
		},
		// REGRESSION: bare unquoted value with trailing comment — must strip comment
		{
			name:  "bare_with_comment",
			input: `engram # note`,
			want:  "engram",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeYAMLScalar(tt.input)
			if got != tt.want {
				t.Errorf("normalizeYAMLScalar(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestReadYAMLMCPServerCommandListFormQuotedWithComment covers the BUG in list-form:
// a list item that is both quoted AND has a trailing inline comment must return
// the bare value (no quotes, no comment).
func TestReadYAMLMCPServerCommandListFormQuotedWithComment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		// BUG: double-quoted list item with comment
		{
			name:    "double_quoted_list_item_with_comment",
			content: "mcp_servers:\n  engram:\n    command:\n      - \"engram\"  # note\n      - mcp\n",
			want:    "engram",
		},
		// BUG: single-quoted list item with comment
		{
			name:    "single_quoted_list_item_with_comment",
			content: "mcp_servers:\n  engram:\n    command:\n      - 'engram' # x\n      - mcp\n",
			want:    "engram",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ReadYAMLMCPServerCommand(tt.content, "engram")
			if !ok {
				t.Fatalf("expected ok=true; got false, cmd=%q", got)
			}
			if got != tt.want {
				t.Errorf("command: got %q, want %q (quotes+comment must be stripped)", got, tt.want)
			}
		})
	}
}

// TestBuildServerBlockEnvOrdering covers COVERAGE: two+ env keys appear in
// lexicographic order, pinning the existing sort.Strings guarantee.
func TestBuildServerBlockEnvOrdering(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"ZEBRA_KEY": "z",
		"ALPHA_KEY": "a",
		"MIDDLE":    "m",
	}
	got := UpsertYAMLMCPServerBlock("", "engram", "engram", nil, env)

	alphaIdx := strings.Index(got, "ALPHA_KEY")
	middleIdx := strings.Index(got, "MIDDLE")
	zebraIdx := strings.Index(got, "ZEBRA_KEY")

	if alphaIdx == -1 || middleIdx == -1 || zebraIdx == -1 {
		t.Fatalf("one or more env keys missing in:\n%s", got)
	}
	if !(alphaIdx < middleIdx && middleIdx < zebraIdx) {
		t.Errorf("env keys not in lexicographic order: ALPHA_KEY@%d MIDDLE@%d ZEBRA_KEY@%d\n%s",
			alphaIdx, middleIdx, zebraIdx, got)
	}
}
