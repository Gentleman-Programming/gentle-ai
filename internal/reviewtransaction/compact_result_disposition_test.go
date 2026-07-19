package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// unreplayablePayload is the reproduction shape from #1469: the reviewer
// emitted syntactically invalid JSON whose claims cite a file that is not in
// the frozen candidate at all.
const unreplayablePayload = `{"findings":[{"id":"R1-001","location":"internal/billing/charge.go:42","claim":"unbounded retry"},],"evidence":["read internal/billing/charge.go"]}`

type disposeFixture struct {
	repo   string
	root   string
	store  CompactStore
	record CompactRecord
}

// newDisposeFixture persists one high-risk reviewing lineage with four
// selected lenses so a disposition of a single lens can be observed against
// the results the other lenses already captured.
func newDisposeFixture(t *testing.T, lineage string) disposeFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "internal/auth/token.go", "package auth\n")
	gitSnapshot(t, repo, "add", "--", "internal/auth/token.go")
	gitSnapshot(t, repo, "commit", "-m", "auth base")
	writeSnapshotFile(t, repo, "internal/auth/token.go", "package auth\n\nconst Token = \"candidate\"\n")

	state := newCompactTestState(t, repo, lineage)
	if state.RiskLevel != RiskHigh || len(state.SelectedLenses) != 4 {
		t.Fatalf("fixture risk = %q with lenses %v, want the high-risk 4R sweep", state.RiskLevel, state.SelectedLenses)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return disposeFixture{repo: repo, root: root, store: store, record: record}
}

