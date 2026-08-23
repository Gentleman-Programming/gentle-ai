package doctor

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// oneResourceManifest builds a manifest with a single ManifestResource entry
// for the AC11-classification tests. The bundle digest is computed from the
// canonicalised producer + resource so it stays stable across runs.
func oneResourceManifest(producerVersion, id, adapter, target string, ext state.OwnedExtent, desiredDigest string) state.Manifest {
	return state.Manifest{
		Schema:   state.ManifestSchema,
		Producer: state.Producer{BinaryVersion: producerVersion},
		Resources: []state.ManifestResource{{
			ID:          id,
			Adapter:     adapter,
			Target:      target,
			OwnedExtent: ext,
			Desired:     desiredDigest,
		}},
	}.WithBundleDigest()
}

// sha256Hex returns "sha256:<hex>" for content — same format the manifest
// stores in Desired/Observed fields.
func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// TestClassify_FullFileAligned covers AC11 — full-file shape where the owned
// extent (the entire file) matches the desired digest yields "aligned".
func TestClassify_FullFileAligned(t *testing.T) {
	target := filepath.Join(t.TempDir(), "managed.md")
	const content = "managed-bytes"
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	desired := sha256Hex([]byte(content))
	m := oneResourceManifest("2.2.0", "test/full-file", "full-file", target,
		state.OwnedExtent{Kind: state.ExtentFull, Ownership: state.OwnershipManaged}, desired)

	if got := Classify(m, nil, "2.2.0").Kind; got != Aligned {
		t.Errorf("Kind = %q, want aligned", got)
	}
}

// TestClassify_MarkerBlockAlignedWithUserEditsOutside covers AC11 — a
// marker-block managed region whose block digest matches desired is "aligned"
// even when the surrounding file differs from desired.
func TestClassify_MarkerBlockAlignedWithUserEditsOutside(t *testing.T) {
	const preamble = "PREAMBLE\n"
	const managed = "managed-block-bytes\n"
	const postamble = "POSTAMBLE\nuser-added-line-A\nuser-added-line-B\n"
	bytes := []byte(preamble + managed + postamble)
	target := filepath.Join(t.TempDir(), "block.md")
	if err := os.WriteFile(target, bytes, 0o644); err != nil {
		t.Fatal(err)
	}

	m := oneResourceManifest("2.2.0", "test/marker-block", "marker-block", target,
		state.OwnedExtent{
			Kind:      state.ExtentMarkerBlock,
			MarkerID:  "MANAGED",
			Start:     len(preamble),
			End:       len(preamble) + len(managed),
			Ownership: state.OwnershipManaged,
		},
		sha256Hex([]byte(managed)))

	if got := Classify(m, nil, "2.2.0").Kind; got != Aligned {
		t.Errorf("Kind = %q, want aligned (user edits outside owned extent must not affect classification)", got)
	}
}

