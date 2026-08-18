package cli

// The damaged-store friction benchmark (bench/journeys ds01–ds05) measured
// the delivery gate answering three different damages — a forged
// authorization binding, a dangling predecessor, a record truncated mid-write
// — with one identical sentence: "complete review authority inventory is
// unavailable or corrupted". The operator could not distinguish the damages
// while inspect-authority already held the exact per-entry diagnosis. These
// tests pin the honest denial: it names the KIND of damage and the runnable
// diagnosis, and the named diagnosis is then dispatched through the real CLI
// router and required to answer with exactly the damage the denial claimed.
// The gate still refuses — naming the kind plus the diagnosis is the whole
// scope; it never guesses repairs.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// mintDamagedStoreRecoveryPair builds, through the CLI alone, one approved
// authority and one recovery successor over a widened docs scope — the same
// healthy store the bench damaged-store fixtures then damage.
func mintDamagedStoreRecoveryPair(t *testing.T, repo string) (predecessorID, successorID string) {
	t.Helper()
	writeDamagedStoreProse(t, repo, "intro")
	finalizeFacadeReviewForRepo(t, repo)
	leaves, err := reviewtransaction.CompactAuthorityLeaves(t.Context(), repo)
	if err != nil || len(leaves) != 1 {
		t.Fatalf("approved predecessor leaves = %#v, %v", leaves, err)
	}
	predecessor, err := leaves[0].Load()
	if err != nil {
		t.Fatal(err)
	}
	writeDamagedStoreProse(t, repo, "widened")
	const successor = "review-damaged-successor"
	if err := RunReviewRecover([]string{"--cwd", repo,
		"--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", successor,
		"--disposition", "scope_changed"}, io.Discard); err != nil {
		t.Fatalf("recover successor: %v", err)
	}
	return predecessor.State.LineageID, successor
}

func writeDamagedStoreProse(t *testing.T, repo, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", name+".md"), []byte("# "+name+"\n\nplain prose, no executable content.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "-A")
}

func damagedStoreLineageDir(t *testing.T, repo, lineage string) string {
	t.Helper()
	return filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2", lineage)
}

