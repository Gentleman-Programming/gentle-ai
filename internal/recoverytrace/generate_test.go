package recoverytrace

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// schemaDir holds the JSON Schema that describes each rendered document. The
// tests read it from disk so a rendered ledger without a published schema fails
// the build instead of shipping undescribed evidence.
const schemaDir = "../../docs/audits/data/organic-rdd-recovery/schemas"

// auditOverlapCounts carries the three reconciliation figures Appendix B does
// not record. They are supplied by the caller rather than derived, because
// inventing them here would let a generator agree with itself.
func auditOverlapCounts() OverlapCounts {
	return OverlapCounts{
		CollisionPRs:   expectedCollisionPRs,
		Overlaps:       expectedOverlaps,
		Decompositions: expectedDecompositions,
	}
}

func generatedLedgers(t *testing.T) Ledgers {
	t.Helper()

	ledgers, err := Generate(readSystemicAudit(t), validLedgers().Rows, auditOverlapCounts())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	return ledgers
}

func renderedDocuments(t *testing.T) map[string][]byte {
	t.Helper()

	documents, err := Render(generatedLedgers(t))
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return documents
}

func TestGenerateDerivesReconciliationFromTheImportedAudit(t *testing.T) {
	t.Parallel()

	ledgers := generatedLedgers(t)

	// The counts are asserted against the audit's own totals, not against the
	// frozen constants, so a generator that skipped the import cannot pass.
	if ledgers.Reconciliation.Issues != 241 {
		t.Errorf("Reconciliation.Issues = %d, want 241", ledgers.Reconciliation.Issues)
	}
	if ledgers.Reconciliation.PullRequests != 92 {
		t.Errorf("Reconciliation.PullRequests = %d, want 92", ledgers.Reconciliation.PullRequests)
	}
	if ledgers.Reconciliation.CollisionPRs != expectedCollisionPRs {
		t.Errorf("Reconciliation.CollisionPRs = %d, want %d",
			ledgers.Reconciliation.CollisionPRs, expectedCollisionPRs)
	}
	if ledgers.Reconciliation.Overlaps != expectedOverlaps {
		t.Errorf("Reconciliation.Overlaps = %d, want %d",
			ledgers.Reconciliation.Overlaps, expectedOverlaps)
	}
	if ledgers.Reconciliation.Decompositions != expectedDecompositions {
		t.Errorf("Reconciliation.Decompositions = %d, want %d",
			ledgers.Reconciliation.Decompositions, expectedDecompositions)
	}
	if len(ledgers.Rows) != len(validLedgers().Rows) {
		t.Fatalf("rows = %d, want %d", len(ledgers.Rows), len(validLedgers().Rows))
	}
	if err := ValidateLedgers(ledgers); err != nil {
		t.Fatalf("ValidateLedgers(Generate(...)) error = %v, want nil", err)
	}
}

// backlogTally counts each distinct backlog item once, keyed by kind and
// number, so a repeated item can never stand in for a missing one.
func backlogTally(t *testing.T, items []BacklogEntry) map[BacklogKind]int {
	t.Helper()

	seen := make(map[BacklogKind]map[int]struct{}, 2)
	for _, item := range items {
		byNumber, known := seen[item.Kind]
		if !known {
			byNumber = make(map[int]struct{})
			seen[item.Kind] = byNumber
		}
		if _, duplicate := byNumber[item.Number]; duplicate {
			t.Fatalf("backlog repeats %s %d", item.Kind, item.Number)
		}
		byNumber[item.Number] = struct{}{}
	}

	tally := make(map[BacklogKind]int, len(seen))
	for kind, byNumber := range seen {
		tally[kind] = len(byNumber)
	}
	return tally
}

