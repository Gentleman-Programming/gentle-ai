package reviewtransaction

import (
	"context"
	"testing"
)

// TestRDDModeResolvesOnlyFromItsPersistedSetting proves review activation is
// controlled solely by the user-owned RDD mode, never by an environment toggle.
func TestRDDModeResolvesOnlyFromItsPersistedSetting(t *testing.T) {
	repo := initSnapshotRepo(t)
	ctx := context.Background()

	status, err := ResolveRDDMode(ctx, repo, RDDGlobalMode{})
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled() {
		t.Fatal("RDD mode defaulted on without a persisted opt-in")
	}

	status, err = ResolveRDDMode(ctx, repo, RDDGlobalMode{Value: string(RDDModeOn)})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled() {
		t.Fatal("explicit persisted global RDD opt-in did not enable review mode")
	}

	if _, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", RDDGlobalMode{}); err != nil {
		t.Fatal(err)
	}
	status, err = ResolveRDDMode(ctx, repo, RDDGlobalMode{Value: string(RDDModeOn)})
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled() {
		t.Fatal("persisted clone-local RDD off did not override global opt-in")
	}
}
