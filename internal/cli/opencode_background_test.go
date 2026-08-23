package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	opencodeactivation "github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestOpenCodeBackgroundIntentValidation(t *testing.T) {
	for _, tt := range []struct {
		raw, want, errText string
	}{
		{"auto", "auto", ""}, {"on", "on", ""}, {"off", "off", ""}, {"maybe", "", "valid values"},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := model.ParseOpenCodeBackgroundIntent(tt.raw)
			if string(got) != tt.want || (tt.errText == "" && err != nil) || (tt.errText != "" && (err == nil || !strings.Contains(err.Error(), tt.errText))) {
				t.Fatalf("parse(%q) = %q, %v", tt.raw, got, err)
			}
		})
	}
	if _, err := ResolveOpenCodeBackground(OpenCodeBackgroundResolveInput{EnvSet: true, EnvValue: "maybe"}); err == nil || !strings.Contains(err.Error(), "auto, on, or off") {
		t.Fatalf("resolver error = %v, want actionable vocabulary", err)
	}
}

func TestResolveOpenCodeBackground(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   OpenCodeBackgroundResolveInput
		want OpenCodeBackgroundResolution
	}{
		{"cli outranks env and prior", OpenCodeBackgroundResolveInput{CLISet: true, CLIValue: model.OpenCodeBackgroundOff, EnvSet: true, EnvValue: "on", PriorManaged: model.OpenCodeBackgroundOn}, OpenCodeBackgroundResolution{Intent: model.OpenCodeBackgroundOff, Effective: model.OpenCodeBackgroundOff, Persist: model.OpenCodeBackgroundOff}},
		{"env outranks prior", OpenCodeBackgroundResolveInput{EnvSet: true, EnvValue: "on", PriorManaged: model.OpenCodeBackgroundOff}, OpenCodeBackgroundResolution{Intent: model.OpenCodeBackgroundOn, Effective: model.OpenCodeBackgroundOn, Persist: model.OpenCodeBackgroundOn}},
		{"prior managed on avoids prompt", OpenCodeBackgroundResolveInput{PriorManaged: model.OpenCodeBackgroundOn, Interactive: true}, OpenCodeBackgroundResolution{Intent: model.OpenCodeBackgroundOn, Effective: model.OpenCodeBackgroundOn}},
		{"prior managed off avoids prompt", OpenCodeBackgroundResolveInput{PriorManaged: model.OpenCodeBackgroundOff, Interactive: true}, OpenCodeBackgroundResolution{Intent: model.OpenCodeBackgroundOff, Effective: model.OpenCodeBackgroundOff}},
		{"unresolved interactive needs prompt", OpenCodeBackgroundResolveInput{Interactive: true}, OpenCodeBackgroundResolution{Intent: model.OpenCodeBackgroundAuto, Effective: model.OpenCodeBackgroundAuto, NeedsPrompt: true}},
		{"unresolved noninteractive stays foreground", OpenCodeBackgroundResolveInput{}, OpenCodeBackgroundResolution{Intent: model.OpenCodeBackgroundAuto, Effective: model.OpenCodeBackgroundOff}},
		{"empty env is ignored", OpenCodeBackgroundResolveInput{EnvSet: true, Interactive: true}, OpenCodeBackgroundResolution{Intent: model.OpenCodeBackgroundAuto, Effective: model.OpenCodeBackgroundAuto, NeedsPrompt: true}},
		{"empty env is ignored noninteractive", OpenCodeBackgroundResolveInput{EnvSet: true}, OpenCodeBackgroundResolution{Intent: model.OpenCodeBackgroundAuto, Effective: model.OpenCodeBackgroundOff}},
		{"explicit auto consults prior policy", OpenCodeBackgroundResolveInput{CLISet: true, CLIValue: model.OpenCodeBackgroundAuto, EnvSet: true, EnvValue: "on", PriorManaged: model.OpenCodeBackgroundOn}, OpenCodeBackgroundResolution{Intent: model.OpenCodeBackgroundAuto, Effective: model.OpenCodeBackgroundOn}},
		{"explicit auto is interactive auto", OpenCodeBackgroundResolveInput{CLISet: true, CLIValue: model.OpenCodeBackgroundAuto, Interactive: true}, OpenCodeBackgroundResolution{Intent: model.OpenCodeBackgroundAuto, Effective: model.OpenCodeBackgroundAuto}},
		{"explicit auto cannot enable without prior", OpenCodeBackgroundResolveInput{CLISet: true, CLIValue: model.OpenCodeBackgroundAuto, EnvSet: true, EnvValue: "on"}, OpenCodeBackgroundResolution{Intent: model.OpenCodeBackgroundAuto, Effective: model.OpenCodeBackgroundOff}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveOpenCodeBackground(tt.in)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("resolution = %#v, error = %v, want %#v", got, err, tt.want)
			}
		})
	}
}

