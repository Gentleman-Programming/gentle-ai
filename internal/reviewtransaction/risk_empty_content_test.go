package reviewtransaction

import "testing"

// A zero-byte file has no content, so nothing about it can be classified. The
// fallback correctly refuses to call it passive -- failing closed on
// unclassifiable content is right -- but it reported the refusal as
// "executable_change", which describes content that is not there. The tier must
// not move; only the description of the evidence has to become true.
func TestEmptyFileIsNotReportedAsAnExecutableChange(t *testing.T) {
	for _, testCase := range []struct {
		name string
		stat DiffStat
	}{
		{name: "added", stat: DiffStat{Path: "EMPTY", OldMode: "000000", NewMode: "100644"}},
		{name: "added with a passive-looking name", stat: DiffStat{Path: "notes.txt", OldMode: "000000", NewMode: "100644"}},
		{name: "removed", stat: DiffStat{Path: "EMPTY", OldMode: "100644", NewMode: "000000"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reasons := deriveSnapshotRiskReasons([]DiffStat{testCase.stat}, nil)
			if len(reasons) != 1 {
				t.Fatalf("empty file produced %d reasons: %#v", len(reasons), reasons)
			}
			if reasons[0].Code == RiskReasonExecutableChange {
				t.Fatalf("a file with no content was described as an executable change: %#v", reasons[0])
			}
			if reasons[0].Code != RiskReasonEmptyContent || reasons[0].Path != testCase.stat.Path {
				t.Fatalf("empty file reason = %#v", reasons[0])
			}
			if reasons[0].Signal != "" {
				t.Fatalf("empty content invented a risk signal: %#v", reasons[0])
			}
			// Fail-closed classification is unchanged: unclassifiable content
			// still gets the consolidated review, never the passive tier.
			if level := riskLevelFromReasons(reasons); level != RiskMedium {
				t.Fatalf("empty file tier = %q, want %q", level, RiskMedium)
			}
		})
	}
}

// The executable-change fallback still has to describe every candidate that
// really does carry content it could not prove passive.
func TestContentBearingFallbackIsStillAnExecutableChange(t *testing.T) {
	stats := []DiffStat{{Path: "view.go", Additions: 3, Deletions: 1, OldMode: "100644", NewMode: "100644"}}
	reasons := deriveSnapshotRiskReasons(stats, nil)
	if len(reasons) != 1 || reasons[0].Code != RiskReasonExecutableChange || reasons[0].Path != "view.go" {
		t.Fatalf("content-bearing fallback = %#v", reasons)
	}
}
