package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// TestSyncWritesManifestOnDurableSuccess asserts a successful sync persists
// the gentle-ai.managed-assets/v1 manifest with producer identity matching
// the running binary version.
func TestSyncWritesManifestOnDurableSuccess(t *testing.T) {
	home := t.TempDir()
	swapPackageVars(t, "2.2.0-test", "feedbeef")

	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}
	if err := PublishSyncManagedAssetsManifestWithBackground(home, selection, "sha256:writer", "sync-success", "", ""); err != nil {
		t.Fatalf("publish: %v", err)
	}

	m, err := state.ReadManifest(home)
	if err != nil {
		t.Fatalf("manifest not written: %v", err)
	}
	if m.Producer.BinaryVersion != AppVersion || m.Producer.Commit != ProducerCommit || m.Schema != state.ManifestSchema {
		t.Errorf("manifest identity wrong: %+v", m.Producer)
	}
}

// TestSyncInterruptedNeverPublishesAligned asserts that when a sync run is
// interrupted (journal has an "intent" with no matching "complete"), doctor
// classifies the bundle as "unknown", never "aligned".
func TestSyncInterruptedNeverPublishesAligned(t *testing.T) {
	home := t.TempDir()
	swapPackageVars(t, "2.2.0-test", "")

	runID := "sync-interrupted-1"
	if err := state.AppendJournal(home, "intent", runID, "agents/opencode/AGENTS.md"); err != nil {
		t.Fatalf("AppendJournal(intent): %v", err)
	}
	if _, err := state.ReadManifest(home); err == nil {
		t.Fatal("manifest present before sync completed; want absent")
	}
	if err := state.AppendJournal(home, "complete", "different-run", ""); err != nil {
		t.Fatalf("AppendJournal(complete-different): %v", err)
	}

	if !interruptedRunDetected(readJournalForTest(t, home)) {
		t.Fatalf("journal does not signal an interrupted run")
	}
}

// TestSyncLegacyNoManifestWritesOneShot asserts a sync run on a legacy
// install (state.json present but no manifest) writes the manifest on
// durable success, never corrupting prior state.
func TestSyncLegacyNoManifestWritesOneShot(t *testing.T) {
	home := t.TempDir()
	swapPackageVars(t, "2.1.10", "")

	legacyState := state.InstallState{
		InstalledAgents:        []string{"opencode"},
		InstalledBinaryVersion: "2.1.10",
		ManagedAssetDigest:     "sha256:legacy-digest",
	}
	if err := state.Write(home, legacyState); err != nil {
		t.Fatalf("seed legacy state: %v", err)
	}

	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}
	if err := PublishSyncManagedAssetsManifestWithBackground(home, selection, "sha256:new-writer", "sync-legacy", "", ""); err != nil {
		t.Fatalf("publish: %v", err)
	}

	m, err := state.ReadManifest(home)
	if err != nil {
		t.Fatalf("manifest not written for legacy install: %v", err)
	}
	if m.Producer.BinaryVersion != "2.1.10" {
		t.Errorf("Producer.BinaryVersion = %q, want 2.1.10", m.Producer.BinaryVersion)
	}
	after, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.json unreadable after sync: %v", err)
	}
	if len(after.InstalledAgents) != 1 || after.InstalledAgents[0] != "opencode" {
		t.Errorf("InstalledAgents = %v, want [opencode] (legacy state must be preserved)", after.InstalledAgents)
	}
}

func readJournalForTest(t *testing.T, home string) []state.JournalEntry {
	t.Helper()
	entries, err := state.ReadJournal(home)
	if err != nil {
		t.Fatalf("ReadJournal: %v", err)
	}
	return entries
}

// interruptedRunDetected mirrors the doctor-level heuristic: any "intent"
// without a matching "complete" on the same run_id, OR any explicit
// "interrupted" marker.
func interruptedRunDetected(entries []state.JournalEntry) bool {
	intent, complete, interrupted := map[string]bool{}, map[string]bool{}, false
	for _, e := range entries {
		switch e.Op {
		case "intent":
			intent[e.RunID] = true
		case "complete":
			complete[e.RunID] = true
		case "interrupted":
			interrupted = true
		}
	}
	if interrupted {
		return true
	}
	for runID := range intent {
		if !complete[runID] {
			return true
		}
	}
	return false
}