// preserveRawResult writes one raw reviewer payload exactly where
// review preserve-result writes its durable incident artifact.
func preserveRawResult(t *testing.T, repo, lineage, lens string, order int, payload []byte) (string, string) {
	t.Helper()
	dir, err := CompactIncidentsDir(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	digest := compactPreservedPayloadDigest(payload)
	path := filepath.Join(dir, fmt.Sprintf("%02d-%s-%s.raw", order, lens, strings.TrimPrefix(digest, "sha256:")[:12]))
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return digest, path
}

// captureValidResult stands in for a successful review capture-result so the
// disposition can be checked against results that must survive it untouched.
func captureValidResult(t *testing.T, store CompactStore, lens string, order int) (string, []byte) {
	t.Helper()
	dir := filepath.Join(store.Dir, CompactReviewerResultsDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := []byte(fmt.Sprintf("{\"lens\":%q,\"findings\":[],\"evidence\":[\"checked exact target\"]}\n", lens))
	path := filepath.Join(dir, fmt.Sprintf("%02d-%s.json", order, lens))
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, payload
}

// disposeAuthorization renders the binding literally rather than through the
// production builder, so the exact wire format stays pinned by the tests.
func disposeAuthorization(repository, lineage, revision, target, lens string, order int, digest, class, actor, reason string) string {
	return "gentle-ai.review-result-disposition-authorization/v1" +
		"\nrepository=" + repository +
		"\nlineage=" + lineage +
		"\nrevision=" + revision +
		"\ntarget_identity=" + target +
		"\nlens=" + lens +
		"\norder=" + strconv.Itoa(order) +
		"\nartifact_digest=" + digest +
		"\nclass=" + class +
		"\nactor=" + actor +
		"\nreason=" + reason
}

func (fixture disposeFixture) request(lens string, order int, digest string, class ResultDispositionClass, absent []string) CompactResultDispositionRequest {
	const actor, reason = "maintainer@example.com", "reviewer output cannot be replayed"
	const diagnostic = "decode reviewer result: invalid character after array element"
	target := fixture.record.State.InitialSnapshot.Identity
	return CompactResultDispositionRequest{
		LineageID: fixture.record.State.LineageID, ExpectedRevision: fixture.record.Revision,
		TargetIdentity: target, Lens: lens, SelectedOrder: order, ArtifactDigest: digest,
		Class: class, Diagnostic: diagnostic, AbsentPaths: absent, Reason: reason, Actor: actor,
		MaintainerAuthorization: disposeAuthorization(fixture.root, fixture.record.State.LineageID,
			fixture.record.Revision, target, lens, order, digest, string(class), actor, reason),
	}
}

// TestDisposeUnreplayablePreservedResultEscalatesAndRetainsCapturedResults is
// the #1469 Case A happy path: the stranded lineage gains a supported terminal
// transition, and every valid captured result survives it byte-for-byte.
func TestDisposeUnreplayablePreservedResultEscalatesAndRetainsCapturedResults(t *testing.T) {
	fixture := newDisposeFixture(t, "dispose-happy-path")
	lenses := fixture.record.State.SelectedLenses
	retained := map[string][]byte{}
	for order := 0; order < 3; order++ {
		path, payload := captureValidResult(t, fixture.store, lenses[order], order)
		retained[path] = payload
	}
	digest, rawPath := preserveRawResult(t, fixture.repo, fixture.record.State.LineageID, lenses[3], 3, []byte(unreplayablePayload))

	record, err := DisposeUnreplayablePreservedResult(context.Background(), fixture.repo,
		fixture.request(lenses[3], 3, digest, ResultDispositionTransportSyntax, nil))
	if err != nil {
		t.Fatalf("disposition of an unreplayable preserved result failed: %v", err)
	}
	if record.State != StateEscalated || record.Replayed || record.Revision == fixture.record.Revision ||
		record.PreviousRevision != fixture.record.Revision || record.ArtifactPath != rawPath {
		t.Fatalf("disposition record = %+v", record)
	}
	if len(record.RetainedLensResults) != 3 {
		t.Fatalf("retained lens results = %v, want the three captured artifacts", record.RetainedLensResults)
	}
	if record.Disposition.Class != ResultDispositionTransportSyntax || record.Disposition.Lens != lenses[3] ||
		record.Disposition.SelectedOrder != 3 || record.Disposition.ArtifactDigest != digest ||
		record.Disposition.TargetIdentity != fixture.record.State.InitialSnapshot.Identity ||
		record.Disposition.DisposedAt.IsZero() {
		t.Fatalf("disposition = %+v", record.Disposition)
	}

	after, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.State.State != StateEscalated || after.Revision != record.Revision {
		t.Fatalf("authority after disposition = %q at %s", after.State.State, after.Revision)
	}
	if len(after.State.ResultDispositions) != 1 {
		t.Fatalf("persisted dispositions = %+v", after.State.ResultDispositions)
	}
	// Requirement 7: the refused payload contributed no admissible content.
	if len(after.State.LensResults) != 0 || len(after.State.Findings) != 0 || len(after.State.Classifications) != 0 ||
		len(after.State.Outcomes) != 0 || after.State.EvidenceHash != "" {
		t.Fatalf("disposition admitted review content: %+v", after.State)
	}
	if !equalStrings(after.State.SelectedLenses, lenses) || !snapshotsEqual(after.State.InitialSnapshot, fixture.record.State.InitialSnapshot) {
		t.Fatal("disposition changed the frozen review scope")
	}
	// Requirement 6: the other lenses' captured artifacts are untouched.
	for path, want := range retained {
		got, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(got, want) {
			t.Fatalf("captured result %s changed: %v", path, readErr)
		}
	}
	// Requirement 3: the preserved raw output is byte-for-byte intact.
	raw, err := os.ReadFile(rawPath)
	if err != nil || !bytes.Equal(raw, []byte(unreplayablePayload)) {
		t.Fatalf("preserved raw output changed: %v", err)
	}
}

// TestDisposeUnreplayablePreservedResultRecordsWrongTargetEvidence pins
// requirement 4: the semantic wrong-target class is recorded distinctly from
// the transport/syntax class, and it must cite paths the payload references
// which the frozen candidate does not contain.
func TestDisposeUnreplayablePreservedResultRecordsWrongTargetEvidence(t *testing.T) {
	fixture := newDisposeFixture(t, "dispose-wrong-target")
	lens := fixture.record.State.SelectedLenses[0]
	payload := []byte(`{"findings":[{"id":"R1-001","location":"internal/billing/charge.go:42"}],"evidence":["read internal/billing/charge.go"]}`)
	digest, rawPath := preserveRawResult(t, fixture.repo, fixture.record.State.LineageID, lens, 0, payload)

	record, err := DisposeUnreplayablePreservedResult(context.Background(), fixture.repo,
		fixture.request(lens, 0, digest, ResultDispositionWrongTarget, []string{"internal/billing/charge.go"}))
	if err != nil {
		t.Fatalf("wrong-target disposition failed: %v", err)
	}
	if record.Disposition.Class != ResultDispositionWrongTarget || !record.Disposition.PayloadDecodable ||
		!equalStrings(record.Disposition.AbsentPaths, []string{"internal/billing/charge.go"}) {
		t.Fatalf("wrong-target disposition = %+v", record.Disposition)
	}
	raw, err := os.ReadFile(rawPath)
	if err != nil || !bytes.Equal(raw, payload) {
		t.Fatalf("preserved raw output changed: %v", err)
	}
}

// TestDisposeUnreplayablePreservedResultRefusesUnboundRequests covers
// requirements 1, 2, 4, and 5 together: every binding mismatch, every missing
// or unproven evidence claim, and revision drift are refused, the lineage
// stays reviewing, and the preserved bytes are never touched by a refusal.
func TestDisposeUnreplayablePreservedResultRefusesUnboundRequests(t *testing.T) {
	fixture := newDisposeFixture(t, "dispose-refusals")
	lenses := fixture.record.State.SelectedLenses
	digest, rawPath := preserveRawResult(t, fixture.repo, fixture.record.State.LineageID, lenses[3], 3, []byte(unreplayablePayload))
	valid := fixture.request(lenses[3], 3, digest, ResultDispositionTransportSyntax, nil)
	otherRevision := "sha256:" + strings.Repeat("ab", 32)

	// wrong_target evidence is only ever weighed against a payload that decoded,
	// so the wrong-target refusals below are driven from a second preserved
	// artifact that is valid JSON. Re-pointing the request at it also re-derives
	// the authorization, since the digest is part of the binding.
	decodableDigest, _ := preserveRawResult(t, fixture.repo, fixture.record.State.LineageID, lenses[3], 3,
		[]byte(`{"findings":[{"id":"R1-001","location":"internal/billing/charge.go:42"}],"evidence":["read internal/billing/charge.go"]}`))
	wrongTarget := func(request *CompactResultDispositionRequest, absent []string) {
		request.Class = ResultDispositionWrongTarget
		request.ArtifactDigest = decodableDigest
		request.AbsentPaths = absent
		request.MaintainerAuthorization = disposeAuthorization(fixture.root, request.LineageID, request.ExpectedRevision,
			request.TargetIdentity, request.Lens, request.SelectedOrder, decodableDigest,
			string(ResultDispositionWrongTarget), request.Actor, request.Reason)
	}

	cases := []struct {
		name    string
		mutate  func(request *CompactResultDispositionRequest)
		wantErr string
	}{
		{
			name: "revision drift",
			mutate: func(request *CompactResultDispositionRequest) {
				request.ExpectedRevision = otherRevision
				request.MaintainerAuthorization = disposeAuthorization(fixture.root, request.LineageID, otherRevision,
					request.TargetIdentity, request.Lens, request.SelectedOrder, digest, string(request.Class), request.Actor, request.Reason)
			},
			wantErr: "expected compact revision",
		},
		{
			name: "authorization bound to another revision",
			mutate: func(request *CompactResultDispositionRequest) {
				request.MaintainerAuthorization = disposeAuthorization(fixture.root, request.LineageID, otherRevision, request.TargetIdentity, request.Lens, request.SelectedOrder, digest, string(request.Class), request.Actor, request.Reason)
			},
			wantErr: "exact maintainer authorization binding",
		},
		{
			name:    "missing authorization",
			mutate:  func(request *CompactResultDispositionRequest) { request.MaintainerAuthorization = "  " },
			wantErr: "non-empty diagnostic, reason, actor, and maintainer authorization",
		},
		{
			name:    "missing diagnostic",
			mutate:  func(request *CompactResultDispositionRequest) { request.Diagnostic = "" },
			wantErr: "non-empty diagnostic, reason, actor, and maintainer authorization",
		},
		{
			name:    "wrong lens for the selected order",
			mutate:  func(request *CompactResultDispositionRequest) { request.Lens = lenses[0] },
			wantErr: "is not the selected lens at order",
		},
		{
			name:    "wrong selected order",
			mutate:  func(request *CompactResultDispositionRequest) { request.SelectedOrder = 1 },
			wantErr: "is not the selected lens at order",
		},
		{
			name:    "changed target identity",
			mutate:  func(request *CompactResultDispositionRequest) { request.TargetIdentity = otherRevision },
			wantErr: "is not the frozen target",
		},
		{
			name:    "digest that no preserved artifact matches",
			mutate:  func(request *CompactResultDispositionRequest) { request.ArtifactDigest = otherRevision },
			wantErr: "requires the preserved incident artifact",
		},
		{
			name:    "wrong-target claim without path evidence",
			mutate:  func(request *CompactResultDispositionRequest) { request.Class = ResultDispositionWrongTarget },
			wantErr: "requires at least one canonical repository-relative absent path",
		},
		{
			name: "wrong-target claim citing a frozen candidate path",
			mutate: func(request *CompactResultDispositionRequest) {
				wrongTarget(request, []string{"internal/auth/token.go"})
			},
			wantErr: "is inside the frozen candidate",
		},
		{
			name: "wrong-target claim the preserved output never makes",
			mutate: func(request *CompactResultDispositionRequest) {
				wrongTarget(request, []string{"internal/unrelated/file.go"})
			},
			wantErr: "does not appear in the preserved reviewer output",
		},
		{
			// The #1469 false-audit defect: an invalid-JSON payload could be
			// committed and permanently audited as the semantic wrong-target
			// claim, which it can never support because it never decoded.
			name: "wrong-target claim over a payload that never decoded",
			mutate: func(request *CompactResultDispositionRequest) {
				request.Class = ResultDispositionWrongTarget
				request.AbsentPaths = []string{"internal/billing/charge.go"}
				request.MaintainerAuthorization = disposeAuthorization(fixture.root, request.LineageID, request.ExpectedRevision,
					request.TargetIdentity, request.Lens, request.SelectedOrder, digest,
					string(ResultDispositionWrongTarget), request.Actor, request.Reason)
			},
			wantErr: "never decoded as JSON",
		},
		{
			name: "transport claim carrying wrong-target evidence",
			mutate: func(request *CompactResultDispositionRequest) {
				request.AbsentPaths = []string{"internal/billing/charge.go"}
			},
			wantErr: "carries no wrong-target path evidence",
		},
		{
			name:    "unsupported class",
			mutate:  func(request *CompactResultDispositionRequest) { request.Class = "unreviewable" },
			wantErr: "requires class",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := valid
			testCase.mutate(&request)
			_, err := DisposeUnreplayablePreservedResult(context.Background(), fixture.repo, request)
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, testCase.wantErr)
			}
			after, loadErr := fixture.store.Load()
			if loadErr != nil || after.Revision != fixture.record.Revision || after.State.State != StateReviewing {
				t.Fatalf("refused disposition mutated authority: %v", loadErr)
			}
			raw, readErr := os.ReadFile(rawPath)
			if readErr != nil || !bytes.Equal(raw, []byte(unreplayablePayload)) {
				t.Fatalf("refused disposition touched the preserved output: %v", readErr)
			}
		})
	}
	if _, err := DisposeUnreplayablePreservedResult(context.Background(), fixture.repo, valid); err != nil {
		t.Fatalf("the exact bound request must still succeed after every refusal: %v", err)
	}
}