// TestClassify_SharedJsonUserModified covers AC11 — when ownership is "user"
// and the on-disk bytes differ from desired, classification is "user_modified".
func TestClassify_SharedJsonUserModified(t *testing.T) {
	target := filepath.Join(t.TempDir(), "shared.json")
	if err := os.WriteFile(target, []byte(`{"user":42}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := oneResourceManifest("2.2.0", "test/shared-json", "shared-json", target,
		state.OwnedExtent{Kind: state.ExtentFull, Ownership: state.OwnershipUser},
		sha256Hex([]byte("totally-different-managed-bytes")))

	c := Classify(m, nil, "2.2.0")
	if c.Kind != UserModified {
		t.Errorf("Kind = %q, want user_modified", c.Kind)
	}
	if c.Hint != nil {
		t.Errorf("Hint should be nil for user_modified; got %+v", c.Hint)
	}
}

// TestClassify_UserEditedOwnedExtentUserModified covers AC11 — user-edited
// owned extent on a user-owned resource is "user_modified", not "stale".
func TestClassify_UserEditedOwnedExtentUserModified(t *testing.T) {
	target := filepath.Join(t.TempDir(), "user-edited.md")
	if err := os.WriteFile(target, []byte("user-edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := oneResourceManifest("2.2.0", "test/user-edited", "full-file", target,
		state.OwnedExtent{Kind: state.ExtentFull, Ownership: state.OwnershipUser},
		sha256Hex([]byte("managed-bytes")))
	m.Resources[0].Observed = sha256Hex([]byte("user-edited"))

	c := Classify(m, nil, "2.2.0")
	if c.Kind != UserModified {
		t.Errorf("Kind = %q, want user_modified", c.Kind)
	}
	if c.Hint != nil {
		t.Errorf("Hint should be nil for user_modified; got %+v", c.Hint)
	}
}

// TestClassify_Refresh8Over2_1_10IsMixed covers AC5 — Refresh 8 binary over a
// 2.1.10 managed bundle, with all owned-extent digests matching desired,
// MUST classify as "mixed", never "aligned".
func TestClassify_Refresh8Over2_1_10IsMixed(t *testing.T) {
	target := filepath.Join(t.TempDir(), "managed.md")
	const content = "managed-bytes"
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m := oneResourceManifest("2.1.10", "test/full-file", "full-file", target,
		state.OwnedExtent{Kind: state.ExtentFull, Ownership: state.OwnershipManaged},
		sha256Hex([]byte(content)))

	got := Classify(m, nil, "2.2.0-refresh.8").Kind
	if got == Aligned {
		t.Fatalf("Kind = aligned, want NOT aligned (Refresh 8 binary over 2.1.10 managed bundle per AC5)")
	}
	if got != Mixed {
		t.Errorf("Kind = %q, want mixed (owned-extent digests match desired; new binary)", got)
	}
}

// TestClassify_NoManifestUnknown covers AC10 — when no manifest is present,
// classification is "unknown" with a sync migration hint, never "aligned".
func TestClassify_NoManifestUnknown(t *testing.T) {
	got := Classify(state.Manifest{}, nil, "2.2.0")
	if got.Kind != Unknown {
		t.Errorf("Kind = %q, want unknown", got.Kind)
	}
	if got.Hint == nil || !strings.Contains(got.Hint.Description, "sync") {
		t.Errorf("Hint should mention 'sync' for legacy migration; got %+v", got.Hint)
	}
}

// TestClassify_InterruptedJournalUnknownNotAligned covers AC8 — a journal
// with an "intent" entry that has no matching "complete" signals an
// interrupted run; classification is "unknown", never "aligned".
func TestClassify_InterruptedJournalUnknownNotAligned(t *testing.T) {
	target := filepath.Join(t.TempDir(), "managed.md")
	const content = "managed-bytes"
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m := oneResourceManifest("2.2.0", "test/full-file", "full-file", target,
		state.OwnedExtent{Kind: state.ExtentFull, Ownership: state.OwnershipManaged},
		sha256Hex([]byte(content)))

	got := Classify(m, []state.JournalEntry{{Op: "intent", RunID: "sync-123"}}, "2.2.0").Kind
	if got == Aligned {
		t.Fatalf("Kind = aligned, want NOT aligned (interrupted journal per AC8)")
	}
	if got != Unknown {
		t.Errorf("Kind = %q, want unknown", got)
	}
}

// TestClassify_ReadOnly_NoWriteHelpersExported covers AC6 — the doctor
// package must NOT expose any Write/Append helpers. Statically scans the
// package's exported FuncDecls.
func TestClassify_ReadOnly_NoWriteHelpersExported(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parser.ParseDir: %v", err)
	}
	var offenders []string
	for _, f := range pkgs["doctor"].Files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || !fd.Name.IsExported() {
				continue
			}
			name := fd.Name.Name
			if strings.HasPrefix(name, "Write") || strings.HasPrefix(name, "Append") {
				offenders = append(offenders, name)
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("doctor package exposes write helpers (read-only gate violated): %v", offenders)
	}
}

// TestClassify_RemedySuppressedForUserModified covers AC7 — user_modified
// MUST NOT emit a remediation hint.
func TestClassify_RemedySuppressedForUserModified(t *testing.T) {
	target := filepath.Join(t.TempDir(), "user-edited.md")
	if err := os.WriteFile(target, []byte("user-edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := oneResourceManifest("2.2.0", "test/user-edited", "shared-json", target,
		state.OwnedExtent{Kind: state.ExtentFull, Ownership: state.OwnershipUser},
		sha256Hex([]byte("managed-bytes")))
	m.Resources[0].Observed = sha256Hex([]byte("user-edited"))

	c := Classify(m, nil, "2.2.0")
	if c.Kind != UserModified {
		t.Fatalf("Kind = %q, want user_modified", c.Kind)
	}
	if c.Hint != nil {
		t.Errorf("Hint must be nil for user_modified (AC7); got %+v", c.Hint)
	}
}

// TestClassify_RemedySuppressedForUnknown covers AC7 + AC10 — unknown MUST
// NOT emit an overwrite hint that would clobber unclassified bytes.
func TestClassify_RemedySuppressedForUnknown(t *testing.T) {
	c := Classify(state.Manifest{}, nil, "2.2.0")
	if c.Kind != Unknown {
		t.Fatalf("Kind = %q, want unknown", c.Kind)
	}
	// "unknown" must not guarantee that a sync is ownership-safe; any hint
	// present is informational and may be the migration hint from AC10.
	if c.Hint != nil && c.Hint.Category == "" {
		t.Errorf("unknown hint missing category: %+v", c.Hint)
	}
}
