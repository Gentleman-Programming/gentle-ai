package sddstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func TestResolveArchiveRequiresApprovedExactReviewReceipt(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(t *testing.T, changeRoot string, receipt reviewtransaction.Receipt, request reviewtransaction.GateRequest)
		wantGate    reviewtransaction.GateResult
		wantArchive DependencyState
		wantNext    string
		wantReason  string
	}{
		{
			name: "missing receipt mirror discovers native receipt",
			mutate: func(t *testing.T, changeRoot string, _ reviewtransaction.Receipt, _ reviewtransaction.GateRequest) {
				if err := os.Remove(filepath.Join(changeRoot, "reviews", "receipt.json")); err != nil {
					t.Fatal(err)
				}
			},
			wantGate: reviewtransaction.GateAllow, wantArchive: DependencyReady, wantNext: "archive",
		},
		{
			name: "missing every receipt invalidates archive",
			mutate: func(t *testing.T, changeRoot string, _ reviewtransaction.Receipt, _ reviewtransaction.GateRequest) {
				if err := os.Remove(filepath.Join(changeRoot, "reviews", "receipt.json")); err != nil {
					t.Fatal(err)
				}
				repo := filepath.Dir(filepath.Dir(filepath.Dir(changeRoot)))
				store, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, "thin-lineage")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(store.Dir, "artifacts", "receipt.json")); err != nil {
					t.Fatal(err)
				}
			},
			wantGate: reviewtransaction.GateInvalidated, wantArchive: DependencyBlocked,
			wantNext: "resolve-review", wantReason: "terminal review receipt is missing",
		},
		{
			name:     "exact authoritative artifacts allow archive",
			mutate:   func(_ *testing.T, _ string, _ reviewtransaction.Receipt, _ reviewtransaction.GateRequest) {},
			wantGate: reviewtransaction.GateAllow, wantArchive: DependencyReady,
			wantNext: "archive",
		},
		{
			name: "missing transaction mirror preserves native authority",
			mutate: func(t *testing.T, changeRoot string, _ reviewtransaction.Receipt, _ reviewtransaction.GateRequest) {
				if err := os.Remove(filepath.Join(changeRoot, "reviews", "transaction.json")); err != nil {
					t.Fatal(err)
				}
			},
			wantGate: reviewtransaction.GateAllow, wantArchive: DependencyReady, wantNext: "archive",
		},
		{
			name: "missing portable chain bundle preserves native authority",
			mutate: func(t *testing.T, changeRoot string, _ reviewtransaction.Receipt, _ reviewtransaction.GateRequest) {
				if err := os.Remove(filepath.Join(changeRoot, "reviews", "chain-bundle.json")); err != nil {
					t.Fatal(err)
				}
			},
			wantGate: reviewtransaction.GateAllow, wantArchive: DependencyReady, wantNext: "archive",
		},
		{
			name: "missing ledger mirror preserves native authority",
			mutate: func(t *testing.T, changeRoot string, _ reviewtransaction.Receipt, _ reviewtransaction.GateRequest) {
				if err := os.Remove(filepath.Join(changeRoot, "reviews", "ledger.json")); err != nil {
					t.Fatal(err)
				}
			},
			wantGate: reviewtransaction.GateAllow, wantArchive: DependencyReady, wantNext: "archive",
		},
		{
			name: "stale ledger mirror preserves native authority",
			mutate: func(t *testing.T, changeRoot string, _ reviewtransaction.Receipt, _ reviewtransaction.GateRequest) {
				write(t, filepath.Join(changeRoot, "reviews", "ledger.json"), "{\"schema\":\"gentle-ai.review-ledger/v1\",\"findings\":[{\"id\":\"stale\"}]}")
			},
			wantGate: reviewtransaction.GateAllow, wantArchive: DependencyReady, wantNext: "archive",
		},
		{
			name: "mismatched transaction mirror preserves native authority",
			mutate: func(t *testing.T, changeRoot string, _ reviewtransaction.Receipt, _ reviewtransaction.GateRequest) {
				var transaction reviewtransaction.Transaction
				path := filepath.Join(changeRoot, "reviews", "transaction.json")
				readJSON(t, path, &transaction)
				transaction.LineageID = "different-lineage"
				writeJSON(t, path, transaction)
			},
			wantGate: reviewtransaction.GateAllow, wantArchive: DependencyReady, wantNext: "archive",
		},
		{
			name: "pending receipt invalidates archive",
			mutate: func(t *testing.T, changeRoot string, receipt reviewtransaction.Receipt, _ reviewtransaction.GateRequest) {
				payload, err := json.Marshal(receipt)
				if err != nil {
					t.Fatalf("Marshal(receipt): %v", err)
				}
				pending := strings.Replace(string(payload), `"terminal_state":"approved"`, `"terminal_state":"pending"`, 1)
				write(t, filepath.Join(changeRoot, "reviews", "receipt.json"), pending)
			},
			wantGate: reviewtransaction.GateInvalidated, wantArchive: DependencyBlocked,
			wantNext: "resolve-review", wantReason: "invalid or non-terminal",
		},
		{
			name: "unrelated candidate requires new lineage",
			mutate: func(t *testing.T, changeRoot string, _ reviewtransaction.Receipt, _ reviewtransaction.GateRequest) {
				write(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Work\n- [x] scope changed\n")
			},
			wantGate: reviewtransaction.GateScopeChanged, wantArchive: DependencyBlocked,
			wantNext: "resolve-review", wantReason: "explicit new lineage",
		},
		{
			name: "stale gate context cannot override native authority",
			mutate: func(t *testing.T, changeRoot string, _ reviewtransaction.Receipt, request reviewtransaction.GateRequest) {
				request.ExternalEvidence = reviewtransaction.ExternalEvidenceEscalating
				writeJSON(t, filepath.Join(changeRoot, "reviews", "gate-context.json"), request)
			},
			wantGate: reviewtransaction.GateAllow, wantArchive: DependencyReady, wantNext: "archive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			changeRoot := seedBoundedReadyChange(t, root)
			writeApprovedReviewArtifacts(t, changeRoot)
			var receipt reviewtransaction.Receipt
			readJSON(t, filepath.Join(changeRoot, "reviews", "receipt.json"), &receipt)
			var request reviewtransaction.GateRequest
			readJSON(t, filepath.Join(changeRoot, "reviews", "gate-context.json"), &request)
			tt.mutate(t, changeRoot, receipt, request)

			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if status.ReviewGate == nil {
				t.Fatal("ReviewGate is nil for final archive decision")
			}
			if status.ReviewGate.Result != tt.wantGate {
				t.Fatalf("ReviewGate.Result = %q, want %q (%s)", status.ReviewGate.Result, tt.wantGate, status.ReviewGate.Reason)
			}
			if status.Dependencies.Archive != tt.wantArchive || status.NextRecommended != tt.wantNext {
				t.Fatalf("archive=%q next=%q, want %q/%q", status.Dependencies.Archive, status.NextRecommended, tt.wantArchive, tt.wantNext)
			}
			if tt.wantReason != "" && !strings.Contains(strings.Join(status.BlockedReasons, "\n"), tt.wantReason) {
				t.Fatalf("BlockedReasons = %v, want containing %q", status.BlockedReasons, tt.wantReason)
			}
		})
	}
}