func TestResolveOpenCodeBackgroundRejectsInvalidValues(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   OpenCodeBackgroundResolveInput
	}{
		{
			name: "invalid explicit CLI value",
			in:   OpenCodeBackgroundResolveInput{CLISet: true, CLIValue: model.OpenCodeBackgroundIntent("maybe")},
		},
		{
			name: "invalid prior managed state",
			in:   OpenCodeBackgroundResolveInput{PriorManaged: model.OpenCodeBackgroundIntent("maybe")},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ResolveOpenCodeBackground(tt.in); err == nil {
				t.Fatal("ResolveOpenCodeBackground() error = nil, want validation error")
			}
		})
	}
}

func TestResolveOpenCodeBackgroundPersistence(t *testing.T) {
	for _, tt := range []struct {
		name    string
		in      OpenCodeBackgroundResolveInput
		persist model.OpenCodeBackgroundIntent
	}{
		{
			name: "explicit auto",
			in:   OpenCodeBackgroundResolveInput{CLISet: true, CLIValue: model.OpenCodeBackgroundAuto},
		},
		{
			name: "default auto",
			in:   OpenCodeBackgroundResolveInput{},
		},
		{
			name: "inherited prior on",
			in:   OpenCodeBackgroundResolveInput{PriorManaged: model.OpenCodeBackgroundOn},
		},
		{
			name: "inherited prior off",
			in:   OpenCodeBackgroundResolveInput{PriorManaged: model.OpenCodeBackgroundOff},
		},
		{
			name:    "explicit on",
			in:      OpenCodeBackgroundResolveInput{CLISet: true, CLIValue: model.OpenCodeBackgroundOn},
			persist: model.OpenCodeBackgroundOn,
		},
		{
			name:    "explicit off",
			in:      OpenCodeBackgroundResolveInput{CLISet: true, CLIValue: model.OpenCodeBackgroundOff},
			persist: model.OpenCodeBackgroundOff,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveOpenCodeBackground(tt.in)
			if err != nil {
				t.Fatalf("ResolveOpenCodeBackground() error = %v", err)
			}
			if got.Persist != tt.persist {
				t.Errorf("Persist = %q, want %q", got.Persist, tt.persist)
			}
		})
	}
}

func TestPrepareOpenCodeBackgroundActivationHonorsEffectiveIntent(t *testing.T) {
	home := t.TempDir()
	oldVersion, oldTarget := runOpenCodeVersion, resolveOpenCodeTarget
	runOpenCodeVersion = func(string) (string, error) { return "1.15.11", nil }
	resolveOpenCodeTarget = func(string, string, string) (string, error) { return "/real/opencode", nil }
	t.Cleanup(func() { runOpenCodeVersion, resolveOpenCodeTarget = oldVersion, oldTarget })

	for _, tt := range []struct {
		name   string
		effect model.OpenCodeBackgroundIntent
		want   string
	}{
		{name: "auto is a no-op", effect: model.OpenCodeBackgroundAuto},
		{name: "on prepares activation", effect: model.OpenCodeBackgroundOn, want: "on"},
		{name: "off prepares deactivation", effect: model.OpenCodeBackgroundOff, want: "off"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resolution := OpenCodeBackgroundResolution{Effective: tt.effect}
			plan, err := prepareOpenCodeBackgroundActivation(home, &resolution, true)
			if err != nil {
				t.Fatalf("prepareOpenCodeBackgroundActivation() error = %v", err)
			}
			if tt.want == "" {
				if plan != nil || resolution.activationPlan != nil {
					t.Fatalf("auto resolution prepared plan %v", plan)
				}
				return
			}
			if plan == nil || plan.Report().Action != tt.want {
				t.Fatalf("prepared action = %v, want %q", plan, tt.want)
			}
		})
	}
}