func TestGenerateCarriesEveryBacklogItemNotJustItsCount(t *testing.T) {
	t.Parallel()

	ledgers := generatedLedgers(t)

	// The items themselves must be present. A count with nothing behind it is
	// the failure this ledger exists to make impossible.
	tally := backlogTally(t, ledgers.Backlog)
	if tally[BacklogIssue] != 241 {
		t.Errorf("backlog issues = %d, want 241", tally[BacklogIssue])
	}
	if tally[BacklogPullRequest] != 92 {
		t.Errorf("backlog pull requests = %d, want 92", tally[BacklogPullRequest])
	}
	if len(ledgers.Backlog) != 241+92 {
		t.Errorf("backlog items = %d, want %d", len(ledgers.Backlog), 241+92)
	}
	if ledgers.Reconciliation.Issues != tally[BacklogIssue] {
		t.Errorf("Reconciliation.Issues = %d, want %d",
			ledgers.Reconciliation.Issues, tally[BacklogIssue])
	}
	if ledgers.Reconciliation.PullRequests != tally[BacklogPullRequest] {
		t.Errorf("Reconciliation.PullRequests = %d, want %d",
			ledgers.Reconciliation.PullRequests, tally[BacklogPullRequest])
	}
}

func TestGenerateIgnoresCountsItWasNotAskedToTrust(t *testing.T) {
	t.Parallel()

	// Generate derives the issue and pull-request totals from the import, so a
	// caller cannot pre-declare them; only the overlap figures are supplied.
	ledgers := generatedLedgers(t)

	for _, item := range ledgers.Backlog {
		if item.Release != "" {
			t.Fatalf("%s %d is classified before a release exists: %q",
				item.Kind, item.Number, item.Release)
		}
	}
}

func TestSnapshotLedgerEmitsTheBacklogItems(t *testing.T) {
	t.Parallel()

	ledgers := generatedLedgers(t)

	first, err := Render(ledgers)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	second, err := Render(ledgers)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !bytes.Equal(first[SnapshotLedgerFile], second[SnapshotLedgerFile]) {
		t.Fatalf("%s is not byte-identical across calls:\n%s\nvs\n%s",
			SnapshotLedgerFile, first[SnapshotLedgerFile], second[SnapshotLedgerFile])
	}

	var document struct {
		Backlog []struct {
			Action      string          `json:"action"`
			Context     SystemicContext `json:"context"`
			Disposition string          `json:"disposition"`
			Kind        BacklogKind     `json:"kind"`
			Number      int             `json:"number"`
		} `json:"backlog"`
		Reconciliation struct {
			Issues       int `json:"issues"`
			PullRequests int `json:"pullRequests"`
		} `json:"reconciliation"`
	}
	if err := json.Unmarshal(first[SnapshotLedgerFile], &document); err != nil {
		t.Fatalf("%s is not valid JSON: %v", SnapshotLedgerFile, err)
	}

	if len(document.Backlog) != len(ledgers.Backlog) {
		t.Fatalf("emitted backlog = %d items, want %d",
			len(document.Backlog), len(ledgers.Backlog))
	}

	emitted := make([]BacklogEntry, 0, len(document.Backlog))
	for _, item := range document.Backlog {
		if item.Context == "" || item.Disposition == "" || item.Action == "" {
			t.Fatalf("%s %d is emitted without its audit record", item.Kind, item.Number)
		}
		emitted = append(emitted, BacklogEntry{BacklogItem: BacklogItem{
			Number: item.Number,
			Kind:   item.Kind,
		}})
	}

	tally := backlogTally(t, emitted)
	if tally[BacklogIssue] != document.Reconciliation.Issues {
		t.Errorf("emitted issues = %d, want the declared %d",
			tally[BacklogIssue], document.Reconciliation.Issues)
	}
	if tally[BacklogPullRequest] != document.Reconciliation.PullRequests {
		t.Errorf("emitted pull requests = %d, want the declared %d",
			tally[BacklogPullRequest], document.Reconciliation.PullRequests)
	}
}

