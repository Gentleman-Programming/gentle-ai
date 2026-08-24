//go:build legacy_compact_receipt

package sddstatus

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// This file is Wave 4 S5 (design.md decision 2, task 6.4): the read-only
// migration path for an existing legacy gentle-ai.sdd-review-binding/v1
// file. parseLegacyBinding reuses parseBinding's unchanged validation and
// projects the result, in memory only, to a reviewtransaction.SDDReceiptRef
// -- it never rewrites and never unlinks the source file.

// TestParseLegacyBindingProjectsToSDDReceiptRef is task 6.2's RED/GREEN.
func TestParseLegacyBindingProjectsToSDDReceiptRef(t *testing.T) {
	binding := runtimeTestReviewBinding("thin", "thin-lineage", 'a', 'b')
	payload, err := bindingBytes(binding)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := parseLegacyBinding(payload)
	if err != nil {
		t.Fatalf("parseLegacyBinding() error = %v", err)
	}
	want := reviewtransaction.SDDReceiptRef{Lineage: binding.Lineage, ReceiptHash: binding.ReceiptHash}
	if ref != want {
		t.Fatalf("parseLegacyBinding() = %#v, want %#v", ref, want)
	}
}

// TestParseLegacyBindingRejectsInvalidPayloadWithoutWriting proves the
// read-only contract holds on the failure path too: an invalid legacy
// binding payload is rejected the same way parseBinding already rejects it,
// and parseLegacyBinding never attempts any write of its own.
func TestParseLegacyBindingRejectsInvalidPayloadWithoutWriting(t *testing.T) {
	if _, err := parseLegacyBinding([]byte(`{"schema":"not-a-binding"}`)); err == nil {
		t.Fatal("parseLegacyBinding(invalid payload) = nil error, want a rejection")
	}
}

// TestParseLegacyBindingLeavesTheOnDiskFileByteIdentical is task 6.2's
// golden byte comparison: an on-disk legacy binding.json is read, parsed,
// projected, and the file on disk is asserted byte-identical afterward --
// parseLegacyBinding never rewrites and never unlinks it.
func TestParseLegacyBindingLeavesTheOnDiskFileByteIdentical(t *testing.T) {
	repo := t.TempDir()
	runSDDStatusGit(t, repo, "init", "-q")
	runSDDStatusGit(t, repo, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, repo, "config", "user.name", "Status Test")
	binding := runtimeTestReviewBinding("thin", "thin-lineage", 'c', 'd')
	path := writeRuntimeLegacyBinding(t, repo, binding)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	ref, err := parseLegacyBinding(before)
	if err != nil {
		t.Fatalf("parseLegacyBinding() error = %v", err)
	}
	if ref.Lineage != binding.Lineage || ref.ReceiptHash != binding.ReceiptHash {
		t.Fatalf("parseLegacyBinding() = %#v, want lineage=%q receipt_hash=%q", ref, binding.Lineage, binding.ReceiptHash)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("legacy binding.json changed after parseLegacyBinding:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if beforeInfo.ModTime() != afterInfo.ModTime() {
		t.Fatalf("legacy binding.json mtime changed after parseLegacyBinding: before=%v after=%v", beforeInfo.ModTime(), afterInfo.ModTime())
	}
}
