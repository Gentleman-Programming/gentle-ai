package assets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The plugin half of the same defect. sessionErrorMessage answered every
// opaque-binding failure that was neither a Git trust refusal nor a structured
// admission rejection with one constant sentence, so a strict-decode rejection
// (#2227), an unreachable authority (#2461) and a failed durable publish
// (#2411) all reached the transcript identically. These tests drive genuinely
// different native failures through the one collapse point and require the
// outputs to differ, which a "message contains substring" assertion cannot do.

// TestReviewPluginNamesDistinctNativeCauses is the two-distinct-causes proof.
func TestReviewPluginNamesDistinctNativeCauses(t *testing.T) {
	cases := []struct {
		name   string
		native string
		want   string
	}{
		{
			name:   "strict-decode-rejection",
			native: `Error: decode reviewer result: json: unknown field "inspection_note"`,
			want:   `unknown field "inspection_note"`,
		},
		{
			name: "durable-publish-refusal",
			native: "Error: repository_context_capture_failed: provider-issued review repository context operation failed; " +
				"cause: publish reviewer result atomically: operation not supported; retry capture-result with the same exact binding or refresh status",
			want: "operation not supported",
		},
	}
	messages := make(map[string]string, len(cases))
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			message := runReviewPluginScenario(t, "after-opaque", tt.native)
			if message == "NO_ERROR" {
				t.Fatal("plugin did not fail despite an always-failing native binary")
			}
			if !strings.Contains(message, "repository_context_capture_failed") {
				t.Fatalf("failure lost its typed code: %s", message)
			}
			if !strings.Contains(message, tt.want) {
				t.Fatalf("failure does not name its own cause %q: %s", tt.want, message)
			}
			messages[tt.name] = message
		})
	}
	if len(messages) != len(cases) {
		t.Fatalf("both roots must reach the collapse point; captured %d", len(messages))
	}
	if messages["strict-decode-rejection"] == messages["durable-publish-refusal"] {
		t.Fatalf("two genuinely different native failures collapsed into one message: %s", messages["strict-decode-rejection"])
	}
}

// TestReviewPluginScrubsForwardedNativeCause is the other half: forwarding a
// cause must not become a path leak. The absolute path must be gone and the
// cause must still be there. Dropping both would also make the path absent,
// which is why the second assertion exists.
func TestReviewPluginScrubsForwardedNativeCause(t *testing.T) {
	const private = "/home/someone/private/repo/.git/gentle-ai/review-transactions/v2/lineage/review-state.json"
	native := "Error: load compact facade review lineage: open " + private + " no such file or directory"

	message := runReviewPluginScenario(t, "after-opaque", native)
	if message == "NO_ERROR" {
		t.Fatal("plugin did not fail despite an always-failing native binary")
	}
	if strings.Contains(message, private) || strings.Contains(message, "/home/someone") {
		t.Fatalf("forwarded native cause leaked an absolute path: %s", message)
	}
	if !strings.Contains(message, "load compact facade review lineage") {
		t.Fatalf("scrubbing removed the cause along with the path: %s", message)
	}
	if !strings.Contains(message, "<redacted>") {
		t.Fatalf("the leaked path was dropped rather than redacted: %s", message)
	}
}

// TestReviewPluginNamesRejectedBindingField covers #2442 and #2461: thirteen
// conditions used to share the sentence "review task binding does not match the
// selected lens", which names none of them. #2461 reported all four lenses
// rejected with exactly that sentence.
func TestReviewPluginNamesRejectedBindingField(t *testing.T) {
	cases := []struct {
		name     string
		scenario string
		want     []string
		reject   []string
	}{
		{
			name:     "quoted-order",
			scenario: "before-quoted-order",
			want:     []string{`field "order"`, "not a quoted string", "a string of length 1"},
		},
		{
			name:     "negative-order",
			scenario: "before-negative-order",
			want:     []string{`field "order"`, "zero or greater", "the integer -1"},
		},
		{
			name:     "unexpected-field-set",
			scenario: "before-unknown-shape",
			want:     []string{"unexpected field set", "lens,lineage,order,revision,subject_hash,target", "expected exactly one of"},
		},
		{
			name:     "short-target",
			scenario: "before-short-target",
			want:     []string{`field "target"`, "64 lowercase hex", "a string of length 11"},
		},
		{
			name:     "wrong-lens",
			scenario: "before-other-lens",
			want:     []string{`field "lens"`, `must equal the launched lens "review-risk"`, `received "review-reliability"`},
		},
		{
			name:     "path-shaped-context",
			scenario: "before-bad-context",
			want:     []string{`field "repository_context"`, "rctx1_"},
			reject:   []string{"/home/someone/private/repo"},
		},
	}
	messages := make(map[string]string, len(cases))
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			message := runReviewPluginScenario(t, tt.scenario, "NATIVE-PROCESS-MUST-NOT-RUN")
			if message == "NO_ERROR" {
				t.Fatal("plugin accepted a binding it must reject")
			}
			if message == "review task binding does not match the selected lens" {
				t.Fatal("binding refusal still names no field")
			}
			for _, want := range tt.want {
				if !strings.Contains(message, want) {
					t.Fatalf("binding refusal does not contain %q: %s", want, message)
				}
			}
			for _, reject := range tt.reject {
				if strings.Contains(message, reject) {
					t.Fatalf("binding refusal echoed untrusted content %q: %s", reject, message)
				}
			}
			if !strings.Contains(message, "gentle-ai review status") {
				t.Fatalf("binding refusal names no executable continuation: %s", message)
			}
			messages[tt.name] = message
		})
	}
	seen := make(map[string]string, len(messages))
	for name, message := range messages {
		if other, collapsed := seen[message]; collapsed {
			t.Fatalf("binding causes %q and %q collapsed into one message: %s", other, name, message)
		}
		seen[message] = name
	}
	if len(messages) != len(cases) {
		t.Fatalf("every binding cause must produce its own refusal; captured %d of %d", len(messages), len(cases))
	}
}

// TestReviewPluginIgnoresStaleReviewCwdOverride covers #2446. A
// GENTLE_AI_REVIEW_CWD value left behind by an earlier session used to outrank
// the OpenCode anchor with no check that it named the same repository, so
// capture silently ran against an unrelated checkout and stranded every lens
// result there. The override is deleted, so the only observable working
// directory is the anchor.
func TestReviewPluginIgnoresStaleReviewCwdOverride(t *testing.T) {
	foreign := t.TempDir()
	log := filepath.Join(t.TempDir(), "native-cwd.log")

	message := runReviewPluginScenarioWithEnv(t, "after-opaque", "", "resolve failed", "",
		"GENTLE_AI_REVIEW_CWD="+foreign, "GENTLE_AI_STUB_CWD_LOG="+log)
	if message == "NO_ERROR" {
		t.Fatal("plugin did not fail despite an always-failing native binary")
	}
	recorded, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("the native binary was never invoked, so the working directory is unproven: %v", err)
	}
	directories := strings.Fields(strings.TrimSpace(string(recorded)))
	if len(directories) == 0 {
		t.Fatal("the native binary recorded no working directory")
	}
	for _, directory := range directories {
		if directory == foreign {
			t.Fatalf("a stale GENTLE_AI_REVIEW_CWD redirected capture to an unrelated repository: %q", directory)
		}
		if filepath.Base(directory) != "work" {
			t.Fatalf("capture ran outside the OpenCode anchor: %q", directory)
		}
	}
}
