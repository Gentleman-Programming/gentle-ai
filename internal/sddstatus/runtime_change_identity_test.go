package sddstatus

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// The native runtime authority must accept every change identity that
// sdd-status already resolves (#2116): sdd-status's directory-based and
// Engram-backed resolution impose no character-shape restriction of their
// own, so the runtime authority cannot impose one narrower than "whatever a
// filesystem path segment or a JSON string can safely represent". This table
// covers the reported classes directly: dashes (the original kebab-case
// contract), uppercase and underscores (DEC-EXAMPLE-CHANGE, tag_improvement),
// dots (3.14), spaces and "@" (a human-readable Engram identity), and a
// nested/topic-namespaced identity containing "/".
func TestOpenRuntimeStoreAcceptsChangeIdentitiesResolvedBySDDStatus(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	for _, change := range []string{
		"kebab-case",
		"DEC-EXAMPLE-CHANGE",
		"Add-User-Auth",
		"add_user_auth",
		"2116-fix-sdd-attempt",
		"MixedCase_And-Digits2",
		"3.14",
		"Change.With.Dots",
		"tag improvement @ v2",
		"features/sub-change",
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

// The ledger directory derived for a name containing "/" or "." must stay a
// single flat component under the encoded namespace: it must not reintroduce
// extra directory levels (which "/" would create verbatim) or resolve
// outside the runtime store's own base directory.
func TestOpenRuntimeStoreEncodesPathLikeChangeAsOneFlatComponent(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	for _, change := range []string{"features/sub-change", "3.14", "a.b/c.d"} {
		store, err := OpenRuntimeStore(context.Background(), repo, change)
		if err != nil {
			t.Fatalf("OpenRuntimeStore(%q) error = %v", change, err)
		}
		base := filepath.Dir(store.Dir)
		if filepath.Base(base) != encodedRuntimeChangeNamespace {
			t.Fatalf("OpenRuntimeStore(%q).Dir = %q, want a single leaf directly under %q", change, store.Dir, encodedRuntimeChangeNamespace)
		}
		if strings.ContainsAny(filepath.Base(store.Dir), `/\`) {
			t.Fatalf("OpenRuntimeStore(%q) leaf %q still contains a path separator", change, filepath.Base(store.Dir))
		}
	}
}

// Every identity the pre-#2116 validator already accepted (letters, digits,
// hyphens, and underscores) must keep resolving to the exact ledger
// directory the old encoding produced -- `strings.ToLower(change)` verbatim,
// with no byte substitution -- or an attempt acquired before this change
// becomes unreachable after it: it cannot be settled, its max-attempts
// budget silently resets, and its request-id replay is lost.
func TestOpenRuntimeStoreEncodedLabelIsStableForPreviouslyValidIdentities(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	for _, change := range []string{"DEC-EXAMPLE-CHANGE", "MixedCase_And-Digits2", "tag_improvement-v2"} {
		store, err := OpenRuntimeStore(context.Background(), repo, change)
		if err != nil {
			t.Fatal(err)
		}
		wantLabel := strings.ToLower(change)
		if leaf := filepath.Base(store.Dir); !strings.HasPrefix(leaf, wantLabel+"-") {
			t.Fatalf("OpenRuntimeStore(%q) leaf = %q, want the unchanged pre-#2116 label %q preserved verbatim", change, leaf, wantLabel)
		}
	}
}

// Widening the accepted identity must not widen what reaches the filesystem:
// these stay rejected because sdd-status itself can never resolve them (a
// change name cannot be "..", cannot escape via a "../" segment, and the
// filesystem/JSON hazards below have no legitimate identity behind them).
func TestOpenRuntimeStoreRejectsUnsafeChangeName(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	for _, change := range []string{
		"",
		".",
		"..",
		"../escape",
		"escape/../../outside",
		`nested\change`,
		"has:colon",
		"control\x00char",
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
	_, err := OpenRuntimeStore(context.Background(), repo, "has:colon")
	if err == nil {
		t.Fatal("OpenRuntimeStore error = nil, want rejection")
	}
	message := err.Error()
	if !strings.Contains(message, `"has:colon"`) {
		t.Fatalf("error %q does not name the rejected value", message)
	}
	if !strings.Contains(message, "non-empty identity") {
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
