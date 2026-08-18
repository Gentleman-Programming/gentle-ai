package sddstatus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sddChangeIdentityCase pins one input against the single verdict every SDD
// surface has to reach for it.
type sddChangeIdentityCase struct {
	change   string
	accepted bool
	// why records the reason a refusal exists, so a later widening has to
	// argue with the reason instead of deleting an unexplained row.
	why string
}

// sddChangeIdentityCases is the contract. The first nine rows are the exact
// identities measured against released v2.4.0 in #2116, where sdd-status
// resolved all nine and the runtime authority refused five of them. The rest
// are the separator, traversal, and platform shapes the widening must keep
// refusing, each with the reason it is refused.
func sddChangeIdentityCases() []sddChangeIdentityCase {
	return []sddChangeIdentityCase{
		// Measured on v2.4.0: sdd-status OK, sdd-attempt OK.
		{change: "kebab-case-ok", accepted: true},
		{change: "snake_case_name", accepted: true},
		{change: "AJ-142", accepted: true},
		{change: "UPPER-CASE", accepted: true},
		// Measured on v2.4.0: sdd-status OK, sdd-attempt rejected. These five
		// are the defect.
		{change: "3.14", accepted: true},
		{change: "dot.in.name", accepted: true},
		{change: "with space", accepted: true},
		{change: "at@sign", accepted: true},
		{change: "mixed_Case-1.2", accepted: true},
		// Reported shapes from the same thread that must also resolve.
		{change: "DEC-EXAMPLE-CHANGE", accepted: true},
		{change: "US999-example-slug", accepted: true},
		{change: "tag_improvement", accepted: true},
		{change: ".hidden", accepted: true},
		{change: "trailing-", accepted: true},
		{change: "double--hyphen", accepted: true},
		{change: strings.Repeat("a", 96), accepted: true},

		{change: "", accepted: false, why: "an empty identity names no change"},
		{change: ".", accepted: false, why: "names the change directory itself"},
		{change: "..", accepted: false, why: "traversal: names the parent of every change"},
		{change: "../escape", accepted: false, why: "traversal out of openspec/changes and out of the ledger root"},
		{change: "..\\escape", accepted: false, why: "the same traversal spelled for Windows"},
		{change: "nested/change", accepted: false, why: "two path segments, so one change could name another change's tree"},
		{change: `nested\change`, accepted: false, why: "the same split spelled for Windows"},
		{change: "-leading", accepted: false, why: "indistinguishable from a flag in every documented invocation"},
		{change: " leading-space", accepted: false, why: "sdd-status trims before resolving, so it is unaddressable there"},
		{change: "trailing-space ", accepted: false, why: "Windows strips a trailing space, merging two identities into one ledger"},
		{change: "trailing-dot.", accepted: false, why: "Windows strips a trailing dot, merging two identities into one ledger"},
		{change: "has:colon", accepted: false, why: "names a Windows drive or an NTFS alternate data stream"},
		{change: "star*glob", accepted: false, why: "unrepresentable on Windows, so the identity would resolve on one platform only"},
		{change: "pipe|name", accepted: false, why: "unrepresentable on Windows"},
		{change: "quote\"name", accepted: false, why: "unrepresentable on Windows and a quoting hazard in every readback"},
		{change: "null\x00byte", accepted: false, why: "truncates the path at the syscall boundary"},
		{change: "newline\nname", accepted: false, why: "a control character splits every line-oriented readback"},
		{change: strings.Repeat("a", 97), accepted: false, why: "the encoded ledger leaf must stay inside the Windows path ceiling"},
	}
}

// The whole defect was two surfaces disagreeing about one identity, and
// nothing pinned their agreement: #2204 widened two character classes and left
// three behind because the contract lived in a regex rather than in a test.
// This is that missing test. For every input, sdd-status and the native
// runtime authority must reach the SAME verdict.
func TestSDDChangeIdentityVerdictIsTheSameOnEverySurface(t *testing.T) {
	for _, tt := range sddChangeIdentityCases() {
		t.Run(identityCaseName(tt.change), func(t *testing.T) {
			if got := validSDDChangeIdentity(tt.change); got != tt.accepted {
				t.Fatalf("validSDDChangeIdentity(%q) = %v, want %v (%s)", tt.change, got, tt.accepted, tt.why)
			}

			repo := initRuntimeLedgerRepo(t)
			_, runtimeErr := OpenRuntimeStore(context.Background(), repo, tt.change)
			if (runtimeErr == nil) != tt.accepted {
				t.Fatalf("OpenRuntimeStore(%q) error = %v, want accepted = %v (%s)", tt.change, runtimeErr, tt.accepted, tt.why)
			}

			resolved, ok := resolveIdentityThroughSDDStatus(t, repo, tt.change)
			if !ok {
				return
			}
			if resolved != tt.accepted {
				t.Fatalf("sdd-status resolved %q = %v, but the runtime authority accepted it = %v; the two identity contracts disagree",
					tt.change, resolved, tt.accepted)
			}
		})
	}
}

