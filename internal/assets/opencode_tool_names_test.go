package assets

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// OpenCode agent prompts are shared by both runtime majors, so a prompt may
// name only a tool both provide, plus the MCP tools the assets already route
// to. Sources: 1.x https://opencode.ai/docs/tools/ and the 1.18.18 binary
// (`task` is the subagent tool keyed by the task permission); 2.x the
// @opencode/core 2.0.23 tool plugins, which rename bash to `shell` and task to
// `subagent` and drop todowrite, lsp, and apply_patch.
var (
	openCodeV1Tools = []string{
		"bash", "edit", "write", "read", "grep", "glob", "lsp", "apply_patch",
		"skill", "todowrite", "webfetch", "websearch", "question", "task",
	}
	openCodeV2Tools = []string{
		"read", "write", "edit", "patch", "glob", "grep", "shell", "subagent",
		"skill", "question", "webfetch", "websearch",
	}
	openCodeMCPTools = []string{"codegraph_explore", "mem_save", "mem_search", "mem_get_observation"}
	// openCodeForeignTools are tool names other harnesses or retired Gentle
	// AI plugins provide; OpenCode agents never receive them.
	openCodeForeignTools = []string{"find", "ls", "list", "delegate"}
)

// openCodeToolSets returns the names valid on every runtime major and the
// names that exist on only one major or nowhere in OpenCode.
func openCodeToolSets() (shared, unshared map[string]bool) {
	shared, unshared = map[string]bool{}, map[string]bool{}
	inV2 := map[string]bool{}
	for _, name := range openCodeV2Tools {
		inV2[name] = true
	}
	for _, name := range openCodeV1Tools {
		if inV2[name] {
			shared[name] = true
		} else {
			unshared[name] = true
		}
	}
	for _, name := range openCodeV2Tools {
		if !shared[name] {
			unshared[name] = true
		}
	}
	for _, name := range openCodeMCPTools {
		shared[name] = true
	}
	for _, name := range openCodeForeignTools {
		unshared[name] = true
	}
	return shared, unshared
}

// openCodeStatusValues are return-contract values that appear in "Use `x`"
// sentences without naming a tool.
var openCodeStatusValues = map[string]bool{
	"blocked": true, "partial": true, "completed": true, "interaction_required": true,
}

var (
	// "`x` tool" and "`x` MCP tool".
	openCodeToolSuffix = regexp.MustCompile("`([A-Za-z][A-Za-z0-9_]*)` (?:MCP )?tools?\\b")
	// "use `x`", "invoke the `x`", "call `x`, `y`, and `z`".
	openCodeToolVerb = regexp.MustCompile("(?i)\\b(?:use|invoke|call)\\s+(?:the\\s+)?((?:`[A-Za-z][A-Za-z0-9_]*`(?:,\\s*(?:and\\s+|or\\s+)?|\\s+(?:and|or)\\s+)?)+)")
	openCodeBacktick = regexp.MustCompile("`([A-Za-z][A-Za-z0-9_]*)`")
)

// openCodeToolReferences returns the backticked tool names a Markdown body
// tells the agent to use.
func openCodeToolReferences(body string) []string {
	seen := map[string]bool{}
	for _, match := range openCodeToolSuffix.FindAllStringSubmatch(body, -1) {
		seen[match[1]] = true
	}
	for _, match := range openCodeToolVerb.FindAllStringSubmatch(body, -1) {
		for _, name := range openCodeBacktick.FindAllStringSubmatch(match[1], -1) {
			if !openCodeStatusValues[name[1]] {
				seen[name[1]] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestOpenCodeToolReferencesDetectForeignToolNames(t *testing.T) {
	t.Parallel()

	for body, want := range map[string]string{
		"use `read`, `grep`, and `find` as the fallback":             "find,grep,read",
		"Use `find` for discovery. Do not assume a `glob` tool.":     "find,glob",
		"prefer the `codegraph_explore` MCP tool":                    "codegraph_explore",
		"Never delegate or invoke `task`. Use `blocked` only for X.": "task",
		"Load it with the native `Skill` tool.":                      "Skill",
		"Report `summary`, `findings`, and `S#` fields.":             "",
	} {
		if got := strings.Join(openCodeToolReferences(body), ","); got != want {
			t.Errorf("openCodeToolReferences(%q) = %q, want %q", body, got, want)
		}
	}
}

// TestOpenCodeAssetsNameOnlyOpenCodeTools guards every shipped OpenCode prompt
// on both runtime majors: naming a tool OpenCode does not provide (for example
// `find`, or telling the agent `glob` is unsupported) leaves a delegated agent
// without a usable search tool, and a 1.x-only name such as `bash` or `task`
// does not exist on 2.x.
func TestOpenCodeAssetsNameOnlyOpenCodeTools(t *testing.T) {
	t.Parallel()

	shared, unshared := openCodeToolSets()
	scanned := 0
	err := fs.WalkDir(FS, "opencode", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return walkErr
		}
		body, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for _, name := range openCodeToolReferences(string(body)) {
			if !shared[name] {
				t.Errorf("%s tells the agent to use `%s`, which is not an OpenCode tool on every runtime major", path, name)
			}
		}
		for _, match := range openCodeBacktick.FindAllStringSubmatch(string(body), -1) {
			if unshared[match[1]] {
				t.Errorf("%s names `%s`, which is not an OpenCode tool on every runtime major", path, match[1])
			}
		}
		if strings.Contains(string(body), "unsupported `glob`") {
			t.Errorf("%s calls the native `glob` tool unsupported", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("no OpenCode Markdown assets scanned")
	}
}
