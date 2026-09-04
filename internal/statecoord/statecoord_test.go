package statecoord

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

func TestLockPathResolvesHomeSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires privileges unavailable in standard test environments")
	}
	realHome := t.TempDir()
	linkedHome := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(realHome, linkedHome); err != nil {
		t.Fatal(err)
	}

	got, err := LockPath(linkedHome)
	if err != nil {
		t.Fatalf("LockPath() error = %v", err)
	}
	resolvedHome, err := filepath.EvalSymlinks(realHome)
	if err != nil {
		t.Fatalf("EvalSymlinks(real home): %v", err)
	}
	want := state.Path(resolvedHome) + ".lock"
	if got != want {
		t.Fatalf("LockPath() = %q, want %q", got, want)
	}
}

func TestBeginExternalOperationReturnsAfterPersistingIntent(t *testing.T) {
	home := t.TempDir()
	intent := testOperation("first")

	if err := BeginExternalOperation(home, intent); err != nil {
		t.Fatalf("BeginExternalOperation() error = %v", err)
	}
	if got := readState(t, home).ExternalOperations; len(got) != 1 || got[0].Phase != state.ExternalPhaseIntent {
		t.Fatalf("journal after BeginExternalOperation() = %#v", got)
	}
}

func TestExternalOperationTransitionsValidateInputs(t *testing.T) {
	home := t.TempDir()
	if err := BeginExternalOperation(home, state.ExternalOperation{Action: state.ExternalActionInstall, Phase: state.ExternalPhaseIntent}); err == nil {
		t.Fatal("BeginExternalOperation() error = nil, want missing tool error")
	}
	if err := AdvanceExternalOperation(home, state.ExternalOperation{Tool: state.ExternalToolGGA, Phase: state.ExternalPhaseApplied}); err == nil {
		t.Fatal("AdvanceExternalOperation() error = nil, want missing action error")
	}
	if err := ClearExternalOperation(home, state.ExternalOperation{Tool: state.ExternalToolGGA, Action: state.ExternalActionInstall}); err == nil {
		t.Fatal("ClearExternalOperation() error = nil, want missing phase error")
	}
}

func TestExternalOperationTransitionsRejectInvalidPhasesWithoutPersisting(t *testing.T) {
	tests := []struct {
		name           string
		persistedPhase state.ExternalPhase
		requestedPhase state.ExternalPhase
		begin          bool
	}{
		{name: "begin must record intent", persistedPhase: state.ExternalPhaseIntent, requestedPhase: state.ExternalPhaseApplied, begin: true},
		{name: "advance rejects unknown phase", persistedPhase: state.ExternalPhaseIntent, requestedPhase: "unknown"},
		{name: "advance cannot regress from applied", persistedPhase: state.ExternalPhaseApplied, requestedPhase: state.ExternalPhaseIntent},
		{name: "advance cannot regress from manual", persistedPhase: state.ExternalPhaseManual, requestedPhase: state.ExternalPhaseApplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			persisted := testOperation("target")
			persisted.Phase = tt.persistedPhase
			initial := state.InstallState{ExternalOperations: []state.ExternalOperation{persisted}}
			if err := state.Write(home, initial); err != nil {
				t.Fatal(err)
			}

			requested := testOperation("target")
			requested.Phase = tt.requestedPhase
			var err error
			if tt.begin {
				err = BeginExternalOperation(home, requested)
			} else {
				err = AdvanceExternalOperation(home, requested)
			}
			if err == nil {
				t.Fatal("external operation transition error = nil, want invalid phase error")
			}
			if got := readState(t, home); !reflect.DeepEqual(got, initial) {
				t.Fatalf("persisted state = %#v, want %#v", got, initial)
			}
		})
	}
}

func TestExternalOperationTransitionsPreserveLatestStateAndOwnership(t *testing.T) {
	home := t.TempDir()
	original := testOperation("target")
	original.BeforePresent = false
	original.PathBeforePresence = []state.ExternalPathPresence{{Path: original.Paths[0], Present: false}}
	unrelated := testOperation("unrelated")
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"pi"}, ExternalOperations: []state.ExternalOperation{original, unrelated}}); err != nil {
		t.Fatal(err)
	}

	advance := testOperation("target")
	advance.BeforePresent = true
	advance.PathBeforePresence = []state.ExternalPathPresence{{Path: advance.Paths[0], Present: true}}
	advance.Phase = state.ExternalPhaseApplied
	advance.Continuation = "settle target"
	if err := AdvanceExternalOperation(home, advance); err != nil {
		t.Fatal(err)
	}
	if err := AdvanceExternalOperation(home, advance); err != nil {
		t.Fatalf("idempotent AdvanceExternalOperation() error = %v", err)
	}

	persisted := readState(t, home)
	if len(persisted.ExternalOperations) != 2 || persisted.InstalledAgents[0] != "pi" {
		t.Fatalf("latest state was not preserved: %#v", persisted)
	}
	got := persisted.ExternalOperations[0]
	if got.BeforePresent || got.PathBeforePresence[0].Present || got.Phase != state.ExternalPhaseApplied || got.Continuation != "settle target" {
		t.Fatalf("advanced operation = %#v, want original ownership and new transition", got)
	}
}

func TestCallerBlockedExternalPhaseDoesNotHoldStateLock(t *testing.T) {
	home := t.TempDir()
	if err := BeginExternalOperation(home, testOperation("first")); err != nil {
		t.Fatal(err)
	}

	externalStarted := make(chan struct{})
	externalRelease := make(chan struct{})
	externalDone := make(chan struct{})
	go func() {
		close(externalStarted)
		<-externalRelease
		close(externalDone)
	}()
	<-externalStarted

	lockResult := make(chan error, 1)
	go func() {
		lockResult <- WithLock(home, func() error { return nil })
	}()
	select {
	case err := <-lockResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("statecoord operation did not acquire and release the lock while external work was blocked")
	}

	close(externalRelease)
	<-externalDone
}

func TestSettleExternalOperationMutatesProvenanceAndClearsOnlyMatch(t *testing.T) {
	home := t.TempDir()
	target, unrelated := testOperation("target"), testOperation("unrelated")
	if err := state.Write(home, state.InstallState{ExternalOperations: []state.ExternalOperation{target, unrelated}}); err != nil {
		t.Fatal(err)
	}
	if err := SettleExternalOperation(home, target, func(persisted *state.InstallState) error {
		persisted.ManagedAssetDigest = "digest"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	persisted := readState(t, home)
	if persisted.ManagedAssetDigest != "digest" || len(persisted.ExternalOperations) != 1 || persisted.ExternalOperations[0].Route != unrelated.Route {
		t.Fatalf("settled state = %#v", persisted)
	}
	if err := ClearExternalOperation(home, unrelated); err != nil {
		t.Fatal(err)
	}
	if got := readState(t, home).ExternalOperations; got != nil {
		t.Fatalf("cleared journal = %#v, want nil", got)
	}
}

func readState(t *testing.T, home string) state.InstallState {
	t.Helper()
	persisted, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	return persisted
}

func testOperation(route string) state.ExternalOperation {
	return state.ExternalOperation{
		Tool:   state.ExternalToolGGA,
		Action: state.ExternalActionInstall,
		Route:  route,
		Paths:  []string{"/home/example/" + route},
		Phase:  state.ExternalPhaseIntent,
	}
}