func TestOpenCodeBackgroundStateIsOptionalAndLossless(t *testing.T) {
	home := t.TempDir()
	want := state.InstallState{
		InstalledAgents: []string{"opencode"}, ManagedAssetDigest: "sha256:asset", SelectionConfigured: true,
		Components: []model.ComponentID{model.ComponentSDD}, CommunityTools: []string{"codegraph"}, CommunityToolsConfigured: true,
		Persona: "neutral", PersonaPresent: true, PendingSync: true, RDDMode: "off", BackgroundIntent: model.OpenCodeBackgroundOn,
	}
	if err := state.Write(home, want); err != nil {
		t.Fatal(err)
	}
	got, err := state.Read(home)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("state round-trip = %#v, error = %v, want %#v", got, err, want)
	}

	legacy := t.TempDir()
	if err := os.MkdirAll(filepath.Join(legacy, ".gentle-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.Path(legacy), []byte(`{"installed_agents":["opencode"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = state.Read(legacy)
	if err != nil || got.BackgroundIntent != "" || len(got.InstalledAgents) != 1 {
		t.Fatalf("legacy state = %#v, error = %v", got, err)
	}
}

func TestOpenCodeBackgroundStateMergePreservesIntent(t *testing.T) {
	got := state.MergeAgents(state.InstallState{
		InstalledAgents:  []string{"claude-code"},
		BackgroundIntent: model.OpenCodeBackgroundOff,
	}, []string{"opencode"})
	if got.BackgroundIntent != model.OpenCodeBackgroundOff || len(got.InstalledAgents) != 2 {
		t.Fatalf("merged state = %#v", got)
	}
}

func TestDryRunReportsBackgroundIntentWithoutWritingState(t *testing.T) {
	flags, err := ParseInstallFlags([]string{"--opencode-background-subagents=off"})
	if err != nil || !flags.OpenCodeBackgroundSubagentsSet || flags.OpenCodeBackgroundSubagents != "off" {
		t.Fatalf("explicit background flag = %#v, err = %v", flags, err)
	}
	var help strings.Builder
	PrintInstallHelp(&help)
	if !strings.Contains(help.String(), "--opencode-background-subagents=auto|on|off") || !strings.Contains(help.String(), OpenCodeBackgroundSubagentsEnv) {
		t.Fatalf("install help omits background contract: %s", help.String())
	}
	t.Setenv(OpenCodeBackgroundSubagentsEnv, "on")
	resolved, err := resolveOpenCodeBackgroundCLI(false, "", state.InstallState{BackgroundIntent: model.OpenCodeBackgroundOff})
	if err != nil || resolved.Effective != model.OpenCodeBackgroundOn || resolved.Persist != model.OpenCodeBackgroundOn {
		t.Fatalf("environment resolution = %#v, err = %v", resolved, err)
	}
	home := t.TempDir()
	original := osUserHomeDir
	originalVersion, originalTarget := runOpenCodeVersion, resolveOpenCodeTarget
	osUserHomeDir = func() (string, error) { return home, nil }
	runOpenCodeVersion = func(string) (string, error) { return "development", nil }
	resolveOpenCodeTarget = func(string, string, string) (string, error) {
		return filepath.Join(home, "opencode-real"), nil
	}
	t.Cleanup(func() { osUserHomeDir = original })
	t.Cleanup(func() { runOpenCodeVersion, resolveOpenCodeTarget = originalVersion, originalTarget })
	result, err := RunInstall([]string{"--dry-run", "--agent", "opencode", "--opencode-background-subagents=on"}, system.DetectionResult{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Background.Intent != model.OpenCodeBackgroundOn || result.Background.Effective != model.OpenCodeBackgroundOn {
		t.Fatalf("background resolution = %#v", result.Background)
	}
	if _, err := os.Stat(state.Path(home)); !os.IsNotExist(err) {
		t.Fatalf("dry-run state error = %v, want no state file", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode")); !os.IsNotExist(err) {
		t.Fatalf("dry-run asset directory error = %v, want no assets", err)
	}
	report := RenderDryRun(result)
	if !strings.Contains(report, "policy effective: on") || !strings.Contains(report, "runtime ready: false") || !strings.Contains(report, "activation status: unknown") {
		t.Fatalf("untruthful dry-run report: %s", report)
	}
}

func TestInstallActivationCapabilityControlsPolicyAndReport(t *testing.T) {
	for _, tt := range []struct {
		name       string
		version    string
		wantStatus string
		wantReady  bool
		wantPolicy bool
		wantNote   string
	}{
		{name: "ready", version: "1.15.11", wantStatus: "ready", wantReady: true, wantPolicy: true},
		{name: "unsupported", version: "1.15.10", wantStatus: "unsupported", wantNote: "execution stays foreground"},
		{name: "unknown", version: "development", wantStatus: "unknown", wantNote: "execution stays foreground"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := installTestHome(t)
			oldVersion, oldTarget, oldPath := runOpenCodeVersion, resolveOpenCodeTarget, addUserPath
			runOpenCodeVersion = func(string) (string, error) { return tt.version, nil }
			realTarget := filepath.Join(home, "opencode-real")
			if err := os.WriteFile(realTarget, []byte("real"), 0o755); err != nil {
				t.Fatal(err)
			}
			resolveOpenCodeTarget = func(string, string, string) (string, error) { return realTarget, nil }
			addUserPath = func(string) error { return nil }
			t.Cleanup(func() { runOpenCodeVersion, resolveOpenCodeTarget, addUserPath = oldVersion, oldTarget, oldPath })

			result, err := RunInstall([]string{"--agent", "opencode", "--component", "sdd", "--opencode-background-subagents=on"}, system.DetectionResult{})
			if err != nil {
				t.Fatal(err)
			}
			if string(result.Background.Activation.Capability.Status) != tt.wantStatus || result.BackgroundPolicyEnabled != tt.wantPolicy {
				t.Fatalf("activation = %#v policy=%t, want status=%q policy=%t", result.Background.Activation, result.BackgroundPolicyEnabled, tt.wantStatus, tt.wantPolicy)
			}
			if result.Background.Activation.Capability.Ready() != tt.wantReady {
				t.Fatalf("capability ready = %t, want %t", result.Background.Activation.Capability.Ready(), tt.wantReady)
			}
			if tt.wantNote != "" && !strings.Contains(result.Verify.FinalNote, tt.wantNote) {
				t.Fatalf("verification note = %q, want %q", result.Verify.FinalNote, tt.wantNote)
			}
		})
	}
}

func TestBackgroundPolicyForwardingIsOpenCodeOnly(t *testing.T) {
	original := injectSDD
	t.Cleanup(func() { injectSDD = original })
	for _, tt := range []struct {
		agent model.AgentID
		want  bool
	}{{model.AgentOpenCode, true}, {model.AgentKilocode, false}} {
		var got bool
		injectSDD = func(_ string, adapter agents.Adapter, _ model.SDDModeID, options ...sdd.InjectOptions) (sdd.InjectionResult, error) {
			if adapter.Agent() != tt.agent {
				t.Fatalf("adapter = %q, want %q", adapter.Agent(), tt.agent)
			}
			if len(options) > 0 {
				got = options[0].IncludeOpenCodeBackgroundPolicy
			}
			return sdd.InjectionResult{}, nil
		}
		if err := (componentApplyStep{component: model.ComponentSDD, homeDir: t.TempDir(), workspaceDir: t.TempDir(), scope: ScopeGlobal, agents: []model.AgentID{tt.agent}, selection: model.Selection{SDDMode: model.SDDModeSingle}, backgroundPolicy: true}).Run(); err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Fatalf("policy = %t, want %t", got, tt.want)
		}
	}
}

func installTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	oldHome, oldRun, oldLookPath := osUserHomeDir, runCommand, cmdLookPath
	oldVersion, oldTarget := runOpenCodeVersion, resolveOpenCodeTarget
	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = missingBinaryLookPath
	runOpenCodeVersion = func(string) (string, error) { return "development", nil }
	resolveOpenCodeTarget = func(string, string, string) (string, error) {
		return filepath.Join(home, "opencode-real"), nil
	}
	t.Cleanup(func() {
		osUserHomeDir, runCommand, cmdLookPath = oldHome, oldRun, oldLookPath
		runOpenCodeVersion, resolveOpenCodeTarget = oldVersion, oldTarget
	})
	return home
}

func TestInstallPublishesIntentTransactionally(t *testing.T) {
	for _, tt := range []struct {
		name, intent, wantErr string
		injectErr             error
		dropAssets            bool
		wantAsset             bool
	}{
		{name: "success on", intent: "on", wantAsset: true},
		{name: "success off", intent: "off", wantAsset: true},
		{name: "pipeline failure", intent: "on", injectErr: errors.New("pipeline injection failed"), wantErr: "execute install pipeline"},
		{name: "post verification failure", intent: "on", dropAssets: true, wantErr: "post-apply verification failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := installTestHome(t)
			oldInject := injectSDD
			injectSDD = func(path string, adapter agents.Adapter, mode model.SDDModeID, options ...sdd.InjectOptions) (sdd.InjectionResult, error) {
				if tt.injectErr != nil {
					return sdd.InjectionResult{}, tt.injectErr
				}
				if tt.dropAssets {
					return sdd.InjectionResult{}, nil
				}
				return sdd.Inject(path, adapter, mode, options...)
			}
			t.Cleanup(func() { injectSDD = oldInject })

			result, err := RunInstall([]string{"--agent", "opencode", "--component", "sdd", "--opencode-background-subagents=" + tt.intent}, system.DetectionResult{})
			if (err != nil) != (tt.wantErr != "") || (err != nil && !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("RunInstall() error = %v, want %q", err, tt.wantErr)
			}
			if tt.wantErr == "" && (!result.Verify.Ready || result.BackgroundPolicyEnabled || (tt.intent == "on" && !strings.Contains(result.Verify.FinalNote, "execution stays foreground"))) {
				t.Fatalf("success report = %#v, want foreground fallback when capability is unknown", result)
			}
			got, readErr := state.Read(home)
			if tt.wantErr == "" {
				if readErr != nil || got.BackgroundIntent != model.OpenCodeBackgroundIntent(tt.intent) {
					t.Fatalf("published intent = %q, read error = %v", got.BackgroundIntent, readErr)
				}
			} else if !os.IsNotExist(readErr) {
				t.Fatalf("state after %s = %v, want absent", tt.name, readErr)
			}
			if _, statErr := os.Stat(filepath.Join(home, ".config", "opencode", "opencode.json")); (statErr == nil) != tt.wantAsset {
				t.Fatalf("asset after %s: err = %v, want present = %t", tt.name, statErr, tt.wantAsset)
			}
		})
	}
}

