package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestBindApprovedReviewRejectsInvalidChangeBeforePublishing(t *testing.T) {
	if _, err := BindApprovedReview(context.Background(), t.TempDir(), "../escape", "approved", ""); err == nil {
		t.Fatal("traversal change name was accepted")
	}
}

func TestBindApprovedReviewCASAndLiveEvidence(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
	binding, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", "")
	if err != nil {
		t.Fatal(err)
	}
	if binding.Schema != reviewBindingSchema || binding.AuthorityRevision == "" || binding.ReceiptHash == "" || binding.GateContext.Gate != "post-apply" {
		t.Fatalf("binding = %#v", binding)
	}
	if _, err := BindApprovedReview(context.Background(), filepath.Join(root, "openspec", "changes", "thin"), "thin", "approved-thin", ""); err != nil {
		t.Fatalf("exact retry with original empty expected revision: %v", err)
	}
	if _, err := BindApprovedReview(context.Background(), root, "thin", "other", binding.Revision); err == nil {
		t.Fatal("conflicting lineage accepted")
	}
	if _, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", "sha256:deadbeef"); err != nil {
		t.Fatalf("identical candidate retry must precede expected revision conflict: %v", err)
	}
	runtimeStore := mustRuntimeStore(t, root, "thin")
	assertNativeBinding(t, runtimeStore, binding.Revision)
	corruptNativeRuntimeBinding(t, runtimeStore)
	if _, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", binding.Revision); err == nil {
		t.Fatal("corrupt binding accepted")
	}
	if err := os.WriteFile(filepath.Join(changeRoot, "tasks.md"), []byte("- [x] 1.1 Done\n# drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", ""); err == nil {
		t.Fatal("working-tree drift bound authority")
	}
}

