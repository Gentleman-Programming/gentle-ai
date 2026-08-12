// Package organicruntime_test proves the journeys a configured agent actually
// performs once Gentle AI stopped owning implementation: the agent implements
// organically, and Gentle AI's authority begins only after a candidate exists.
//
// Every assertion here is driven through the real gentle-ai binary and the real
// `review` command surface against real Git repositories and a real bare remote.
// There is no runtime fixture, no TLS control plane, and no bearer session: the
// retired control plane cannot be proven, only the shipped product can.
package organicruntime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/versions"
)

const (
	// realAgentE2EEnvironment gates the pinned real-agent journeys, which need a
	// pinned OpenCode plus network access to the pinned plugin package.
	realAgentE2EEnvironment = "GENTLE_AI_REAL_AGENT_E2E"
	pinnedOpenCodeVersion   = versions.OpenCode

	organicLocalTimeout     = 90 * time.Second
	organicSetupTimeout     = 5 * time.Minute
	organicAgentTimeout     = 4 * time.Minute
	organicCommandWaitDelay = 10 * time.Second

	// organicWithdrawalDeadline replaces the retired short-TTL harness. The
	// surviving authorization has no wall-clock lifetime: it is withdrawn by an
	// explicit user action instead of expiring, so the harness that used to sleep
	// out a TTL now performs the withdrawal instantly. The deadline only keeps CI
	// honest — a journey that needs longer than this has stopped being a journey.
	organicWithdrawalDeadline = 60 * time.Second
)

// Actor process contract. The delegated worker must be a real, separate OS
// process rather than an in-test closure, because the behaviour under test is
// exactly what a sub-agent does on its own: it implements and commits, and it
// never escalates its own route. Re-executing the compiled test binary keeps
// that real without adding a language runtime dependency to the suite.
const (
	organicActorRoleEnvironment    = "GENTLE_AI_ORGANIC_ACTOR_ROLE"
	organicActorRepoEnvironment    = "GENTLE_AI_ORGANIC_ACTOR_REPO"
	organicActorPathEnvironment    = "GENTLE_AI_ORGANIC_ACTOR_PATH"
	organicActorBodyEnvironment    = "GENTLE_AI_ORGANIC_ACTOR_BODY"
	organicActorMessageEnvironment = "GENTLE_AI_ORGANIC_ACTOR_MESSAGE"
	organicActorBinaryEnvironment  = "GENTLE_AI_ORGANIC_ACTOR_BINARY"
	organicTestBinaryEnvironment   = "GENTLE_AI_ORGANIC_TEST_BINARY"

	organicActorRoleDirect    = "direct"
	organicActorRoleDelegated = "delegated"

	organicDirectActorMarker    = "ORGANIC_DIRECT_CANDIDATE_COMMITTED"
	organicDelegatedActorMarker = "ORGANIC_DELEGATED_CANDIDATE_COMMITTED"
)

// Wire vocabulary. These are literals on purpose: an end-to-end test pins the
// contract the product emits, it does not re-derive it from the packages that
// emit it.
const (
	organicRiskLow    = "low"
	organicRiskMedium = "medium"
	organicRiskHigh   = "high"

	organicStateApproved           = "approved"
	organicStateValidating         = "validating"
	organicStateCorrectionRequired = "correction_required"

	organicGateSchema = "gentle-ai.review-gate-result/v1"
	organicModeSchema = "gentle-ai.review-mode/v1"

	organicGateAllow = "allow"
	organicModeOff   = "off"
)

var organicBinary string

func TestMain(m *testing.M) {
	if role := strings.TrimSpace(os.Getenv(organicActorRoleEnvironment)); role != "" {
		os.Exit(runOrganicActor(role))
	}
	if binary := strings.TrimSpace(os.Getenv(organicTestBinaryEnvironment)); binary != "" {
		resolvedBinary, err := exec.LookPath(binary)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s=%q does not resolve to an executable: %v\n", organicTestBinaryEnvironment, binary, err)
			os.Exit(1)
		}
		organicBinary = resolvedBinary
		os.Exit(m.Run())
	}
	workspace, err := os.MkdirTemp("", "organic-e2e-binary")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create organic binary workspace: %v\n", err)
		os.Exit(1)
	}
	binary, err := buildOrganicBinary(workspace)
	if err != nil {
		_ = os.RemoveAll(workspace)
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	organicBinary = binary
	code := m.Run()
	_ = os.RemoveAll(workspace)
	os.Exit(code)
}

// runOrganicActor is the implementation actor. It edits exactly one already
// understood file and explicitly creates the candidate commit, which is the only
// thing an organic actor owes: the provider never creates or guesses a commit.
func runOrganicActor(role string) int {
	repo := os.Getenv(organicActorRepoEnvironment)
	relative := os.Getenv(organicActorPathEnvironment)
	body := os.Getenv(organicActorBodyEnvironment)
	message := os.Getenv(organicActorMessageEnvironment)
	if repo == "" || relative == "" || message == "" {
		fmt.Fprintln(os.Stderr, "organic actor requires repository, path, and message")
		return 1
	}

	marker := organicDirectActorMarker
	if role == organicActorRoleDelegated {
		marker = organicDelegatedActorMarker
		// A delegated worker observes authority read-only. It must not start a
		// review, select a route, or promote itself into SDD: escalating its own
		// route is precisely the failure this journey exists to catch.
		if err := assertOrganicDelegatedWorkerStaysInRoute(repo); err != nil {
			fmt.Fprintf(os.Stderr, "delegated actor escalated its own route: %v\n", err)
			return 1
		}
	}

	target := filepath.Join(repo, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "organic actor mkdir: %v\n", err)
		return 1
	}
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "organic actor write: %v\n", err)
		return 1
	}
	for _, arguments := range [][]string{
		{"add", "--", relative},
		{"commit", "-q", "-m", message},
	} {
		if _, err := organicGitOutput(context.Background(), repo, arguments...); err != nil {
			fmt.Fprintf(os.Stderr, "organic actor git: %v\n", err)
			return 1
		}
	}
	fmt.Print(marker)
	return 0
}

func assertOrganicDelegatedWorkerStaysInRoute(repo string) error {
	binary := os.Getenv(organicActorBinaryEnvironment)
	if binary == "" {
		return errors.New("delegated actor has no gentle-ai binary to observe authority with")
	}
	ctx, cancel := context.WithTimeout(context.Background(), organicLocalTimeout)
	defer cancel()
	command := organicCommandContext(ctx, binary, "review", "mode", "status", "--cwd", repo, "--json")
	command.Env = os.Environ()
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("read review mode: %w", err)
	}
	var mode organicModeResult
	if err := json.Unmarshal(output, &mode); err != nil {
		return fmt.Errorf("decode review mode: %w", err)
	}
	if mode.Schema != organicModeSchema || mode.Operation != "status" {
		return fmt.Errorf("delegated actor read an unexpected authority projection %#v", mode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Journeys
// ---------------------------------------------------------------------------

// TestOrganicDirectoryIdentityAcceptsCanonicalAliases keeps the repository
// selection boundary: relative, absolute, aliased, and `git -C` forms all denote
// exactly one repository, and a non-directory never does.
func TestOrganicDirectoryIdentityAcceptsCanonicalAliases(t *testing.T) {
	t.Parallel()
	harness := newOrganicHarness(t)
	worktree := harness.repo.worktree

	canonical := harness.commonDir()
	relative := harness.git("rev-parse", "--git-common-dir")
	if !filepath.IsAbs(relative) {
		relative = filepath.Join(worktree, relative)
	}
	if !sameOrganicDirectory(canonical, relative) {
		t.Fatalf("relative and absolute common-dir forms denote different repositories: %q vs %q", relative, canonical)
	}

	parent := filepath.Dir(worktree)
	viaParent, err := organicGitOutput(context.Background(), parent, "-C", worktree, "rev-parse", "--absolute-git-dir")
	if err != nil {
		t.Fatal(err)
	}
	if !sameOrganicDirectory(canonical, viaParent) {
		t.Fatalf("git -C form denotes a different repository: %q vs %q", viaParent, canonical)
	}

	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(worktree, alias); err != nil {
		t.Skipf("directory aliases are unavailable: %v", err)
	}
	viaAlias, err := organicGitOutput(context.Background(), alias, "rev-parse", "--absolute-git-dir")
	if err != nil {
		t.Fatal(err)
	}
	if !sameOrganicDirectory(canonical, viaAlias) {
		t.Fatalf("aliased worktree denotes a different repository: %q vs %q", viaAlias, canonical)
	}

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if sameOrganicDirectory(canonical, file) {
		t.Fatal("a regular file was accepted as the repository directory")
	}
}

// TestOrganicConfiguredAgentReceivesRoutingGuidance is the optional-SDD
// "proposed" leg. Every configured agent is told the same thing through its own
// delivery strategy: three routes exist, SDD is only ever proposed, and it is
// selected only by an explicit request or an accepted proposal.
// organicRoutingGuidanceRequiredFragments is the routing-guidance content
// every configured agent must receive, shared between this file's Cursor
// case and organic_runtime_real_agent_detection_test.go's Claude Code /
// OpenCode cases (see that file for why they're split).
var organicRoutingGuidanceRequiredFragments = []string{
	"Direct inline",
	"Delegated direct",
	"Optional SDD",
	"never selects SDD",
	"never create SDD artifacts",
	"gentle-ai review mode enable|disable|status",
	"disabled/unmanaged",
}

// TestOrganicConfiguredAgentReceivesRoutingGuidanceCursor proves the
// markdown-rules delivery strategy for Cursor. Cursor's Detect is
// config-dir-only (~/.cursor, no PATH lookup — see
// internal/agents/cursor/adapter.go), so this case needs no real agent
// binary and runs unconditionally in the ordinary unit sweep.
//
// Claude Code and OpenCode's equivalent cases used to live in this same
// table-driven test, but their detection follows the inherited PATH to a
// real installed binary — install refuses instead of installing a missing
// runtime now (agentInstallStep in internal/cli/run.go) — so running them
// here depended on those binaries happening to be on the machine running
// `go test ./...`, which is true on developer machines but not on every CI
// runner. They now live in
// organic_runtime_real_agent_detection_test.go, gated behind the
// real_agent_e2e build tag so they run only in the organic-runtime-e2e CI
// job, which installs both runtimes first.
func TestOrganicConfiguredAgentReceivesRoutingGuidanceCursor(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	home := t.TempDir()
	if _, err := organicGitOutput(context.Background(), workspace, "init", "--quiet", "--initial-branch=main", "."); err != nil {
		t.Fatal(err)
	}
	// Cursor's Detect looks for ~/.cursor, which this fake isolated HOME
	// never has. Simulate Cursor as already installed so gentle-ai does not
	// correctly refuse an undetected agent here — this test targets
	// routing-guidance delivery, not agent install behavior.
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	const path = ".cursor/rules/gentle-ai.mdc"
	output, stderr, err := runOrganicCommand(
		t, organicBinary, workspace, organicEnvironment(home),
		"install", "--agent", "cursor", "--scope", "workspace", "--components", "permissions",
	)
	if err != nil {
		t.Fatalf("install cursor: %v\nstdout:\n%s\nstderr:\n%s", err, output, stderr)
	}
	rendered, readErr := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(path)))
	if readErr != nil {
		t.Fatalf("configured agent cursor received no routing guidance at %s: %v", path, readErr)
	}
	for _, fragment := range organicRoutingGuidanceRequiredFragments {
		if !bytes.Contains(rendered, []byte(fragment)) {
			t.Fatalf("routing guidance for cursor omits %q:\n%s", fragment, rendered)
		}
	}
}

// TestOrganicReviewTierIsSelectedByEvidenceNotSize pins the proportional tier.
// Tier 0 runs zero AI reviewers and asks nothing; tier 1 runs exactly one
// consolidated review; tier 2 runs the focused 4R only when named evidence
// demands it. The two large mechanical rows exist to prove the inverse: volume
// never escalates a tier.
func TestOrganicReviewTierIsSelectedByEvidenceNotSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		files       map[string]string
		risk        string
		lensCount   int
		wantsPrompt bool
		minLines    int
	}{
		{
			name:  "tier 0 passive documentation",
			files: map[string]string{"docs/note.md": organicLines("documentation line", 12)},
			risk:  organicRiskLow,
		},
		{
			name:  "tier 0 large passive documentation",
			files: map[string]string{"docs/handbook.md": organicLines("handbook line", 2000)},
			risk:  organicRiskLow,
			// Two thousand authored lines of prose stay at zero reviewers: the
			// classifier reads content, never volume.
			minLines: 2000,
		},
		{
			name:        "tier 1 ordinary source",
			files:       map[string]string{"internal/feature/flag.go": "package feature\n\nfunc Enabled() bool { return true }\n"},
			risk:        organicRiskMedium,
			lensCount:   1,
			wantsPrompt: true,
		},
		{
			name:        "tier 1 large mechanical source",
			files:       organicMechanicalFiles(12, 100),
			risk:        organicRiskMedium,
			lensCount:   1,
			wantsPrompt: true,
			// 1200+ mechanical lines across 12 files must stay on one consolidated
			// review. Escalating here would be size-driven, not evidence-driven.
			minLines: 1200,
		},
		{
			name:        "tier 2 authorization hot path",
			files:       map[string]string{"internal/auth/session.go": "package auth\n\nfunc Session() bool { return true }\n"},
			risk:        organicRiskHigh,
			lensCount:   4,
			wantsPrompt: true,
		},
		{
			name:        "tier 2 shell process source",
			files:       map[string]string{"scripts/deploy.sh": "#!/bin/sh\nset -eu\necho deploy\n"},
			risk:        organicRiskHigh,
			lensCount:   4,
			wantsPrompt: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			harness := newOrganicHarness(t)
			harness.writeFiles(test.files)

			started, stderr := harness.startReview("organic-tier")
			if started.RiskLevel != test.risk || len(started.SelectedLenses) != test.lensCount {
				t.Fatalf("tier = %q with %d lenses, want %q with %d", started.RiskLevel, len(started.SelectedLenses), test.risk, test.lensCount)
			}
			if started.LensesRequired != (test.lensCount > 0) {
				t.Fatalf("lenses_required = %t for %d selected lenses", started.LensesRequired, test.lensCount)
			}
			if test.minLines > 0 && started.ChangedLines < test.minLines {
				t.Fatalf("changed lines = %d, want at least %d for the volume claim to mean anything", started.ChangedLines, test.minLines)
			}
			// Tier 0 is silent structural readback. Emitting a consent prompt here
			// would reintroduce exactly the ceremony the readback exists to remove.
			if prompted := strings.TrimSpace(stderr) != ""; prompted != test.wantsPrompt {
				t.Fatalf("consent prompt emitted = %t, want %t; stderr:\n%s", prompted, test.wantsPrompt, stderr)
			}

			approved := harness.approveReview("organic-tier", started)
			if approved.State != organicStateApproved || approved.ReceiptPath == "" {
				t.Fatalf("tier %q did not reach one terminal receipt: %#v", test.risk, approved)
			}
			harness.assertNoSDDArtifacts()
		})
	}
}