func TestPersistInstallStateFullInstallPreservesUnrelatedFields(t *testing.T) {
	home := t.TempDir()
	lastCheck := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	recorded := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	existing := state.InstallState{InstalledAgents: []string{"claude-code"}, ManagedAssetDigest: "old", LastUpdateCheck: &lastCheck, PendingSync: true, RDDMode: "off", RDDModeRecordedAt: &recorded, BackgroundIntent: model.OpenCodeBackgroundOff}
	if err := state.Write(home, existing); err != nil {
		t.Fatal(err)
	}
	fresh := state.InstallState{InstalledAgents: []string{"opencode"}, SelectionConfigured: true, Components: []model.ComponentID{model.ComponentPermission}, Persona: "gentleman", BackgroundIntent: model.OpenCodeBackgroundOn}
	if err := persistInstallState(home, fresh, []string{"opencode"}, InstallFlags{}, "new"); err != nil {
		t.Fatal(err)
	}
	got, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	want := fresh
	want.ManagedAssetDigest = "new"
	want.LastUpdateCheck, want.PendingSync = existing.LastUpdateCheck, existing.PendingSync
	want.RDDMode, want.RDDModeRecordedAt = existing.RDDMode, existing.RDDModeRecordedAt
	want.PersonaPresent = true
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("full install state = %#v, want %#v", got, want)
	}
}

