package main

import (
	"context"
	"testing"
)

// TestRejectsGOOSMismatch verifies Verify fails closed when GOOS differs.
func TestRejectsGOOSMismatch(t *testing.T) {
	ctx := context.Background()
	observed := Identity{
		TargetBinarySHA256:  "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		EmbeddedVCSRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		VCSModified:         false,
		SourceRevision:      "cccccccccccccccccccccccccccccccccccccccc",
		ClassifierVersion:   "v0.1.0",
		RuntimeGOOS:         "darwin",
		RuntimeGOARCH:       "amd64",
		Mode:                "driven",
	}
	expected := observed
	expected.RuntimeGOOS = "linux"

	err := Verify(ctx, expected, observed)
	if err == nil {
		t.Fatal("Verify did not reject GOOS mismatch")
	}
}

// TestRejectsGOARCHMismatch verifies Verify fails closed when GOARCH differs.
func TestRejectsGOARCHMismatch(t *testing.T) {
	ctx := context.Background()
	observed := Identity{
		TargetBinarySHA256:  "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		EmbeddedVCSRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		VCSModified:         false,
		SourceRevision:      "cccccccccccccccccccccccccccccccccccccccc",
		ClassifierVersion:   "v0.1.0",
		RuntimeGOOS:         "linux",
		RuntimeGOARCH:       "386",
		Mode:                "driven",
	}
	expected := observed
	expected.RuntimeGOARCH = "amd64"

	err := Verify(ctx, expected, observed)
	if err == nil {
		t.Fatal("Verify did not reject GOARCH mismatch")
	}
}

// TestRejectsVCSRevisionMismatch verifies Verify fails closed when VCS revisions differ.
func TestRejectsVCSRevisionMismatch(t *testing.T) {
	ctx := context.Background()
	observed := Identity{
		TargetBinarySHA256:  "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		EmbeddedVCSRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		VCSModified:         false,
		SourceRevision:      "cccccccccccccccccccccccccccccccccccccccc",
		ClassifierVersion:   "v0.1.0",
		RuntimeGOOS:         "linux",
		RuntimeGOARCH:       "amd64",
		Mode:                "driven",
	}
	expected := observed
	expected.EmbeddedVCSRevision = "dddddddddddddddddddddddddddddddddddddddd"

	err := Verify(ctx, expected, observed)
	if err == nil {
		t.Fatal("Verify did not reject VCS revision mismatch")
	}
}

// TestRejectsVCSModifiedTrue verifies Verify fails closed when the binary was built
// from a dirty tree.
func TestRejectsVCSModifiedTrue(t *testing.T) {
	ctx := context.Background()
	observed := Identity{
		TargetBinarySHA256:  "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		EmbeddedVCSRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		VCSModified:         true,
		SourceRevision:      "cccccccccccccccccccccccccccccccccccccccc",
		ClassifierVersion:   "v0.1.0",
		RuntimeGOOS:         "linux",
		RuntimeGOARCH:       "amd64",
		Mode:                "driven",
	}
	expected := observed
	expected.VCSModified = false

	err := Verify(ctx, expected, observed)
	if err == nil {
		t.Fatal("Verify did not reject VCSModified=true")
	}
}

// TestRejectsTargetSHA256Mismatch verifies Verify fails closed when the binary digest
// differs from the expected value.
func TestRejectsTargetSHA256Mismatch(t *testing.T) {
	ctx := context.Background()
	observed := Identity{
		TargetBinarySHA256:  "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		EmbeddedVCSRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		VCSModified:         false,
		SourceRevision:      "cccccccccccccccccccccccccccccccccccccccc",
		ClassifierVersion:   "v0.1.0",
		RuntimeGOOS:         "linux",
		RuntimeGOARCH:       "amd64",
		Mode:                "driven",
	}
	expected := observed
	expected.TargetBinarySHA256 = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	err := Verify(ctx, expected, observed)
	if err == nil {
		t.Fatal("Verify did not reject SHA-256 mismatch")
	}
}

// TestPassesExactTuple verifies Verify returns nil when all fields match exactly.
func TestPassesExactTuple(t *testing.T) {
	ctx := context.Background()
	observed := Identity{
		TargetBinarySHA256:  "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		EmbeddedVCSRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		VCSModified:         false,
		SourceRevision:      "cccccccccccccccccccccccccccccccccccccccc",
		ClassifierVersion:   "v0.1.0",
		RuntimeGOOS:         "linux",
		RuntimeGOARCH:       "amd64",
		Mode:                "driven",
	}
	expected := observed // exact match

	err := Verify(ctx, expected, observed)
	if err != nil {
		t.Fatalf("Verify rejected exact tuple: %v", err)
	}
}
