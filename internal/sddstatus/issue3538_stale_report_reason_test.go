package sddstatus

import (
	"path/filepath"
	"strings"
	"testing"
)

// Regression for #3538. Reporters saw `verify: ready`, `archive: blocked`,
// `nextRecommended: verify` with an empty blockedReasons even though every
// task was complete and a verify-report existed. Two paths produced that
// silent tuple: a stale report whose totals disagree with the native heading
// count (published only when Incomplete), and the post-remediation override
// whose refresh instruction reached phaseInstructions only. This test pins
// the stale-report shape: the persisted envelope was admitted against the
// totals the caller supplied, yet native counting of the specs disagrees.
func TestStaleVerifyReportNamesNativeTotalsInBlockedReasons(t *testing.T) {
	const spec = "### Requirement: Auth\n#### Scenario: Expected behavior\n"
	report := testVerifyEnvelope("pass", 0, 0, "13/13", "46/46", 0, 0)
	if admission := ValidateVerifyReportAdmission(report, SpecCounts{Requirements: 13, Scenarios: 46}); !admission.Valid {
		t.Fatalf("admission with caller-supplied totals = %#v, want valid", admission)
	}

	for _, backend := range []string{"openspec", "engram"} {
		t.Run(backend, func(t *testing.T) {
			root := t.TempDir()
			var status Status
			var err error
			switch backend {
			case "openspec":
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
				write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), spec)
				write(t, filepath.Join(changeRoot, "verify-report.md"), report)
				status, err = Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			case "engram":
				mkdir(t, filepath.Join(root, ".engram"))
				project := strings.ToLower(filepath.Base(root))
				restore := stubEngramExport(t, []engramObservation{
					{Title: "sdd/thin/proposal", Content: "# Proposal\n", Project: project, Scope: "project"},
					{Title: "sdd/thin/spec", Content: spec, Project: project, Scope: "project"},
					{Title: "sdd/thin/design", Content: "# Design\n", Project: project, Scope: "project"},
					{Title: "sdd/thin/tasks", Content: "- [x] 1.1 Done\n", Project: project, Scope: "project"},
					{Title: "sdd/thin/verify-report", Content: report, Project: project, Scope: "project"},
				})
				defer restore()
				status, err = Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			}
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "verify" {
				t.Fatalf("status = verify %q archive %q next %q, want ready/blocked/verify", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended)
			}
			if !status.TaskProgress.AllComplete {
				t.Fatalf("TaskProgress = %#v, want all complete", status.TaskProgress)
			}
			joined := strings.Join(status.BlockedReasons, "\n")
			for _, want := range []string{
				"does not match actual requirement count 1",
				"rerun SDD verification",
			} {
				if !strings.Contains(joined, want) {
					t.Fatalf("BlockedReasons = %v, want containing %q", status.BlockedReasons, want)
				}
			}
		})
	}
}
