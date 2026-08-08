package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestNegotiatedReviewStartContextIsFrozenWhileLegacyBytesStayPrivate(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "private tracked candidate\n", 0o644)
	writeReviewStartCandidate(t, repo, "private.txt", "private intended candidate\n", 0o644)
	lineage := "review-start-private-context"

	var legacyOutput bytes.Buffer
	if err := RunReview([]string{"start", "--cwd", repo, "--lineage", lineage}, &legacyOutput); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"candidate_diff", "changed_path_manifest", "private tracked candidate", "private intended candidate"} {
		if bytes.Contains(legacyOutput.Bytes(), []byte(forbidden)) {
			t.Fatalf("unnegotiated START leaked %q:\n%s", forbidden, legacyOutput.String())
		}
	}
	var legacy ReviewFacadeStartResult
	legacyDecoder := json.NewDecoder(bytes.NewReader(legacyOutput.Bytes()))
	legacyDecoder.DisallowUnknownFields()
	if err := legacyDecoder.Decode(&legacy); err != nil {
		t.Fatal(err)
	}
	var exactLegacy bytes.Buffer
	if err := encodeReviewJSON(&exactLegacy, legacy); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacyOutput.Bytes(), exactLegacy.Bytes()) {
		t.Fatalf("unnegotiated START bytes changed:\ngot=%s\nwant=%s", legacyOutput.String(), exactLegacy.String())
	}

	resumed := runNegotiatedReviewStart(t, repo, lineage)
	if resumed.Action != string(reviewtransaction.CompactStartResumed) {
		t.Fatalf("negotiated replay action = %q, want resumed", resumed.Action)
	}
	assertNegotiatedStartFrozenContext(t, repo, resumed)
	frozenBase, frozenCandidate := resumed.BaseTree, resumed.CandidateTree
	frozenManifest := append([]reviewtransaction.ChangedPathManifestEntry(nil), (*resumed.ChangedPathManifest)...)
	encoded, err := json.Marshal(resumed)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("private tracked candidate")) || bytes.Contains(encoded, []byte("private intended candidate")) {
		t.Fatalf("negotiated START leaked candidate bytes: %s", encoded)
	}
	// #2394: private.txt reaches the candidate through the index it was
	// declared in, so no path carries intended-untracked provenance any more.
	if len(frozenManifest) != 2 || frozenManifest[0].Path != "private.txt" || frozenManifest[0].IntendedUntracked ||
		frozenManifest[1].Path != "tracked.txt" || frozenManifest[1].IntendedUntracked {
		t.Fatalf("negotiated manifest = %#v", frozenManifest)
	}

	writeReviewStartCandidate(t, repo, "tracked.txt", "mutated live workspace\n", 0o644)
	writeReviewStartCandidate(t, repo, "private.txt", "mutated private workspace\n", 0o644)
	hostileAttributes := filepath.Join(t.TempDir(), "hostile-attributes")
	if err := os.WriteFile(hostileAttributes, []byte("*.txt binary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hostileGlobalConfig := filepath.Join(t.TempDir(), "global.gitconfig")
	if output, err := exec.Command("git", "config", "--file", hostileGlobalConfig, "core.attributesFile", hostileAttributes).CombinedOutput(); err != nil {
		t.Fatalf("write hostile global Git config: %v\n%s", err, output)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", hostileGlobalConfig)
	runReviewCLIGit(t, repo, "config", "core.attributesFile", hostileAttributes)
	if err := os.WriteFile(filepath.Join(repo, ".git", "info", "attributes"), []byte("*.txt binary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocked := runNegotiatedReviewStart(t, repo, lineage)
	if blocked.Action != string(reviewtransaction.CompactStartBlocked) || blocked.BaseTree != frozenBase || blocked.CandidateTree != frozenCandidate ||
		!reflect.DeepEqual(*blocked.ChangedPathManifest, frozenManifest) {
		t.Fatalf("blocked START did not retain frozen context: %#v", blocked)
	}
}

func TestNegotiatedReviewStartContextCoversCreatedReuseAndRecovery(t *testing.T) {
	t.Run("created and receipt replay", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
		lineage := "review-start-context-reuse"
		created := runNegotiatedReviewStart(t, repo, lineage)
		if created.Action != string(reviewtransaction.CompactStartCreated) {
			t.Fatalf("created action = %q", created.Action)
		}
		assertNegotiatedStartFrozenContext(t, repo, created)
		completeNegotiatedStartReview(t, repo, created, true)

		reused := runNegotiatedReviewStart(t, repo, lineage)
		if reused.Action != string(reviewtransaction.CompactStartReuseReceipt) || reused.BaseTree != created.BaseTree || reused.CandidateTree != created.CandidateTree ||
			!reflect.DeepEqual(*reused.ChangedPathManifest, *created.ChangedPathManifest) {
			t.Fatalf("receipt replay START = %#v", reused)
		}
	})

	t.Run("recovery selection", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "tracked.txt", "candidate before escalation\n", 0o644)
		lineage := "review-start-context-recovery"
		created := runNegotiatedReviewStart(t, repo, lineage)
		completeNegotiatedStartReview(t, repo, created, false)

		writeReviewStartCandidate(t, repo, "tracked.txt", "replacement target after escalation\n", 0o644)
		recovery := runNegotiatedReviewStart(t, repo, lineage)
		if recovery.Action != string(reviewtransaction.CompactStartBlocked) || recovery.LineageID != lineage ||
			recovery.BaseTree != created.BaseTree || recovery.CandidateTree != created.CandidateTree || !reflect.DeepEqual(*recovery.ChangedPathManifest, *created.ChangedPathManifest) {
			t.Fatalf("recovery START = %#v", recovery)
		}
	})
}

func TestNegotiatedReviewStartLargeRepositoryUsesBoundedReferenceAdmission(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("uses real git commands and a repository tree larger than 4 MiB")
	}
	repo := initReviewCLIRepo(t)
	baseCommit, supportingPath := installLargeReviewRepositoryHistory(t, repo)
	lineage := "review-start-large-repository"
	args := boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", lineage, "--base-ref", baseCommit,
	})
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("START with a repository path inventory larger than 4 MiB: %v", err)
	}
	started := decodeNegotiatedReviewStart(t, output.Bytes())
	if started.Action != string(reviewtransaction.CompactStartCreated) || started.ChangedPathManifest == nil ||
		len(*started.ChangedPathManifest) != 1 || (*started.ChangedPathManifest)[0].Path != "main.go" {
		t.Fatalf("large-repository START = %#v", started)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(record.State.SelectedLenses) == 0 {
		t.Fatalf("large-repository START selected no reviewer lenses: %#v", record.State)
	}
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	input := filepath.Join(t.TempDir(), "reviewer.json")
	captureArgs := []string{
		"--cwd", repo, "--lineage", lineage, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}
	result.Evidence = []string{"fabricated proof: missing-inventory.go:1"}
	writeReviewCLIJSON(t, input, result)
	if err := RunReviewCaptureResult(captureArgs, io.Discard); err == nil || !strings.Contains(err.Error(), "outside the frozen repository") {
		t.Fatalf("invalid immutable-tree reference admission error = %v", err)
	}
	result.Evidence = []string{"supporting proof: " + supportingPath + ":1"}
	writeReviewCLIJSON(t, input, result)
	if err := RunReviewCaptureResult(captureArgs, io.Discard); err != nil {
		t.Fatalf("valid immutable-tree reference was rejected: %v", err)
	}
}