func TestBindApprovedReviewUsesNestedOpenSpecPlanningWorkspace(t *testing.T) {
	root := t.TempDir()
	planningRoot := filepath.Join(root, "packages", "app")
	changeRoot := seedReadyChange(t, planningRoot, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")

	binding, err := BindApprovedReview(context.Background(), planningRoot, "thin", "approved-thin", "")
	if err != nil {
		t.Fatal(err)
	}
	deeperPath := filepath.Join(planningRoot, "src", "feature")
	if err := os.MkdirAll(deeperPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := BindApprovedReview(context.Background(), deeperPath, "thin", "approved-thin", binding.Revision); err != nil {
		t.Fatalf("bind from deeper package path: %v", err)
	}
	runtimeStore := mustRuntimeStore(t, root, "thin")
	assertNativeBinding(t, runtimeStore, binding.Revision)
	status, err := Resolve(ResolveOptions{CWD: planningRoot, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.PlanningHome.Path != filepath.Join(planningRoot, "openspec") || status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("nested planning status did not consume canonical binding: %#v", status)
	}
}

func TestBindApprovedReviewRejectsAmbiguousPlanningChanges(t *testing.T) {
	for _, tt := range []struct {
		name string
		seed func(t *testing.T, root, planningRoot string)
	}{
		{name: "ancestor collision", seed: func(t *testing.T, root, planningRoot string) {
			seedReadyChange(t, root, "thin", "- [x] 1.1 Root\n")
			seedReadyChange(t, planningRoot, "thin", "- [x] 1.1 Package\n")
		}},
		{name: "sibling collision", seed: func(t *testing.T, root, planningRoot string) {
			seedReadyChange(t, planningRoot, "thin", "- [x] 1.1 App\n")
			seedReadyChange(t, filepath.Join(root, "packages", "api"), "thin", "- [x] 1.1 API\n")
		}},
		{name: "symlinked sibling collision", seed: func(t *testing.T, root, planningRoot string) {
			seedReadyChange(t, planningRoot, "thin", "- [x] 1.1 App\n")
			outside := t.TempDir()
			seedReadyChange(t, outside, "thin", "- [x] 1.1 API\n")
			link := filepath.Join(root, "packages", "api", "openspec")
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(outside, "openspec"), link); err != nil {
				t.Skipf("symlink fixture unavailable: %v", err)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			planningRoot := filepath.Join(root, "packages", "app")
			tt.seed(t, root, planningRoot)
			runSDDStatusGit(t, root, "init", "-q")

			if _, err := BindApprovedReview(context.Background(), planningRoot, "thin", "approved-thin", ""); err == nil || !strings.Contains(err.Error(), "ambiguous") {
				t.Fatalf("ambiguous planning changes error = %v", err)
			}
		})
	}
}

func TestBindApprovedReviewRejectsOpenSpecSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	planningRoot := filepath.Join(root, "packages", "app")
	if err := os.MkdirAll(planningRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	seedReadyChange(t, outside, "thin", "- [x] 1.1 Outside\n")
	if err := os.Symlink(filepath.Join(outside, "openspec"), filepath.Join(planningRoot, "openspec")); err != nil {
		t.Fatal(err)
	}
	runSDDStatusGit(t, root, "init", "-q")

	if _, err := BindApprovedReview(context.Background(), planningRoot, "thin", "approved-thin", ""); err == nil || !strings.Contains(err.Error(), "outside repository") {
		t.Fatalf("OpenSpec symlink escape error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "gentle-ai")); !os.IsNotExist(err) {
		t.Fatalf("symlink escape created runtime authority: %v", err)
	}
}

func TestBindApprovedReviewIgnoresGitExcludedUnreadableCollision(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(root, ".gitignore"), ".data/\nignored/\n")
	seedReadyChange(t, filepath.Join(root, "ignored"), "thin", "- [x] ignored\n")
	unreadable := filepath.Join(root, ".data", "postgres")
	write(t, filepath.Join(unreadable, "base", "state"), "runtime\n")
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")

	if _, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", ""); err != nil {
		t.Fatalf("Git-excluded runtime path affected binding: %v", err)
	}
}

func TestBindingChangeRootsSkipsDeletedCachedRoot(t *testing.T) {
	root := t.TempDir()
	selected := seedReadyChange(t, root, "thin", "- [x] selected\n")
	deleted := seedReadyChange(t, filepath.Join(root, "packages", "api"), "thin", "- [x] deleted\n")
	runSDDStatusGit(t, root, "init", "-q")
	runSDDStatusGit(t, root, "add", ".")
	if err := os.RemoveAll(deleted); err != nil {
		t.Fatal(err)
	}

	got, err := bindingChangeRoots(context.Background(), root, "thin")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{selected}) {
		t.Fatalf("bindingChangeRoots() = %v, want only %q", got, selected)
	}
}

func TestBindingChangeRootsUsesFirstRootAndIncludesSymlink(t *testing.T) {
	root := t.TempDir()
	outer := filepath.Join(root, "openspec", "changes", "thin")
	write(t, filepath.Join(outer, "openspec", "changes", "thin", "tasks.md"), "nested\n")
	outside := t.TempDir()
	seedReadyChange(t, outside, "thin", "- [x] linked\n")
	linkedOpenSpec := filepath.Join(root, "packages", "api", "openspec")
	if err := os.MkdirAll(filepath.Dir(linkedOpenSpec), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "openspec"), linkedOpenSpec); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	linked := filepath.Join(linkedOpenSpec, "changes", "thin")
	opaque := filepath.Join(root, "packages", "web", "openspec", "changes", "thin")
	if err := os.MkdirAll(opaque, 0o755); err != nil {
		t.Fatal(err)
	}
	runSDDStatusGit(t, opaque, "init", "-q")
	runSDDStatusGit(t, root, "init", "-q")

	got, err := bindingChangeRoots(context.Background(), root, "thin")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{outer, linked, opaque}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bindingChangeRoots() = %v, want %v", got, want)
	}
}

func TestBindApprovedReviewRejectsSuccessfulGitDiagnosticsBeforeRuntimeMutation(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "thin", "- [x] done\n")
	runSDDStatusGit(t, root, "init", "-q")
	runSDDStatusGit(t, root, "config", "core.fsmonitor", "/nonexistent")

	_, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", "")
	if err == nil || !strings.Contains(err.Error(), "diagnostics") {
		t.Fatalf("BindApprovedReview() diagnostic error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".git", "gentle-ai")); !os.IsNotExist(statErr) {
		t.Fatalf("Git diagnostic created runtime authority: %v", statErr)
	}
}

func TestBindApprovedReviewPreflightsGitInventoryBeforeRuntimeMutation(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "thin", "- [x] done\n")
	runSDDStatusGit(t, root, "init", "-q")
	if err := os.WriteFile(filepath.Join(root, ".git", "index"), []byte("corrupt index\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", "")
	var commandErr *reviewtransaction.GitCommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("BindApprovedReview() error = %T %v, want typed Git failure", err, err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".git", "gentle-ai")); !os.IsNotExist(statErr) {
		t.Fatalf("Git inventory failure created runtime authority: %v", statErr)
	}
}