// TestOrganicImplementationRoutesReachDelivery walks the two organic
// implementation routes end to end: a real actor process produces the candidate,
// the proportional review approves it, the delivery gate authorizes the push,
// and the bare remote moves exactly once under compare-and-swap.
func TestOrganicImplementationRoutesReachDelivery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		role   string
		marker string
		path   string
		body   string
	}{
		{
			name:   "direct inline",
			role:   organicActorRoleDirect,
			marker: organicDirectActorMarker,
			path:   "docs/direct-note.md",
			body:   organicLines("direct implementation line", 10),
		},
		{
			name:   "delegated direct",
			role:   organicActorRoleDelegated,
			marker: organicDelegatedActorMarker,
			path:   "docs/delegated-note.md",
			body:   organicLines("delegated implementation line", 10),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			harness := newOrganicHarness(t)
			harness.runActor(test.role, test.path, test.body, "docs: add an organic note", test.marker)

			candidate := harness.git("rev-parse", "HEAD")
			if candidate == harness.repo.baseRevision {
				t.Fatal("the actor never created a candidate commit")
			}

			lineage := "organic-" + strings.ReplaceAll(test.name, " ", "-")
			started, _ := harness.startReview(lineage, "--base-ref", "origin/main")
			approved := harness.approveReview(lineage, started)
			if approved.State != organicStateApproved {
				t.Fatalf("%s route did not approve its candidate: %#v", test.name, approved)
			}

			gate := harness.gate("pre-push")
			if !gate.Allowed || gate.Result != organicGateAllow {
				t.Fatalf("pre-push gate refused an approved candidate: %#v", gate)
			}

			harness.pushWithLease(harness.repo.baseRevision)
			harness.assertRemoteBlob(test.path, test.body)
			harness.assertOnlyMainRef()
			harness.assertStaleLeaseIsRejected(harness.repo.baseRevision)

			// The route is what this journey selects; SDD is what it must never
			// select. A delegated worker in particular must not promote itself.
			harness.assertNoSDDArtifacts()
			harness.assertSingleReviewLineage(lineage)
		})
	}
}

// TestOrganicOptionalSDDDeclineAndAccept covers both answers to the one optional
// route question. Declining leaves the repository free of SDD state; accepting
// creates the SDD runtime and binds it to the same approved organic receipt.
func TestOrganicOptionalSDDDeclineAndAccept(t *testing.T) {
	t.Parallel()

	t.Run("declined", func(t *testing.T) {
		t.Parallel()
		harness := newOrganicHarness(t)
		harness.runActor(organicActorRoleDirect, "docs/declined.md", organicLines("declined line", 8), "docs: implement directly", organicDirectActorMarker)

		started, _ := harness.startReview("organic-sdd-declined", "--base-ref", "origin/main")
		if approved := harness.approveReview("organic-sdd-declined", started); approved.State != organicStateApproved {
			t.Fatalf("declined route did not approve: %#v", approved)
		}
		// This is the proposal's core claim, so it stays verbatim: direct and
		// delegated work never create SDD artifacts, prompts, phase attempts, or
		// synthetic SDD runs.
		harness.assertNoSDDArtifacts()
		if _, err := os.Stat(filepath.Join(harness.repo.worktree, "openspec")); !os.IsNotExist(err) {
			t.Fatalf("declined route created OpenSpec artifacts: %v", err)
		}
	})

	t.Run("accepted", func(t *testing.T) {
		t.Parallel()
		harness := newOrganicHarness(t)
		const change = "organic-accepted-change"
		harness.writeFiles(map[string]string{
			filepath.ToSlash(filepath.Join("openspec", "changes", change, "proposal.md")): "# Proposal\n\nAccepted optional SDD.\n",
			"docs/accepted.md": organicLines("accepted line", 8),
		})

		started, _ := harness.startReview("organic-sdd-accepted")
		approved := harness.approveReview("organic-sdd-accepted", started)
		if approved.State != organicStateApproved {
			t.Fatalf("accepted route did not approve: %#v", approved)
		}

		payload := harness.gentle("review", "bind-sdd",
			"--cwd", harness.repo.worktree,
			"--change", change,
			"--lineage", "organic-sdd-accepted",
			"--expected-binding-revision", "",
		)
		var binding struct {
			Schema      string `json:"schema"`
			Change      string `json:"change"`
			Lineage     string `json:"lineage"`
			ReceiptHash string `json:"receipt_hash"`
			GateContext struct {
				Gate string `json:"gate"`
			} `json:"gate_context"`
		}
		if err := json.Unmarshal(payload, &binding); err != nil {
			t.Fatalf("decode bind-sdd result: %v\n%s", err, payload)
		}
		if binding.Change != change || binding.Lineage != "organic-sdd-accepted" || binding.ReceiptHash == "" {
			t.Fatalf("accepted SDD binding = %#v", binding)
		}
		// The accepted answer is the only one that may create SDD state, so this
		// is the exact inverse of the declined assertion above.
		if !harness.hasSDDArtifacts() {
			t.Fatal("accepted optional SDD created no SDD runtime state")
		}
	})
}

// TestOrganicBoundedCorrectionAllowsExactlyOne proves the ordinary review budget:
// one candidate-caused blocker buys one scoped correction, and the transaction
// refuses a second one instead of looping until clean.
func TestOrganicBoundedCorrectionAllowsExactlyOne(t *testing.T) {
	t.Parallel()
	harness := newOrganicHarness(t)
	const lineage = "organic-correction"
	const path = "internal/feature/limit.go"

	harness.writeFiles(map[string]string{path: organicLimitSource("broken")})
	started, _ := harness.startReview(lineage)
	if started.RiskLevel != organicRiskMedium || len(started.SelectedLenses) != 1 || started.CorrectionBudget <= 0 {
		t.Fatalf("correction journey needs one consolidated review with a budget: %#v", started)
	}

	harness.captureReviewerResultOrFail(lineage, started, 0, organicReviewerResult{
		Lens: started.SelectedLenses[0],
		Findings: []organicFinding{{
			Location:          path + ":5",
			Severity:          "CRITICAL",
			Claim:             "the candidate returns the wrong terminal value",
			ProofRefs:         []string{"a differential test passes on base and fails on the candidate"},
			EvidenceClass:     "deterministic",
			CausalDisposition: "introduced",
		}},
		Evidence: []string{"the focused differential test failed on the candidate"},
	})
	required := harness.finalize(lineage, "--captured-results=true")
	if required.State != organicStateCorrectionRequired {
		t.Fatalf("candidate-caused blocker did not require a correction: %#v", required)
	}

	forecast := harness.finalize(lineage, "--correction-lines", "2")
	if forecast.State != organicStateCorrectionRequired {
		t.Fatalf("in-budget forecast escalated: %#v", forecast)
	}

	harness.writeFiles(map[string]string{path: organicLimitSource("fixed")})
	waiting := harnessCorrectionStatus(t, harness, lineage)
	if waiting.NextTransition == nil || waiting.NextTransition.Kind != "collect" ||
		waiting.NextTransition.ReasonCode != "correction_repository_verification_required" ||
		waiting.NextTransition.Collect == nil || len(waiting.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("corrected candidate did not request repository verification evidence: %#v", waiting)
	}
	input := waiting.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != "review.capture-evidence" {
		t.Fatalf("correction evidence capture operation = %q", input.CaptureOperation)
	}
	var correctionTarget string
	for _, argument := range input.Arguments {
		if argument.Name == "target" {
			correctionTarget = argument.Value
		}
	}
	if correctionTarget == "" {
		t.Fatalf("correction evidence transition omitted its candidate target: %#v", input)
	}
	harness.gentle(
		"review", "capture-evidence", "--cwd", harness.repo.worktree, "--lineage", lineage,
		"--target", correctionTarget, "--expected-revision", waiting.Authority.Revision,
		"--outcome", "passed", "--input", harness.writeEvidence(),
	)
	ready := harnessCorrectionStatus(t, harness, lineage)
	if ready.ValidationRequest == nil || ready.ValidationRequest.CorrectionTargetIdentity != correctionTarget ||
		ready.NextTransition == nil || ready.NextTransition.Kind != "collect" || ready.NextTransition.ReasonCode != "targeted_validation_required" {
		t.Fatalf("passed correction evidence did not expose the bound targeted-validation request: %#v", ready)
	}
	validation := harness.writeJSON("validation.json", organicValidationResult{
		TargetedValidationRequestHash: ready.ValidationRequest.RequestHash,
		CorrectionTargetIdentity:      ready.ValidationRequest.CorrectionTargetIdentity,
		OriginalCriteria:              organicValidationCheck{Passed: true, Evidence: []string{"the original acceptance test passed"}},
		CorrectionRegression:          organicValidationCheck{Passed: true, Evidence: []string{"the targeted regression test passed"}},
		FollowUps:                     []any{},
	})
	approved := harness.finalize(lineage, "--validation", validation, "--captured-evidence")
	if approved.State != organicStateApproved || approved.ReceiptPath == "" {
		t.Fatalf("atomic correction acceptance did not produce a terminal receipt: %#v", approved)
	}

	before := harness.lineageDigest(lineage)
	_, stderr, err := harness.gentleAllowFailure("review", "finalize", "--cwd", harness.repo.worktree, "--lineage", lineage, "--captured-results=true")
	if err == nil {
		t.Fatal("a second correction was accepted after the bounded one was consumed")
	}
	if !strings.Contains(stderr, "terminal review finalize accepts no review inputs") {
		t.Fatalf("second correction was refused without a discoverable reason: %s", stderr)
	}
	if after := harness.lineageDigest(lineage); after != before {
		t.Fatal("the refused second correction still mutated review authority")
	}
}

// TestOrganicFlexibleDeliveryReusesOneReceipt proves the receipt is content
// bound, not route bound: one immutable receipt authorizes direct commit, direct
// push, and a pull request with or without an issue, and none of those routes
// reopens review.
func TestOrganicFlexibleDeliveryReusesOneReceipt(t *testing.T) {
	t.Parallel()
	harness := newOrganicHarness(t)
	const lineage = "organic-delivery"
	const path = "docs/delivery-note.md"
	body := organicLines("delivery line", 10)

	harness.writeFiles(map[string]string{path: body})
	harness.git("add", "--", path)

	started, _ := harness.startReview(lineage, "--projection", "staged")
	if approved := harness.approveReview(lineage, started); approved.State != organicStateApproved {
		t.Fatalf("staged candidate did not approve: %#v", approved)
	}

	// Route 1: direct commit.
	commitGate := harness.gate("pre-commit")
	if !commitGate.Allowed {
		t.Fatalf("pre-commit refused the approved candidate: %#v", commitGate)
	}
	harness.git("commit", "-q", "-m", "docs: add a delivery note")

	// Route 2: direct push, under the same receipt and after the commit.
	pushGate := harness.gate("pre-push")
	if !pushGate.Allowed {
		t.Fatalf("pre-push refused the approved candidate: %#v", pushGate)
	}

	// Routes 3 and 4: a pull request with and without an issue reference. The two
	// branches carry the same tree under different commits, which is the point:
	// Gentle AI binds content, so neither the delivery route nor the commit
	// identity reopens review, and the issue reference is repository policy that
	// the receipt neither requires nor records. Both run before publication,
	// because the pull-request boundary is the unpublished remote base.
	prGate := harness.gate("pre-pr", "--base-ref", "origin/main")
	if !prGate.Allowed {
		t.Fatalf("pre-pr without an issue refused the approved candidate: %#v", prGate)
	}
	harness.git("checkout", "-q", "-b", "organic-pr-with-issue")
	harness.git("commit", "-q", "--amend", "--allow-empty", "-m", "docs: add a delivery note\n\nRefs: #17")
	issueGate := harness.gate("pre-pr", "--base-ref", "origin/main")
	if !issueGate.Allowed {
		t.Fatalf("pre-pr with an issue refused the approved candidate: %#v", issueGate)
	}

	harness.git("checkout", "-q", "main")
	harness.pushWithLease(harness.repo.baseRevision)
	harness.assertRemoteBlob(path, body)

	digests := map[string]string{
		"pre-commit":              commitGate.Context.BundleDigest,
		"pre-push":                pushGate.Context.BundleDigest,
		"pre-pr without an issue": prGate.Context.BundleDigest,
		"pre-pr with an issue":    issueGate.Context.BundleDigest,
	}
	for gate, digest := range digests {
		if digest == "" || digest != commitGate.Context.BundleDigest {
			t.Fatalf("%s validated a different receipt (%q) than the one that was approved (%q)", gate, digest, commitGate.Context.BundleDigest)
		}
	}
	// Four delivery routes, one lineage: changing the route, or rewriting the
	// commit over an unchanged tree, never reopened review.
	harness.assertSingleReviewLineage(lineage)
	harness.assertNoSDDArtifacts()
	harness.assertOnlyMainRef()
}

