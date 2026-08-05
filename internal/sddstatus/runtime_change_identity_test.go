package sddstatus

import (
	"context"
	"fmt"
	"os"
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
		"double__underscore",
		"mixed-_separator",
		"mixed_-separator",
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
	if !strings.Contains(message, fmt.Sprintf("at most %d bytes", RuntimeChangeLimit)) {
		t.Fatalf("error %q does not state the shared byte limit", message)
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

func TestOpenRuntimeStoreDoesNotSelectNonMaterialLegacyDirectory(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{name: "empty", setup: func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "records scaffold", setup: func(t *testing.T, path string) {
			if err := os.MkdirAll(filepath.Join(path, "records"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "lock only", setup: func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "LOCK"), []byte("stale\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := initRuntimeLedgerRepo(t)
			change := "Empty_Legacy"
			canonical, err := OpenRuntimeStore(context.Background(), repo, change)
			if err != nil {
				t.Fatal(err)
			}
			legacy := filepath.Join(canonical.commonDir, "gentle-ai", "sdd-runtime", "v1", change)
			test.setup(t, legacy)
			reopened, err := OpenRuntimeStore(context.Background(), repo, change)
			if err != nil {
				t.Fatal(err)
			}
			if reopened.Dir != canonical.Dir {
				t.Fatalf("reopened path = %q, want canonical %q", reopened.Dir, canonical.Dir)
			}
		})
	}
}

func TestOpenRuntimeStoreRejectsSymlinkedCompatibilityComponents(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, string, string)
	}{
		{name: "runtime root", setup: func(t *testing.T, base, _ string) {
			outside := t.TempDir()
			if err := os.MkdirAll(filepath.Dir(base), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, base); err != nil {
				t.Skipf("symlink fixture unavailable: %v", err)
			}
		}},
		{name: "legacy root", setup: func(t *testing.T, base, _ string) {
			outside := t.TempDir()
			if err := os.MkdirAll(base, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(base, "v1")); err != nil {
				t.Skipf("symlink fixture unavailable: %v", err)
			}
		}},
		{name: "records component", setup: func(t *testing.T, base, change string) {
			legacy := filepath.Join(base, "v1", change)
			if err := os.MkdirAll(legacy, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(t.TempDir(), filepath.Join(legacy, "records")); err != nil {
				t.Skipf("symlink fixture unavailable: %v", err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := initRuntimeLedgerRepo(t)
			change := "Symlinked_Legacy"
			store, err := OpenRuntimeStore(context.Background(), repo, change)
			if err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(store.commonDir, "gentle-ai", "sdd-runtime")
			test.setup(t, base, change)
			if _, err := OpenRuntimeStore(context.Background(), repo, change); err == nil {
				t.Fatal("OpenRuntimeStore accepted a symlinked compatibility path")
			}
		})
	}
}

func TestOpenRuntimeStoreRefusesCaseFoldedLegacyIdentityCollision(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	upper, err := OpenRuntimeStore(context.Background(), repo, "Case-Variant")
	if err != nil {
		t.Fatal(err)
	}
	legacy := upper
	// Put the uppercase record at the lowercase spelling to model the same
	// directory that a case-insensitive filesystem would expose.
	legacy.Dir = filepath.Join(legacy.commonDir, "gentle-ai", "sdd-runtime", "v1", "case-variant")
	if _, err := legacy.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "case-folded-legacy", WorkUnit: "case-folded", EvidenceGoal: "reject identity alias", MaxAttempts: 1, MaxChangedLines: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRuntimeStore(context.Background(), repo, "case-variant"); err == nil {
		t.Fatal("OpenRuntimeStore accepted a legacy record belonging to a case-variant identity")
	}
}

func TestOpenRuntimeStorePreservesLogicalIdentityInRecords(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	for _, change := range []string{"lowercase-change", "Uppercase-Change", "underscore_change"} {
		t.Run(change, func(t *testing.T) {
			store, err := OpenRuntimeStore(context.Background(), repo, change)
			if err != nil {
				t.Fatal(err)
			}
			status, err := store.Begin(context.Background(), BeginAttemptRequest{
				RequestID: "identity-" + strings.ToLower(strings.ReplaceAll(change, "_", "-")),
				WorkUnit:  "identity-record", EvidenceGoal: "preserve exact change identity", MaxAttempts: 1, MaxChangedLines: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if status.Change != change || status.ActiveAttempt == nil {
				t.Fatalf("begin status = %#v, want exact change %q", status, change)
			}
			reopened, err := OpenRuntimeStore(context.Background(), repo, change)
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := reopened.Status()
			if err != nil {
				t.Fatal(err)
			}
			if replayed.Change != change || replayed.ActiveAttempt == nil {
				t.Fatalf("replayed status = %#v, want exact change %q", replayed, change)
			}
		})
	}
}

func TestOpenRuntimeStoreReopensAndWritesLegacyLedger(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	change := "Legacy_Case"
	canonical, err := OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	legacy := canonical
	legacy.Dir = filepath.Join(legacy.commonDir, "gentle-ai", "sdd-runtime", "v1", change)
	started, err := legacy.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "legacy-begin", WorkUnit: "legacy-compatibility", EvidenceGoal: "reopen old ledger", MaxAttempts: 2, MaxChangedLines: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy.Dir, ".head-crashed"), []byte("partial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy.Dir, "records", ".record-crashed"), []byte("partial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "legacy-attempt\n")
	reopened, err := OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Dir != legacy.Dir || reopened.Change != change {
		t.Fatalf("reopened store = %#v, want legacy path %q and exact identity", reopened, legacy.Dir)
	}
	reopened.ReviewDisabled = true
	finished, err := reopened.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "legacy-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('1'), Diagnosis: "legacy ledger remained writable",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup complete", ProcessEvidence: "no process remained",
	})
	if err != nil {
		t.Fatal(err)
	}
	if finished.Change != change || countRuntimeRecords(t, legacy.Dir) != 2 {
		t.Fatalf("finished legacy status = %#v, records = %d", finished, countRuntimeRecords(t, legacy.Dir))
	}
	if _, err := os.Stat(canonical.Dir); !os.IsNotExist(err) {
		t.Fatalf("compatibility write created canonical ledger %q: %v", canonical.Dir, err)
	}
}

func TestOpenRuntimeStoreRejectsMalformedLegacySibling(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	change := "Malformed_Legacy"
	canonical, err := OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	legacy := canonical
	legacy.Dir = filepath.Join(legacy.commonDir, "gentle-ai", "sdd-runtime", "v1", change)
	if _, err := legacy.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "malformed-begin", WorkUnit: "malformed", EvidenceGoal: "reject sibling", MaxAttempts: 1, MaxChangedLines: 1,
	}); err != nil {
		t.Fatal(err)
	}
	malformed := filepath.Join(legacy.Dir, "records", strings.Repeat("a", 64)+".json")
	if err := os.WriteFile(malformed, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRuntimeStore(context.Background(), repo, change); err == nil {
		t.Fatal("OpenRuntimeStore adopted a legacy ledger with a malformed sibling record")
	}
}

func TestOpenRuntimeStoreRejectsOrphanedLegacyRecord(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	change := "orphaned-legacy"
	canonical, err := OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	legacy := canonical
	legacy.Dir = filepath.Join(legacy.commonDir, "gentle-ai", "sdd-runtime", "v1", change)
	if _, err := legacy.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "orphaned-begin", WorkUnit: "orphaned", EvidenceGoal: "reject orphan", MaxAttempts: 1, MaxChangedLines: 1,
	}); err != nil {
		t.Fatal(err)
	}
	head, exists, err := readRuntimeHead(filepath.Join(legacy.Dir, "HEAD"))
	if err != nil || !exists {
		t.Fatalf("legacy HEAD = %q/%v, err=%v", head, exists, err)
	}
	record, err := legacy.loadRecord(head)
	if err != nil {
		t.Fatal(err)
	}
	record.RequestID = "orphaned-record"
	record.PreviousRevision = ""
	revision, payload, err := runtimeRecordRevision(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.publishRecord(revision, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRuntimeStore(context.Background(), repo, change); err == nil {
		t.Fatal("OpenRuntimeStore adopted a legacy ledger with an orphaned final record")
	}
}

func TestOpenRuntimeStoreUsesExistingCanonicalLedger(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	change := "Canonical_Case"
	canonical, err := OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := canonical.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "canonical-begin", WorkUnit: "canonical", EvidenceGoal: "prefer canonical", MaxAttempts: 1, MaxChangedLines: 1,
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Dir != canonical.Dir {
		t.Fatalf("reopened canonical path = %q, want %q", reopened.Dir, canonical.Dir)
	}
}

func TestOpenRuntimeStoreRefusesMateriallyAmbiguousLegacyAndCanonicalLedgers(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	change := "Ambiguous_Case"
	canonical, err := OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	legacy := canonical
	legacy.Dir = filepath.Join(legacy.commonDir, "gentle-ai", "sdd-runtime", "v1", change)
	for index, store := range []RuntimeStore{canonical, legacy} {
		if _, err := store.Begin(context.Background(), BeginAttemptRequest{
			RequestID: fmt.Sprintf("ambiguous-begin-%d", index), WorkUnit: "ambiguous", EvidenceGoal: "refuse split brain", MaxAttempts: 1, MaxChangedLines: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := OpenRuntimeStore(context.Background(), repo, change); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("OpenRuntimeStore with both ledgers = %v, want an ambiguity refusal", err)
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
