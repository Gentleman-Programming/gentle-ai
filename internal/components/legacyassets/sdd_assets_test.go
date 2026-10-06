package legacyassets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func releasedFixture(t *testing.T, tag, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", tag, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// One inventory: the command names snapshots enumerate are exactly the
// command files the registry proves, and only Claude Code had the
// namespaced names.
func TestRetiredSDDAssetInventoryMatchesRegistry(t *testing.T) {
	var registered []string
	for key := range releasedSDDAssetDigests {
		if name, ok := strings.CutPrefix(key, sddCommandKind); ok {
			registered = append(registered, name)
		}
	}
	var enumerated []string
	for _, path := range SlashCommandPaths(model.AgentClaudeCode, "commands") {
		enumerated = append(enumerated, filepath.Base(path))
	}
	slices.Sort(registered)
	slices.Sort(enumerated)
	if !slices.Equal(registered, enumerated) {
		t.Fatalf("registry commands %v differ from enumerated %v", registered, enumerated)
	}
	paths := RetiredSDDAssetPaths(model.AgentOpenCode, SDDAssetDirs{Skills: "skills", Commands: "commands"})
	for _, path := range paths {
		if strings.HasPrefix(filepath.Base(path), "gentle-") {
			t.Errorf("OpenCode inventory lists Claude-only command %s", path)
		}
	}
	for _, want := range []string{
		filepath.Join("skills", "sdd-apply", "SKILL.md"),
		filepath.Join("skills", "_shared", "openspec-convention.md"),
		filepath.Join("commands", "sdd-apply.md"),
	} {
		if !slices.Contains(paths, want) {
			t.Errorf("inventory omits %s", want)
		}
	}
	for _, path := range paths {
		if !strings.Contains(filepath.Base(path), "sdd-") && !strings.Contains(filepath.ToSlash(path), "/sdd-") && filepath.Base(path) != "openspec-convention.md" {
			t.Errorf("inventory lists a non-SDD path %s", path)
		}
	}
}

func TestRetireSDDAssetsRemovesOnlyOwnedFiles(t *testing.T) {
	root := t.TempDir()
	dirs := SDDAssetDirs{Skills: filepath.Join(root, "skills"), Commands: filepath.Join(root, "commands")}
	skill := releasedFixture(t, "v3.7.0", "skill-sdd-init.md")
	owned := []string{
		filepath.Join(dirs.Commands, "sdd-init.md"),
		filepath.Join(dirs.Skills, "sdd-init", "SKILL.md"),
		filepath.Join(dirs.Skills, "sdd-verify", "references", "report-format.md"),
		filepath.Join(dirs.Skills, "_shared", "sdd-orchestrator-sections.md"),
	}
	writeFixture(t, owned[0], releasedFixture(t, "v3.7.0", "opencode-sdd-init.md"))
	// Line endings converted by an editor or checkout are not authorship.
	writeFixture(t, owned[1], []byte(strings.ReplaceAll(string(skill), "\n", "\r\n")))
	writeFixture(t, owned[2], releasedFixture(t, "v3.7.0", "skill-sdd-verify-report-format.md"))
	writeFixture(t, owned[3], releasedFixture(t, "v3.7.0", "skill-shared-sdd-orchestrator-sections.md"))
	edited := filepath.Join(dirs.Commands, "sdd-apply.md")
	writeFixture(t, edited, []byte("my apply\n"))
	beside := filepath.Join(dirs.Skills, "sdd-init", "notes.md")
	writeFixture(t, beside, []byte("mine\n"))
	unrelated := filepath.Join(dirs.Skills, "go-testing", "SKILL.md")
	writeFixture(t, unrelated, []byte("keep\n"))
	linked := filepath.Join(dirs.Commands, "sdd-verify.md")
	if err := os.Symlink(owned[0], linked); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	res, err := RetireSDDAssets(model.AgentOpenCode, dirs)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(owned)
	if !slices.Equal(res.Removed, owned) {
		t.Fatalf("Removed = %v, want %v", res.Removed, owned)
	}
	if want := []string{edited, linked}; !slices.Equal(res.Preserved, []string{edited, linked}) {
		t.Fatalf("Preserved = %v, want %v", res.Preserved, want)
	}
	for _, path := range []string{edited, beside, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s was touched: %v", path, err)
		}
	}
	if _, err := os.Lstat(linked); err != nil {
		t.Errorf("symlinked command was removed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dirs.Skills, "sdd-verify")); !os.IsNotExist(err) {
		t.Errorf("emptied skill directory kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dirs.Skills, "_shared")); err != nil {
		t.Errorf("shared directory removed: %v", err)
	}
	if actions := res.ManualActions(); len(actions) != 2 || !strings.Contains(actions[0], "move or delete it") {
		t.Errorf("ManualActions = %v", actions)
	}

	again, err := RetireSDDAssets(model.AgentOpenCode, dirs)
	if err != nil || len(again.Removed) != 0 || !slices.Equal(again.Preserved, res.Preserved) {
		t.Fatalf("second retirement = %+v, %v", again, err)
	}
}

// An edited skill is one unit with the released files beside it, and it keeps
// the shared SDD references it reads until it is gone.
func TestRetireSDDAssetsKeepsEditedSkillUnit(t *testing.T) {
	skills := filepath.Join(t.TempDir(), "skills")
	editedSkill := filepath.Join(skills, "sdd-verify", "SKILL.md")
	reference := filepath.Join(skills, "sdd-verify", "references", "report-format.md")
	shared := filepath.Join(skills, "_shared", "sdd-orchestrator-sections.md")
	writeFixture(t, editedSkill, []byte("---\nname: sdd-verify\n---\nmine\n"))
	writeFixture(t, reference, releasedFixture(t, "v3.7.0", "skill-sdd-verify-report-format.md"))
	writeFixture(t, shared, releasedFixture(t, "v3.7.0", "skill-shared-sdd-orchestrator-sections.md"))

	res, err := RetireSDDAssets(model.AgentClaudeCode, SDDAssetDirs{Skills: skills})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 0 || !slices.Equal(res.Preserved, []string{editedSkill}) {
		t.Fatalf("result = %+v", res)
	}
	for _, path := range []string{reference, shared} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s removed while the edited skill remains: %v", path, err)
		}
	}

	if err := os.Remove(editedSkill); err != nil {
		t.Fatal(err)
	}
	res, err = RetireSDDAssets(model.AgentClaudeCode, SDDAssetDirs{Skills: skills})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{shared, reference}; !slices.Equal(res.Removed, []string{shared, reference}) || len(res.Preserved) != 0 {
		t.Fatalf("after the user removed the skill: %+v, want removed %v", res, want)
	}
}