// TestOrganicKillSwitchStopsAtTheDeliveryBoundary proves safe disablement. The
// candidate still exists locally, nothing reaches the remote, no authority
// generation is written, and the refusal is typed and discoverable rather than a
// silent no-op or a fabricated approval.
func TestOrganicKillSwitchStopsAtTheDeliveryBoundary(t *testing.T) {
	t.Parallel()
	harness := newOrganicHarness(t)
	harness.runActor(organicActorRoleDirect, "docs/killed.md", organicLines("killed line", 8), "docs: implement before the switch", organicDirectActorMarker)

	mode := harness.disableReview()
	if mode.Schema != organicModeSchema || mode.Status.Effective != organicModeOff || mode.Status.Source != "clone_local" {
		t.Fatalf("kill switch produced no typed outcome: %#v", mode)
	}
	generationsAfterDisable := harness.reviewModeGenerations()

	// The candidate is committed locally. That is deliberate: disabling review
	// must never destroy the user's work.
	if harness.git("rev-parse", "HEAD") == harness.repo.baseRevision {
		t.Fatal("the kill-switch journey never reached a committed candidate")
	}

	// The universal empty-candidate guard (issue #2586) refuses a clean tree
	// before the kill switch can name itself, so the disabled attempt carries
	// a real pending candidate: the refusal under test here is the kill
	// switch's, and it must name its deciding source.
	harness.writeFiles(map[string]string{"docs/disabled-attempt.md": organicLines("pending while disabled", 3)})
	harness.git("add", "--", "docs/disabled-attempt.md")
	_, stderr, err := harness.gentleAllowFailure("review", "start", "--cwd", harness.repo.worktree, "--lineage", "organic-killed")
	if err == nil {
		t.Fatal("review start succeeded while receipt-driven development was disabled")
	}
	if !strings.Contains(stderr, "receipt-driven development is disabled") || !strings.Contains(stderr, "clone_local") {
		t.Fatalf("disabled start was refused without naming the deciding source: %s", stderr)
	}
	harness.git("rm", "-f", "-q", "--", "docs/disabled-attempt.md")

	// The delivery boundary reports an unmanaged, receiptless candidate instead of
	// inventing an approval.
	gate := harness.gateAllowFailure("pre-push")
	if gate.Schema != organicGateSchema || gate.Allowed || gate.Result == organicGateAllow {
		t.Fatalf("disabled delivery gate did not fail closed: %#v", gate)
	}
	// Wave 5 Slice 2 (design decision 4): the kill switch is consulted
	// before any authority read, so this report carries no discovery-kind
	// detail at all -- there is no receipt-discovery outcome to describe,
	// because discovery never runs.
	if gate.Context.Denial != nil {
		t.Fatalf("disabled delivery gate leaked discovery-kind detail: %#v", gate.Context.Denial)
	}
	// The guidance installed on all 16 adapters promises this exact token under a
	// disabled switch. Asserting it here is what keeps that promise honest, and
	// distinguishes "unmanaged by choice" from "blocked because something broke".
	if gate.Delivery != "disabled/unmanaged" {
		t.Fatalf("disabled delivery gate did not report the promised disposition: %q", gate.Delivery)
	}

	// Zero effects: no review authority, no additional compare-and-swap
	// generation, and a remote that never moved.
	if _, err := os.Stat(filepath.Join(harness.commonDir(), "gentle-ai", "review-transactions", "v2")); !os.IsNotExist(err) {
		t.Fatalf("a disabled start still created review authority: %v", err)
	}
	if after := harness.reviewModeGenerations(); !equalOrganicStrings(after, generationsAfterDisable) {
		t.Fatalf("a disabled start advanced review-mode CAS generations: %v -> %v", generationsAfterDisable, after)
	}
	remote := harness.bareGit("rev-parse", "refs/heads/main")
	if remote != harness.repo.baseRevision {
		t.Fatalf("the remote moved while review was disabled: %s != %s", remote, harness.repo.baseRevision)
	}
	harness.assertOnlyMainRef()
	harness.assertNoSDDArtifacts()
}

// TestOrganicKillSwitchReportsUnmanagedDeliveryOverPriorReceipts proves the
// disabled disposition survives review history (community report, PR #1801).
// The virgin-clone journey above holds without any receipts; this one holds in
// a repository that already completed reviewed flows: a stale receipt that no
// longer governs the candidate is the expected state of a disabled clone — no
// new receipt could have been created while disabled — so the gate still
// reports `disabled/unmanaged` instead of failing closed on a receipt mismatch.
func TestOrganicKillSwitchReportsUnmanagedDeliveryOverPriorReceipts(t *testing.T) {
	t.Parallel()
	harness := newOrganicHarness(t)
	const lineage = "organic-killed-prior"
	const path = "docs/prior-note.md"

	// A completed reviewed flow: the candidate is committed and reviewed
	// against its base, so a terminal receipt exists before the switch flips.
	harness.writeFiles(map[string]string{path: organicLines("prior line", 8)})
	harness.git("add", "--", path)
	harness.git("commit", "-q", "-m", "docs: reviewed before the switch")
	started, _ := harness.startReview(lineage, "--base-ref", harness.repo.baseRevision, "--committed-only")
	if approved := harness.approveReview(lineage, started); approved.State != organicStateApproved {
		t.Fatalf("the prior reviewed flow did not approve its candidate: %#v", approved)
	}

	if mode := harness.disableReview(); mode.Status.Effective != organicModeOff {
		t.Fatalf("kill switch produced no typed outcome: %#v", mode)
	}

	// The community-reported shape: a new commit authored while disabled, in a
	// repository that still holds the earlier receipt.
	harness.writeFiles(map[string]string{"docs/disabled-note.md": organicLines("disabled line", 6)})
	harness.git("add", "--", "docs/disabled-note.md")
	harness.git("commit", "-q", "-m", "docs: authored while disabled")

	// harness.gate fails the test on a non-zero exit, so this also proves the
	// gate reports instead of vetoing.
	gate := harness.gate("pre-push")
	if gate.Schema != organicGateSchema || gate.Allowed || gate.Result == organicGateAllow {
		t.Fatalf("disabled delivery over a prior receipt fabricated an approval: %#v", gate)
	}
	if gate.Delivery != "disabled/unmanaged" {
		t.Fatalf("disabled delivery over a prior receipt did not report the promised disposition: %q", gate.Delivery)
	}
	// Wave 5 Slice 2 (design decision 4): the switch is consulted before any
	// authority read, so the prior receipt is never even discovered while
	// disabled -- no discovery-kind detail leaks.
	if gate.Context.Denial != nil {
		t.Fatalf("disabled delivery over a prior receipt leaked discovery-kind detail: %#v", gate.Context.Denial)
	}

	// Reporting moved nothing: the remote is untouched and no branch appeared.
	if remote := harness.bareGit("rev-parse", "refs/heads/main"); remote != harness.repo.baseRevision {
		t.Fatalf("the remote moved while review was disabled: %s != %s", remote, harness.repo.baseRevision)
	}
	harness.assertOnlyMainRef()
	harness.assertNoSDDArtifacts()
}

// TestOrganicKillSwitchReportsUnmanagedDeliveryOverWorkspaceReceipt proves the
// second community-reported shape (Wladimirfn, PR #1801): a workspace
// (current-changes) receipt delivered exactly as reviewed in one commit, then a
// new commit authored while disabled, then pre-push. The candidate now
// publishes two commits past the reviewed base, so the receipt's one-commit
// delivery rule cannot hold — a deterministic mismatch between candidate shape
// and a provably healthy receipt, never corruption — and the disabled gate
// still reports `disabled/unmanaged` with a successful exit.
func TestOrganicKillSwitchReportsUnmanagedDeliveryOverWorkspaceReceipt(t *testing.T) {
	t.Parallel()
	harness := newOrganicHarness(t)
	const lineage = "organic-killed-workspace"
	const path = "docs/workspace-note.md"

	// A completed workspace reviewed flow over the dirty candidate...
	harness.writeFiles(map[string]string{path: organicLines("workspace line", 8)})
	started, _ := harness.startReview(lineage)
	if approved := harness.approveReview(lineage, started); approved.State != organicStateApproved {
		t.Fatalf("the workspace reviewed flow did not approve its candidate: %#v", approved)
	}
	// ...delivered exactly as reviewed, in one commit that was never pushed.
	harness.git("add", "--", path)
	harness.git("commit", "-q", "-m", "docs: reviewed workspace delivery")

	if mode := harness.disableReview(); mode.Status.Effective != organicModeOff {
		t.Fatalf("kill switch produced no typed outcome: %#v", mode)
	}

	// The community-reported shape: a second commit authored while disabled.
	harness.writeFiles(map[string]string{"docs/disabled-note.md": organicLines("disabled line", 6)})
	harness.git("add", "--", "docs/disabled-note.md")
	harness.git("commit", "-q", "-m", "docs: authored while disabled")

	// harness.gate fails the test on a non-zero exit, so this also proves the
	// gate reports instead of vetoing with a fabricated corruption verdict.
	gate := harness.gate("pre-push")
	if gate.Schema != organicGateSchema || gate.Allowed || gate.Result == organicGateAllow {
		t.Fatalf("disabled delivery over a workspace receipt fabricated an approval: %#v", gate)
	}
	if gate.Delivery != "disabled/unmanaged" {
		t.Fatalf("disabled delivery over a workspace receipt did not report the promised disposition: %q", gate.Delivery)
	}
	// Wave 5 Slice 2 (design decision 4): the switch is consulted before any
	// authority read, so the healthy receipt is never even discovered while
	// disabled -- no discovery-kind detail (not even "delivery-shape-mismatch")
	// leaks.
	if gate.Context.Denial != nil {
		t.Fatalf("disabled delivery over a workspace receipt leaked discovery-kind detail: %#v", gate.Context.Denial)
	}

	// Reporting moved nothing: the remote is untouched and no branch appeared.
	if remote := harness.bareGit("rev-parse", "refs/heads/main"); remote != harness.repo.baseRevision {
		t.Fatalf("the remote moved while review was disabled: %s != %s", remote, harness.repo.baseRevision)
	}
	harness.assertOnlyMainRef()
	harness.assertNoSDDArtifacts()
}

// TestOrganicKillSwitchReEnableLandsOnTheFreshFullReview drives issue #1877's
// sequence end to end through the real binary: reviewed baseline, kill switch
// off, work delivered under `disabled/unmanaged` (recorded as unmanaged, never
// blocked), switch back on — and from there the journey runs ONLY what the
// product's own messages name, until the archive stop clears through one fresh
// full review of the current state. The maintainer's rule is that this fresh
// review subsumes the unmanaged history: no retroactive reconciliation, no
// blessing of past unmanaged deliveries, no durable per-delivery ledger — and
// no zero-byte review may ever read as coverage.
func TestOrganicKillSwitchReEnableLandsOnTheFreshFullReview(t *testing.T) {
	t.Parallel()
	harness := newOrganicHarness(t)
	const change = "reenable-change"
	harness.seedOrganicSDDChange(change)

	// Baseline: reviews on, one unit reviewed and delivered normally.
	harness.writeFiles(map[string]string{"docs/baseline.md": organicLines("baseline line", 6)})
	harness.git("add", "--", "docs/baseline.md")
	baselineStarted, _ := harness.startReview("organic-reenable-baseline")
	if approved := harness.approveReview("organic-reenable-baseline", baselineStarted); approved.State != organicStateApproved {
		t.Fatalf("the baseline reviewed flow did not approve its candidate: %#v", approved)
	}
	if gate := harness.gate("pre-commit"); gate.Result != organicGateAllow {
		t.Fatalf("baseline delivery gate = %#v, want allow", gate)
	}
	harness.git("commit", "-q", "-m", "docs: baseline reviewed delivery")
	baselineCommit := harness.git("rev-parse", "HEAD")

	if mode := harness.disableReview(); mode.Status.Effective != organicModeOff {
		t.Fatalf("kill switch produced no typed outcome: %#v", mode)
	}

	// Two units delivered under the disabled policy. The gate reports at exit
	// 0 with the pinned disposition — that behavior is load-bearing.
	for _, unit := range []string{"one", "two"} {
		harness.writeFiles(map[string]string{"docs/unmanaged-" + unit + ".md": organicLines("unmanaged "+unit, 6)})
		harness.git("add", "--", "docs/unmanaged-"+unit+".md")
		gate := harness.gate("pre-commit")
		if gate.Allowed || gate.Result == organicGateAllow || gate.Delivery != "disabled/unmanaged" {
			t.Fatalf("disabled delivery gate for unit %s = %#v, want the unmanaged disposition", unit, gate)
		}
		harness.git("commit", "-q", "-m", "docs: unmanaged delivery "+unit)
	}

	// The disabled window proceeds unmanaged even though the stale baseline
	// receipt is still in history: corrective verify cycle CRITICAL-1 makes
	// this structural absence (sdd-status's own reviewGate, distinct from the
	// pre-commit delivery gate checked above, which correctly keeps its own
	// "disabled/unmanaged" disposition) -- not a populated disposition.
	// Declining to manage is not a blocker demanding a review the switch
	// refuses to run.
	disabled := harness.sddStatus(change)
	if disabled.Dependencies.Archive == "blocked" {
		t.Fatalf("disabled archive over a stale receipt = blocked; reasons = %v", disabled.BlockedReasons)
	}
	if disabled.ReviewGate != nil {
		t.Fatalf("disabled window produced a review gate instead of structural absence: %#v", disabled.ReviewGate)
	}

	if mode := harness.enableReview(); mode.Status.Effective != "on" {
		t.Fatalf("re-enable produced no typed outcome: %#v", mode)
	}

	// The archive stop is back and names the fresh full review runnably.
	blocked := harness.sddStatus(change)
	if blocked.Dependencies.Archive != "blocked" || blocked.ReviewGate == nil {
		t.Fatalf("re-enabled archive over unmanaged history = %#v, want blocked", blocked)
	}
	if blocked.ReviewGate.Result == organicGateAllow {
		t.Fatalf("re-enabling silently passed over unmanaged history: %#v", blocked.ReviewGate)
	}
	tokens := organicNamedContinuation(t, blocked.ReviewGate.Reason)

	// Run exactly what it names. The tree is clean, so the universal
	// empty-candidate guard (issue #2586) refuses before any authority is
	// created, and its refusal names the --base-ref rerun. The zero-byte
	// receipt this journey used to fabricate here can no longer exist on any
	// route: the not-coverage defense moved from the archive stop to the
	// start itself.
	if len(tokens) < 2 || tokens[0] != "review" || tokens[1] != "start" {
		t.Fatalf("named continuation is %v, want gentle-ai review start", tokens)
	}
	_, emptyStderr, emptyErr := harness.gentleAllowFailure(tokens...)
	if emptyErr == nil {
		t.Fatal("a clean-tree start froze an empty candidate instead of refusing")
	}
	if !strings.Contains(emptyStderr, "no pending changes") || !strings.Contains(emptyStderr, "--base-ref") {
		t.Fatalf("empty-candidate refusal does not name the committed-work rerun: %q", emptyStderr)
	}

	// The refused start recorded nothing: the stop stays blocked and keeps
	// naming the fresh review with its base-ref selector.
	stillBlocked := harness.sddStatus(change)
	if stillBlocked.Dependencies.Archive != "blocked" || stillBlocked.ReviewGate == nil ||
		stillBlocked.ReviewGate.Result == organicGateAllow {
		t.Fatalf("a refused empty start unblocked the archive stop: %#v", stillBlocked)
	}
	// The stop keeps naming the fresh full review; the --base-ref rerun for
	// committed work now travels in the refusal's own hint (asserted above),
	// because the zero-byte receipt that used to teach the stop that detail
	// can no longer exist. The operator supplies the one placeholder value —
	// the boundary to re-govern from — and the fresh full review freezes the
	// delivered range.
	tokens = organicNamedContinuation(t, stillBlocked.ReviewGate.Reason)
	freshStart := harness.runNamedReviewStart(tokens, "--base-ref", baselineCommit)
	if freshStart.ChangedFiles == 0 {
		t.Fatalf("the fresh full review froze no content: %#v", freshStart)
	}
	if approved := harness.approveReview(freshStart.LineageID, freshStart); approved.State != organicStateApproved {
		t.Fatalf("the fresh full review did not approve: %#v", approved)
	}

	// Completing the named review clears the stop: the fresh receipt governs
	// the current state and the stale terminal receipts remain mere history.
	cleared := harness.sddStatus(change)
	if cleared.ReviewGate == nil || cleared.ReviewGate.Result != organicGateAllow {
		t.Fatalf("completing the named fresh review did not clear the stop: %#v", cleared)
	}
	if cleared.Dependencies.Archive == "blocked" || cleared.NextRecommended != "archive" {
		t.Fatalf("archive routing after the fresh full review = %#v, want ready/archive", cleared)
	}

	// The next unit after re-enable is ordinary managed work again: the
	// lifecycle gate denies, names the fresh review, and running what it names
	// clears the gate.
	harness.writeFiles(map[string]string{"docs/next.md": organicLines("next line", 6)})
	harness.git("add", "--", "docs/next.md")
	_, stderr, err := harness.gentleAllowFailure("review", "validate", "--cwd", harness.repo.worktree, "--gate", "pre-commit")
	if err == nil {
		t.Fatal("the gate allowed unreviewed new work after re-enable")
	}
	tokens = organicNamedContinuation(t, stderr)
	nextStart := harness.runNamedReviewStart(tokens)
	if nextStart.ChangedFiles == 0 {
		t.Fatalf("the named review start froze no candidate: %#v", nextStart)
	}
	if approved := harness.approveReview(nextStart.LineageID, nextStart); approved.State != organicStateApproved {
		t.Fatalf("the next unit's review did not approve: %#v", approved)
	}
	if gate := harness.gate("pre-commit"); gate.Result != organicGateAllow {
		t.Fatalf("gate after the named review = %#v, want allow", gate)
	}
	harness.git("commit", "-q", "-m", "docs: managed unit after re-enable")
}