// TestDisposeUnreplayablePreservedResultRefusesDecodablePayload keeps ordinary
// capture untouched (requirement 8): a payload that is valid JSON is never
// routed through the transport/syntax disposition, so the operation cannot be
// used to sidestep review capture-result.
func TestDisposeUnreplayablePreservedResultRefusesDecodablePayload(t *testing.T) {
	fixture := newDisposeFixture(t, "dispose-decodable")
	lens := fixture.record.State.SelectedLenses[0]
	digest, _ := preserveRawResult(t, fixture.repo, fixture.record.State.LineageID, lens, 0,
		[]byte(`{"findings":[],"evidence":["checked exact target"]}`))
	_, err := DisposeUnreplayablePreservedResult(context.Background(), fixture.repo,
		fixture.request(lens, 0, digest, ResultDispositionTransportSyntax, nil))
	if err == nil || !strings.Contains(err.Error(), "syntactically valid JSON") {
		t.Fatalf("error = %v, want a refusal that redirects to capture-result", err)
	}
}

// TestDisposeUnreplayablePreservedResultRefusesCapturedLens forbids using the
// disposition to shadow a lens whose valid result was already captured.
func TestDisposeUnreplayablePreservedResultRefusesCapturedLens(t *testing.T) {
	fixture := newDisposeFixture(t, "dispose-captured-lens")
	lens := fixture.record.State.SelectedLenses[0]
	captureValidResult(t, fixture.store, lens, 0)
	digest, _ := preserveRawResult(t, fixture.repo, fixture.record.State.LineageID, lens, 0, []byte(unreplayablePayload))
	_, err := DisposeUnreplayablePreservedResult(context.Background(), fixture.repo,
		fixture.request(lens, 0, digest, ResultDispositionTransportSyntax, nil))
	if err == nil || !strings.Contains(err.Error(), "already holds a captured reviewer result") {
		t.Fatalf("error = %v, want a refusal to disposition a captured lens", err)
	}
}

