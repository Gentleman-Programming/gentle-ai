package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// The defect these tests pin: on the provider-issued repository-context path
// every failure was replaced by one constant sentence per typed code. Nine-plus
// community reports (#2227, #2382, #2411, #2461) describe genuinely different
// roots and quote the same string, so the report cannot distinguish them and
// neither can the maintainer reading it.
//
// The opacity itself is legitimate: an absolute path must never reach the
// session transcript. So the property proven here is not "stop being opaque",
// it is "keep the typed code AND name a scrubbed cause", and the proof shape is
// always the same: drive TWO genuinely different roots into ONE collapse point
// and require the two outputs to differ and each to name its own cause. A test
// that asserts one message contains one substring would pass against the
// collapsed constant and prove nothing.

// gitObjectPermissionStderr is a second, unrelated Git setup failure. It shares
// the collapse point with gitMissingRepositoryStderr and is neither an
// ownership refusal nor a missing repository, so the two can only be told apart
// by a forwarded cause.
const gitObjectPermissionStderr = "fatal: cannot access the object database: Permission denied"

// TestOpaqueRepositoryContextResolutionNamesDistinctCauses covers the widest
// collapse point, reviewRepositoryContextResolutionFailure in
// review_incident.go. Every non-trust resolution failure answered with the same
// repository_context_unavailable sentence, and resolution front-runs authority
// discovery (ResolveReviewRepositoryContext itself loads the compact record
// through validateLiveReviewRepositoryContext), so an environmental Git
// refusal, an absent authority record and an unparsable one all converge here.
// Four genuinely different roots, one string.
func TestOpaqueRepositoryContextResolutionNamesDistinctCauses(t *testing.T) {
	reviewEnabledHome(t)
	cases := []struct {
		name    string
		arrange func(t *testing.T, repo, lineage string)
		want    string
	}{
		{
			name:    "git-missing-repository",
			arrange: func(t *testing.T, _, _ string) { installFailingGitShim(t, gitMissingRepositoryStderr) },
			want:    "not a git repository",
		},
		{
			name:    "git-object-permission",
			arrange: func(t *testing.T, _, _ string) { installFailingGitShim(t, gitObjectPermissionStderr) },
			want:    "cannot access the object database",
		},
		{
			name: "authority-record-absent",
			arrange: func(t *testing.T, repo, lineage string) {
				if err := os.Remove(reviewCLICompactStatePath(repo, lineage)); err != nil {
					t.Fatal(err)
				}
			},
			want: nativeNotExistCause(t),
		},
		{
			name: "authority-record-unparsable",
			arrange: func(t *testing.T, repo, lineage string) {
				if err := os.WriteFile(reviewCLICompactStatePath(repo, lineage), []byte("{not json"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "invalid character",
		},
	}
	messages := make(map[string]string, len(cases))
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			lineage := "opaque-cause-" + tt.name
			args, _, repo := startedOpaqueCaptureBinding(t, lineage)
			tt.arrange(t, repo, lineage)

			err := RunReviewCaptureResult(append(append([]string{}, args...), "--preflight"), io.Discard)
			if err == nil {
				t.Fatal("preflight succeeded despite an unresolvable repository context")
			}
			message := err.Error()
			assertOpaqueFailureNamesCause(t, message, "repository_context_unavailable", tt.want, repo)
			messages[tt.name] = message
		})
	}
	wantMessages := len(cases)
	if runtime.GOOS == "windows" {
		// The Git shim is a POSIX shell script, so Windows exercises the two
		// native filesystem roots while retaining their distinct-cause proof.
		wantMessages -= 2
	}
	assertPairwiseDistinct(t, messages, wantMessages)
}