func TestNativeReceiptDiscoveryDefersChangedScopeToOneNativeGateEvaluation(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedBoundedReadyChange(t, root)
	writeApprovedReviewArtifacts(t, changeRoot)
	if err := os.Remove(filepath.Join(changeRoot, "reviews", "receipt.json")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Work\n- [x] changed after review\n")
	original := evaluateNativeReviewGate
	count := 0
	evaluateNativeReviewGate = func(ctx context.Context, repo string, receipt reviewtransaction.Receipt, request reviewtransaction.GateRequest) reviewtransaction.NativeGateEvaluation {
		count++
		return reviewtransaction.EvaluateNativeGate(ctx, repo, receipt, request)
	}
	t.Cleanup(func() { evaluateNativeReviewGate = original })

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateScopeChanged {
		t.Fatalf("native evaluations=%d gate=%#v, want one scope-changed evaluation", count, status.ReviewGate)
	}
}

func TestNativeReceiptDiscoveryRejectsMultipleTerminalLineagesAsAmbiguous(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedBoundedReadyChange(t, root)
	writeApprovedReviewArtifacts(t, changeRoot)
	if err := os.Remove(filepath.Join(changeRoot, "reviews", "receipt.json")); err != nil {
		t.Fatal(err)
	}
	writeAdditionalApprovedNativeReceipt(t, root, "second-lineage")
	original := evaluateNativeReviewGate
	count := 0
	evaluateNativeReviewGate = func(ctx context.Context, repo string, receipt reviewtransaction.Receipt, request reviewtransaction.GateRequest) reviewtransaction.NativeGateEvaluation {
		count++
		return reviewtransaction.EvaluateNativeGate(ctx, repo, receipt, request)
	}
	t.Cleanup(func() { evaluateNativeReviewGate = original })

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateInvalidated || !strings.Contains(status.ReviewGate.Reason, "restore the change-local reviews/receipt.json mirror") {
		t.Fatalf("native evaluations=%d gate=%#v, want ambiguous discovery before evaluation", count, status.ReviewGate)
	}
}

func TestReviewArtifactPathsUseExactOpenSpecAndEngramReferences(t *testing.T) {
	changeRoot := t.TempDir()
	for _, name := range []string{"transaction.json", "policy.md", "ledger.json", "receipt.json", "chain-bundle.json", "gate-context.json"} {
		write(t, filepath.Join(changeRoot, "reviews", name), "{}\n")
	}
	paths, err := resolveArtifactPaths(changeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := firstPath(paths.ReviewState); got != filepath.Join(changeRoot, "reviews", "transaction.json") {
		t.Fatalf("OpenSpec transaction path = %q", got)
	}
	if got := firstPath(paths.ReviewLedger); got != filepath.Join(changeRoot, "reviews", "ledger.json") {
		t.Fatalf("OpenSpec ledger path = %q", got)
	}
	if got := firstPath(paths.ReviewPolicy); got != filepath.Join(changeRoot, "reviews", "policy.md") {
		t.Fatalf("OpenSpec policy path = %q", got)
	}

	engram := engramArtifactPaths("bounded", map[string]engramObservation{
		"review/transaction":  {Title: "sdd/bounded/review/transaction", Content: "{}"},
		"review/policy":       {Title: "sdd/bounded/review/policy", Content: "policy"},
		"review/ledger":       {Title: "sdd/bounded/review/ledger", Content: "{}"},
		"review/receipt":      {Title: "sdd/bounded/review/receipt", Content: "{}"},
		"review/chain-bundle": {Title: "sdd/bounded/review/chain-bundle", Content: "{}"},
		"review/gate-context": {Title: "sdd/bounded/review/gate-context", Content: "{}"},
	})
	for label, got := range map[string]string{
		"transaction":  firstPath(engram.ReviewState),
		"ledger":       firstPath(engram.ReviewLedger),
		"policy":       firstPath(engram.ReviewPolicy),
		"receipt":      firstPath(engram.ReviewReceipt),
		"chain-bundle": firstPath(engram.ReviewBundle),
		"context":      firstPath(engram.ReviewContext),
	} {
		want := "sdd/bounded/review/" + label
		if label == "context" {
			want = "sdd/bounded/review/gate-context"
		}
		if got != want {
			t.Errorf("Engram %s path = %q, want %q", label, got, want)
		}
	}
}

func TestEngramReviewArtifactDiscoveryRetainsOnlyExactNames(t *testing.T) {
	observations := []engramObservation{
		{Title: "sdd/bounded/review/chain-bundle", Content: "exact bundle", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/bounded/review/chain-bundles", Content: "plural", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/bounded/review/chain-bundle/extra", Content: "nested", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/bounded/review/arbitrary", Content: "arbitrary", Project: "gentle-ai", Scope: "project"},
	}

	changes := collectEngramChanges(observations, "gentle-ai")
	if !reflect.DeepEqual(changes, []string{"bounded"}) {
		t.Fatalf("collectEngramChanges() = %v, want exact chain-bundle change", changes)
	}
	artifacts := engramArtifactsForChange(observations, "gentle-ai", "bounded")
	if len(artifacts) != 1 || artifacts["review/chain-bundle"].Content != "exact bundle" {
		t.Fatalf("engramArtifactsForChange() = %#v, want only exact review/chain-bundle", artifacts)
	}
}

func TestResolveEngramArchiveRecoversRetainedPolicyWithoutSourceArtifacts(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedBoundedReadyChange(t, root)
	writeApprovedReviewArtifacts(t, changeRoot)
	mkdir(t, filepath.Join(root, ".engram"))
	project := strings.ToLower(filepath.Base(root))
	observations := boundedEngramObservations(t, project, changeRoot)
	if err := os.RemoveAll(filepath.Join(changeRoot, "reviews")); err != nil {
		t.Fatal(err)
	}
	restore := stubEngramExport(t, observations)
	defer restore()

	status, ok, err := resolveEngramStatus(root, "thin", false)
	if err != nil {
		t.Fatalf("resolveEngramStatus() error = %v", err)
	}
	if !ok {
		t.Fatal("resolveEngramStatus() did not retain the Engram change")
	}
	if status.Artifacts["reviewBundle"] != ArtifactDone || firstPath(status.ArtifactPaths.ReviewBundle) != "sdd/thin/review/chain-bundle" {
		t.Fatalf("Engram bundle artifact = %q paths=%v", status.Artifacts["reviewBundle"], status.ArtifactPaths.ReviewBundle)
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow || status.Dependencies.Archive != DependencyReady || status.NextRecommended != "archive" {
		t.Fatalf("Engram archive status = gate=%#v archive=%q next=%q", status.ReviewGate, status.Dependencies.Archive, status.NextRecommended)
	}
}

func TestResolveEngramArchiveIgnoresNonAuthoritativeRetainedPolicy(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedBoundedReadyChange(t, root)
	writeApprovedReviewArtifacts(t, changeRoot)
	mkdir(t, filepath.Join(root, ".engram"))
	observations := boundedEngramObservations(t, strings.ToLower(filepath.Base(root)), changeRoot)
	for index := range observations {
		if observations[index].Title == "sdd/thin/review/policy" {
			observations[index].Content = "tampered policy\n"
		}
	}
	if err := os.RemoveAll(filepath.Join(changeRoot, "reviews")); err != nil {
		t.Fatal(err)
	}
	restore := stubEngramExport(t, observations)
	defer restore()

	status, ok, err := resolveEngramStatus(root, "thin", false)
	if err != nil {
		t.Fatalf("resolveEngramStatus() error = %v", err)
	}
	if !ok {
		t.Fatal("resolveEngramStatus() did not retain the Engram change")
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow || status.Dependencies.Archive != DependencyReady {
		t.Fatalf("tampered Engram policy status = gate=%#v archive=%q", status.ReviewGate, status.Dependencies.Archive)
	}
}

func boundedEngramObservations(t *testing.T, project, changeRoot string) []engramObservation {
	t.Helper()
	observation := func(title, path string) engramObservation {
		t.Helper()
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return engramObservation{Title: title, Content: string(payload), Project: project, Scope: "project"}
	}
	return []engramObservation{
		observation("sdd/thin/proposal", filepath.Join(changeRoot, "proposal.md")),
		observation("sdd/thin/spec", filepath.Join(changeRoot, "specs", "auth", "spec.md")),
		observation("sdd/thin/design", filepath.Join(changeRoot, "design.md")),
		observation("sdd/thin/tasks", filepath.Join(changeRoot, "tasks.md")),
		{Title: "sdd/thin/apply-progress", Content: "all work units complete", Project: project, Scope: "project"},
		observation("sdd/thin/verify-report", filepath.Join(changeRoot, "verify-report.md")),
		observation("sdd/thin/review/transaction", filepath.Join(changeRoot, "reviews", "transaction.json")),
		observation("sdd/thin/review/policy", filepath.Join(changeRoot, "reviews", "policy.md")),
		observation("sdd/thin/review/ledger", filepath.Join(changeRoot, "reviews", "ledger.json")),
		observation("sdd/thin/review/receipt", filepath.Join(changeRoot, "reviews", "receipt.json")),
		observation("sdd/thin/review/chain-bundle", filepath.Join(changeRoot, "reviews", "chain-bundle.json")),
		observation("sdd/thin/review/gate-context", filepath.Join(changeRoot, "reviews", "gate-context.json")),
	}
}

const incompatibleTransactionSummary = "# Native Review Transaction\n\n- lineage: thin-lineage\n- state: approved\n"

func TestReadReviewTransactionRejectsIncompatibleNonJSONArtifact(t *testing.T) {
	markdownPath := filepath.Join(t.TempDir(), "transaction.json")
	write(t, markdownPath, incompatibleTransactionSummary)
	for _, tt := range []struct {
		name    string
		path    string
		content string
	}{
		{name: "engram markdown content", content: incompatibleTransactionSummary},
		{name: "markdown transaction mirror", path: markdownPath},
	} {
		t.Run(tt.name, func(t *testing.T) {
			transaction, reason := readReviewTransaction(tt.path, tt.content)
			if transaction != nil {
				t.Fatalf("readReviewTransaction() = %#v, want nil transaction", transaction)
			}
			if strings.Contains(reason, "invalid character") {
				t.Fatalf("readReviewTransaction() reason = %q, want compatibility reason instead of raw JSON parse error", reason)
			}
			if !strings.Contains(reason, "not a native JSON review transaction") {
				t.Fatalf("readReviewTransaction() reason = %q, want incompatible-artifact reason", reason)
			}
		})
	}
}

func TestReadReviewTransactionRejectsJSONNonObjectsAsInvalid(t *testing.T) {
	for _, payload := range []string{`[]`, `null`, `"legacy"`} {
		t.Run(payload, func(t *testing.T) {
			transaction, reason := readReviewTransaction("", payload)
			if transaction != nil || !strings.Contains(reason, "bounded review transaction is invalid") {
				t.Fatalf("readReviewTransaction() = (%#v, %q), want invalid native transaction", transaction, reason)
			}
		})
	}
}

func TestResolveEngramBridgesCompactAuthorityOverIncompatibleTransactionArtifact(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "compact-thin")
	mkdir(t, filepath.Join(root, ".engram"))
	project := strings.ToLower(filepath.Base(root))
	observation := func(title, path string) engramObservation {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return engramObservation{Title: title, Content: string(payload), Project: project, Scope: "project"}
	}
	observations := []engramObservation{
		observation("sdd/thin/proposal", filepath.Join(changeRoot, "proposal.md")),
		observation("sdd/thin/spec", filepath.Join(changeRoot, "specs", "auth", "spec.md")),
		observation("sdd/thin/design", filepath.Join(changeRoot, "design.md")),
		observation("sdd/thin/tasks", filepath.Join(changeRoot, "tasks.md")),
		{Title: "sdd/thin/apply-progress", Content: "all work units complete", Project: project, Scope: "project"},
		{Title: "sdd/thin/review/transaction", Content: incompatibleTransactionSummary, Project: project, Scope: "project"},
	}
	restore := stubEngramExport(t, observations)
	defer restore()

	status, ok, err := resolveEngramStatus(root, "thin", false)
	if err != nil {
		t.Fatalf("resolveEngramStatus() error = %v", err)
	}
	if !ok {
		t.Fatal("resolveEngramStatus() did not retain the Engram change")
	}
	if status.ReviewTransaction != nil {
		t.Fatalf("ReviewTransaction = %#v, want incompatible artifact treated as missing", status.ReviewTransaction)
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if strings.Contains(reasons, "invalid character") {
		t.Fatalf("BlockedReasons = %v, want no raw JSON parse error", status.BlockedReasons)
	}
	if status.Dependencies.Verify != DependencyReady || status.NextRecommended != "verify" {
		t.Fatalf("verify=%q next=%q reasons=%v, want compact authority bridged to ready/verify", status.Dependencies.Verify, status.NextRecommended, status.BlockedReasons)
	}
}

func TestResolveEngramDoesNotBridgeCompactAuthorityOverMalformedJSONTransaction(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "compact-thin")
	mkdir(t, filepath.Join(root, ".engram"))
	project := strings.ToLower(filepath.Base(root))
	observation := func(title, path string) engramObservation {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return engramObservation{Title: title, Content: string(payload), Project: project, Scope: "project"}
	}
	observations := []engramObservation{
		observation("sdd/thin/proposal", filepath.Join(changeRoot, "proposal.md")),
		observation("sdd/thin/spec", filepath.Join(changeRoot, "specs", "auth", "spec.md")),
		observation("sdd/thin/design", filepath.Join(changeRoot, "design.md")),
		observation("sdd/thin/tasks", filepath.Join(changeRoot, "tasks.md")),
		{Title: "sdd/thin/apply-progress", Content: "all work units complete", Project: project, Scope: "project"},
		{Title: "sdd/thin/review/transaction", Content: `{"state":`, Project: project, Scope: "project"},
	}
	restore := stubEngramExport(t, observations)
	defer restore()

	status, ok, err := resolveEngramStatus(root, "thin", false)
	if err != nil {
		t.Fatalf("resolveEngramStatus() error = %v", err)
	}
	if !ok {
		t.Fatal("resolveEngramStatus() did not retain the Engram change")
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if status.Dependencies.Verify != DependencyBlocked || status.NextRecommended != "review" {
		t.Fatalf("verify=%q next=%q reasons=%v, want malformed JSON to remain blocked on review", status.Dependencies.Verify, status.NextRecommended, status.BlockedReasons)
	}
	if !strings.Contains(reasons, "bounded review transaction is invalid") {
		t.Fatalf("BlockedReasons = %v, want malformed JSON reason", status.BlockedReasons)
	}
	for _, payload := range []string{`[]`, `null`, `"legacy"`} {
		observations[len(observations)-1].Content = payload
		status, _, err = resolveEngramStatus(root, "thin", false)
		if err != nil {
			t.Fatalf("resolveEngramStatus(%s) error = %v", payload, err)
		}
		if status.Dependencies.Verify != DependencyBlocked || status.NextRecommended != "review" {
			t.Fatalf("payload=%s verify=%q next=%q, want blocked/review", payload, status.Dependencies.Verify, status.NextRecommended)
		}
	}
}

func TestResolveEngramFailsClosedOnIncompatibleTransactionWithoutNativeAuthority(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, ".engram"))
	project := strings.ToLower(filepath.Base(root))
	observations := []engramObservation{
		{Title: "sdd/thin/proposal", Content: "# Proposal\n", Project: project, Scope: "project"},
		{Title: "sdd/thin/spec", Content: "### Requirement: Auth\n#### Scenario: Valid login\n", Project: project, Scope: "project"},
		{Title: "sdd/thin/design", Content: "# Design\n", Project: project, Scope: "project"},
		{Title: "sdd/thin/tasks", Content: "- [x] 1.1 Done\n", Project: project, Scope: "project"},
		{Title: "sdd/thin/apply-progress", Content: "all work units complete", Project: project, Scope: "project"},
		{Title: "sdd/thin/review/transaction", Content: incompatibleTransactionSummary, Project: project, Scope: "project"},
	}
	restore := stubEngramExport(t, observations)
	defer restore()

	status, ok, err := resolveEngramStatus(root, "thin", false)
	if err != nil {
		t.Fatalf("resolveEngramStatus() error = %v", err)
	}
	if !ok {
		t.Fatal("resolveEngramStatus() did not retain the Engram change")
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if strings.Contains(reasons, "invalid character") {
		t.Fatalf("BlockedReasons = %v, want no raw JSON parse error", status.BlockedReasons)
	}
	if status.Dependencies.Verify != DependencyBlocked || !strings.Contains(reasons, "not a native JSON review transaction") {
		t.Fatalf("verify=%q reasons=%v, want fail-closed compatibility reason", status.Dependencies.Verify, status.BlockedReasons)
	}
}

func TestResolveFinalVerifyWaitsForAllTasks(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n- [ ] 1.2 Pending\n")
	write(t, filepath.Join(changeRoot, "apply-progress.md"), "focused work-unit checks passed\n")
	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Dependencies.Verify != DependencyBlocked || status.NextRecommended != "apply" {
		t.Fatalf("verify=%q next=%q, want blocked/apply", status.Dependencies.Verify, status.NextRecommended)
	}
}

func TestResolveStartsBoundedReviewBeforeFinalVerification(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Dependencies.Verify != DependencyBlocked || status.NextRecommended != "review" {
		t.Fatalf("without transaction verify=%q next=%q, want blocked/review", status.Dependencies.Verify, status.NextRecommended)
	}
	dispatcher := RenderDispatcherMarkdown(status)
	for _, want := range []string{"### Next Review Operation", "gentle-ai review start", "gentle-ai review finalize", "gentle-ai review validate --gate post-apply", "reconcile existing terminal mirrors"} {
		if !strings.Contains(dispatcher, want) {
			t.Fatalf("dispatcher missing %q:\n%s", want, dispatcher)
		}
	}
	for _, forbidden := range []string{"review-start", "review-step", "review-resume", "review-validate", "review-bundle-export", "--machine-transaction-out", "--intended-untracked-manifest"} {
		if strings.Contains(dispatcher, forbidden) {
			t.Fatalf("dispatcher exposes lower-level review command %q:\n%s", forbidden, dispatcher)
		}
	}

	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: "thin-lineage", Mode: reviewtransaction.ModeOrdinary4R, Generation: 1,
		Snapshot: reviewtransaction.Snapshot{
			Kind: reviewtransaction.TargetCurrentChanges, BaseTree: strings.Repeat("a", 40), CandidateTree: strings.Repeat("b", 40),
			PathsDigest: shaID("2"), IntendedUntracked: []string{}, IntendedUntrackedProof: shaID("3"), Paths: []string{"internal/example.go"}, Identity: shaID("4"),
		},
		PolicyHash: shaID("5"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	if err := freezeStatusFindings(tx, []reviewtransaction.Finding{}); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ClassifyEvidence([]reviewtransaction.FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(changeRoot, "reviews", "transaction.json"), tx)

	status, err = Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Dependencies.Verify != DependencyReady || status.NextRecommended != "verify" {
		t.Fatalf("ready transaction verify=%q next=%q, want ready/verify", status.Dependencies.Verify, status.NextRecommended)
	}
}

func TestDiscoverCompactPreVerifyAuthorityFailsClosedWithoutExactlyOneEligibleStore(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	runSDDStatusGit(t, root, "init", "-q")
	runSDDStatusGit(t, root, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, root, "config", "user.name", "Status Test")
	runSDDStatusGit(t, root, "add", ".")
	runSDDStatusGit(t, root, "commit", "-qm", "base")

	bridge := discoverCompactPreVerifyAuthority(context.Background(), root, "thin", "")
	if bridge.Eligible || !strings.Contains(bridge.Reason, "no eligible") {
		t.Fatalf("empty compact bridge = %#v", bridge)
	}
}

func TestResolveBridgesExactlyOnePathBoundCompactAuthorityToVerifyOnly(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "compact-thin")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "verify" || status.ReviewTransaction != nil {
		t.Fatalf("compact bridge status = %#v", status)
	}
	if _, err := os.Stat(filepath.Join(changeRoot, "reviews", "transaction.json")); !os.IsNotExist(err) {
		t.Fatalf("bridge synthesized local review mirror: %v", err)
	}
}

const emptyOutputHash = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestAuthorityOnlyFailedReportRequiresStructuredFailClosedEvidence(t *testing.T) {
	valid := authorityOnlyVerifyEnvelope(shaID("0"), "125", "125")
	for _, tt := range []struct {
		name   string
		report string
		want   bool
	}{
		{name: "valid authority-only preflight denial", report: valid, want: true},
		{name: "zero exits are not preflight denial", report: authorityOnlyVerifyEnvelope(shaID("0"), "0", "0")},
		{name: "arbitrary output hashes are not empty output", report: strings.Replace(valid, emptyOutputHash, shaID("2"), 1)},
		{name: "markdown substrings are not evidence", report: boundedVerifyEnvelope(shaID("1"), "fail") + "\nauthority_only_failure: true\nmissing_review_authority: true\n"},
		{name: "executed command failed", report: strings.Replace(valid, "test_exit_code: 125", "test_exit_code: 1", 1)},
		{name: "substantive failure", report: strings.Replace(valid, "substantive_failure: false", "substantive_failure: true", 1)},
		{name: "malformed blocker count", report: strings.Replace(valid, "blockers: 1", "blockers: unknown", 1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := authorityOnlyFailedReport(tt.report); got != tt.want {
				t.Fatalf("authorityOnlyFailedReport() = %v, want %v", got, tt.want)
			}
		})
	}
	if authorityChangedSinceReport(valid, shaID("0")) {
		t.Fatal("unchanged authority revision must not permit recovery")
	}
}

func TestResolveRecoversOnlyAuthorityMissingHistoricalVerification(t *testing.T) {
	for _, tt := range []struct {
		name      string
		testExit  string
		wantReady bool
	}{
		{name: "new authority permits retry", testExit: "125", wantReady: true},
		{name: "command failure remains denied", testExit: "1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
			write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Valid login\n")
			report := authorityOnlyVerifyEnvelope(shaID("0"), tt.testExit, "125")
			write(t, filepath.Join(changeRoot, "verify-report.md"), report)
			writeApprovedCompactAuthorityForChange(t, root, changeRoot, "compact-thin")

			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantReady {
				if status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "verify" {
					t.Fatalf("recovery status = %#v", status)
				}
				if status.RemediationState != (RemediationState{}) || strings.Contains(strings.Join(status.BlockedReasons, "\n"), "remediation") {
					t.Fatalf("recovery retained remediation blockers: %#v", status)
				}
				if got := readText(filepath.Join(changeRoot, "verify-report.md")); got != report {
					t.Fatal("recovery rewrote historical verification evidence")
				}
			} else if status.Dependencies.Verify != DependencyBlocked || status.NextRecommended == "verify" {
				t.Fatalf("denied recovery status = %#v", status)
			}
		})
	}
}

func TestResolveRecoversWithObservedStaleAndNewLiveCompactLineages(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Valid login\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "compact-stale")
	staleStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, "compact-stale")
	if err != nil {
		t.Fatal(err)
	}
	staleRecord, err := staleStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	report := authorityOnlyVerifyEnvelope(staleRecord.Revision, "125", "125")
	write(t, filepath.Join(changeRoot, "verify-report.md"), report)

	write(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n# stale intermediate scope\n")
	writeApprovedCompactAuthorityForChangeWithTasks(t, root, changeRoot, "compact-live", "- [x] 1.1 Done\n# new approved scope\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "verify" {
		t.Fatalf("multi-lineage recovery status = %#v", status)
	}
	if got := readText(filepath.Join(changeRoot, "verify-report.md")); got != report {
		t.Fatal("multi-lineage recovery rewrote historical verification evidence")
	}
}

func TestObservedRevisionSkipsOnlyScopeChangedPredecessor(t *testing.T) {
	for _, tt := range []struct {
		name     string
		observed string
		revision string
		result   reviewtransaction.GateResult
		want     bool
	}{
		{name: "matching scope changed predecessor", observed: shaID("a"), revision: shaID("a"), result: reviewtransaction.GateScopeChanged, want: true},
		{name: "matching invalidated predecessor", observed: shaID("a"), revision: shaID("a"), result: reviewtransaction.GateInvalidated},
		{name: "matching escalated predecessor", observed: shaID("a"), revision: shaID("a"), result: reviewtransaction.GateEscalated},
		{name: "matching allow authority", observed: shaID("a"), revision: shaID("a"), result: reviewtransaction.GateAllow},
		{name: "different scope changed authority", observed: shaID("a"), revision: shaID("b"), result: reviewtransaction.GateScopeChanged},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := skipsObservedStalePredecessor(tt.observed, tt.revision, tt.result); got != tt.want {
				t.Fatalf("skipsObservedStalePredecessor(%q, %q, %q) = %v, want %v", tt.observed, tt.revision, tt.result, got, tt.want)
			}
		})
	}
}

