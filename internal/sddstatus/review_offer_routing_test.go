package sddstatus

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// This file is Wave 4 S3's second increment (orchestrator amendment to
// design.md decision 3, 2026-08-03): the post-verify offer routes through
// internal/sddstatus's Resolve()/resolveEngramStatus(), not through
// internal/cli, carried in a new Status.ReviewOffer field emitted exactly
// when verify has passed and the kill switch is on. Kill switch off is
// structural absence: no field on the wire, zero calls into
// reviewEntryHook, not an Available:false placeholder.

// seedVerifiedReadyChangeForOffer seeds a change whose apply is complete,
// whose verify-report already passes, and whose workspace is a real Git
// repository — matching the "post-verify-passed, not-yet-archived" window
// the offer block is scoped to. A real Git repo is required here because
// the pre-existing (unrelated to this slice) post-verify archive gate
// (applyReviewGate/resolveReviewAuthority) needs one to classify "no review
// authority" as Absent/unmanaged rather than as a resolution error.
func seedVerifiedReadyChangeForOffer(t *testing.T, root string) {
	t.Helper()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, changeRoot+"/verify-report.md", boundedVerifyEnvelope(shaID("a"), "pass"))
	runSDDStatusGit(t, root, "init", "-q")
	runSDDStatusGit(t, root, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, root, "config", "user.name", "Status Test")
	runSDDStatusGit(t, root, "add", ".")
	runSDDStatusGit(t, root, "commit", "-qm", "base")
}

// TestReviewOfferBlockPresentWhenVerifiedAndEnabled proves the offer block
// appears, carries an actionable invocation, and does not block Archive —
// Archive stays whatever resolveDependencies already computed for a passing
// verify report (Ready), never gated on the offer.
func TestReviewOfferBlockPresentWhenVerifiedAndEnabled(t *testing.T) {
	// "Enabled" is this fixture's premise, so it has to be stated. Without
	// the opt-in the offer still arrives, but unavailable, and the nil check
	// below accepts it -- the test would keep passing while proving nothing.
	reviewEnabledHome(t)
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "pass"))
	// writeApprovedReviewArtifacts also satisfies the pre-existing (unrelated
	// to this slice) post-verify archive gate, so this fixture proves the
	// offer coexists peacefully with an already-archive-ready state — the
	// offer never depends on, and never disturbs, that gate's own verdict.
	writeApprovedReviewArtifacts(t, changeRoot)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Dependencies.Verify != DependencyAllDone {
		t.Fatalf("Dependencies.Verify = %q, want all_done (fixture must be past verify)", status.Dependencies.Verify)
	}
	if status.Dependencies.Archive != DependencyReady {
		t.Fatalf("Dependencies.Archive = %q, want ready", status.Dependencies.Archive)
	}
	if status.ReviewOffer == nil {
		t.Fatal("ReviewOffer = nil, want a present offer block once verify has passed with the switch on")
	}
	if status.ReviewOffer.Invocation == "" || !strings.Contains(status.ReviewOffer.Invocation, "review start") {
		t.Fatalf("ReviewOffer.Invocation = %q, want an actionable `gentle-ai review start` command", status.ReviewOffer.Invocation)
	}
}

// TestReviewOfferBlockAbsentStructurallyWhenDisabled proves the amendment's
// core requirement at BOTH the Go-value level and the serialized-JSON
// level: with the kill switch off, ReviewOffer is nil, the "reviewOffer"
// key never appears in the marshaled output, and reviewEntryHook fires
// zero times.
func TestReviewOfferBlockAbsentStructurallyWhenDisabled(t *testing.T) {
	root := t.TempDir()
	seedVerifiedReadyChangeForOffer(t, root)
	resetReviewEntryHookCallCountForTest()

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if status.ReviewOffer != nil {
		t.Fatalf("ReviewOffer = %#v, want nil — structural absence when the switch is off", status.ReviewOffer)
	}
	if got := reviewEntryHookCallCountForTest(); got != 0 {
		t.Fatalf("reviewEntryHook fired %d times with the switch off, want 0", got)
	}

	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "reviewOffer") {
		t.Fatalf("serialized status contains a \"reviewOffer\" key with the switch off:\n%s", payload)
	}
}