func TestInstallBackgroundInvalidSourcesFailBeforeMutation(t *testing.T) {
	for _, tt := range []struct {
		name  string
		args  []string
		env   string
		prior bool
	}{
		{name: "invalid CLI", args: []string{"--opencode-background-subagents=maybe"}},
		{name: "invalid environment", env: "maybe"},
		{name: "invalid prior state", prior: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			oldHome, oldInject := osUserHomeDir, injectSDD
			osUserHomeDir = func() (string, error) { return home, nil }
			injectCalls := 0
			injectSDD = func(string, agents.Adapter, model.SDDModeID, ...sdd.InjectOptions) (sdd.InjectionResult, error) {
				injectCalls++
				return sdd.InjectionResult{}, nil
			}
			t.Cleanup(func() { osUserHomeDir, injectSDD = oldHome, oldInject })
			t.Setenv(OpenCodeBackgroundSubagentsEnv, tt.env)
			var before []byte
			if tt.prior {
				if err := state.Write(home, state.InstallState{BackgroundIntent: model.OpenCodeBackgroundIntent("maybe")}); err != nil {
					t.Fatal(err)
				}
				before, _ = os.ReadFile(state.Path(home))
			}
			args := append([]string{"--agent", "opencode", "--component", "sdd"}, tt.args...)
			if _, err := RunInstall(args, system.DetectionResult{}); err == nil {
				t.Fatal("RunInstall() error = nil, want preflight rejection")
			}
			if injectCalls != 0 {
				t.Fatalf("inject calls = %d, want preflight before pipeline", injectCalls)
			}
			if _, statErr := os.Stat(filepath.Join(home, ".config", "opencode")); !os.IsNotExist(statErr) {
				t.Fatalf("asset mutation = %v", statErr)
			}
			if tt.prior {
				after, _ := os.ReadFile(state.Path(home))
				if !reflect.DeepEqual(after, before) {
					t.Fatalf("prior state mutated: before %s after %s", before, after)
				}
			}
		})
	}
}

func syncBackgroundTestHome(t *testing.T) string {
	home := t.TempDir()
	oldHome, oldBackup, oldRun, oldLookPath := osUserHomeDir, backup.UserHomeDirFn, runCommand, cmdLookPath
	oldVersion, oldTarget := runOpenCodeVersion, resolveOpenCodeTarget
	osUserHomeDir = func() (string, error) { return home, nil }
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	runOpenCodeVersion = func(string) (string, error) { return "development", nil }
	resolveOpenCodeTarget = func(string, string, string) (string, error) {
		return filepath.Join(home, "opencode-real"), nil
	}
	t.Cleanup(func() {
		osUserHomeDir, backup.UserHomeDirFn, runCommand, cmdLookPath = oldHome, oldBackup, oldRun, oldLookPath
		runOpenCodeVersion, resolveOpenCodeTarget = oldVersion, oldTarget
	})
	return home
}