func TestCompactAuthorityPathBindingRejectsForeignAndTraversalOpenSpecPaths(t *testing.T) {
	base := reviewtransaction.CompactState{GenesisPaths: []string{"openspec/changes/thin/tasks.md"}, InitialSnapshot: reviewtransaction.Snapshot{Paths: []string{"openspec/changes/thin/tasks.md"}}}
	for _, tt := range []struct {
		name       string
		paths      []string
		wantReason bool
	}{
		{name: "foreign change is irrelevant", paths: []string{"openspec/changes/other/tasks.md"}},
		{name: "traversal", paths: []string{"openspec/changes/thin/../other/tasks.md"}, wantReason: true},
		{name: "dot segment", paths: []string{"openspec/changes/thin/./tasks.md"}, wantReason: true},
		{name: "duplicate separator", paths: []string{"openspec/changes/thin//tasks.md"}, wantReason: true},
		{name: "mixed foreign path", paths: []string{"openspec/changes/thin/tasks.md", "openspec/config.yaml"}, wantReason: true},
		{name: "mixed foreign change", paths: []string{"openspec/changes/thin/tasks.md", "openspec/changes/other/tasks.md"}, wantReason: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := base
			state.GenesisPaths, state.InitialSnapshot.Paths = tt.paths, tt.paths
			if bound, reason := compactAuthorityPathsBound(state, "thin"); bound || (reason != "") != tt.wantReason {
				t.Fatalf("paths %v = bound=%v reason=%q, wantReason=%v", tt.paths, bound, reason, tt.wantReason)
			}
		})
	}
}

