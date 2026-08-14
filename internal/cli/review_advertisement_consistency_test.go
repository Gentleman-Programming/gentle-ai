package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/advisoryreview"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// TestAdvertisementSurfacesAgreeAtEveryCommit is task 8.1's CI cross-surface
// consistency gate (SEN-RTC-4/7/8/9/10). Every advertisement surface must
// agree at every commit of this change; the final flip leaves claude and
// opencode advertising ContractReviewTransportV1 and
// ContractImmutableReviewExecutorV1 while codex is wired but unadvertised on
// every surface:
//
//   - capabilitymanifest.ForAgent maps (both review contracts dormant for codex)
//   - the compiled reviewImmutableRuntimeCapability / reviewTransportSupportedRuntimeIDs
//   - the advisoryreview generic prompt/text seam (Runtime.Supports)
//   - docs/review-integration.md
//
// A commit that advertises a runtime through one surface while another denies
// it fails here, before any review authority exists.
func TestAdvertisementSurfacesAgreeAtEveryCommit(t *testing.T) {
	advertises := func(agent model.AgentID, contract capabilitymanifest.ContractID) bool {
		t.Helper()
		manifest, err := capabilitymanifest.ForAgent(agent)
		if err != nil {
			t.Fatalf("ForAgent(%s): %v", agent, err)
		}
		return manifest.Advertises(contract)
	}

	// Post-flip world (SEN-RTC-8): claude and opencode advertise both review
	// contracts; codex advertises neither.
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode} {
		if !advertises(agent, capabilitymanifest.ContractReviewTransportV1) {
			t.Fatalf("%s no longer advertises %s after the flip", agent, capabilitymanifest.ContractReviewTransportV1)
		}
		if !advertises(agent, capabilitymanifest.ContractImmutableReviewExecutorV1) {
			t.Fatalf("%s no longer advertises %s after the flip", agent, capabilitymanifest.ContractImmutableReviewExecutorV1)
		}
	}
	for _, contract := range []capabilitymanifest.ContractID{
		capabilitymanifest.ContractReviewTransportV1,
		capabilitymanifest.ContractImmutableReviewExecutorV1,
	} {
		if advertises(model.AgentCodex, contract) {
			t.Fatalf("codex still advertises %s after the flip", contract)
		}
	}

	// The compiled capability projection names exactly the advertised set
	// (SEN-RTC-9): claude-code, opencode.
	if got := strings.Join(reviewTransportSupportedRuntimeIDs(), ","); got != "claude-code,opencode" {
		t.Fatalf("reviewTransportSupportedRuntimeIDs() = %q, want claude-code,opencode", got)
	}

	// The advisoryreview generic prompt/text seam admits exactly the same
	// advertised runtimes (SEN-RTC-8: Supports() must not advertise codex).
	// All three runtime symbols are pinned so the check fails if the seam
	// ever drops an advertised runtime as well as if codex sneaks back in.
	advertised := map[advisoryreview.Runtime]bool{}
	for _, agent := range reviewTransportSupportedRuntimeIDs() {
		advertised[advisoryreview.Runtime(agent)] = true
	}
	for runtime, want := range map[advisoryreview.Runtime]bool{
		advisoryreview.RuntimeClaudeCode: true,
		advisoryreview.RuntimeOpenCode:   true,
		advisoryreview.RuntimeCodex:      false,
	} {
		if got := runtime.Supports(); got != want || got != advertised[runtime] {
			t.Fatalf("advisoryreview runtime %q Supports() = %t, want %t (slice-8 advertisement consistency)", runtime, got, want)
		}
		if runtime.Supports() != advertised[runtime] {
			t.Fatalf("advisoryreview runtime %q Supports() = %t disagrees with the compiled advertised set", runtime, runtime.Supports())
		}
	}

	// The docs surface agrees with the compiled surfaces: the enforced
	// fresh-reviewer boundary is Claude Code + OpenCode, and codex is wired
	// but unadvertised.
	docsSource, err := os.ReadFile(filepath.Join("..", "..", "docs", "review-integration.md"))
	if err != nil {
		t.Fatal(err)
	}
	docs := string(docsSource)
	for _, need := range []string{
		"Claude Code and OpenCode provide the enforced fresh reviewer-execution boundary",
		"wired but unadvertised",
	} {
		if !strings.Contains(docs, need) {
			t.Fatalf("docs/review-integration.md no longer states the post-flip advertisement world (missing %q)", need)
		}
	}
}
