package recoverytrace

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repositoryRoot is where the declared paths are resolved from. The rows name
// repository-relative paths, but the tests run inside the package directory, so
// every existence check is anchored here rather than at the working directory.
const repositoryRoot = "../.."

func declaredLedgers(t *testing.T) Ledgers {
	t.Helper()

	ledgers, err := Generate(readSystemicAudit(t), RecoveryRows(), RecoveryOverlaps())
	if err != nil {
		t.Fatalf("Generate(RecoveryRows()) error = %v", err)
	}
	return ledgers
}

func TestDeclaredRowsAreReleasableEvidence(t *testing.T) {
	t.Parallel()

	// Generate validates before it returns, but ValidateLedgers is called again
	// so a future change that loosened Generate could not quietly weaken the
	// guarantee this ledger exists to provide.
	if err := ValidateLedgers(declaredLedgers(t)); err != nil {
		t.Fatalf("ValidateLedgers(declared rows) error = %v, want nil", err)
	}
}

func TestDeclaredRowsCoverEveryPathExactlyOnce(t *testing.T) {
	t.Parallel()

	rows := RecoveryRows()
	if len(rows) == 0 {
		t.Fatal("no rows are declared; the recovery would have no audit record")
	}

	seen := make(map[string]Disposition, len(rows))
	for _, row := range rows {
		if previous, duplicate := seen[row.Path]; duplicate {
			t.Fatalf("%s is declared twice: %s and %s", row.Path, previous, row.Disposition)
		}
		seen[row.Path] = row.Disposition
	}
}

// TestDeclaredProofsNameFilesThatExist is the rule that keeps the ledger from
// citing evidence nobody wrote. A proof reference to an unwritten test would
// otherwise read exactly like a proof to a test that passes.
func TestDeclaredProofsNameFilesThatExist(t *testing.T) {
	t.Parallel()

	for _, row := range RecoveryRows() {
		for _, proof := range append(append([]string{}, row.Proof...), row.DestinationProof...) {
			if !strings.HasSuffix(proof, "_test.go") {
				t.Errorf("%s cites %q, which is not a test file", row.Path, proof)
				continue
			}
			if _, err := os.Stat(filepath.Join(repositoryRoot, filepath.Clean(proof))); err != nil {
				t.Errorf("%s cites %q, which does not exist: %v", row.Path, proof, err)
			}
		}
	}
}

// TestDeclaredRowsRecordPublicationForEveryPath enforces that presence was
// actually measured. A row with no publication evidence is an unchecked path,
// which is a different fact from a path known to be absent.
func TestDeclaredRowsRecordPublicationForEveryPath(t *testing.T) {
	t.Parallel()

	for _, row := range RecoveryRows() {
		if len(row.Publication) != requiredRefs {
			t.Errorf("%s records %d refs, want %d", row.Path, len(row.Publication), requiredRefs)
			continue
		}
		if row.Publication[0].Ref != remoteMainRef || row.Publication[1].Ref != localMainRef {
			t.Errorf("%s does not record both branch refs: %+v", row.Path, row.Publication)
		}
		if !releaseTagRef(row.Publication[2].Ref) {
			t.Errorf("%s records %q where a release tag belongs", row.Path, row.Publication[2].Ref)
		}
	}
}

// TestDeclaredEarlyDeviationsAreOnlyDeletions guards the one deviation the
// recovery is allowed to take. Marking a retained row as an early removal would
// claim an authorization that only a deletion can hold.
func TestDeclaredEarlyDeviationsAreOnlyDeletions(t *testing.T) {
	t.Parallel()

	for _, row := range RecoveryRows() {
		if row.EarlyDeviation && row.Disposition != DispositionDelete {
			t.Errorf("%s takes the early-removal deviation as %s", row.Path, row.Disposition)
		}
	}
}

// TestDeclaredDeferredRowsClaimNoInvariant keeps the deferred rows honest: a
// path nobody has written, and a removal nobody has authorized, cannot also be
// credited with proving something.
func TestDeclaredDeferredRowsClaimNoInvariant(t *testing.T) {
	t.Parallel()

	for _, row := range RecoveryRows() {
		if row.Disposition != DispositionDefer {
			continue
		}
		if row.Invariant != "" || len(row.Proof) > 0 {
			t.Errorf("%s is deferred but claims %q proven by %v", row.Path, row.Invariant, row.Proof)
		}
	}
}

func TestDeclaredRowsRenderDeterministically(t *testing.T) {
	t.Parallel()

	source := readSystemicAudit(t)
	forward := RecoveryRows()
	reversed := make([]Row, 0, len(forward))
	for index := len(forward) - 1; index >= 0; index-- {
		reversed = append(reversed, forward[index])
	}

	inOrder, err := Generate(source, forward, RecoveryOverlaps())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	shuffled, err := Generate(source, reversed, RecoveryOverlaps())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	first, err := Render(inOrder)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	second, err := Render(shuffled)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("render emitted %d then %d documents", len(first), len(second))
	}
	for name, document := range first {
		if !bytes.Equal(document, second[name]) {
			t.Fatalf("%s depends on the order the rows were declared in", name)
		}
	}
}

// TestRecoveryRowsCannotBeMutatedByACaller proves the declaration is a value,
// not shared state. A caller that edited the returned slice would otherwise
// change evidence every later caller reads.
func TestRecoveryRowsCannotBeMutatedByACaller(t *testing.T) {
	t.Parallel()

	rows := RecoveryRows()
	rows[0].Contributor = ""
	rows[0].Disposition = DispositionDelete

	if RecoveryRows()[0].Contributor == "" {
		t.Fatal("RecoveryRows() returns shared state a caller can rewrite")
	}
}