// TestOrganicTerminalAuthoritySurvivesWithdrawalAndReplaysWithoutEffect keeps the
// expiry-stable terminal state. The authorization that permitted the review is
// withdrawn afterwards, and the terminal receipt still validates, replays
// byte-identically, and produces no additional effect.
// TestOrganicTerminalAuthoritySurvivesWithdrawalAndReplaysWithoutEffect is
// Wave 5 Slice 2's most consequential black-box behavior reversal (design
// decision 4; rdd-receipt-only-gates/spec.md's "Kill switch off
// short-circuits before authority discovery" scenario, a firm requirement,
// not a tagged pending assumption): the kill switch is now consulted before
// ANY receipt or authority read, so even the terminal receipt this journey
// just earned is never consulted while disabled -- the gate reports the
// generic disabled/unmanaged shape instead of replaying the same allow. The
// name and behavior this test asserts changed accordingly: the terminal
// AUTHORITY survives withdrawal unmutated (proven by re-enabling and
// replaying below), but it no longer GOVERNS DELIVERY while withdrawn.
func TestOrganicTerminalAuthoritySurvivesWithdrawalAndReplaysWithoutEffect(t *testing.T) {
	t.Parallel()
	deadline := time.Now().Add(organicWithdrawalDeadline)
	harness := newOrganicHarness(t)
	const lineage = "organic-withdrawal"
	const path = "docs/withdrawn.md"
	body := organicLines("withdrawal line", 10)

	harness.writeFiles(map[string]string{path: body})
	started, _ := harness.startReview(lineage)
	if approved := harness.approveReview(lineage, started); approved.State != organicStateApproved {
		t.Fatalf("withdrawal journey did not approve its candidate: %#v", approved)
	}

	firstGate := harness.gentle("review", "validate", "--cwd", harness.repo.worktree, "--gate", "post-apply")
	firstFinalize := harness.gentle("review", "finalize", "--cwd", harness.repo.worktree, "--lineage", lineage)
	beforeWithdrawal := harness.lineageDigest(lineage)

	// Withdraw the authorization. Unlike the retired wall-clock lease this is an
	// explicit event, so the harness withdraws instead of sleeping.
	if mode := harness.disableReview(); mode.Status.Effective != organicModeOff {
		t.Fatalf("authorization withdrawal did not take effect: %#v", mode)
	}
	generationsAfterWithdrawal := harness.reviewModeGenerations()

	// The withdrawal must be real, otherwise everything below is vacuous.
	if _, _, err := harness.gentleAllowFailure("review", "start", "--cwd", harness.repo.worktree, "--lineage", "organic-withdrawal-successor"); err == nil {
		t.Fatal("a new review started after the authorization was withdrawn")
	}

	// While withdrawn: the terminal receipt just earned is never consulted --
	// the gate reports the generic disabled/unmanaged shape, not a replay of
	// the pre-withdrawal allow.
	withdrawnGate := harness.gentle("review", "validate", "--cwd", harness.repo.worktree, "--gate", "post-apply")
	if bytes.Equal(withdrawnGate, firstGate) {
		t.Fatal("the terminal gate result did not change after withdrawal -- the receipt was consulted while disabled")
	}
	if !bytes.Contains(withdrawnGate, []byte(`"disabled/unmanaged"`)) {
		t.Fatalf("withdrawn gate did not report disabled/unmanaged: %s", withdrawnGate)
	}
	if bytes.Contains(withdrawnGate, []byte(`"allowed":true`)) {
		t.Fatalf("withdrawn gate fabricated an approval: %s", withdrawnGate)
	}
	replayedFinalize := harness.gentle("review", "finalize", "--cwd", harness.repo.worktree, "--lineage", lineage)
	if !bytes.Equal(replayedFinalize, firstFinalize) {
		t.Fatalf("the terminal finalize replay changed bytes:\nfirst:\n%s\nreplay:\n%s", firstFinalize, replayedFinalize)
	}

	if after := harness.lineageDigest(lineage); after != beforeWithdrawal {
		t.Fatal("replaying a terminal review mutated its authority")
	}
	if after := harness.reviewModeGenerations(); !equalOrganicStrings(after, generationsAfterWithdrawal) {
		t.Fatalf("replay advanced review-mode CAS generations: %v -> %v", generationsAfterWithdrawal, after)
	}
	if remote := harness.bareGit("rev-parse", "refs/heads/main"); remote != harness.repo.baseRevision {
		t.Fatalf("replay moved the remote: %s != %s", remote, harness.repo.baseRevision)
	}
	harness.assertOnlyMainRef()

	// Re-enabling rediscovers the SAME unmutated receipt, and it governs
	// again exactly as before withdrawal -- proving the switch never touched
	// the authority itself, only whether it is consulted.
	if mode := harness.enableReview(); mode.Status.Effective != "on" {
		t.Fatalf("re-enabling did not take effect: %#v", mode)
	}
	reEnabledGate := harness.gentle("review", "validate", "--cwd", harness.repo.worktree, "--gate", "post-apply")
	if !bytes.Equal(reEnabledGate, firstGate) {
		t.Fatalf("the terminal gate result changed across a withdraw/re-enable cycle:\nbefore:\n%s\nafter:\n%s", firstGate, reEnabledGate)
	}

	if time.Now().After(deadline) {
		t.Fatalf("the withdrawal journey exceeded its %s CI budget", organicWithdrawalDeadline)
	}
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

type organicRepository struct {
	worktree     string
	bare         string
	baseRevision string
}

type organicHarness struct {
	t    *testing.T
	repo organicRepository
	home string
}

func newOrganicHarness(t *testing.T) *organicHarness {
	t.Helper()
	harness := &organicHarness{t: t, repo: initOrganicRepository(t), home: t.TempDir()}
	return harness
}

// environment isolates the run from the developer's own global review mode. A
// suite that reads the real user state would pass or fail for reasons that have
// nothing to do with the product.
func (harness *organicHarness) environment() []string {
	return organicEnvironment(harness.home)
}

func organicEnvironment(home string) []string {
	environment := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"PATH=" + os.Getenv("PATH"),
		"LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		// CI makes the one-time consent question deterministically unanswerable,
		// which is exactly the non-interactive path this suite asserts on.
		"CI=1",
	}
	if value := os.Getenv("SYSTEMROOT"); value != "" {
		environment = append(environment, "SYSTEMROOT="+value)
	}
	if value := os.Getenv("TMPDIR"); value != "" {
		environment = append(environment, "TMPDIR="+value)
	}
	for _, name := range []string{"OPENCODE_DISABLE_PROJECT_CONFIG", "OPENCODE_DISABLE_EXTERNAL_SKILLS"} {
		if value := os.Getenv(name); value != "" {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

func (harness *organicHarness) gentle(arguments ...string) []byte {
	harness.t.Helper()
	stdout, stderr, err := runOrganicCommand(harness.t, organicBinary, harness.repo.worktree, harness.environment(), arguments...)
	if err != nil {
		harness.t.Fatalf("gentle-ai %v: %v\nstdout:\n%s\nstderr:\n%s", arguments, err, stdout, stderr)
	}
	return []byte(stdout)
}

func (harness *organicHarness) gentleAllowFailure(arguments ...string) (string, string, error) {
	harness.t.Helper()
	return runOrganicCommand(harness.t, organicBinary, harness.repo.worktree, harness.environment(), arguments...)
}

func runOrganicCommand(t *testing.T, binary, dir string, environment []string, arguments ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), organicLocalTimeout)
	defer cancel()
	command := organicCommandContext(ctx, binary, arguments...)
	command.Dir = dir
	command.Env = environment
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

// startReview freezes the live candidate and returns both the typed result and
// the console stream, because whether a question was asked is itself an
// assertion in the tier-0 journey.
func (harness *organicHarness) startReview(lineage string, extra ...string) (organicStartResult, string) {
	harness.t.Helper()
	arguments := []string{"review", "start", "--cwd", harness.repo.worktree}
	if lineage != "" {
		arguments = append(arguments, "--lineage", lineage)
	}
	arguments = append(arguments, extra...)
	stdout, stderr, err := harness.gentleAllowFailure(arguments...)
	if err != nil {
		harness.t.Fatalf("review start %v: %v\nstdout:\n%s\nstderr:\n%s", arguments, err, stdout, stderr)
	}
	var started organicStartResult
	if err := json.Unmarshal([]byte(stdout), &started); err != nil {
		harness.t.Fatalf("decode review start: %v\n%s", err, stdout)
	}
	return started, stderr
}

// approveReview runs the proportional plan the tier selected: zero reviewers for
// passive content, and one result per selected lens plus final evidence
// otherwise. The suite never selects lenses itself.
func (harness *organicHarness) approveReview(lineage string, started organicStartResult) organicFinalizeResult {
	harness.t.Helper()
	if len(started.SelectedLenses) == 0 {
		return harness.finalize(lineage)
	}
	for index, lens := range started.SelectedLenses {
		harness.captureReviewerResult(lineage, started, index, organicReviewerResult{
			Lens:     lens,
			Findings: []organicFinding{},
			Evidence: []string{"inspected every frozen candidate path for " + lens},
		})
	}
	if result := harness.finalize(lineage, "--captured-results=true"); result.State != organicStateValidating {
		harness.t.Fatalf("reviewer results did not reach validation: %#v", result)
	}
	return harness.finalize(lineage, "--evidence", harness.writeEvidence())
}

// captureReviewerResult admits one reviewer result through the native route.
//
// finalize takes no unadmitted reviewer results, so the suite asks the binary
// for the lens's frozen binding, echoes the provider-issued subject hash and the
// inspected path manifest back into the caller's payload, and captures it. Only
// the binding is supplied here; the findings and evidence stay exactly as the
// caller wrote them, so a test that means to submit a rejectable result still
// submits one.
func (harness *organicHarness) captureReviewerResult(lineage string, started organicStartResult, order int, result organicReviewerResult) (string, string, error) {
	harness.t.Helper()
	lens := started.SelectedLenses[order]
	binding := []string{
		"review", "capture-result", "--cwd", harness.repo.worktree, "--lineage", lineage,
		"--target", started.targetIdentity(), "--lens", lens, "--order", strconv.Itoa(order),
	}
	var preflight organicCapturePreflight
	if err := json.Unmarshal(harness.gentle(append(binding, "--preflight")...), &preflight); err != nil {
		harness.t.Fatalf("decode capture-result preflight for %s: %v", lens, err)
	}
	paths := make([]string, len(preflight.ChangedPathManifest))
	for index, entry := range preflight.ChangedPathManifest {
		paths[index] = entry.Path
	}
	result.SubjectHash = preflight.ArtifactSubject.SubjectHash
	result.Inspection = &organicInspection{Status: "completed", Paths: paths}
	input := harness.writeJSON(fmt.Sprintf("reviewer-%d.json", order), result)
	return harness.gentleAllowFailure(append(binding, "--input", input)...)
}

// captureReviewerResultOrFail is captureReviewerResult for the callers that
// require admission to succeed.
func (harness *organicHarness) captureReviewerResultOrFail(lineage string, started organicStartResult, order int, result organicReviewerResult) {
	harness.t.Helper()
	if _, stderr, err := harness.captureReviewerResult(lineage, started, order, result); err != nil {
		harness.t.Fatalf("capture reviewer result for %s: %v\n%s", started.SelectedLenses[order], err, stderr)
	}
}

type organicCapturePreflight struct {
	ArtifactSubject struct {
		SubjectHash string `json:"subject_hash"`
	} `json:"artifact_subject"`
	ChangedPathManifest []struct {
		Path string `json:"path"`
	} `json:"changed_path_manifest"`
}

type organicInspection struct {
	Status string   `json:"status"`
	Paths  []string `json:"paths"`
}

func (harness *organicHarness) finalize(lineage string, extra ...string) organicFinalizeResult {
	harness.t.Helper()
	arguments := []string{"review", "finalize", "--cwd", harness.repo.worktree}
	if lineage != "" {
		arguments = append(arguments, "--lineage", lineage)
	}
	arguments = append(arguments, extra...)
	var result organicFinalizeResult
	payload := harness.gentle(arguments...)
	if err := json.Unmarshal(payload, &result); err != nil {
		harness.t.Fatalf("decode review finalize: %v\n%s", err, payload)
	}
	return result
}

func (harness *organicHarness) gate(gate string, extra ...string) organicGateResult {
	harness.t.Helper()
	arguments := append([]string{"review", "validate", "--cwd", harness.repo.worktree, "--gate", gate}, extra...)
	var result organicGateResult
	payload := harness.gentle(arguments...)
	if err := json.Unmarshal(payload, &result); err != nil {
		harness.t.Fatalf("decode review validate: %v\n%s", err, payload)
	}
	return result
}

// gateAllowFailure decodes a denied gate. A denial exits non-zero on purpose, so
// the typed projection still has to be readable.
func (harness *organicHarness) gateAllowFailure(gate string, extra ...string) organicGateResult {
	harness.t.Helper()
	arguments := append([]string{"review", "validate", "--cwd", harness.repo.worktree, "--gate", gate}, extra...)
	stdout, _, _ := harness.gentleAllowFailure(arguments...)
	var result organicGateResult
	decoder := json.NewDecoder(strings.NewReader(stdout))
	if err := decoder.Decode(&result); err != nil {
		harness.t.Fatalf("decode denied review validate: %v\n%s", err, stdout)
	}
	return result
}

func (harness *organicHarness) disableReview() organicModeResult {
	harness.t.Helper()
	payload := harness.gentle("review", "mode", "disable", "--cwd", harness.repo.worktree, "--scope", "clone", "--json")
	var mode organicModeResult
	if err := json.Unmarshal(payload, &mode); err != nil {
		harness.t.Fatalf("decode review mode: %v\n%s", err, payload)
	}
	return mode
}

// enableReview flips the clone-local kill switch back on: the other half of
// the disable journeys, and the point where issue #1877's re-enable sequence
// begins.
func (harness *organicHarness) enableReview() organicModeResult {
	harness.t.Helper()
	payload := harness.gentle("review", "mode", "enable", "--cwd", harness.repo.worktree, "--scope", "clone", "--json")
	var mode organicModeResult
	if err := json.Unmarshal(payload, &mode); err != nil {
		harness.t.Fatalf("decode review mode: %v\n%s", err, payload)
	}
	return mode
}

// organicSDDStatus is the slice of `sdd-status --json` the archive journeys
// consume: the archive dependency, the review gate record, and the routing.
type organicSDDStatus struct {
	Dependencies struct {
		Archive string `json:"archive"`
	} `json:"dependencies"`
	ReviewGate *struct {
		Result   string `json:"result"`
		Reason   string `json:"reason"`
		Delivery string `json:"delivery"`
	} `json:"reviewGate"`
	NextRecommended string   `json:"nextRecommended"`
	BlockedReasons  []string `json:"blockedReasons"`
}

func (harness *organicHarness) sddStatus(change string) organicSDDStatus {
	harness.t.Helper()
	payload := harness.gentle("sdd-status", change, "--cwd", harness.repo.worktree, "--json")
	var status organicSDDStatus
	if err := json.Unmarshal(payload, &status); err != nil {
		harness.t.Fatalf("decode sdd-status: %v\n%s", err, payload)
	}
	return status
}

// organicNamedContinuation returns the argument tokens of the first
// `gentle-ai ...` command a product message names, read exactly as an operator
// would: to the end of the line, stopping at the first `<placeholder>` whose
// value the operator supplies.
func organicNamedContinuation(t *testing.T, message string) []string {
	t.Helper()
	const product = "gentle-ai "
	index := strings.Index(message, product)
	if index < 0 {
		t.Fatalf("message names no runnable gentle-ai command: %q", message)
	}
	tail := message[index+len(product):]
	if cut := strings.IndexAny(tail, "\n"); cut >= 0 {
		tail = tail[:cut]
	}
	tokens := []string{}
	for _, token := range strings.Fields(tail) {
		token = strings.Trim(token, ",.;:'\"`)]")
		if token == "" || strings.HasPrefix(token, "<") {
			break
		}
		tokens = append(tokens, token)
	}
	if len(tokens) == 0 {
		t.Fatalf("message names no runnable gentle-ai command: %q", message)
	}
	return tokens
}

// runNamedReviewStart dispatches a `gentle-ai review start ...` continuation
// read out of a product message, with the working directory already at the
// repository so the invocation runs exactly as printed. extra carries only an
// operator-supplied placeholder value the message asked for.
func (harness *organicHarness) runNamedReviewStart(tokens []string, extra ...string) organicStartResult {
	harness.t.Helper()
	if len(tokens) < 2 || tokens[0] != "review" || tokens[1] != "start" {
		harness.t.Fatalf("named continuation is %v, want gentle-ai review start", tokens)
	}
	payload := harness.gentle(append(append([]string{}, tokens...), extra...)...)
	var started organicStartResult
	if err := json.Unmarshal(payload, &started); err != nil {
		harness.t.Fatalf("decode named review start: %v\n%s", err, payload)
	}
	return started
}

// organicSDDVerifyReport is the fenced envelope a completed independent
// verification writes. Its exact shape matters: a report the product cannot
// parse routes as "verification is missing", which is a different journey.
const organicSDDVerifyReport = "```yaml\n" +
	"schema: gentle-ai.verify-result/v1\n" +
	"evidence_revision: sha256:1111111111111111111111111111111111111111111111111111111111111111\n" +
	"verdict: pass\n" +
	"blockers: 0\n" +
	"critical_findings: 0\n" +
	"requirements: 1/1\n" +
	"scenarios: 1/1\n" +
	"test_command: go test ./internal/example\n" +
	"test_exit_code: 0\n" +
	"test_output_hash: sha256:2222222222222222222222222222222222222222222222222222222222222222\n" +
	"build_command: go test ./cmd/gentle-ai\n" +
	"build_exit_code: 0\n" +
	"build_output_hash: sha256:3333333333333333333333333333333333333333333333333333333333333333\n" +
	"```\n"

// seedOrganicSDDChange commits a complete OpenSpec change at its archive
// decision: planning done, every task checked, and a parseable passing
// verification report — so `sdd-status` routes on the review gate alone.
func (harness *organicHarness) seedOrganicSDDChange(change string) {
	harness.t.Helper()
	root := "openspec/changes/" + change + "/"
	harness.writeFiles(map[string]string{
		root + "proposal.md": "# " + change + "\n\n## Why\n\nthe journey drives a delivery cycle.\n",
		root + "design.md":   "# design\n\n## Approach\n\nplain prose, no executable content.\n",
		root + "tasks.md":    "# tasks\n\n- [x] 1.1 write the prose\n",
		root + "specs/prose/spec.md": "### Requirement: prose exists\n" +
			"#### Scenario: prose is present\n\n- **WHEN** the reader opens the docs\n- **THEN** the prose is there\n",
		root + "verify-report.md": organicSDDVerifyReport,
	})
	harness.git("add", "--", "openspec")
	harness.git("commit", "-q", "-m", "test: seed the SDD change")
}

// reviewModeGenerations lists the clone-local kill-switch compare-and-swap
// records. Their count is how a rejected operation proves it wrote nothing.
func (harness *organicHarness) reviewModeGenerations() []string {
	harness.t.Helper()
	root := filepath.Join(harness.commonDir(), "gentle-ai", "review-mode", "rar-authority", "v1", "rdd-mode")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		harness.t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "gen-") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

// lineageDigest fingerprints every authority file of one lineage so a replay can
// prove it changed nothing at all, not merely that it reported the same state.
func (harness *organicHarness) lineageDigest(lineage string) string {
	harness.t.Helper()
	root := filepath.Join(harness.commonDir(), "gentle-ai", "review-transactions", "v2", lineage)
	var builder strings.Builder
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		payload, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		fmt.Fprintf(&builder, "%s\x00%x\n", filepath.ToSlash(relative), payload)
		return nil
	})
	if err != nil {
		harness.t.Fatalf("digest review lineage %q: %v", lineage, err)
	}
	return builder.String()
}

