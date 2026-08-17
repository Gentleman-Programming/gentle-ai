package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestNegotiatedStatusMatchesReviewStartRDDMode(t *testing.T) {
	tests := []struct {
		name      string
		global    string
		cloneOff  bool
		enabled   bool
		wantScope string
	}{
		{name: "global off", global: "disable", wantScope: "global"},
		{name: "global unset clone off", cloneOff: true, wantScope: "clone"},
		{name: "global on clone off", global: "enable", cloneOff: true, wantScope: "clone"},
		{name: "global on", global: "enable", enabled: true},
		{name: "global unset clone unset default on", enabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviewModeHome(t)
			repo := initReviewCLIRepo(t)
			if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("changed\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if tt.global != "" {
				if err := RunReviewMode([]string{tt.global, "--cwd", repo, "--scope", "global"}, io.Discard); err != nil {
					t.Fatal(err)
				}
			}
			if tt.cloneOff {
				disableReviewForClone(t, repo)
			}
			var output bytes.Buffer
			stderr := captureReviewProcessStderr(t)
			if err := RunReview([]string{
				"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--agent", "opencode",
				"--action-eligibility", "--next-transition",
			}, &output); err != nil {
				t.Fatalf("STATUS: %v", err)
			}
			var status ReviewTargetStatusResult
			if err := json.Unmarshal(output.Bytes(), &status); err != nil {
				t.Fatalf("decode STATUS: %v\n%s", err, output.String())
			}
			allowed := status.Eligibility.AllowedActions[0]
			if tt.enabled {
				if allowed.Action != "review.start" || status.NextTransition.Kind != reviewNextTransitionExecute || status.NextTransition.Execute.Operation != "review.start" {
					t.Fatalf("enabled STATUS = %#v", status)
				}
				if !strings.Contains(status.NextTransition.Execute.Command, "--cwd="+repo) {
					t.Fatalf("START command is not repository-bound: %q", status.NextTransition.Execute.Command)
				}
			} else {
				if allowed.Action != "stop" || allowed.ReasonCode != "forbidden_rdd_disabled" {
					t.Fatalf("disabled eligibility = %#v", status.Eligibility)
				}
				if status.NextTransition.Kind != reviewNextTransitionStop || status.NextTransition.ReasonCode != "rdd_disabled" {
					t.Fatalf("disabled transition = %#v", status.NextTransition)
				}
			}
			// A successful negotiated STATUS is byte-silent on stderr in
			// every mode: the rdd_disabled continuation is not narrated, the
			// reason code stays structural in next_transition.reason_code.
			if got := stderr(); got != "" {
				t.Fatalf("negotiated STATUS wrote stderr, want zero bytes:\n%q", got)
			}
			startErr := RunReview([]string{
				"start", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--agent", "opencode",
				"--target", status.TargetIdentity, "--projection", "workspace", "--consent", "granted",
			}, io.Discard)
			if gotDisabled := errors.Is(startErr, reviewtransaction.ErrRDDDisabled); gotDisabled != !tt.enabled {
				t.Fatalf("START disabled = %v, want %v (error %v)", gotDisabled, !tt.enabled, startErr)
			}
		})
	}
}

func TestNegotiatedStatusFailsWhenEffectiveModeCannotResolve(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.MkdirAll(filepath.Join(home, ".gentle-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".gentle-ai", "state.json"), []byte("{\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--agent", "opencode",
		"--action-eligibility", "--next-transition",
	}, &output); err == nil {
		t.Fatalf("STATUS mode-resolution error = %v\n%s", err, output.String())
	}
}

func TestNegotiatedStatusKeepsRDDDisabledStopWhenUntrackedSelectionIsNeeded(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "global"}, io.Discard); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--agent", "opencode",
		"--action-eligibility", "--next-transition",
	}, &output); err != nil {
		t.Fatalf("STATUS with untracked content: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatalf("decode STATUS: %v\n%s", err, output.String())
	}
	if allowed := status.Eligibility.AllowedActions[0]; allowed.Action != "stop" || allowed.ReasonCode != "forbidden_rdd_disabled" {
		t.Fatalf("disabled eligibility = %#v", status.Eligibility)
	}
	if status.NextTransition.Kind != reviewNextTransitionStop || status.NextTransition.ReasonCode != "rdd_disabled" || status.NextTransition.Collect != nil {
		t.Fatalf("disabled transition was replaced by untracked collection: %#v", status.NextTransition)
	}
}

// TestDisabledStatusStaysSilentForEveryCWDSpelling replaces the old
// narration-canonicalization test: the rdd_disabled continuation is no longer
// narrated at all, because a successful negotiated STATUS must write zero
// bytes to stderr regardless of how the caller spelled --cwd.
func TestDisabledStatusStaysSilentForEveryCWDSpelling(t *testing.T) {
	for _, cwd := range [][]string{{"--cwd=."}, {"--cwd", "."}} {
		t.Run(strings.Join(cwd, " "), func(t *testing.T) {
			reviewModeHome(t)
			repo := initReviewCLIRepo(t)
			t.Chdir(repo)
			if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "global"}, io.Discard); err != nil {
				t.Fatal(err)
			}

			var output bytes.Buffer
			stderr := captureReviewProcessStderr(t)
			args := append([]string{"status"}, cwd...)
			args = append(args, "--contract", ReviewIntegrationContractV2, "--agent", "opencode", "--next-transition")
			if err := RunReview(args, &output); err != nil {
				t.Fatalf("STATUS: %v\n%s", err, output.String())
			}
			var status ReviewTargetStatusResult
			if err := json.Unmarshal(output.Bytes(), &status); err != nil {
				t.Fatalf("decode STATUS: %v\n%s", err, output.String())
			}
			if status.NextTransition == nil || status.NextTransition.ReasonCode != "rdd_disabled" {
				t.Fatalf("disabled transition = %#v", status.NextTransition)
			}
			if got := stderr(); got != "" {
				t.Fatalf("disabled negotiated STATUS wrote stderr, want zero bytes:\n%q", got)
			}
		})
	}
}
