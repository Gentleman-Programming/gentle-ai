package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// setFakeHome points os.UserHomeDir at dir for the duration of the test.
func setFakeHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// filesystemRoot returns the platform filesystem root ("/" on POSIX, the
// volume root on Windows).
func filesystemRoot() string {
	if runtime.GOOS == "windows" {
		return filepath.VolumeName(os.Getenv("SystemDrive")) + `\`
	}
	return "/"
}

func TestSkillRegistryRefreshSkipsFilesystemRootQuietly(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)

	var buf bytes.Buffer
	err := runSkillRegistryRefresh([]string{"--quiet", "--no-gitignore", "--cwd", filesystemRoot()}, &buf)
	if err != nil {
		t.Fatalf("refresh --cwd %s must skip quietly, got error: %v", filesystemRoot(), err)
	}
	if buf.Len() != 0 {
		t.Fatalf("refresh --quiet at filesystem root must print nothing, got %q", buf.String())
	}
}

func TestSkillRegistryRefreshSkipsFilesystemRootWithNotice(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)

	var buf bytes.Buffer
	err := runSkillRegistryRefresh([]string{"--no-gitignore", "--cwd", filesystemRoot()}, &buf)
	if err != nil {
		t.Fatalf("refresh --cwd %s must skip with a notice, got error: %v", filesystemRoot(), err)
	}
	out := buf.String()
	if !strings.Contains(out, "skipped") {
		t.Fatalf("non-quiet skip must name the skip, got %q", out)
	}
	if !strings.Contains(out, "project") {
		t.Fatalf("non-quiet skip must name the runnable continuation (run from a project root), got %q", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("non-quiet skip must be a single line, got %q", out)
	}
}

func TestSkillRegistryRefreshSkipsHomeDirectoryWithoutRegistry(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)

	var buf bytes.Buffer
	err := runSkillRegistryRefresh([]string{"--quiet", "--no-gitignore", "--cwd", home}, &buf)
	if err != nil {
		t.Fatalf("refresh --cwd $HOME must skip, got error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("refresh --quiet at home must print nothing, got %q", buf.String())
	}
	if _, statErr := os.Stat(filepath.Join(home, ".atl")); !os.IsNotExist(statErr) {
		t.Fatalf("refresh at home must not create %s: stat err = %v", filepath.Join(home, ".atl"), statErr)
	}
}

func TestSkillRegistryRefreshSkipsHomeDirectoryEvenWithExistingRegistry(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	if err := os.MkdirAll(filepath.Join(home, ".atl"), 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := runSkillRegistryRefresh([]string{"--quiet", "--no-gitignore", "--cwd", home}, &buf)
	if err != nil {
		t.Fatalf("refresh --cwd $HOME must always skip, got error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".atl", "skill-registry.md")); !os.IsNotExist(statErr) {
		t.Fatalf("refresh at home must not write a registry: stat err = %v", statErr)
	}
}

func TestSkillRegistryRefreshSkipsFreshNonProjectDirectory(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	fresh := t.TempDir()

	var buf bytes.Buffer
	err := runSkillRegistryRefresh([]string{"--quiet", "--no-gitignore", "--cwd", fresh}, &buf)
	if err != nil {
		t.Fatalf("refresh in a fresh non-project dir must skip, got error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("refresh --quiet in a fresh non-project dir must print nothing, got %q", buf.String())
	}
	if _, statErr := os.Stat(filepath.Join(fresh, ".atl")); !os.IsNotExist(statErr) {
		t.Fatalf("refresh must not create .atl in a non-project dir: stat err = %v", statErr)
	}
}

func TestSkillRegistryRefreshProceedsInGitProject(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := runSkillRegistryRefresh([]string{"--quiet", "--no-gitignore", "--cwd", project}, &buf)
	if err != nil {
		t.Fatalf("refresh in a git project must proceed, got error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".atl", "skill-registry.md")); statErr != nil {
		t.Fatalf("refresh in a git project must write the registry: %v", statErr)
	}
}

func TestSkillRegistryRefreshProceedsWithGitWorktreeFile(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	project := t.TempDir()
	// Git worktrees and submodules use a .git file, not a directory.
	if err := os.WriteFile(filepath.Join(project, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := runSkillRegistryRefresh([]string{"--quiet", "--no-gitignore", "--cwd", project}, &buf)
	if err != nil {
		t.Fatalf("refresh in a git worktree must proceed, got error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".atl", "skill-registry.md")); statErr != nil {
		t.Fatalf("refresh in a git worktree must write the registry: %v", statErr)
	}
}

func TestSkillRegistryRefreshProceedsWithExistingRegistry(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".atl"), 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := runSkillRegistryRefresh([]string{"--quiet", "--no-gitignore", "--cwd", project}, &buf)
	if err != nil {
		t.Fatalf("refresh with an existing .atl must proceed, got error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".atl", "skill-registry.md")); statErr != nil {
		t.Fatalf("refresh with an existing .atl must write the registry: %v", statErr)
	}
}

func TestSkillRegistryRefreshProceedsWithProjectSkillDir(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := runSkillRegistryRefresh([]string{"--quiet", "--no-gitignore", "--cwd", project}, &buf)
	if err != nil {
		t.Fatalf("refresh in a skills workspace must proceed, got error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".atl", "skill-registry.md")); statErr != nil {
		t.Fatalf("refresh in a skills workspace must write the registry: %v", statErr)
	}
}

// TestSkillRegistryRefreshLiteralDotCWDProceedsInGitProject proves --cwd . reaches the project refresh path.
func TestSkillRegistryRefreshLiteralDotCWDProceedsInGitProject(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, project)

	var buf bytes.Buffer
	err := runSkillRegistryRefresh([]string{"--quiet", "--no-gitignore", "--cwd", "."}, &buf)
	if err != nil {
		t.Fatalf("refresh --cwd . in a git project must proceed, got error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".atl", "skill-registry.md")); statErr != nil {
		t.Fatalf("refresh --cwd . in a git project must write the registry instead of skipping as filesystem-root: %v", statErr)
	}
}

// TestSkillRegistryListLiteralDotCWDReportsProjectSkill proves --cwd . preserves project skill identity.
func TestSkillRegistryListLiteralDotCWDReportsProjectSkill(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	project := t.TempDir()
	writeSkillFile(t, filepath.Join(project, "skills", "project-local", "SKILL.md"), `---
name: project-local
description: project skill visible from literal dot cwd
---

Use the project skill.
`)
	chdir(t, project)

	var buf bytes.Buffer
	err := runSkillRegistryList([]string{"--json", "--cwd", "."}, &buf)
	if err != nil {
		t.Fatalf("list --json --cwd . must succeed, got error: %v", err)
	}

	var rows []struct {
		Name  string `json:"name"`
		Scope string `json:"scope"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("list --json --cwd . emitted invalid JSON: %v\n%s", err, buf.String())
	}
	if len(rows) != 1 {
		t.Fatalf("list --json --cwd . rows = %#v, want exactly the project skill", rows)
	}
	if rows[0].Name != "project-local" {
		t.Fatalf("skill name = %q, want project-local", rows[0].Name)
	}
	if rows[0].Scope != "project" {
		t.Fatalf("skill scope = %q, want project for literal --cwd .", rows[0].Scope)
	}
	wantPath := filepath.Join(project, "skills", "project-local", "SKILL.md")
	if !filepath.IsAbs(rows[0].Path) || filepath.Clean(rows[0].Path) != wantPath {
		t.Fatalf("skill path = %q, want absolute project path %q", rows[0].Path, wantPath)
	}
}

// chdir moves the process working directory for app-boundary cwd tests and restores it during cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}

// writeSkillFile writes a skill file fixture, creating parent directories as needed.
func writeSkillFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
