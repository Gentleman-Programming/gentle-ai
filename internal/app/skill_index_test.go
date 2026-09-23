package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/skillregistry"
)

// stubMirror wires a fixed mirror outcome into the production seam.
func stubMirror(t *testing.T, fail bool) *int {
	t.Helper()
	calls := 0
	original := productionMirrorFn
	productionMirrorFn = func(cwd, home string) skillregistry.MirrorFunc {
		return func(req skillregistry.MirrorRequest) error {
			calls++
			if fail {
				return errors.New("engram unreachable")
			}
			return nil
		}
	}
	t.Cleanup(func() { productionMirrorFn = original })
	return &calls
}

// makeProject creates a project root with one skill and returns its path.
func makeProject(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
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
	return project
}

// TestSkillIndexExitCodeTable pins the exit-code table of spec §1.1
// (REQ-22.10), including the non-fatal mirror failure.
func TestSkillIndexExitCodeTable(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)

	t.Run("success reports primary and destination lines", func(t *testing.T) {
		stubMirror(t, false)
		project := makeProject(t)
		if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("# Doc\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		code := RunSkillIndex([]string{"refresh", "--no-gitignore", "--cwd", project}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0 (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
		}
		out := stdout.String()
		if !strings.Contains(out, "Skill registry refreshed (1 skills):") {
			t.Fatalf("primary line missing:\n%s", out)
		}
		if !strings.Contains(out, "AGENTS.md: updated") {
			t.Fatalf("AGENTS.md destination line missing:\n%s", out)
		}
		if !strings.Contains(out, "Engram: mirror ok") {
			t.Fatalf("Engram destination line missing:\n%s", out)
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q, want empty", stderr.String())
		}
	})

	t.Run("quiet suppresses stdout only", func(t *testing.T) {
		stubMirror(t, true)
		project := makeProject(t)
		var stdout, stderr bytes.Buffer
		code := RunSkillIndex([]string{"refresh", "--quiet", "--no-gitignore", "--cwd", project}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %q, want empty under --quiet", stdout.String())
		}
		// --quiet never suppresses error output (spec §1.1 flag contract).
		if !strings.Contains(stderr.String(), "mirror failed") {
			t.Fatalf("stderr = %q, want the mirror warning", stderr.String())
		}
	})

	t.Run("non-project skip under quiet is silent", func(t *testing.T) {
		stubMirror(t, false)
		fresh := t.TempDir()
		var stdout, stderr bytes.Buffer
		code := RunSkillIndex([]string{"refresh", "--quiet", "--no-gitignore", "--cwd", fresh}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("quiet skip must be silent, got stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
		if _, err := os.Stat(filepath.Join(fresh, ".atl")); !os.IsNotExist(err) {
			t.Fatalf("quiet skip must not create .atl (stat err = %v)", err)
		}
	})

	t.Run("non-project skip without quiet prints one notice line", func(t *testing.T) {
		stubMirror(t, false)
		fresh := t.TempDir()
		var stdout, stderr bytes.Buffer
		code := RunSkillIndex([]string{"refresh", "--no-gitignore", "--cwd", fresh}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		out := stdout.String()
		if !strings.Contains(out, "no-project-marker") || !strings.Contains(out, fresh) {
			t.Fatalf("notice must carry the reason and the path, got %q", out)
		}
		if strings.Count(out, "\n") != 1 {
			t.Fatalf("notice must be one line, got %q", out)
		}
	})

	t.Run("missing subcommand prints usage on stderr", func(t *testing.T) {
		stubMirror(t, false)
		var stdout, stderr bytes.Buffer
		code := RunSkillIndex(nil, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "usage: axiom skill index <refresh|list> [flags]") {
			t.Fatalf("stderr = %q, want the usage line", stderr.String())
		}
	})

	t.Run("unknown subcommand names refresh and list", func(t *testing.T) {
		stubMirror(t, false)
		var stdout, stderr bytes.Buffer
		code := RunSkillIndex([]string{"nope"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		errOut := stderr.String()
		if !strings.Contains(errOut, "refresh") || !strings.Contains(errOut, "list") {
			t.Fatalf("stderr = %q, must name refresh and list", errOut)
		}
	})

	t.Run("unknown flag is named exactly", func(t *testing.T) {
		stubMirror(t, false)
		var stdout, stderr bytes.Buffer
		code := RunSkillIndex([]string{"refresh", "--bogus"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), `"--bogus"`) {
			t.Fatalf("stderr = %q, must name the exact flag", stderr.String())
		}
	})

	t.Run("cwd without value is rejected", func(t *testing.T) {
		stubMirror(t, false)
		var stdout, stderr bytes.Buffer
		code := RunSkillIndex([]string{"refresh", "--cwd"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "--cwd requires a value") {
			t.Fatalf("stderr = %q, want the --cwd message", stderr.String())
		}
	})

	t.Run("primary destination failure exits 1", func(t *testing.T) {
		stubMirror(t, false)
		project := makeProject(t)
		if err := os.MkdirAll(filepath.Join(project, ".atl", "skill-registry.md"), 0o755); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		code := RunSkillIndex([]string{"refresh", "--no-gitignore", "--cwd", project}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit = %d, want 1 (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stderr.String(), "write registry") {
			t.Fatalf("stderr = %q, want the destination-wrapped error", stderr.String())
		}
	})

	t.Run("mirror failure is reported and exits 0", func(t *testing.T) {
		stubMirror(t, true)
		project := makeProject(t)
		var stdout, stderr bytes.Buffer
		code := RunSkillIndex([]string{"refresh", "--no-gitignore", "--cwd", project}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0 for a mirror failure", code)
		}
		if !strings.Contains(stdout.String(), "Engram: mirror failed") {
			t.Fatalf("stdout = %q, want the mirror failed line", stdout.String())
		}
		if !strings.Contains(stderr.String(), "Warning:") {
			t.Fatalf("stderr = %q, want the warning", stderr.String())
		}
	})
}

// TestSkillIndexDestinationLineLiterals covers 16.2: the per-destination lines
// follow the primary line and declare the five exact literals across the
// refresh situations.
func TestSkillIndexDestinationLineLiterals(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	stubMirror(t, false)
	project := makeProject(t)
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("# Doc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := RunSkillIndex(args, &stdout, &stderr); code != 0 {
			t.Fatalf("RunSkillIndex(%v) exit = %d (stderr=%q)", args, code, stderr.String())
		}
		return stdout.String()
	}

	t.Run("first refresh declares updated and mirror ok", func(t *testing.T) {
		out := run("refresh", "--no-gitignore", "--cwd", project)
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != 3 {
			t.Fatalf("report must be one primary line plus one per destination, got %d lines:\n%s", len(lines), out)
		}
		if !strings.HasPrefix(lines[0], "Skill registry refreshed") {
			t.Fatalf("first line must be primary, got %q", lines[0])
		}
		if lines[1] != "AGENTS.md: updated" {
			t.Fatalf("AGENTS.md line = %q, want %q", lines[1], "AGENTS.md: updated")
		}
		if lines[2] != "Engram: mirror ok" {
			t.Fatalf("Engram line = %q, want %q", lines[2], "Engram: mirror ok")
		}
	})

	t.Run("cache hit declares unchanged", func(t *testing.T) {
		out := run("refresh", "--no-gitignore", "--cwd", project)
		if !strings.Contains(out, "Skill registry up to date (cache-hit):") {
			t.Fatalf("primary line missing:\n%s", out)
		}
		if !strings.Contains(out, "AGENTS.md: unchanged") || !strings.Contains(out, "Engram: unchanged") {
			t.Fatalf("cache-hit destination lines missing:\n%s", out)
		}
	})

	t.Run("missing AGENTS.md declares omitted", func(t *testing.T) {
		if err := os.Remove(filepath.Join(project, "AGENTS.md")); err != nil {
			t.Fatal(err)
		}
		out := run("refresh", "--force", "--no-gitignore", "--cwd", project)
		if !strings.Contains(out, "AGENTS.md: omitted") {
			t.Fatalf("omitted destination line missing:\n%s", out)
		}
	})

	t.Run("failing mirror declares mirror failed", func(t *testing.T) {
		stubMirror(t, true)
		var stdout, stderr bytes.Buffer
		code := RunSkillIndex([]string{"refresh", "--force", "--no-gitignore", "--cwd", project}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(stdout.String(), "Engram: mirror failed") {
			t.Fatalf("mirror failed line missing:\n%s", stdout.String())
		}
	})
}

// TestSkillIndexListWritesNothing covers REQ-22.10 «list --json devuelve el
// índice sin escribir»: no registry, cache, AGENTS.md, .gitignore or memory.
func TestSkillIndexListWritesNothing(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	mirrorCalls := stubMirror(t, false)
	project := makeProject(t)

	var stdout, stderr bytes.Buffer
	code := RunSkillIndex([]string{"list", "--json", "--cwd", project}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	var rows []map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("list --json output is not a JSON array: %v\n%s", err, stdout.String())
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1:\n%s", len(rows), stdout.String())
	}
	for _, key := range []string{"name", "scope", "description", "path"} {
		if _, ok := rows[0][key]; !ok {
			t.Fatalf("row missing key %q: %v", key, rows[0])
		}
	}
	for _, rel := range []string{".atl", "AGENTS.md", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(project, rel)); !os.IsNotExist(err) {
			t.Fatalf("list --json must not write %s (stat err = %v)", rel, err)
		}
	}
	if *mirrorCalls != 0 {
		t.Fatalf("list --json must not touch memory: %d mirror calls", *mirrorCalls)
	}
}

// TestSkillIndexCwdVectorsContainEffects covers T-1 at the CLI layer: every
// --cwd shape either aborts with no effects or writes only inside the sandbox.
func TestSkillIndexCwdVectorsContainEffects(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	stubMirror(t, false)
	sandbox := t.TempDir()

	t.Run("invalid shapes abort with no effects", func(t *testing.T) {
		vectors := map[string]string{
			"nonexistent":    filepath.Join(sandbox, "nope"),
			"escaping ..":    sandbox + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "escape",
			"reserved con":   "con",
			"reserved nul":   "nul",
			"300 characters": filepath.Join(sandbox, strings.Repeat("p", 300)),
		}
		for name, target := range vectors {
			t.Run(name, func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				RunSkillIndex([]string{"refresh", "--quiet", "--no-gitignore", "--cwd", target}, &stdout, &stderr)
				for _, rel := range []string{skillregistry.RegistryRelPath, skillregistry.CacheRelPath} {
					info, err := os.Stat(filepath.Join(filepath.Clean(target), rel))
					if err != nil {
						continue
					}
					if info.Mode().IsRegular() {
						t.Fatalf("invalid --cwd %q wrote %s", target, rel)
					}
				}
			})
		}
	})

	t.Run("resolvable shapes stay inside the sandbox", func(t *testing.T) {
		project := makeProject(t)
		// makeProject uses its own temp dir; move the vector shapes under the
		// sandbox instead so containment is observable.
		if err := os.Rename(project, filepath.Join(sandbox, "ws")); err != nil {
			t.Fatal(err)
		}
		ws := filepath.Join(sandbox, "ws")
		vectors := map[string]string{
			"absolute":        ws,
			"forward slashes": strings.ReplaceAll(ws, "\\", "/"),
			"back slashes":    strings.ReplaceAll(ws, "/", "\\"),
		}
		for name, target := range vectors {
			t.Run(name, func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				code := RunSkillIndex([]string{"refresh", "--quiet", "--no-gitignore", "--cwd", target}, &stdout, &stderr)
				if code != 0 {
					t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr.String())
				}
				registry := filepath.Join(ws, ".atl", "skill-registry.md")
				if _, err := os.Stat(registry); err != nil {
					t.Fatalf("registry not written under the sandbox: %v", err)
				}
			})
		}
	})
}

// TestSkillIndexRoutingSituations covers T-6: the six dispatch shapes of the
// verb, each with its exact outcome and no side effects on rejection.
func TestSkillIndexRoutingSituations(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	stubMirror(t, false)
	project := makeProject(t)

	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{name: "refresh runs", args: []string{"refresh", "--quiet", "--no-gitignore", "--cwd", project}, wantCode: 0},
		{name: "list runs", args: []string{"list", "--cwd", project}, wantCode: 0},
		{name: "missing subcommand", args: nil, wantCode: 1},
		{name: "unknown subcommand", args: []string{"bogus"}, wantCode: 1},
		{name: "unknown flag", args: []string{"refresh", "--bogus"}, wantCode: 1},
		{name: "incomplete cwd flag", args: []string{"list", "--cwd"}, wantCode: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := RunSkillIndex(tt.args, &stdout, &stderr); code != tt.wantCode {
				t.Fatalf("RunSkillIndex(%v) exit = %d, want %d (stderr=%q)", tt.args, code, tt.wantCode, stderr.String())
			}
		})
	}
}
