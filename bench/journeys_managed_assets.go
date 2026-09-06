package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// historicalManagedAssetStateSHA256 pins the exact state bytes emitted by
// `gentle-ai sync --agents opencode` from predecessor 8c2cbd80.
const historicalManagedAssetStateSHA256 = "6c337347a4c94055f321b6e4aef7fa6ee96f4ca0159e2e8d99e385557120a329"

//go:embed testdata/managed-assets/state-8c2cbd80.json
var historicalManagedAssetState []byte

var managedAssetsStatusCapability = &Capability{
	Verb:  []string{"review", "status"},
	Flags: []string{"--cwd", "--contract", "--agent", "--next-transition"},
}

// staleManagedAssetState copies the exact predecessor product artifact without
// decoding or changing it, reproducing the cross-version stale-asset state.
func staleManagedAssetState(sandbox *Sandbox) error {
	if got := fmt.Sprintf("%x", sha256.Sum256(historicalManagedAssetState)); got != historicalManagedAssetStateSHA256 {
		return fmt.Errorf("historical managed asset state SHA-256 = %s, want %s", got, historicalManagedAssetStateSHA256)
	}
	path := filepath.Join(sandbox.Home, ".gentle-ai", "state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, historicalManagedAssetState, 0o644); err != nil {
		return err
	}
	copied, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(copied, historicalManagedAssetState) {
		return fmt.Errorf("historical managed asset state copy differs from its fixture")
	}
	// The predecessor artifact is the whole install state, and it predates the
	// kill switch, so copying it over the journey's opted-in state.json takes
	// receipt-driven development back off. Opt in again the same way the runner
	// did — through the product's own command, which leaves the stale digest
	// exactly as this fixture wrote it. Without this the journey would measure
	// a review-disabled STATUS instead of the stale-asset refusal it exists to
	// measure.
	return optIntoReviewMode(sandbox)
}

// staleManagedAssetsStatusIsNotUnknown is the RED-first proof for #3299/#4170's
// STATUS-time reclassification: a stale product-created managed-asset digest
// must fail selectorless STATUS's own preflight, as a typed `stop` carrying
// the exact candidate-preserving `gentle-ai sync` continuation, BEFORE STATUS
// ever offers a START that preflight would refuse anyway. Executing a printed
// START used to be the only way to discover the skew; now the skew is visible
// -- and its one runnable remedy is named -- without ever attempting one.
func staleManagedAssetsStatusIsNotUnknown(r *journeyRun) error {
	status, err := readStatusForContract(r, reviewContractV2, "--agent", "opencode")
	if err != nil {
		return err
	}
	if status.NextTransition.Kind != "stop" || status.NextTransition.ReasonCode != "managed_assets_outdated" ||
		status.NextTransition.Execute.Operation != "" {
		return fmt.Errorf("stale managed assets initial STATUS = %+v", status.NextTransition)
	}
	continuation := status.NextTransition.Continuation
	if continuation == nil || continuation.Operation != "sync" || continuation.Agent != "opencode" ||
		!strings.HasPrefix(continuation.Command, "gentle-ai sync") {
		return fmt.Errorf("stale managed assets STATUS continuation = %+v", continuation)
	}

	inspection, err := proveInspection(r.sandbox)
	if err != nil {
		return err
	}
	if !inspection.Complete || !inspection.Valid || inspection.Totals.CompactEntries != 0 || inspection.Totals.LoadedEntries != 0 || inspection.Totals.Edges != 0 {
		return fmt.Errorf("stale managed assets STATUS created authority: %+v", inspection)
	}

	// STATUS must remain usable: running the exact printed continuation
	// reconciles the recorded digest through the product's own documented
	// remedy (docs/review-integration.md), and the very same candidate is
	// offered again with nothing else about it changed.
	syncArgs, err := printedCommandArguments(continuation.Command)
	if err != nil {
		return fmt.Errorf("stale managed assets continuation %w", err)
	}
	sync := r.run(syncArgs, false)
	if sync.ExitCode != 0 {
		return fmt.Errorf("stale managed assets continuation %q exited %d: %s", continuation.Command, sync.ExitCode, firstLine(sync.Stderr))
	}

	reconciled, err := readStatusForContract(r, reviewContractV2, "--agent", "opencode")
	if err != nil {
		return err
	}
	if reconciled.NextTransition.Kind != "execute" || reconciled.NextTransition.ReasonCode != "fresh_target_ready" ||
		reconciled.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("stale managed assets reconciliation STATUS = %+v", reconciled.NextTransition)
	}
	return nil
}

func managedAssetJourneys() []Journey {
	return []Journey{
		{
			ID:     "j93-stale-managed-assets-start-is-not-unknown",
			Review: reviewOptedIn,
			Title:  "Stale managed assets: v2 OpenCode STATUS stops before authority with a runnable sync continuation, and stays usable",
			Source: "issue #2822, #3299, #4170: a stale product-created managed-asset digest refuses at STATUS's own preflight, before any START is offered, carrying the exact `gentle-ai sync` continuation that reconciles it",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: staged prose candidate", Fixture: stageDocs("stale-managed-assets")},
				{Name: "fixture: predecessor managed asset digest", Fixture: staleManagedAssetState},
				{Name: "v2 OpenCode STATUS is a typed stop with a sync continuation, and stays usable", Requires: managedAssetsStatusCapability, Composite: staleManagedAssetsStatusIsNotUnknown},
			},
		},
	}
}