func TestBindApprovedReviewDoesNotFallBackPastNestedPlanningWorkspace(t *testing.T) {
	root := t.TempDir()
	planningRoot := filepath.Join(root, "packages", "app")
	seedReadyChange(t, root, "thin", "- [x] 1.1 Root\n")
	seedReadyChange(t, planningRoot, "other", "- [x] 1.1 Package\n")
	runSDDStatusGit(t, root, "init", "-q")

	if _, err := BindApprovedReview(context.Background(), planningRoot, "thin", "approved-thin", ""); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("nested planning workspace fallback error = %v", err)
	}
}

func TestBindApprovedReviewChecksNestedPlanningLedger(t *testing.T) {
	root := t.TempDir()
	planningRoot := filepath.Join(root, "packages", "app")
	changeRoot := seedReadyChange(t, planningRoot, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
	write(t, filepath.Join(changeRoot, "reviews", "ledger.json"), `{"schema":"gentle-ai.review-ledger/v1","findings":[{"id":"wrong"}]}`)

	if _, err := BindApprovedReview(context.Background(), planningRoot, "thin", "approved-thin", ""); err == nil || !strings.Contains(err.Error(), "ledger does not equal") {
		t.Fatalf("nested planning ledger error = %v", err)
	}
	runtimeStore := mustRuntimeStore(t, root, "thin")
	if _, err := os.Stat(filepath.Join(runtimeStore.Dir, "HEAD")); !os.IsNotExist(err) {
		t.Fatalf("failed nested bind mutated native binding authority: %v", err)
	}
}

func TestResolveConsumesOnlyAnExplicitValidBinding(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")

	withoutBinding, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if withoutBinding.NextRecommended != "verify" || withoutBinding.Dependencies.Verify != DependencyReady {
		t.Fatalf("unbound authority status = %#v", withoutBinding)
	}

	if _, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", ""); err != nil {
		t.Fatal(err)
	}
	bound, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if bound.NextRecommended != "verify" || bound.Dependencies.Verify != DependencyReady || bound.Dependencies.Archive != DependencyBlocked || bound.ReviewGate == nil || bound.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("bound authority status = %#v", bound)
	}
}

func TestBindApprovedReviewRequiresTheSelectedCanonicalChange(t *testing.T) {
	for _, change := range []string{"../escape", "thin-", "thin--binding", strings.Repeat("a", 129)} {
		if _, err := BindApprovedReview(context.Background(), t.TempDir(), change, "approved", ""); err == nil {
			t.Fatalf("invalid change %q was accepted", change)
		}
	}
}

func TestBindApprovedReviewPreservesAuthorityAcrossBindingPublicationFailures(t *testing.T) {
	for _, name := range []string{"HEAD replace", "directory sync"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
			writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, "approved-thin")
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			runtimeStore := mustRuntimeStore(t, root, "thin")
			if err := runtimeStore.ensureDirectories(); err != nil {
				t.Fatal(err)
			}

			originalReplace, originalSync := runtimeReplaceHead, runtimeSyncDirectory
			t.Cleanup(func() { runtimeReplaceHead, runtimeSyncDirectory = originalReplace, originalSync })
			want := "replace native binding HEAD"
			if name == "HEAD replace" {
				runtimeReplaceHead = func(string, string) error { return errors.New(want) }
			} else {
				want = "sync native binding"
				runtimeSyncDirectory = func(path string) error {
					if filepath.Clean(path) == filepath.Clean(runtimeStore.Dir) {
						return errors.New(want)
					}
					return originalSync(path)
				}
			}
			_, err = BindApprovedReview(context.Background(), root, "thin", "approved-thin", "")
			runtimeReplaceHead, runtimeSyncDirectory = originalReplace, originalSync
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("binding publication error = %v, want %q", err, want)
			}
			after, loadErr := store.Load()
			if loadErr != nil || after.Revision != before.Revision || !reflect.DeepEqual(after.State, before.State) {
				t.Fatalf("binding publication changed authority: before=%#v after=%#v error=%v", before, after, loadErr)
			}

			if name == "HEAD replace" {
				if _, statErr := os.Stat(filepath.Join(runtimeStore.Dir, "HEAD")); !os.IsNotExist(statErr) {
					t.Fatalf("failed HEAD replace published binding: %v", statErr)
				}
				if _, retryErr := BindApprovedReview(context.Background(), root, "thin", "approved-thin", ""); retryErr != nil {
					t.Fatalf("HEAD replace retry did not reuse immutable record: %v", retryErr)
				}
				return
			}

			assertNativeBinding(t, runtimeStore, mustBindingRevision(t, root, "thin"))
			runtimeSyncDirectory = func(path string) error {
				if filepath.Clean(path) == filepath.Clean(runtimeStore.Dir) {
					return errors.New("sync native binding again")
				}
				return originalSync(path)
			}
			_, retryErr := BindApprovedReview(context.Background(), root, "thin", "approved-thin", "")
			var publicationErr *ReviewBindingPublicationError
			if !errors.As(retryErr, &publicationErr) {
				t.Fatalf("repeated sync failure = %T %v, want ReviewBindingPublicationError", retryErr, retryErr)
			}
			syncs := 0
			runtimeSyncDirectory = func(path string) error {
				if filepath.Clean(path) == filepath.Clean(runtimeStore.Dir) {
					syncs++
				}
				return originalSync(path)
			}
			_, retryErr = BindApprovedReview(context.Background(), root, "thin", "approved-thin", "")
			runtimeSyncDirectory = originalSync
			if retryErr != nil || syncs != 1 {
				t.Fatalf("binding retry did not repeat native directory sync: syncs=%d err=%v", syncs, retryErr)
			}
		})
	}
}