// TestOpaqueRepositoryContextCaptureNamesDistinctCauses covers the collapse
// point reported as #2411: everything past admission answered with one
// repository_context_capture_failed sentence, which is why a preflight that
// cannot reach any of these paths appeared to "pass" while capture "failed".
//
// The property is that the causes are DISTINGUISHABLE, not that they share a
// code. An occupied slot now carries its own, because #2411's rc.6 occurrence
// showed the shared code's action ("retry the same exact binding") is the one
// thing that cannot work while the slot is occupied. So the code is asserted
// per case; a cause that stops being distinguishable still fails here.
func TestOpaqueRepositoryContextCaptureNamesDistinctCauses(t *testing.T) {
	reviewEnabledHome(t)
	cases := []struct {
		name    string
		arrange func(t *testing.T, repo, lineage string, binding []string)
		code    string
		want    string
	}{
		{
			name: "slot-occupied",
			arrange: func(t *testing.T, _, _ string, binding []string) {
				first := admissibleOpaqueReviewerResult(t, binding, "first reviewer evidence")
				if err := RunReviewCaptureResult(append(append([]string{}, binding...), "--input", first), io.Discard); err != nil {
					t.Fatalf("first capture failed: %v", err)
				}
			},
			code: reviewerResultSlotOccupiedCode,
			want: reviewNextTransitionRefreshCommandV21,
		},
		{
			name: "compact-authority-unwritable",
			arrange: func(t *testing.T, repo, lineage string, _ []string) {
				blockCanonicalAuthorityWrite(t, repo, lineage)
			},
			code: "repository_context_capture_failed",
			want: "permission denied",
		},
	}
	messages := make(map[string]string, len(cases))
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			lineage := "opaque-capture-" + tt.name
			binding, _, repo := startedOpaqueCaptureBinding(t, lineage)
			result := admissibleOpaqueReviewerResult(t, binding, "second reviewer evidence")
			tt.arrange(t, repo, lineage, binding)

			err := RunReviewCaptureResult(append(append([]string{}, binding...), "--input", result), io.Discard)
			if err == nil {
				t.Fatal("capture succeeded despite an unusable authority store")
			}
			message := err.Error()
			assertOpaqueFailureNamesCause(t, message, tt.code, tt.want, repo)
			messages[tt.name] = message
		})
	}
	wantMessages := len(cases)
	if runtime.GOOS == "windows" {
		// blockCanonicalAuthorityWrite requires POSIX permissions, so Windows
		// exercises only the occupied-slot root while retaining its
		// distinct-cause proof.
		wantMessages--
	}
	assertPairwiseDistinct(t, messages, wantMessages)
}

// TestOpaqueRepositoryContextCauseIsScrubbed is the other half of the contract:
// a cause that WOULD leak an absolute path must reach the caller with the path
// gone and the rest of the cause intact. Forwarding an unscrubbed cause and
// forwarding nothing are both failures.
func TestOpaqueRepositoryContextCauseIsScrubbed(t *testing.T) {
	reviewEnabledHome(t)
	lineage := "opaque-scrub-cause"
	args, _, repo := startedOpaqueCaptureBinding(t, lineage)
	if err := os.Remove(reviewCLICompactStatePath(repo, lineage)); err != nil {
		t.Fatal(err)
	}

	err := RunReviewCaptureResult(append(append([]string{}, args...), "--preflight"), io.Discard)
	if err == nil {
		t.Fatal("preflight succeeded despite a removed authority record")
	}
	message := err.Error()
	// The underlying os error names the absolute state-file path, so this cause
	// is exactly the shape the opacity exists to contain.
	if strings.Contains(message, repo) || strings.Contains(message, lineage+"/review-state.json") {
		t.Fatalf("forwarded cause leaked an absolute path: %s", message)
	}
	if !strings.Contains(message, nativeNotExistCause(t)) {
		t.Fatalf("scrubbing removed the cause along with the path: %s", message)
	}
	if scrubbed := reviewScrubDefectReportField(message); scrubbed != message {
		t.Fatalf("failure is not path-safe; the privacy gate rewrote it to %q", scrubbed)
	}
}

func assertOpaqueFailureNamesCause(t *testing.T, message, code, wantCause, repo string) {
	t.Helper()
	if !strings.Contains(message, code) {
		t.Fatalf("failure lost its typed code %q: %s", code, message)
	}
	if !strings.Contains(message, wantCause) {
		t.Fatalf("failure does not name its own cause %q: %s", wantCause, message)
	}
	if strings.Contains(message, repo) {
		t.Fatalf("failure leaked a private path: %s", message)
	}
}