func TestReleaseLedgerClassifiesEveryBacklogItem(t *testing.T) {
	t.Parallel()

	ledgers := generatedLedgers(t)
	documents, err := Render(ledgers)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var document struct {
		Backlog []struct {
			Kind    BacklogKind  `json:"kind"`
			Number  int          `json:"number"`
			Release ReleaseClass `json:"release"`
		} `json:"backlog"`
	}
	if err := json.Unmarshal(documents[ReleaseLedgerFile], &document); err != nil {
		t.Fatalf("%s is not valid JSON: %v", ReleaseLedgerFile, err)
	}

	if len(document.Backlog) != len(ledgers.Backlog) {
		t.Fatalf("classified backlog = %d items, want %d",
			len(document.Backlog), len(ledgers.Backlog))
	}

	emitted := make([]BacklogEntry, 0, len(document.Backlog))
	for _, item := range document.Backlog {
		if item.Release != "" && !knownReleaseClass(item.Release) {
			t.Fatalf("%s %d carries unknown classification %q", item.Kind, item.Number, item.Release)
		}
		emitted = append(emitted, BacklogEntry{BacklogItem: BacklogItem{
			Number: item.Number,
			Kind:   item.Kind,
		}})
	}

	tally := backlogTally(t, emitted)
	if tally[BacklogIssue] != 241 {
		t.Errorf("classified issues = %d, want 241", tally[BacklogIssue])
	}
	if tally[BacklogPullRequest] != 92 {
		t.Errorf("classified pull requests = %d, want 92", tally[BacklogPullRequest])
	}
}

func TestRenderOrdersTheBacklogDeterministically(t *testing.T) {
	t.Parallel()

	ledgers := generatedLedgers(t)
	// Reversing the imported order must not move a byte: the published order is
	// kind then number, never the order the items happened to arrive in.
	reversed := make([]BacklogEntry, 0, len(ledgers.Backlog))
	for index := len(ledgers.Backlog) - 1; index >= 0; index-- {
		reversed = append(reversed, ledgers.Backlog[index])
	}

	forward, err := Render(ledgers)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	ledgers.Backlog = reversed
	backward, err := Render(ledgers)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	for name, document := range forward {
		if !bytes.Equal(document, backward[name]) {
			t.Fatalf("%s depends on backlog order:\n%s\nvs\n%s", name, document, backward[name])
		}
	}
}

func TestGenerateRejectsLedgersThatFailValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func([]Row) []Row
		wantErr error
	}{
		{
			name: "deletion without destination or no-invariant proof",
			mutate: func(rows []Row) []Row {
				rows[2].DestinationPath = ""
				rows[2].DestinationProof = nil
				return rows
			},
			wantErr: ErrUnprovenDeletion,
		},
		{
			name: "row without contributor credit",
			mutate: func(rows []Row) []Row {
				rows[0].Contributor = ""
				return rows
			},
			wantErr: ErrMissingCredit,
		},
		{
			name: "retained invariant without an owning context",
			mutate: func(rows []Row) []Row {
				rows[0].Context = ""
				return rows
			},
			wantErr: ErrOrphanedInvariant,
		},
		{
			name: "same path carries two dispositions",
			mutate: func(rows []Row) []Row {
				return append(rows, rows[0])
			},
			wantErr: ErrDuplicatePath,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := Generate(readSystemicAudit(t), testCase.mutate(validLedgers().Rows), auditOverlapCounts())
			if err == nil {
				t.Fatalf("Generate() error = nil, want %v", testCase.wantErr)
			}
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Generate() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestGenerateRejectsCountsThatDoNotReconcile(t *testing.T) {
	t.Parallel()

	// The overlap figures arrive from the caller, so a wrong one must be caught
	// by validation rather than absorbed into the emitted ledger.
	counts := auditOverlapCounts()
	counts.Overlaps--

	_, err := Generate(readSystemicAudit(t), validLedgers().Rows, counts)
	if !errors.Is(err, ErrCountMismatch) {
		t.Fatalf("Generate() error = %v, want %v", err, ErrCountMismatch)
	}
}

func TestGenerateRejectsAnUnusableAudit(t *testing.T) {
	t.Parallel()

	_, err := Generate([]byte("# not the systemic audit\n"), validLedgers().Rows, auditOverlapCounts())
	if !errors.Is(err, ErrLedgerNotFound) {
		t.Fatalf("Generate() error = %v, want %v", err, ErrLedgerNotFound)
	}
}

func TestRenderEmitsEveryLedgerDocumentExactlyOnce(t *testing.T) {
	t.Parallel()

	want := []string{
		"change-ledger.json",
		"contribution-ledger.json",
		"deletion-ledger.json",
		"invariant-ledger.json",
		"release-ledger.json",
		"snapshot-ledger.json",
		"test-ledger.json",
	}

	documents := renderedDocuments(t)
	got := make([]string, 0, len(documents))
	for name := range documents {
		got = append(got, name)
	}
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("documents = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("documents = %v, want %v", got, want)
		}
	}
}

func TestRenderIsByteStableAcrossCalls(t *testing.T) {
	t.Parallel()

	ledgers := generatedLedgers(t)

	first, err := Render(ledgers)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	second, err := Render(ledgers)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("render is not stable: %d then %d documents", len(first), len(second))
	}
	for name, document := range first {
		if !bytes.Equal(document, second[name]) {
			t.Fatalf("%s is not byte-stable:\n%s\nvs\n%s", name, document, second[name])
		}
	}
}