func TestNegotiatedReviewStartContextValidationDistinguishesMissingAndEmpty(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	writeReviewStartCandidate(t, repo, "z.txt", "second candidate\n", 0o644)
	valid := runNegotiatedReviewStart(t, repo, "review-start-context-validation")

	for _, test := range []struct {
		name   string
		mutate func(*ReviewIntegrationStartResult)
	}{
		{name: "missing artifact subjects", mutate: func(result *ReviewIntegrationStartResult) { result.ArtifactSubjects = nil }},
		{name: "artifact subject mismatch", mutate: func(result *ReviewIntegrationStartResult) {
			subjects := append([]reviewtransaction.ArtifactSubject(nil), result.ArtifactSubjects...)
			subjects[0].SubjectHash = "sha256:" + strings.Repeat("0", 64)
			result.ArtifactSubjects = subjects
		}},
		{name: "missing base tree", mutate: func(result *ReviewIntegrationStartResult) { result.BaseTree = "" }},
		{name: "missing candidate tree", mutate: func(result *ReviewIntegrationStartResult) { result.CandidateTree = "" }},
		{name: "missing manifest", mutate: func(result *ReviewIntegrationStartResult) { result.ChangedPathManifest = nil }},
		{name: "manifest count mismatch", mutate: func(result *ReviewIntegrationStartResult) {
			empty := []reviewtransaction.ChangedPathManifestEntry{}
			result.ChangedPathManifest = &empty
		}},
		{name: "noncanonical manifest", mutate: func(result *ReviewIntegrationStartResult) {
			manifest := append([]reviewtransaction.ChangedPathManifestEntry(nil), (*result.ChangedPathManifest)...)
			manifest[0].Path = "../outside.txt"
			result.ChangedPathManifest = &manifest
		}},
		{name: "manifest order mismatch", mutate: func(result *ReviewIntegrationStartResult) {
			manifest := append([]reviewtransaction.ChangedPathManifestEntry(nil), (*result.ChangedPathManifest)...)
			manifest[0], manifest[1] = manifest[1], manifest[0]
			result.ChangedPathManifest = &manifest
		}},
		{name: "manifest status flags mismatch", mutate: func(result *ReviewIntegrationStartResult) {
			manifest := append([]reviewtransaction.ChangedPathManifestEntry(nil), (*result.ChangedPathManifest)...)
			manifest[0].Deleted = true
			result.ChangedPathManifest = &manifest
		}},
		{name: "manifest mode mismatch", mutate: func(result *ReviewIntegrationStartResult) {
			manifest := append([]reviewtransaction.ChangedPathManifestEntry(nil), (*result.ChangedPathManifest)...)
			manifest[0].OldMode = "invalid"
			result.ChangedPathManifest = &manifest
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := valid
			test.mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatalf("Validate() accepted invalid context: %#v", invalid)
			}
		})
	}

	emptyManifest := []reviewtransaction.ChangedPathManifestEntry{}
	valid.ChangedPathManifest = &emptyManifest
	valid.LensesRequired = false
	valid.RiskLevel = reviewtransaction.RiskLow
	valid.SelectedLenses = []string{}
	valid.ArtifactSubjects = []reviewtransaction.ArtifactSubject{}
	valid.ChangedFiles = 0
	valid.ChangedLines = 0
	valid.CorrectionBudget = 0
	valid.RiskReasons = []reviewtransaction.RiskReason{{Code: reviewtransaction.RiskReasonNonExecutableOnly}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() rejected present empty context: %v", err)
	}
}