func TestSyncBackgroundFlagAndHelp(t *testing.T) {
	flags, err := ParseSyncFlags([]string{"--opencode-background-subagents=on"})
	if err != nil || !flags.OpenCodeBackgroundSubagentsSet || flags.OpenCodeBackgroundSubagents != "on" {
		t.Fatalf("sync background flag = %#v, error = %v", flags, err)
	}
	var help strings.Builder
	PrintSyncHelp(&help)
	if !strings.Contains(help.String(), "--opencode-background-subagents=auto|on|off") || !strings.Contains(help.String(), OpenCodeBackgroundSubagentsEnv) {
		t.Fatalf("sync help omits background contract: %s", help.String())
	}
}

func TestSyncBackgroundPrecedenceAndDryRunReporting(t *testing.T) {
	for _, tt := range []struct {
		name        string
		args        []string
		env         string
		prior       model.OpenCodeBackgroundIntent
		wantIntent  model.OpenCodeBackgroundIntent
		wantEffect  model.OpenCodeBackgroundIntent
		wantPersist model.OpenCodeBackgroundIntent
	}{
		{name: "cli outranks environment and prior", args: []string{"--opencode-background-subagents=on"}, env: "off", prior: model.OpenCodeBackgroundOff, wantIntent: model.OpenCodeBackgroundOn, wantEffect: model.OpenCodeBackgroundOn, wantPersist: model.OpenCodeBackgroundOn},
		{name: "environment outranks prior", env: "on", prior: model.OpenCodeBackgroundOff, wantIntent: model.OpenCodeBackgroundOn, wantEffect: model.OpenCodeBackgroundOn, wantPersist: model.OpenCodeBackgroundOn},
		{name: "prior state is inherited", prior: model.OpenCodeBackgroundOff, wantIntent: model.OpenCodeBackgroundOff, wantEffect: model.OpenCodeBackgroundOff},
		{name: "unresolved auto stays foreground", wantIntent: model.OpenCodeBackgroundAuto, wantEffect: model.OpenCodeBackgroundOff},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := syncBackgroundTestHome(t)
			oldVersion, oldTarget := runOpenCodeVersion, resolveOpenCodeTarget
			runOpenCodeVersion = func(string) (string, error) { return "development", nil }
			resolveOpenCodeTarget = func(string, string, string) (string, error) {
				return filepath.Join(home, "opencode-real"), nil
			}
			t.Cleanup(func() { runOpenCodeVersion, resolveOpenCodeTarget = oldVersion, oldTarget })
			var before []byte
			if tt.prior != "" {
				if err := state.Write(home, state.InstallState{InstalledAgents: []string{"opencode"}, BackgroundIntent: tt.prior}); err != nil {
					t.Fatal(err)
				}
				before, _ = os.ReadFile(state.Path(home))
			}
			t.Setenv(OpenCodeBackgroundSubagentsEnv, tt.env)
			args := []string{"--agents", "opencode", "--dry-run"}
			args = append(args, tt.args...)
			result, err := RunSync(args)
			if err != nil {
				t.Fatalf("RunSync() error = %v", err)
			}
			if result.Background.Intent != tt.wantIntent || result.Background.Effective != tt.wantEffect || result.Background.Persist != tt.wantPersist {
				t.Fatalf("background resolution = %#v, want intent=%q effective=%q persist=%q", result.Background, tt.wantIntent, tt.wantEffect, tt.wantPersist)
			}
			report := RenderSyncReport(result)
			if tt.wantEffect == model.OpenCodeBackgroundOn && (!strings.Contains(report, "activation status: unknown") || !strings.Contains(report, "runtime ready: false")) {
				t.Fatalf("untruthful sync report: %s", report)
			}
			after, readErr := os.ReadFile(state.Path(home))
			if tt.prior == "" {
				if !os.IsNotExist(readErr) {
					t.Fatalf("dry-run state error = %v, want no state file", readErr)
				}
			} else if readErr != nil {
				t.Fatalf("dry-run state read error = %v", readErr)
			} else if !reflect.DeepEqual(after, before) {
				t.Fatal("dry-run changed prior state")
			}
		})
	}
}