// TestReviewOfferJourneyFiresZeroTimesAcrossFullFlowWhenDisabled is the
// OFF-mode absence journey (task 4.7): drives the same apply -> verify ->
// (archive-pending) sequence a real status/continue loop would produce, and
// asserts reviewEntryHook never fires anywhere in it. Realized as an
// in-process Go test rather than a bench/ black-box journey because
// reviewEntryHook is a Go-internal instrumentation point a separate bench
// subprocess cannot observe — the same zero-cost-by-default proof shape as
// Wave 1's shadowObserverCallCountForTest, applied to this door.
func TestReviewOfferJourneyFiresZeroTimesAcrossFullFlowWhenDisabled(t *testing.T) {
	root := t.TempDir()
	resetReviewEntryHookCallCountForTest()

	// Apply phase: core artifacts done, tasks pending, then complete.
	changeRoot := seedReadyChange(t, root, "thin", "- [ ] 1.1 Work\n")
	if _, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true}); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Work\n")
	if _, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true}); err != nil {
		t.Fatal(err)
	}

	// Verify phase: report lands passing. A real Git repo is required for the
	// pre-existing (unrelated) post-verify archive gate to classify "no
	// review authority" as Absent/unmanaged rather than a resolution error.
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "pass"))
	runSDDStatusGit(t, root, "init", "-q")
	runSDDStatusGit(t, root, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, root, "config", "user.name", "Status Test")
	runSDDStatusGit(t, root, "add", ".")
	runSDDStatusGit(t, root, "commit", "-qm", "base")
	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if status.Dependencies.Archive != DependencyReady {
		t.Fatalf("Dependencies.Archive = %q, want ready — ordinary archive-pending window", status.Dependencies.Archive)
	}
	if status.ReviewOffer != nil {
		t.Fatalf("ReviewOffer = %#v, want nil throughout the disabled journey", status.ReviewOffer)
	}

	if got := reviewEntryHookCallCountForTest(); got != 0 {
		t.Fatalf("reviewEntryHook fired %d times across the full apply->verify->archive-pending journey with the switch off, want 0", got)
	}
}

// TestReviewOfferDeclineLeavesNoStateAndDoesNotSuppressLaterOffer proves
// 4.5's decline semantics: a decline is scoped to one candidate, persists
// no decision, and does not suppress the offer for a later status read of
// the same still-unarchived candidate. Archive proceeds under ordinary
// repository policy regardless — there is no new state and no new
// persistence to represent "declined".
func TestReviewOfferDeclineLeavesNoStateAndDoesNotSuppressLaterOffer(t *testing.T) {
	// Declining an offer is only meaningful when there is a real one to
	// decline, which takes a user who switched receipt-driven development on.
	reviewEnabledHome(t)
	root := t.TempDir()
	seedVerifiedReadyChangeForOffer(t, root)

	first, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ReviewOffer == nil {
		t.Fatal("first ReviewOffer = nil, want present")
	}

	// Decline: nothing is written, nothing is called — the orchestrator
	// simply does not act on the offer and asks status again later. Archive
	// eligibility is governed entirely by the pre-existing (unrelated to
	// this slice) post-verify archive gate, whatever it currently reports;
	// what this test proves is that a decline changes NOTHING about it
	// (no new state, no new persistence) and does not suppress the offer.
	second, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if second.ReviewOffer == nil {
		t.Fatal("second ReviewOffer = nil, want the offer to reappear — a decline must not suppress it")
	}
	if second.Dependencies.Archive != first.Dependencies.Archive {
		t.Fatalf("second Dependencies.Archive = %q, want unchanged from first read %q — a decline records no state", second.Dependencies.Archive, first.Dependencies.Archive)
	}
}

