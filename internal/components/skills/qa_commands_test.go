package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// allQASkillIDs mirrors the full 10-skill QA-Orchestrator v3 suite: 5
// ledger-bound stages that depend on QAStateMachine/{change}, plus 5
// standalone tools (qa-supervisor is the flow entrypoint but is not itself
// ledger-bound — it only routes via `gentle-ai qa-status`).
func allQASkillIDs() []model.SkillID {
	return []model.SkillID{
		model.SkillQAExplore,
		model.SkillQASpec,
		model.SkillQAApply,
		model.SkillQAVerify,
		model.SkillQADocs,
		model.SkillQASupervisor,
		model.SkillQALocatorHunting,
		model.SkillQADocReference,
		model.SkillQADocAccess,
		model.SkillQAEvidence,
	}
}

// TestInjectQACommandsWritesStandaloneToolCommandsForOpenCode is the RED/GREEN
// core assertion: with all 10 QA skills selected, exactly 5 OpenCode slash
// command files are written — one per standalone QA tool — with content
// matching the embedded asset bytes exactly.
func TestInjectQACommandsWritesStandaloneToolCommandsForOpenCode(t *testing.T) {
	home := t.TempDir()

	result, err := InjectQACommands(home, opencodeAdapter(), allQASkillIDs())
	if err != nil {
		t.Fatalf("InjectQACommands() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("InjectQACommands() changed = false, want true")
	}

	commandsDir := opencodeAdapter().CommandsDir(home)
	wantFiles := map[model.SkillID]string{
		model.SkillQASupervisor:     "qa-supervisor.md",
		model.SkillQALocatorHunting: "qa-locator-hunting.md",
		model.SkillQADocReference:   "qa-doc-reference.md",
		model.SkillQADocAccess:      "qa-doc-access.md",
		model.SkillQAEvidence:       "qa-evidence.md",
	}

	if len(result.Files) != len(wantFiles) {
		t.Fatalf("InjectQACommands() files = %v, want exactly %d entries", result.Files, len(wantFiles))
	}

	for id, fileName := range wantFiles {
		t.Run(string(id), func(t *testing.T) {
			path := filepath.Join(commandsDir, fileName)
			if !containsFile(result.Files, path) {
				t.Fatalf("InjectQACommands() files = %v, missing %q", result.Files, path)
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("ReadFile(%q) error = %v", path, readErr)
			}
			want, assetErr := assets.Read(qaCommandAssetDir + "/" + fileName)
			if assetErr != nil {
				t.Fatalf("assets.Read(%q) error = %v", qaCommandAssetDir+"/"+fileName, assetErr)
			}
			if string(got) != want {
				t.Fatalf("%s content does not match embedded asset bytes exactly", path)
			}
		})
	}
}

// TestStageSkillsNeverGetStandaloneCommands is the whole point of this
// feature: the 5 ledger-bound QA stages (qa-explore, qa-spec, qa-apply,
// qa-verify, qa-docs) depend on QAStateMachine/{change} orchestration, so a
// same-named /qa-explore (etc.) command must never exist — it would let a
// user skip qa-supervisor's routing through `gentle-ai qa-status` entirely.
func TestStageSkillsNeverGetStandaloneCommands(t *testing.T) {
	home := t.TempDir()

	result, err := InjectQACommands(home, opencodeAdapter(), allQASkillIDs())
	if err != nil {
		t.Fatalf("InjectQACommands() error = %v", err)
	}

	commandsDir := opencodeAdapter().CommandsDir(home)
	stageFiles := []string{
		"qa-explore.md",
		"qa-spec.md",
		"qa-apply.md",
		"qa-verify.md",
		"qa-docs.md",
	}
	for _, fileName := range stageFiles {
		t.Run(fileName, func(t *testing.T) {
			path := filepath.Join(commandsDir, fileName)
			if containsFile(result.Files, path) {
				t.Fatalf("InjectQACommands() must never write a stage command file, wrote %q", path)
			}
			if _, statErr := os.Stat(path); statErr == nil {
				t.Fatalf("stage command file %q was written to disk — stages depend on QAStateMachine/{change}, never a standalone command", path)
			}
		})
	}
}

// TestInjectQACommandsSkipsClaudeCode confirms Claude Code gets zero QA
// command files regardless of selection: Claude Code already resolves a
// same-named skill directory as /<skill-name> natively, so writing a command
// file here would just be a stale duplicate of that native resolution (this
// is exactly why SDD prefixes its own commands gentle-sdd-* — to dodge that
// same native resolution — but QA tools want the native resolution, not a
// workaround for it).
func TestInjectQACommandsSkipsClaudeCode(t *testing.T) {
	home := t.TempDir()

	result, err := InjectQACommands(home, claudeAdapter(), allQASkillIDs())
	if err != nil {
		t.Fatalf("InjectQACommands() error = %v", err)
	}
	if result.Changed {
		t.Fatalf("InjectQACommands(claude) changed = true, want false")
	}
	if len(result.Files) != 0 {
		t.Fatalf("InjectQACommands(claude) files = %v, want none", result.Files)
	}

	commandsDir := claudeAdapter().CommandsDir(home)
	for _, fileName := range []string{"qa-supervisor.md", "qa-locator-hunting.md", "qa-doc-reference.md", "qa-doc-access.md", "qa-evidence.md"} {
		path := filepath.Join(commandsDir, fileName)
		if _, statErr := os.Stat(path); statErr == nil {
			t.Fatalf("Claude Code must never receive a QA command file, found %q", path)
		}
	}
}

// TestInjectQACommandsSelectionGating mirrors skills.Inject's "only write
// what's selected" discipline: selecting a subset of standalone QA tools
// writes only those command files, none of the others.
func TestInjectQACommandsSelectionGating(t *testing.T) {
	home := t.TempDir()

	result, err := InjectQACommands(home, opencodeAdapter(), []model.SkillID{
		model.SkillQALocatorHunting,
		model.SkillQASupervisor,
	})
	if err != nil {
		t.Fatalf("InjectQACommands() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("InjectQACommands() changed = false, want true")
	}
	if len(result.Files) != 2 {
		t.Fatalf("InjectQACommands() files = %v, want exactly 2", result.Files)
	}

	commandsDir := opencodeAdapter().CommandsDir(home)
	for _, fileName := range []string{"qa-locator-hunting.md", "qa-supervisor.md"} {
		path := filepath.Join(commandsDir, fileName)
		if !containsFile(result.Files, path) {
			t.Fatalf("InjectQACommands() files = %v, missing selected %q", result.Files, path)
		}
	}
	for _, fileName := range []string{"qa-doc-reference.md", "qa-doc-access.md", "qa-evidence.md"} {
		path := filepath.Join(commandsDir, fileName)
		if _, statErr := os.Stat(path); statErr == nil {
			t.Fatalf("unselected QA tool command %q was written", path)
		}
	}
}

// TestQACommandPathsMatchesInjectQACommands ensures the path-listing helper
// used by install/uninstall backup and rollback tracking agrees exactly with
// what InjectQACommands actually writes, so a selective install never leaves
// an orphaned command file untracked (and never over-tracks an unselected one).
func TestQACommandPathsMatchesInjectQACommands(t *testing.T) {
	home := t.TempDir()
	selected := []model.SkillID{model.SkillQADocAccess, model.SkillQAEvidence, model.SkillQAExplore}

	result, err := InjectQACommands(home, opencodeAdapter(), selected)
	if err != nil {
		t.Fatalf("InjectQACommands() error = %v", err)
	}

	got := QACommandPaths(opencodeAdapter(), home, selected)
	if len(got) != len(result.Files) {
		t.Fatalf("QACommandPaths() = %v, want to match InjectQACommands() files %v", got, result.Files)
	}
	for _, path := range result.Files {
		if !containsFile(got, path) {
			t.Fatalf("QACommandPaths() = %v, missing written path %q", got, path)
		}
	}

	if paths := QACommandPaths(claudeAdapter(), home, allQASkillIDs()); len(paths) != 0 {
		t.Fatalf("QACommandPaths(claude) = %v, want none", paths)
	}
}
