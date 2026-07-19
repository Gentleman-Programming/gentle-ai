package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

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
		preflight.Lens != record.State.SelectedLenses[0] || preflight.SelectedOrder != 0 {
		t.Fatalf("preflight result = %+v", preflight)
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
	if err := os.WriteFile(input, []byte(`{"findings":[],"evidence":["checked exact target"]}`), 0o600); err != nil {
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
	args := []string{
		"--cwd", parent, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}
	var first, replay bytes.Buffer
	if err := RunReviewPreserveResult(args, &first); err != nil {
		t.Fatalf("preserve under the non-reviewing repository failed: %v", err)
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
	extracted := `{"findings":[],"evidence":["checked exact target"]}`
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