func (harness *organicHarness) assertSingleReviewLineage(expected string) {
	harness.t.Helper()
	root := filepath.Join(harness.commonDir(), "gentle-ai", "review-transactions", "v2")
	entries, err := os.ReadDir(root)
	if err != nil {
		harness.t.Fatalf("read review authority: %v", err)
	}
	lineages := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			lineages = append(lineages, entry.Name())
		}
	}
	if len(lineages) != 1 || lineages[0] != expected {
		harness.t.Fatalf("review lineages = %v, want exactly [%s]", lineages, expected)
	}
}

// assertNoSDDArtifacts is the proposal's core claim and survives verbatim:
// direct and delegated work never create SDD, trace, or evaluation state.
func (harness *organicHarness) assertNoSDDArtifacts() {
	harness.t.Helper()
	if name, found := harness.sddArtifact(); found {
		harness.t.Fatalf("organic implementation created forbidden SDD/trace/evaluation artifact %q", name)
	}
}

func (harness *organicHarness) hasSDDArtifacts() bool {
	harness.t.Helper()
	_, found := harness.sddArtifact()
	return found
}

func (harness *organicHarness) sddArtifact() (string, bool) {
	harness.t.Helper()
	root := filepath.Join(harness.commonDir(), "gentle-ai")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		harness.t.Fatal(err)
	}
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if strings.HasPrefix(name, "sdd-") || name == "sdd" || name == "trace" || name == "evaluation" {
			return filepath.Join(root, entry.Name()), true
		}
	}
	return "", false
}

// commonDir resolves the repository the way the product does, so an aliased or
// relative invocation cannot silently point the assertions at another clone.
func (harness *organicHarness) commonDir() string {
	harness.t.Helper()
	common := harness.git("rev-parse", "--git-common-dir")
	if !filepath.IsAbs(common) {
		common = filepath.Join(harness.repo.worktree, common)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(common))
	if err != nil {
		harness.t.Fatal(err)
	}
	return resolved
}

func (harness *organicHarness) runActor(role, path, body, message, marker string) {
	harness.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), organicLocalTimeout)
	defer cancel()
	command := organicCommandContext(ctx, os.Args[0])
	command.Dir = harness.repo.worktree
	command.Env = append(harness.environment(),
		organicActorRoleEnvironment+"="+role,
		organicActorRepoEnvironment+"="+harness.repo.worktree,
		organicActorPathEnvironment+"="+path,
		organicActorBodyEnvironment+"="+body,
		organicActorMessageEnvironment+"="+message,
		organicActorBinaryEnvironment+"="+organicBinary,
	)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		harness.t.Fatalf("%s actor: %v\nstdout:\n%s\nstderr:\n%s", role, err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), marker) {
		harness.t.Fatalf("%s actor did not report %q: %s", role, marker, stdout.String())
	}
}

// writeFiles writes candidate files and declares them. Since #2394 a new file
// only enters the review candidate once the user put it in the index, so a
// journey that means to have its files reviewed has to say so the same way a
// real user does.
func (harness *organicHarness) writeFiles(files map[string]string) {
	harness.t.Helper()
	for relative, body := range files {
		target := filepath.Join(harness.repo.worktree, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			harness.t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
			harness.t.Fatal(err)
		}
		harness.git("add", "--", relative)
	}
}

func (harness *organicHarness) writeJSON(name string, value any) string {
	harness.t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		harness.t.Fatal(err)
	}
	// Review inputs live outside the repository so they never become part of the
	// candidate they describe.
	path := filepath.Join(harness.t.TempDir(), name)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		harness.t.Fatal(err)
	}
	return path
}

func (harness *organicHarness) writeRawReviewerResult(name string, payload []byte) string {
	harness.t.Helper()
	path := filepath.Join(harness.t.TempDir(), name)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		harness.t.Fatal(err)
	}
	return path
}

func (harness *organicHarness) writeEvidence() string {
	harness.t.Helper()
	path := filepath.Join(harness.t.TempDir(), "evidence.txt")
	if err := os.WriteFile(path, []byte("focused and full verification: pass\n"), 0o600); err != nil {
		harness.t.Fatal(err)
	}
	return path
}

func (harness *organicHarness) git(arguments ...string) string {
	harness.t.Helper()
	output, err := organicGitOutput(context.Background(), harness.repo.worktree, arguments...)
	if err != nil {
		harness.t.Fatal(err)
	}
	return output
}

func (harness *organicHarness) bareGit(arguments ...string) string {
	harness.t.Helper()
	output, err := organicBareGitOutput(context.Background(), harness.repo.bare, arguments...)
	if err != nil {
		harness.t.Fatal(err)
	}
	return output
}

// pushWithLease publishes under compare-and-swap against the exact revision the
// candidate was reviewed on top of.
func (harness *organicHarness) pushWithLease(expected string) {
	harness.t.Helper()
	harness.git("push", "--quiet", "--force-with-lease=refs/heads/main:"+expected, "origin", "HEAD:refs/heads/main")
	local := harness.git("rev-parse", "HEAD")
	if remote := harness.bareGit("rev-parse", "refs/heads/main"); remote != local {
		harness.t.Fatalf("remote ref = %s, want the delivered candidate %s", remote, local)
	}
}

