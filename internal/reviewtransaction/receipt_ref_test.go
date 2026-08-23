//go:build legacy_compact_receipt

package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// This file is Wave 4 S5 (design.md decision 1, tasks 6.1-6.3, 6.9-6.10):
// SDD's terminal pointer into review authority. SDDReceiptRef is deliberately
// exactly two fields — "what do I ask about" (Lineage) and "which bytes did
// I see" (ReceiptHash) — so validity is always one call, never a second
// store SDD would have to keep in sync.

// TestSDDReceiptRefIsExactlyTwoFields is task 6.1's RED/GREEN: a third field
// would be a value SDD has to keep in sync — the mirror this wave retires.
// Modelled on TestRuntimeObjectiveIsSoleWorkUnitScopeOwner (Wave 4 S2).
func TestSDDReceiptRefIsExactlyTwoFields(t *testing.T) {
	refType := reflect.TypeOf(SDDReceiptRef{})
	if refType.NumField() != 2 {
		t.Fatalf("SDDReceiptRef has %d fields, want exactly 2 (Lineage, ReceiptHash)", refType.NumField())
	}
	for index, wantName := range []string{"Lineage", "ReceiptHash"} {
		if got := refType.Field(index).Name; got != wantName {
			t.Fatalf("SDDReceiptRef.Field(%d) = %q, want %q", index, got, wantName)
		}
	}
}

// TestSDDReceiptRefJSONRoundTripRejectsExtraFields proves a SDDReceiptRef decoded
// from untrusted JSON with an extra field is refused, not silently
// truncated to the known two — a third field arriving over the wire is
// exactly the re-derivation shape decision 1 forbids, and it must fail
// loudly rather than be quietly dropped.
func TestSDDReceiptRefJSONRoundTripRejectsExtraFields(t *testing.T) {
	clean := `{"lineage":"thin-lineage","receipt_hash":"sha256:` + shaIDForTest("a") + `"}`
	var ref SDDReceiptRef
	if err := strictDecodeSDDReceiptRef([]byte(clean), &ref); err != nil {
		t.Fatalf("strictDecodeSDDReceiptRef(clean) error = %v", err)
	}
	if ref.Lineage != "thin-lineage" {
		t.Fatalf("ref.Lineage = %q, want thin-lineage", ref.Lineage)
	}

	withExtra := `{"lineage":"thin-lineage","receipt_hash":"sha256:` + shaIDForTest("a") + `","authority_revision":"sha256:` + shaIDForTest("b") + `"}`
	var polluted SDDReceiptRef
	if err := strictDecodeSDDReceiptRef([]byte(withExtra), &polluted); err == nil {
		t.Fatal("strictDecodeSDDReceiptRef(withExtra) = nil error, want a rejection of the unknown field")
	}
}

func shaIDForTest(char string) string {
	out := make([]byte, 64)
	for index := range out {
		out[index] = char[0]
	}
	return string(out)
}

// TestValidateSDDReceiptRefGitRepositorySelection is task 6.9/6.10's threat-
// matrix (Git repository selection): relative --cwd, linked worktree,
// symlinked common dir, and foreign-repo refusal, proven through the exact
// same SnapshotBuilder.ResolveRepositoryRoot common-dir authority every
// other native entry point already uses.
func TestValidateSDDReceiptRefGitRepositorySelection(t *testing.T) {
	t.Run("relative cwd resolves through the working directory", func(t *testing.T) {
		root := t.TempDir()
		runGitForTest(t, root, "init", "-q")
		runGitForTest(t, root, "config", "user.email", "receipt-ref@example.com")
		runGitForTest(t, root, "config", "user.name", "Receipt Ref Test")
		if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGitForTest(t, root, "add", ".")
		runGitForTest(t, root, "commit", "-qm", "base")

		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(cwd) }()
		if err := os.Chdir(root); err != nil {
			t.Fatal(err)
		}

		result, reason, err := ValidateSDDReceiptRef(context.Background(), ".", SDDReceiptRef{Lineage: "missing-lineage", ReceiptHash: "sha256:" + shaIDForTest("c")})
		if err != nil {
			t.Fatalf("ValidateSDDReceiptRef(relative cwd) error = %v", err)
		}
		if result == GateAllow {
			t.Fatalf("ValidateSDDReceiptRef(relative cwd, no authority) = allow, reason=%q, want a non-allow result for a lineage with no compact authority", reason)
		}
	})

	t.Run("foreign repository refuses", func(t *testing.T) {
		root := t.TempDir()
		result, reason, err := ValidateSDDReceiptRef(context.Background(), root, SDDReceiptRef{Lineage: "any-lineage", ReceiptHash: "sha256:" + shaIDForTest("d")})
		if err == nil && result == GateAllow {
			t.Fatalf("ValidateSDDReceiptRef(non-repository) = allow, reason=%q, want refusal or non-allow for a path that is not a Git repository", reason)
		}
	})

	t.Run("linked worktree resolves through its own common dir", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		linked := filepath.Join(t.TempDir(), "linked")
		if err := runSnapshotGit(repo, "worktree", "add", "-b", "receipt-ref-linked", linked, "HEAD"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = runSnapshotGit(repo, "worktree", "remove", "--force", linked) })

		result, reason, err := ValidateSDDReceiptRef(context.Background(), linked, SDDReceiptRef{Lineage: "missing-lineage", ReceiptHash: "sha256:" + shaIDForTest("e")})
		if err != nil {
			t.Fatalf("ValidateSDDReceiptRef(linked worktree) error = %v", err)
		}
		if result == GateAllow {
			t.Fatalf("ValidateSDDReceiptRef(linked worktree, no authority) = allow, reason=%q, want a non-allow result for a lineage with no compact authority", reason)
		}
	})

	t.Run("symlinked common dir resolves to the same authority", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		alias := t.TempDir() + "-alias"
		if err := os.Symlink(repo, alias); err != nil {
			t.Skipf("symlink unsupported on this platform: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(alias) })

		result, reason, err := ValidateSDDReceiptRef(context.Background(), alias, SDDReceiptRef{Lineage: "missing-lineage", ReceiptHash: "sha256:" + shaIDForTest("f")})
		if err != nil {
			t.Fatalf("ValidateSDDReceiptRef(symlinked repo) error = %v", err)
		}
		if result == GateAllow {
			t.Fatalf("ValidateSDDReceiptRef(symlinked repo, no authority) = allow, reason=%q, want a non-allow result for a lineage with no compact authority", reason)
		}
	})
}

func runGitForTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	_, err := runGit(context.Background(), dir, nil, nil, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
}
