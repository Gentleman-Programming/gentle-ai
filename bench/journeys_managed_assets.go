package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// historicalManagedAssetStateSHA256 pins the exact state bytes emitted by
	// `gentle-ai sync --agents opencode` from predecessor 8c2cbd80.
	historicalManagedAssetStateSHA256 = "6c337347a4c94055f321b6e4aef7fa6ee96f4ca0159e2e8d99e385557120a329"
	managedAssetRemediation           = "managed reviewer assets are outdated; run `gentle-ai sync`"
)

//go:embed testdata/managed-assets/state-8c2cbd80.json
var historicalManagedAssetState []byte

var managedAssetsStartCapability = &Capability{
	Verb:  []string{"review", "start"},
	Flags: []string{"--cwd", "--contract", "--target", "--projection", "--agent", "--consent"},
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

func staleManagedAssetsStartIsNotUnknown(r *journeyRun) error {
	status, err := readStatusForContract(r, reviewContractV2, "--agent", "opencode")
	if err != nil {
		return err
	}
	if status.NextTransition.Kind != "execute" || status.NextTransition.ReasonCode != "fresh_target_ready" ||
		status.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("stale managed assets initial STATUS = %+v", status.NextTransition)
	}

	start, err := runPrintedTransition(r, status)
	if err != nil {
		return err
	}
	if start.ExitCode == 0 {
		return fmt.Errorf("stale managed assets START succeeded: %s", start.Stdout)
	}
	var failure struct {
		Operation       string `json:"operation"`
		Phase           string `json:"phase"`
		Code            string `json:"code"`
		MutationOutcome string `json:"mutation_outcome"`
		NextAction      string `json:"next_action"`
		Cause           string `json:"cause"`
	}
	if err := json.Unmarshal([]byte(start.Stdout), &failure); err != nil {
		return fmt.Errorf("parse stale managed assets START failure: %w (stderr: %s)", err, firstLine(start.Stderr))
	}
	if failure.Operation != "review.start" || failure.Phase != "preflight" || failure.Code != "managed_assets_outdated" ||
		failure.MutationOutcome != "not_started" || failure.NextAction != "stop" || failure.Cause != managedAssetRemediation {
		return fmt.Errorf("stale managed assets START failure = %#v", failure)
	}

	inspection, err := proveInspection(r.sandbox)
	if err != nil {
		return err
	}
	if !inspection.Complete || !inspection.Valid || inspection.Totals.CompactEntries != 0 || inspection.Totals.LoadedEntries != 0 || inspection.Totals.Edges != 0 {
		return fmt.Errorf("stale managed assets START created authority: %+v", inspection)
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
			Title:  "Stale managed assets: v2 OpenCode START stops before authority and STATUS remains usable",
			Source: "issue #2822: a stale product-created managed-asset digest refuses before START can mutate; negotiated output must not claim native execution",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: staged prose candidate", Fixture: stageDocs("stale-managed-assets")},
				{Name: "fixture: predecessor managed asset digest", Fixture: staleManagedAssetState},
				{Name: "v2 OpenCode START is typed not-started and reconciliation stays usable", Requires: managedAssetsStartCapability, Composite: staleManagedAssetsStartIsNotUnknown},
			},
		},
	}
}
