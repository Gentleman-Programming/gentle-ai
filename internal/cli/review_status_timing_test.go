package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

func TestReviewStatusPhaseTimingsEnabledDetection(t *testing.T) {
	for _, tc := range []struct {
		args []string
		env  string
		want bool
	}{
		{nil, "", false},
		{[]string{"--phase-timings"}, "", true},
		{[]string{"--phase-timings=true"}, "", true},
		{[]string{"--phase-timings=false"}, "", false},
		{nil, "1", true},
		{nil, "true", true},
		{nil, "0", false},
		{[]string{"--phase-timings=false", "--phase-timings"}, "", true},
		{[]string{"--phase-timings", "--phase-timings=false"}, "", false},
		{[]string{"--phase-timings=false", "--phase-timings=true"}, "", true},
		{[]string{"--phase-timings=true", "--phase-timings=false"}, "", false},
		{[]string{"--phase-timings=false"}, "true", false},
		{[]string{"--phase-timings", "--", "--phase-timings=false"}, "", true},
		{[]string{"--phase-timings=false", "--", "--phase-timings"}, "", false},
	} {
		t.Setenv(reviewStatusPhaseTimingsEnvVar, tc.env)
		if got := reviewStatusPhaseTimingsEnabled(tc.args); got != tc.want {
			t.Errorf("args=%v env=%q got %v, want %v", tc.args, tc.env, got, tc.want)
		}
	}
}

func TestRunReviewStatusPhaseTimingsOptInAndSilence(t *testing.T) {
	repo := initReviewCLIRepo(t)
	var stderrBuffer bytes.Buffer
	originalWriter := reviewStatusPhaseTimingsWriter
	reviewStatusPhaseTimingsWriter = &stderrBuffer
	t.Cleanup(func() { reviewStatusPhaseTimingsWriter = originalWriter })

	var stdout bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo}, &stdout); err != nil {
		t.Fatalf("RunReview status: %v", err)
	}
	if stderrBuffer.Len() != 0 {
		t.Fatalf("expected silent stderr, got: %q", stderrBuffer.String())
	}

	stderrBuffer.Reset()
	stdout.Reset()
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--phase-timings", "--phase-timings=false"}, &stdout); err != nil {
		t.Fatalf("RunReview status: %v", err)
	}
	if stderrBuffer.Len() != 0 {
		t.Fatalf("expected silent stderr with trailing --phase-timings=false, got: %q", stderrBuffer.String())
	}

	stderrBuffer.Reset()
	stdout.Reset()
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--phase-timings=false", "--phase-timings"}, &stdout); err != nil {
		t.Fatalf("RunReview status: %v", err)
	}
	out := stderrBuffer.String()
	for _, phase := range []string{"repository_identity_resolution=", "snapshot_construction=", "implicit_base_range_derivation=", "predecessor_consumption_ancestry="} {
		if !strings.Contains(out, phase) {
			t.Errorf("missing phase %q in %s", phase, out)
		}
	}

	stderrBuffer.Reset()
	stdout.Reset()
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--phase-timings"}, &stdout); err != nil {
		t.Fatalf("RunReview status: %v", err)
	}
	out = stderrBuffer.String()
	for _, phase := range []string{"repository_identity_resolution=", "snapshot_construction=", "implicit_base_range_derivation=", "predecessor_consumption_ancestry="} {
		if !strings.Contains(out, phase) {
			t.Errorf("missing phase %q in %s", phase, out)
		}
	}
}

func TestRunReviewStatusTimeoutIncludesPhaseTimingsInCause(t *testing.T) {
	originalRunner, originalTimeout, originalWriter := reviewFacadeCommandRunner, reviewFacadeOperationTimeout, reviewStatusPhaseTimingsWriter
	var stderrBuffer bytes.Buffer
	reviewStatusPhaseTimingsWriter = &stderrBuffer
	t.Cleanup(func() {
		reviewFacadeCommandRunner, reviewFacadeOperationTimeout, reviewStatusPhaseTimingsWriter = originalRunner, originalTimeout, originalWriter
	})

	reviewFacadeOperationTimeout = 25 * time.Millisecond
	reviewFacadeCommandRunner = func(ctx context.Context, _ []string, _ io.Writer) error {
		defer reviewtransaction.StartStatusPhase(ctx, reviewtransaction.StatusPhasePredecessorConsumptionAncestry)()
		select {
		case <-time.After(100 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	var stdout bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--phase-timings"}, &stdout); err == nil {
		t.Fatal("expected timeout error")
	}
	f := decodeReviewIntegrationFailure(t, stdout.Bytes())
	if f.Code != "operation_timeout" || f.Phase != "pre_native" || !strings.Contains(f.Cause, "predecessor_consumption_ancestry=") {
		t.Fatalf("unexpected failure: %+v", f)
	}
	if !strings.Contains(stderrBuffer.String(), "status_phase_timings:") {
		t.Fatalf("missing stderr timings: %s", stderrBuffer.String())
	}
}
