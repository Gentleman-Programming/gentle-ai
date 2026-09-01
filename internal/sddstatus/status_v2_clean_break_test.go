package sddstatus

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

const freshV2RerunInstruction = "Start a fresh implementation state and rerun `gentle-ai sdd-status --contract gentle-ai.sdd-status/v2`."

func TestSDDStatusV2CleanBreak(t *testing.T) {
	t.Run("v2 is the sole default and v1 is refused read-only", func(t *testing.T) {
		if SchemaVersion != 2 {
			t.Fatalf("SchemaVersion = %d, want 2", SchemaVersion)
		}

		defaultArgs, err := ParseCommandArgs([]string{"thin"})
		if err != nil {
			t.Fatalf("ParseCommandArgs(default) error = %v", err)
		}
		if defaultArgs.Contract != "gentle-ai.sdd-status/v2" {
			t.Fatalf("default contract = %q, want gentle-ai.sdd-status/v2", defaultArgs.Contract)
		}

		v2Args, err := ParseCommandArgs([]string{"thin", "--contract", "gentle-ai.sdd-status/v2"})
		if err != nil {
			t.Fatalf("ParseCommandArgs(v2) error = %v", err)
		}
		if v2Args.Contract != "gentle-ai.sdd-status/v2" {
			t.Fatalf("v2 contract = %q, want gentle-ai.sdd-status/v2", v2Args.Contract)
		}

		_, err = ParseCommandArgs([]string{"thin", "--contract", "gentle-ai.sdd-status/v1"})
		if err == nil || err.Error() != "unsupported sdd-status contract \"gentle-ai.sdd-status/v1\". "+freshV2RerunInstruction {
			t.Fatalf("v1 refusal = %v, want one fresh-v2 rerun instruction", err)
		}
		afterRefusalDefaultArgs, err := ParseCommandArgs([]string{"thin"})
		if err != nil {
			t.Fatalf("ParseCommandArgs(default after v1 refusal) error = %v", err)
		}
		if !reflect.DeepEqual(afterRefusalDefaultArgs, defaultArgs) {
			t.Fatalf("v1 refusal changed default command parsing result: before=%#v after=%#v", defaultArgs, afterRefusalDefaultArgs)
		}
	})

	t.Run("tagged test corpus has no retired public v1 status pins", func(t *testing.T) {
		files, err := taggedStatusTestFiles()
		if err != nil {
			t.Fatalf("taggedStatusTestFiles() error = %v", err)
		}
		for _, file := range files {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", file, err)
			}
			if pins := retiredV1StatusTestPins(string(content)); len(pins) > 0 {
				t.Errorf("build-tagged test %s retains retired public v1 status pins: %s", file, strings.Join(pins, ", "))
			}
		}
	})

	t.Run("projection has the exact v2 authority-free key sets", func(t *testing.T) {
		status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)
		projected, err := ProjectStatusV2(status)
		if err != nil {
			t.Fatalf("v2 projection error = %v", err)
		}
		payload, err := json.Marshal(projected)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]json.RawMessage
		if err := json.Unmarshal(payload, &document); err != nil {
			t.Fatal(err)
		}
		assertExactJSONKeys(t, document, []string{
			"schemaName", "schemaVersion", "changeName", "artifactStore", "planningHome", "changeRoot",
			"artifactPaths", "contextFiles", "artifacts", "taskProgress", "dependencies", "applyState",
			"actionContext", "relationships", "remediationState", "nextRecommended", "blockedReasons",
		})
		assertJSONNestedKeys(t, document, "artifactPaths", []string{"proposal", "specs", "design", "tasks", "applyProgress", "verifyReport"})
		assertJSONNestedKeys(t, document, "contextFiles", []string{"proposal", "specs", "design", "tasks", "applyProgress", "verifyReport"})
		assertJSONNestedKeys(t, document, "artifacts", []string{"proposal", "specs", "design", "tasks", "applyProgress", "verifyReport"})
		assertJSONNestedKeys(t, document, "remediationState", []string{"required", "complete", "failedEvidenceRevision", "reason"})
		for _, forbidden := range []string{"reviewGate", "reviewTransaction", "reVerify", "runtimeStatus", "reviewPolicy", "reviewLedger", "reviewReceipt", "reviewBundle", "reviewContext", "reviewState", "lineageId", "generation", "fixBatch", "correctionBudget"} {
			if strings.Contains(string(payload), forbidden) {
				t.Fatalf("v2 projection retained authority key %q: %s", forbidden, payload)
			}
		}
	})

	t.Run("fresh enabled offer is lineage-free and disabled status omits it without changing archive", func(t *testing.T) {
		reviewEnabledHome(t)
		repo := initRuntimeLedgerRepo(t)
		changeRoot := seedReadyChange(t, repo, "thin", "- [x] 1.1 Work\n")
		write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0))
		write(t, filepath.Join(changeRoot, "reviews", "transaction.json"), "{\"retired\":true}\n")

		enabled, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "thin"})
		if err != nil {
			t.Fatal(err)
		}
		if enabled.Dependencies.Archive != DependencyReady || enabled.NextRecommended != "archive" {
			t.Fatalf("enabled archive = %q next = %q, want ready/archive", enabled.Dependencies.Archive, enabled.NextRecommended)
		}
		enabledProjection, err := ProjectStatusV2(enabled)
		if err != nil {
			t.Fatal(err)
		}
		enabledPayload, err := json.Marshal(enabledProjection)
		if err != nil {
			t.Fatal(err)
		}
		var enabledDocument map[string]json.RawMessage
		if err := json.Unmarshal(enabledPayload, &enabledDocument); err != nil {
			t.Fatal(err)
		}
		offerPayload, ok := enabledDocument["reviewOffer"]
		if !ok {
			t.Fatalf("enabled post-verify status omitted reviewOffer: %s", enabledPayload)
		}
		var offer struct {
			Available  bool   `json:"available"`
			Invocation string `json:"invocation"`
		}
		if err := json.Unmarshal(offerPayload, &offer); err != nil {
			t.Fatal(err)
		}
		var offerKeys map[string]json.RawMessage
		if err := json.Unmarshal(offerPayload, &offerKeys); err != nil {
			t.Fatal(err)
		}
		assertExactJSONKeys(t, offerKeys, []string{"available", "invocation"})
		if !offer.Available || offer.Invocation == "" {
			t.Fatalf("enabled v2 reviewOffer = %#v, want an available actionable offer", offer)
		}
		for _, forbidden := range []string{"reviewGate", "reviewTransaction", "reVerify", "runtimeStatus"} {
			if _, present := enabledDocument[forbidden]; present {
				t.Fatalf("enabled offer retained review authority key %q: %s", forbidden, enabledPayload)
			}
		}

		disabled, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "thin", ReviewDisabled: true})
		if err != nil {
			t.Fatal(err)
		}
		if disabled.Dependencies.Archive != DependencyReady || disabled.NextRecommended != "archive" {
			t.Fatalf("disabled archive = %q next = %q, want ready/archive", disabled.Dependencies.Archive, disabled.NextRecommended)
		}
		disabledProjection, err := ProjectStatusV2(disabled)
		if err != nil {
			t.Fatal(err)
		}
		disabledPayload, err := json.Marshal(disabledProjection)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(disabledPayload), "reviewOffer") {
			t.Fatalf("disabled status retained reviewOffer: %s", disabledPayload)
		}

		beforeRepeatedRead := snapshotStatusReadTree(t, repo)
		repeated, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "thin"})
		if err != nil {
			t.Fatal(err)
		}
		if repeated.ReviewOffer == nil || !repeated.ReviewOffer.Available {
			t.Fatalf("existing retired authority suppressed the fresh offer: %#v", repeated.ReviewOffer)
		}
		if repeated.Dependencies.Archive != DependencyReady || repeated.NextRecommended != "archive" {
			t.Fatalf("repeated enabled archive = %q next = %q, want ready/archive", repeated.Dependencies.Archive, repeated.NextRecommended)
		}
		if afterRepeatedRead := snapshotStatusReadTree(t, repo); afterRepeatedRead != beforeRepeatedRead {
			t.Fatal("reading a fresh offer persisted status state")
		}
	})

	t.Run("unfinished tasks block archive independently of review mode", func(t *testing.T) {
		repo := initRuntimeLedgerRepo(t)
		changeRoot := seedReadyChange(t, repo, "thin", "- [x] 1.1 Work\n- [ ] 1.2 Remaining\n")
		write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0))

		for _, reviewDisabled := range []bool{false, true} {
			status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "thin", ReviewDisabled: reviewDisabled})
			if err != nil {
				t.Fatal(err)
			}
			if status.Dependencies.Archive != DependencyBlocked || status.NextRecommended == "archive" {
				t.Fatalf("reviewDisabled=%t archive=%q next=%q, want unfinished tasks to block archive", reviewDisabled, status.Dependencies.Archive, status.NextRecommended)
			}
		}
	})

	t.Run("shipped assets and goldens no longer pin v1", func(t *testing.T) {
		contract := assets.MustRead("skills/_shared/sdd-status-contract.md")
		if strings.Contains(contract, "sdd-status/v1") || strings.Contains(contract, "Native status v1") {
			t.Fatalf("active status asset still pins v1")
		}
		if !strings.Contains(contract, "sdd-status/v2") {
			t.Fatal("active status asset does not advertise v2")
		}
		if golden := mustReadStatusGolden(t, "sdd-claude-cmd-gentle-sdd-status.golden"); strings.Contains(golden, "sdd-status/v1") || strings.Contains(golden, "Native status v1") {
			t.Fatal("generated status golden still pins v1")
		}
	})
}

