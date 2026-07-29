package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResnapshotVerificationSubjectRequiresEverySnapshotFieldEqual(
	t *testing.T,
) {
	repo := initSnapshotRepo(t)
	path := filepath.Join(repo, "candidate.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := SnapshotBuilder{Repo: repo}
	expected, err := builder.Build(
		context.Background(),
		Target{
			Kind:              TargetCurrentChanges,
			IntendedUntracked: []string{"candidate.txt"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	live, err := ResnapshotVerificationSubject(
		context.Background(),
		repo,
		expected,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !SnapshotsEqualExact(expected, live) {
		t.Fatal("exact post-verification resnapshot changed an equal subject")
	}

	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	observed, err := ResnapshotVerificationSubject(
		context.Background(),
		repo,
		expected,
	)
	if !errors.Is(err, ErrVerificationSubjectMutated) {
		t.Fatalf("mutated resnapshot error = %v", err)
	}
	if SnapshotsEqualExact(expected, observed) ||
		expected.Identity == observed.Identity {
		t.Fatalf(
			"mutated resnapshot did not preserve distinct identities: %#v / %#v",
			expected,
			observed,
		)
	}
}
