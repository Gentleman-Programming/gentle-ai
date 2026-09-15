package sddstatus

import (
	"path/filepath"
	"strings"
	"testing"
)

// #3538 historical count mismatches no longer gate archive or mutate reports.
func TestStaleVerifyReportDoesNotGateArchive(t *testing.T) {
	const spec = "### Requirement: Auth\n#### Scenario: Expected behavior\n"
	report := testVerifyEnvelope("pass", 0, 0, "13/13", "46/46", 0, 0)

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
			if status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyReady || status.NextRecommended != "archive" {
				t.Fatalf("status = verify %q archive %q next %q, want ready/ready/archive", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended)
			}
			if !status.TaskProgress.AllComplete {
				t.Fatalf("TaskProgress = %#v, want all complete", status.TaskProgress)
			}
			if len(status.BlockedReasons) != 0 {
				t.Fatalf("historical report blocked archive: %v", status.BlockedReasons)
			}
		})
	}
}
