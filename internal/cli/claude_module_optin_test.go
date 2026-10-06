package cli

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// #5256 T2c B: --claude-orchestrator-modules is the explicit first-install
// opt-in for the user-global Claude module pilot. It is install-only, off by
// default, and accepted only for a global install that selects Claude.

const claudeModulesFlag = "--claude-orchestrator-modules"

const absentPilotFile = "<absent>"

func claudeInstallArgs(extra ...string) []string {
	return append([]string{"--agent", "claude-code", "--components", "persona"}, extra...)
}

// plannedClaudeModulePaths returns the nine paths a pilot-enabled global
// Claude install plans: the core, the seven known module names and the ledger.
func plannedClaudeModulePaths(t *testing.T, home string) []string {
	t.Helper()

	options := agentguidance.RoutingOptions{ClaudeGlobalModules: true, ReviewContract: routingReviewContract}
	paths, err := agentguidance.RoutingPathsWithOptions(home, model.AgentClaudeCode, options)
	if err != nil || len(paths) != 9 {
		t.Fatalf("RoutingPathsWithOptions(pilot) = %v, %v; want nine paths", paths, err)
	}
	return paths
}

// pilotFileBytes reads every path, recording a missing file as absent.
func pilotFileBytes(t *testing.T, paths []string) map[string]string {
	t.Helper()

	files := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			files[path] = string(data)
		case os.IsNotExist(err):
			files[path] = absentPilotFile
		default:
			t.Fatalf("read %s: %v", path, err)
		}
	}
	return files
}

func requireEmptyHome(t *testing.T, home string) {
	t.Helper()

	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("home was written: %v, %v", entries, err)
	}
}

func TestClaudeModuleOptInIsADocumentedInstallFlag(t *testing.T) {
	var help strings.Builder
	PrintInstallHelp(&help)
	if !strings.Contains(help.String(), claudeModulesFlag) {
		t.Errorf("install help omits %s:\n%s", claudeModulesFlag, help.String())
	}
	for _, args := range [][]string{{claudeModulesFlag}, {claudeModulesFlag + "=true"}, {claudeModulesFlag + "=false"}} {
		if _, err := ParseInstallFlags(args); err != nil {
			t.Errorf("ParseInstallFlags(%q) error = %v", args, err)
		}
	}
}

func TestClaudeModuleOptInRejectsWorkspaceAndNonClaudeBeforeWriting(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "workspace scope", args: claudeInstallArgs("--scope", "workspace", claudeModulesFlag), want: "--scope global"},
		{name: "selection without Claude", args: []string{"--agent", "opencode", "--components", "persona", claudeModulesFlag}, want: string(model.AgentClaudeCode)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := installTestHome(t)

			_, err := RunInstall(tt.args, system.DetectionResult{})
			if err == nil || !strings.Contains(err.Error(), claudeModulesFlag) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RunInstall(%q) error = %v, want a %s rejection naming %q", tt.args, err, claudeModulesFlag, tt.want)
			}
			requireEmptyHome(t, home)
		})
	}

	t.Run("explicit false keeps the default", func(t *testing.T) {
		home := installTestHome(t)
		args := []string{"--dry-run", "--scope", "workspace", "--agent", "opencode", claudeModulesFlag + "=false"}
		if _, err := RunInstall(args, system.DetectionResult{}); err != nil {
			t.Fatalf("RunInstall(%q) error = %v", args, err)
		}
		requireEmptyHome(t, home)
	})
}

func TestClaudeModuleOptInDefaultsToMonolithAndDryRunWritesNothing(t *testing.T) {
	t.Run("default install", func(t *testing.T) {
		home := installTestHome(t)
		if _, err := RunInstall(claudeInstallArgs(), system.DetectionResult{}); err != nil {
			t.Fatalf("RunInstall() error = %v", err)
		}
		paths := plannedClaudeModulePaths(t, home)
		files := pilotFileBytes(t, paths)
		for _, path := range paths {
			// Only the monolithic core exists; no module or ledger.
			if want, got := path == paths[0], files[path] != absentPilotFile; got != want {
				t.Errorf("default install: %s present = %v, want %v", path, got, want)
			}
		}
	})

	t.Run("dry run", func(t *testing.T) {
		home := installTestHome(t)
		result, err := RunInstall(claudeInstallArgs("--dry-run", claudeModulesFlag), system.DetectionResult{})
		if err != nil || !result.DryRun {
			t.Fatalf("RunInstall(dry run) = DryRun %v, error %v", result.DryRun, err)
		}
		requireEmptyHome(t, home)
	})
}

