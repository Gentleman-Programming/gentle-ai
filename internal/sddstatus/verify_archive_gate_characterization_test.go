package sddstatus

// This file characterizes the verify -> archive gate's observable behavior
// through the public Resolve entry point. It was written in two passes, both
// recorded here rather than silently overwritten, per this repository's own
// characterization-test discipline (INC-19 D-03,
// create_increment_characterization_test.go): every assertion compares full
// field values (structs, exact strings, exact slices), never substrings
// (strings.Contains).
//
// Pass 1 (2026-09-20, before task 8.3 touched status.go/verification.go)
// pinned the PRE-absorption contract: Archive only reached "ready" once a
// CURRENT and PASSING verify report existed (the old resolveDependencies).
// Pass 2 (2026-09-20, immediately after re-deriving status.go from upstream's
// 62ce74b7 "make verification optional and archive without attestation")
// updates TestCharacterization_TasksIncompleteBlocksVerifyAndArchive's
// Verify/Archive expectations and replaces the old
// TestCharacterization_PreF4MandatoryVerificationGatesArchive with
// TestCharacterization_PostF4OptionalVerificationNeverGatesArchive below,
// transcribed from this package's real output on the new status.go -- the
// same real-machine's Docker Linux verification run is what surfaced pass
// 1's now-stale expectations as failures, exactly as this file's own pass-1
// comment said it should.
//
// TestCharacterization_PlanningArtifactsRouteBeforeVerify,
// TestCharacterization_TasksIncompleteBlocksVerifyAndArchive (its
// NextRecommended assertion, not its Dependencies one) and
// TestCharacterization_EditAuthorityMissingBlocksApplyAndArchive exercise
// gate behavior that 62ce74b7 does not touch and stayed green across both
// passes without any changes to their assertions.

import (
	"fmt"
	"path/filepath"
	"testing"
)

// exactDependencies fails with full-struct evidence (never a per-field
// substring check) when got does not equal want byte-for-byte.
func exactDependencies(t *testing.T, label string, got, want Dependencies) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: Dependencies = %#v, want %#v", label, got, want)
	}
}

// exactBlockedReasons fails with full-slice evidence when got does not
// equal want element-for-element (length and content), never a substring
// membership check.
func exactBlockedReasons(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: BlockedReasons = %#v (len=%d), want %#v (len=%d)", label, got, len(got), want, len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: BlockedReasons = %#v, want %#v", label, got, want)
		}
	}
}

// TestCharacterization_PlanningArtifactsRouteBeforeVerify pins that, with no
// planning artifacts on disk at all, Resolve routes to "propose" and leaves
// Verify and Archive blocked. Orthogonal to 62ce74b7 (research/verification
// optionality never relaxes the propose/spec/design/tasks ordering).
func TestCharacterization_PlanningArtifactsRouteBeforeVerify(t *testing.T) {
	root := t.TempDir()
	const change = "characterization-no-artifacts"
	mkdir(t, filepath.Join(root, "openspec", "changes", change))

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: change, IncludeInstructions: false})
	if err != nil {
		t.Fatal(err)
	}
	if status.NextRecommended != "propose" {
		t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, "propose")
	}
	exactDependencies(t, "no artifacts", status.Dependencies, Dependencies{
		Proposal: DependencyBlocked,
		Specs:    DependencyBlocked,
		Design:   DependencyBlocked,
		Tasks:    DependencyBlocked,
		Apply:    DependencyBlocked,
		Verify:   DependencyBlocked,
		Archive:  DependencyBlocked,
	})
}

// TestCharacterization_TasksIncompleteBlocksVerifyAndArchive pins that, once
// proposal/specs/design/tasks all exist but at least one task is still
// unchecked, Apply is "ready" and NextRecommended is "apply" -- that routing
// choice is orthogonal to 62ce74b7 and stayed green across both passes
// unchanged (resolveNextRecommended still checks dependencies.Apply ==
// DependencyReady first, before Verify or Archive).
//
// Its Verify/Archive dependency values are NOT orthogonal, and pass 2 updates
// them: post-62ce74b7, resolveDependencies sets Verify/Archive to "ready"
// from coreReady and a non-blocked applyState alone, with no dependency on
// TaskProgress.AllComplete at all -- "ready" now means "not structurally
// blocked", not "recommended now" (that distinction is NextRecommended's job,
// and it still correctly says "apply" here, checked above). Upstream's own
// ported TestOptionalVerificationDoesNotAuthorizeOrGateArchive asserts this
// same "Archive always ready once coreReady" shape for its own partial-tasks
// cases, so this is not a local regression -- it is upstream's shipped
// design, transcribed here.
func TestCharacterization_TasksIncompleteBlocksVerifyAndArchive(t *testing.T) {
	root := t.TempDir()
	const change = "characterization-tasks-incomplete"
	seedReadyChange(t, root, change, "- [x] 1.1 Done\n- [ ] 1.2 Pending\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: change, IncludeInstructions: false})
	if err != nil {
		t.Fatal(err)
	}
	if status.NextRecommended != "apply" {
		t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, "apply")
	}
	exactDependencies(t, "tasks incomplete", status.Dependencies, Dependencies{
		Proposal: DependencyAllDone,
		Specs:    DependencyAllDone,
		Design:   DependencyAllDone,
		Tasks:    DependencyAllDone,
		Apply:    DependencyReady,
		Verify:   DependencyReady,
		Archive:  DependencyReady,
	})
	if status.TaskProgress != (TaskProgress{Total: 2, Completed: 1, Pending: 1, AllComplete: false}) {
		t.Fatalf("TaskProgress = %#v, want {Total:2 Completed:1 Pending:1 AllComplete:false}", status.TaskProgress)
	}
}

