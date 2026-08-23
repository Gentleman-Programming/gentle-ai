package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

type shippedReviewStoreEntry struct {
	Path  string
	Mode  fs.FileMode
	Bytes []byte
}

func snapshotShippedReviewStore(t *testing.T, repo string) []shippedReviewStoreEntry {
	t.Helper()
	root := reviewCLIAuthorityRoot(t, repo)
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		t.Fatal(err)
	}

	var entries []shippedReviewStoreEntry
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot := shippedReviewStoreEntry{Path: filepath.ToSlash(relative), Mode: info.Mode()}
		if info.Mode().IsRegular() {
			snapshot.Bytes, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		entries = append(entries, snapshot)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return entries
}

func assertShippedReviewStoreUnchanged(t *testing.T, before, after []shippedReviewStoreEntry) {
	t.Helper()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("shipped review validate mutated the complete review-store tree:\nbefore: %#v\nafter: %#v", before, after)
	}
}

func prepareShippedNonDecidingAuthorityFixture(t *testing.T, repo, fixture string) {
	t.Helper()
	target := reviewtransaction.Target{
		Kind:              reviewtransaction.TargetCurrentChanges,
		Projection:        reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: []string{},
	}
	switch fixture {
	case "clean store":
		return
	case "active reviewing authority":
		if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--lineage", "nondeciding-reviewing"}, io.Discard); err != nil {
			t.Fatalf("seed active reviewing authority: %v", err)
		}
	case "historical approved receipt":
		seedHistoricalCompatibilityApprovedCompactReceipt(t, repo, "nondeciding-approved", target)
	case "corrupt and ambiguous authority":
		seedHistoricalCompatibilityApprovedCompactReceipt(t, repo, "nondeciding-ambiguous-a", target)
		seedHistoricalCompatibilityApprovedCompactReceipt(t, repo, "nondeciding-ambiguous-b", target)
		corrupt := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "nondeciding-corrupt", "review-state.json")
		if err := os.MkdirAll(filepath.Dir(corrupt), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(corrupt, []byte("{not-json\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	case "lock residue":
		lock := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "LOCK")
		if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(lock, []byte("interrupted historical authority lock\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown non-deciding fixture %q", fixture)
	}
}

func runShippedNonDecidingReviewValidate(t *testing.T, repo string, gate reviewtransaction.GateKind) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{"validate", "--cwd", repo, "--gate", string(gate)}, &output); err != nil {
		t.Fatalf("shipped review validate --gate %s: %v\n%s", gate, err, output.String())
	}
	return append([]byte(nil), output.Bytes()...)
}