// forgeDamagedStoreRecoveryReason edits the recorded recovery reason after the
// fact and recomputes the revision over the edited state, exactly as the
// bench's ds01/ds02 fixtures author the forged-binding shape: the maintainer
// authorization keeps its correct schema prefix and now binds content the
// record no longer holds.
func forgeDamagedStoreRecoveryReason(t *testing.T, repo, lineage string) {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.Recovery == nil {
		t.Fatalf("lineage %q holds no recovery provenance to damage", lineage)
	}
	record.State.Recovery.Reason += " (edited after the fact)"
	revision, err := reviewtransaction.CompactRevisionForState(record.State)
	if err != nil {
		t.Fatal(err)
	}
	record.Revision = revision
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), append(payload, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func truncateDamagedStoreRecord(t *testing.T, repo, lineage string) {
	t.Helper()
	path := filepath.Join(damagedStoreLineageDir(t, repo, lineage), "review-state.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload[:len(payload)/2], 0o644); err != nil {
		t.Fatal(err)
	}
}

// damagedStoreInspection is the subset of the inspect-authority envelope these
// tests read back after dispatching the named diagnosis.
type damagedStoreInspection struct {
	Complete bool `json:"complete"`
	Valid    bool `json:"valid"`
	Edges    []struct {
		SuccessorLineageID    string   `json:"successor_lineage_id"`
		Valid                 bool     `json:"valid"`
		Problems              []string `json:"problems"`
		NonReconcilableReason string   `json:"non_reconcilable_reason"`
	} `json:"edges"`
	EntryDiagnostics []struct {
		LineageID string `json:"lineage_id"`
		Problem   string `json:"problem"`
	} `json:"entry_diagnostics"`
}

// liftBacktickedReviewCommand lifts one backtick-quoted runnable
// `gentle-ai review <verb> ...` command out of a refusal, mirroring what an
// operator would copy. Tokens stop at the first unfilled <placeholder>; a
// flag left dangling by that cut is dropped so the operator-owned values can
// be appended in its place.
func liftBacktickedReviewCommand(t *testing.T, message, verb string) []string {
	t.Helper()
	longest := []string{}
	for _, match := range regexp.MustCompile("`gentle-ai ([^`]*)`").FindAllStringSubmatch(message, -1) {
		tokens := []string{}
		for _, token := range strings.Fields(match[1]) {
			token = strings.Trim(token, "'\"")
			if strings.HasPrefix(token, "<") {
				break
			}
			tokens = append(tokens, token)
		}
		if len(tokens) > 0 && strings.HasPrefix(tokens[len(tokens)-1], "--") {
			tokens = tokens[:len(tokens)-1]
		}
		// The longest match, not the first: a refusal often mentions the verb
		// in prose before rendering it in full, and an operator copies the
		// one with the arguments in it.
		if len(tokens) >= 2 && tokens[0] == "review" && tokens[1] == verb && len(tokens) > len(longest) {
			longest = tokens
		}
	}
	if len(longest) > 0 {
		return longest
	}
	t.Fatalf("message names no runnable `gentle-ai review %s ...`: %q", verb, message)
	return nil
}

// dispatchNamedDiagnosisInRepo is dispatchNamedDiagnosis for a refusal that
// names the diagnosis without a --cwd, which is how an operator reads it: they
// are standing in the repository the refusal came from. The command is lifted
// from the message verbatim and only that standing position is supplied.
func dispatchNamedDiagnosisInRepo(t *testing.T, repo, message string) damagedStoreInspection {
	t.Helper()
	tokens := liftBacktickedReviewCommand(t, message, "inspect-authority")
	var output bytes.Buffer
	args := append(append([]string{}, tokens[1:]...), "--cwd", repo)
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("the diagnosis the refusal named exits non-zero (named dead end): gentle-ai %s: %v\n%s",
			strings.Join(args, " "), err, output.String())
	}
	var report damagedStoreInspection
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("diagnosis output is not an inspection envelope: %v\n%s", err, output.String())
	}
	return report
}

// dispatchNamedDiagnosis lifts the named runnable inspection out of the
// denial, dispatches it through the real CLI router, and returns the parsed
// report.
func dispatchNamedDiagnosis(t *testing.T, repo, message string) damagedStoreInspection {
	t.Helper()
	tokens := liftBacktickedReviewCommand(t, message, "inspect-authority")
	var output bytes.Buffer
	if err := RunReview(tokens[1:], &output); err != nil {
		t.Fatalf("the diagnosis the denial named exits non-zero (named dead end): gentle-ai %s: %v\n%s",
			strings.Join(tokens, " "), err, output.String())
	}
	var report damagedStoreInspection
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("diagnosis output is not an inspection envelope: %v\n%s", err, output.String())
	}
	return report
}