// Claude Code's lazy SDD workflow was rendered per installation, so its
// ownership cannot be proven: it is snapshotted, kept, and reported.
func TestRetireSDDAssetsReportsUnprovableClaudeWorkflow(t *testing.T) {
	skills := filepath.Join(t.TempDir(), "skills")
	workflow := filepath.Join(skills, "_shared", "sdd-orchestrator-workflow.md")
	writeFixture(t, workflow, []byte("## SDD Workflow (Spec-Driven Development)\n"))
	dirs := SDDAssetDirs{Skills: skills}
	if !slices.Contains(RetiredSDDAssetPaths(model.AgentClaudeCode, dirs), workflow) {
		t.Fatal("Claude Code inventory omits the lazy SDD workflow")
	}
	if slices.Contains(RetiredSDDAssetPaths(model.AgentCodex, dirs), workflow) {
		t.Fatal("only Claude Code received the lazy SDD workflow")
	}
	res, err := RetireSDDAssets(model.AgentClaudeCode, dirs)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 0 || !slices.Equal(res.Preserved, []string{workflow}) {
		t.Fatalf("result = %+v", res)
	}
	if actions := res.ManualActions(); len(actions) != 1 || !strings.Contains(actions[0], "workflow") || !strings.Contains(actions[0], "move or delete it") {
		t.Fatalf("ManualActions = %v", actions)
	}
}

// A symlinked skills root, skill directory, or _shared directory belongs to
// the user (dotfiles): retirement never enters it, so files in its target are
// never removed, and it is reported once with an action.
func TestRetireSDDAssetsDoesNotEnterSymlinkedDirectories(t *testing.T) {
	base := t.TempDir()
	skill := releasedFixture(t, "v3.7.0", "skill-sdd-init.md")
	shared := releasedFixture(t, "v3.7.0", "skill-shared-sdd-orchestrator-sections.md")
	dotfiles := filepath.Join(base, "dotfiles")
	writeFixture(t, filepath.Join(dotfiles, "sdd-init", "SKILL.md"), skill)
	writeFixture(t, filepath.Join(dotfiles, "_shared", "sdd-orchestrator-sections.md"), shared)
	writeFixture(t, filepath.Join(dotfiles, "sdd-verify-refs", "report-format.md"), releasedFixture(t, "v3.7.0", "skill-sdd-verify-report-format.md"))

	skills := filepath.Join(base, "skills")
	commands := filepath.Join(base, "commands")
	owned := filepath.Join(commands, "sdd-init.md")
	writeFixture(t, owned, releasedFixture(t, "v2.0.0", "claude-sdd-init.md"))
	if err := os.MkdirAll(filepath.Join(skills, "sdd-verify"), 0o755); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		filepath.Join(skills, "sdd-init"):                 filepath.Join(dotfiles, "sdd-init"),
		filepath.Join(skills, "_shared"):                  filepath.Join(dotfiles, "_shared"),
		filepath.Join(skills, "sdd-verify", "references"): filepath.Join(dotfiles, "sdd-verify-refs"),
	}
	for link, target := range links {
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}

	res, err := RetireSDDAssets(model.AgentClaudeCode, SDDAssetDirs{Skills: skills, Commands: commands})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(dotfiles, "sdd-init", "SKILL.md"),
		filepath.Join(dotfiles, "_shared", "sdd-orchestrator-sections.md"),
		filepath.Join(dotfiles, "sdd-verify-refs", "report-format.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("retirement removed %s through or beside a symlinked directory: %v", path, err)
		}
	}
	if _, err := os.Lstat(owned); !os.IsNotExist(err) {
		t.Errorf("owned command in a real directory survived: %v", err)
	}
	actions := strings.Join(res.ManualActions(), "\n")
	for link := range links {
		if !strings.Contains(actions, link) {
			t.Errorf("symlinked directory %s not reported: %v", link, res.ManualActions())
		}
	}
	if !strings.Contains(actions, "move or delete") {
		t.Errorf("no action for the symlinked directories: %v", res.ManualActions())
	}

	// A symlinked root is not entered either.
	root := filepath.Join(base, "linked-skills")
	if err := os.Symlink(dotfiles, root); err != nil {
		t.Fatal(err)
	}
	res, err = RetireSDDAssets(model.AgentClaudeCode, SDDAssetDirs{Skills: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 0 || !strings.Contains(strings.Join(res.ManualActions(), "\n"), root) {
		t.Fatalf("symlinked skills root entered or unreported: %+v %v", res, res.ManualActions())
	}
	if _, err := os.Stat(filepath.Join(dotfiles, "sdd-init", "SKILL.md")); err != nil {
		t.Fatalf("retirement removed a file through a symlinked root: %v", err)
	}
}
