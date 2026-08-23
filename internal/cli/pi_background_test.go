package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

func TestPiBackgroundIntentValidation(t *testing.T) {
	for _, tt := range []struct {
		raw, want, errText string
	}{
		{"auto", "auto", ""}, {"on", "on", ""}, {"off", "off", ""}, {"maybe", "", "valid values"},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := model.ParsePiBackgroundIntent(tt.raw)
			if string(got) != tt.want || (tt.errText == "" && err != nil) || (tt.errText != "" && (err == nil || !strings.Contains(err.Error(), tt.errText))) {
				t.Fatalf("parse(%q) = %q, %v", tt.raw, got, err)
			}
		})
	}
	if _, err := ResolvePiBackground(PiBackgroundResolveInput{EnvSet: true, EnvValue: "maybe"}); err == nil || !strings.Contains(err.Error(), "auto, on, or off") {
		t.Fatalf("resolver error = %v, want actionable vocabulary", err)
	}
}

func TestResolvePiBackground(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   PiBackgroundResolveInput
		want PiBackgroundResolution
	}{
		{"cli outranks env and prior", PiBackgroundResolveInput{CLISet: true, CLIValue: model.PiBackgroundOff, EnvSet: true, EnvValue: "on", PriorManaged: model.PiBackgroundOn}, PiBackgroundResolution{Intent: model.PiBackgroundOff, Effective: model.PiBackgroundOff, Persist: model.PiBackgroundOff}},
		{"env outranks prior", PiBackgroundResolveInput{EnvSet: true, EnvValue: "on", PriorManaged: model.PiBackgroundOff}, PiBackgroundResolution{Intent: model.PiBackgroundOn, Effective: model.PiBackgroundOn, Persist: model.PiBackgroundOn}},
		{"prior managed on avoids prompt", PiBackgroundResolveInput{PriorManaged: model.PiBackgroundOn, Interactive: true}, PiBackgroundResolution{Intent: model.PiBackgroundOn, Effective: model.PiBackgroundOn}},
		{"prior managed off avoids prompt", PiBackgroundResolveInput{PriorManaged: model.PiBackgroundOff, Interactive: true}, PiBackgroundResolution{Intent: model.PiBackgroundOff, Effective: model.PiBackgroundOff}},
		{"unresolved interactive needs prompt", PiBackgroundResolveInput{Interactive: true}, PiBackgroundResolution{Intent: model.PiBackgroundAuto, Effective: model.PiBackgroundAuto, NeedsPrompt: true}},
		{"unresolved noninteractive stays off", PiBackgroundResolveInput{}, PiBackgroundResolution{Intent: model.PiBackgroundAuto, Effective: model.PiBackgroundOff}},
		{"empty env is ignored", PiBackgroundResolveInput{EnvSet: true, Interactive: true}, PiBackgroundResolution{Intent: model.PiBackgroundAuto, Effective: model.PiBackgroundAuto, NeedsPrompt: true}},
		{"empty env is ignored noninteractive", PiBackgroundResolveInput{EnvSet: true}, PiBackgroundResolution{Intent: model.PiBackgroundAuto, Effective: model.PiBackgroundOff}},
		{"explicit auto consults prior policy", PiBackgroundResolveInput{CLISet: true, CLIValue: model.PiBackgroundAuto, EnvSet: true, EnvValue: "on", PriorManaged: model.PiBackgroundOn}, PiBackgroundResolution{Intent: model.PiBackgroundAuto, Effective: model.PiBackgroundOn}},
		{"explicit auto is interactive auto", PiBackgroundResolveInput{CLISet: true, CLIValue: model.PiBackgroundAuto, Interactive: true}, PiBackgroundResolution{Intent: model.PiBackgroundAuto, Effective: model.PiBackgroundAuto}},
		{"explicit auto cannot enable without prior", PiBackgroundResolveInput{CLISet: true, CLIValue: model.PiBackgroundAuto, EnvSet: true, EnvValue: "on"}, PiBackgroundResolution{Intent: model.PiBackgroundAuto, Effective: model.PiBackgroundOff}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePiBackground(tt.in)
			got.managed = false // projection gating is proven by TestProjectionOnlyForManagedDecisions
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("resolution = %#v, error = %v, want %#v", got, err, tt.want)
			}
		})
	}
}