func TestSyncBackgroundInvalidSourcesFailBeforeMutation(t *testing.T) {
	for _, tt := range []struct {
		name  string
		args  []string
		env   string
		prior model.OpenCodeBackgroundIntent
	}{
		{name: "invalid CLI", args: []string{"--opencode-background-subagents=maybe"}},
		{name: "invalid environment", env: "maybe"},
		{name: "invalid prior state", prior: model.OpenCodeBackgroundIntent("maybe")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := syncBackgroundTestHome(t)
			oldInject := injectSDD
			injectCalls := 0
			injectSDD = func(string, agents.Adapter, model.SDDModeID, ...sdd.InjectOptions) (sdd.InjectionResult, error) {
				injectCalls++
				return sdd.InjectionResult{}, nil
			}
			t.Cleanup(func() { injectSDD = oldInject })
			t.Setenv(OpenCodeBackgroundSubagentsEnv, tt.env)
			var before []byte
			if tt.prior != "" {
				if err := state.Write(home, state.InstallState{InstalledAgents: []string{"opencode"}, BackgroundIntent: tt.prior}); err != nil {
					t.Fatal(err)
				}
				before, _ = os.ReadFile(state.Path(home))
			}
			args := append([]string{"--agents", "opencode", "--sdd-mode", "single"}, tt.args...)
			if _, err := RunSync(args); err == nil {
				t.Fatal("RunSync() error = nil, want preflight rejection")
			}
			if injectCalls != 0 {
				t.Fatalf("inject calls = %d, want validation before pipeline", injectCalls)
			}
			if _, err := os.Stat(filepath.Join(home, ".config", "opencode")); !os.IsNotExist(err) {
				t.Fatalf("asset mutation = %v", err)
			}
			if tt.prior != "" {
				after, _ := os.ReadFile(state.Path(home))
				if !reflect.DeepEqual(after, before) {
					t.Fatalf("prior state mutated: before %s after %s", before, after)
				}
			}
		})
	}
}