// TestDisposeUnreplayablePreservedResultRefusesCaptureRacingTheCommit closes
// the #1469 disposition/capture race. capture-result publishes its artifact
// without mutating the compact authority state, so it never moves the revision
// the disposition compare-and-swaps on: a capture that lands after the
// disposition's early already-captured scan is invisible to the CAS, and the
// lineage would be terminally escalated over a lens that now holds a valid
// captured result, orphaning that work.
//
// The window is opened deterministically by holding the exclusive maintenance
// lease the commit path must take before the store lock. The disposition
// therefore completes its early scan, blocks, and only reaches its locked
// already-captured re-scan after the capture has landed.
func TestDisposeUnreplayablePreservedResultRefusesCaptureRacingTheCommit(t *testing.T) {
	fixture := newDisposeFixture(t, "dispose-capture-race")
	lens := fixture.record.State.SelectedLenses[3]
	digest, rawPath := preserveRawResult(t, fixture.repo, fixture.record.State.LineageID, lens, 3, []byte(unreplayablePayload))
	request := fixture.request(lens, 3, digest, ResultDispositionTransportSyntax, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	maintenance, err := AcquireReviewMaintenanceExclusive(ctx, fixture.repo)
	if err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		record CompactResultDispositionRecord
		err    error
	}
	results := make(chan outcome, 1)
	go func() {
		record, disposeErr := DisposeUnreplayablePreservedResult(context.Background(), fixture.repo, request)
		results <- outcome{record: record, err: disposeErr}
	}()

	// The disposition's early scan runs before the commit path takes the
	// maintenance lease, so by now it has already observed an empty
	// reviewer-results directory and is blocked ahead of its locked re-scan.
	time.Sleep(100 * time.Millisecond)
	capturedPath, capturedPayload := captureValidResult(t, fixture.store, lens, 3)
	if err := maintenance.Release(); err != nil {
		t.Fatal(err)
	}

	got := <-results
	if got.err == nil || !strings.Contains(got.err.Error(), "already holds a captured reviewer result") {
		t.Fatalf("error = %v, want the disposition refused for a lens captured during the commit window", got.err)
	}
	after, err := fixture.store.Load()
	if err != nil || after.Revision != fixture.record.Revision || after.State.State != StateReviewing ||
		len(after.State.ResultDispositions) != 0 {
		t.Fatalf("racing capture was escalated over: revision %s state %q dispositions %+v (%v)",
			after.Revision, after.State.State, after.State.ResultDispositions, err)
	}
	// The capture that won the race keeps its artifact byte-for-byte.
	if raced, readErr := os.ReadFile(capturedPath); readErr != nil || !bytes.Equal(raced, capturedPayload) {
		t.Fatalf("captured reviewer result changed: %v", readErr)
	}
	if raw, readErr := os.ReadFile(rawPath); readErr != nil || !bytes.Equal(raw, []byte(unreplayablePayload)) {
		t.Fatalf("refused disposition touched the preserved output: %v", readErr)
	}
}