func TestDiscoverCompactPreVerifyAuthorityIgnoresForeignChangeAuthority(t *testing.T) {
	root := t.TempDir()
	activeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	foreignRoot := seedReadyChange(t, root, "other", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, foreignRoot, "compact-other")
	writeApprovedCompactAuthorityForChange(t, root, activeRoot, "compact-thin")

	bridge := discoverCompactPreVerifyAuthority(context.Background(), root, "thin", "")
	if !bridge.Eligible || bridge.Relevant || bridge.Reason != "" {
		t.Fatalf("bridge with foreign authority = %#v, want one eligible active authority", bridge)
	}
}

func TestResolveBlocksAmbiguousPathBoundCompactAuthorities(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "compact-one")
	first, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, "compact-one")
	if err != nil {
		t.Fatal(err)
	}
	record, err := first.Load()
	if err != nil {
		t.Fatal(err)
	}
	lines := record.State.OriginalChangedLines
	secondState, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: "compact-two", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: record.State.InitialSnapshot, PolicyHash: record.State.PolicyHash, RiskLevel: record.State.RiskLevel,
		SelectedLenses: record.State.SelectedLenses, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, secondState.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := second.Replace("", "review/start", secondState)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]reviewtransaction.LensResult, len(secondState.SelectedLenses))
	for index, lens := range secondState.SelectedLenses {
		results[index] = reviewtransaction.LensResult{Lens: lens, Findings: []reviewtransaction.Finding{}, Evidence: []string{"review complete"}}
	}
	if err := secondState.CompleteReview(reviewtransaction.CompactReviewInput{LensResults: results, Classifications: []reviewtransaction.FindingEvidence{}, RefuterOutcomes: []reviewtransaction.EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = second.Replace(revision, "review/complete-review", secondState)
	if err != nil {
		t.Fatal(err)
	}
	if err := secondState.CompleteVerification([]byte("verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Replace(revision, "review/complete-verification", secondState); err != nil {
		t.Fatal(err)
	}
	receipt, err := secondState.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(second.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Dependencies.Verify != DependencyBlocked || status.NextRecommended != "resolve-review" || !strings.Contains(strings.Join(status.BlockedReasons, "\n"), "multiple eligible") {
		t.Fatalf("ambiguous compact bridge status = %#v", status)
	}
}

func TestResolveRejectsReceiptMismatchAndNonAllowCompactBridge(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(t *testing.T, root string)
	}{
		{name: "receipt mismatch", mutate: func(t *testing.T, root string) {
			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, "compact-thin")
			if err != nil {
				t.Fatal(err)
			}
			write(t, store.ReceiptPath(), "{}\n")
		}},
		{name: "post apply gate denies", mutate: func(t *testing.T, root string) {
			write(t, filepath.Join(root, "openspec", "changes", "thin", "tasks.md"), "- [x] 1.1 Done\n# changed after compact approval\n")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
			writeApprovedCompactAuthorityForChange(t, root, changeRoot, "compact-thin")
			tt.mutate(t, root)
			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			if err != nil {
				t.Fatal(err)
			}
			if status.Dependencies.Verify != DependencyBlocked || status.NextRecommended != "resolve-review" || status.Dependencies.Archive != DependencyBlocked {
				t.Fatalf("%s status = %#v", tt.name, status)
			}
		})
	}
}

func TestResolveRoutesStaleVerifyEvidenceToVerifyUnderApprovedCompactAuthority(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Valid login\n#### Scenario: Rejected login\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("d"), "pass"))
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "compact-thin")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("ReviewGate = %#v, want allow", status.ReviewGate)
	}
	if len(status.BlockedReasons) != 0 {
		t.Fatalf("BlockedReasons = %v, want empty", status.BlockedReasons)
	}
	if status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "verify" {
		t.Fatalf("verify=%q archive=%q next=%q, want ready/blocked/verify", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended)
	}
	if status.RemediationState != (RemediationState{}) {
		t.Fatalf("RemediationState = %#v, want empty for stale evidence", status.RemediationState)
	}
}

