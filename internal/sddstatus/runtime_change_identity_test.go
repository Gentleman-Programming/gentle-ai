package sddstatus

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// The native runtime authority must accept every change identity that
// sdd-status already resolves. Rejecting an Engram identity such as
// DEC-EXAMPLE-CHANGE stalls the whole attempt gate for a change whose
// artifacts resolve normally.
func TestOpenRuntimeStoreAcceptsChangeIdentitiesResolvedBySDDStatus(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	for _, change := range []string{
		"DEC-EXAMPLE-CHANGE",
		"Add-User-Auth",
		"add_user_auth",
		"2116-fix-sdd-attempt",
		"MixedCase_And-Digits2",
	} {
		store, err := OpenRuntimeStore(context.Background(), repo, change)
		if err != nil {
			t.Fatalf("OpenRuntimeStore(%q) error = %v, want accepted", change, err)
		}
		if store.Change != change {
			t.Fatalf("OpenRuntimeStore(%q).Change = %q, want the identity preserved verbatim", change, store.Change)
		}
	}
}

// Widening the accepted identity must not widen what reaches the filesystem.
func TestOpenRuntimeStoreRejectsUnsafeChangeName(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	for _, change := range []string{
		"",
		".",
		"..",
		"../escape",
		"nested/change",
		`nested\change`,
		"-leading",
		"trailing-",
		"double--hyphen",
		".hidden",
		"has space",
		"has:colon",
		strings.Repeat("a", 97),
	} {
		if _, err := OpenRuntimeStore(context.Background(), repo, change); err == nil {
			t.Fatalf("OpenRuntimeStore(%q) error = nil, want rejection", change)
		}
	}
}

// The rejection has to say which value failed and what shape is expected,
// otherwise callers are left guessing the flag contract.
func TestOpenRuntimeStoreRejectionNamesValueAndShape(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	_, err := OpenRuntimeStore(context.Background(), repo, "nested/change")
	if err == nil {
		t.Fatal("OpenRuntimeStore error = nil, want rejection")
	}
	message := err.Error()
	if !strings.Contains(message, `"nested/change"`) {
		t.Fatalf("error %q does not name the rejected value", message)
	}
	if !strings.Contains(message, "letters, digits") {
		t.Fatalf("error %q does not state the expected shape", message)
	}
	if !strings.Contains(message, "gentle-ai sdd-status") {
		t.Fatalf("error %q does not name the command that reveals the resolved identity", message)
	}
}

// The encoded suffix is squeezed from both sides, so the width is pinned here
// rather than left to a future edit. Narrower and a birthday search over
// crafted case variants becomes practical, letting two identities share one
// ledger directory. Wider and the leaf stops being addressable on Windows,
// where an identity at the length limit already crowds the path ceiling.
func TestOpenRuntimeStoreEncodedSuffixKeepsItsPinnedWidth(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "DEC-EXAMPLE-CHANGE")
	if err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Base(store.Dir)
	suffix := leaf[strings.LastIndex(leaf, "-")+1:]
	if len(suffix) != 32 {
		t.Fatalf("encoded suffix %q is %d hex characters, want exactly 32 (128 bits)", suffix, len(suffix))
	}
	if strings.Trim(suffix, "0123456789abcdef") != "" {
		t.Fatalf("encoded suffix %q is not lowercase hex", suffix)
	}
}

// Ledgers already on disk live at v1/<change>. A kebab-case identity must keep
// resolving to that exact directory or every existing attempt chain is orphaned.
func TestOpenRuntimeStoreKeepsLegacyDirectoryForKebabChange(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "legacy-kebab-change")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(store.Dir) != "legacy-kebab-change" || filepath.Base(filepath.Dir(store.Dir)) != "v1" {
		t.Fatalf("legacy ledger directory = %q, want v1/legacy-kebab-change", store.Dir)
	}
}

// On case-insensitive filesystems two identities differing only in case would
// share one ledger directory, silently merging unrelated attempt chains.
func TestOpenRuntimeStoreSeparatesCaseVariantChangeDirectories(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	upper, err := OpenRuntimeStore(context.Background(), repo, "Case-Variant")
	if err != nil {
		t.Fatal(err)
	}
	lower, err := OpenRuntimeStore(context.Background(), repo, "case-variant")
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(upper.Dir, lower.Dir) {
		t.Fatalf("case variants share ledger directory %q on a case-insensitive filesystem", upper.Dir)
	}
}

// The encoded namespace must be unreachable as a legacy identity, so an
// encoded directory can never collide with a kebab-case change's ledger.
func TestOpenRuntimeStoreEncodedNamespaceIsNotAValidLegacyIdentity(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "DEC-EXAMPLE-CHANGE")
	if err != nil {
		t.Fatal(err)
	}
	namespace := filepath.Base(filepath.Dir(store.Dir))
	if namespace == "v1" {
		t.Fatalf("encoded identity reused the legacy namespace at %q", store.Dir)
	}
	if legacyRuntimeChangeDir(namespace) {
		t.Fatalf("encoded namespace %q is also a valid legacy change name", namespace)
	}
}
