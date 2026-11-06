package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	piagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestBackupRootsHonorGentleAIHome(t *testing.T) {
	for _, tt := range []struct {
		name     string
		override bool
	}{
		{name: "default stays under ~/.gentle-ai"},
		{name: "GENTLE_AI_HOME moves backups", override: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			want := filepath.Join(home, ".gentle-ai", "backups")
			t.Setenv(state.HomeEnv, "")
			if tt.override {
				override := filepath.Join(t.TempDir(), "gentle-ai")
				t.Setenv(state.HomeEnv, override)
				want = filepath.Join(override, "backups")
			}

			rt, err := newInstallRuntime(home, ScopeGlobal, ChannelStable, model.Selection{}, planner.ResolvedPlan{}, system.PlatformProfile{})
			if err != nil {
				t.Fatalf("newInstallRuntime() error = %v", err)
			}
			if rt.backupRoot != want {
				t.Fatalf("install backupRoot = %q, want %q", rt.backupRoot, want)
			}
			if info, err := os.Stat(want); err != nil || !info.IsDir() {
				t.Fatalf("install backup root %q not created: %v", want, err)
			}

			syncRT, err := newSyncRuntimeWithScope(home, model.Selection{}, ScopeGlobal)
			if err != nil {
				t.Fatalf("newSyncRuntimeWithScope() error = %v", err)
			}
			if syncRT.backupRoot != want {
				t.Fatalf("sync backupRoot = %q, want %q", syncRT.backupRoot, want)
			}
			if got := filepath.Clean(backupRootDir(home)); got != want {
				t.Fatalf("restore backupRootDir() = %q, want %q", got, want)
			}
			if tt.override {
				if _, err := os.Stat(filepath.Join(home, ".gentle-ai")); !os.IsNotExist(err) {
					t.Fatalf("~/.gentle-ai created despite %s (stat err = %v)", state.HomeEnv, err)
				}
			}
		})
	}
}