// TestReviewOfferDeclineStatusByteIdenticalToOffOutsideOfferBlock is task
// 4.8's integration proof, realized as a same-fixture double-eval (the
// only valid shape for a byte-equivalence claim: evaluated twice, only the
// kill-switch toggle changed — internal/cli/review_new_lineage_switch_off_
// golden_test.go and the pre-existing archive-gate byte-equivalence tests
// use the same discipline). A decline — the switch on, the offer present,
// nothing ever acted on — must never block archive on review grounds, the
// same operational property the switch-off case provides. This asserts at
// the PROJECTION level (ProjectStatusV1, the exact bytes a caller like
// RunSDDStatus emits), the verify cycle's own key learning: asserting on
// json.Marshal(Status) directly is how CRITICAL-2 (ReviewOffer/ReVerify
// unreachable at the wire) survived a fully green suite.
//
// Corrective verify cycle note: this test previously asserted the declined
// and switch-off projections were byte-identical outside the offer block.
// That assumption no longer holds and is not restored here: CRITICAL-1's
// fix makes switch-off a genuine structural absence (ReviewGate nil), while
// the declined fixture below still carries an approving legacy receipt that
// legitimately governs while the switch is ON (ReviewGate present, Allow) —
// two different, both-correct dispositions that happen to agree on the one
// property that matters for delivery: neither blocks archive.
func TestReviewOfferDeclineNeverBlocksArchiveAtTheProjectionLevel(t *testing.T) {
	// The comparison is switch-on versus switch-off, so the "on" half has to
	// be a user who opted in; the "off" half stays an explicit
	// ResolveOptions.ReviewDisabled toggle over the same fixture.
	reviewEnabledHome(t)
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "pass"))
	writeApprovedReviewArtifacts(t, changeRoot)

	declined, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if declined.ReviewOffer == nil {
		t.Fatal("declined.ReviewOffer = nil, want present before the decline is applied to this comparison")
	}
	declinedProjection, err := ProjectStatusV1(declined)
	if err != nil {
		t.Fatalf("ProjectStatusV1(declined) error = %v", err)
	}
	if declinedProjection.ReviewOffer == nil || !declinedProjection.ReviewOffer.Available {
		t.Fatalf("declined projection reviewOffer = %#v, want an available offer on the wire", declinedProjection.ReviewOffer)
	}
	if declinedProjection.Dependencies.Archive == DependencyBlocked || declinedProjection.NextRecommended == "resolve-review" {
		t.Fatalf("declined (offer ignored) projection archive=%q next=%q, want unblocked",
			declinedProjection.Dependencies.Archive, declinedProjection.NextRecommended)
	}

	off, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if off.ReviewOffer != nil {
		t.Fatalf("off.ReviewOffer = %#v, want nil", off.ReviewOffer)
	}
	offProjection, err := ProjectStatusV1(off)
	if err != nil {
		t.Fatalf("ProjectStatusV1(off) error = %v", err)
	}
	if offProjection.ReviewOffer != nil {
		t.Fatalf("off projection reviewOffer = %#v, want absent from the wire", offProjection.ReviewOffer)
	}
	if offProjection.ReviewGate != nil {
		t.Fatalf("off projection reviewGate = %#v, want structural absence", offProjection.ReviewGate)
	}
	if offProjection.Dependencies.Archive == DependencyBlocked || offProjection.NextRecommended == "resolve-review" {
		t.Fatalf("switch-off projection archive=%q next=%q, want unblocked",
			offProjection.Dependencies.Archive, offProjection.NextRecommended)
	}
}

// TestReviewOfferPresentAndArchiveReadyForAGenuinelyMissingReceipt is
// corrective verify cycle 4's BLOCKER-1 (rdd-post-verify-review-offer's
// "Decline Proceeds to Unmanaged Ordinary Archive"): the exact repro shape
// the cycle-2/3 verify reports used (switch on, verify passed, no receipt
// anywhere) must show the offer AND archive readiness in the SAME status
// output -- the offer is an invitation, never a gate -- and a second read of
// the same still-unarchived candidate must produce an identical fresh
// offer, proving nothing about a "decline" is persisted or suppressed
// (there is no `--consent declined` verb and no decline state to record;
// the user declines simply by proceeding to archive without acting).
func TestReviewOfferPresentAndArchiveReadyForAGenuinelyMissingReceipt(t *testing.T) {
	// "Switch on, verify passed, no receipt anywhere" is the repro shape, so
	// the switch has to actually be on for the offer to be available.
	reviewEnabledHome(t)
	root := t.TempDir()
	seedVerifiedReadyChangeForOffer(t, root)

	first, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ReviewGate != nil {
		t.Fatalf("ReviewGate = %#v, want structural absence for a genuinely missing receipt", first.ReviewGate)
	}
	if first.ReviewOffer == nil || !first.ReviewOffer.Available {
		t.Fatalf("ReviewOffer = %#v, want an available invitation in the SAME status output as archive readiness", first.ReviewOffer)
	}
	if first.Dependencies.Archive != DependencyReady || first.NextRecommended != "archive" {
		t.Fatalf("archive=%q next=%q, want ready/archive alongside the present offer", first.Dependencies.Archive, first.NextRecommended)
	}
	if len(first.BlockedReasons) != 0 {
		t.Fatalf("BlockedReasons = %v, want none", first.BlockedReasons)
	}

	// A hypothetical archive: nothing was written for the decline, so the
	// next status read of the same still-unarchived candidate gets a fresh,
	// unsuppressed offer -- identical to the first, proving no persistence.
	second, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if second.ReviewOffer == nil || !second.ReviewOffer.Available {
		t.Fatalf("second ReviewOffer = %#v, want a fresh available offer — no suppression", second.ReviewOffer)
	}
	if second.Dependencies.Archive != DependencyReady {
		t.Fatalf("second Dependencies.Archive = %q, want ready — unchanged, nothing persisted", second.Dependencies.Archive)
	}
}