func TestBindingFailsClosedForLedgerDriftAndChangedLiveEvidence(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(t *testing.T, root, changeRoot string)
	}{
		{name: "mismatched external ledger", mutate: func(t *testing.T, _ string, changeRoot string) {
			if err := os.MkdirAll(filepath.Join(changeRoot, "reviews"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(changeRoot, "reviews", "ledger.json"), []byte(`{"schema":"gentle-ai.review-ledger/v1","findings":[{"id":"wrong"}]}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "staged candidate drift", mutate: func(t *testing.T, root, changeRoot string) {
			if err := os.WriteFile(filepath.Join(changeRoot, "tasks.md"), []byte("- [x] 1.1 Done\n# staged drift\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runSDDStatusGit(t, root, "add", "openspec/changes/thin/tasks.md")
		}},
		{name: "committed candidate drift", mutate: func(t *testing.T, root, changeRoot string) {
			if err := os.WriteFile(filepath.Join(changeRoot, "tasks.md"), []byte("- [x] 1.1 Done\n# committed drift\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runSDDStatusGit(t, root, "add", "openspec/changes/thin/tasks.md")
			runSDDStatusGit(t, root, "commit", "-qm", "drift")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
			writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
			tt.mutate(t, root, changeRoot)
			if _, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", ""); err == nil {
				t.Fatal("changed live evidence created a binding")
			}
			runtimeStore := mustRuntimeStore(t, root, "thin")
			if _, err := os.Stat(filepath.Join(runtimeStore.Dir, "HEAD")); !os.IsNotExist(err) {
				t.Fatalf("failed bind mutated native binding authority: %v", err)
			}
		})
	}
}

func TestResolveRejectsCorruptOrChangedBoundEvidence(t *testing.T) {
	for _, tt := range []struct {
		name, wantNext, wantReason string
		mutate                     func(t *testing.T, root string, store reviewtransaction.CompactStore, binding ReviewBinding)
	}{
		{name: "corrupt binding", wantNext: "resolve-blockers", wantReason: "native SDD runtime authority is unreadable", mutate: func(t *testing.T, root string, _ reviewtransaction.CompactStore, _ ReviewBinding) {
			corruptNativeRuntimeBinding(t, mustRuntimeStore(t, root, "thin"))
		}},
		{name: "changed receipt", wantNext: "resolve-review", wantReason: "receipt", mutate: func(t *testing.T, _ string, store reviewtransaction.CompactStore, _ ReviewBinding) {
			if err := os.WriteFile(store.ReceiptPath(), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
			writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
			binding, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", "")
			if err != nil {
				t.Fatal(err)
			}
			store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, "approved-thin")
			tt.mutate(t, root, store, binding)
			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			if err != nil {
				t.Fatal(err)
			}
			if status.NextRecommended != tt.wantNext || status.Dependencies.Verify != DependencyBlocked {
				t.Fatalf("%s status = %#v", tt.name, status)
			}
			if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), tt.wantReason) {
				t.Fatalf("%s BlockedReasons = %v, want containing %q", tt.name, status.BlockedReasons, tt.wantReason)
			}
		})
	}
}

func TestBoundReviewUsesNormalVerifyThenArchiveRouting(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Binding\n#### Scenario: Exact authority\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
	if _, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", ""); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "pass"))

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Dependencies.Verify != DependencyAllDone || status.Dependencies.Archive != DependencyReady || status.NextRecommended != "archive" || status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("bound completed verification status = %#v", status)
	}
	corruptNativeRuntimeBinding(t, mustRuntimeStore(t, root, "thin"))
	status, err = Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "resolve-blockers" {
		t.Fatalf("corrupt completed binding status = %#v", status)
	}
}

func TestBoundReviewRoutesStaleVerifyEvidenceToVerify(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Binding\n#### Scenario: Exact authority\n#### Scenario: Added after verification\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
	if _, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", ""); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "pass"))

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("ReviewGate = %#v, want allow", status.ReviewGate)
	}
	if len(status.BlockedReasons) != 0 {
		t.Fatalf("BlockedReasons = %v, want empty", status.BlockedReasons)
	}
	if status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "verify" {
		t.Fatalf("verify=%q archive=%q next=%q, want ready/blocked/verify", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended)
	}
	if status.RemediationState != (RemediationState{}) {
		t.Fatalf("RemediationState = %#v, want empty for stale evidence", status.RemediationState)
	}
}

func TestBoundReviewGrantsCompactRemediationBudgetForFailedVerdictWithIncompleteScenarios(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Binding\n#### Scenario: Exact authority\n#### Scenario: Added after verification\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
	if _, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", ""); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "fail"))

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Dependencies.Verify != DependencyBlocked || status.NextRecommended != "remediate" {
		t.Fatalf("verify=%q next=%q, want blocked/remediate for failed verdict", status.Dependencies.Verify, status.NextRecommended)
	}
	if !status.RemediationState.Required || status.RemediationState.CorrectionBudgetRemaining <= 0 || status.RemediationState.LineageID != "approved-thin" || status.RemediationState.FailedEvidenceRevision != shaID("a") {
		t.Fatalf("RemediationState = %#v, want transaction-bound nonzero compact budget", status.RemediationState)
	}
}

func TestSelectedBindingSupersedesOnlyItsLegacyReviewAuthority(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Binding\n#### Scenario: Exact authority\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "pass"))
	writeApprovedReviewArtifacts(t, changeRoot)
	if err := os.Remove(filepath.Join(changeRoot, "verify-report.md")); err != nil {
		t.Fatal(err)
	}
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
	if _, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", ""); err != nil {
		t.Fatal(err)
	}
	legacy, err := reviewtransaction.AuthoritativeStore(context.Background(), root, "thin-lineage")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.LoadChain(); err != nil {
		t.Fatalf("binding removed or changed legacy authority: %v", err)
	}
	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.NextRecommended != "verify" || status.Dependencies.Verify != DependencyReady || status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("selected binding did not supersede only the selected legacy authority: %#v", status)
	}
}

func TestValidBindingDoesNotAdvanceIncompleteApply(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [ ] 1.1 Pending\n")
	writeApprovedCompactAuthorityForChangeWithTasks(t, root, changeRoot, "approved-thin", "- [ ] 1.1 Pending\n# approved compact scope\n")
	if _, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", ""); err != nil {
		t.Fatal(err)
	}
	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.ApplyState != ApplyReady || status.Dependencies.Apply != DependencyReady || status.Dependencies.Verify != DependencyBlocked || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "apply" {
		t.Fatalf("incomplete bound status = %#v", status)
	}
}

func TestBindApprovedReviewSanitizesHostileGitEnvironmentFromSubdirectory(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
	subdirectory := filepath.Join(root, "nested")
	if err := os.MkdirAll(subdirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	hostile := t.TempDir()
	runSDDStatusGit(t, hostile, "init", "-q")
	for name, value := range map[string]string{
		"GIT_DIR":        filepath.Join(hostile, ".git"),
		"GIT_WORK_TREE":  hostile,
		"GIT_COMMON_DIR": filepath.Join(hostile, ".git"),
		"GIT_INDEX_FILE": filepath.Join(hostile, ".git", "index"),
	} {
		t.Setenv(name, value)
	}
	t.Chdir(root)
	if _, err := BindApprovedReview(context.Background(), "nested", "thin", "approved-thin", ""); err != nil {
		t.Fatal(err)
	}
	assertNativeBinding(t, mustRuntimeStore(t, root, "thin"), mustBindingRevision(t, root, "thin"))
}

func mustRuntimeStore(t *testing.T, repo, change string) RuntimeStore {
	t.Helper()
	store, err := OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func mustBindingRevision(t *testing.T, repo, change string) string {
	t.Helper()
	status, err := mustRuntimeStore(t, repo, change).Status()
	if err != nil || status.Binding == nil {
		t.Fatalf("native binding status = %#v, err=%v", status, err)
	}
	return status.BindingRevision
}

func assertNativeBinding(t *testing.T, store RuntimeStore, want string) {
	t.Helper()
	status, err := store.Status()
	if err != nil || status.Binding == nil || status.Binding.Revision != want || status.BindingRevision != want {
		t.Fatalf("native binding status = %#v, err=%v, want=%q", status, err, want)
	}
	legacyPath := filepath.Join(store.commonDir, "gentle-ai", "sdd-review-bindings", "v1", store.Change, "binding.json")
	if _, statErr := os.Stat(legacyPath); !os.IsNotExist(statErr) {
		t.Fatalf("native bind unexpectedly dual-wrote legacy compatibility artifact: %v", statErr)
	}
}

func corruptNativeRuntimeBinding(t *testing.T, store RuntimeStore) {
	t.Helper()
	status, err := store.Status()
	if err != nil || status.Revision == "" {
		t.Fatalf("load native binding before corruption: status=%#v err=%v", status, err)
	}
	path := filepath.Join(store.Dir, "records", strings.TrimPrefix(status.Revision, "sha256:")+".json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestParseBindingNamesTheConditionItFailed is root 8 (#2471) on the SDD
// binding surface: twelve integrity conditions used to answer with one shared
// "invalid binding", so an operator holding that error knew only that bytes
// this package wrote itself were wrong. These are OUR persisted ledger bytes,
// so a violation is tamper or corruption, which is exactly why the condition
// has to be named: it decides whether the reader escalates or repairs.
func TestParseBindingNamesTheConditionItFailed(t *testing.T) {
	valid := ReviewBinding{
		Schema: reviewBindingSchema, Change: "root8-binding", Lineage: "root8-lineage",
		AuthorityRevision: "sha256:" + strings.Repeat("a", 64),
		ReceiptHash:       "sha256:" + strings.Repeat("b", 64),
		GateContext: reviewtransaction.GateContext{
			Gate: reviewtransaction.GatePostApply, LineageID: "root8-lineage",
			StoreRevision: "sha256:" + strings.Repeat("a", 64),
		},
	}
	valid.Revision = bindingDigest(valid)
	payload, err := bindingBytes(valid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseBinding(payload); err != nil {
		t.Fatalf("valid fixture rejected: %v", err)
	}

	for name, corrupt := range map[string]func(ReviewBinding) ReviewBinding{
		"schema is not":                    func(b ReviewBinding) ReviewBinding { b.Schema = "gentle-ai.wrong/v1"; return b },
		"change name is not":               func(b ReviewBinding) ReviewBinding { b.Change = "../escape"; return b },
		"lineage is not":                   func(b ReviewBinding) ReviewBinding { b.Lineage = "not a lineage!"; return b },
		"receipt_hash is not":              func(b ReviewBinding) ReviewBinding { b.ReceiptHash = "notadigest"; return b },
		"gate_context.gate is not":         func(b ReviewBinding) ReviewBinding { b.GateContext.Gate = reviewtransaction.GatePreCommit; return b },
		"gate_context.lineage_id does not": func(b ReviewBinding) ReviewBinding { b.GateContext.LineageID = "other-lineage"; return b },
		"gate_context.store_revision does not": func(b ReviewBinding) ReviewBinding {
			b.GateContext.StoreRevision = "sha256:" + strings.Repeat("c", 64)
			return b
		},
	} {
		t.Run(name, func(t *testing.T) {
			broken := corrupt(valid)
			broken.Revision = bindingDigest(broken)
			brokenPayload, err := bindingBytes(broken)
			if err != nil {
				t.Fatal(err)
			}
			_, parseErr := parseBinding(brokenPayload)
			if parseErr == nil {
				t.Fatal("corrupted binding was accepted")
			}
			if !strings.Contains(parseErr.Error(), name) {
				t.Fatalf("error %q does not name the failed condition %q", parseErr, name)
			}
		})
	}

	// The digest condition needs its own case: it is the one violation that
	// cannot be reached by recomputing Revision after corrupting a field.
	tampered := valid
	tampered.Revision = "sha256:" + strings.Repeat("d", 64)
	tamperedPayload, err := bindingBytes(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseBinding(tamperedPayload); err == nil ||
		!strings.Contains(err.Error(), "revision does not match the digest") {
		t.Fatalf("tampered revision error = %v", err)
	}
}