// assertStaleLeaseIsRejected proves the publication really is a compare-and-swap.
// It needs something to publish, because an up-to-date push would succeed
// without ever consulting the lease and would prove nothing.
func (harness *organicHarness) assertStaleLeaseIsRejected(stale string) {
	harness.t.Helper()
	before := harness.bareGit("rev-parse", "refs/heads/main")
	harness.writeFiles(map[string]string{"docs/lease-probe.md": "lease probe\n"})
	harness.git("add", "--", "docs/lease-probe.md")
	harness.git("commit", "-q", "-m", "test: probe the publication lease")
	if _, err := organicGitOutput(
		context.Background(), harness.repo.worktree,
		"push", "--quiet", "--force-with-lease=refs/heads/main:"+stale, "origin", "HEAD:refs/heads/main",
	); err == nil {
		harness.t.Fatal("a stale compare-and-swap lease was accepted")
	}
	if after := harness.bareGit("rev-parse", "refs/heads/main"); after != before {
		harness.t.Fatalf("a rejected compare-and-swap still moved the remote: %s -> %s", before, after)
	}
	harness.git("reset", "--quiet", "--hard", before)
}

// assertRemoteBlob proves delivery reached the bare repository as exact content,
// not merely as a moved ref.
func (harness *organicHarness) assertRemoteBlob(path, body string) {
	harness.t.Helper()
	entry := harness.bareGit("ls-tree", "refs/heads/main", "--", path)
	if entry == "" {
		harness.t.Fatalf("delivered path %q is absent from the remote tree", path)
	}
	fields := strings.Fields(entry)
	if len(fields) < 3 {
		harness.t.Fatalf("unreadable remote tree entry %q", entry)
	}
	if fields[0] != "100644" {
		harness.t.Fatalf("delivered mode = %q, want 100644", fields[0])
	}
	blob := harness.bareGit("cat-file", "blob", fields[2])
	if blob != strings.TrimRight(body, "\n") {
		harness.t.Fatalf("delivered blob content differs:\nwant:\n%s\ngot:\n%s", body, blob)
	}
	tree := harness.bareGit("rev-parse", "refs/heads/main^{tree}")
	if localTree := harness.git("rev-parse", "HEAD^{tree}"); tree != localTree {
		harness.t.Fatalf("delivered tree = %s, want the reviewed tree %s", tree, localTree)
	}
}

func (harness *organicHarness) assertOnlyMainRef() {
	harness.t.Helper()
	refs := harness.bareGit("for-each-ref", "--format=%(refname)")
	if refs != "refs/heads/main" {
		harness.t.Fatalf("bare repository refs = %q, want only refs/heads/main", refs)
	}
}

// ---------------------------------------------------------------------------
// Wire projections
// ---------------------------------------------------------------------------

type organicStartResult struct {
	Operation        string   `json:"operation"`
	Action           string   `json:"action"`
	LensesRequired   bool     `json:"lenses_required"`
	LineageID        string   `json:"lineage_id"`
	State            string   `json:"state"`
	RiskLevel        string   `json:"risk_level"`
	SelectedLenses   []string `json:"selected_lenses"`
	ChangedFiles     int      `json:"changed_files"`
	ChangedLines     int      `json:"changed_lines"`
	CorrectionBudget int      `json:"correction_budget"`
	TargetIdentity   string   `json:"target_identity"`

	Repository *organicStartRepositoryContext `json:"repository_context,omitempty"`
	// Hint is the informational recovery pointer an empty-candidate start
	// carries; the re-enable journey follows it verbatim.
	Hint string `json:"hint"`
}

type organicStartRepositoryContext struct {
	TargetIdentity string `json:"target_identity"`
}

func (result organicStartResult) targetIdentity() string {
	if result.TargetIdentity != "" {
		return result.TargetIdentity
	}
	if result.Repository != nil {
		return result.Repository.TargetIdentity
	}
	return ""
}

type organicFinalizeResult struct {
	Operation     string `json:"operation"`
	LineageID     string `json:"lineage_id"`
	State         string `json:"state"`
	Action        string `json:"action"`
	StoreRevision string `json:"store_revision"`
	ReceiptPath   string `json:"receipt_path"`
}

type organicGateResult struct {
	Schema  string             `json:"schema"`
	Result  string             `json:"result"`
	Allowed bool               `json:"allowed"`
	Action  string             `json:"action"`
	Reason  string             `json:"reason"`
	Context organicGateContext `json:"context"`
	// Delivery carries the disposition the shipped agent guidance promises. The
	// guidance tells all 16 adapters to expect this token under a disabled
	// switch, so the wire has to actually produce it.
	Delivery string `json:"delivery"`
}

type organicGateContext struct {
	Gate          string             `json:"gate"`
	LineageID     string             `json:"lineage_id"`
	StoreRevision string             `json:"store_revision"`
	BundleDigest  string             `json:"bundle_digest"`
	BaseTree      string             `json:"base_tree"`
	CandidateTree string             `json:"candidate_tree"`
	Denial        *organicGateDenial `json:"denial"`
}

type organicGateDenial struct {
	Stage string `json:"stage"`
	Code  string `json:"code"`
}

type organicModeResult struct {
	Schema    string `json:"schema"`
	Operation string `json:"operation"`
	Scope     string `json:"scope"`
	Status    struct {
		Schema     string `json:"schema"`
		Global     string `json:"global"`
		CloneLocal string `json:"clone_local"`
		Effective  string `json:"effective"`
		Source     string `json:"source"`
		Revision   string `json:"revision"`
	} `json:"status"`
}

type organicReviewerResult struct {
	SubjectHash string             `json:"subject_hash,omitempty"`
	Inspection  *organicInspection `json:"inspection,omitempty"`
	Lens        string             `json:"lens"`
	Findings    []organicFinding   `json:"findings"`
	Evidence    []string           `json:"evidence"`
}

type organicFinding struct {
	ID                string   `json:"id,omitempty"`
	Location          string   `json:"location"`
	Severity          string   `json:"severity"`
	Claim             string   `json:"claim"`
	ProofRefs         []string `json:"proof_refs"`
	EvidenceClass     string   `json:"evidence_class"`
	CausalDisposition string   `json:"causal_disposition"`
}

type organicValidationCheck struct {
	Passed   bool     `json:"passed"`
	Evidence []string `json:"evidence"`
}

type organicValidationResult struct {
	TargetedValidationRequestHash string                 `json:"targeted_validation_request_hash,omitempty"`
	CorrectionTargetIdentity      string                 `json:"correction_target_identity,omitempty"`
	OriginalCriteria              organicValidationCheck `json:"original_criteria"`
	CorrectionRegression          organicValidationCheck `json:"correction_regression"`
	FollowUps                     []any                  `json:"follow_ups"`
}

