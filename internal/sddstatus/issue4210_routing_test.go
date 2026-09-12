package sddstatus

import (
	"path/filepath"
	"strings"
	"testing"
)

// #4210: verification evidence cannot block implementation that has not yet
// reached its final verification gate. It remains required once tasks complete.
func TestHistoricalVerificationEvidenceDoesNotBlockPendingApply(t *testing.T) {
	partialFailure := testVerifyEnvelope("fail", 1, 0, "0/1", "0/1", 1, 0)
	if admission := ValidateVerifyReportAdmission(partialFailure, SpecCounts{Requirements: 1, Scenarios: 1}); !admission.Valid {
		t.Fatalf("partial failed report admission = %#v, want valid", admission)
	}

	for _, tt := range []struct {
		name   string
		report string
	}{
		{name: "admitted partial fail", report: partialFailure},
		{name: "legacy report without envelope", report: "## Verification\n\nVerdict: FAIL\n\n- pending coverage\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			changeRoot := seedReadyChange(t, root, "historical", "- [ ] 1.1 Finish implementation\n")
			write(t, filepath.Join(changeRoot, "verify-report.md"), tt.report)

			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "historical"})
			if err != nil {
				t.Fatal(err)
			}
			if status.Dependencies.Apply != DependencyReady || status.NextRecommended != string(PhaseApply) {
				t.Fatalf("routing = apply %q next %q, want ready/apply", status.Dependencies.Apply, status.NextRecommended)
			}
			if len(status.BlockedReasons) != 0 {
				t.Fatalf("pending implementation inherited final-verification blocker: %v", status.BlockedReasons)
			}
			if status.Dependencies.Verify != DependencyBlocked || status.Dependencies.Archive != DependencyBlocked {
				t.Fatalf("dependencies = %#v, want verification and archive blocked until tasks complete", status.Dependencies)
			}
		})
	}
}

func TestHistoricalVerificationEvidenceStillBlocksFinalization(t *testing.T) {
	partialFailure := testVerifyEnvelope("fail", 1, 0, "0/1", "0/1", 1, 0)
	for _, tt := range []struct {
		name   string
		report string
	}{
		{name: "admitted partial fail", report: partialFailure},
		{name: "legacy report without envelope", report: "## Verification\n\nVerdict: FAIL\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			changeRoot := seedReadyChange(t, root, "historical", "- [x] 1.1 Implementation complete\n")
			write(t, filepath.Join(changeRoot, "verify-report.md"), tt.report)

			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "historical"})
			if err != nil {
				t.Fatal(err)
			}
			if status.Dependencies.Apply != DependencyAllDone || status.Dependencies.Verify != DependencyReady || status.NextRecommended != string(PhaseVerify) {
				t.Fatalf("routing = %#v next %q, want apply all_done, verify ready, next verify", status.Dependencies, status.NextRecommended)
			}
			if status.Dependencies.Archive != DependencyBlocked || len(status.BlockedReasons) == 0 {
				t.Fatalf("finalization bypassed incomplete verification: archive %q reasons %v", status.Dependencies.Archive, status.BlockedReasons)
			}
		})
	}
}

func TestCompleteVerificationFailureStillRequiresRemediation(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "failed", "- [x] 1.1 Implementation complete\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("fail", 1, 0, "1/1", "1/1", 1, 0))

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	if !status.RemediationState.Required || status.NextRecommended != string(PhaseRemediate) {
		t.Fatalf("complete failure = remediation %#v next %q, want required/remediate", status.RemediationState, status.NextRecommended)
	}
	if status.Dependencies.Archive != DependencyBlocked || !strings.Contains(strings.Join(status.BlockedReasons, "\n"), "remediation") {
		t.Fatalf("complete failure bypassed remediation: archive %q reasons %v", status.Dependencies.Archive, status.BlockedReasons)
	}
}

func TestHistoricalVerificationEvidenceEngramParity(t *testing.T) {
	partialFailure := testVerifyEnvelope("fail", 1, 0, "0/1", "0/1", 1, 0)
	for _, report := range []struct {
		name    string
		content string
	}{
		{name: "admitted partial fail", content: partialFailure},
		{name: "legacy report without envelope", content: "## Verification\n\nVerdict: FAIL\n"},
	} {
		for _, tasks := range []struct {
			name            string
			content         string
			wantApply       DependencyState
			wantVerify      DependencyState
			wantNext        string
			wantBlockReason bool
		}{
			{name: "pending tasks", content: "- [ ] 1.1 Finish implementation\n", wantApply: DependencyReady, wantVerify: DependencyBlocked, wantNext: string(PhaseApply)},
			{name: "completed tasks", content: "- [x] 1.1 Implementation complete\n", wantApply: DependencyAllDone, wantVerify: DependencyReady, wantNext: string(PhaseVerify), wantBlockReason: true},
		} {
			t.Run(report.name+"/"+tasks.name, func(t *testing.T) {
				root := t.TempDir()
				mkdir(t, filepath.Join(root, ".engram"))
				project := strings.ToLower(filepath.Base(root))
				restore := stubEngramExport(t, []engramObservation{
					{Title: "sdd/historical/proposal", Content: "# Proposal\n", Project: project, Scope: "project"},
					{Title: "sdd/historical/spec", Content: "### Requirement: routing\n#### Scenario: historical verification\n", Project: project, Scope: "project"},
					{Title: "sdd/historical/design", Content: "# Design\n", Project: project, Scope: "project"},
					{Title: "sdd/historical/tasks", Content: tasks.content, Project: project, Scope: "project"},
					{Title: "sdd/historical/verify-report", Content: report.content, Project: project, Scope: "project"},
				})
				defer restore()

				status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "historical"})
				if err != nil {
					t.Fatal(err)
				}
				if status.ArtifactStore != ArtifactStoreEngram || status.Dependencies.Apply != tasks.wantApply ||
					status.Dependencies.Verify != tasks.wantVerify || status.NextRecommended != tasks.wantNext {
					t.Fatalf("Engram status = store %q dependencies %#v next %q", status.ArtifactStore, status.Dependencies, status.NextRecommended)
				}
				if (len(status.BlockedReasons) != 0) != tasks.wantBlockReason {
					t.Fatalf("Engram blockers = %v, want blocker present %t", status.BlockedReasons, tasks.wantBlockReason)
				}
			})
		}
	}
}