func TestPiBackgroundPolicyPathRules(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(t.TempDir(), "pi-base")
	piConfigDir := filepath.Join(t.TempDir(), "config")

	for _, tt := range []struct {
		name        string
		configHome  string
		piConfigDir string
		want        string
	}{
		{name: "default", want: filepath.Join(home, ".pi", "gentle-ai", "background-subagents.json")},
		{name: "GENTLE_PI_CONFIG_HOME keeps its base meaning", configHome: configHome, want: filepath.Join(configHome, "gentle-ai", "background-subagents.json")},
		{name: "GENTLE_AI_PI_CONFIG_DIR is the gentle-pi config folder itself", piConfigDir: piConfigDir, want: filepath.Join(piConfigDir, "background-subagents.json")},
		{name: "GENTLE_AI_PI_CONFIG_DIR wins over GENTLE_PI_CONFIG_HOME", configHome: configHome, piConfigDir: piConfigDir, want: filepath.Join(piConfigDir, "background-subagents.json")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(PiConfigHomeEnv, tt.configHome)
			t.Setenv(piagent.GentlePiConfigDirEnv, tt.piConfigDir)
			if got := piBackgroundPolicyPath(home); got != tt.want {
				t.Fatalf("piBackgroundPolicyPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPiPersonaHonorsGentlePiConfigDir(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	piConfigDir := filepath.Join(t.TempDir(), "config")
	t.Setenv(piagent.GentlePiConfigDirEnv, piConfigDir)
	want := filepath.Join(piConfigDir, "persona.json")
	selection := model.Selection{Persona: model.PersonaNeutral}
	adapters := resolveAdapters([]model.AgentID{model.AgentPi})

	global := componentPathsWithWorkspaceScoped(home, workspace, ScopeGlobal, selection, adapters, model.ComponentPersona)
	if !containsPath(global, want) {
		t.Fatalf("global Pi persona paths = %v, missing %q", global, want)
	}
	workspacePaths := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, selection, adapters, model.ComponentPersona)
	if !containsPath(workspacePaths, filepath.Join(workspace, ".pi", "gentle-ai", "persona.json")) {
		t.Fatalf("workspace Pi persona paths = %v, want the workspace file unchanged", workspacePaths)
	}

	install := componentApplyStep{
		component: model.ComponentPersona, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal,
		agents: []model.AgentID{model.AgentPi}, selection: selection,
	}
	if err := install.Run(); err != nil {
		t.Fatalf("install persona error = %v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("install did not write %q: %v", want, err)
	}
	if err := os.Remove(want); err != nil {
		t.Fatal(err)
	}

	var changed []string
	syncStep := componentSyncStep{
		id: "sync:persona", component: model.ComponentPersona, homeDir: home, workspaceDir: workspace,
		agents: []model.AgentID{model.AgentPi}, selection: selection, changedFiles: &changed,
	}
	if err := syncStep.Run(); err != nil {
		t.Fatalf("sync persona error = %v", err)
	}
	if !containsPath(changed, want) {
		t.Fatalf("sync changed files = %v, missing %q", changed, want)
	}
	if _, err := os.Stat(filepath.Join(home, ".pi", "gentle-ai", "persona.json")); !os.IsNotExist(err) {
		t.Fatalf("~/.pi/gentle-ai/persona.json written despite %s (stat err = %v)", piagent.GentlePiConfigDirEnv, err)
	}
}

// writeFakeEngram writes a POSIX script that records its arguments and
// answers `version` like a real Engram binary.
func writeFakeEngram(t *testing.T, dir string) (bin, log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX script stands in for the engram binary")
	}
	bin = filepath.Join(dir, "engram")
	log = filepath.Join(dir, "engram.log")
	script := "#!/bin/sh\necho \"$*\" >> '" + log + "'\nif [ \"$1\" = version ]; then echo 'engram 1.15.3'; fi\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

func TestEngramStepUsesCallerSuppliedBinary(t *testing.T) {
	home := t.TempDir()
	bin, log := writeFakeEngram(t, t.TempDir())
	t.Setenv(system.EngramBinEnv, bin)

	restoreHome, restoreCommand, restoreLookPath, restoreDownload, restorePath := osUserHomeDir, runCommand, cmdLookPath, engramDownloadFn, addUserPath
	t.Cleanup(func() {
		osUserHomeDir, runCommand, cmdLookPath, engramDownloadFn, addUserPath = restoreHome, restoreCommand, restoreLookPath, restoreDownload, restorePath
	})
	osUserHomeDir = func() (string, error) { return home, nil }
	lookedUp := []string{}
	cmdLookPath = func(name string) (string, error) {
		lookedUp = append(lookedUp, name)
		return missingBinaryLookPath(name)
	}
	recorder := &commandRecorder{}
	runCommand = recorder.record
	engramDownloadFn = func(system.PlatformProfile) (string, error) {
		t.Fatal("engram download must not run with a caller-supplied binary")
		return "", nil
	}
	addUserPath = func(string) error {
		t.Fatal("PATH must not change with a caller-supplied binary")
		return nil
	}

	state := &runtimeState{}
	step := componentApplyStep{
		component: model.ComponentEngram, homeDir: home, workspaceDir: t.TempDir(), scope: ScopeGlobal,
		agents: []model.AgentID{model.AgentPi}, selection: model.Selection{Components: []model.ComponentID{model.ComponentEngram}},
		profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}, channel: ChannelStable, state: state,
	}
	if err := step.Run(); err != nil {
		t.Fatalf("engram step error = %v", err)
	}

	for _, command := range recorder.get() {
		if !strings.HasPrefix(command, bin+" ") {
			t.Fatalf("engram step ran %q; only the caller-supplied binary may run", command)
		}
	}
	for _, name := range lookedUp {
		if name == "engram" || name == "brew" || name == "go" {
			t.Fatalf("engram step looked up %q on PATH despite %s", name, system.EngramBinEnv)
		}
	}
	if state.engramVersion != "engram 1.15.3" || state.engramVersionErr != nil {
		t.Fatalf("engram version = %q, %v; want the caller binary's version", state.engramVersion, state.engramVersionErr)
	}
	if raw, _ := os.ReadFile(log); !strings.Contains(string(raw), "version") {
		t.Fatalf("caller binary was not validated with `version`; log = %q", raw)
	}
}

func TestEngramHealthChecksUseCallerSuppliedBinary(t *testing.T) {
	bin, _ := writeFakeEngram(t, t.TempDir())
	t.Setenv(system.EngramBinEnv, bin)
	t.Setenv("PATH", t.TempDir())

	state := &runtimeState{engramVersionResolved: true, engramVersion: "engram 1.15.3"}
	for _, check := range engramHealthChecks(state, []model.AgentID{model.AgentPi}) {
		if err := check.Run(t.Context()); err != nil {
			t.Fatalf("%s error = %v", check.ID, err)
		}
	}
}

func TestPreparedInstallRejectsEngramBinaryThatDoesNotRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX script stands in for the engram binary")
	}
	home := t.TempDir()
	bin := filepath.Join(t.TempDir(), "engram")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(system.EngramBinEnv, bin)
	restoreHome, restoreCommand, restoreLookPath := osUserHomeDir, runCommand, cmdLookPath
	t.Cleanup(func() { osUserHomeDir, runCommand, cmdLookPath = restoreHome, restoreCommand, restoreLookPath })
	osUserHomeDir = func() (string, error) { return home, nil }
	// Nothing real may run if the preflight is missing: record commands and
	// report every tool as absent.
	recorder := &commandRecorder{}
	runCommand = recorder.record
	cmdLookPath = missingBinaryLookPath

	prepared, err := PrepareInstall([]string{"--agent", "pi", "--component", "engram"}, macOSDetectionResult())
	if err != nil {
		t.Fatalf("PrepareInstall() error = %v", err)
	}
	_, err = RunPreparedInstall(prepared, macOSDetectionResult())
	if err == nil || !strings.Contains(err.Error(), system.EngramBinEnv) {
		t.Fatalf("RunPreparedInstall() error = %v, want an error naming %s", err, system.EngramBinEnv)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 || len(recorder.get()) != 0 {
		t.Fatalf("nothing may be installed for an Engram binary that does not run; home has %v, commands %v", entries, recorder.get())
	}
}

func TestInstallHelpListsIsolationVariables(t *testing.T) {
	var help strings.Builder
	PrintInstallHelp(&help)
	for _, name := range []string{state.HomeEnv, piagent.GentlePiConfigDirEnv, system.EngramBinEnv} {
		if !strings.Contains(help.String(), name) {
			t.Fatalf("install help does not list %s:\n%s", name, help.String())
		}
	}
}

// A workspace install whose workspace is the home folder keeps the workspace
// persona file; only a global install follows GENTLE_AI_PI_CONFIG_DIR.
func TestPiPersonaConfigPathsFollowScopeNotRootEquality(t *testing.T) {
	home := t.TempDir()
	piConfigDir := filepath.Join(t.TempDir(), "config")
	t.Setenv(piagent.GentlePiConfigDirEnv, piConfigDir)
	workspace := filepath.Join(home, ".pi", "gentle-ai", "persona.json")
	if got := piPersonaConfigPaths(home, home, ScopeWorkspace); len(got) != 1 || got[0] != workspace {
		t.Fatalf("workspace install in the home folder = %v, want [%s]", got, workspace)
	}
	if got := piPersonaConfigPaths(home, home, ScopeGlobal); len(got) != 1 || got[0] != filepath.Join(piConfigDir, "persona.json") {
		t.Fatalf("global install = %v, want [%s]", got, filepath.Join(piConfigDir, "persona.json"))
	}
}
