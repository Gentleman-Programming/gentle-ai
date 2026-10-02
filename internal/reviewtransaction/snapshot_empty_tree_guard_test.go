package reviewtransaction

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestSnapshotBuilderBuildRefusesUnresolvableBaseTree verifies that Build()
// returns an error (never a zero-value baseTree) when the base_ref cannot
// resolve to a Git tree. Regression guard for start-candidate-context-failure.
func TestSnapshotBuilderBuildRefusesUnresolvableBaseTree(t *testing.T) {
	repo := initSnapshotRepo(t)
	builder := SnapshotBuilder{Repo: repo}

	snapshot, err := builder.Build(t.Context(), Target{
		Kind:       TargetBaseDiff,
		BaseRef:    "refs/heads/nonexistent-that-does-not-exist",
		Projection: ProjectionWorkspace,
	})

	if err == nil {
		t.Fatal("Build with unresolvable base_ref = nil error, want error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "base_ref") && !strings.Contains(errMsg, "nonexistent") {
		t.Logf("error should reference the unresolvable ref: %q", errMsg)
	}

	// Snapshot must be zero-value.
	if snapshot.BaseTree != "" {
		t.Errorf("BaseTree = %q on error, want empty", snapshot.BaseTree)
	}
}

// TestSnapshotBuilderBuildRejectsEmptyCandidateTree verifies that Build()
// refuses when the candidate tree cannot be resolved.
func TestSnapshotBuilderBuildRejectsEmptyCandidateTree(t *testing.T) {
	repo := initSnapshotRepo(t)
	builder := SnapshotBuilder{Repo: repo}

	_, err := builder.Build(t.Context(), Target{
		Kind:       TargetExactRevision,
		Revision:   "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		Projection: ProjectionWorkspace,
	})

	if err == nil {
		t.Fatal("Build with nonexistent revision = nil error, want error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "revision") && !strings.Contains(errMsg, "deadbeef") {
		t.Logf("error should mention the bad revision: %q", errMsg)
	}
}

// TestSnapshotBuilderBuildSnapshotNonEmptyOnSuccess verifies that a successful
// Build() always produces non-empty BaseTree and CandidateTree.
func TestSnapshotBuilderBuildSnapshotNonEmptyOnSuccess(t *testing.T) {
	t.Run("base-diff", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		builder := SnapshotBuilder{Repo: repo}

		snapshot, err := builder.Build(t.Context(), Target{
			Kind:       TargetBaseDiff,
			BaseRef:    "HEAD",
			Projection: ProjectionWorkspace,
		})
		if err != nil {
			t.Fatalf("Build error = %v", err)
		}

		if snapshot.BaseTree == "" {
			t.Error("BaseTree is empty on success")
		}
		if snapshot.CandidateTree == "" {
			t.Error("CandidateTree is empty on success")
		}
		if len(snapshot.BaseTree) < 7 {
			t.Errorf("BaseTree too short: %q", snapshot.BaseTree)
		}
		if len(snapshot.CandidateTree) < 7 {
			t.Errorf("CandidateTree too short: %q", snapshot.CandidateTree)
		}
	})

	t.Run("current-changes", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "new.txt", "new\n")
		builder := SnapshotBuilder{Repo: repo}

		snapshot, err := builder.Build(t.Context(), Target{
			Kind:              TargetCurrentChanges,
			IntendedUntracked: []string{"new.txt"},
		})
		if err != nil {
			t.Fatalf("Build error = %v", err)
		}

		if snapshot.BaseTree == "" {
			t.Error("BaseTree is empty on success")
		}
		if snapshot.CandidateTree == "" {
			t.Error("CandidateTree is empty on success")
		}
	})
}

// TestSnapshotBuilderBuildBaseWorkspaceOverlayRejectsEmptyBase verifies that
// TargetBaseWorkspaceOverlay with an unresolvable base_ref returns an error.
func TestSnapshotBuilderBuildBaseWorkspaceOverlayRejectsEmptyBase(t *testing.T) {
	repo := initSnapshotRepo(t)
	builder := SnapshotBuilder{Repo: repo}

	snapshot, err := builder.Build(t.Context(), Target{
		Kind:              TargetBaseWorkspaceOverlay,
		BaseRef:           "refs/heads/does-not-exist-either",
		Projection:        ProjectionWorkspace,
		IntendedUntracked: []string{},
	})

	if err == nil {
		t.Fatal("Build(TargetBaseWorkspaceOverlay) with bad base_ref = nil, want error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "base_ref") && !strings.Contains(errMsg, "does-not-exist") {
		t.Logf("error should mention base_ref: %q", errMsg)
	}

	if snapshot.BaseTree != "" {
		t.Errorf("BaseTree = %q on error, want empty", snapshot.BaseTree)
	}
}

// TestBlankTreeGuardsExerciseEmptyGitOutput proves the SnapshotBuilder's
// field-specific refusals when just one tree command succeeds with blank output.
func TestBlankTreeGuardsExerciseEmptyGitOutput(t *testing.T) {
	// Reuse the test executable for a portable, successful Git-output stand-in.
	if len(os.Args) >= 3 && os.Args[len(os.Args)-2] == "--blank-tree-output" {
		fmt.Print(os.Args[len(os.Args)-1])
		os.Exit(0)
	}
	repo := initSnapshotRepo(t)
	builder := SnapshotBuilder{Repo: repo}
	// Use a concrete base tree so candidate injection cannot also hit the base.
	baseTree, err := builder.resolveTree(t.Context(), "HEAD")
	if err != nil || baseTree == "" {
		t.Fatalf("resolve valid base: %q, %v", baseTree, err)
	}
	for _, field := range []struct {
		name string
		args []string
		want string
	}{
		{"base", []string{"rev-parse", "--verify", baseTree + "^{tree}"}, "base tree empty after resolving target base-diff; review the repository state and rerun with a valid base_ref"},
		{"candidate", []string{"write-tree"}, "candidate tree empty after building target base-diff; the working tree or staged index may be corrupted"},
	} {
		for _, output := range []struct{ name, value string }{{"empty", ""}, {"whitespace", " \t\r\n"}} {
			t.Run(field.name+"/"+output.name, func(t *testing.T) {
				original := gitCommandContext
				t.Cleanup(func() { gitCommandContext = original })
				intercepted := 0
				gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
					// runGit prepends --no-replace-objects -C <repo>.
					if name == "git" && len(args) >= 3 && strings.Join(args[3:], "\x00") == strings.Join(field.args, "\x00") {
						intercepted++
						// runGit replaces Cmd.Env; pass helper control in argv.
						return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBlankTreeGuardsExerciseEmptyGitOutput$", "--", "--blank-tree-output", output.value)
					}
					return original(ctx, name, args...)
				}
				snapshot, err := builder.Build(t.Context(), Target{
					Kind: TargetBaseDiff, BaseRef: baseTree, Projection: ProjectionWorkspace,
				})
				if intercepted != 1 || err == nil || err.Error() != field.want {
					t.Fatalf("%s guard: intercepted %d, error %v; want %q", field.name, intercepted, err, field.want)
				}
				if snapshot.BaseTree != "" || snapshot.CandidateTree != "" {
					t.Fatalf("blank tree refusal returned partial snapshot: %#v", snapshot)
				}
			})
		}
	}
}