// resolveIdentityThroughSDDStatus scaffolds the change as sdd-status would
// find it and reports whether status resolved it. The second result is false
// when the host filesystem cannot hold a directory by that name, which is not
// a disagreement: an identity that cannot exist cannot be resolved by either
// surface.
func resolveIdentityThroughSDDStatus(t *testing.T, repo, change string) (bool, bool) {
	t.Helper()
	if change != "" {
		changeRoot := filepath.Join(repo, "openspec", "changes", change)
		if err := os.MkdirAll(changeRoot, 0o755); err != nil {
			return false, false
		}
		if filepath.Base(changeRoot) != change {
			// The name did not survive the join, so the directory that exists
			// is not the identity under test.
			return false, false
		}
		if err := os.WriteFile(filepath.Join(changeRoot, "proposal.md"), []byte("# Proposal\n"), 0o644); err != nil {
			return false, false
		}
	}
	status, err := Resolve(ResolveOptions{WorkspaceRoot: repo, ChangeName: change})
	if err != nil {
		t.Fatalf("Resolve(%q) error = %v", change, err)
	}
	return status.ChangeRoot != nil, true
}

func identityCaseName(change string) string {
	if change == "" {
		return "empty"
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == ' ' || r == '/' {
			return '_'
		}
		return r
	}, change)
}

// A refused identity has to say which value failed, why, and where to read the
// identity SDD actually resolved. The bare "invalid SDD change name" this
// replaces is what made the defect take trial and error to diagnose.
func TestOpenRuntimeStoreRejectionNamesValueReasonAndDiscovery(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	_, err := OpenRuntimeStore(context.Background(), repo, "nested/change")
	if err == nil {
		t.Fatal("OpenRuntimeStore error = nil, want rejection")
	}
	message := err.Error()
	for _, want := range []string{`"nested/change"`, "one addressable path segment", "gentle-ai sdd-status"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q does not contain %q", message, want)
		}
	}
}

// sdd-status must refuse a bad identity BEFORE planning writes artifacts under
// it, and say so. Reporters landed in exactly the opposite state: planning
// accepted the identity, created durable artifacts, and only then blocked with
// no rename or migration operation to recover with.
func TestSDDStatusRefusesUnusableIdentityWithTheReasonAndTheExit(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	status, err := Resolve(ResolveOptions{WorkspaceRoot: repo, ChangeName: "has:colon"})
	if err != nil {
		t.Fatal(err)
	}
	if status.ChangeRoot != nil {
		t.Fatalf("sdd-status resolved an identity the runtime authority refuses: %v", *status.ChangeRoot)
	}
	joined := strings.Join(status.BlockedReasons, " ")
	for _, want := range []string{`"has:colon"`, "cannot contain", "Rename it"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("blocked reasons %q do not contain %q", joined, want)
		}
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

// Widening the accepted identity must not widen what reaches the filesystem.
// The identity is preserved verbatim on the store; the directory name is
// derived, and only [a-z0-9-_] may reach it.
func TestOpenRuntimeStoreKeepsIdentityRunesOutOfTheLedgerPath(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	for _, change := range []string{"3.14", "dot.in.name", "with space", "at@sign", "mixed_Case-1.2", ".hidden"} {
		store, err := OpenRuntimeStore(context.Background(), repo, change)
		if err != nil {
			t.Fatalf("OpenRuntimeStore(%q) error = %v", change, err)
		}
		if store.Change != change {
			t.Fatalf("OpenRuntimeStore(%q).Change = %q, want the identity preserved verbatim", change, store.Change)
		}
		leaf := filepath.Base(store.Dir)
		if strings.Trim(leaf, "abcdefghijklmnopqrstuvwxyz0123456789-_") != "" {
			t.Fatalf("ledger leaf %q for identity %q carries a rune outside [a-z0-9-_]", leaf, change)
		}
		if strings.HasPrefix(leaf, ".") || strings.HasSuffix(leaf, ".") || strings.HasSuffix(leaf, " ") {
			t.Fatalf("ledger leaf %q for identity %q is hidden or Windows-truncatable", leaf, change)
		}
	}
}

// Two identities that differ only in the runes the leaf folds away must still
// get two ledgers, or unrelated attempt chains merge.
func TestOpenRuntimeStoreSeparatesIdentitiesThatShareAFoldedLabel(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	seen := map[string]string{}
	for _, change := range []string{"a.b", "a b", "a@b", "a_b", "A_B", "a-b", "A.B"} {
		store, err := OpenRuntimeStore(context.Background(), repo, change)
		if err != nil {
			t.Fatalf("OpenRuntimeStore(%q) error = %v", change, err)
		}
		if previous, clash := seen[strings.ToLower(store.Dir)]; clash {
			t.Fatalf("identities %q and %q share ledger directory %q", previous, change, store.Dir)
		}
		seen[strings.ToLower(store.Dir)] = change
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

// Widening the identity must not move any ledger the previous contract already
// served. Every identity #2204 accepted lowercases into [a-z0-9-_] alone, so
// the derived leaf is unchanged for all of them.
func TestOpenRuntimeStoreDoesNotMoveLedgersTheEarlierContractAccepted(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	for change, wantLeaf := range map[string]string{
		"DEC-EXAMPLE-CHANGE":   "dec-example-change",
		"Add-User-Auth":        "add-user-auth",
		"add_user_auth":        "add_user_auth",
		"MixedCase_And-Digits": "mixedcase_and-digits",
	} {
		store, err := OpenRuntimeStore(context.Background(), repo, change)
		if err != nil {
			t.Fatalf("OpenRuntimeStore(%q) error = %v", change, err)
		}
		leaf := filepath.Base(store.Dir)
		if !strings.HasPrefix(leaf, wantLeaf+"-") {
			t.Fatalf("ledger leaf for %q = %q, want the pre-existing %q-<digest>; existing ledgers would be orphaned", change, leaf, wantLeaf)
		}
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
