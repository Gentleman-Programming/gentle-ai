package cli

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// updateReviewNewLineageSwitchOffGolden regenerates the byte-equivalence
// golden fixtures below (design's Testing Strategy: "Golden JSON per gate,
// regenerated only via -update"), matching the same convention
// internal/components/golden_test.go and shadow_matrix_test.go already use.
var updateReviewNewLineageSwitchOffGolden = flag.Bool("update", false, "update review new-lineage switch-off byte-equivalence golden files")

// TestReviewNewLineageSwitchOffByteEquivalence* (tasks 5.6-5.10) prove
// Amendment C's additive resolveGoverningAuthority branch (design decision
// 4) changes NOTHING at any of the five gates when the new-lineage
// activation switch has never created a v3 authority: the ordinary
// hook-invoked shape — no explicit --lineage marker — costs no extra Git
// call and produces byte-identical output to the pre-Wave-3-Slice-4
// baseline, captured here as a golden fixture per gate. This is
// switch-off byte-EQUIVALENCE, never a cutover (spec
// rdd-new-lineage-activation, "Additive Gate Branch, Switch-Off
// Byte-Equivalence, Not a Cutover").
func TestReviewNewLineageSwitchOffByteEquivalencePostApply(t *testing.T) {
	reviewEnabledHome(t)
	assertReviewNewLineageSwitchOffByteEquivalence(t, reviewtransaction.GatePostApply)
}

func TestReviewNewLineageSwitchOffByteEquivalencePreCommit(t *testing.T) {
	reviewEnabledHome(t)
	assertReviewNewLineageSwitchOffByteEquivalence(t, reviewtransaction.GatePreCommit)
}

func TestReviewNewLineageSwitchOffByteEquivalencePrePush(t *testing.T) {
	reviewEnabledHome(t)
	assertReviewNewLineageSwitchOffByteEquivalence(t, reviewtransaction.GatePrePush)
}

func TestReviewNewLineageSwitchOffByteEquivalencePrePR(t *testing.T) {
	reviewEnabledHome(t)
	assertReviewNewLineageSwitchOffByteEquivalence(t, reviewtransaction.GatePrePR)
}

func TestReviewNewLineageSwitchOffByteEquivalenceRelease(t *testing.T) {
	reviewEnabledHome(t)
	assertReviewNewLineageSwitchOffByteEquivalence(t, reviewtransaction.GateRelease)
}

func assertReviewNewLineageSwitchOffByteEquivalence(t *testing.T, gate reviewtransaction.GateKind) {
	t.Helper()
	// GENTLE_AI_RDD_NEW_LINEAGE is deliberately never set in this fixture:
	// the activation switch is start-only (design decision 5), so a v3
	// authority never exists here regardless. What this test actually
	// proves is decision 4's own zero-cost construction: no explicit
	// --lineage marker means resolveGoverningAuthority returns before any
	// v3 lookup, switch state notwithstanding.
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate with the new-lineage switch off\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(gate)}, &output)
	if err == nil {
		t.Fatalf("switch-off %s gate over a receiptless candidate allowed instead of denying:\n%s", gate, output.String())
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateInvalidated)
	if fields := strictReviewJSONFields(t, output.Bytes()); !reflect.DeepEqual(fields, wantEnabledReviewGateFields) {
		t.Fatalf("switch-off %s gate field set = %v, want %v", gate, fields, wantEnabledReviewGateFields)
	}

	goldenPath := filepath.Join("testdata", "review_new_lineage_switch_off", string(gate)+".golden.json")
	golden := reviewNewLineageSwitchOffGolden(t, goldenPath, output.Bytes())
	if !bytes.Equal(bytes.TrimRight(output.Bytes(), "\n"), bytes.TrimRight(golden, "\n")) {
		t.Fatalf("switch-off %s gate output diverged from the byte-equivalence golden (rerun `go test ./internal/cli/... -run TestReviewNewLineageSwitchOffByteEquivalence -update` only if this is an intended legacy-shape change):\ngot:\n%s\nwant:\n%s",
			gate, output.String(), string(golden))
	}

	// The additive branch is a true no-op for this candidate: no v3/ root
	// is ever created.
	if _, statErr := os.Stat(filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v3")); !os.IsNotExist(statErr) {
		t.Fatalf("switch-off %s gate created a v3 authority root: %v", gate, statErr)
	}
}

func reviewNewLineageSwitchOffGolden(t *testing.T, path string, got []byte) []byte {
	t.Helper()
	if *updateReviewNewLineageSwitchOffGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		normalized := append(bytes.TrimRight(got, "\n"), '\n')
		if err := os.WriteFile(path, normalized, 0o644); err != nil {
			t.Fatal(err)
		}
		return normalized
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read byte-equivalence golden %s: %v (regenerate with -update)", path, err)
	}
	return payload
}