// TestDamagedEntryNamesItsOwnDamageKindAndDiagnosis drives the three ds damage
// shapes and requires two things of each.
//
// The delivery gate for an UNRELATED live candidate never inherits the damage:
// the generic "complete review authority inventory is unavailable or
// corrupted" sentence that the ds01-ds05 benchmark measured was a verdict
// about a repository, issued to an operator working on one file.
//
// The damaged entry itself still names its own kind of damage plus a runnable
// diagnosis, and that diagnosis is dispatched through the real router and
// required to answer with exactly the damage claimed. Naming the kind is what
// the benchmark found missing; scoping the verdict is what makes the naming
// reach the right person.
func TestDamagedEntryNamesItsOwnDamageKindAndDiagnosis(t *testing.T) {
	const forgedKind = "binding bound to different content"
	// The inspection's own vocabulary, not the retired gate sentence's: the
	// diagnosis is the surface that names the damage now, so its words are
	// the ones that have to be distinct per shape.
	const danglingKind = "missing predecessor"
	const truncatedKind = "malformed_compact_state"

	tests := []struct {
		name        string
		stage       func(t *testing.T, repo string)
		wantKind    string
		absentKinds []string
		verify      func(t *testing.T, report damagedStoreInspection)
	}{
		{
			name: "forged authorization binding",
			stage: func(t *testing.T, repo string) {
				_, successor := mintDamagedStoreRecoveryPair(t, repo)
				forgeDamagedStoreRecoveryReason(t, repo, successor)
			},
			wantKind:    forgedKind,
			absentKinds: []string{danglingKind, truncatedKind},
			verify: func(t *testing.T, report damagedStoreInspection) {
				if len(report.Edges) != 1 || report.Edges[0].Valid || !strings.Contains(report.Edges[0].NonReconcilableReason, forgedKind) {
					t.Fatalf("the named diagnosis does not answer with the claimed damage: %#v", report)
				}
			},
		},
		{
			name: "dangling predecessor",
			stage: func(t *testing.T, repo string) {
				predecessor, _ := mintDamagedStoreRecoveryPair(t, repo)
				if err := os.RemoveAll(damagedStoreLineageDir(t, repo, predecessor)); err != nil {
					t.Fatal(err)
				}
			},
			wantKind:    danglingKind,
			absentKinds: []string{forgedKind, truncatedKind},
			verify: func(t *testing.T, report damagedStoreInspection) {
				if len(report.Edges) != 1 || report.Edges[0].Valid ||
					!anyDamagedStoreProblemContains(report.Edges[0].Problems, "missing predecessor") {
					t.Fatalf("the named diagnosis does not answer with the claimed damage: %#v", report)
				}
			},
		},
		{
			name: "record truncated mid-write",
			stage: func(t *testing.T, repo string) {
				_, successor := mintDamagedStoreRecoveryPair(t, repo)
				truncateDamagedStoreRecord(t, repo, successor)
			},
			wantKind:    truncatedKind,
			absentKinds: []string{forgedKind, danglingKind},
			verify: func(t *testing.T, report damagedStoreInspection) {
				if report.Complete || len(report.EntryDiagnostics) != 1 || report.EntryDiagnostics[0].Problem != truncatedKind {
					t.Fatalf("the named diagnosis does not answer with the claimed damage: %#v", report)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			test.stage(t, repo)

			// The unrelated live candidate is answered on its own terms.
			var output bytes.Buffer
			runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "post-apply"}, &output)
			if runErr != nil {
				if strings.Contains(runErr.Error(), "unavailable or corrupted") {
					t.Fatalf("the live candidate inherited a repository-wide corruption verdict:\n%v", runErr)
				}
				for _, kind := range append([]string{test.wantKind}, test.absentKinds...) {
					if strings.Contains(runErr.Error(), kind) {
						t.Fatalf("the live candidate's denial borrowed a damage kind from history (%q):\n%v", kind, runErr)
					}
				}
			}

			// The damaged entry names its own damage, and the diagnosis it
			// names answers.
			blocked := reviewtransaction.CompactAuthorityLineageBlocked(t.Context(), repo, damagedStoreSuccessorLineage)
			if blocked == nil {
				blocked = damagedStoreEntryProblem(t, repo, damagedStoreSuccessorLineage)
			}
			message := blocked.Error()
			if !strings.Contains(message, damagedStoreSuccessorLineage) {
				t.Fatalf("the block does not name the entry it refuses:\n%s", message)
			}
			if !strings.Contains(message, "gentle-ai review inspect-authority") {
				t.Fatalf("the block names no runnable diagnosis:\n%s", message)
			}
			report := dispatchNamedDiagnosisInRepo(t, repo, message)
			test.verify(t, report)
			named := damagedStoreDiagnosisText(t, report)
			if !strings.Contains(named, test.wantKind) {
				t.Fatalf("the diagnosis does not name the kind of damage (%q):\n%s", test.wantKind, named)
			}
			for _, absent := range test.absentKinds {
				if strings.Contains(named, absent) {
					t.Fatalf("the diagnosis claims a damage kind this store does not hold (%q):\n%s", absent, named)
				}
			}
		})
	}
}

const damagedStoreSuccessorLineage = "review-damaged-successor"

