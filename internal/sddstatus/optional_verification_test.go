package sddstatus

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOptionalVerificationDoesNotAuthorizeOrGateArchive(t *testing.T) {
	reports := map[string]string{
		"missing":   "",
		"malformed": "# Verify\n```json\n{broken\n",
		"failed":    "# Verification\nCRITICAL: runtime check failed (exit 1).\n",
		"stale":     "# Historical verification\nVerified an older implementation with different requirements.\n",
	}
	for _, store := range []string{"openspec", "engram"} {
		for reportName, report := range reports {
			for _, complete := range []bool{false, true} {
				name := store + "/" + reportName + "/partial"
				if complete {
					name = store + "/" + reportName + "/complete"
				}
				t.Run(name, func(t *testing.T) {
					root := t.TempDir()
					tasks := "- [ ] 1.1 Implement behavior\n"
					wantNext := "apply"
					if complete {
						tasks = "- [x] 1.1 Implement behavior\n"
						wantNext = "archive"
					}
					var reportPath string
					var observations []engramObservation
					if store == "openspec" {
						change := seedReadyChange(t, root, "optional", tasks)
						reportPath = filepath.Join(change, "verify-report.md")
						if report != "" {
							write(t, reportPath, report)
						}
					} else {
						write(t, filepath.Join(root, "openspec", "config.yaml"), "sdd:\n  artifact_store: engram\n")
						t.Setenv("ENGRAM_PROJECT", "gentle-ai")
						for kind, content := range map[string]string{"proposal": "# Proposal\n", "spec": "### Requirement: Auth\n#### Scenario: Expected behavior\n", "design": "# Design\n", "tasks": tasks, "verify-report": report} {
							if content != "" {
								observations = append(observations, engramObservation{Title: "sdd/optional/" + kind, Content: content, Project: "gentle-ai", Scope: "project"})
							}
						}
						t.Cleanup(stubEngramExport(t, observations))
					}
					original := append([]engramObservation(nil), observations...)
					status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "optional", IncludeInstructions: true})
					if err != nil {
						t.Fatal(err)
					}
					if status.NextRecommended != wantNext || status.Dependencies.Archive != DependencyReady || status.Dependencies.Verify != DependencyReady || len(status.BlockedReasons) != 0 {
						t.Errorf("next=%s archive=%s verify=%s blockers=%v; want %s/ready/ready/no blockers", status.NextRecommended, status.Dependencies.Archive, status.Dependencies.Verify, status.BlockedReasons, wantNext)
					}
					if status.TaskProgress.AllComplete != complete {
						t.Errorf("task truth changed: %+v", status.TaskProgress)
					}
					instructions := strings.Join(status.PhaseInstructions.Verify, "\n")
					if !strings.Contains(instructions, "optional") || !strings.Contains(instructions, "partial") {
						t.Errorf("explicit verify must allow optional partial diagnostics: %s", instructions)
					}
					if reportPath != "" && report != "" {
						got, err := os.ReadFile(reportPath)
						if err != nil || string(got) != report {
							t.Fatalf("historical report mutated: %q, %v", got, err)
						}
					}
					if !reflect.DeepEqual(observations, original) {
						t.Fatal("historical Engram observations changed")
					}
				})
			}
		}
	}
}

func TestOptionalVerificationRetainsEditAuthorityBlock(t *testing.T) {
	for _, store := range []string{"openspec", "engram"} {
		t.Run(store, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			tasks := "- [ ] Edit `" + filepath.ToSlash(filepath.Join(outside, "main.go")) + "`\n"
			if store == "openspec" {
				seedReadyChange(t, root, "permission", tasks)
			} else {
				write(t, filepath.Join(root, "openspec", "config.yaml"), "sdd:\n  artifact_store: engram\n")
				t.Setenv("ENGRAM_PROJECT", "gentle-ai")
				var observations []engramObservation
				for kind, content := range map[string]string{"proposal": "# Proposal", "spec": "# Spec", "design": "# Design", "tasks": tasks} {
					observations = append(observations, engramObservation{Title: "sdd/permission/" + kind, Content: content, Project: "gentle-ai", Scope: "project"})
				}
				t.Cleanup(stubEngramExport(t, observations))
			}
			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "permission"})
			if err != nil {
				t.Fatal(err)
			}
			if status.ApplyState != ApplyBlocked || status.Dependencies.Archive != DependencyBlocked || !strings.Contains(strings.Join(status.BlockedReasons, "\n"), "edit_authority_missing") {
				t.Fatalf("optional verification bypassed permissions: %+v", status)
			}
			if !reflect.DeepEqual(status.ActionContext.AllowedEditRoots, []string{root}) {
				t.Fatalf("diagnostics granted edit roots: %v", status.ActionContext.AllowedEditRoots)
			}
		})
	}
}
