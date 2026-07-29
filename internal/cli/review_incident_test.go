package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestValidResultIncidentClass(t *testing.T) {
	cases := map[reviewtransaction.ResultIncidentClass]bool{
		reviewtransaction.ResultIncidentEmptyResult:    true,
		reviewtransaction.ResultIncidentNestedEnvelope: true,
		"":                          true,
		"wrong_target":              false,
		"transport_syntax":          false,
		"../evil":                   false,
		"arbitrary string not enum": false,
	}
	for class, want := range cases {
		if got := reviewtransaction.ValidResultIncidentClass(class); got != want {
			t.Fatalf("ValidResultIncidentClass(%q) = %v, want %v", class, got, want)
		}
	}
}

func TestReviewCaptureResultPreflightVerifiesBindingWithoutMutation(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, false)
	bindingArgs := func(lens string, order string) []string {
		return []string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
			"--lens", lens, "--order", order, "--preflight",
		}
	}
	var output bytes.Buffer
	if err := RunReviewCaptureResult(bindingArgs(record.State.SelectedLenses[0], "0"), &output); err != nil {
		t.Fatalf("valid preflight failed: %v", err)
	}
	var preflight reviewCapturePreflightResult
	decodeStrictReviewJSON(t, output.Bytes(), &preflight)
	if preflight.Schema != reviewCapturePreflightSchema || preflight.Capability != reviewCapturePreflightCapability ||
		preflight.RepositoryRoot == "" ||
		preflight.LineageID != started.LineageID || preflight.TargetIdentity != record.State.InitialSnapshot.Identity ||
		preflight.Lens != record.State.SelectedLenses[0] || preflight.SelectedOrder != 0 ||
		preflight.ArtifactSubject.SubjectHash == "" || preflight.ArtifactSubject.AuthorityRevision != record.Revision ||
		preflight.ArtifactSubject.Lens != preflight.Lens || preflight.ArtifactSubject.SelectedOrder != preflight.SelectedOrder {
		t.Fatalf("preflight result = %+v", preflight)
	}
	if err := reviewtransaction.ValidateArtifactSubject(preflight.ArtifactSubject); err != nil {
		t.Fatalf("preflight artifact subject = %#v: %v", preflight.ArtifactSubject, err)
	}
	wantContext, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), record.State.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.BaseTree != wantContext.BaseTree || preflight.CandidateTree != wantContext.CandidateTree ||
		!reflect.DeepEqual(preflight.ChangedPathManifest, wantContext.ChangedPathManifest) {
		t.Fatalf("preflight frozen context differs from authority\ngot=%s..%s %#v\nwant=%s..%s %#v", preflight.BaseTree, preflight.CandidateTree, preflight.ChangedPathManifest, wantContext.BaseTree, wantContext.CandidateTree, wantContext.ChangedPathManifest)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, reviewtransaction.CompactReviewerResultsDir)); !os.IsNotExist(err) {
		t.Fatal("preflight persisted a reviewer result artifact")
	}
	after, err := store.Load()
	if err != nil || after.Revision != record.Revision {
		t.Fatalf("preflight mutated review authority: %v", err)
	}
	if err := RunReviewCaptureResult(append(bindingArgs(record.State.SelectedLenses[0], "0"), "--input", "-"), io.Discard); err == nil {
		t.Fatal("preflight combined with --input was accepted")
	}
	if err := RunReviewCaptureResult(bindingArgs("review-risk", "0"), io.Discard); err == nil {
		t.Fatal("wrong-lens preflight was accepted")
	}
}

