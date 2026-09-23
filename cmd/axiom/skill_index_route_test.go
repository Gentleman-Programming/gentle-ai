package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/app"
)

// countCollisionLines counts the lines that clarify the `skill list` vs
// `skill index list` collision (REQ-22.10: exactly one per help surface).
func countCollisionLines(text string) int {
	count := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "skill list") && strings.Contains(line, "skill index list") {
			count++
		}
	}
	return count
}

// makeSkillIndexProject creates a project root with one skill and points the
// process home at a sandbox so no real user state is read.
func makeSkillIndexProject(t *testing.T) (project, home string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	project = t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(project, "skills", "go-testing")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: go-testing\ndescription: \"Trigger: Go tests.\"\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return project, home
}

// TestSkillGroupRoutesIndexToUnifiedEngine covers F17 routing: `axiom skill
// index` reaches the unified engine and reports the spec §1.1 usage line.
func TestSkillGroupRoutesIndexToUnifiedEngine(t *testing.T) {
	makeSkillIndexProject(t)

	var stdout, stderr bytes.Buffer
	code := runSkillGroup([]string{"index"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "usage: axiom skill index <refresh|list> [flags]") {
		t.Fatalf("stderr = %q, want the usage line", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runSkillGroup([]string{"index", "bogus"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "refresh") || !strings.Contains(errOut, "list") {
		t.Fatalf("stderr = %q, must name refresh and list", errOut)
	}
}

// TestSkillGroupHelpListsIndexAndCollisionNote covers REQ-22.10: the `axiom
// skill` help lists `index` alongside `scan`, `list`, `approve`, `reject` and
// clarifies the collision in exactly one line.
func TestSkillGroupHelpListsIndexAndCollisionNote(t *testing.T) {
	help := skillGroupHelp()
	if !strings.Contains(help, "Opciones: index, scan, list, approve, reject") {
		t.Fatalf("skill help must list index alongside scan, list, approve, reject:\n%s", help)
	}
	if got := countCollisionLines(help); got != 1 {
		t.Fatalf("collision clarification lines = %d, want exactly 1:\n%s", got, help)
	}
}

// TestAxiomHelpListsSkillIndexAndCollisionNote covers F17: printHelp lists
// `skill index` and the same one-line collision clarification.
func TestAxiomHelpListsSkillIndexAndCollisionNote(t *testing.T) {
	help := axiomHelpText()
	for _, want := range []string{"skill index", "skill scan", "skill list", "skill approve", "skill reject"} {
		if !strings.Contains(help, want) {
			t.Fatalf("printHelp must document %q:\n%s", want, help)
		}
	}
	if got := countCollisionLines(help); got != 1 {
		t.Fatalf("collision clarification lines = %d, want exactly 1:\n%s", got, help)
	}
}

// TestSkillRegistryCompatArgvLiteral covers gate 4 (REQ-22.14): the plugin's
// literal argv terminates 0 and writes the registry.
func TestSkillRegistryCompatArgvLiteral(t *testing.T) {
	project, _ := makeSkillIndexProject(t)

	var buf bytes.Buffer
	err := app.RunArgs([]string{"skill-registry", "refresh", "--quiet", "--no-gitignore", "--cwd", project}, &buf)
	if err != nil {
		t.Fatalf("skill-registry refresh (plugin argv) error = %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("--quiet output = %q, want empty", buf.String())
	}
	if _, statErr := os.Stat(filepath.Join(project, ".atl", "skill-registry.md")); statErr != nil {
		t.Fatalf("registry not written: %v", statErr)
	}
}

// TestSkillRegistryCompatRefreshPrimaryLineOnly covers gate 4: without
// --quiet the legacy surface prints only the primary line.
func TestSkillRegistryCompatRefreshPrimaryLineOnly(t *testing.T) {
	project, _ := makeSkillIndexProject(t)

	var buf bytes.Buffer
	if err := app.RunArgs([]string{"skill-registry", "refresh", "--no-gitignore", "--cwd", project}, &buf); err != nil {
		t.Fatalf("skill-registry refresh error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Skill registry refreshed (1 skills):") {
		t.Fatalf("primary line missing:\n%s", out)
	}
	if strings.Contains(out, "AGENTS.md:") || strings.Contains(out, "Engram:") {
		t.Fatalf("legacy surface must stay primary-line-only:\n%s", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("legacy surface must print exactly one line, got %q", out)
	}
}

// TestSkillRegistryCompatListJSONShape covers REQ-22.14 «list de
// compatibilidad conserva su forma de salida».
func TestSkillRegistryCompatListJSONShape(t *testing.T) {
	project, _ := makeSkillIndexProject(t)

	var buf bytes.Buffer
	if err := app.RunArgs([]string{"skill-registry", "list", "--json", "--cwd", project}, &buf); err != nil {
		t.Fatalf("skill-registry list --json error = %v", err)
	}
	var rows []map[string]string
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, buf.String())
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1:\n%s", len(rows), buf.String())
	}
	for _, key := range []string{"name", "scope", "description", "path"} {
		if _, ok := rows[0][key]; !ok {
			t.Fatalf("row missing key %q: %v", key, rows[0])
		}
	}
}

// TestSkillRegistryCompatErrorMessages covers REQ-22.14: the legacy error
// messages are byte-identical to the pre-INC-22 ones.
func TestSkillRegistryCompatErrorMessages(t *testing.T) {
	makeSkillIndexProject(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing subcommand",
			args: []string{"skill-registry"},
			want: "usage: gentle-ai skill-registry <refresh|list> [flags]",
		},
		{
			name: "unknown subcommand",
			args: []string{"skill-registry", "nope"},
			want: `unknown skill-registry command "nope" (want refresh or list)`,
		},
		{
			name: "unknown refresh flag",
			args: []string{"skill-registry", "refresh", "--bogus"},
			want: `unknown skill-registry refresh argument "--bogus"`,
		},
		{
			name: "unknown list flag",
			args: []string{"skill-registry", "list", "--bogus"},
			want: `unknown skill-registry list argument "--bogus"`,
		},
		{
			name: "cwd without value",
			args: []string{"skill-registry", "refresh", "--cwd"},
			want: "--cwd requires a value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := app.RunArgs(tt.args, &buf)
			if err == nil {
				t.Fatalf("RunArgs(%v) error = nil, want %q", tt.args, tt.want)
			}
			if err.Error() != tt.want {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

// TestRunSkillApproveWarnsOnIndexRegenerationFailure covers F18: the hook
// lives in Manager.Approve, the CLI reports a regeneration failure as a
// warning, and the exit code stays 0 (REQ-22.13).
func TestRunSkillApproveWarnsOnIndexRegenerationFailure(t *testing.T) {
	workspace, _ := makeSkillIndexProject(t)
	inbox := filepath.Join(workspace, ".axiom", "skills", "inbox", "branch-pr")
	if err := os.MkdirAll(inbox, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inbox, "SKILL.md"), []byte("---\nname: branch-pr\n---\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runSkillApprove([]string{"branch-pr", "--path", workspace}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "skills", "branch-pr", "SKILL.md")); statErr != nil {
		t.Fatalf("promotion did not complete: %v", statErr)
	}
	out := stdout.String()
	if !strings.Contains(out, "branch-pr") {
		t.Fatalf("success report missing:\n%s", out)
	}
	// The Engram mirror cannot succeed against a sandboxed home, so the index
	// regeneration reports a failure — as a warning, never as an error.
	if !strings.Contains(stderr.String(), "Aviso:") {
		t.Fatalf("stderr = %q, want the regeneration warning", stderr.String())
	}
}
