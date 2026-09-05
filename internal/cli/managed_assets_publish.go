package cli

import (
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// ProducerCommit is the source commit identifier baked into the running
// binary. Set by main.go from a GoReleaser ldflag at build time. Tests and
// dev builds default to "unknown" so the manifest is still authoritative for
// the producer identity it does carry (version + commit shape).
var ProducerCommit = "unknown"

// PublishInstallManagedAssetsManifest executes the install-side slice of the
// atomic-commit protocol: persist install state, write the
// gentle-ai.managed-assets/v1 manifest, and journal intent + complete entries
// around the work so a crash between the two never reports a false `aligned`
// to doctor.
//
// The caller passes a unique runID so a parallel or repeated install does not
// conflate journal entries across runs.
func PublishInstallManagedAssetsManifest(homeDir string, newState state.InstallState, agentIDs []string, flags InstallFlags, writer, runID string) error {
	if err := state.AppendJournal(homeDir, "intent", runID, ""); err != nil {
		return err
	}
	if err := persistInstallState(homeDir, newState, agentIDs, flags, writer); err != nil {
		return err
	}
	return publishManagedAssetsManifest(homeDir, runID)
}

// PublishSyncManagedAssetsManifestWithBackground is the production sync
// wiring: it journals intent, persists the sync state with the resolved
// OpenCode/Pi background intents, writes the manifest, and journals
// complete. Used by RunSync so the sync state AND the manifest are
// published as a single durably-committed unit.
func PublishSyncManagedAssetsManifestWithBackground(homeDir string, selection model.Selection, writer, runID string, background model.OpenCodeBackgroundIntent, piBackground model.PiBackgroundIntent) error {
	if err := state.AppendJournal(homeDir, "intent", runID, ""); err != nil {
		return err
	}
	if err := persistSyncManagedAssetStateWithBackground(homeDir, selection, writer, background, piBackground); err != nil {
		return err
	}
	return publishManagedAssetsManifest(homeDir, runID)
}

// publishManagedAssetsManifest writes the manifest (with producer identity
// from AppVersion and ProducerCommit, plus a deterministic bundle digest) and
// then appends a "complete" journal entry. Caller must have already journaled
// the matching "intent" (above).
func publishManagedAssetsManifest(homeDir, runID string) error {
	m := state.Manifest{
		Schema: state.ManifestSchema,
		Producer: state.Producer{
			BinaryVersion: AppVersion,
			Commit:        ProducerCommit,
		},
	}.WithBundleDigest()
	if err := state.WriteManifestAtomic(homeDir, m); err != nil {
		return err
	}
	return state.AppendJournal(homeDir, "complete", runID, "")
}