func TestReviewCaptureResultNestedRepositoryFailsActionablyAndStaysRetriable(t *testing.T) {
	parent, child := initNestedReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(child, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, child)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), child, started.LineageID)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, child, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
		t.Fatal(err)
	}
	args := func(cwd string, rest ...string) []string {
		return append([]string{
			"--cwd", cwd, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
			"--lens", record.State.SelectedLenses[0], "--order", "0",
		}, rest...)
	}
	parentRoot, err := (reviewtransaction.SnapshotBuilder{Repo: parent}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range [][]string{{"--input", input}, {"--preflight"}} {
		err := RunReviewCaptureResult(args(parent, mode...), io.Discard)
		if err == nil {
			t.Fatalf("nested-repo capture %v from parent repository was accepted", mode)
		}
		if !strings.Contains(err.Error(), parentRoot) || !strings.Contains(err.Error(), "--cwd") {
			t.Fatalf("nested-repo capture error is not actionable: %v", err)
		}
	}
	// The failed parent-repository capture must not consume the exactly-once
	// native lens slot: the same capture succeeds from the reviewing repository.
	var output bytes.Buffer
	if err := RunReviewCaptureResult(args(child, "--input", input), &output); err != nil {
		t.Fatalf("retry from reviewing repository failed: %v", err)
	}
	manifest := strings.TrimSpace(output.String())
	if err := RunReviewFacadeFinalize([]string{"--cwd", child, "--lineage", started.LineageID, "--result-artifact", manifest}, io.Discard); err != nil {
		t.Fatalf("finalize after recovered capture failed: %v", err)
	}
}

func TestReviewPreserveResultDurableIncidentArtifact(t *testing.T) {
	parent, child := initNestedReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(child, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, child)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), child, started.LineageID)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	raw := "raw reviewer output\nthat is not JSON"
	input := filepath.Join(t.TempDir(), "raw.txt")
	if err := os.WriteFile(input, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	wrongRepositoryArgs := []string{
		"--cwd", parent, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}
	if err := RunReviewPreserveResult(wrongRepositoryArgs, io.Discard); err == nil {
		t.Fatal("preserve-result accepted a repository without the live reviewing authority")
	}
	args := append([]string{}, wrongRepositoryArgs...)
	args[1] = child
	var first, replay bytes.Buffer
	if err := RunReviewPreserveResult(args, &first); err != nil {
		t.Fatalf("preserve under the reviewing repository failed: %v", err)
	}
	var artifact reviewIncidentArtifact
	decodeStrictReviewJSON(t, first.Bytes(), &artifact)
	if artifact.Schema != reviewIncidentArtifactSchema || artifact.Capability != reviewIncidentArtifactCapability ||
		artifact.LineageID != started.LineageID || artifact.TargetIdentity != record.State.InitialSnapshot.Identity ||
		artifact.Lens != record.State.SelectedLenses[0] || artifact.SelectedOrder != 0 {
		t.Fatalf("incident artifact = %+v", artifact)
	}
	if !strings.Contains(artifact.Path, filepath.Join("gentle-ai", "review-transactions", "incidents", started.LineageID)) {
		t.Fatalf("incident artifact path %q is outside the durable incidents area", artifact.Path)
	}
	preserved, err := os.ReadFile(artifact.Path)
	if err != nil || string(preserved) != raw {
		t.Fatalf("preserved bytes mismatch: %v %q", err, preserved)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Lstat(artifact.Path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("incident artifact is not owner-only: %v %v", err, info.Mode())
		}
	}
	if err := RunReviewPreserveResult(args, &replay); err != nil || first.String() != replay.String() {
		t.Fatalf("preserve replay changed: %v", err)
	}
	// A different raw result for the same slot is preserved separately instead
	// of overwriting the first incident.
	if err := os.WriteFile(input, []byte("second distinct raw output"), 0o600); err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	if err := RunReviewPreserveResult(args, &second); err != nil {
		t.Fatal(err)
	}
	var other reviewIncidentArtifact
	decodeStrictReviewJSON(t, second.Bytes(), &other)
	if other.Path == artifact.Path || other.SHA256 == artifact.SHA256 {
		t.Fatal("distinct raw result reused the first incident artifact")
	}
	// An incident artifact is never a captured lens result: finalize must
	// reject its manifest.
	if err := RunReviewFacadeFinalize([]string{"--cwd", child, "--lineage", started.LineageID, "--result-artifact", strings.TrimSpace(first.String())}, io.Discard); err == nil {
		t.Fatal("finalize accepted an incident artifact as a captured lens result")
	}
}