// TestCharacterization_EditAuthorityMissingBlocksApplyAndArchive pins that a
// task naming a path outside the resolved workspace root blocks Apply and
// Archive with the exact "edit_authority_missing" reason. Upstream's own
// ported TestOptionalVerificationRetainsEditAuthorityBlock keeps this
// unchanged after 62ce74b7, so this scenario is stable across the whole
// re-derivation, not just before it.
func TestCharacterization_EditAuthorityMissingBlocksApplyAndArchive(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	const change = "characterization-edit-authority"
	tasks := "- [ ] Edit `" + filepath.ToSlash(filepath.Join(outside, "main.go")) + "`\n"
	seedReadyChange(t, root, change, tasks)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: change, IncludeInstructions: false})
	if err != nil {
		t.Fatal(err)
	}
	if status.ApplyState != ApplyBlocked {
		t.Fatalf("ApplyState = %q, want %q", status.ApplyState, ApplyBlocked)
	}
	if status.Dependencies.Archive != DependencyBlocked {
		t.Fatalf("Dependencies.Archive = %q, want %q", status.Dependencies.Archive, DependencyBlocked)
	}
	// Transcribed verbatim from this test's real output on 2026-09-20 (D-03
	// precedent), parameterized only by this run's own root/outside temp
	// dirs -- never a substring check.
	exactBlockedReasons(t, "edit authority missing", status.BlockedReasons, []string{
		fmt.Sprintf("blocked(edit_authority_missing): tasks.md targets edit paths outside the authorized edit roots: \"%s\"; edit tasks.md so every work unit stays inside the authorized edit roots, or grant this change edit authority for the named paths, or mark a read-only input with (read-only) right after its backticked path", outside),
		fmt.Sprintf("Run `gentle-ai sdd-continue \"%s\" --cwd \"%s\"` with authorized change-directory writes to prepare the required marker; this grants no edit roots.", change, root),
	})
}

// TestCharacterization_PostF4OptionalVerificationNeverGatesArchive pins the
// POST-absorption contract (superseding
// TestCharacterization_PreF4MandatoryVerificationGatesArchive, removed in
// this same pass): once every task is complete and apply is done, Archive
// reaches "ready" and NextRecommended is "archive" regardless of whether a
// verify report exists, is missing, or is passing -- verification is a
// purely informational diagnostic now, matching upstream's shipped
// TestOptionalVerificationDoesNotAuthorizeOrGateArchive. Verify itself caps
// at "ready", never reaching the old "all_done" state, because
// resolveDependencies no longer has a passing/current-report-driven upgrade
// path at all. Both sub-tests are transcribed from this package's real
// output on 2026-09-20, immediately after re-deriving status.go.
func TestCharacterization_PostF4OptionalVerificationNeverGatesArchive(t *testing.T) {
	t.Run("no_report_at_all_still_reaches_archive", func(t *testing.T) {
		root := t.TempDir()
		const change = "characterization-no-report"
		seedReadyChange(t, root, change, "- [x] 1.1 Done\n")

		status, err := Resolve(ResolveOptions{CWD: root, ChangeName: change, IncludeInstructions: false})
		if err != nil {
			t.Fatal(err)
		}
		if status.NextRecommended != "archive" {
			t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, "archive")
		}
		exactDependencies(t, "no report", status.Dependencies, Dependencies{
			Proposal: DependencyAllDone,
			Specs:    DependencyAllDone,
			Design:   DependencyAllDone,
			Tasks:    DependencyAllDone,
			Apply:    DependencyAllDone,
			Verify:   DependencyReady,
			Archive:  DependencyReady,
		})
	})

	t.Run("passing_current_report_still_reaches_archive", func(t *testing.T) {
		root := t.TempDir()
		const change = "characterization-passing-report"
		changeRoot := seedReadyChange(t, root, change, "- [x] 1.1 Done\n")
		write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0))

		status, err := Resolve(ResolveOptions{CWD: root, ChangeName: change, IncludeInstructions: false})
		if err != nil {
			t.Fatal(err)
		}
		if status.NextRecommended != "archive" {
			t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, "archive")
		}
		exactDependencies(t, "passing report", status.Dependencies, Dependencies{
			Proposal: DependencyAllDone,
			Specs:    DependencyAllDone,
			Design:   DependencyAllDone,
			Tasks:    DependencyAllDone,
			Apply:    DependencyAllDone,
			Verify:   DependencyReady,
			Archive:  DependencyReady,
		})
		exactBlockedReasons(t, "passing report", status.BlockedReasons, []string{})
	})
}