// TestDisposeUnreplayablePreservedResultReplayConverges pins requirement 5's
// idempotency clause: repeating the exact committed request neither applies a
// second disposition nor fails.
func TestDisposeUnreplayablePreservedResultReplayConverges(t *testing.T) {
	fixture := newDisposeFixture(t, "dispose-replay")
	lens := fixture.record.State.SelectedLenses[2]
	digest, _ := preserveRawResult(t, fixture.repo, fixture.record.State.LineageID, lens, 2, []byte(unreplayablePayload))
	request := fixture.request(lens, 2, digest, ResultDispositionTransportSyntax, nil)

	first, err := DisposeUnreplayablePreservedResult(context.Background(), fixture.repo, request)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := DisposeUnreplayablePreservedResult(context.Background(), fixture.repo, request)
	if err != nil {
		t.Fatalf("exact replay of a committed disposition failed: %v", err)
	}
	if !replay.Replayed || replay.Revision != first.Revision || !reflect.DeepEqual(replay.Disposition, first.Disposition) {
		t.Fatalf("replay = %+v, want convergence on %+v", replay, first)
	}
	after, err := fixture.store.Load()
	if err != nil || len(after.State.ResultDispositions) != 1 || after.Revision != first.Revision {
		t.Fatalf("replay double-applied the disposition: %+v (%v)", after.State.ResultDispositions, err)
	}

	// A different request against the already escalated lineage is refused
	// rather than silently converging.
	other := request
	other.Reason = "a different reason"
	other.MaintainerAuthorization = disposeAuthorization(fixture.root, other.LineageID, other.ExpectedRevision,
		other.TargetIdentity, other.Lens, other.SelectedOrder, digest, string(other.Class), other.Actor, other.Reason)
	if _, err := DisposeUnreplayablePreservedResult(context.Background(), fixture.repo, other); err == nil ||
		!strings.Contains(err.Error(), "already terminally escalated") {
		t.Fatalf("error = %v, want a refusal for a non-matching request", err)
	}
}