// TestReviewPreservedResultReplaysThroughCaptureAndFinalize pins the recovery
// contract documented on RunReviewPreserveResult: a preserved incident payload
// must replay through `review capture-result --input <preserved path>` and
// finalize. The plugin must therefore preserve the EXTRACTED strict reviewer
// JSON on capture failure — a preserved raw task envelope (what the plugin
// held in output.output before extraction) is rejected by the strict replay
// decoder and is not a recoverable artifact.
func TestReviewPreservedResultReplaysThroughCaptureAndFinalize(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	extracted := string(admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0))
	envelope := "<task id=\"lens-1\" state=\"completed\">\n<task_result>\n" + extracted + "\n</task_result>\n</task>"
	preserve := func(raw string) string {
		t.Helper()
		input := filepath.Join(t.TempDir(), "raw.txt")
		if err := os.WriteFile(input, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if err := RunReviewPreserveResult([]string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
			"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
		}, &output); err != nil {
			t.Fatalf("preserve failed: %v", err)
		}
		var artifact reviewIncidentArtifact
		decodeStrictReviewJSON(t, output.Bytes(), &artifact)
		return artifact.Path
	}
	captureArgs := func(preserved string) []string {
		return []string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
			"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", preserved,
		}
	}
	// An envelope-wrapped preserved payload is unrecoverable: strict replay
	// decoding rejects it, and the failed replay must not consume the slot.
	if err := RunReviewCaptureResult(captureArgs(preserve(envelope)), io.Discard); err == nil {
		t.Fatal("envelope-wrapped preserved payload replayed through capture-result")
	}
	// The extracted strict JSON — what the plugin preserves on capture
	// failure — replays and finalizes.
	var output bytes.Buffer
	if err := RunReviewCaptureResult(captureArgs(preserve(extracted)), &output); err != nil {
		t.Fatalf("extracted preserved payload failed replay: %v", err)
	}
	manifest := strings.TrimSpace(output.String())
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", manifest}, io.Discard); err != nil {
		t.Fatalf("finalize after preserved-result replay failed: %v", err)
	}
}

