package legacyassets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// Only the value of an exact `model = "..."` or `model_reasoning_effort =
// "..."` line is what the profile writer substituted; everything else is
// compared byte for byte.
func TestNormalizeCodexProfileErasesOnlySubstitutedValues(t *testing.T) {
	for _, tc := range []struct{ a, b string }{
		{"model = \"gpt-6-sol\"\n\nmodel_reasoning_effort = \"medium\"\n", "model = \"custom \\\"x\\\"\"\n\nmodel_reasoning_effort = \"xhigh\"\n"},
		{"model_reasoning_effort = \"xhigh\"\n", "model_reasoning_effort = \"low\"\n"},
	} {
		if NormalizeCodexProfile(tc.a) != NormalizeCodexProfile(tc.b) {
			t.Errorf("renders %q and %q normalize differently", tc.a, tc.b)
		}
	}
	for _, edited := range []string{
		"model = \"gpt-6-sol\"\nmodel_reasoning_effort = \"medium\"\n",
		"model = \"gpt-6-sol\"\n\nmodel_reasoning_effort = \"medium\"\nservice_tier = \"fast\"\n",
		"model = \"gpt-6-sol\" # mine\n\nmodel_reasoning_effort = \"medium\"\n",
		"model = \"gpt-6-sol\"\r\n\r\nmodel_reasoning_effort = \"medium\"\r\n",
		"model = 'gpt-6-sol'\n\nmodel_reasoning_effort = \"medium\"\n",
	} {
		if NormalizeCodexProfile(edited) == NormalizeCodexProfile("model = \"gpt-6-sol\"\n\nmodel_reasoning_effort = \"medium\"\n") {
			t.Errorf("edited profile %q normalizes to a render", edited)
		}
	}
}

// The Codex profiles and the Kimi SDD module join the one retired SDD asset
// inventory, so install, sync, and every snapshot list the same paths.
func TestRetiredSDDAssetInventoryListsCodexProfilesAndKimiModule(t *testing.T) {
	dirs := SDDAssetDirs{CodexHome: "codex", KimiHome: "kimi"}
	paths := inventoryPaths(model.AgentCodex, dirs)
	for _, want := range []string{
		filepath.Join("codex", "sdd-cheap.config.toml"),
		filepath.Join("codex", "sdd-mid.config.toml"),
		filepath.Join("codex", "sdd-strong.config.toml"),
		filepath.Join("kimi", "sdd-orchestrator.md"),
	} {
		if !slices.Contains(paths, want) {
			t.Errorf("inventory omits %s: %v", want, paths)
		}
	}
	if len(paths) != 4 {
		t.Errorf("inventory = %v, want only the profiles and the module", paths)
	}
}

func TestRetireSDDAssetsRemovesOwnedCodexProfilesAndKimiModule(t *testing.T) {
	root := t.TempDir()
	dirs := SDDAssetDirs{CodexHome: filepath.Join(root, ".codex"), KimiHome: filepath.Join(root, ".kimi")}
	strong := filepath.Join(dirs.CodexHome, "sdd-strong.config.toml")
	mid := filepath.Join(dirs.CodexHome, "sdd-mid.config.toml")
	cheap := filepath.Join(dirs.CodexHome, "sdd-cheap.config.toml")
	module := filepath.Join(dirs.KimiHome, "sdd-orchestrator.md")
	writeFixture(t, strong, releasedFixture(t, "v3.7.0", "codex-sdd-strong.config.toml"))
	// A user's model choice is what the writer substituted, not authorship.
	writeFixture(t, mid, []byte("model = \"my-own-model\"\n\nmodel_reasoning_effort = \"xhigh\"\n"))
	// v1.36.0 to v1.38.0 wrote only the effort.
	writeFixture(t, cheap, releasedFixture(t, "v1.36.0", "codex-sdd-strong.config.toml"))
	writeFixture(t, module, releasedFixture(t, "v3.7.0", "kimi-sdd-orchestrator.md"))
	config := filepath.Join(dirs.CodexHome, "config.toml")
	writeFixture(t, config, []byte("model = \"gpt-6-sol\"\n"))

	res, err := RetireSDDAssets(model.AgentCodex, dirs)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{cheap, mid, strong, module}
	slices.Sort(want)
	if !slices.Equal(res.Removed, want) || len(res.Preserved) != 0 {
		t.Fatalf("result = %+v, want removed %v", res, want)
	}
	if _, err := os.Stat(config); err != nil {
		t.Errorf("Codex config.toml was touched: %v", err)
	}
	again, err := RetireSDDAssets(model.AgentCodex, dirs)
	if err != nil || len(again.Removed) != 0 || len(again.Preserved) != 0 {
		t.Fatalf("second retirement = %+v, %v", again, err)
	}
}