type organicCorrectionStatus struct {
	Authority struct {
		Revision string `json:"revision"`
	} `json:"authority"`
	NextTransition *struct {
		Kind       string `json:"kind"`
		ReasonCode string `json:"reason_code"`
		Collect    *struct {
			Inputs []struct {
				CaptureOperation string `json:"capture_operation"`
				Arguments        []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"arguments"`
			} `json:"inputs"`
		} `json:"collect"`
	} `json:"next_transition"`
	ValidationRequest *struct {
		RequestHash              string `json:"request_hash"`
		CorrectionTargetIdentity string `json:"correction_target_identity"`
	} `json:"validation_request"`
}

func harnessCorrectionStatus(t *testing.T, harness *organicHarness, lineage string) organicCorrectionStatus {
	t.Helper()
	payload := harness.gentle(
		"review", "status", "--cwd", harness.repo.worktree, "--lineage", lineage,
		"--contract", "gentle-ai.review-integration/v1", "--next-transition",
	)
	var status organicCorrectionStatus
	if err := json.Unmarshal(payload, &status); err != nil {
		t.Fatalf("decode correction status: %v\n%s", err, payload)
	}
	return status
}

// ---------------------------------------------------------------------------
// Repository fixtures and shared utilities
// ---------------------------------------------------------------------------

func initOrganicRepository(t *testing.T) organicRepository {
	t.Helper()
	repo := t.TempDir()
	for _, arguments := range [][]string{
		{"init", "--quiet", "--initial-branch=main", "."},
		{"config", "user.name", "Organic E2E"},
		{"config", "user.email", "organic-e2e@example.invalid"},
		{"config", "commit.gpgsign", "false"},
	} {
		if _, err := organicGitOutput(context.Background(), repo, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("organic runtime\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := organicGitOutput(context.Background(), repo, "add", "--", "tracked.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := organicGitOutput(context.Background(), repo, "commit", "-q", "-m", "test: seed the organic repository"); err != nil {
		t.Fatal(err)
	}
	baseRevision, err := organicGitOutput(context.Background(), repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	bare := filepath.Join(t.TempDir(), "origin.git")
	if _, err := organicGitOutput(context.Background(), repo, "init", "--bare", "--quiet", bare); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"remote", "add", "origin", bare},
		{"push", "--quiet", "--set-upstream", "origin", "main:refs/heads/main"},
	} {
		if _, err := organicGitOutput(context.Background(), repo, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if err := requireOrganicOnlyMainRef(context.Background(), bare); err != nil {
		t.Fatal(err)
	}
	return organicRepository{worktree: repo, bare: bare, baseRevision: baseRevision}
}

func organicGitOutput(parent context.Context, repo string, arguments ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, organicLocalTimeout)
	defer cancel()
	command := organicCommandContext(ctx, "git", append([]string{"-C", repo}, arguments...)...)
	command.Env = append(os.Environ(), "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git -C %q %v: %w\n%s", repo, arguments, err, output)
	}
	return strings.TrimSpace(string(output)), nil
}

func organicBareGitOutput(parent context.Context, bare string, arguments ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, organicLocalTimeout)
	defer cancel()
	command := organicCommandContext(ctx, "git", append([]string{"--git-dir=" + bare}, arguments...)...)
	command.Env = append(os.Environ(), "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git --git-dir=%q %v: %w\n%s", bare, arguments, err, output)
	}
	return strings.TrimSpace(string(output)), nil
}

func requireOrganicOnlyMainRef(parent context.Context, bare string) error {
	refs, err := organicBareGitOutput(parent, bare, "for-each-ref", "--format=%(refname)")
	if err != nil {
		return err
	}
	if refs != "refs/heads/main" {
		return fmt.Errorf("bare repository refs = %q, want only refs/heads/main", refs)
	}
	return nil
}

func sameOrganicDirectory(left, right string) bool {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && leftInfo.IsDir() && rightInfo.IsDir() && os.SameFile(leftInfo, rightInfo)
}

func organicCommandContext(ctx context.Context, name string, arguments ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, arguments...)
	command.WaitDelay = organicCommandWaitDelay
	return command
}

func organicModuleRoot() (string, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("resolve the organic test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..")), nil
}

// buildOrganicBinary compiles the product once for the whole package. Every
// journey drives that one binary, so a per-journey build would only buy slower
// feedback for the same proof.
func buildOrganicBinary(workspace string) (string, error) {
	moduleRoot, err := organicModuleRoot()
	if err != nil {
		return "", err
	}
	name := "gentle-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(workspace, name)
	ctx, cancel := context.WithTimeout(context.Background(), organicSetupTimeout)
	defer cancel()
	command := organicCommandContext(ctx, "go", "build", "-trimpath", "-o", path, "./cmd/gentle-ai")
	command.Dir = moduleRoot
	command.Env = os.Environ()
	if output, err := command.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build the gentle-ai test binary: %w\n%s", err, output)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("built gentle-ai binary %q is unusable: %v", path, err)
	}
	return path, nil
}

func organicLines(prefix string, count int) string {
	var builder strings.Builder
	for index := 1; index <= count; index++ {
		fmt.Fprintf(&builder, "%s %03d\n", prefix, index)
	}
	return builder.String()
}

func organicMechanicalFiles(files, linesPerFile int) map[string]string {
	rendered := make(map[string]string, files)
	for index := 1; index <= files; index++ {
		var builder strings.Builder
		builder.WriteString("package mechanical\n\n")
		for line := 1; line <= linesPerFile; line++ {
			fmt.Fprintf(&builder, "// mechanical line %03d\n", line)
		}
		rendered[fmt.Sprintf("internal/mechanical/unit%02d.go", index)] = builder.String()
	}
	return rendered
}

// organicLimitSource renders the same unit twice with exactly one differing
// line, so the bounded correction stays inside the frozen budget and the budget
// itself is what the assertions are about.
func organicLimitSource(state string) string {
	var builder strings.Builder
	builder.WriteString("package feature\n\n")
	for index := 1; index <= 12; index++ {
		fmt.Fprintf(&builder, "// Limit documents the bounded terminal value, note %02d.\n", index)
	}
	builder.WriteString("func Limit() int {\n")
	fmt.Fprintf(&builder, "\treturn %s\n", map[string]string{"broken": "-1", "fixed": "1"}[state])
	builder.WriteString("}\n")
	return builder.String()
}

func equalOrganicStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Pinned real-agent journeys
// ---------------------------------------------------------------------------

// TestRealAgentOrganicJourneys runs the same organic journeys through a real
// configured agent. The agent runtime, its sub-agent mechanism, its tool calls,
// the gentle-ai binary, and the repository are all real; only the model is a
// fixture, because a scripted model is what makes an agent journey repeatable.
func TestRealAgentOrganicJourneys(t *testing.T) {
	if os.Getenv(realAgentE2EEnvironment) != "1" {
		t.Skip("set GENTLE_AI_REAL_AGENT_E2E=1 to run the pinned real-agent journeys")
	}
	requireOrganicExecutableVersion(t, "opencode", pinnedOpenCodeVersion)
	sharedConfig := prepareOpenCodeConfig(t)
	sharedCache := t.TempDir()

	tests := []struct {
		name         string
		outcome      string
		role         string
		marker       string
		path         string
		delegated    bool
		actorPrompt  string
		wantSubagent bool
	}{
		{
			name:    "direct inline implementation",
			outcome: "Apply one already-understood mechanical documentation change and deliver it.",
			role:    organicActorRoleDirect,
			marker:  organicDirectActorMarker,
			path:    "docs/real-direct.md",
		},
		{
			name:      "delegated direct implementation",
			outcome:   "Understand the documentation set, implement the bounded outcome, and deliver it.",
			role:      organicActorRoleDelegated,
			marker:    organicDelegatedActorMarker,
			path:      "docs/real-delegated.md",
			delegated: true,
			actorPrompt: "Act as the delegated-direct implementation worker. Implement the exact " +
				"admitted documentation scope, explicitly commit it, and return exactly " +
				organicDelegatedActorMarker + ". Never propose or create SDD state.",
			wantSubagent: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newOrganicHarness(t)
			body := organicLines("real agent line", 10)
			lineage := "organic-real-" + test.role

			script := []openCodeTurn{
				{tool: "bash", arguments: map[string]any{"command": organicActorToolCommand(t)}},
				{tool: "bash", arguments: map[string]any{"command": organicReviewToolCommand(t,
					"review", "start", "--cwd", harness.repo.worktree, "--base-ref", "origin/main", "--lineage", lineage,
				)}},
				{tool: "bash", arguments: map[string]any{"command": organicReviewToolCommand(t,
					"review", "finalize", "--cwd", harness.repo.worktree, "--lineage", lineage,
				)}},
				{tool: "bash", arguments: map[string]any{"command": organicReviewToolCommand(t,
					"review", "validate", "--cwd", harness.repo.worktree, "--gate", "pre-push",
				)}},
			}
			if test.delegated {
				// The implementation step becomes a real sub-agent, and only the
				// sub-agent may commit the candidate.
				script[0] = openCodeTurn{tool: "task", arguments: map[string]any{
					"description":   "Run the delegated organic actor",
					"prompt":        test.actorPrompt,
					"subagent_type": "general",
				}}
			}

			model := newOpenCodeFixtureServer(t, script, test.actorPrompt)
			defer model.Close()

			home := t.TempDir()
			environment := append(harness.environment(),
				"XDG_CONFIG_HOME="+sharedConfig,
				"XDG_CACHE_HOME="+sharedCache,
				"OPENCODE_CONFIG_DIR="+filepath.Join(sharedConfig, "opencode"),
				"OPENCODE_TEST_HOME="+filepath.Join(home, "opencode"),
				"OPENCODE_CONFIG_CONTENT="+organicOpenCodeConfig(t, model.URL),
				"OPENCODE_AUTH_CONTENT={}",
				"OPENCODE_DISABLE_PROJECT_CONFIG=1",
				"OPENCODE_DISABLE_AUTOUPDATE=1",
				"OPENCODE_DISABLE_AUTOCOMPACT=1",
				"OPENCODE_DISABLE_CLAUDE_CODE=1",
				"OPENCODE_DISABLE_DEFAULT_PLUGINS=1",
				"OPENCODE_DISABLE_EXTERNAL_SKILLS=1",
				"OPENCODE_DISABLE_LSP_DOWNLOAD=1",
				"OPENCODE_DISABLE_MODELS_FETCH=1",
				"OPENCODE_EXPERIMENTAL_DISABLE_FILEWATCHER=1",
				"OPENCODE_FAST_BOOT=1",
				"OPENCODE_PURE=1",
				organicActorRoleEnvironment+"="+test.role,
				organicActorRepoEnvironment+"="+harness.repo.worktree,
				organicActorPathEnvironment+"="+test.path,
				organicActorBodyEnvironment+"="+body,
				organicActorMessageEnvironment+"=docs: implement the real-agent outcome",
				organicActorBinaryEnvironment+"="+organicBinary,
				"GENTLE_AI_ORGANIC_ACTOR_EXECUTABLE="+os.Args[0],
				"GENTLE_AI_ORGANIC_BINARY="+organicBinary,
			)

			ctx, cancel := context.WithTimeout(context.Background(), organicAgentTimeout)
			defer cancel()
			command := organicCommandContext(ctx, "opencode", "run", "--pure",
				"--format", "json", "--agent", "organic", "--model", "fixture/fixture",
				"--dir", harness.repo.worktree, test.outcome,
			)
			command.Dir = harness.repo.worktree
			command.Env = environment
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("opencode run: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
			}
			model.assertComplete(t, test.wantSubagent)

			transcript := stdout.String()
			if !strings.Contains(transcript, test.marker) {
				t.Fatalf("the real agent never reported %q:\n%s", test.marker, transcript)
			}
			if harness.git("rev-parse", "HEAD") == harness.repo.baseRevision {
				t.Fatal("the real agent never created a candidate commit")
			}
			gate := harness.gate("pre-push")
			if !gate.Allowed || gate.Result != organicGateAllow {
				t.Fatalf("the real-agent candidate was refused at delivery: %#v", gate)
			}
			// A real sub-agent must not escalate its own route either.
			harness.assertNoSDDArtifacts()
			harness.assertSingleReviewLineage(lineage)
		})
	}
}

func TestRealAgentInstalledSDDApplyExecutorDoesNotDelegate(t *testing.T) {
	if os.Getenv(realAgentE2EEnvironment) != "1" {
		t.Skip("set GENTLE_AI_REAL_AGENT_E2E=1 to run the pinned real-agent journeys")
	}
	requireOrganicExecutableVersion(t, "opencode", pinnedOpenCodeVersion)

	configRoot := prepareOpenCodeConfig(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", configRoot)

	const (
		executorPrompt               = "Execute the assigned SDD apply phase without delegation."
		executorCompletionPrefix     = "SDD_APPLY_EXECUTOR_COMPLETED:"
		orchestratorCompletionMarker = "SDD_APPLY_ORCHESTRATOR_COMPLETED"
	)
	executorNonce := fmt.Sprintf("sdd-apply-executor-nonce-%d", time.Now().UnixNano())
	fixture := newOpenCodeFixtureServer(t, []openCodeTurn{
		{tool: "task", arguments: map[string]any{
			"description":   "Run the installed SDD apply executor",
			"prompt":        executorPrompt,
			"subagent_type": "sdd-apply",
		}},
	}, executorPrompt)
	defer fixture.Close()
	fixture.requireInstalledSDDApplyExecutor = true
	fixture.executorNonce = executorNonce
	fixture.executorCompletionPrefix = executorCompletionPrefix
	fixture.completion = orchestratorCompletionMarker
	fixture.actorCommand = "echo " + executorNonce

	settingsPath := filepath.Join(configRoot, "opencode", "opencode.json")
	if err := os.WriteFile(settingsPath, []byte(organicOpenCodeConfig(t, fixture.URL)), 0o600); err != nil {
		t.Fatalf("write OpenCode fixture config: %v", err)
	}
	if _, err := sdd.Inject(home, opencode.NewAdapter(), model.SDDModeMulti); err != nil {
		t.Fatalf("install SDD OpenCode assets: %v", err)
	}

	workdir := t.TempDir()
	environment := append(os.Environ(),
		"OPENCODE_CONFIG_DIR="+filepath.Join(configRoot, "opencode"),
		"OPENCODE_TEST_HOME="+filepath.Join(home, "opencode"),
		"OPENCODE_AUTH_CONTENT={}",
		"OPENCODE_DISABLE_PROJECT_CONFIG=1",
		"OPENCODE_DISABLE_AUTOUPDATE=1",
		"OPENCODE_DISABLE_AUTOCOMPACT=1",
		"OPENCODE_DISABLE_CLAUDE_CODE=1",
		"OPENCODE_DISABLE_DEFAULT_PLUGINS=1",
		"OPENCODE_DISABLE_EXTERNAL_SKILLS=1",
		"OPENCODE_DISABLE_LSP_DOWNLOAD=1",
		"OPENCODE_DISABLE_MODELS_FETCH=1",
		"OPENCODE_EXPERIMENTAL_DISABLE_FILEWATCHER=1",
		"OPENCODE_FAST_BOOT=1",
		"OPENCODE_PURE=1",
	)
	ctx, cancel := context.WithTimeout(context.Background(), organicAgentTimeout)
	defer cancel()
	command := organicCommandContext(ctx, "opencode", "run", "--pure",
		"--format", "json", "--agent", "gentle-orchestrator", "--model", "fixture/fixture",
		"--dir", workdir, "Delegate the assigned phase to the installed sdd-apply executor.",
	)
	command.Dir = workdir
	command.Env = environment
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("run installed sdd-apply executor: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	fixture.assertComplete(t, true)
	fixture.assertInstalledSDDApplyExecutorProof(t)
	if !strings.Contains(stdout.String(), orchestratorCompletionMarker) {
		t.Fatalf("orchestrator did not complete after the executor result round trip:\n%s", stdout.String())
	}
}

func TestInstalledSDDApplyExecutorProofRejectsOrchestratorSpoof(t *testing.T) {
	const (
		nonce            = "sdd-apply-executor-nonce-spoof-control"
		executorComplete = "SDD_APPLY_EXECUTOR_COMPLETED:" + nonce
	)
	fixture := &openCodeFixtureServer{
		requireInstalledSDDApplyExecutor: true,
		executorNonce:                    nonce,
		executorCompletionPrefix:         "SDD_APPLY_EXECUTOR_COMPLETED:",
		executorSubagentResult:           executorComplete,
	}
	recorder := httptest.NewRecorder()
	input := openAIRequest{Messages: []openAIMessage{{Role: "tool", Content: executorComplete}}}

	if fixture.acceptInstalledSDDApplyExecutorRoundTrip(recorder, input) {
		t.Fatal("an orchestrator-spoofed executor completion was accepted without an executor bash result")
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("spoof response status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(fixture.failure, "without executor bash result") {
		t.Fatalf("spoof refusal = %q, want missing executor bash result", fixture.failure)
	}
}

func TestInstalledSDDApplyExecutorRoundTripRejectsMissingCredentials(t *testing.T) {
	tests := []struct {
		name   string
		nonce  string
		prefix string
	}{
		{name: "empty nonce", prefix: "SDD_APPLY_EXECUTOR_COMPLETED:"},
		{name: "empty completion prefix", nonce: "sdd-apply-executor-nonce-missing-prefix"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const executorResult = "SDD_APPLY_EXECUTOR_COMPLETED:fixture-result"
			fixture := &openCodeFixtureServer{
				executorNonce:            tt.nonce,
				executorCompletionPrefix: tt.prefix,
				executorBashResult:       tt.nonce,
				executorSubagentResult:   executorResult,
			}
			recorder := httptest.NewRecorder()
			input := openAIRequest{Messages: []openAIMessage{{Role: "tool", Content: executorResult}}}

			if fixture.acceptInstalledSDDApplyExecutorRoundTrip(recorder, input) {
				t.Fatal("round trip accepted missing executor proof credentials")
			}
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("response status = %d, want %d", recorder.Code, http.StatusInternalServerError)
			}
			if !strings.Contains(fixture.failure, "missing nonce or completion marker") {
				t.Fatalf("refusal = %q, want missing credentials", fixture.failure)
			}
		})
	}
}

func TestInstalledSDDApplyExecutorRoundTripRejectsUnrelatedBashOutput(t *testing.T) {
	const (
		nonce            = "sdd-apply-executor-nonce-unrelated-output"
		executorComplete = "SDD_APPLY_EXECUTOR_COMPLETED:" + nonce
	)
	fixture := &openCodeFixtureServer{
		executorNonce:            nonce,
		executorCompletionPrefix: "SDD_APPLY_EXECUTOR_COMPLETED:",
		executorBashResult:       "completed a different command successfully",
		executorSubagentResult:   executorComplete,
	}
	recorder := httptest.NewRecorder()
	input := openAIRequest{Messages: []openAIMessage{{Role: "tool", Content: executorComplete}}}

	if fixture.acceptInstalledSDDApplyExecutorRoundTrip(recorder, input) {
		t.Fatal("round trip accepted non-empty bash output without the executor nonce")
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("response status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(fixture.failure, "without executor bash result") {
		t.Fatalf("refusal = %q, want nonce-bound executor bash result", fixture.failure)
	}
}

// organicActorToolCommand runs the compiled actor process from the agent's own
// bash tool, so the implementation step is a real child process of a real agent.
func organicActorToolCommand(t *testing.T) string {
	t.Helper()
	return organicToolCommand(t, "GENTLE_AI_ORGANIC_ACTOR_EXECUTABLE")
}

func organicReviewToolCommand(t *testing.T, arguments ...string) string {
	t.Helper()
	return organicToolCommand(t, "GENTLE_AI_ORGANIC_BINARY", arguments...)
}

// organicToolCommand turns one fixture-authored argv into the string the
// agent's bash tool executes, without round-tripping the argv through a shell
// flavour we do not control.
//
// On POSIX systems the agent's tool shell is sh-compatible, so the argv is
// authored directly in sh syntax — byte-identical to what the journeys always
// ran on Linux. On Windows the agent's tool shell is PowerShell (or cmd),
// neither of which parses POSIX single-quoting (PowerShell fails with
// "ParserError: Unexpected token" on a single-quoted argument such as
// 'review'), so the argv is baked by Go into a generated .cmd trampoline
// and the command becomes that script's bare path: a single token that
// PowerShell and cmd both resolve as a plain invocation.
func organicToolCommand(t *testing.T, binaryEnvironment string, arguments ...string) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		command := `"$` + binaryEnvironment + `"`
		for _, argument := range arguments {
			command += " '" + strings.ReplaceAll(argument, "'", `'\''`) + "'"
		}
		return command
	}

	invocation := `"%` + binaryEnvironment + `%"`
	for _, argument := range arguments {
		if strings.ContainsAny(argument, `"%`) {
			t.Fatalf("tool argument %q cannot be embedded safely in a cmd trampoline", argument)
		}
		invocation += ` "` + argument + `"`
	}
	script := filepath.Join(t.TempDir(), "tool.cmd")
	if strings.ContainsAny(script, " \t'\"") {
		t.Fatalf("tool trampoline path %q would itself need shell quoting", script)
	}
	content := strings.Join([]string{"@echo off", invocation, "exit /b %ERRORLEVEL%"}, "\r\n") + "\r\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return script
}

type openCodeTurn struct {
	tool      string
	arguments map[string]any
}

type openCodeFixtureServer struct {
	*httptest.Server
	mu                               sync.Mutex
	script                           []openCodeTurn
	actorPrompt                      string
	actorCommand                     string
	completion                       string
	subagentCompletion               string
	requireInstalledSDDApplyExecutor bool
	sawInstalledSDDApplyExecutor     bool
	executorNonce                    string
	executorCompletionPrefix         string
	executorBashResult               string
	executorSubagentResult           string
	executorRoundTripResult          string
	mainCalls                        int
	subagentStarts                   int
	failure                          string
}

func newOpenCodeFixtureServer(t *testing.T, script []openCodeTurn, actorPrompt string) *openCodeFixtureServer {
	t.Helper()
	fixture := &openCodeFixtureServer{
		script:      script,
		actorPrompt: actorPrompt,
		// Precomputed with the test handle: the HTTP handler that serves the
		// delegated worker turn has no *testing.T of its own.
		actorCommand: organicActorToolCommand(t),
	}
	fixture.Server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	return fixture
}

type openAIRequest struct {
	Messages []openAIMessage   `json:"messages"`
	Tools    []json.RawMessage `json:"tools"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func (fixture *openCodeFixtureServer) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method", http.StatusMethodNotAllowed)
		return
	}
	var input openAIRequest
	if err := json.NewDecoder(io.LimitReader(request.Body, 8<<20)).Decode(&input); err != nil {
		fixture.fail(writer, "decode model request: %v", err)
		return
	}
	if len(input.Tools) == 0 {
		fixture.writeText(writer, "Organic runtime journey", "stop")
		return
	}
	if fixture.isSubagent(input) {
		if fixture.requireInstalledSDDApplyExecutor && !fixture.acceptInstalledSDDApplyExecutor(writer, input) {
			return
		}
		fixture.mu.Lock()
		fixture.subagentStarts++
		fixture.mu.Unlock()
		last := input.Messages[len(input.Messages)-1]
		if last.Role == "tool" {
			if fixture.requireInstalledSDDApplyExecutor && !fixture.captureInstalledSDDApplyExecutorBashResult(writer, messageText(last.Content)) {
				return
			}
			completion := fixture.subagentCompletion
			if fixture.requireInstalledSDDApplyExecutor {
				fixture.mu.Lock()
				completion = fixture.executorSubagentResult
				fixture.mu.Unlock()
			}
			if completion == "" {
				completion = organicDelegatedActorMarker
			}
			fixture.writeText(writer, completion, "stop")
			return
		}
		fixture.writeTool(writer, "delegated-actor", "bash", map[string]any{"command": fixture.actorCommand})
		return
	}

	fixture.mu.Lock()
	fixture.mainCalls++
	call := fixture.mainCalls
	fixture.mu.Unlock()
	if call > len(fixture.script) {
		if fixture.requireInstalledSDDApplyExecutor && !fixture.acceptInstalledSDDApplyExecutorRoundTrip(writer, input) {
			return
		}
		completion := fixture.completion
		if completion == "" {
			completion = "Organic journey complete."
		}
		fixture.writeText(writer, completion, "stop")
		return
	}
	turn := fixture.script[call-1]
	fixture.writeTool(writer, fmt.Sprintf("turn-%d", call), turn.tool, turn.arguments)
}

func (fixture *openCodeFixtureServer) acceptInstalledSDDApplyExecutor(writer http.ResponseWriter, input openAIRequest) bool {
	var transcript strings.Builder
	for _, message := range input.Messages {
		transcript.WriteString(messageText(message.Content))
	}
	content := transcript.String()
	// The proof asserts the role contract the executor actually received, not
	// one wording of it. Ordering between two blocks is no longer the property
	// under test: a single block whose every imperative follows its own
	// condition is, and the executor branch must come first because these
	// skills are delegate_only and the sub-agent is the intended reader.
	role := strings.Index(content, "## Execution Role")
	if role < 0 {
		fixture.fail(writer, "installed sdd-apply executor did not load its Execution Role block")
		return false
	}
	executor := strings.Index(content, "If you are the `sdd-apply` sub-agent")
	orchestrator := strings.Index(content, "If you loaded this skill through the `skill()` tool")
	if executor < 0 || orchestrator < 0 || executor > orchestrator {
		fixture.fail(writer, "installed sdd-apply executor role block does not state the executor branch before the orchestrator branch")
		return false
	}
	if !strings.Contains(content, "continue with the phase work below. Do not delegate. Do not call the Skill tool.") {
		fixture.fail(writer, "installed sdd-apply executor is missing its non-delegating continuation instruction")
		return false
	}
	for _, retraction := range []string{"does NOT apply to you", "the gate above", "the gate below"} {
		if strings.Contains(content, retraction) {
			fixture.fail(writer, "installed sdd-apply executor received a retraction phrase %q that undoes an earlier imperative", retraction)
			return false
		}
	}
	for _, rawTool := range input.Tools {
		var tool struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if err := json.Unmarshal(rawTool, &tool); err != nil {
			fixture.fail(writer, "decode installed sdd-apply tool definition: %v", err)
			return false
		}
		if tool.Function.Name == "task" {
			fixture.fail(writer, "installed sdd-apply executor was offered the task delegation tool")
			return false
		}
	}
	fixture.mu.Lock()
	fixture.sawInstalledSDDApplyExecutor = true
	fixture.mu.Unlock()
	return true
}

func (fixture *openCodeFixtureServer) captureInstalledSDDApplyExecutorBashResult(writer http.ResponseWriter, result string) bool {
	fixture.mu.Lock()
	nonce := fixture.executorNonce
	prefix := fixture.executorCompletionPrefix
	fixture.mu.Unlock()
	if nonce == "" || prefix == "" {
		fixture.fail(writer, "installed sdd-apply executor proof is missing its nonce or completion marker")
		return false
	}
	if !strings.Contains(result, nonce) {
		fixture.fail(writer, "installed sdd-apply executor bash result does not contain its nonce")
		return false
	}
	fixture.mu.Lock()
	fixture.executorBashResult = result
	fixture.executorSubagentResult = prefix + nonce
	fixture.mu.Unlock()
	return true
}

func (fixture *openCodeFixtureServer) acceptInstalledSDDApplyExecutorRoundTrip(writer http.ResponseWriter, input openAIRequest) bool {
	fixture.mu.Lock()
	nonce := fixture.executorNonce
	prefix := fixture.executorCompletionPrefix
	bashResult := fixture.executorBashResult
	executorResult := fixture.executorSubagentResult
	fixture.mu.Unlock()
	if nonce == "" || prefix == "" {
		fixture.fail(writer, "installed sdd-apply executor proof is missing nonce or completion marker")
		return false
	}
	if !strings.Contains(bashResult, nonce) {
		fixture.fail(writer, "orchestrator cannot complete the executor proof without executor bash result")
		return false
	}
	if executorResult == "" {
		fixture.fail(writer, "installed sdd-apply executor did not return a nonce-bound subagent result")
		return false
	}
	for index := len(input.Messages) - 1; index >= 0; index-- {
		message := input.Messages[index]
		if message.Role != "tool" {
			continue
		}
		result := messageText(message.Content)
		if strings.Contains(result, executorResult) && strings.Contains(result, nonce) {
			fixture.mu.Lock()
			fixture.executorRoundTripResult = result
			fixture.mu.Unlock()
			return true
		}
	}
	fixture.fail(writer, "orchestrator did not receive the nonce-bound executor subagent result")
	return false
}

func (fixture *openCodeFixtureServer) assertInstalledSDDApplyExecutorProof(t *testing.T) {
	t.Helper()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	nonce := fixture.executorNonce
	prefix := fixture.executorCompletionPrefix
	if nonce == "" || prefix == "" {
		t.Fatal("installed sdd-apply executor proof is missing nonce or completion marker")
	}
	executorResult := prefix + nonce
	if fixture.completion == executorResult {
		t.Fatal("orchestrator and executor completion markers must be distinct")
	}
	if !strings.Contains(fixture.actorCommand, nonce) {
		t.Fatal("the installed sdd-apply executor did not receive its nonce-bearing bash command")
	}
	if !strings.Contains(fixture.executorBashResult, nonce) {
		t.Fatal("the installed sdd-apply executor did not return its bash-produced nonce")
	}
	if fixture.executorSubagentResult != executorResult {
		t.Fatalf("executor subagent result = %q, want %q", fixture.executorSubagentResult, executorResult)
	}
	if !strings.Contains(fixture.executorRoundTripResult, executorResult) {
		t.Fatal("orchestrator did not receive the executor result through its task round trip")
	}
}

// isSubagent recognises the delegated worker session. OpenCode gives the
// sub-agent its own conversation seeded with the delegation prompt, so the
// prompt's presence in a user message is what distinguishes the two sessions.
func (fixture *openCodeFixtureServer) isSubagent(input openAIRequest) bool {
	if strings.TrimSpace(fixture.actorPrompt) == "" {
		return false
	}
	for _, message := range input.Messages {
		if message.Role == "user" && strings.Contains(messageText(message.Content), fixture.actorPrompt) {
			return true
		}
	}
	return false
}

func (fixture *openCodeFixtureServer) fail(writer http.ResponseWriter, format string, arguments ...any) {
	fixture.mu.Lock()
	fixture.failure = fmt.Sprintf(format, arguments...)
	fixture.mu.Unlock()
	http.Error(writer, "fixture failure", http.StatusInternalServerError)
}

func (fixture *openCodeFixtureServer) writeTool(writer http.ResponseWriter, id, name string, arguments any) {
	encoded, _ := json.Marshal(arguments)
	fixture.writeChunks(writer, []any{
		map[string]any{
			"id": "chat", "object": "chat.completion.chunk", "created": 0, "model": "fixture",
			"choices": []any{map[string]any{
				"index": 0,
				"delta": map[string]any{
					"role": "assistant",
					"tool_calls": []any{map[string]any{
						"index": 0, "id": "call_" + id, "type": "function",
						"function": map[string]any{"name": name, "arguments": string(encoded)},
					}},
				},
				"finish_reason": nil,
			}},
		},
		organicFinishChunk("tool_calls"),
	})
}

func (fixture *openCodeFixtureServer) writeText(writer http.ResponseWriter, content, reason string) {
	fixture.writeChunks(writer, []any{
		map[string]any{
			"id": "chat", "object": "chat.completion.chunk", "created": 0, "model": "fixture",
			"choices": []any{map[string]any{
				"index":         0,
				"delta":         map[string]any{"role": "assistant", "content": content},
				"finish_reason": nil,
			}},
		},
		organicFinishChunk(reason),
	})
}

func organicFinishChunk(reason string) map[string]any {
	return map[string]any{
		"id": "chat", "object": "chat.completion.chunk", "created": 0, "model": "fixture",
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": reason}},
		"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	}
}

func (fixture *openCodeFixtureServer) writeChunks(writer http.ResponseWriter, chunks []any) {
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	for _, chunk := range chunks {
		encoded, _ := json.Marshal(chunk)
		_, _ = fmt.Fprintf(writer, "data: %s\n\n", encoded)
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	_, _ = io.WriteString(writer, "data: [DONE]\n\n")
}

func (fixture *openCodeFixtureServer) assertComplete(t *testing.T, wantSubagent bool) {
	t.Helper()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.failure != "" {
		t.Fatal(fixture.failure)
	}
	if fixture.mainCalls < len(fixture.script) {
		t.Fatalf("the agent issued %d of %d scripted turns", fixture.mainCalls, len(fixture.script))
	}
	if hadSubagent := fixture.subagentStarts > 0; hadSubagent != wantSubagent {
		t.Fatalf("real sub-agent used = %t, want %t", hadSubagent, wantSubagent)
	}
	if fixture.requireInstalledSDDApplyExecutor && !fixture.sawInstalledSDDApplyExecutor {
		t.Fatal("the installed sdd-apply executor never loaded its runtime prompt")
	}
}

func organicOpenCodeConfig(t *testing.T, serverURL string) string {
	t.Helper()
	config := map[string]any{
		"provider": map[string]any{
			"fixture": map[string]any{
				"npm":     "@ai-sdk/openai-compatible",
				"name":    "Organic E2E Fixture",
				"options": map[string]any{"baseURL": serverURL + "/v1", "apiKey": "fixture"},
				"models":  map[string]any{"fixture": map[string]any{"name": "Fixture"}},
			},
		},
		"agent": map[string]any{
			"organic": map[string]any{
				"description": "Organic runtime E2E",
				"mode":        "primary",
				"model":       "fixture/fixture",
				"permission":  map[string]any{"bash": "allow", "task": "allow", "edit": "deny"},
			},
		},
		"plugin":     []any{},
		"compaction": map[string]any{"auto": false},
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func prepareOpenCodeConfig(t *testing.T) string {
	t.Helper()
	requireOrganicExecutable(t, "npm")
	root := t.TempDir()
	config := filepath.Join(root, "opencode")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"private":true,"dependencies":{"@opencode-ai/plugin":"` + pinnedOpenCodeVersion + `"}}` + "\n")
	if err := os.WriteFile(filepath.Join(config, "package.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), organicSetupTimeout)
	defer cancel()
	command := organicCommandContext(ctx, "npm", "install", "--ignore-scripts", "--no-audit", "--no-fund", "--package-lock=false", "--prefix", config)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("prepare the pinned OpenCode plugin: %v\n%s", err, output)
	}
	return root
}

func requireOrganicExecutable(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Fatalf("required executable %s: %v", name, err)
	}
}

func requireOrganicExecutableVersion(t *testing.T, name, expected string) {
	t.Helper()
	requireOrganicExecutable(t, name)
	ctx, cancel := context.WithTimeout(context.Background(), organicLocalTimeout)
	defer cancel()
	command := organicCommandContext(ctx, name, "--version")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s --version: %v\n%s", name, err, output)
	}
	if strings.TrimSpace(string(output)) != expected {
		t.Fatalf("%s version = %q, want %q", name, strings.TrimSpace(string(output)), expected)
	}
}

func messageText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		var builder strings.Builder
		for _, part := range value {
			encoded, _ := json.Marshal(part)
			builder.Write(encoded)
		}
		return builder.String()
	default:
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
}
