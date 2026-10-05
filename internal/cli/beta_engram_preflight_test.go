package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestBetaEngramPreflightStopsBeforeBackupAndApply(t *testing.T) {
	original := cmdLookPath
	t.Cleanup(func() { cmdLookPath = original })
	profiles := []system.PlatformProfile{
		{OS: "darwin", PackageManager: "brew", Supported: true},
		{OS: "linux", PackageManager: "apt", Supported: true},
		{OS: "windows", PackageManager: "winget", Supported: true},
	}
	cases := []struct {
		name                                       string
		channel                                    InstallChannel
		resolvedEngram, requestedEngram, goPresent bool
	}{
		{"beta missing Go", ChannelBeta, true, true, false},
		{"beta Go present", ChannelBeta, true, true, true},
		{"stable missing Go", ChannelStable, true, true, false},
		{"beta no Engram", ChannelBeta, false, false, false},
		{"resolved Engram only", ChannelBeta, true, false, false},
		{"requested Engram only", ChannelBeta, false, true, false},
	}
	for _, profile := range profiles {
		for _, tc := range cases {
			t.Run(profile.OS+"/"+tc.name, func(t *testing.T) {
				goLookups := 0
				cmdLookPath = func(name string) (string, error) {
					if name == "go" {
						goLookups++
						if tc.goPresent {
							return "go", nil
						}
					}
					return "", errNotFound{}
				}
				r := installRuntime{homeDir: t.TempDir(), channel: tc.channel, profile: profile, state: &runtimeState{}}
				if tc.resolvedEngram {
					r.resolved = planner.ResolvedPlan{OrderedComponents: []model.ComponentID{model.ComponentEngram}}
				}
				if tc.requestedEngram {
					r.selection.Components = []model.ComponentID{model.ComponentEngram}
				}
				plan := r.stagePlan()
				if len(plan.Prepare) != 2 || plan.Prepare[0].ID() != "prepare:check-dependencies" || plan.Prepare[1].ID() != "prepare:backup-snapshot" {
					t.Fatalf("unexpected prepare plan: %#v", plan.Prepare)
				}
				backupCalls, applyCalls := 0, 0
				// Keep the real preflight from the actual install plan, replacing
				// only later side effects so reaching them is directly observable.
				plan.Prepare[1] = betaPreflightSentinel{"prepare:backup-snapshot", &backupCalls}
				plan.Apply = []pipeline.Step{betaPreflightSentinel{"component:engram", &applyCalls}}
				result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
				requiresGo := tc.channel == ChannelBeta && tc.resolvedEngram
				if requiresGo && !tc.goPresent {
					if result.Err == nil || backupCalls != 0 || applyCalls != 0 {
						t.Fatalf("missing Go: err=%v backup=%d apply=%d", result.Err, backupCalls, applyCalls)
					}
					for _, want := range []string{"beta Engram requires Go", "before backup or component apply", system.InstallHintForDep("go", profile)} {
						if !strings.Contains(result.Err.Error(), want) {
							t.Fatalf("error %q missing %q", result.Err, want)
						}
					}
				} else if result.Err != nil || backupCalls != 1 || applyCalls != 1 {
					t.Fatalf("exempt/present Go: err=%v backup=%d apply=%d", result.Err, backupCalls, applyCalls)
				}
				wantLookups := 0
				if requiresGo {
					wantLookups = 1
				}
				if goLookups != wantLookups {
					t.Fatalf("Go lookups=%d, want %d", goLookups, wantLookups)
				}
			})
		}
	}
}

func TestBetaEngramPreflightPublicInstallConsumers(t *testing.T) {
	originalHome, originalLookup := osUserHomeDir, cmdLookPath
	originalCLI, originalTUI := installStagePlan, tuiInstallStagePlan
	t.Cleanup(func() {
		osUserHomeDir, cmdLookPath = originalHome, originalLookup
		installStagePlan, tuiInstallStagePlan = originalCLI, originalTUI
	})
	for _, entry := range []string{"beta", "nightly", "tui-stable"} {
		t.Run(entry, func(t *testing.T) {
			home := t.TempDir()
			osUserHomeDir = func() (string, error) { return home, nil }
			goLookups, backupCalls, applyCalls := 0, 0, 0
			cmdLookPath = func(name string) (string, error) {
				if name == "go" {
					goLookups++
				}
				return "", errNotFound{}
			}
			seam := func(r *installRuntime) pipeline.StagePlan {
				wantChannel := ChannelBeta
				if entry == "tui-stable" {
					wantChannel = ChannelStable
				}
				if r.channel != wantChannel {
					t.Fatalf("channel=%q, want %q", r.channel, wantChannel)
				}
				plan := r.stagePlan()
				checks, backups, checkIndex, backupIndex := 0, 0, -1, -1
				for i, step := range plan.Prepare {
					switch step.ID() {
					case "prepare:check-dependencies":
						checks++
						checkIndex = i
					case "prepare:backup-snapshot":
						backups++
						backupIndex = i
						plan.Prepare[i] = betaPreflightSentinel{step.ID(), &backupCalls}
					}
				}
				if checks != 1 || backups != 1 || checkIndex >= backupIndex {
					t.Fatal("missing, duplicate, or reordered preflight/backup steps")
				}
				plan.Apply = []pipeline.Step{betaPreflightSentinel{"component:engram", &applyCalls}}
				return plan
			}
			installStagePlan, tuiInstallStagePlan = seam, seam
			if entry == "tui-stable" {
				selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{model.ComponentEngram}}
				resolved, err := planner.NewResolver(planner.MVPGraph()).Resolve(selection)
				if err != nil {
					t.Fatal(err)
				}
				result, orchestrator := ExecuteTUIInstallWithBackgroundAndOrchestrator(home, selection, resolved, windowsProfile, model.OpenCodeBackgroundAuto, model.PiBackgroundAuto, nil)
				if orchestrator != nil {
					defer orchestrator.Finish()
				}
				if result.Err != nil || goLookups != 0 || backupCalls != 1 || applyCalls != 1 {
					t.Fatalf("stable TUI: err=%v go=%d backup=%d apply=%d", result.Err, goLookups, backupCalls, applyCalls)
				}
				return
			}
			_, err := RunInstall([]string{"--agent", "opencode", "--component", "engram", "--channel", entry}, system.DetectionResult{})
			if err == nil || !strings.Contains(err.Error(), "beta Engram requires Go") || goLookups != 1 || backupCalls != 0 || applyCalls != 0 {
				t.Fatalf("public CLI: err=%v go=%d backup=%d apply=%d", err, goLookups, backupCalls, applyCalls)
			}
		})
	}
}

type betaPreflightSentinel struct {
	id    string
	calls *int
}

func (s betaPreflightSentinel) ID() string { return s.id }
func (s betaPreflightSentinel) Run() error {
	*s.calls++
	return nil
}