func TestResolveRejectsForeignCompactAuthorityForStaleVerifyEvidence(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	foreignRoot := seedReadyChange(t, root, "other", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Valid login\n#### Scenario: Rejected login\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("d"), "pass"))
	writeApprovedCompactAuthorityForChange(t, root, foreignRoot, "compact-other")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Dependencies.Verify != DependencyBlocked || status.NextRecommended != "resolve-review" {
		t.Fatalf("verify=%q next=%q, want blocked/resolve-review", status.Dependencies.Verify, status.NextRecommended)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), `compact review authority is not bound to selected change "thin"`) {
		t.Fatalf("BlockedReasons = %v, want foreign-authority rejection", status.BlockedReasons)
	}
}

func TestResolveEngramRoutesStaleVerifyEvidenceToVerifyUnderApprovedCompactAuthority(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "compact-thin")
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, "compact-thin")
	if err != nil {
		t.Fatal(err)
	}
	receiptPayload := readText(store.ReceiptPath())
	if receiptPayload == "" {
		t.Fatal("compact receipt payload is empty")
	}
	mkdir(t, filepath.Join(root, ".engram"))
	project := strings.ToLower(filepath.Base(root))
	restore := stubEngramExport(t, []engramObservation{
		{Title: "sdd/thin/proposal", Content: "# Proposal\n", Project: project, Scope: "project"},
		{Title: "sdd/thin/spec", Content: "### Requirement: Auth\n#### Scenario: Valid login\n#### Scenario: Rejected login\n", Project: project, Scope: "project"},
		{Title: "sdd/thin/design", Content: "# Design\n", Project: project, Scope: "project"},
		{Title: "sdd/thin/tasks", Content: "- [x] 1.1 Done\n", Project: project, Scope: "project"},
		{Title: "sdd/thin/verify-report", Content: boundedVerifyEnvelope(shaID("d"), "pass"), Project: project, Scope: "project"},
		{Title: "sdd/thin/review/receipt", Content: receiptPayload, Project: project, Scope: "project"},
	})
	defer restore()

	status, ok, err := resolveEngramStatus(root, "thin", false)
	if err != nil {
		t.Fatalf("resolveEngramStatus() error = %v", err)
	}
	if !ok {
		t.Fatal("resolveEngramStatus() did not retain the Engram change")
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("ReviewGate = %#v, want allow", status.ReviewGate)
	}
	if len(status.BlockedReasons) != 0 {
		t.Fatalf("BlockedReasons = %v, want empty", status.BlockedReasons)
	}
	if status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "verify" {
		t.Fatalf("verify=%q archive=%q next=%q, want ready/blocked/verify", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended)
	}
	if status.RemediationState != (RemediationState{}) {
		t.Fatalf("RemediationState = %#v, want empty for stale evidence", status.RemediationState)
	}
}