// damagedStoreEntryProblem falls back to the inventory entry's own problem for
// a shape the graph never classifies -- an unreadable record never becomes an
// edge, so it carries no graph violation, only its own decode failure.
func damagedStoreEntryProblem(t *testing.T, repo, lineage string) error {
	t.Helper()
	report, err := reviewtransaction.InventoryAuthority(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range report.Entries {
		if entry.LineageID == lineage && len(entry.Problems) > 0 {
			return errors.New(strings.Join(entry.Problems, "; "))
		}
	}
	t.Fatalf("no entry or block names %q: %#v", lineage, report.Entries)
	return nil
}

// damagedStoreDiagnosisText flattens everything the inspection says about the
// damage, so a per-shape claim is matched against the product's own words.
func damagedStoreDiagnosisText(t *testing.T, report damagedStoreInspection) string {
	t.Helper()
	parts := []string{}
	for _, edge := range report.Edges {
		parts = append(parts, edge.NonReconcilableReason)
		parts = append(parts, edge.Problems...)
	}
	for _, diagnostic := range report.EntryDiagnostics {
		parts = append(parts, diagnostic.Problem)
	}
	return strings.Join(parts, "\n")
}

func anyDamagedStoreProblemContains(problems []string, fragment string) bool {
	for _, problem := range problems {
		if strings.Contains(problem, fragment) {
			return true
		}
	}
	return false
}

// TestOrphanedSuccessorHasARunnableAbandonThatDoesNotBlockNewWork is the CLI
// half of the ds04 measurement, after the scope rule.
//
// `review start` over a graph whose successor names a missing predecessor
// refused, and the orphaned successor's abandonment was the way out -- for
// everybody, whether or not they had anything to do with that successor. New
// work now starts; the orphaned successor is still an orphan, still refuses
// for itself, and its abandonment is still rendered as a runnable command.
// This test lifts that command and its authorization template values out of
// the refusal, supplies only the operator-owned actor and reason, dispatches
// through the real router, and requires the entry to be gone afterwards.
func TestOrphanedSuccessorHasARunnableAbandonThatDoesNotBlockNewWork(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	predecessor, successor := mintDamagedStoreRecoveryPair(t, repo)
	if err := os.RemoveAll(damagedStoreLineageDir(t, repo, predecessor)); err != nil {
		t.Fatal(err)
	}
	writeDamagedStoreProse(t, repo, "fresh")

	// New work is not the orphan's business.
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo}, &output); err != nil {
		t.Fatalf("an orphaned successor blocked a fresh unrelated start: %v\n%s", err, output.String())
	}

	// The orphan is still an orphan, and says so.
	blocked := reviewtransaction.CompactAuthorityLineageBlocked(t.Context(), repo, successor)
	if blocked == nil || !strings.Contains(blocked.Error(), "dangling predecessor") {
		t.Fatalf("the orphaned successor does not name its own graph defect: %v", blocked)
	}

	// Its abandonment is still rendered runnable, by the surface an operator
	// reaches when they act on that entry.
	reclaimErr := RunReviewReclaim([]string{"--cwd", repo, "--lineage", successor,
		"--reason", "its predecessor is gone", "--actor", "maintainer@example.com"}, io.Discard)
	if reclaimErr == nil {
		t.Fatal("reclaim accepted an entry that holds authoritative state")
	}
	message := reclaimErr.Error()

	tokens := liftBacktickedReviewCommand(t, message, "abandon")
	// The template block in the message carries the persisted binding values;
	// the operator supplies only actor and reason, exactly as the template
	// instructs.
	expectedRevision := regexp.MustCompile(`\nrevision=(\S+)`).FindStringSubmatch(message)
	snapshotIdentity := regexp.MustCompile(`\nsnapshot_identity=(\S+)`).FindStringSubmatch(message)
	if expectedRevision == nil || snapshotIdentity == nil {
		t.Fatalf("the refusal renders no authorization template: %q", message)
	}
	const actor = "maintainer@example.com"
	const reason = reviewtransaction.CompactAbandonReasonOperatorDisposition
	report, err := reviewtransaction.InventoryAuthority(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	var discarded *reviewtransaction.CompactDiscardedWorkSummary
	for _, entry := range report.Entries {
		if entry.LineageID == successor {
			discarded = entry.DiscardedWork
			break
		}
	}
	if discarded == nil {
		t.Fatalf("review status publishes no discarded-work summary for %q", successor)
	}
	authorization := reviewtransaction.RenderCompactAbandonAuthorization(
		successor, expectedRevision[1], snapshotIdentity[1], actor, reason, *discarded)
	abandonArgs := append(tokens[1:], "--reason", reason, "--actor", actor, "--maintainer-authorization", authorization)
	var abandoned bytes.Buffer
	if err := RunReview(abandonArgs, &abandoned); err != nil {
		t.Fatalf("the abandonment the refusal named exits non-zero (named dead end): %v\n%s", err, abandoned.String())
	}

	if reviewtransaction.CompactAuthorityLineageBlocked(t.Context(), repo, successor) != nil {
		t.Fatal("the abandonment the refusal named left the orphan in place")
	}
	output.Reset()
	if err := RunReviewFacadeStart([]string{"--cwd", repo}, &output); err != nil {
		t.Fatalf("start refuses after the named exit ran: %v\n%s", err, output.String())
	}
}