func TestSyncBackgroundPolicyForwardingIsOpenCodeOnly(t *testing.T) {
	oldInject := injectSDD
	t.Cleanup(func() { injectSDD = oldInject })
	for _, tt := range []struct {
		agent model.AgentID
		want  bool
	}{{model.AgentOpenCode, true}, {model.AgentKilocode, false}} {
		t.Run(string(tt.agent), func(t *testing.T) {
			var got bool
			injectSDD = func(_ string, adapter agents.Adapter, _ model.SDDModeID, options ...sdd.InjectOptions) (sdd.InjectionResult, error) {
				if adapter.Agent() != tt.agent {
					t.Fatalf("adapter = %q, want %q", adapter.Agent(), tt.agent)
				}
				if len(options) > 0 {
					got = options[0].IncludeOpenCodeBackgroundPolicy
				}
				return sdd.InjectionResult{}, nil
			}
			if err := (componentSyncStep{
				component: model.ComponentSDD,
				homeDir:   t.TempDir(), workspaceDir: t.TempDir(),
				agents:           []model.AgentID{tt.agent},
				selection:        model.Selection{SDDMode: model.SDDModeSingle},
				backgroundPolicy: true,
			}).Run(); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("policy = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestSyncBackgroundPublicationWaitsForVerification(t *testing.T) {
	for _, tt := range []struct {
		name      string
		intent    model.OpenCodeBackgroundIntent
		injectErr error
		dropAsset bool
		wantErr   string
	}{
		{name: "success on", intent: model.OpenCodeBackgroundOn},
		{name: "success off", intent: model.OpenCodeBackgroundOff},
		{name: "pipeline failure", intent: model.OpenCodeBackgroundOn, injectErr: errors.New("pipeline injection failed"), wantErr: "execute sync pipeline"},
		{name: "verification failure", intent: model.OpenCodeBackgroundOn, dropAsset: true, wantErr: "post-sync verification failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := syncBackgroundTestHome(t)
			lastUpdate := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
			if tt.wantErr == "" {
				prior := model.OpenCodeBackgroundOff
				if tt.intent == model.OpenCodeBackgroundOff {
					prior = model.OpenCodeBackgroundOn
				}
				if err := state.Write(home, state.InstallState{InstalledAgents: []string{"opencode"}, LastUpdateCheck: &lastUpdate, PendingSync: true, RDDMode: "off", BackgroundIntent: prior}); err != nil {
					t.Fatal(err)
				}
			}
			oldInject := injectSDD
			injectSDD = func(path string, adapter agents.Adapter, mode model.SDDModeID, options ...sdd.InjectOptions) (sdd.InjectionResult, error) {
				if tt.injectErr != nil {
					return sdd.InjectionResult{}, tt.injectErr
				}
				if tt.dropAsset {
					return sdd.InjectionResult{}, nil
				}
				return sdd.Inject(path, adapter, mode, options...)
			}
			t.Cleanup(func() { injectSDD = oldInject })

			selection := model.Selection{
				Agents:     []model.AgentID{model.AgentOpenCode},
				Components: []model.ComponentID{model.ComponentSDD, model.ComponentPersona},
				SDDMode:    model.SDDModeSingle,
				Persona:    model.PersonaNeutral,
			}
			background := OpenCodeBackgroundResolution{Intent: tt.intent, Effective: tt.intent, Persist: tt.intent}
			result, err := runSyncWithSelection(home, selection, background, PiBackgroundResolution{})
			if (err != nil) != (tt.wantErr != "") || (err != nil && !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("sync error = %v, want %q", err, tt.wantErr)
			}
			if tt.wantErr == "" {
				if !result.Verify.Ready || result.BackgroundPolicyEnabled || (tt.intent == model.OpenCodeBackgroundOn && !strings.Contains(result.Verify.FinalNote, "execution stays foreground")) {
					t.Fatalf("success result = %#v, want verified foreground fallback when capability is unknown", result)
				}
				got, readErr := state.Read(home)
				if readErr != nil || got.BackgroundIntent != tt.intent || !got.PendingSync || got.RDDMode != "off" || got.LastUpdateCheck == nil || !got.LastUpdateCheck.Equal(lastUpdate) {
					t.Fatalf("published intent = %q, read error = %v", got.BackgroundIntent, readErr)
				}
			} else {
				got, readErr := state.Read(home)
				if !os.IsNotExist(readErr) || got.BackgroundIntent != "" {
					t.Fatalf("state after %s = %#v, read error = %v; want no published intent", tt.name, got, readErr)
				}
			}
		})
	}
}

func TestSyncReportsManagedLauncherChanges(t *testing.T) {
	home := syncBackgroundTestHome(t)
	target := filepath.Join(home, "opencode-real")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldVersion, oldTarget, oldPath := runOpenCodeVersion, resolveOpenCodeTarget, addUserPath
	runOpenCodeVersion = func(string) (string, error) { return "1.15.11", nil }
	resolveOpenCodeTarget = func(string, string, string) (string, error) { return target, nil }
	addUserPath = func(string) error { return nil }
	t.Cleanup(func() { runOpenCodeVersion, resolveOpenCodeTarget, addUserPath = oldVersion, oldTarget, oldPath })

	background := OpenCodeBackgroundResolution{
		Intent:    model.OpenCodeBackgroundOn,
		Effective: model.OpenCodeBackgroundOn,
		Persist:   model.OpenCodeBackgroundOn,
	}
	if _, err := prepareOpenCodeBackgroundActivation(home, &background, true); err != nil {
		t.Fatal(err)
	}
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenCode},
		Components: []model.ComponentID{model.ComponentSDD, model.ComponentPersona},
		SDDMode:    model.SDDModeSingle,
		Persona:    model.PersonaNeutral,
	}
	result, err := runSyncWithSelection(home, selection, background, PiBackgroundResolution{})
	if err != nil {
		t.Fatal(err)
	}
	if result.NoOp || result.FilesChanged == 0 {
		t.Fatalf("sync result = NoOp %t, FilesChanged %d; want launcher change", result.NoOp, result.FilesChanged)
	}
	// Issue #3209: the managed launcher set is host-dependent (POSIX script vs
	// cmd+ps1 pair), so the assertion must name the host's launchers.
	for _, launcher := range opencodeactivation.ManagedLauncherPaths(home, runtime.GOOS) {
		if !contains(result.ChangedFiles, launcher) {
			t.Fatalf("sync ChangedFiles %v missing managed launcher %q", result.ChangedFiles, launcher)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestSyncBackgroundNoOpStillPublishesExplicitIntent(t *testing.T) {
	home := syncBackgroundTestHome(t)
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenCode},
		Components: []model.ComponentID{model.ComponentSDD, model.ComponentPersona},
		SDDMode:    model.SDDModeSingle,
		Persona:    model.PersonaNeutral,
	}
	background := OpenCodeBackgroundResolution{Intent: model.OpenCodeBackgroundOn, Effective: model.OpenCodeBackgroundOn, Persist: model.OpenCodeBackgroundOn}
	if _, err := runSyncWithSelection(home, selection, background, PiBackgroundResolution{}); err != nil {
		t.Fatalf("initial sync error = %v", err)
	}
	persisted, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	persisted.BackgroundIntent = ""
	if err := state.Write(home, persisted); err != nil {
		t.Fatal(err)
	}
	result, err := runSyncWithSelection(home, selection, background, PiBackgroundResolution{})
	if err != nil {
		t.Fatalf("no-op sync error = %v", err)
	}
	if !result.NoOp || result.FilesChanged != 0 {
		t.Fatalf("no-op result = NoOp %t, FilesChanged %d", result.NoOp, result.FilesChanged)
	}
	persisted, err = state.Read(home)
	if err != nil || persisted.BackgroundIntent != model.OpenCodeBackgroundOn {
		t.Fatalf("no-op published intent = %q, error = %v", persisted.BackgroundIntent, err)
	}
}