func assertEnabledUnmanagedGatePayload(t *testing.T, payload []byte, gate reviewtransaction.GateKind) {
	t.Helper()
	var result struct {
		Schema   string         `json:"schema"`
		Result   string         `json:"result"`
		Allowed  bool           `json:"allowed"`
		Action   string         `json:"action"`
		Context  map[string]any `json:"context"`
		Delivery string         `json:"delivery"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("decode enabled unmanaged payload: %v\n%s", err, payload)
	}
	if result.Schema != ReviewValidateSchema || result.Result != string(reviewtransaction.GateInvalidated) || result.Allowed ||
		result.Action != reviewDeliveryPolicyAction || result.Delivery != string(reviewtransaction.RDDDeliveryUnmanaged) {
		t.Fatalf("enabled unmanaged result = %#v\n%s", result, payload)
	}
	if len(result.Context) != 1 || result.Context["gate"] != string(gate) {
		t.Fatalf("enabled unmanaged context = %#v, want only gate %q", result.Context, gate)
	}
	compact := strings.NewReplacer(" ", "", "\n", "", "\t", "", "\r", "").Replace(strings.ToLower(string(payload)))
	for _, forbidden := range []string{"\"result\":\"allow\"", "\"allowed\":true", "approved", "receipt", "lineage", "receipt_governed", "disabled"} {
		if strings.Contains(compact, forbidden) {
			t.Fatalf("enabled unmanaged output contains forbidden %q:\n%s", forbidden, payload)
		}
	}
}

func TestShippedReviewValidateAllGatesIgnoreAuthorityStateWithoutMutation(t *testing.T) {
	reviewModeHome(t)
	modeRepo := initReviewCLIRepo(t)
	if err := RunReviewMode([]string{"enable", "--scope", "global", "--cwd", modeRepo}, io.Discard); err != nil {
		t.Fatalf("enable receipt-driven development: %v", err)
	}
	schema := compileWholePublishedReviewSchema(t, "v2", "gate-result.schema.json")
	fixtures := []string{
		"clean store",
		"active reviewing authority",
		"historical approved receipt",
		"corrupt and ambiguous authority",
		"lock residue",
	}
	gates := []reviewtransaction.GateKind{
		reviewtransaction.GatePostApply,
		reviewtransaction.GatePreCommit,
		reviewtransaction.GatePrePush,
		reviewtransaction.GatePrePR,
		reviewtransaction.GateRelease,
	}

	for _, gate := range gates {
		t.Run(string(gate), func(t *testing.T) {
			var expected []byte
			for _, fixture := range fixtures {
				t.Run(fixture, func(t *testing.T) {
					repo := initReviewCLIRepo(t)
					if gate == reviewtransaction.GatePrePush && runReviewCLIGit(t, repo, "remote") != "" {
						t.Fatal("pre-push fixture unexpectedly has an upstream remote")
					}
					prepareShippedNonDecidingAuthorityFixture(t, repo, fixture)
					before := snapshotShippedReviewStore(t, repo)
					payload := runShippedNonDecidingReviewValidate(t, repo, gate)
					after := snapshotShippedReviewStore(t, repo)
					assertShippedReviewStoreUnchanged(t, before, after)
					assertEnabledUnmanagedGatePayload(t, payload, gate)
					validatePublishedReviewSchema(t, schema, payload)
					if expected == nil {
						expected = payload
						return
					}
					if !bytes.Equal(expected, payload) {
						t.Fatalf("historical authority fixture changed shipped gate output:\nexpected:\n%s\nactual:\n%s", expected, payload)
					}
				})
			}
		})
	}
}

type nonDecidingGateSemantic struct {
	Schema   string         `json:"schema"`
	Result   string         `json:"result"`
	Allowed  bool           `json:"allowed"`
	Action   string         `json:"action"`
	Reason   string         `json:"reason"`
	Context  map[string]any `json:"context"`
	Delivery string         `json:"delivery"`
}

func decodeNonDecidingGateSemantic(t *testing.T, payload []byte) nonDecidingGateSemantic {
	t.Helper()
	var result nonDecidingGateSemantic
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("decode non-deciding gate result: %v\n%s", err, payload)
	}
	return result
}

func TestShippedReviewValidateAllGatesNegotiatedRoutesAgree(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := RunReviewMode([]string{"enable", "--scope", "global", "--cwd", repo}, io.Discard); err != nil {
		t.Fatal(err)
	}
	schema := compileWholePublishedReviewSchema(t, "v2", "gate-result.schema.json")
	gates := []reviewtransaction.GateKind{
		reviewtransaction.GatePostApply,
		reviewtransaction.GatePreCommit,
		reviewtransaction.GatePrePush,
		reviewtransaction.GatePrePR,
		reviewtransaction.GateRelease,
	}
	for _, gate := range gates {
		t.Run(string(gate), func(t *testing.T) {
			plain := runShippedNonDecidingReviewValidate(t, repo, gate)
			want := decodeNonDecidingGateSemantic(t, plain)
			validatePublishedReviewSchema(t, schema, plain)

			for _, contract := range []string{ReviewIntegrationContractV1, ReviewIntegrationContractV2} {
				t.Run(contract, func(t *testing.T) {
					var output bytes.Buffer
					if err := RunReview([]string{"validate", "--contract", contract, "--cwd", repo, "--gate", string(gate)}, &output); err != nil {
						t.Fatalf("negotiated review validate: %v\n%s", err, output.String())
					}
					var envelope ReviewIntegrationOperationResult
					decodeStrictReviewJSON(t, output.Bytes(), &envelope)
					if envelope.Contract != contract || envelope.Operation != ReviewIntegrationOperationValidate {
						t.Fatalf("negotiated review validate envelope = %#v", envelope)
					}
					validatePublishedReviewSchema(t, schema, envelope.Result)
					if got := decodeNonDecidingGateSemantic(t, envelope.Result); !reflect.DeepEqual(got, want) {
						t.Fatalf("negotiated %s result = %#v, want %#v", contract, got, want)
					}
				})
			}

		})
	}
}

func TestShippedReviewValidateDisabledRouteIsByteStableAndUnmanaged(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := RunReviewMode([]string{"disable", "--scope", "global", "--cwd", repo}, io.Discard); err != nil {
		t.Fatalf("disable receipt-driven development: %v", err)
	}
	prepareShippedNonDecidingAuthorityFixture(t, repo, "lock residue")
	schema := compileWholePublishedReviewSchema(t, "v2", "gate-result.schema.json")
	gates := []reviewtransaction.GateKind{
		reviewtransaction.GatePostApply,
		reviewtransaction.GatePreCommit,
		reviewtransaction.GatePrePush,
		reviewtransaction.GatePrePR,
		reviewtransaction.GateRelease,
	}

	for _, gate := range gates {
		t.Run(string(gate), func(t *testing.T) {
			before := snapshotShippedReviewStore(t, repo)
			first := runShippedNonDecidingReviewValidate(t, repo, gate)
			afterFirst := snapshotShippedReviewStore(t, repo)
			assertShippedReviewStoreUnchanged(t, before, afterFirst)
			second := runShippedNonDecidingReviewValidate(t, repo, gate)
			afterSecond := snapshotShippedReviewStore(t, repo)
			assertShippedReviewStoreUnchanged(t, afterFirst, afterSecond)
			if !bytes.Equal(first, second) {
				t.Fatalf("disabled non-deciding gate output changed between identical calls:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			validatePublishedReviewSchema(t, schema, first)
			got := decodeNonDecidingGateSemantic(t, first)
			if got.Result != string(reviewtransaction.GateInvalidated) || got.Allowed || got.Action != reviewDeliveryPolicyAction ||
				got.Delivery != string(reviewtransaction.RDDDeliveryDisabledUnmanaged) || len(got.Context) != 1 || got.Context["gate"] != string(gate) {
				t.Fatalf("disabled non-deciding result = %#v", got)
			}
		})
	}
}