func TestClaudeModuleOptInCreatesPilotAndKeepsItOnRerun(t *testing.T) {
	home := installTestHome(t)
	paths := plannedClaudeModulePaths(t, home)

	if _, err := RunInstall(claudeInstallArgs(claudeModulesFlag), system.DetectionResult{}); err != nil {
		t.Fatalf("opt-in RunInstall() error = %v", err)
	}
	if installed, err := agentguidance.ClaudeGlobalModulePaths(home); err != nil || !slices.Equal(installed, paths) {
		t.Fatalf("ClaudeGlobalModulePaths() = %v, %v; want the installed pilot %v", installed, err, paths)
	}
	pilot := pilotFileBytes(t, paths)
	core, ledger := paths[0], paths[len(paths)-1]
	if pilot[core] == absentPilotFile || pilot[ledger] == absentPilotFile {
		t.Fatalf("opt-in install wrote no core or ledger: %v", pilot)
	}
	modules := 0
	for _, path := range paths[1 : len(paths)-1] {
		if pilot[path] != absentPilotFile {
			modules++
			if !strings.Contains(pilot[core], filepath.Base(path)) {
				t.Errorf("core does not point at installed module %s", path)
			}
		}
	}
	if modules == 0 {
		t.Fatal("opt-in install wrote no module")
	}

	// The pre-install snapshot planned all nine paths, recording the absent
	// ones, so restoring it removes the pilot again.
	manifests, err := filepath.Glob(filepath.Join(home, ".gentle-ai", "backups", "*", backup.ManifestFilename))
	if err != nil || len(manifests) != 1 {
		t.Fatalf("install snapshots = %v, %v; want exactly one", manifests, err)
	}
	manifest, err := backup.ReadManifest(manifests[0])
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}
	for _, path := range paths {
		if !slices.ContainsFunc(manifest.Entries, func(entry backup.ManifestEntry) bool {
			return entry.OriginalPath == path && (path == core || !entry.Existed)
		}) {
			t.Errorf("pre-install snapshot does not record %s as it was before the install", path)
		}
	}

	// A rerun with the flag and a later install without it keep the pilot
	// files byte for byte; backups and state may still change.
	for _, args := range [][]string{claudeInstallArgs(claudeModulesFlag), claudeInstallArgs()} {
		if _, err := RunInstall(args, system.DetectionResult{}); err != nil {
			t.Fatalf("RunInstall(%q) error = %v", args, err)
		}
		if got := pilotFileBytes(t, paths); !maps.Equal(got, pilot) {
			t.Fatalf("RunInstall(%q) changed the installed pilot files", args)
		}
	}
}

// A ledger is detected by presence only, so a malformed one still selects the
// pilot; module preparation then rejects it before any pilot file is written.
// Earlier steps of the same run (hooks, persona) may write and are restored by
// the ordinary install rollback; this test asserts only the nine pilot paths.
func TestClaudeModuleOptInMalformedLedgerFailsWithoutPilotWrites(t *testing.T) {
	home := installTestHome(t)
	paths := plannedClaudeModulePaths(t, home)
	mustWriteFile(t, paths[0], []byte("# My own rules\n"))
	mustWriteFile(t, paths[len(paths)-1], []byte("{not json"))
	before := pilotFileBytes(t, paths)

	_, err := RunInstall(claudeInstallArgs(claudeModulesFlag), system.DetectionResult{})
	if err == nil || !strings.Contains(err.Error(), "execute install pipeline") {
		t.Fatalf("RunInstall(malformed ledger) error = %v, want the install pipeline to fail", err)
	}
	if after := pilotFileBytes(t, paths); !maps.Equal(after, before) {
		t.Fatalf("failed opt-in install changed pilot files:\nbefore %v\nafter  %v", before, after)
	}
}
