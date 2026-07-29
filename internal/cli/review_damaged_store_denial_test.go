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
		if len(tokens) >= 2 && tokens[0] == "review" && tokens[1] == verb {
			return tokens
		}
	}
	t.Fatalf("message names no runnable `gentle-ai review %s ...`: %q", verb, message)
	return nil
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

// TestGateDenialOverDamagedStoreNamesDamageKindAndDiagnosis drives the
// post-apply gate over the three ds damage shapes and requires each denial to
// name its own kind of damage — not the shared generic sentence alone — plus
// the runnable diagnosis, which is then dispatched and required to answer.
func TestGateDenialOverDamagedStoreNamesDamageKindAndDiagnosis(t *testing.T) {
	const forgedKind = "binding bound to different content"
	const danglingKind = "missing from the store"
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
			reviewModeHome(t)
			repo := initReviewCLIRepo(t)
			test.stage(t, repo)

			var output bytes.Buffer
			runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "post-apply"}, &output)
			if runErr == nil {
				t.Fatalf("post-apply allowed a damaged store: %s", output.String())
			}
			var gateErr ReviewGateDeniedError
			if !errors.As(runErr, &gateErr) {
				t.Fatalf("post-apply denial = %T %v, want ReviewGateDeniedError", runErr, runErr)
			}
			message := gateErr.Error()
			if !strings.Contains(message, test.wantKind) {
				t.Fatalf("gate denial does not name the kind of damage (%q):\n%s", test.wantKind, message)
			}
			for _, absent := range test.absentKinds {
				if strings.Contains(message, absent) {
					t.Fatalf("gate denial claims a damage kind this store does not hold (%q):\n%s", absent, message)
				}
			}
			test.verify(t, dispatchNamedDiagnosis(t, repo, message))
		})
	}
}

func anyDamagedStoreProblemContains(problems []string, fragment string) bool {
	for _, problem := range problems {
		if strings.Contains(problem, fragment) {
			return true
		}
	}
	return false
}

// TestStartRefusalOverDanglingPredecessorNamesRunnableAbandon is the CLI half
// of the ds04 measurement: `review start` over a graph whose successor names a
// missing predecessor refused with the bare graph violation and named
// nothing, while abandoning the orphaned successor provably clears it. The
// refusal now renders that abandonment; this test lifts the command and the
// authorization template values out of the message, supplies only the
// operator-owned actor and reason, dispatches through the real router, and
// requires the same start to then succeed.
func TestStartRefusalOverDanglingPredecessorNamesRunnableAbandon(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	predecessor, successor := mintDamagedStoreRecoveryPair(t, repo)
	if err := os.RemoveAll(damagedStoreLineageDir(t, repo, predecessor)); err != nil {
		t.Fatal(err)
	}
	writeDamagedStoreProse(t, repo, "fresh")

	var output bytes.Buffer
	startErr := RunReviewFacadeStart([]string{"--cwd", repo}, &output)
	if startErr == nil {
		t.Fatalf("start succeeded over a dangling predecessor: %s", output.String())
	}
	message := startErr.Error()
	if !strings.Contains(message, "dangling predecessor") {
		t.Fatalf("start refusal does not name the graph defect: %q", message)
	}

	tokens := liftBacktickedReviewCommand(t, message, "abandon")
	// The template block in the message carries the persisted binding values;
	// the operator supplies only actor and reason, exactly as the template
	// instructs.
	expectedRevision := regexp.MustCompile(`\nrevision=(\S+)`).FindStringSubmatch(message)
	snapshotIdentity := regexp.MustCompile(`\nsnapshot_identity=(\S+)`).FindStringSubmatch(message)
	if expectedRevision == nil || snapshotIdentity == nil {
		t.Fatalf("start refusal renders no authorization template: %q", message)
	}
	const actor, reason = "maintainer@example.com", "its predecessor is gone"
	authorization := reviewtransaction.RenderCompactAbandonAuthorization(
		successor, expectedRevision[1], snapshotIdentity[1], actor, reason)
	abandonArgs := append(tokens[1:], "--reason", reason, "--actor", actor, "--maintainer-authorization", authorization)
	var abandoned bytes.Buffer
	if err := RunReview(abandonArgs, &abandoned); err != nil {
		t.Fatalf("the abandonment the refusal named exits non-zero (named dead end): %v\n%s", err, abandoned.String())
	}

	output.Reset()
	if err := RunReviewFacadeStart([]string{"--cwd", repo}, &output); err != nil {
		t.Fatalf("start still refuses after the named exit ran: %v\n%s", err, output.String())
	}
}

// TestReclaimRefusalOverTruncatedRecordNamesDiagnosisNotReconcile is the CLI
// half of the ds05 measurement: reclaim refused the half-written record by
// pointing at reconcile-authority, and reconcile then answered `unexpected
// EOF`. The refusal now carries the precise diagnosis and the runnable
// inspection, which is dispatched and required to answer.
func TestReclaimRefusalOverTruncatedRecordNamesDiagnosisNotReconcile(t *testing.T) {
	reviewModeHome(t)
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
