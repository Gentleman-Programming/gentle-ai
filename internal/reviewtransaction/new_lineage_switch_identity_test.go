package reviewtransaction

import (
	"context"
	"testing"
)

// This file is coverage closure for spec rdd-new-lineage-activation ->
// "Distinct Env Switch, Default Off, Legacy Path When Disabled" ->
// "Switch identity never overloads another switch": GENTLE_AI_RDD_NEW_LINEAGE
// and the user-owned RDD kill switch are independent reads
// (NewLineageActivationEnabled, ResolveRDDMode). No production behavior
// changes here — these are new tests only.
//
// The third pairing this file originally proved -- GENTLE_AI_RDD_SHADOW
// (shadowObservationEnabled) never overloading the activation switch --
// retired with the shadow observer itself (Wave 7 S2a): with no shadow
// switch left to read, there is nothing left to prove non-overloaded
// against for that pairing.

// TestNewLineageActivationSwitchIndependentOfKillSwitch proves the
// pairing the scenario names: the user-owned RDD kill switch — persisted,
// resolved through ResolveRDDMode, never an environment variable — is
// unaffected by the new-lineage activation env var, and the reverse: setting
// the kill switch off never flips the activation env var's own reading.
func TestNewLineageActivationSwitchIndependentOfKillSwitch(t *testing.T) {
	repo := initSnapshotRepo(t)
	ctx := context.Background()

	for _, activation := range []string{"", "1"} {
		t.Run("kill-switch-resolution-with-activation="+activation, func(t *testing.T) {
			t.Setenv(newLineageActivationEnvVar, activation)
			// An explicit global opt-in, not an absent opinion. Receipt-driven
			// development is off by default, so resolving with no opinion would
			// report disabled for a reason that has nothing to do with the
			// activation variable, and this assertion would stop testing the
			// independence it exists for.
			status, err := ResolveRDDMode(ctx, repo, RDDGlobalMode{Value: string(RDDModeOn)})
			if err != nil {
				t.Fatal(err)
			}
			if !status.Enabled() {
				t.Fatalf("kill switch resolved disabled against an explicit global on (activation=%q) — the activation env var must never influence it", activation)
			}
		})
	}

	// Reverse: recording the kill switch off must never flip the activation
	// switch's own env-only reading, in either direction.
	t.Setenv(newLineageActivationEnvVar, "")
	if _, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", RDDGlobalMode{}); err != nil {
		t.Fatal(err)
	}
	if NewLineageActivationEnabled() {
		t.Fatal("recording the kill switch off turned the activation switch on")
	}
	t.Setenv(newLineageActivationEnvVar, "1")
	if !NewLineageActivationEnabled() {
		t.Fatal("the kill switch being off suppressed the activation env var's own true reading")
	}
	status, err := ResolveRDDMode(ctx, repo, RDDGlobalMode{})
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled() {
		t.Fatal("the activation env var being on reversed the kill switch's own off recording")
	}
}