func TestResolveEngramRejectsForeignCompactAuthorityForStaleVerifyEvidence(t *testing.T) {
	root := t.TempDir()
	foreignRoot := seedReadyChange(t, root, "other", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, foreignRoot, "compact-other")
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, "compact-other")
	if err != nil {
		t.Fatal(err)
	}
	receiptPayload := readText(store.ReceiptPath())
	mkdir(t, filepath.Join(root, ".engram"))
	project := strings.ToLower(filepath.Base(root))
	restore := stubEngramExport(t, []engramObservation{
		{Title: "sdd/thin/proposal", Content: "# Proposal\n", Project: project, Scope: "project"},
		{Title: "sdd/thin/spec", Content: "### Requirement: Auth\n#### Scenario: Valid login\n#### Scenario: Rejected login\n", Project: project, Scope: "project"},
		{Title: "sdd/thin/design", Content: "# Design\n", Project: project, Scope: "project"},
		{Title: "sdd/thin/tasks", Content: "- [x] 1.1 Done\n", Project: project, Scope: "project"},
		{Title: "sdd/thin/verify-report", Content: boundedVerifyEnvelope(shaID("d"), "pass"), Project: project, Scope: "project"},
		{Title: "sdd/thin/review/receipt", Content: receiptPayload, Project: project, Scope: "project"},
	})
	defer restore()

	status, ok, err := resolveEngramStatus(root, "thin", false)
	if err != nil || !ok {
		t.Fatalf("resolveEngramStatus() = ok %v, error %v", ok, err)
	}
	if status.Dependencies.Verify != DependencyBlocked || status.NextRecommended != "resolve-review" {
		t.Fatalf("verify=%q next=%q, want blocked/resolve-review", status.Dependencies.Verify, status.NextRecommended)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), `compact review authority is not bound to selected change "thin"`) {
		t.Fatalf("BlockedReasons = %v, want foreign-authority rejection", status.BlockedReasons)
	}
}