// nativeNotExistCause asks THIS installation how it renders a missing file,
// instead of pinning the sentence a US English installation renders. What the
// assertion needs is that the forwarded cause survives scrubbing, and the
// condition behind it -- ERROR_FILE_NOT_FOUND, the errno the removed authority
// record leaves behind -- does not vary. The sentence does: syscall.Errno.Error
// asks FormatMessage for US English first and falls back to the installation's
// own message resources when that lookup fails, so an installation carrying no
// English resources renders the same errno in its own language. Pinning the
// text asserted the machine, not the contract.
func nativeNotExistCause(t *testing.T) string {
	t.Helper()
	// The parent directory must exist. A missing parent is a different
	// condition (ERROR_PATH_NOT_FOUND on Windows) and renders a different
	// sentence, which would silently weaken every assertion built on this.
	_, err := os.Open(filepath.Join(t.TempDir(), "absent-authority-record.json"))
	if err == nil {
		t.Fatal("opening an absent file succeeded, so this platform cannot render the cause under test")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe reported something other than a missing file, so its sentence is not the one under test: %v", err)
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("probe error carries no separable cause to render: %v", err)
	}
	return pathErr.Err.Error()
}

// opaqueNativeCause still serves one call site, and its two arms are safe for
// different reasons. The Windows arm expects "unsafe RAR authority path",
// which gentle-ai produces itself. The Unix arm does pin an errno text, "not
// a directory", and is safe because Go renders errno 20 from its own
// compile-time table rather than from the installation. Neither arm reaches
// FormatMessage, which is what the replaced assertions depended on.
func opaqueNativeCause(unix, windows string) string {
	if runtime.GOOS == "windows" {
		return windows
	}
	return unix
}

// assertPairwiseDistinct is the whole point of these tests: every root that
// reaches one collapse point must leave it with its own message. Asserting a
// single message contains a single substring would pass against a constant.
func assertPairwiseDistinct(t *testing.T, messages map[string]string, want int) {
	t.Helper()
	if len(messages) != want {
		t.Fatalf("every root must reach the collapse point; captured %d of %d", len(messages), want)
	}
	seen := make(map[string]string, len(messages))
	for name, message := range messages {
		if other, collapsed := seen[message]; collapsed {
			t.Fatalf("roots %q and %q collapsed into one message: %s", other, name, message)
		}
		seen[message] = name
	}
}

func reviewCLICompactStoreDir(repo, lineage string) string {
	return filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2", lineage)
}

func reviewCLICompactStatePath(repo, lineage string) string {
	return filepath.Join(reviewCLICompactStoreDir(repo, lineage), "review-state.json")
}

// admissibleOpaqueReviewerResult writes a reviewer result the native admission
// accepts for the supplied opaque binding, reading the subject hash and the
// inspected manifest back from the product's own preflight.
func admissibleOpaqueReviewerResult(t *testing.T, binding []string, evidence string) string {
	t.Helper()
	var preflightOutput bytes.Buffer
	if err := RunReviewCaptureResult(append(append([]string{}, binding...), "--preflight"), &preflightOutput); err != nil {
		t.Fatalf("capture preflight failed: %v", err)
	}
	var preflight reviewCapturePreflightResult
	if err := json.Unmarshal(preflightOutput.Bytes(), &preflight); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, len(preflight.ChangedPathManifest))
	for index, entry := range preflight.ChangedPathManifest {
		paths[index] = entry.Path
	}
	path := filepath.Join(t.TempDir(), "reviewer-result.json")
	writeReviewCLIJSON(t, path, facadeReviewerResult{
		SubjectHash: preflight.ArtifactSubject.SubjectHash,
		Inspection: reviewtransaction.ArtifactInspection{
			Status: reviewtransaction.ArtifactInspectionCompleted, Paths: paths,
		},
		Findings: []facadeFinding{},
		Evidence: []string{evidence},
	})
	return path
}
