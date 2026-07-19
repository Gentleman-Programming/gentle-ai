package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func TestReviewCaptureResultStrictBindingReplayAndFinalize(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	input := filepath.Join(t.TempDir(), "result.json")
	args := func(lineage, target, lens, order string) []string {
		return []string{"--cwd", repo, "--lineage", lineage, "--target", target, "--lens", lens, "--order", order, "--input", input}
	}
	validArgs := args(started.LineageID, record.State.InitialSnapshot.Identity, record.State.SelectedLenses[0], "0")
	for _, payload := range []string{"prose", `{}`, `{"findings":[],"evidence":[]} {}`, `{"findings":[],"evidence":[],"unknown":true}`} {
		if err := os.WriteFile(input, []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewCaptureResult(validArgs, io.Discard); err == nil {
			t.Fatalf("invalid payload accepted: %s", payload)
		}
	}
	if err := os.WriteFile(input, []byte(`{"findings":[],"evidence":["checked exact target"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{
		args("wrong-lineage", record.State.InitialSnapshot.Identity, record.State.SelectedLenses[0], "0"),
		args(started.LineageID, "sha256:"+strings.Repeat("0", 64), record.State.SelectedLenses[0], "0"),
		args(started.LineageID, record.State.InitialSnapshot.Identity, "review-risk", "0"),
		args(started.LineageID, record.State.InitialSnapshot.Identity, record.State.SelectedLenses[0], "1"),
	} {
		if err := RunReviewCaptureResult(bad, io.Discard); err == nil {
			t.Fatal("wrong capture binding accepted")
		}
	}
	var first, replay bytes.Buffer
	if err := RunReviewCaptureResult(validArgs, &first); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureResult(validArgs, &replay); err != nil || first.String() != replay.String() {
		t.Fatalf("exact replay changed: %v", err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, first.Bytes(), &artifact)
	if err := os.WriteFile(input, []byte(`{"findings":[],"evidence":["different"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureResult(validArgs, io.Discard); err == nil {
		t.Fatal("mismatched replay accepted")
	}
	manifest := strings.TrimSpace(first.String())
	for _, finalize := range [][]string{
		{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", manifest, "--result", input},
		{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", manifest, "--result-artifact", manifest},
	} {
		if err := RunReviewFacadeFinalize(finalize, io.Discard); err == nil {
			t.Fatal("mixed or duplicate artifact accepted")
		}
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", manifest}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestReviewCaptureResultWaitsForMaintenanceBeforePublication(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, false)
	input := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(input, []byte(`{"findings":[],"evidence":["checked exact target"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	held, err := reviewtransaction.AcquireReviewMaintenanceExclusive(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity, "--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input}
	if err := RunReviewCaptureResult(args, io.Discard); !errors.Is(err, reviewtransaction.ErrAuthorityLockTimeout) {
		t.Fatalf("capture while maintenance held = %v", err)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("authority changed while capture blocked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, reviewtransaction.CompactReviewerResultsDir)); !os.IsNotExist(err) {
		t.Fatalf("capture published while maintenance held: %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureResult(args, io.Discard); err != nil {
		t.Fatalf("capture after maintenance release: %v", err)
	}
}
func TestReviewArtifactSubstitutionFailsBeforeMutation(t *testing.T) {
	mutations := []struct {
		name string
		run  func(*reviewResultArtifact)
	}{
		{"lineage", func(a *reviewResultArtifact) { a.LineageID = "wrong" }},
		{"target", func(a *reviewResultArtifact) { a.TargetIdentity = "sha256:" + strings.Repeat("0", 64) }},
		{"lens", func(a *reviewResultArtifact) { a.Lens = "review-risk" }},
		{"order", func(a *reviewResultArtifact) { a.SelectedOrder = 1 }},
		{"hash", func(a *reviewResultArtifact) { a.SHA256 = "sha256:" + strings.Repeat("0", 64) }},
		{"path", func(a *reviewResultArtifact) { a.Path = filepath.Join(t.TempDir(), "result.json") }},
	}
	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			repo, started, store, record, artifact := capturedArtifact(t)
			tt.run(&artifact)
			assertArtifactFinalizeUnchanged(t, repo, started.LineageID, store, record.Revision, artifact)
		})
	}
	for _, kind := range []string{"directory", "symlink", "mode", "bytes", "race"} {
		t.Run(kind, func(t *testing.T) {
			repo, started, store, record, artifact := capturedArtifact(t)
			original, _ := os.ReadFile(artifact.Path)
			replacement := filepath.Join(t.TempDir(), "replacement.json")
			if err := os.WriteFile(replacement, original, 0o600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "directory":
				_ = os.Remove(artifact.Path)
				_ = os.Mkdir(artifact.Path, 0o700)
			case "symlink":
				_ = os.Remove(artifact.Path)
				_ = os.Symlink(filepath.Join(t.TempDir(), "missing"), artifact.Path)
			case "mode":
				if runtime.GOOS == "windows" {
					t.Skip("Windows ACLs do not map to Unix mode bits")
				}
				_ = os.Chmod(artifact.Path, 0o644)
			case "bytes":
				_ = os.WriteFile(artifact.Path, []byte("replacement"), 0o600)
			case "race":
				reviewArtifactAfterLstat = func() {
					reviewArtifactAfterLstat = func() {}
					if err := os.Rename(artifact.Path, artifact.Path+".old"); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(replacement, artifact.Path); err != nil {
						t.Fatal(err)
					}
				}
				t.Cleanup(func() { reviewArtifactAfterLstat = func() {} })
			}
			assertArtifactFinalizeUnchanged(t, repo, started.LineageID, store, record.Revision, artifact)
		})
	}
	if !reviewArtifactModeSafeForOS(0o666, false, "windows") || reviewArtifactModeSafeForOS(0o666, false, "linux") {
		t.Fatal("platform permission semantics changed")
	}
}
func TestReviewCaptureResultConcurrentSelectedLenses(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, true)
	manifests := make([]string, len(record.State.SelectedLenses))
	var wg sync.WaitGroup
	for order, lens := range record.State.SelectedLenses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			input := filepath.Join(t.TempDir(), fmt.Sprintf("%d.json", order))
			_ = os.WriteFile(input, []byte(`{"findings":[],"evidence":["checked exact target"]}`), 0o600)
			var output bytes.Buffer
			err := RunReviewCaptureResult([]string{"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity, "--lens", lens, "--order", fmt.Sprint(order), "--input", input}, &output)
			if err != nil {
				t.Errorf("capture %s: %v", lens, err)
				return
			}
			manifests[order] = strings.TrimSpace(output.String())
		}()
	}
	wg.Wait()
	if _, err := readFacadeReviewerArtifacts(manifests, store.Dir, record.State); err != nil {
		t.Fatal(err)
	}
}
func TestCaptureReviewerArtifactDirectorySync(t *testing.T) {
	originalGOOS, originalSync := reviewArtifactRuntimeGOOS, syncReviewerArtifactDirectory
	t.Cleanup(func() { reviewArtifactRuntimeGOOS, syncReviewerArtifactDirectory = originalGOOS, originalSync })
	warning := []byte(`{"findings":[{"severity":"WARNING"}],"evidence":["unchanged"]}` + "\n")
	cases := []struct {
		name, goos string
		err        error
		wantOK     bool
	}{
		{"fatal", "linux", errors.New("disk sync failed"), false},
		{"invalid", "linux", syscall.EINVAL, true},
		{"unsupported", "linux", errors.ErrUnsupported, true},
		{"windows permission", "windows", os.ErrPermission, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			storeDir := t.TempDir()
			state := reviewtransaction.CompactState{LineageID: "lineage", InitialSnapshot: reviewtransaction.Snapshot{Identity: "target"}, SelectedLenses: []string{"review-correctness"}}
			reviewArtifactRuntimeGOOS = func() string { return tt.goos }
			syncReviewerArtifactDirectory = func(string) error { return tt.err }
			artifact, err := captureReviewerArtifact(storeDir, state, 0, warning)
			path := filepath.Join(storeDir, "reviewer-results", "00-review-correctness.json")
			if !tt.wantOK {
				if _, statErr := os.Stat(path); err == nil || artifact != (reviewResultArtifact{}) || !os.IsNotExist(statErr) {
					t.Fatalf("fatal sync returned artifact or retained publication: artifact=%+v err=%v", artifact, err)
				}
				return
			}
			got, readErr := os.ReadFile(path)
			if err != nil || readErr != nil || !bytes.Equal(got, warning) {
				t.Fatalf("compatible sync changed WARNING result: capture=%v read=%v got=%q", err, readErr, got)
			}
		})
	}
}
func newArtifactReview(t *testing.T, high bool) (string, ReviewFacadeStartResult, reviewtransaction.CompactStore, reviewtransaction.CompactRecord) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	name := "tracked.txt"
	if high {
		name = "service-token.ts"
	}
	if err := os.WriteFile(filepath.Join(repo, name), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return repo, started, store, record
}
func capturedArtifact(t *testing.T) (string, ReviewFacadeStartResult, reviewtransaction.CompactStore, reviewtransaction.CompactRecord, reviewResultArtifact) {
	t.Helper()
	repo, started, store, record := newArtifactReview(t, false)
	input := filepath.Join(t.TempDir(), "result.json")
	_ = os.WriteFile(input, []byte(`{"findings":[],"evidence":["checked exact target"]}`), 0o600)
	var output bytes.Buffer
	if err := RunReviewCaptureResult([]string{"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity, "--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input}, &output); err != nil {
		t.Fatal(err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, output.Bytes(), &artifact)
	return repo, started, store, record, artifact
}
func assertArtifactFinalizeUnchanged(t *testing.T, repo, lineage string, store reviewtransaction.CompactStore, revision string, artifact reviewResultArtifact) {
	t.Helper()
	payload, _ := json.Marshal(artifact)
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage, "--result-artifact", string(payload)}, io.Discard); err == nil {
		t.Fatal("substituted artifact accepted")
	}
	after, _ := store.Load()
	if after.Revision != revision {
		t.Fatal("artifact mismatch mutated authority")
	}
}