func TestResolveGrantsCompactRemediationBudgetForFailedVerdictWithIncompleteScenarios(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Valid login\n#### Scenario: Added after verification\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("d"), "fail"))
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "compact-thin")
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, "compact-thin")
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Dependencies.Verify != DependencyBlocked || status.NextRecommended != "remediate" {
		t.Fatalf("verify=%q next=%q, want blocked/remediate for failed verdict", status.Dependencies.Verify, status.NextRecommended)
	}
	wantBudget := before.State.CorrectionBudget - before.State.CumulativeCorrectionLines
	if !status.RemediationState.Required || status.RemediationState.CorrectionBudget != wantBudget || wantBudget <= 0 || status.RemediationState.LineageID != "compact-thin" || status.RemediationState.FailedEvidenceRevision != shaID("d") {
		t.Fatalf("RemediationState = %#v, want transaction-bound nonzero compact budget", status.RemediationState)
	}
	after, err := store.Load()
	if err != nil || after.Revision != before.Revision {
		t.Fatalf("compact authority changed during status resolution: before=%q after=%q err=%v", before.Revision, after.Revision, err)
	}
}

func TestResolveRejectsCompactRemediationWhenFrozenBudgetIsZero(t *testing.T) {
	compact := reviewtransaction.CompactState{
		LineageID: "compact-thin", Generation: 1, State: reviewtransaction.StateApproved,
		CorrectionBudget: 2, CumulativeCorrectionLines: 2,
	}
	state := resolveBoundedRemediation(true, verifyResultEvaluation{
		EvidenceRevision: shaID("d"), Reason: "scenarios are incomplete",
	}, nil, &compact, "bounded review transaction is missing", "")

	if state.Required || !strings.Contains(state.Reason, "compact review authority has no correction budget") {
		t.Fatalf("RemediationState = %#v, want fail-closed exhausted budget", state)
	}
}

func TestResolveRemediationIsBoundToBudgetAndFailedEvidenceRevision(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Valid login\n")
	revision := shaID("d")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(revision, "fail"))

	tx := remediationTransaction(t, revision, false)
	writeJSON(t, filepath.Join(changeRoot, "reviews", "transaction.json"), tx)
	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "remediate" || !status.RemediationState.Required {
		t.Fatalf("next=%q remediation=%#v, want bounded remediation", status.NextRecommended, status.RemediationState)
	}

	stale := remediationTransaction(t, shaID("e"), false)
	writeJSON(t, filepath.Join(changeRoot, "reviews", "transaction.json"), stale)
	status, err = Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve(stale) error = %v", err)
	}
	if status.NextRecommended != "resolve-review" || status.RemediationState.Required {
		t.Fatalf("stale next=%q remediation=%#v", status.NextRecommended, status.RemediationState)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), "does not match failed evidence revision") {
		t.Fatalf("BlockedReasons = %v", status.BlockedReasons)
	}
}

func seedBoundedReadyChange(t *testing.T, root string) string {
	t.Helper()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Valid login\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("1"), "pass"))
	return changeRoot
}

