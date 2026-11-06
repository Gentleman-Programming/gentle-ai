package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

func TestRctx2RealRepositoryDiagnosticsWithoutMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("creates real Git repositories and compact authority")
	}
	const lineage = "rctx2-diagnostics"
	argsA, _, repoA := startedOpaqueCaptureBinding(t, lineage)
	argsB, _, repoB := startedOpaqueCaptureBinding(t, lineage)
	before := make(map[string][]byte)
	for _, repo := range []string{repoA, repoB} {
		store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
		if err != nil {
			t.Fatal(err)
		}
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		binding := reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: lineage, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
		}
		handle, err := reviewtransaction.DeriveReviewRepositoryContextHandle(t.Context(), repo, binding)
		if err != nil {
			t.Fatal(err)
		}
		args := argsA
		if repo == repoB {
			args = argsB
		}
		if handle != args[slices.Index(args, "--repository-context")+1] {
			t.Fatal("derived handle differs from real authority-backed provider binding")
		}
		before[store.StatePath()], err = os.ReadFile(store.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if err := RunReviewCaptureResult(append(append([]string{}, args...), "--preflight"), &output); err != nil {
			t.Fatalf("matching repository positive control: %v", err)
		}
		if bytes.Contains(output.Bytes(), []byte(repo)) {
			t.Fatal("positive control leaked repository path")
		}
	}
	missing := reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: "rctx2-no-authority", TargetIdentity: argsA[slices.Index(argsA, "--target")+1], Revision: argsA[slices.Index(argsA, "--expected-revision")+1],
	}
	missingHandle, err := reviewtransaction.DeriveReviewRepositoryContextHandle(t.Context(), repoA, missing)
	if err != nil {
		t.Fatal(err)
	}
	missingArgs := replaceReviewArgument(t, argsA, "--lineage", missing.LineageID)
	missingArgs = replaceReviewContextArgument(t, missingArgs, missingHandle)
	inactive := missing
	inactive.LineageID, inactive.Revision = lineage, "sha256:"+strings.Repeat("f", 64)
	inactiveHandle, err := reviewtransaction.DeriveReviewRepositoryContextHandle(t.Context(), repoA, inactive)
	if err != nil {
		t.Fatal(err)
	}
	inactiveArgs := replaceReviewArgument(t, argsA, "--expected-revision", inactive.Revision)
	inactiveArgs = replaceReviewContextArgument(t, inactiveArgs, inactiveHandle)
	for _, tt := range []struct {
		name, code, action string
		args               []string
	}{
		{"wrong repository", "rctx2_binding_unusable", "verify --cwd", replaceReviewArgument(t, argsA, "--cwd", repoB)},
		{"other repository handle and binding", "rctx2_binding_unusable", "verify --cwd", replaceReviewArgument(t, argsB, "--cwd", repoA)},
		{"malformed digest", "rctx2_binding_unusable", "refresh", replaceReviewContextArgument(t, argsA, "rctx2_not-hex")},
		{"valid-shape arbitrary digest", "rctx2_binding_unusable", "verify --cwd", replaceReviewContextArgument(t, argsA, "rctx2_"+strings.Repeat("a", 64))},
		{"malformed digest without authority", "rctx2_binding_unusable", "refresh", replaceReviewContextArgument(t, missingArgs, "rctx2_not-hex")},
		{"wrong revision", "rctx2_binding_unusable", "refresh", replaceReviewArgument(t, argsA, "--expected-revision", "sha256:"+strings.Repeat("0", 64))},
		{"missing authority", "rctx2_resolution_failed", "underlying cause", missingArgs},
		{"matching digest but inactive binding", "rctx2_resolution_failed", "underlying cause", inactiveArgs},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := RunReviewCaptureResult(append(append([]string{}, tt.args...), "--preflight"), io.Discard)
			if err == nil {
				t.Fatal("invalid repository context accepted")
			}
			message := err.Error()
			for _, want := range []string{tt.code, tt.action} {
				if !strings.Contains(message, want) {
					t.Errorf("error = %q; want %q", message, want)
				}
			}
			for _, forbidden := range []string{repoA, repoB, os.Getenv("HOME"), "binding was valid when issued", "rctx2_identity_mismatch"} {
				if strings.Contains(message, forbidden) {
					t.Errorf("error leaks or overclaims %q: %s", forbidden, message)
				}
			}
			if reviewScrubDefectReportField(message) != message {
				t.Error("error is not path-safe")
			}
		})
	}
	for path, want := range before {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("diagnostics mutated compact authority: %v", err)
		}
	}
	if _, err := os.Stat(reviewCLICompactStoreDir(repoA, missing.LineageID)); !os.IsNotExist(err) {
		t.Fatalf("resolution created absent authority: %v", err)
	}
}