// TestDisposeResultSuccessorNeverAdmitsOrRewritesReviewState locks the state
// machine edge itself: reviewing -> escalated is the only legal shape, and the
// transition may not smuggle in lens results or any other authority change.
func TestDisposeResultSuccessorNeverAdmitsOrRewritesReviewState(t *testing.T) {
	fixture := newDisposeFixture(t, "dispose-successor")
	previous := fixture.record.State
	disposition := CompactResultDisposition{
		Lens: previous.SelectedLenses[0], SelectedOrder: 0, TargetIdentity: previous.InitialSnapshot.Identity,
		ArtifactDigest: "sha256:" + strings.Repeat("cd", 32), Class: ResultDispositionTransportSyntax,
		Diagnostic: "invalid character after array element", Reason: "unreplayable", Actor: "maintainer@example.com",
		DisposedAt: time.Unix(1700000000, 0).UTC(), MaintainerAuthorization: "binding",
	}
	legal := previous
	legal.State = StateEscalated
	legal.ResultDispositions = []CompactResultDisposition{disposition}
	if err := validateCompactSuccessor(previous, legal, CompactResultDispositionOperation); err != nil {
		t.Fatalf("reviewing -> escalated disposition edge rejected: %v", err)
	}

	admitted := legal
	admitted.LensResults = []LensResult{{Lens: previous.SelectedLenses[0], Findings: []Finding{}, Evidence: []string{"fabricated"}}}
	if err := validateCompactSuccessor(previous, admitted, CompactResultDispositionOperation); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("disposition admitting a lens result = %v", err)
	}

	approved := legal
	approved.State = StateApproved
	if err := validateCompactSuccessor(previous, approved, CompactResultDispositionOperation); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("disposition approving the lineage = %v", err)
	}

	none := previous
	none.State = StateEscalated
	if err := validateCompactSuccessor(previous, none, CompactResultDispositionOperation); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("escalation without a recorded disposition = %v", err)
	}

	terminal := legal
	terminal.State = StateEscalated
	if err := validateCompactSuccessor(legal, terminal, CompactResultDispositionOperation); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("disposition from a non-reviewing predecessor = %v", err)
	}
}

