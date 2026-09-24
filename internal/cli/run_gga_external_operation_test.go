package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/gga"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestComponentApplyStepGGARecordsIntentBeforeExternalCommands(t *testing.T) {
	home := t.TempDir()
	step := newGGAExternalOperationStep(t, home)

	runCommand = func(string, ...string) error {
		persisted, err := state.Read(home)
		if err != nil {
			t.Fatalf("state.Read() while external command starts: %v", err)
		}
		if len(persisted.ExternalOperations) != 1 {
			t.Fatalf("external operations while external command starts = %#v, want one durable intent", persisted.ExternalOperations)
		}
		operation := persisted.ExternalOperations[0]
		if operation.Tool != state.ExternalToolGGA || operation.Action != state.ExternalActionInstall || operation.Phase != state.ExternalPhaseIntent {
			t.Fatalf("external operation while external command starts = %#v, want GGA install intent", operation)
		}
		return nil
	}

	if err := step.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestComponentApplyStepGGARecordsHomebrewProvenanceWithoutScriptPaths(t *testing.T) {
	home := t.TempDir()
	step := newGGAExternalOperationStep(t, home)
	step.profile = system.PlatformProfile{OS: "darwin", PackageManager: "brew"}
	observations := 0
	step.observeGGAExternalRoute = func(route string, paths []string) (gga.RouteQueryResult, error) {
		observations++
		if route != "brew" || len(paths) != 0 {
			t.Fatalf("external route observation = (%q, %v), want brew without script paths", route, paths)
		}
		return gga.RouteQueryResult{FormulaPresent: observations == 2}, nil
	}
	commands := 0
	runCommand = func(name string, _ ...string) error {
		commands++
		if name != "brew" {
			t.Fatalf("install command = %q, want brew", name)
		}
		persisted, err := state.Read(home)
		if err != nil {
			t.Fatalf("state.Read() while brew command starts: %v", err)
		}
		if len(persisted.ExternalOperations) != 1 {
			t.Fatalf("external operations while brew command starts = %#v, want one durable intent", persisted.ExternalOperations)
		}
		operation := persisted.ExternalOperations[0]
		if operation.Route != "brew" || operation.BeforePresent || len(operation.Paths) != 0 || operation.Phase != state.ExternalPhaseIntent {
			t.Fatalf("external operation while brew command starts = %#v, want brew intent without script paths and BeforePresent=false", operation)
		}
		return nil
	}

	if err := step.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if observations != 2 || commands != 2 {
		t.Fatalf("brew observations and commands = (%d, %d), want (2, 2)", observations, commands)
	}
	persisted, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.GGA.PackageManager != "brew" || len(persisted.GGA.ScriptInstallPaths) != 0 || len(persisted.ExternalOperations) != 0 {
		t.Fatalf("settled brew install state = %#v, want brew provenance and cleared journal", persisted)
	}
}

func TestComponentApplyStepGGASettlesOnlyNewScriptPaths(t *testing.T) {
	home := t.TempDir()
	paths := gga.ExternalInstallPaths("windows", home)
	if err := os.MkdirAll(filepath.Dir(paths[1]), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths[1], []byte("existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	step := newGGAExternalOperationStep(t, home)

	runCommand = func(_ string, args ...string) error {
		for _, arg := range args {
			if filepath.Base(arg) == "install.sh" {
				if err := os.WriteFile(paths[0], []byte("created"), 0o755); err != nil {
					t.Fatalf("write created GGA path: %v", err)
				}
			}
		}
		return nil
	}

	if err := step.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	persisted, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.ExternalOperations) != 0 {
		t.Fatalf("external operations after settled install = %#v, want none", persisted.ExternalOperations)
	}
	if got, want := persisted.GGA.ScriptInstallPaths, []string{paths[0]}; !reflect.DeepEqual(got, want) {
		t.Fatalf("GGA script provenance = %v, want only newly-created path %v", got, want)
	}
}

func TestComponentApplyStepGGAClearsJournalWhenNoScriptOwnershipWasCreated(t *testing.T) {
	home := t.TempDir()
	paths := gga.ExternalInstallPaths("windows", home)
	if err := os.MkdirAll(filepath.Dir(paths[0]), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("existing"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	step := newGGAExternalOperationStep(t, home)
	runCommand = func(string, ...string) error { return nil }

	if err := step.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	persisted, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.ExternalOperations) != 0 {
		t.Fatalf("external operations after no-op install = %#v, want none", persisted.ExternalOperations)
	}
	if persisted.GGA.PackageManager != "" || len(persisted.GGA.ScriptInstallPaths) != 0 {
		t.Fatalf("GGA provenance after no-op install = %#v, want no ownership claim", persisted.GGA)
	}
}

func TestComponentApplyStepGGACommandFailurePreservesManualJournal(t *testing.T) {
	home := t.TempDir()
	step := newGGAExternalOperationStep(t, home)
	runCommand = func(string, ...string) error { return errors.New("installer failed") }

	if err := step.Run(); err == nil {
		t.Fatal("Run() error = nil, want installer failure")
	}

	assertGGAManualOperation(t, home)
}

func TestComponentApplyStepGGAIndeterminatePostObservationPreservesManualJournal(t *testing.T) {
	home := t.TempDir()
	step := newGGAExternalOperationStep(t, home)
	observations := 0
	step.observeGGAExternalRoute = func(_ string, paths []string) (gga.RouteQueryResult, error) {
		observations++
		if observations == 2 {
			return gga.RouteQueryResult{UnknownPaths: []gga.RoutePathQueryError{{Path: paths[0], Err: errors.New("permission denied")}}}, nil
		}
		return gga.RouteQueryResult{}, nil
	}
	runCommand = func(string, ...string) error { return nil }

	if err := step.Run(); err == nil {
		t.Fatal("Run() error = nil, want indeterminate post-observation failure")
	}

	assertGGAManualOperation(t, home)
}

func assertGGAManualOperation(t *testing.T, home string) {
	t.Helper()
	persisted, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.ExternalOperations) != 1 {
		t.Fatalf("external operations after failed install = %#v, want preserved manual operation", persisted.ExternalOperations)
	}
	if got := persisted.ExternalOperations[0].Phase; got != state.ExternalPhaseManual {
		t.Fatalf("external operation phase after failed install = %q, want %q", got, state.ExternalPhaseManual)
	}
}

func newGGAExternalOperationStep(t *testing.T, home string) componentApplyStep {
	t.Helper()

	originalRunCommand := runCommand
	originalAvailable := ggaAvailableCheck
	originalCleanup := cleanupGGAInstallDir
	t.Cleanup(func() {
		runCommand = originalRunCommand
		ggaAvailableCheck = originalAvailable
		cleanupGGAInstallDir = originalCleanup
	})

	ggaAvailableCheck = func(system.PlatformProfile) bool { return false }
	cleanupGGAInstallDir = func() error { return nil }
	return componentApplyStep{
		id:           "component:gga",
		component:    model.ComponentGGA,
		homeDir:      home,
		workspaceDir: home,
		profile:      system.PlatformProfile{OS: "windows", PackageManager: "winget"},
	}
}