// Every release's Kimi module is owned: the v1.21.0 to v1.46.0 verbatim asset
// and the later renders.
func TestOwnsRetiredKimiModuleAcrossReleases(t *testing.T) {
	for _, tag := range []string{"v1.21.0", "v3.7.0"} {
		data := releasedFixture(t, tag, "kimi-sdd-orchestrator.md")
		if !OwnsRetiredSDDAsset(sddModuleKind+"sdd-orchestrator.md", data) {
			t.Errorf("%s Kimi module is not owned", tag)
		}
		edited := append(append([]byte(nil), data...), "\n## My rule\n"...)
		if OwnsRetiredSDDAsset(sddModuleKind+"sdd-orchestrator.md", edited) {
			t.Errorf("edited %s Kimi module is owned", tag)
		}
	}
}

func TestRetireSDDAssetsPreservesUserCodexProfilesAndKimiModule(t *testing.T) {
	root := t.TempDir()
	dirs := SDDAssetDirs{CodexHome: filepath.Join(root, ".codex"), KimiHome: filepath.Join(root, ".kimi")}
	profile := filepath.Join(dirs.CodexHome, "sdd-strong.config.toml")
	module := filepath.Join(dirs.KimiHome, "sdd-orchestrator.md")
	writeFixture(t, profile, []byte("user-content\n"))
	writeFixture(t, module, []byte("# my orchestrator\n"))
	linkedTarget := filepath.Join(root, "dotfiles", "sdd-mid.config.toml")
	writeFixture(t, linkedTarget, releasedFixture(t, "v3.7.0", "codex-sdd-strong.config.toml"))
	linked := filepath.Join(dirs.CodexHome, "sdd-mid.config.toml")
	if err := os.Symlink(linkedTarget, linked); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	res, err := RetireSDDAssets(model.AgentCodex, dirs)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{linked, profile, module}
	slices.Sort(want)
	if len(res.Removed) != 0 || !slices.Equal(res.Preserved, want) {
		t.Fatalf("result = %+v, want preserved %v", res, want)
	}
	for path, content := range map[string]string{profile: "user-content\n", module: "# my orchestrator\n"} {
		if data, err := os.ReadFile(path); err != nil || string(data) != content {
			t.Errorf("%s changed: %q %v", path, data, err)
		}
	}
	if _, err := os.Stat(linkedTarget); err != nil {
		t.Errorf("symlink target removed: %v", err)
	}
	if actions := strings.Join(res.ManualActions(), "\n"); !strings.Contains(actions, profile) || !strings.Contains(actions, "move or delete it") {
		t.Errorf("ManualActions = %v", res.ManualActions())
	}

	// A symlinked Codex home is the user's: never entered, reported once.
	linkedHome := filepath.Join(root, "linked-codex")
	if err := os.Symlink(filepath.Dir(linkedTarget), linkedHome); err != nil {
		t.Fatal(err)
	}
	res, err = RetireSDDAssets(model.AgentCodex, SDDAssetDirs{CodexHome: linkedHome})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 0 || !slices.Equal(res.UnsupportedDirs, []string{linkedHome}) {
		t.Fatalf("symlinked Codex home entered or unreported: %+v", res)
	}
}

func TestRetireKimiSDDIncludeRemovesOnlyTheReleasedLine(t *testing.T) {
	hub := filepath.Join(t.TempDir(), ".kimi", "KIMI.md")
	released := string(releasedFixture(t, "v3.7.0", "kimi-KIMI.md"))
	user := released + "\n## Mine\n{% include \"sdd-orchestrator.md\" %}\n"
	writeFixture(t, hub, []byte(user))

	res, err := RetireKimiSDDInclude(filepath.Dir(hub), hub)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(user, "{% include \"sdd-orchestrator.md\" ignore missing %}\n", "", 1)
	if got, _ := os.ReadFile(hub); string(got) != want || !res.Removed {
		t.Fatalf("hub = %q (removed %v), want %q", got, res.Removed, want)
	}
	if !strings.Contains(want, "{% include \"strict-tdd-mode.md\" ignore missing %}") {
		t.Fatal("fixture lost a neighbouring include")
	}
	again, err := RetireKimiSDDInclude(filepath.Dir(hub), hub)
	if err != nil || again.Removed {
		t.Fatalf("second retirement = %+v, %v", again, err)
	}
	missingRoot := t.TempDir()
	if missing, err := RetireKimiSDDInclude(missingRoot, filepath.Join(missingRoot, "KIMI.md")); err != nil || missing.Removed || len(missing.ManualActions()) != 0 {
		t.Fatalf("missing hub = %+v, %v", missing, err)
	}
}