func TestResolvePiBackgroundRejectsInvalidValues(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   PiBackgroundResolveInput
	}{
		{name: "invalid explicit CLI value", in: PiBackgroundResolveInput{CLISet: true, CLIValue: model.PiBackgroundIntent("maybe")}},
		{name: "invalid prior managed state", in: PiBackgroundResolveInput{PriorManaged: model.PiBackgroundIntent("maybe")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ResolvePiBackground(tt.in); err == nil {
				t.Fatal("ResolvePiBackground() error = nil, want validation error")
			}
		})
	}
}

func TestResolvePiBackgroundPersistence(t *testing.T) {
	for _, tt := range []struct {
		name    string
		in      PiBackgroundResolveInput
		persist model.PiBackgroundIntent
	}{
		{name: "explicit auto", in: PiBackgroundResolveInput{CLISet: true, CLIValue: model.PiBackgroundAuto}},
		{name: "default auto", in: PiBackgroundResolveInput{}},
		{name: "inherited prior on", in: PiBackgroundResolveInput{PriorManaged: model.PiBackgroundOn}},
		{name: "inherited prior off", in: PiBackgroundResolveInput{PriorManaged: model.PiBackgroundOff}},
		{name: "explicit on", in: PiBackgroundResolveInput{CLISet: true, CLIValue: model.PiBackgroundOn}, persist: model.PiBackgroundOn},
		{name: "explicit off", in: PiBackgroundResolveInput{CLISet: true, CLIValue: model.PiBackgroundOff}, persist: model.PiBackgroundOff},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePiBackground(tt.in)
			if err != nil {
				t.Fatalf("ResolvePiBackground() error = %v", err)
			}
			if got.Persist != tt.persist {
				t.Errorf("Persist = %q, want %q", got.Persist, tt.persist)
			}
		})
	}
}

func TestPiBackgroundEnvResolutionOutranksPriorState(t *testing.T) {
	t.Setenv(PiBackgroundSubagentsEnv, "on")
	resolved, err := resolvePiBackgroundCLI(false, "", state.InstallState{PiBackgroundIntent: model.PiBackgroundOff})
	if err != nil || resolved.Effective != model.PiBackgroundOn || resolved.Persist != model.PiBackgroundOn {
		t.Fatalf("environment resolution = %#v, err = %v", resolved, err)
	}
}

func TestPiBackgroundStateIsOptionalAndLossless(t *testing.T) {
	home := t.TempDir()
	want := state.InstallState{
		InstalledAgents: []string{"pi"}, ManagedAssetDigest: "sha256:asset", SelectionConfigured: true,
		Components: []model.ComponentID{model.ComponentEngram}, Persona: "neutral", PersonaPresent: true,
		PiBackgroundIntent: model.PiBackgroundOn,
	}
	if err := state.Write(home, want); err != nil {
		t.Fatal(err)
	}
	got, err := state.Read(home)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("state round-trip = %#v, error = %v, want %#v", got, err, want)
	}
	raw, err := os.ReadFile(state.Path(home))
	if err != nil || !strings.Contains(string(raw), `"pi_background_subagents": "on"`) {
		t.Fatalf("persisted key missing: %s, error = %v", raw, err)
	}

	legacy := t.TempDir()
	if err := os.MkdirAll(filepath.Join(legacy, ".gentle-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.Path(legacy), []byte(`{"installed_agents":["pi"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = state.Read(legacy)
	if err != nil || got.PiBackgroundIntent != "" || len(got.InstalledAgents) != 1 {
		t.Fatalf("legacy state = %#v, error = %v", got, err)
	}
}

func TestPiBackgroundStateMergePreservesIntent(t *testing.T) {
	got := state.MergeAgents(state.InstallState{
		InstalledAgents:    []string{"claude-code"},
		PiBackgroundIntent: model.PiBackgroundOff,
	}, []string{"pi"})
	if got.PiBackgroundIntent != model.PiBackgroundOff || len(got.InstalledAgents) != 2 {
		t.Fatalf("merged state = %#v", got)
	}
}

func TestPreparePiBackgroundProjectionHonorsEffectiveIntent(t *testing.T) {
	home := t.TempDir()
	for _, tt := range []struct {
		name   string
		effect model.PiBackgroundIntent
		hasPi  bool
		want   model.PiBackgroundIntent
	}{
		{name: "auto is a no-op", effect: model.PiBackgroundAuto, hasPi: true},
		{name: "no pi agent is a no-op", effect: model.PiBackgroundOn, hasPi: false},
		{name: "unresolved is a no-op", effect: "", hasPi: true},
		{name: "on projects on", effect: model.PiBackgroundOn, hasPi: true, want: model.PiBackgroundOn},
		{name: "off projects off", effect: model.PiBackgroundOff, hasPi: true, want: model.PiBackgroundOff},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resolution := PiBackgroundResolution{Effective: tt.effect, managed: true}
			plan := preparePiBackgroundProjection(home, &resolution, tt.hasPi)
			if tt.want == "" {
				if plan != nil || resolution.projectionPlan != nil {
					t.Fatalf("no-op resolution prepared plan %v", plan)
				}
				return
			}
			if plan == nil || plan.policy != tt.want || resolution.projectionPlan != plan {
				t.Fatalf("prepared plan = %#v, want policy %q", plan, tt.want)
			}
			if plan.path != filepath.Join(home, ".pi", "gentle-ai", "background-subagents.json") {
				t.Fatalf("plan path = %q", plan.path)
			}
		})
	}
}