func TestReviewPreserveResultRejectsUnsafeBindings(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	input := filepath.Join(t.TempDir(), "raw.txt")
	if err := os.WriteFile(input, []byte("raw output"), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := map[string]string{
		"cwd": repo, "lineage": started.LineageID, "target": record.State.InitialSnapshot.Identity,
		"lens": record.State.SelectedLenses[0], "order": "0", "input": input,
	}
	cases := map[string]map[string]string{
		"lineage":     {"lineage": "Not_A/Lineage"},
		"target":      {"target": "sha256:xyz"},
		"lens":        {"lens": "risk"},
		"order":       {"order": "9"},
		"empty input": {"input": filepath.Join(t.TempDir(), "missing.txt")},
	}
	for name, override := range cases {
		t.Run(name, func(t *testing.T) {
			args := []string{}
			for _, key := range []string{"cwd", "lineage", "target", "lens", "order", "input"} {
				value := valid[key]
				if replacement, ok := override[key]; ok {
					value = replacement
				}
				args = append(args, "--"+key, value)
			}
			if err := RunReviewPreserveResult(args, io.Discard); err == nil {
				t.Fatalf("unsafe preserve binding %q was accepted", name)
			}
		})
	}
}

func TestReviewPreserveResultRejectsUnsafeClass(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	input := filepath.Join(t.TempDir(), "raw.txt")
	if err := os.WriteFile(input, []byte("raw output"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, err := reviewtransaction.CompactIncidentsDir(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{"../evil", "transport_syntax", "../../etc/passwd"}
	for _, class := range cases {
		t.Run(class, func(t *testing.T) {
			before, _ := os.ReadDir(dir)
			args := []string{
				"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
				"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input, "--class", class,
			}
			if err := RunReviewPreserveResult(args, io.Discard); err == nil {
				t.Fatalf("unsafe --class %q was accepted", class)
			}
			after, _ := os.ReadDir(dir)
			if len(after) != len(before) {
				t.Fatalf("unsafe --class %q wrote a file under the incidents dir: before=%d after=%d", class, len(before), len(after))
			}
		})
	}
}

func TestReviewPreserveResultRecordsIncidentClass(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	preserve := func(t *testing.T, class string) reviewIncidentArtifact {
		t.Helper()
		input := filepath.Join(t.TempDir(), "raw.txt")
		if err := os.WriteFile(input, []byte("raw output for "+class), 0o600); err != nil {
			t.Fatal(err)
		}
		args := []string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
			"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
		}
		if class != "" {
			args = append(args, "--class", class)
		}
		var output bytes.Buffer
		if err := RunReviewPreserveResult(args, &output); err != nil {
			t.Fatalf("preserve --class %q failed: %v", class, err)
		}
		var artifact reviewIncidentArtifact
		decodeStrictReviewJSON(t, output.Bytes(), &artifact)
		if _, err := os.Stat(artifact.Path); err != nil {
			t.Fatalf("preserved artifact does not exist at %q: %v", artifact.Path, err)
		}
		return artifact
	}
	t.Run("empty_result", func(t *testing.T) {
		artifact := preserve(t, string(reviewtransaction.ResultIncidentEmptyResult))
		if artifact.Class != reviewtransaction.ResultIncidentEmptyResult {
			t.Fatalf("Class = %q, want %q", artifact.Class, reviewtransaction.ResultIncidentEmptyResult)
		}
		want := fmt.Sprintf("00-%s-empty_result-", record.State.SelectedLenses[0])
		if !strings.HasPrefix(filepath.Base(artifact.Path), want) {
			t.Fatalf("Path basename %q does not have prefix %q", filepath.Base(artifact.Path), want)
		}
	})
	t.Run("nested_envelope", func(t *testing.T) {
		artifact := preserve(t, string(reviewtransaction.ResultIncidentNestedEnvelope))
		if artifact.Class != reviewtransaction.ResultIncidentNestedEnvelope {
			t.Fatalf("Class = %q, want %q", artifact.Class, reviewtransaction.ResultIncidentNestedEnvelope)
		}
		want := fmt.Sprintf("00-%s-nested_envelope-", record.State.SelectedLenses[0])
		if !strings.HasPrefix(filepath.Base(artifact.Path), want) {
			t.Fatalf("Path basename %q does not have prefix %q", filepath.Base(artifact.Path), want)
		}
	})
	t.Run("omitted class is backward compatible", func(t *testing.T) {
		artifact := preserve(t, "")
		if artifact.Class != "" {
			t.Fatalf("Class = %q, want empty", artifact.Class)
		}
		prefix := fmt.Sprintf("00-%s-", record.State.SelectedLenses[0])
		base := filepath.Base(artifact.Path)
		digest12 := strings.TrimSuffix(strings.TrimPrefix(base, prefix), ".raw")
		if !strings.HasPrefix(base, prefix) || !strings.HasSuffix(base, ".raw") || len(digest12) != 12 {
			t.Fatalf("Path basename %q does not match the old-shape order-lens-digest12 format", base)
		}
	})
}

// TestReviewPreserveResultDuplicateRecoveryIsIdempotent pins scenario 6
// (duplicate recovery): recovery is defined as relaunching the identical
// GENTLE_AI_REVIEW_BINDING and replaying its capture. A second identical
// recovery capture for the same slot must be idempotent through the existing
// store-lock + CAS in CaptureReviewerResult, with no new revision minted.
func TestReviewPreserveResultDuplicateRecoveryIsIdempotent(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, false)
	rawInput := filepath.Join(t.TempDir(), "raw.txt")
	if err := os.WriteFile(rawInput, []byte("raw reviewer output\nthat is not JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	var preserved bytes.Buffer
	if err := RunReviewPreserveResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", rawInput, "--class", "empty_result",
	}, &preserved); err != nil {
		t.Fatalf("preserve incident failed: %v", err)
	}
	var incident reviewIncidentArtifact
	decodeStrictReviewJSON(t, preserved.Bytes(), &incident)
	if incident.Class != reviewtransaction.ResultIncidentEmptyResult {
		t.Fatalf("incident Class = %q, want %q", incident.Class, reviewtransaction.ResultIncidentEmptyResult)
	}

	extractedInput := filepath.Join(t.TempDir(), "extracted.json")
	if err := os.WriteFile(extractedInput, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
		t.Fatal(err)
	}
	captureArgs := []string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", extractedInput,
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	var first, second bytes.Buffer
	if err := RunReviewCaptureResult(captureArgs, &first); err != nil {
		t.Fatalf("first recovery capture failed: %v", err)
	}
	if err := RunReviewCaptureResult(captureArgs, &second); err != nil {
		t.Fatalf("duplicate recovery capture failed: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("duplicate recovery capture produced different manifest bytes:\nfirst=%q\nsecond=%q", first.String(), second.String())
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision {
		t.Fatalf("duplicate recovery capture bumped the review authority revision: %q -> %q", before.Revision, after.Revision)
	}
}

// TestReviewPreserveResultInterruptedRecoveryRetries pins scenario 7
// (interrupted recovery): a recovery capture that fails partway (rejected
// payload) must not consume or corrupt the slot, so a subsequent retry of the
// identical binding with the correct extracted payload succeeds and finalizes.
func TestReviewPreserveResultInterruptedRecoveryRetries(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	rawInput := filepath.Join(t.TempDir(), "raw.txt")
	if err := os.WriteFile(rawInput, []byte("raw reviewer output\nthat is not JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewPreserveResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", rawInput,
	}, io.Discard); err != nil {
		t.Fatalf("preserve failed: %v", err)
	}

	captureArgs := func(input string) []string {
		return []string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
			"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
		}
	}
	// Simulate an interrupted recovery relaunch: the first attempt replays an
	// incomplete payload (missing the required evidence array) and fails
	// before the slot is captured.
	interrupted := filepath.Join(t.TempDir(), "interrupted.json")
	if err := os.WriteFile(interrupted, []byte(`{"findings":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureResult(captureArgs(interrupted), io.Discard); err == nil {
		t.Fatal("interrupted recovery capture with an incomplete payload was accepted")
	}
	// A clean retry of the identical binding with the correct extracted
	// payload must succeed and finalize.
	extracted := filepath.Join(t.TempDir(), "extracted.json")
	if err := os.WriteFile(extracted, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewCaptureResult(captureArgs(extracted), &output); err != nil {
		t.Fatalf("retry after interrupted recovery failed: %v", err)
	}
	manifest := strings.TrimSpace(output.String())
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", manifest}, io.Discard); err != nil {
		t.Fatalf("finalize after interrupted-recovery retry failed: %v", err)
	}
}

func initNestedReviewCLIRepo(t *testing.T) (string, string) {
	t.Helper()
	parent := initReviewCLIRepo(t)
	child := filepath.Join(parent, "nested")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, child, "init", "-q")
	runReviewCLIGit(t, child, "config", "user.email", "test@example.com")
	runReviewCLIGit(t, child, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(child, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, child, "add", "tracked.txt")
	runReviewCLIGit(t, child, "commit", "-qm", "base")
	return parent, child
}