// TestCompactResultDispositionStateShapeIsValidated keeps a disposition from
// ever being persisted outside a terminal escalation or against a lens, order,
// target, or path evidence it does not actually bind.
func TestCompactResultDispositionStateShapeIsValidated(t *testing.T) {
	fixture := newDisposeFixture(t, "dispose-state-shape")
	base := fixture.record.State
	base.State = StateEscalated
	disposition := CompactResultDisposition{
		Lens: base.SelectedLenses[1], SelectedOrder: 1, TargetIdentity: base.InitialSnapshot.Identity,
		ArtifactDigest: "sha256:" + strings.Repeat("ef", 32), Class: ResultDispositionTransportSyntax,
		Diagnostic: "invalid character", Reason: "unreplayable", Actor: "maintainer@example.com",
		DisposedAt: time.Unix(1700000000, 0).UTC(), MaintainerAuthorization: "binding",
	}
	base.ResultDispositions = []CompactResultDisposition{disposition}
	if err := base.Validate(); err != nil {
		t.Fatalf("well-formed escalated disposition state rejected: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(state *CompactState)
		wantErr string
	}{
		{name: "not terminally escalated", mutate: func(state *CompactState) { state.State = StateReviewing }, wantErr: "only a terminally escalated compact state"},
		{name: "lens does not match order", mutate: func(state *CompactState) { state.ResultDispositions[0].Lens = state.SelectedLenses[0] }, wantErr: "does not bind a selected lens and order"},
		{name: "order out of range", mutate: func(state *CompactState) { state.ResultDispositions[0].SelectedOrder = 9 }, wantErr: "does not bind a selected lens and order"},
		{name: "target is not the frozen target", mutate: func(state *CompactState) {
			state.ResultDispositions[0].TargetIdentity = "sha256:" + strings.Repeat("11", 32)
		}, wantErr: "does not bind the frozen target"},
		{name: "digest is not a sha256 identity", mutate: func(state *CompactState) { state.ResultDispositions[0].ArtifactDigest = "malformed" }, wantErr: "does not bind the frozen target"},
		{name: "missing authorization", mutate: func(state *CompactState) { state.ResultDispositions[0].MaintainerAuthorization = "" }, wantErr: "requires a diagnostic, reason, actor, authorization, and timestamp"},
		{name: "transport class with path evidence", mutate: func(state *CompactState) {
			state.ResultDispositions[0].AbsentPaths = []string{"internal/billing/charge.go"}
		}, wantErr: "carries no wrong-target path evidence"},
		{name: "wrong-target class without path evidence", mutate: func(state *CompactState) { state.ResultDispositions[0].Class = ResultDispositionWrongTarget }, wantErr: "requires canonical absent-path evidence"},
		{
			name: "wrong-target class citing a frozen candidate path",
			mutate: func(state *CompactState) {
				state.ResultDispositions[0].Class = ResultDispositionWrongTarget
				state.ResultDispositions[0].AbsentPaths = []string{"internal/auth/token.go"}
			},
			wantErr: "cites a path inside the frozen candidate",
		},
		{
			// A persisted state may not carry the semantic wrong-target claim
			// over a payload it recorded as never having decoded.
			name: "wrong-target class over a payload that never decoded",
			mutate: func(state *CompactState) {
				state.ResultDispositions[0].Class = ResultDispositionWrongTarget
				state.ResultDispositions[0].AbsentPaths = []string{"internal/billing/charge.go"}
				state.ResultDispositions[0].PayloadDecodable = false
			},
			wantErr: "must record a payload that actually decoded",
		},
		{
			name: "transport/syntax class over a payload that decoded",
			mutate: func(state *CompactState) {
				state.ResultDispositions[0].PayloadDecodable = true
			},
			wantErr: "must record a payload that did not decode",
		},
		{name: "unknown class", mutate: func(state *CompactState) { state.ResultDispositions[0].Class = "unreviewable" }, wantErr: "invalid reviewer result disposition class"},
		{
			name: "duplicate order",
			mutate: func(state *CompactState) {
				state.ResultDispositions = append(state.ResultDispositions, state.ResultDispositions[0])
			},
			wantErr: "recorded twice",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			state := base
			state.ResultDispositions = append([]CompactResultDisposition{}, disposition)
			testCase.mutate(&state)
			err := state.Validate()
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, testCase.wantErr)
			}
		})
	}
}

