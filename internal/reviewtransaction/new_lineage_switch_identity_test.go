package reviewtransaction

import (
	"context"
	"testing"
)

// This file is coverage closure for spec rdd-new-lineage-activation ->
// "Distinct Env Switch, Default Off, Legacy Path When Disabled" ->
// "Switch identity never overloads another switch": GENTLE_AI_RDD_NEW_LINEAGE,
// GENTLE_AI_RDD_SHADOW, and the user-owned RDD kill switch are three
// independent reads (NewLineageActivationEnabled, shadowObservationEnabled,
// ResolveRDDMode). No production behavior changes here — these are new
// tests only.

// TestNewLineageActivationSwitchIdentityNeverOverloadsAnotherSwitch drives
// NewLineageActivationEnabled and shadowObservationEnabled — the switch's own
// single readers — across all four combinations of the two env vars, proving
// each reader answers for its own variable alone.
func TestNewLineageActivationSwitchIdentityNeverOverloadsAnotherSwitch(t *testing.T) {
	cases := []struct {
		name           string
		newLineageEnv  string
		shadowEnv      string
		wantActivation bool
		wantShadow     bool
	}{
		{"both unset", "", "", false, false},
		{"only shadow set", "", "1", false, true},
		{"only activation set", "1", "", true, false},
		{"both set", "1", "1", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(newLineageActivationEnvVar, tc.newLineageEnv)
			t.Setenv(shadowObservationEnvVar, tc.shadowEnv)
			if got := NewLineageActivationEnabled(); got != tc.wantActivation {
				t.Fatalf("NewLineageActivationEnabled() with %s=%q %s=%q = %v, want %v (the shadow switch must never activate new-lineage starts)",
					newLineageActivationEnvVar, tc.newLineageEnv, shadowObservationEnvVar, tc.shadowEnv, got, tc.wantActivation)
			}
			if got := shadowObservationEnabled(); got != tc.wantShadow {
				t.Fatalf("shadowObservationEnabled() with %s=%q %s=%q = %v, want %v (the activation switch must never enable shadow observation)",
					newLineageActivationEnvVar, tc.newLineageEnv, shadowObservationEnvVar, tc.shadowEnv, got, tc.wantShadow)
			}
		})
	}
}

// TestNewLineageActivationSwitchIndependentOfKillSwitch proves the third
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
			status, err := ResolveRDDMode(ctx, repo, RDDGlobalMode{})
			if err != nil {
				t.Fatal(err)
			}
			if !status.Enabled() {
				t.Fatalf("kill switch resolved disabled with no override present (activation=%q) — the activation env var must never influence it", activation)
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