func TestResolveRuntimeAuthorityFailureBlocksFinalRoutingBeforeOfferingReview(t *testing.T) {
	for _, store := range []ArtifactStore{ArtifactStoreOpenSpec, ArtifactStoreEngram} {
		t.Run(string(store), func(t *testing.T) {
			repo := initRuntimeLedgerRepo(t)
			const change = "runtime-routing"
			if store == ArtifactStoreOpenSpec {
				changeRoot := seedReadyChange(t, repo, change, "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0))
			} else {
				if err := os.MkdirAll(filepath.Join(repo, ".engram"), 0o755); err != nil {
					t.Fatal(err)
				}
				runRuntimeLedgerGit(t, repo, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")
				restore := stubEngramExport(t, []engramObservation{
					{Title: "sdd/" + change + "/proposal", Content: "# Proposal\n", Project: "gentle-ai", Scope: "project"},
					{Title: "sdd/" + change + "/spec", Content: "### Requirement: Runtime\n#### Scenario: Routing\n", Project: "gentle-ai", Scope: "project"},
					{Title: "sdd/" + change + "/design", Content: "# Design\n", Project: "gentle-ai", Scope: "project"},
					{Title: "sdd/" + change + "/tasks", Content: "- [x] 1.1 Work\n", Project: "gentle-ai", Scope: "project"},
					{Title: "sdd/" + change + "/verify-report", Content: testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0), Project: "gentle-ai", Scope: "project"},
				})
				t.Cleanup(restore)
			}

			runtimeStore := mustRuntimeStore(t, repo, change)
			if _, err := runtimeStore.Begin(context.Background(), BeginAttemptRequest{
				RequestID: "begin-corrupt-runtime", WorkUnit: "verify", EvidenceGoal: "prove final routing",
				MaxAttempts: 1, MaxChangedLines: DefaultRuntimeChangedLines,
			}); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(runtimeStore.Dir, "HEAD"), []byte("corrupt\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: change})
			if err != nil {
				t.Fatal(err)
			}
			if status.Dependencies.Verify != DependencyBlocked || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "resolve-blockers" {
				t.Fatalf("runtime authority failure routing = verify %q archive %q next %q, want blocked/blocked/resolve-blockers", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended)
			}
			if status.ReviewOffer != nil {
				t.Fatalf("runtime authority failure published reviewOffer after blocking final routing: %#v", status.ReviewOffer)
			}
		})
	}
}

func TestResolveBoundedRemediationCompletesAuthorityFreeEvidence(t *testing.T) {
	failedEvidenceRevision := "sha256:" + strings.Repeat("d", 64)
	remediation := resolveBoundedRemediation(true, verifyResultEvaluation{
		EvidenceRevision: failedEvidenceRevision,
		Reason:           "verification failed",
	}, remediationResultEvidence(failedEvidenceRevision))
	if !remediation.Complete || remediation.Required || remediation.Reason != "" {
		t.Fatalf("authority-free remediation = %#v, want completed evidence", remediation)
	}
}

func taggedStatusTestFiles() ([]string, error) {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		return nil, err
	}
	var tagged []string
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if hasGoBuildConstraint(string(content)) {
			tagged = append(tagged, file)
		}
	}
	sort.Strings(tagged)
	return tagged, nil
}