func TestRetireSDDOrchestratorBlockRemovesOnlyTheManagedBlock(t *testing.T) {
	dir := t.TempDir()
	prompt := filepath.Join(dir, "agents.md")
	content := "# Mine\n\n<!-- gentle-ai:engram-protocol -->\nengram\n<!-- /gentle-ai:engram-protocol -->\n\n<!-- gentle-ai:sdd-orchestrator -->\n# SDD Orchestrator\n<!-- /gentle-ai:sdd-orchestrator -->\n\n## After\n"
	writeFixture(t, prompt, []byte(content))

	// The removal target must not alias the active prompt on case-insensitive
	// filesystems; active-file identity preservation is covered separately.
	active := filepath.Join(dir, "active-prompt.md")
	writeFixture(t, active, []byte("# Active\n"))
	res, err := RetireSDDOrchestratorBlock(dir, prompt, active)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Mine\n\n<!-- gentle-ai:engram-protocol -->\nengram\n<!-- /gentle-ai:engram-protocol -->\n\n## After\n"
	if got, _ := os.ReadFile(prompt); string(got) != want || !res.Removed {
		t.Fatalf("prompt = %q (removed %v), want %q", got, res.Removed, want)
	}
	if got, err := os.ReadFile(active); err != nil || string(got) != "# Active\n" {
		t.Fatalf("active prompt = %q, %v; want unchanged", got, err)
	}
	again, err := RetireSDDOrchestratorBlock(dir, prompt, "")
	if err != nil || again.Removed {
		t.Fatalf("second retirement = %+v, %v", again, err)
	}

	// An unterminated block has no ownership boundary: untouched, reported.
	open := "<!-- gentle-ai:sdd-orchestrator -->\nmine to the end\n"
	writeFixture(t, prompt, []byte(open))
	if res, err := RetireSDDOrchestratorBlock(dir, prompt, ""); err != nil || res.Removed || !strings.Contains(strings.Join(res.ManualActions(), "\n"), "move or delete it") {
		t.Fatalf("unterminated block = %+v %v, %v", res, res.ManualActions(), err)
	}
	if got, _ := os.ReadFile(prompt); string(got) != open {
		t.Fatalf("unterminated block rewritten: %q", got)
	}
}

// On a case-insensitive filesystem agents.md is the active AGENTS.md, whose
// block routing guidance converts in place; retirement leaves it alone.
func TestRetireSDDOrchestratorBlockSkipsTheActivePrompt(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "AGENTS.md")
	content := "<!-- gentle-ai:sdd-orchestrator -->\nx\n<!-- /gentle-ai:sdd-orchestrator -->\n"
	writeFixture(t, active, []byte(content))
	alias := filepath.Join(dir, "alias.md")
	if err := os.Link(active, alias); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	res, err := RetireSDDOrchestratorBlock(dir, alias, active)
	if err != nil || res.Removed {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if got, _ := os.ReadFile(active); string(got) != content {
		t.Fatalf("active prompt rewritten: %q", got)
	}
}

// Symlinked prompt files are never rewritten; the retired text is reported.
func TestRetiredSDDPromptTextInSymlinkIsReported(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "KIMI.md")
	content := "{% include \"sdd-orchestrator.md\" ignore missing %}\n"
	writeFixture(t, target, []byte(content))
	hub := filepath.Join(dir, "KIMI.md")
	if err := os.Symlink(target, hub); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	res, err := RetireKimiSDDInclude(dir, hub)
	if err != nil || res.Removed {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if got, _ := os.ReadFile(target); string(got) != content {
		t.Fatalf("symlink target rewritten: %q", got)
	}
	if actions := res.ManualActions(); len(actions) != 1 || !strings.Contains(actions[0], hub) || !strings.Contains(actions[0], "sdd-orchestrator.md") {
		t.Fatalf("ManualActions = %v", actions)
	}

	block := filepath.Join(dir, "dotfiles", "agents.md")
	writeFixture(t, block, []byte("<!-- gentle-ai:sdd-orchestrator -->\nx\n<!-- /gentle-ai:sdd-orchestrator -->\n"))
	link := filepath.Join(dir, "agents.md")
	if err := os.Symlink(block, link); err != nil {
		t.Fatal(err)
	}
	res, err = RetireSDDOrchestratorBlock(dir, link, "")
	if err != nil || res.Removed || len(res.ManualActions()) != 1 {
		t.Fatalf("symlinked prompt = %+v %v, %v", res, res.ManualActions(), err)
	}
}