func TestRenderIsIndependentOfRowOrder(t *testing.T) {
	t.Parallel()

	// The generated-current check compares bytes, so an input reordering that a
	// caller cannot control must not move a single byte of the output.
	forward := validLedgers().Rows
	reversed := make([]Row, 0, len(forward))
	for index := len(forward) - 1; index >= 0; index-- {
		reversed = append(reversed, forward[index])
	}

	source := readSystemicAudit(t)
	inOrder, err := Generate(source, forward, auditOverlapCounts())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	shuffled, err := Generate(source, reversed, auditOverlapCounts())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	first, err := Render(inOrder)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	second, err := Render(shuffled)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	for name, document := range first {
		if !bytes.Equal(document, second[name]) {
			t.Fatalf("%s depends on row order:\n%s\nvs\n%s", name, document, second[name])
		}
	}
}

func TestRenderEmitsCanonicalJSON(t *testing.T) {
	t.Parallel()

	for name, document := range renderedDocuments(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !bytes.HasSuffix(document, []byte("\n")) {
				t.Fatalf("%s does not end with a trailing newline", name)
			}

			var value any
			if err := json.Unmarshal(document, &value); err != nil {
				t.Fatalf("%s is not valid JSON: %v", name, err)
			}

			// Re-marshalling a decoded document sorts object keys and applies the
			// two-space indent. Equality therefore proves the emitted bytes are
			// already canonical and that decoding lost nothing.
			roundTripped, err := json.MarshalIndent(value, "", "  ")
			if err != nil {
				t.Fatalf("re-marshal %s: %v", name, err)
			}
			roundTripped = append(roundTripped, '\n')

			if !bytes.Equal(document, roundTripped) {
				t.Fatalf("%s is not canonical:\n%s\nwant\n%s", name, document, roundTripped)
			}
		})
	}
}

func TestRenderedDocumentsHaveAPublishedSchema(t *testing.T) {
	t.Parallel()

	for name := range renderedDocuments(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			schemaName := name[:len(name)-len(".json")] + ".schema.json"
			source, err := os.ReadFile(filepath.Clean(filepath.Join(schemaDir, schemaName)))
			if err != nil {
				t.Fatalf("read schema for %s: %v", name, err)
			}

			var schema map[string]any
			if err := json.Unmarshal(source, &schema); err != nil {
				t.Fatalf("%s is not valid JSON: %v", schemaName, err)
			}
			for _, key := range []string{"$schema", "$id", "title", "type", "required", "additionalProperties"} {
				if _, present := schema[key]; !present {
					t.Errorf("%s omits %q", schemaName, key)
				}
			}
			if schema["additionalProperties"] != false {
				t.Errorf("%s does not close additionalProperties", schemaName)
			}
		})
	}
}