// TestReclaimRefusalOverTruncatedRecordNamesDiagnosisNotReconcile is the CLI
// half of the ds05 measurement: reclaim refused the half-written record by
// pointing at reconcile-authority, and reconcile then answered `unexpected
// EOF`. The refusal now carries the precise diagnosis and the runnable
// inspection, which is dispatched and required to answer.
func TestReclaimRefusalOverTruncatedRecordNamesDiagnosisNotReconcile(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	_, successor := mintDamagedStoreRecoveryPair(t, repo)
	truncateDamagedStoreRecord(t, repo, successor)

	err := RunReviewReclaim([]string{"--cwd", repo, "--lineage", successor,
		"--reason", "the record is half written", "--actor", "maintainer@example.com"}, io.Discard)
	if err == nil {
		t.Fatal("reclaim accepted a store entry holding an unreadable record")
	}
	message := err.Error()
	if !strings.Contains(message, "malformed_compact_state") {
		t.Fatalf("reclaim refusal does not carry the inspection's classification: %q", message)
	}
	if strings.Contains(message, "gentle-ai review reconcile-authority") {
		t.Fatalf("reclaim still names reconcile for a record reconcile cannot load (named dead end): %q", message)
	}
	report := dispatchNamedDiagnosis(t, repo, message)
	if report.Complete || len(report.EntryDiagnostics) != 1 ||
		report.EntryDiagnostics[0].LineageID != successor ||
		report.EntryDiagnostics[0].Problem != "malformed_compact_state" {
		t.Fatalf("the named diagnosis does not answer with the claimed damage: %#v", report)
	}
}

func TestNegotiatedFinalizeClassifiesNamedDamagedAuthorization(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	_, successor := mintDamagedStoreRecoveryPair(t, repo)
	forgeDamagedStoreRecoveryReason(t, repo, successor)
	statePath := filepath.Join(damagedStoreLineageDir(t, repo, successor), "review-state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	runErr := RunReview([]string{"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", successor, "--captured-results"}, &output)
	if runErr == nil {
		t.Fatal("negotiated finalize accepted a named damaged authorization")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Operation != ReviewIntegrationOperationFinalize || failure.Code != "escalated_recovery_authorization_inexact" ||
		failure.Phase != "pre_native" || failure.MutationOutcome != ReviewMutationNotStarted ||
		failure.NextAction != "review.repair" || failure.RetrySafe ||
		failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired ||
		strings.Contains(output.String(), "operation_outcome_unknown") || strings.Contains(output.String(), "reconcile-authority") {
		t.Fatalf("named damaged authorization failure = %#v\n%s", failure, output.String())
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("typed pre-native finalize changed the damaged authority bytes")
	}
}