func readPiBackgroundPolicy(t *testing.T, path string) (string, string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read projected policy: %v", err)
	}
	var decoded struct {
		Schema string `json:"schema"`
		Policy string `json:"policy"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode projected policy %s: %v", raw, err)
	}
	return decoded.Schema, decoded.Policy
}

func TestPiBackgroundProjectionStepWritesResolvedPolicy(t *testing.T) {
	for _, policy := range []model.PiBackgroundIntent{model.PiBackgroundOn, model.PiBackgroundOff} {
		t.Run(string(policy), func(t *testing.T) {
			home := t.TempDir()
			resolution := PiBackgroundResolution{Effective: policy, managed: true}
			plan := preparePiBackgroundProjection(home, &resolution, true)
			step := piBackgroundProjectionStep{id: "pi:background-projection", plan: plan}
			if step.ID() != "pi:background-projection" {
				t.Fatalf("step id = %q", step.ID())
			}
			if err := step.Run(); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			schema, got := readPiBackgroundPolicy(t, plan.path)
			if schema != "gentle-pi.background-subagents/v1" || got != string(policy) {
				t.Fatalf("projected policy = %q/%q, want schema v1 policy %q", schema, got, policy)
			}
			if !reflect.DeepEqual(plan.ChangedPaths(), []string{plan.path}) {
				t.Fatalf("ChangedPaths() = %v, want %v", plan.ChangedPaths(), []string{plan.path})
			}
		})
	}
}

func TestPiBackgroundProjectionHonorsConfigHomeOverride(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(t.TempDir(), "custom-pi")
	t.Setenv(PiConfigHomeEnv, configHome)
	resolution := PiBackgroundResolution{Effective: model.PiBackgroundOn, managed: true}
	plan := preparePiBackgroundProjection(home, &resolution, true)
	want := filepath.Join(configHome, "gentle-ai", "background-subagents.json")
	if plan == nil || plan.path != want {
		t.Fatalf("plan path = %#v, want %q", plan, want)
	}
	if err := plan.Apply(); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("policy under GENTLE_PI_CONFIG_HOME: %v", err)
	}
}

func TestPiBackgroundProjectionRollback(t *testing.T) {
	t.Run("created file is removed", func(t *testing.T) {
		home := t.TempDir()
		resolution := PiBackgroundResolution{Effective: model.PiBackgroundOn, managed: true}
		plan := preparePiBackgroundProjection(home, &resolution, true)
		step := piBackgroundProjectionStep{id: "pi:background-projection", plan: plan}
		if err := step.Run(); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if err := step.Rollback(); err != nil {
			t.Fatalf("Rollback() error = %v", err)
		}
		if _, err := os.Stat(plan.path); !os.IsNotExist(err) {
			t.Fatalf("rollback left created file: %v", err)
		}
	})
	t.Run("prior managed file is restored", func(t *testing.T) {
		home := t.TempDir()
		path := filepath.Join(home, ".pi", "gentle-ai", "background-subagents.json")
		prior := []byte("{\n  \"schema\": \"gentle-pi.background-subagents/v1\",\n  \"policy\": \"on\"\n}\n")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, prior, 0o644); err != nil {
			t.Fatal(err)
		}
		resolution := PiBackgroundResolution{Effective: model.PiBackgroundOff, managed: true}
		plan := preparePiBackgroundProjection(home, &resolution, true)
		step := piBackgroundProjectionStep{id: "pi:background-projection", plan: plan}
		if err := step.Run(); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if _, policy := readPiBackgroundPolicy(t, path); policy != "off" {
			t.Fatalf("projected policy = %q, want off", policy)
		}
		if err := step.Rollback(); err != nil {
			t.Fatalf("Rollback() error = %v", err)
		}
		restored, err := os.ReadFile(path)
		if err != nil || !reflect.DeepEqual(restored, prior) {
			t.Fatalf("restored = %s, error = %v, want prior bytes", restored, err)
		}
	})
	t.Run("unchanged managed file is untouched", func(t *testing.T) {
		home := t.TempDir()
		resolution := PiBackgroundResolution{Effective: model.PiBackgroundOn, managed: true}
		first := preparePiBackgroundProjection(home, &resolution, true)
		if err := first.Apply(); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}
		before, err := os.ReadFile(first.path)
		if err != nil {
			t.Fatal(err)
		}
		second := preparePiBackgroundProjection(home, &resolution, true)
		if err := second.Apply(); err != nil {
			t.Fatalf("re-Apply() error = %v", err)
		}
		if len(second.ChangedPaths()) != 0 {
			t.Fatalf("ChangedPaths() = %v, want none for convergent write", second.ChangedPaths())
		}
		if err := second.Rollback(); err != nil {
			t.Fatalf("Rollback() error = %v", err)
		}
		after, err := os.ReadFile(first.path)
		if err != nil || !reflect.DeepEqual(after, before) {
			t.Fatalf("unchanged file mutated: %s, error = %v", after, err)
		}
	})
}

func TestPiBackgroundProjectionRefusesForeignFile(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
	}{
		{name: "missing schema field", content: `{"policy":"on"}`},
		{name: "foreign schema", content: `{"schema":"someone-else/v9","policy":"on"}`},
		{name: "not json", content: "policy = on\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, ".pi", "gentle-ai", "background-subagents.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			resolution := PiBackgroundResolution{Effective: model.PiBackgroundOn, managed: true}
			plan := preparePiBackgroundProjection(home, &resolution, true)
			step := piBackgroundProjectionStep{id: "pi:background-projection", plan: plan}
			// Non-fatal skip: the surrounding apply stage must survive.
			if err := step.Run(); err != nil {
				t.Fatalf("Run() error = %v, want non-fatal skip", err)
			}
			if !strings.Contains(plan.skipReason, "refusing to overwrite") {
				t.Fatalf("skip reason = %q, want recorded refusal", plan.skipReason)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != tt.content {
				t.Fatalf("foreign file mutated: %s, error = %v", after, err)
			}
			if err := step.Rollback(); err != nil {
				t.Fatalf("Rollback() after refusal error = %v", err)
			}
			after, err = os.ReadFile(path)
			if err != nil || string(after) != tt.content {
				t.Fatalf("foreign file mutated by rollback: %s, error = %v", after, err)
			}
		})
	}
}

func TestProjectionOnlyForManagedDecisions(t *testing.T) {
	writeManagedOn := func(t *testing.T, home string) string {
		t.Helper()
		managedOn := PiBackgroundResolution{Effective: model.PiBackgroundOn, managed: true}
		if err := preparePiBackgroundProjection(home, &managedOn, true).Apply(); err != nil {
			t.Fatal(err)
		}
		return managedOn.projectionPlan.path
	}
	t.Run("implicit auto leaves existing on policy byte-identical", func(t *testing.T) {
		home := t.TempDir()
		path := writeManagedOn(t, home)
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		implicit, err := ResolvePiBackground(PiBackgroundResolveInput{})
		if err != nil || implicit.Effective != model.PiBackgroundOff {
			t.Fatalf("implicit resolution = %#v, error = %v", implicit, err)
		}
		if plan := preparePiBackgroundProjection(home, &implicit, true); plan != nil {
			t.Fatalf("implicit auto prepared plan %#v, want none", plan)
		}
		after, err := os.ReadFile(path)
		if err != nil || !reflect.DeepEqual(after, before) {
			t.Fatalf("existing on policy mutated: %s, error = %v", after, err)
		}
	})
	t.Run("explicit off still rewrites the managed file", func(t *testing.T) {
		home := t.TempDir()
		writeManagedOn(t, home)
		off, err := ResolvePiBackground(PiBackgroundResolveInput{CLISet: true, CLIValue: model.PiBackgroundOff})
		if err != nil {
			t.Fatal(err)
		}
		plan := preparePiBackgroundProjection(home, &off, true)
		if plan == nil {
			t.Fatal("explicit off prepared no plan")
		}
		if err := plan.Apply(); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}
		if _, policy := readPiBackgroundPolicy(t, plan.path); policy != "off" {
			t.Fatalf("policy = %q, want off", policy)
		}
	})
}

func TestPiBackgroundInstallFlagAndHelp(t *testing.T) {
	flags, err := ParseInstallFlags([]string{"--pi-background-subagents=off"})
	if err != nil || !flags.PiBackgroundSubagentsSet || flags.PiBackgroundSubagents != "off" {
		t.Fatalf("explicit pi background flag = %#v, err = %v", flags, err)
	}
	unset, err := ParseInstallFlags(nil)
	if err != nil || unset.PiBackgroundSubagentsSet {
		t.Fatalf("unset pi background flag = %#v, err = %v", unset, err)
	}
	var help strings.Builder
	PrintInstallHelp(&help)
	if !strings.Contains(help.String(), "--pi-background-subagents=auto|on|off") || !strings.Contains(help.String(), PiBackgroundSubagentsEnv) {
		t.Fatalf("install help omits pi background contract: %s", help.String())
	}
}

func TestPiBackgroundSyncFlagAndHelp(t *testing.T) {
	flags, err := ParseSyncFlags([]string{"--pi-background-subagents=on"})
	if err != nil || !flags.PiBackgroundSubagentsSet || flags.PiBackgroundSubagents != "on" {
		t.Fatalf("sync pi background flag = %#v, error = %v", flags, err)
	}
	var help strings.Builder
	PrintSyncHelp(&help)
	if !strings.Contains(help.String(), "--pi-background-subagents=auto|on|off") || !strings.Contains(help.String(), PiBackgroundSubagentsEnv) {
		t.Fatalf("sync help omits pi background contract: %s", help.String())
	}
}

func TestSyncPiBackgroundPrecedenceAndDryRunReporting(t *testing.T) {
	for _, tt := range []struct {
		name        string
		args        []string
		env         string
		prior       model.PiBackgroundIntent
		wantIntent  model.PiBackgroundIntent
		wantEffect  model.PiBackgroundIntent
		wantPersist model.PiBackgroundIntent
	}{
		{name: "cli outranks environment and prior", args: []string{"--pi-background-subagents=on"}, env: "off", prior: model.PiBackgroundOff, wantIntent: model.PiBackgroundOn, wantEffect: model.PiBackgroundOn, wantPersist: model.PiBackgroundOn},
		{name: "environment outranks prior", env: "on", prior: model.PiBackgroundOff, wantIntent: model.PiBackgroundOn, wantEffect: model.PiBackgroundOn, wantPersist: model.PiBackgroundOn},
		{name: "prior state is inherited", prior: model.PiBackgroundOff, wantIntent: model.PiBackgroundOff, wantEffect: model.PiBackgroundOff},
		{name: "unresolved auto stays off", wantIntent: model.PiBackgroundAuto, wantEffect: model.PiBackgroundOff},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := syncBackgroundTestHome(t)
			var before []byte
			if tt.prior != "" {
				if err := state.Write(home, state.InstallState{InstalledAgents: []string{"pi"}, PiBackgroundIntent: tt.prior}); err != nil {
					t.Fatal(err)
				}
				before, _ = os.ReadFile(state.Path(home))
			}
			t.Setenv(PiBackgroundSubagentsEnv, tt.env)
			args := []string{"--agents", "pi", "--dry-run"}
			args = append(args, tt.args...)
			result, err := RunSync(args)
			if err != nil {
				t.Fatalf("RunSync() error = %v", err)
			}
			if result.PiBackground.Intent != tt.wantIntent || result.PiBackground.Effective != tt.wantEffect || result.PiBackground.Persist != tt.wantPersist {
				t.Fatalf("pi background resolution = %#v, want intent=%q effective=%q persist=%q", result.PiBackground, tt.wantIntent, tt.wantEffect, tt.wantPersist)
			}
			if _, statErr := os.Stat(filepath.Join(home, ".pi", "gentle-ai", "background-subagents.json")); !os.IsNotExist(statErr) {
				t.Fatalf("dry-run projected policy: %v", statErr)
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