func hasGoBuildConstraint(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "//go:build "), strings.HasPrefix(line, "// +build "):
			return true
		case strings.HasPrefix(line, "//"):
			continue
		default:
			return false
		}
	}
	return false
}

func retiredV1StatusTestPins(content string) []string {
	pins := make([]string, 0, 3)
	if strings.Contains(content, "ProjectStatusV1") {
		pins = append(pins, "ProjectStatusV1")
		if strings.Contains(content, "ReviewGate") {
			pins = append(pins, "ReviewGate status projection")
		}
	}
	if strings.Contains(content, "gentle-ai.sdd-status/v1") {
		pins = append(pins, "contract v1")
	}
	return pins
}

func snapshotStatusReadTree(t *testing.T, root string) string {
	t.Helper()
	var entries []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if entry.IsDir() {
			entries = append(entries, "dir:"+relative)
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(content)
		entries = append(entries, fmt.Sprintf("file:%s:%x", relative, digest))
		return nil
	}); err != nil {
		t.Fatalf("snapshotStatusReadTree(%s): %v", root, err)
	}
	sort.Strings(entries)
	return strings.Join(entries, "\n")
}

func assertExactJSONKeys(t *testing.T, document map[string]json.RawMessage, expected []string) {
	t.Helper()
	want := make(map[string]struct{}, len(expected))
	for _, key := range expected {
		want[key] = struct{}{}
	}
	for key := range document {
		if _, ok := want[key]; !ok {
			t.Fatalf("unexpected JSON key %q", key)
		}
		delete(want, key)
	}
	for key := range want {
		t.Fatalf("missing JSON key %q", key)
	}
}

func assertJSONNestedKeys(t *testing.T, document map[string]json.RawMessage, key string, expected []string) {
	t.Helper()
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(document[key], &nested); err != nil {
		t.Fatalf("decode %s: %v", key, err)
	}
	assertExactJSONKeys(t, nested, expected)
}

func mustReadStatusGolden(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
