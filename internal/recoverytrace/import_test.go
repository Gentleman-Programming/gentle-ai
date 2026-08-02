package recoverytrace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// systemicAuditPath is the authoritative source of the backlog reconciliation.
// The importer reads it directly so a drifting audit fails the build instead of
// silently reconciling against a stale copy.
const systemicAuditPath = "../../docs/audits/2026-07-23-systemic-remediation-architecture.md"

func readSystemicAudit(t *testing.T) []byte {
	t.Helper()

	source, err := os.ReadFile(filepath.Clean(systemicAuditPath))
	if err != nil {
		t.Fatalf("read systemic audit: %v", err)
	}
	return source
}

func TestImportAppendixBReconcilesTheAuthoritativeBacklog(t *testing.T) {
	t.Parallel()

	items, err := ImportAppendixB(readSystemicAudit(t))
	if err != nil {
		t.Fatalf("ImportAppendixB() error = %v", err)
	}

	var issues, pullRequests int
	for _, item := range items {
		switch item.Kind {
		case BacklogIssue:
			issues++
		case BacklogPullRequest:
			pullRequests++
		default:
			t.Fatalf("item %d has unknown kind %q", item.Number, item.Kind)
		}
	}

	if issues != expectedIssues {
		t.Errorf("issues = %d, want %d", issues, expectedIssues)
	}
	if pullRequests != expectedPullRequests {
		t.Errorf("pull requests = %d, want %d", pullRequests, expectedPullRequests)
	}
}

func TestImportAppendixBAssignsEveryItemAKnownContext(t *testing.T) {
	t.Parallel()

	items, err := ImportAppendixB(readSystemicAudit(t))
	if err != nil {
		t.Fatalf("ImportAppendixB() error = %v", err)
	}

	for _, item := range items {
		if !knownContext(item.Context) {
			t.Fatalf("item %d has unknown context %q", item.Number, item.Context)
		}
		if item.Disposition == "" || item.Action == "" {
			t.Fatalf("item %d lacks a disposition or action", item.Number)
		}
	}
}

func TestImportAppendixBIsDeterministic(t *testing.T) {
	t.Parallel()

	source := readSystemicAudit(t)
	first, err := ImportAppendixB(source)
	if err != nil {
		t.Fatalf("ImportAppendixB() error = %v", err)
	}
	second, err := ImportAppendixB(source)
	if err != nil {
		t.Fatalf("ImportAppendixB() error = %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("import is not deterministic: %d then %d items", len(first), len(second))
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("item %d differs between imports: %+v vs %+v", index, first[index], second[index])
		}
	}
}

// syntheticAppendix builds a minimal document with the same markers and shape as
// the real audit, so error cases stay readable and independent of its 241 rows.
func syntheticAppendix(issueRows, prRows string) []byte {
	var builder strings.Builder
	builder.WriteString("<!-- ISSUE_LEDGER_START -->\n")
	builder.WriteString("| # | Context | Snapshot disposition | Action |\n")
	builder.WriteString("|---:|---|---|---|\n")
	builder.WriteString(issueRows)
	builder.WriteString("<!-- ISSUE_LEDGER_END -->\n")
	builder.WriteString("<!-- PR_LEDGER_START -->\n")
	builder.WriteString("| # | Context | Snapshot disposition | Action |\n")
	builder.WriteString("|---:|---|---|---|\n")
	builder.WriteString(prRows)
	builder.WriteString("<!-- PR_LEDGER_END -->\n")
	return []byte(builder.String())
}

func TestImportAppendixBRejectsUnusableSource(t *testing.T) {
	t.Parallel()

	const goodIssue = "| 77 | `MMI` | `covered-by-systemic-fix` | `I1` |\n"
	const goodPR = "| 688 | `ACI` | `superseded-by-systemic-fix` | `P4` |\n"

	cases := []struct {
		name    string
		source  []byte
		wantErr error
	}{
		{
			name:    "issue table has no start marker",
			source:  []byte("| 77 | `MMI` | `covered-by-systemic-fix` | `I1` |\n"),
			wantErr: ErrLedgerNotFound,
		},
		{
			name: "issue table is truncated before its end marker",
			source: []byte("<!-- ISSUE_LEDGER_START -->\n" +
				"| # | Context | Snapshot disposition | Action |\n" + goodIssue),
			wantErr: ErrLedgerNotFound,
		},
		{
			name:    "row omits a column",
			source:  syntheticAppendix("| 77 | `MMI` | `covered-by-systemic-fix` |\n", goodPR),
			wantErr: ErrMalformedRow,
		},
		{
			name:    "row number is not an integer",
			source:  syntheticAppendix("| seventy-seven | `MMI` | `x` | `I1` |\n", goodPR),
			wantErr: ErrMalformedRow,
		},
		{
			name:    "row names a context outside the nine",
			source:  syntheticAppendix("| 77 | `XYZ` | `covered-by-systemic-fix` | `I1` |\n", goodPR),
			wantErr: ErrUnknownContext,
		},
		{
			name:    "same issue number appears twice",
			source:  syntheticAppendix(goodIssue+goodIssue, goodPR),
			wantErr: ErrDuplicateBacklogItem,
		},
		{
			name:    "same pull request number appears twice",
			source:  syntheticAppendix(goodIssue, goodPR+goodPR),
			wantErr: ErrDuplicateBacklogItem,
		},
		{
			name:    "ledger is empty",
			source:  syntheticAppendix("", goodPR),
			wantErr: ErrMalformedRow,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := ImportAppendixB(testCase.source)
			if err == nil {
				t.Fatalf("ImportAppendixB() error = nil, want %v", testCase.wantErr)
			}
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("ImportAppendixB() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestImportAppendixBAllowsTheSameNumberAcrossKinds(t *testing.T) {
	t.Parallel()

	// Issue #688 and pull request #688 are different items. Deduplication must be
	// scoped per kind or valid backlogs would be rejected.
	source := syntheticAppendix(
		"| 688 | `MMI` | `covered-by-systemic-fix` | `I1` |\n",
		"| 688 | `ACI` | `superseded-by-systemic-fix` | `P4` |\n",
	)

	items, err := ImportAppendixB(source)
	if err != nil {
		t.Fatalf("ImportAppendixB() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
}