// TestRctx2MalformedBindingTupleReportsBindingUnusable pins the outer precheck
// contract: a malformed lineage, target, or revision supplied with an rctx2
// handle is a structural binding failure, so it must surface the same typed
// rctx2_binding_unusable code the in-package resolver produces instead of the
// generic repository_context_unavailable. A non-V2 handle keeps the generic
// code, proving the shared validator was not changed, and no diagnostic path
// may mutate authority or leak a path.
func TestRctx2MalformedBindingTupleReportsBindingUnusable(t *testing.T) {
	if testing.Short() {
		t.Skip("creates real Git repositories and compact authority")
	}
	const lineage = "rctx2-malformed-binding"
	args, _, repo := startedOpaqueCaptureBinding(t, lineage)
	statePath := reviewCLICompactStatePath(repo, lineage)
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	malformed := func(name, value string) []string {
		return replaceReviewArgument(t, args, name, value)
	}
	genericRctx3 := replaceReviewContextArgument(t, malformed("--expected-revision", "not-a-revision"), "rctx3_not-sealed")
	genericRctx1 := replaceReviewContextArgument(t, malformed("--expected-revision", "not-a-revision"), "rctx1_"+strings.Repeat("a", 64))
	for _, tt := range []struct {
		name, code string
		args       []string
	}{
		{"malformed revision", "rctx2_binding_unusable", malformed("--expected-revision", "not-a-revision")},
		{"malformed target", "rctx2_binding_unusable", malformed("--target", "not-a-target")},
		{"malformed lineage", "rctx2_binding_unusable", malformed("--lineage", "NOT-canonical")},
		{"non-V2 rctx3 handle keeps generic code", "repository_context_unavailable", genericRctx3},
		{"non-V2 rctx1 handle keeps generic code", "repository_context_unavailable", genericRctx1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := RunReviewCaptureResult(append(append([]string{}, tt.args...), "--preflight"), io.Discard)
			if err == nil {
				t.Fatal("malformed binding accepted")
			}
			message := err.Error()
			if !strings.Contains(message, tt.code) {
				t.Errorf("error = %q; want code %q", message, tt.code)
			}
			if tt.code != "rctx2_binding_unusable" && strings.Contains(message, "rctx2_binding_unusable") {
				t.Errorf("non-V2 failure was relabelled as a V2 binding failure: %s", message)
			}
			for _, forbidden := range []string{repo, os.Getenv("HOME")} {
				if strings.Contains(message, forbidden) {
					t.Errorf("error leaks %q: %s", forbidden, message)
				}
			}
			if reviewScrubDefectReportField(message) != message {
				t.Errorf("error is not path-safe: %s", message)
			}
		})
	}
	after, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("diagnostics mutated compact authority: %v", err)
	}
}

func TestRctx2ClassifierDoesNotRelabelGenericErrors(t *testing.T) {
	for _, cause := range []error{
		errors.New("invalid rctx2 repository context"),
		errors.New("review repository context identity changed"),
		errors.New("unrelated repository failure"),
	} {
		err := reviewRepositoryContextResolutionFailure(cause)
		var classified *reviewOpaqueContextOperationError
		if !errors.As(err, &classified) || classified.Code != "repository_context_unavailable" {
			t.Fatalf("generic error was relabelled: %s", err)
		}
	}
}