func writeApprovedReviewArtifacts(t *testing.T, changeRoot string) {
	t.Helper()
	repo := filepath.Dir(filepath.Dir(filepath.Dir(changeRoot)))
	runSDDStatusGit(t, repo, "init", "-q")
	runSDDStatusGit(t, repo, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, repo, "config", "user.name", "Status Test")
	runSDDStatusGit(t, repo, "add", ".")
	runSDDStatusGit(t, repo, "commit", "-qm", "base")
	tasksPath := filepath.Join(changeRoot, "tasks.md")
	tasks, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	write(t, tasksPath, string(tasks)+"\n# Reviewed candidate\n")

	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	reviewsDir := filepath.Join(changeRoot, "reviews")
	policyPath := filepath.Join(reviewsDir, "policy.md")
	ledgerPath := filepath.Join(reviewsDir, "ledger.json")
	verifyPath := filepath.Join(changeRoot, "verify-report.md")
	write(t, policyPath, "bounded archive policy\n")
	write(t, ledgerPath, reviewtransaction.CanonicalEmptyLedger)
	policyHash, err := reviewtransaction.HashArtifact(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	ledgerHash, err := reviewtransaction.HashArtifact(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	evidenceHash, err := reviewtransaction.HashArtifact(verifyPath)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: "thin-lineage", Mode: reviewtransaction.ModeOrdinary4R, Generation: 1,
		Snapshot: snapshot, PolicyHash: policyHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, "thin-lineage")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	revision, err := store.Append("", reviewtransaction.Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := reviewtransaction.CanonicalLedger([]reviewtransaction.Finding{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.FreezeFindings([]reviewtransaction.Finding{}, ledger, ledgerHash); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Append(revision, reviewtransaction.Record{Operation: "review/freeze-findings", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ClassifyEvidence([]reviewtransaction.FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Append(revision, reviewtransaction.Record{Operation: "review/classify", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Append(revision, reviewtransaction.Record{Operation: "review/begin-final-verification", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFinalVerification(evidenceHash, true); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Append(revision, reviewtransaction.Record{Operation: "review/complete-final-verification", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := store.ExportBundle()
	if err != nil {
		t.Fatal(err)
	}
	request := reviewtransaction.GateRequest{
		Schema: reviewtransaction.GateRequestSchema, Gate: reviewtransaction.GatePostApply,
		Target:          reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{}},
		StoreRevision:   revision,
		GenesisRevision: bundle.GenesisRevision, ChainIdentity: bundle.ChainIdentity, BundleDigest: bundle.BundleDigest,
		PolicyArtifact: policyPath, LedgerArtifact: ledgerPath, EvidenceArtifact: verifyPath,
	}
	writeJSON(t, filepath.Join(reviewsDir, "transaction.json"), tx)
	writeJSON(t, filepath.Join(reviewsDir, "receipt.json"), receipt)
	if err := reviewtransaction.WriteReceiptAtomic(filepath.Join(store.Dir, "artifacts", "receipt.json"), receipt); err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteChainBundleAtomic(filepath.Join(reviewsDir, "chain-bundle.json"), bundle); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(reviewsDir, "gate-context.json"), request)
}

func writeApprovedCompactAuthorityForChange(t *testing.T, repo, changeRoot, lineage string) {
	writeApprovedCompactAuthorityForChangeWithTasks(t, repo, changeRoot, lineage, "- [x] 1.1 Done\n# approved compact scope\n")
}

func writeApprovedCompactAuthorityForChangeWithTasks(t *testing.T, repo, changeRoot, lineage, tasks string) {
	t.Helper()
	runSDDStatusGit(t, repo, "init", "-q")
	runSDDStatusGit(t, repo, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, repo, "config", "user.name", "Status Test")
	runSDDStatusGit(t, repo, "add", ".")
	runSDDStatusGit(t, repo, "commit", "-qm", "base")
	write(t, filepath.Join(changeRoot, "tasks.md"), tasks)
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == reviewtransaction.RiskMedium {
		lenses = []string{reviewtransaction.LensReliability}
	} else if risk == reviewtransaction.RiskHigh {
		lenses = []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot, PolicyHash: shaID("c"), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]reviewtransaction.LensResult, len(lenses))
	for index, lens := range lenses {
		results[index] = reviewtransaction.LensResult{Lens: lens, Findings: []reviewtransaction.Finding{}, Evidence: []string{"review complete"}}
	}
	if err := state.CompleteReview(reviewtransaction.CompactReviewInput{LensResults: results, Classifications: []reviewtransaction.FindingEvidence{}, RefuterOutcomes: []reviewtransaction.EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
}

func writeAdditionalApprovedNativeReceipt(t *testing.T, repo, lineage string) {
	t.Helper()
	sourceStore, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, "thin-lineage")
	if err != nil {
		t.Fatal(err)
	}
	source, _, err := sourceStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: lineage, Mode: source.Transaction.Mode, Generation: source.Transaction.Generation,
		Snapshot: source.Transaction.Snapshot, PolicyHash: source.Transaction.PolicyHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	revision, err := store.Append("", reviewtransaction.Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := reviewtransaction.CanonicalLedger([]reviewtransaction.Finding{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.FreezeFindings([]reviewtransaction.Finding{}, ledger, source.Transaction.LedgerHash); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Append(revision, reviewtransaction.Record{Operation: "review/freeze-findings", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ClassifyEvidence([]reviewtransaction.FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Append(revision, reviewtransaction.Record{Operation: "review/classify", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Append(revision, reviewtransaction.Record{Operation: "review/begin-final-verification", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFinalVerification(source.Transaction.EvidenceHash, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(revision, reviewtransaction.Record{Operation: "review/complete-final-verification", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteReceiptAtomic(filepath.Join(store.Dir, "artifacts", "receipt.json"), receipt); err != nil {
		t.Fatal(err)
	}
}

func TestApplyReviewGateDiscoversCompactStateAndReceiptWithoutMirrors(t *testing.T) {
	repo := t.TempDir()
	runSDDStatusGit(t, repo, "init", "-q")
	runSDDStatusGit(t, repo, "config", "user.email", "test@example.com")
	runSDDStatusGit(t, repo, "config", "user.name", "Test")
	write(t, filepath.Join(repo, "tracked.txt"), "base\n")
	runSDDStatusGit(t, repo, "add", "tracked.txt")
	runSDDStatusGit(t, repo, "commit", "-qm", "base")
	write(t, filepath.Join(repo, "tracked.txt"), "candidate\n")
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == reviewtransaction.RiskMedium {
		lenses = []string{reviewtransaction.LensReliability}
	} else if risk == reviewtransaction.RiskHigh {
		lenses = []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: "compact-sdd", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: shaID("1"), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]reviewtransaction.LensResult, len(lenses))
	for index, lens := range lenses {
		results[index] = reviewtransaction.LensResult{Lens: lens, Findings: []reviewtransaction.Finding{}, Evidence: []string{"independent causal review completed"}}
	}
	if err := state.CompleteReview(reviewtransaction.CompactReviewInput{LensResults: results, Classifications: []reviewtransaction.FindingEvidence{}, RefuterOutcomes: []reviewtransaction.EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("independent SDD specification and runtime verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, _ := state.Receipt()
	if err := reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	status := Status{Dependencies: Dependencies{Verify: DependencyAllDone, Archive: DependencyReady}, TaskProgress: TaskProgress{AllComplete: true}}
	applyReviewGate(&status, repo, "", "")
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow || status.Dependencies.Archive != DependencyReady {
		t.Fatalf("compact SDD gate = %#v", status)
	}
}

func boundedVerifyEnvelope(revision, verdict string) string {
	return strings.Join([]string{
		"```yaml",
		"schema: gentle-ai.verify-result/v1",
		"evidence_revision: " + revision,
		"verdict: " + verdict,
		"blockers: 0",
		"critical_findings: 0",
		"requirements: 1/1",
		"scenarios: 1/1",
		"test_command: go test ./internal/example",
		"test_exit_code: 0",
		"test_output_hash: " + shaID("2"),
		"build_command: go test ./cmd/gentle-ai",
		"build_exit_code: 0",
		"build_output_hash: " + shaID("3"),
		"```",
	}, "\n")
}

func authorityOnlyVerifyEnvelope(revision, testExit, buildExit string) string {
	return strings.ReplaceAll(strings.Join([]string{
		"```yaml",
		"schema: gentle-ai.verify-result/v1",
		"evidence_revision: " + shaID("1"),
		"verdict: fail",
		"blockers: 1",
		"critical_findings: 1",
		"requirements: 0/1",
		"scenarios: 0/1",
		"test_command: go test ./...",
		"test_exit_code: " + testExit,
		"test_output_hash: " + emptyOutputHash,
		"build_command: go vet ./...",
		"build_exit_code: " + buildExit,
		"build_output_hash: " + emptyOutputHash,
		"authority_only_failure: true",
		"missing_review_authority: true",
		"substantive_failure: false",
		"command_failed: false",
		"observed_authority_revision: " + revision,
		"```",
	}, "\n"), "\r\n", "\n")
}

func remediationTransaction(t *testing.T, revision string, ready bool) reviewtransaction.Transaction {
	t.Helper()
	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: "thin-lineage", Mode: reviewtransaction.ModeOrdinary4R, Generation: 1,
		Snapshot: reviewtransaction.Snapshot{
			Kind: reviewtransaction.TargetCurrentChanges, BaseTree: strings.Repeat("a", 40), CandidateTree: strings.Repeat("b", 40),
			PathsDigest: shaID("4"), IntendedUntracked: []string{}, IntendedUntrackedProof: shaID("8"),
			Paths: []string{"internal/example.go"}, Identity: shaID("9"),
		},
		PolicyHash: shaID("5"),
	})
	if err != nil {
		t.Fatalf("NewTransaction() error = %v", err)
	}
	_ = tx.StartReview()
	_ = freezeStatusFindings(tx, []reviewtransaction.Finding{{ID: "R1-001", Severity: "CRITICAL"}})
	_, _ = tx.ClassifyEvidence([]reviewtransaction.FindingEvidence{{FindingID: "R1-001", Class: reviewtransaction.EvidenceDeterministic, Causality: reviewtransaction.CausalIntroduced, Proof: "failing focused test"}})
	if err := tx.BeginFix(revision); err != nil {
		t.Fatalf("BeginFix() error = %v", err)
	}
	if ready {
		fix := tx.Snapshot
		fix.Kind = reviewtransaction.TargetFixDiff
		fix.BaseTree = tx.FinalCandidateTree
		fix.CandidateTree = strings.Repeat("c", 40)
		fix.LedgerIDs = []string{"R1-001"}
		fix.Identity = shaID("a")
		if err := tx.CompleteFix(fix, shaID("b"), []string{"R1-001"}); err != nil {
			t.Fatalf("CompleteFix() error = %v", err)
		}
		if err := tx.ValidateFixDelta([]string{"R1-001"}, true); err != nil {
			t.Fatalf("ValidateFixDelta() error = %v", err)
		}
	}
	return *tx
}

func readJSON(t *testing.T, path string, destination any) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, destination); err != nil {
		t.Fatal(err)
	}
}

func runSDDStatusGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(%T): %v", value, err)
	}
	write(t, path, string(payload)+"\n")
}

func shaID(char string) string {
	return fmt.Sprintf("sha256:%s", strings.Repeat(char, 64))
}

func freezeStatusFindings(tx *reviewtransaction.Transaction, findings []reviewtransaction.Finding) error {
	ledger, err := reviewtransaction.CanonicalLedger(findings)
	if err != nil {
		return err
	}
	return tx.FreezeFindings(findings, ledger, "")
}
