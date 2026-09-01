package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
		Context         struct {
			ManagedAssets *struct {
				ExpectedDigest  string `json:"expected_digest"`
				PersistedDigest string `json:"persisted_digest"`
				BinaryVersion   string `json:"binary_version"`
				BinaryPath      string `json:"binary_path"`
				Remediation     string `json:"remediation"`
			} `json:"managed_assets"`
		} `json:"context"`
	}
	if err := json.Unmarshal([]byte(start.Stdout), &failure); err != nil {
		return fmt.Errorf("parse stale managed assets START failure: %w (stderr: %s)", err, firstLine(start.Stderr))
	}
	if failure.Operation != "review.start" || failure.Phase != "preflight" || failure.Code != "managed_assets_outdated" ||
		failure.MutationOutcome != "not_started" || failure.NextAction != "stop" {
		return fmt.Errorf("stale managed assets START failure = %#v", failure)
	}
	// #3848: the constant remediation sentence stays the stable prefix, and the
	// cause now names both digests so an operator can see the disagreement.
	// The runnable same-binary invocation travels in the managed_assets context
	// because the cause's privacy gate redacts absolute paths.
	persistedDigest := staleFixtureManagedAssetDigest()
	if persistedDigest == "" {
		return fmt.Errorf("stale fixture state carries no managed_asset_digest")
	}
	if !strings.HasPrefix(failure.Cause, managedAssetRemediation) ||
		!strings.Contains(failure.Cause, "expected digest sha256:") || !strings.Contains(failure.Cause, persistedDigest) {
		return fmt.Errorf("stale managed assets cause = %q, want prefix %q naming both digests", failure.Cause, managedAssetRemediation)
	}
	skew := failure.Context.ManagedAssets
	if skew == nil || skew.PersistedDigest != persistedDigest || !strings.HasPrefix(skew.ExpectedDigest, "sha256:") ||
		skew.ExpectedDigest == skew.PersistedDigest || skew.BinaryVersion == "" || skew.BinaryPath == "" ||
		skew.Remediation != "the sync must be run by this same binary: "+skew.BinaryPath+" sync" {
		return fmt.Errorf("stale managed assets context = %#v", skew)
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

	// #3848 hole 1: the recommended sync used to return its zero-agent no-op
	// before the managed-asset persistence step, so in a home with no agents
	// (exactly this sandbox) it exited 0 while leaving the stale digest, and
	// the refusal above could never converge. Run the remediation with the
	// same binary and prove the printed START now passes preflight.
	sync := r.run([]string{"sync"}, false)
	if sync.ExitCode != 0 {
		return fmt.Errorf("zero-agent sync exit %d: %s %s", sync.ExitCode, firstLine(sync.Stdout), firstLine(sync.Stderr))
	}
	stateBytes, err := os.ReadFile(filepath.Join(r.sandbox.Home, ".gentle-ai", "state.json"))
	if err != nil {
		return err
	}
	var converged struct {
		ManagedAssetDigest  string   `json:"managed_asset_digest"`
		ManagedAssetDigests []string `json:"managed_asset_digests"`
	}
	if err := json.Unmarshal(stateBytes, &converged); err != nil {
		return err
	}
	// The zero-agent sync converges additively: the scalar keeps the writing
	// binary's digest so a second binary sharing this home stays authorized,
	// and this binary's digest joins the managed_asset_digests set.
	if converged.ManagedAssetDigest != persistedDigest {
		return fmt.Errorf("zero-agent sync rewrote the scalar digest to %q, want the writer's %q preserved", converged.ManagedAssetDigest, persistedDigest)
	}
	if !slices.Contains(converged.ManagedAssetDigests, skew.ExpectedDigest) {
		return fmt.Errorf("zero-agent sync set %v does not contain this binary's %q", converged.ManagedAssetDigests, skew.ExpectedDigest)
	}
	restarted, err := runPrintedTransition(r, reconciled)
	if err != nil {
		return err
	}
	var started struct {
		LineageID string `json:"lineage_id"`
	}
	if restarted.ExitCode != 0 || json.Unmarshal([]byte(restarted.Stdout), &started) != nil || started.LineageID == "" {
		return fmt.Errorf("START after converging sync = exit %d: %s %s", restarted.ExitCode, firstLine(restarted.Stdout), firstLine(restarted.Stderr))
	}
	return nil
}

// staleFixtureManagedAssetDigest reads the digest the predecessor fixture
// recorded, so assertions compare against the artifact itself instead of a
// second hand-copied constant.
func staleFixtureManagedAssetDigest() string {
	var persisted struct {
		ManagedAssetDigest string `json:"managed_asset_digest"`
	}
	if json.Unmarshal(historicalManagedAssetState, &persisted) != nil {
		return ""
	}
	return persisted.ManagedAssetDigest
}

func managedAssetJourneys() []Journey {
	return []Journey{
		{
			ID:     "j93-stale-managed-assets-start-is-not-unknown",
			Review: reviewOptedIn,
			Title:  "Stale managed assets: v2 OpenCode START stops before authority, names both digests, and a zero-agent sync converges the preflight",
			Source: "issue #2822: a stale product-created managed-asset digest refuses before START can mutate; issue #3848: the refusal names both digests and the same-binary remediation, and a zero-agent sync persists the running binary's digest instead of no-opping past it",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: staged prose candidate", Fixture: stageDocs("stale-managed-assets")},
				{Name: "fixture: predecessor managed asset digest", Fixture: staleManagedAssetState},
				{Name: "v2 OpenCode START is typed not-started with convergence diagnostics, and the same-binary sync unblocks START", Requires: managedAssetsStartCapability, Composite: staleManagedAssetsStartIsNotUnknown},
			},
		},
	}
}