// Snapshots declare only present inventory files retirement may touch.
func TestPresentRetiredSDDAssetPathsSkipsAbsentAndLinkedFiles(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, ".codex")
	profile := filepath.Join(home, "sdd-strong.config.toml")
	writeFixture(t, profile, []byte("x"))
	if got := PresentRetiredSDDAssetPaths(model.AgentCodex, SDDAssetDirs{CodexHome: home}); !slices.Equal(got, []string{profile}) {
		t.Fatalf("present = %v, want %v", got, []string{profile})
	}
	linked := filepath.Join(root, "linked")
	if err := os.Symlink(home, linked); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got := PresentRetiredSDDAssetPaths(model.AgentCodex, SDDAssetDirs{CodexHome: linked}); len(got) != 0 {
		t.Fatalf("present through a symlinked home = %v", got)
	}
}

func TestRetiredSDDRuntimeFilesLocatesLegacyCodexAndKimiFiles(t *testing.T) {
	codex := RetiredSDDRuntimeFiles(model.AgentCodex, "root")
	if codex.Dirs.CodexHome != filepath.Join("root", ".codex") || codex.Prompt != filepath.Join("root", ".codex", "agents.md") || codex.Active != filepath.Join("root", ".codex", "AGENTS.md") {
		t.Errorf("Codex = %+v", codex)
	}
	kimi := RetiredSDDRuntimeFiles(model.AgentKimi, "root")
	if kimi.Dirs.KimiHome != filepath.Join("root", ".kimi") || kimi.Hub != filepath.Join("root", ".kimi", "KIMI.md") {
		t.Errorf("Kimi = %+v", kimi)
	}
	if other := RetiredSDDRuntimeFiles(model.AgentClaudeCode, "root"); other != (SDDRuntimeFiles{}) {
		t.Errorf("Claude Code = %+v", other)
	}
}

// Retirement removes one exactly delimited block, its line ending, and one
// blank line before it; every other byte stays, including extra blank lines.
func TestRetireSDDOrchestratorBlockKeepsSurroundingBytesExact(t *testing.T) {
	dir := t.TempDir()
	prompt := filepath.Join(dir, "agents.md")
	block := "<!-- gentle-ai:sdd-orchestrator -->\nsdd\n<!-- /gentle-ai:sdd-orchestrator -->"
	for _, tc := range []struct{ before, after, want string }{
		{"a\n\n\n\n", "\n", "a\n\n\n"},
		{"a\n", "\nb\n", "a\nb\n"},
		{"", "\n\nb\n", "\nb\n"},
		{"a\r\n\r\n", "\r\nb  \r\n", "a\r\nb  \r\n"},
		{"a\n\n", "", "a\n"},
	} {
		writeFixture(t, prompt, []byte(tc.before+block+tc.after))
		res, err := RetireSDDOrchestratorBlock(dir, prompt, "")
		if err != nil || !res.Removed {
			t.Fatalf("%q: %+v, %v", tc.before+block+tc.after, res, err)
		}
		if got, _ := os.ReadFile(prompt); string(got) != tc.want {
			t.Errorf("%q -> %q, want %q", tc.before+block+tc.after, got, tc.want)
		}
	}
}

// Ambiguous markers have no trustworthy ownership boundary: the file is kept
// byte for byte and reported.
func TestRetireSDDOrchestratorBlockPreservesAmbiguousMarkers(t *testing.T) {
	dir := t.TempDir()
	prompt := filepath.Join(dir, "agents.md")
	open, end := "<!-- gentle-ai:sdd-orchestrator -->", "<!-- /gentle-ai:sdd-orchestrator -->"
	for name, content := range map[string]string{
		"orphan open before a block": open + "\nmy notes\n\n" + open + "\nsdd\n" + end + "\n",
		"orphan close":               "my notes\n" + end + "\n",
		"close before open":          end + "\nmine\n" + open + "\n",
		"two blocks":                 open + "\na\n" + end + "\nmine\n" + open + "\nb\n" + end + "\n",
		"nested block":               open + "\n" + open + "\nx\n" + end + "\n" + end + "\n",
		"inline marker":              "see " + open + "\nx\n" + end + "\n",
		"marker not on its own line": open + " mine\nx\n" + end + "\n",
	} {
		writeFixture(t, prompt, []byte(content))
		res, err := RetireSDDOrchestratorBlock(dir, prompt, "")
		if err != nil || res.Removed {
			t.Fatalf("%s: %+v, %v", name, res, err)
		}
		if got, _ := os.ReadFile(prompt); string(got) != content {
			t.Errorf("%s: rewritten to %q", name, got)
		}
		if actions := res.ManualActions(); len(actions) != 1 || !strings.Contains(actions[0], prompt) || !strings.Contains(actions[0], "move or delete it") {
			t.Errorf("%s: ManualActions = %v", name, actions)
		}
	}
}

