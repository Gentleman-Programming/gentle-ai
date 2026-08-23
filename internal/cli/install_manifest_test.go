package cli

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// TestInstallWritesManifestOnDurableSuccess covers AC1 — a successful
// install MUST persist the gentle-ai.managed-assets/v1 manifest with
// producer identity matching the running binary version.
func TestInstallWritesManifestOnDurableSuccess(t *testing.T) {
	home := t.TempDir()
	swapPackageVars(t, "2.2.0-test", "abc1234")

	newState := state.InstallState{
		InstalledAgents:        []string{"opencode"},
		InstalledBinaryVersion: AppVersion,
		ManagedAssetDigest:     "sha256:fake-digest",
	}
	newState.SetSelection(model.Selection{Agents: []model.AgentID{model.AgentOpenCode}})

	if err := PublishInstallManagedAssetsManifest(home, newState, []string{"opencode"}, InstallFlags{}, "sha256:fake-digest", "install-test-1"); err != nil {
		t.Fatalf("PublishInstallManagedAssetsManifest: %v", err)
	}

	m, err := state.ReadManifest(home)
	if err != nil {
		t.Fatalf("manifest not written: %v", err)
	}
	if m.Producer.BinaryVersion != AppVersion || m.Producer.Commit != ProducerCommit || m.Schema != state.ManifestSchema {
		t.Errorf("manifest identity wrong: %+v", m.Producer)
	}
	if m.Bundle.Algo != "sha256" || m.Bundle.Digest == "" {
		t.Errorf("bundle metadata wrong: %+v", m.Bundle)
	}
}

// TestInstallJournalIntentPrecedesMutation covers AC8 — the journal MUST
// record the "intent" entry before any resource is mutated, and the
// "complete" entry only after the publish succeeds.
func TestInstallJournalIntentPrecedesMutation(t *testing.T) {
	home := t.TempDir()
	swapPackageVars(t, "2.2.0-test", "")

	newState := state.InstallState{
		InstalledAgents:        []string{"opencode"},
		InstalledBinaryVersion: AppVersion,
		ManagedAssetDigest:     "sha256:fake",
	}
	newState.SetSelection(model.Selection{Agents: []model.AgentID{model.AgentOpenCode}})
	if err := PublishInstallManagedAssetsManifest(home, newState, []string{"opencode"}, InstallFlags{}, "sha256:fake", "install-precedence"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	entries, err := state.ReadJournal(home)
	if err != nil {
		t.Fatalf("ReadJournal: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2; got %+v", len(entries), entries)
	}
	if entries[0].Op != "intent" || entries[0].RunID != "install-precedence" {
		t.Errorf("entries[0] = %+v, want intent/run=install-precedence", entries[0])
	}
	if entries[1].Op != "complete" || entries[1].RunID != "install-precedence" {
		t.Errorf("entries[1] = %+v, want complete/run=install-precedence", entries[1])
	}
	if _, err := os.Stat(state.ManifestPath(home)); err != nil {
		t.Errorf("manifest file not present after durable success: %v", err)
	}
}

// swapPackageVars pins AppVersion and ProducerCommit for the duration of a
// test so the published manifest carries the expected identity.
func swapPackageVars(t *testing.T, version, commit string) {
	t.Helper()
	prevVersion, prevCommit := AppVersion, ProducerCommit
	AppVersion = version
	ProducerCommit = commit
	t.Cleanup(func() {
		AppVersion = prevVersion
		ProducerCommit = prevCommit
	})
}