func TestNegotiatedReviewStartContextFailureReportsTruthfulAuthorityProvenance(t *testing.T) {
	t.Run("new authority remains uncreated", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
		lineage := "review-start-context-pre-native"
		restore := forceReviewStartContextFailure(errors.New("forced pre-native context failure"))
		t.Cleanup(restore)

		var output bytes.Buffer
		if err := RunReview(boundNegotiatedStartArgs(t, []string{"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage}), &output); err == nil {
			t.Fatal("negotiated START unexpectedly succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		if failure.Phase != "pre_native" || failure.MutationOutcome != ReviewMutationNotStarted || failure.RetrySafe ||
			failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired || failure.NextAction != "stop" || failure.LineageID != lineage {
			t.Fatalf("pre-native context failure = %#v", failure)
		}
		stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		if len(stores) != 0 {
			t.Fatalf("context failure persisted authority stores: %#v", stores)
		}
	})

	t.Run("existing authority remains selected and unchanged", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
		lineage := "review-start-context-existing"
		if err := RunReview([]string{"start", "--cwd", repo, "--lineage", lineage}, io.Discard); err != nil {
			t.Fatal(err)
		}
		store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
		if err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(store.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		restore := forceReviewStartContextFailure(errors.New("forced existing-authority context failure"))
		t.Cleanup(restore)

		var output bytes.Buffer
		if err := RunReview(boundNegotiatedStartArgs(t, []string{"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage}), &output); err == nil {
			t.Fatal("negotiated START unexpectedly succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		if failure.Phase != "native_committed" || failure.MutationOutcome != ReviewMutationUnknown || failure.RetrySafe ||
			failure.AuthorityApplicability != "current_target" || failure.Replayability != reviewtransaction.ReplayabilityStatusRequired ||
			failure.NextAction != "review.status" || !reflect.DeepEqual(failure.RequiredInputs, []string{"lineage_id"}) || failure.LineageID != lineage {
			t.Fatalf("existing-authority context failure = %#v", failure)
		}
		after, err := os.ReadFile(store.StatePath())
		if err != nil || !bytes.Equal(after, before) {
			t.Fatalf("existing authority changed after context failure: %v", err)
		}
	})
}

func assertNegotiatedStartFrozenContext(t *testing.T, repo string, result ReviewIntegrationStartResult) {
	t.Helper()
	if result.BaseTree == "" || result.CandidateTree == "" || result.ChangedPathManifest == nil {
		t.Fatalf("negotiated START context is missing: %#v", result)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, result.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	want, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), record.State.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if result.BaseTree != want.BaseTree || result.CandidateTree != want.CandidateTree || !reflect.DeepEqual(*result.ChangedPathManifest, want.ChangedPathManifest) {
		t.Fatalf("START context does not match frozen authority:\ngot=%#v\nwant=%#v", result, want)
	}
}

func completeNegotiatedStartReview(t *testing.T, repo string, started ReviewIntegrationStartResult, approved bool) {
	t.Helper()
	resultPath := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Lens: started.SelectedLenses[0], Findings: []facadeFinding{}, Evidence: []string{"reviewed frozen candidate context"},
	})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if approved {
		evidence := filepath.Join(t.TempDir(), "evidence.txt")
		if err := os.WriteFile(evidence, []byte("focused verification passed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--evidence", evidence}, io.Discard); err != nil {
			t.Fatal(err)
		}
		return
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	next := record.State
	if err := next.CompleteVerification([]byte("verification failed"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(record.Revision, "review/complete-verification", next); err != nil {
		t.Fatal(err)
	}
}

func forceReviewStartContextFailure(forced error) func() {
	original := renderReviewStartFrozenCandidateContext
	renderReviewStartFrozenCandidateContext = func(context.Context, reviewtransaction.SnapshotBuilder, reviewtransaction.Snapshot) (reviewtransaction.FrozenCandidateContext, error) {
		return reviewtransaction.FrozenCandidateContext{}, forced
	}
	return func() { renderReviewStartFrozenCandidateContext = original }
}

func installLargeReviewRepositoryHistory(t *testing.T, repo string) (string, string) {
	t.Helper()
	sharedBlob := strings.TrimSpace(runReviewCLIGitInput(t, repo, []byte("package inventory\n"), "hash-object", "-w", "--stdin"))
	baseBlob := strings.TrimSpace(runReviewCLIGitInput(t, repo, []byte("package main\n\nfunc value() int { return 1 }\n"), "hash-object", "-w", "--stdin"))
	candidateBlob := strings.TrimSpace(runReviewCLIGitInput(t, repo, []byte("package main\n\nfunc value() int { return 2 }\n"), "hash-object", "-w", "--stdin"))
	const inventoryEntries = 24_000
	padding := strings.Repeat("x", 160)
	paths := make([]string, inventoryEntries)
	var baseTreeInput bytes.Buffer
	var candidateTreeInput bytes.Buffer
	for index := range paths {
		paths[index] = fmt.Sprintf("inventory-%05d-%s.go", index, padding)
		for _, tree := range []*bytes.Buffer{&baseTreeInput, &candidateTreeInput} {
			fmt.Fprintf(tree, "100644 blob %s\t%s\x00", sharedBlob, paths[index])
		}
	}
	fmt.Fprintf(&baseTreeInput, "100644 blob %s\tmain.go\x00", baseBlob)
	fmt.Fprintf(&candidateTreeInput, "100644 blob %s\tmain.go\x00", candidateBlob)
	baseTree := strings.TrimSpace(runReviewCLIGitInput(t, repo, baseTreeInput.Bytes(), "mktree", "-z"))
	candidateTree := strings.TrimSpace(runReviewCLIGitInput(t, repo, candidateTreeInput.Bytes(), "mktree", "-z"))
	baseCommit := strings.TrimSpace(runReviewCLIGit(t, repo, "commit-tree", baseTree, "-m", "large base tree"))
	candidateCommit := strings.TrimSpace(runReviewCLIGit(t, repo, "commit-tree", candidateTree, "-p", baseCommit, "-m", "tiny candidate"))
	runReviewCLIGit(t, repo, "update-ref", "HEAD", candidateCommit)
	inventory := runReviewCLIGit(t, repo, "ls-tree", "-r", "-z", "--name-only", "--full-tree", baseTree)
	if len(inventory) <= 4<<20 {
		t.Fatalf("repository path inventory = %d bytes, want more than 4 MiB", len(inventory))
	}
	return baseCommit, paths[0]
}

func runReviewCLIGitInput(t *testing.T, repo string, input []byte, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}