// A symlinked runtime directory (dotfiles) is never entered: the prompt
// behind it is not edited or snapshotted, and the directory is reported.
func TestRetiredSDDPromptTextBehindSymlinkedDirectoryIsReported(t *testing.T) {
	base := t.TempDir()
	dotfiles := filepath.Join(base, "dotfiles")
	hubContent := "{% include \"sdd-orchestrator.md\" ignore missing %}\n"
	blockContent := "mine\n\n<!-- gentle-ai:sdd-orchestrator -->\nx\n<!-- /gentle-ai:sdd-orchestrator -->\n"
	writeFixture(t, filepath.Join(dotfiles, "KIMI.md"), []byte(hubContent))
	writeFixture(t, filepath.Join(dotfiles, "agents.md"), []byte(blockContent))
	root := filepath.Join(base, ".kimi")
	if err := os.Symlink(dotfiles, root); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	hub := filepath.Join(root, "KIMI.md")
	prompt := filepath.Join(root, "agents.md")
	if RetirablePromptFile(root, hub) || RetirablePromptFile(root, prompt) {
		t.Error("a prompt behind a symlinked directory is retirable")
	}
	res, err := RetireKimiSDDInclude(root, hub)
	if err != nil || res.Removed {
		t.Fatalf("hub = %+v, %v", res, err)
	}
	if actions := strings.Join(res.ManualActions(), "\n"); !strings.Contains(actions, root) || !strings.Contains(actions, "sdd-orchestrator.md") {
		t.Errorf("hub ManualActions = %v", res.ManualActions())
	}
	block, err := RetireSDDOrchestratorBlock(root, prompt, "")
	if err != nil || block.Removed || !strings.Contains(strings.Join(block.ManualActions(), "\n"), root) {
		t.Fatalf("prompt = %+v %v, %v", block, block.ManualActions(), err)
	}
	for name, content := range map[string]string{"KIMI.md": hubContent, "agents.md": blockContent} {
		if got, _ := os.ReadFile(filepath.Join(dotfiles, name)); string(got) != content {
			t.Errorf("%s behind the symlinked directory rewritten: %q", name, got)
		}
	}

	// Nothing to retire behind the link: nothing to report.
	writeFixture(t, filepath.Join(dotfiles, "KIMI.md"), []byte("mine\n"))
	if res, err := RetireKimiSDDInclude(root, hub); err != nil || len(res.ManualActions()) != 0 {
		t.Fatalf("clean hub behind a link = %+v %v, %v", res, res.ManualActions(), err)
	}
	real := filepath.Join(base, "real")
	writeFixture(t, filepath.Join(real, "KIMI.md"), []byte(hubContent))
	if !RetirablePromptFile(real, filepath.Join(real, "KIMI.md")) {
		t.Error("a regular prompt in a real directory is not retirable")
	}
}

// A runtime home that is a regular file holds no prompt to retire; sync must
// not abort on the ENOTDIR its children report.
func TestRetiredSDDPromptTextUnderAFileRuntimeHomeIsAbsent(t *testing.T) {
	home := t.TempDir()
	for _, name := range []string{".codex", ".kimi"} {
		dir := filepath.Join(home, name)
		if err := os.WriteFile(dir, []byte("not a directory\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if res, err := RetireSDDOrchestratorBlock(dir, filepath.Join(dir, "agents.md"), ""); err != nil || res.Removed {
			t.Errorf("%s/agents.md: result %+v, error %v", name, res, err)
		}
		if res, err := RetireKimiSDDInclude(dir, filepath.Join(dir, "KIMI.md")); err != nil || res.Removed {
			t.Errorf("%s/KIMI.md: result %+v, error %v", name, res, err)
		}
		if got, err := os.ReadFile(dir); err != nil || string(got) != "not a directory\n" {
			t.Errorf("%s changed: %q, %v", name, got, err)
		}
	}
}
