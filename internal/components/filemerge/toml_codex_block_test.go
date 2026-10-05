package filemerge

import (
	"strings"
	"testing"
)

// codexBlockUpserters covers every helper that replaces a whole
// [mcp_servers.<id>] block. Each one must treat header-like lines inside a
// multiline string as value text (issue #5026).
var codexBlockUpserters = []struct {
	name  string
	id    string
	block string
	fn    func(string) string
}{
	{
		name:  "UpsertCodexEngramBlock",
		id:    "engram",
		block: "[mcp_servers.engram]\ncommand = \"engram\"\nargs = [\"mcp\", \"--tools=agent\"]",
		fn: func(content string) string {
			return UpsertCodexEngramBlock(content, "")
		},
	},
	{
		name:  "UpsertCodexMCPServerBlock",
		id:    "context7",
		block: "[mcp_servers.context7]\ncommand = \"npx\"\nargs = [\"-y\", \"context7-mcp\"]",
		fn: func(content string) string {
			return UpsertCodexMCPServerBlock(content, "context7", "npx", []string{"-y", "context7-mcp"})
		},
	},
	{
		name:  "UpsertCodexRemoteMCPServerBlock",
		id:    "context7",
		block: "[mcp_servers.context7]\nurl = \"https://mcp.context7.com/mcp\"",
		fn: func(content string) string {
			return UpsertCodexRemoteMCPServerBlock(content, "context7", "https://mcp.context7.com/mcp")
		},
	},
}

func TestUpsertCodexBlocks_IgnoreTableLikeMultilineValues(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// want is the expected output without the appended managed block.
		want string
	}{
		{
			name: "fake header in basic multiline string without real block",
			input: `model = "gpt-5.5"
developer_instructions = """
Use this config:
[mcp_servers.{{ID}}]
command = "fake"
Keep this text.
"""

[features]
multi_agent = true
`,
			want: `model = "gpt-5.5"
developer_instructions = """
Use this config:
[mcp_servers.{{ID}}]
command = "fake"
Keep this text.
"""

[features]
multi_agent = true`,
		},
		{
			name: "fake header in literal multiline string without real block",
			input: `developer_instructions = '''
[mcp_servers.{{ID}}]
command = "C:\fake"
  indented text stays
'''
`,
			want: `developer_instructions = '''
[mcp_servers.{{ID}}]
command = "C:\fake"
  indented text stays
'''`,
		},
		{
			name: "real block before a literal multiline string with a fake header",
			input: `[mcp_servers.{{ID}}]
command = "old"
args = ["old"]

[profile]
developer_instructions = '''
[mcp_servers.{{ID}}]
command = "fake"
'''
note = "tail"
`,
			want: `[profile]
developer_instructions = '''
[mcp_servers.{{ID}}]
command = "fake"
'''
note = "tail"`,
		},
		{
			name: "real block after a basic multiline string with a fake header",
			input: `developer_instructions = """
[mcp_servers.{{ID}}]
"""

[mcp_servers.{{ID}}]
command = "old"
description = """
[features]
"""
stale = true

[features]
multi_agent = true
`,
			want: `developer_instructions = """
[mcp_servers.{{ID}}]
"""

[features]
multi_agent = true`,
		},
		{
			name: "triple quotes in an ordinary string or comment do not hide a real block",
			input: `note = "contains ''' and \"\"\" inside" # a """ comment
# """ opens nothing
[mcp_servers.{{ID}}]
command = "old"

[features]
multi_agent = true
`,
			want: `note = "contains ''' and \"\"\" inside" # a """ comment
# """ opens nothing
[features]
multi_agent = true`,
		},
	}

	for _, upserter := range codexBlockUpserters {
		for _, tt := range tests {
			t.Run(upserter.name+"/"+tt.name, func(t *testing.T) {
				input := strings.ReplaceAll(tt.input, "{{ID}}", upserter.id)
				want := strings.ReplaceAll(tt.want, "{{ID}}", upserter.id) + "\n\n" + upserter.block + "\n"

				first := upserter.fn(input)
				if first != want {
					t.Fatalf("%s() mismatch:\nwant:\n%s\ngot:\n%s", upserter.name, want, first)
				}
				if second := upserter.fn(first); second != first {
					t.Fatalf("%s() is not idempotent:\nfirst:\n%s\nsecond:\n%s", upserter.name, first, second)
				}
			})
		}
	}
}