// TestDispositionLensResultExemptionStaysNarrow guards the one invariant this
// feature relaxes: a lineage escalated by disposition legitimately has no lens
// results, but that exemption must never excuse a partially completed review
// or a disposition that smuggles frozen review content alongside it.
func TestDispositionLensResultExemptionStaysNarrow(t *testing.T) {
	fixture := newDisposeFixture(t, "dispose-exemption")
	base := fixture.record.State
	base.State = StateEscalated

	// Without a disposition, an escalated lineage still owes every lens result.
	if err := base.Validate(); err == nil || !strings.Contains(err.Error(), "requires every selected lens result") {
		t.Fatalf("escalated state without dispositions = %v", err)
	}

	disposed := base
	disposed.ResultDispositions = []CompactResultDisposition{{
		Lens: base.SelectedLenses[0], SelectedOrder: 0, TargetIdentity: base.InitialSnapshot.Identity,
		ArtifactDigest: "sha256:" + strings.Repeat("ef", 32), Class: ResultDispositionTransportSyntax,
		Diagnostic: "invalid character", Reason: "unreplayable", Actor: "maintainer@example.com",
		DisposedAt: time.Unix(1700000000, 0).UTC(), MaintainerAuthorization: "binding",
	}}
	if err := disposed.Validate(); err != nil {
		t.Fatalf("dispositioned escalation rejected: %v", err)
	}

	// The exemption is void the moment any review content is frozen.
	canonical, err := CanonicalCompactLensResult(LensResult{Lens: base.SelectedLenses[0], Findings: []Finding{}, Evidence: []string{"partial"}})
	if err != nil {
		t.Fatal(err)
	}
	partial := disposed
	partial.LensResults = []LensResult{canonical}
	if err := partial.Validate(); err == nil || !strings.Contains(err.Error(), "must hold no frozen review content") {
		t.Fatalf("dispositioned state carrying a lens result = %v", err)
	}
	graded := disposed
	graded.EvidenceHash = "sha256:" + strings.Repeat("aa", 32)
	if err := graded.Validate(); err == nil || !strings.Contains(err.Error(), "must hold no frozen review content") {
		t.Fatalf("dispositioned state carrying verification evidence = %v", err)
	}
}

// TestDisposedLineageIsNoLongerPristine keeps the abandon surface honest: an
// escalated lineage carrying an audited disposition is history, never a
// pristine entry that could be quarantined away.
func TestDisposedLineageIsNoLongerPristine(t *testing.T) {
	fixture := newDisposeFixture(t, "dispose-not-pristine")
	state := fixture.record.State
	if !compactPristineReviewing(state) {
		t.Fatal("fixture is not a pristine reviewing lineage")
	}
	state.ResultDispositions = []CompactResultDisposition{{Lens: state.SelectedLenses[0]}}
	if compactPristineReviewing(state) {
		t.Fatal("a lineage carrying a reviewer result disposition must not read as pristine")
	}
}
