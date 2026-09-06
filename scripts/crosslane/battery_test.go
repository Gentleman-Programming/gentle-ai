package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiReviewEnvironmentAllowlistAndLocators(t *testing.T) {
	operatorHome := t.TempDir()
	const operatorPath = "/operator/bin"
	for _, tt := range []struct{ name, home, path, locator, wantDir, wantErr string }{
		{"explicit locator", operatorHome, operatorPath, "/operator/pi-agent", "/operator/pi-agent", ""},
		{"empty locator falls back", operatorHome, operatorPath, "", filepath.Join(operatorHome, ".pi", "agent"), ""},
		{"empty HOME fails closed", "", operatorPath, "/operator/pi-agent", "", "operator HOME is empty; refusing to launch the Pi relay without an explicit runtime locator"},
		{"empty PATH fails closed", operatorHome, "", "/operator/pi-agent", "", "operator PATH is empty; refusing to launch the Pi relay without an explicit runtime locator"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PI_CODING_AGENT_DIR", tt.locator)
			t.Setenv("API_KEY", "api-key-sentinel")
			t.Setenv("GENTLE_SENTINEL", "gentle-sentinel")
			env, err := piReviewEnvironment(tt.home, tt.path)
			if err != tt.wantErr || err != "" && env != nil {
				t.Fatalf("environment/error = %q/%q, want %q/nil", env, err, tt.wantErr)
			}
			if err != "" {
				return
			}
			got := strings.Join(env, "\n")
			want := strings.Join([]string{"HOME=" + tt.home, "PATH=" + tt.path, "PI_CODING_AGENT_DIR=" + tt.wantDir, "GENTLE_PI_REVIEW_RELAY_CONTRACT=gentle-pi.review-relay/v1"}, "\n")
			if got != want || strings.Contains(got, "api-key-sentinel") || strings.Contains(got, "gentle-sentinel") {
				t.Fatalf("environment = %q, want exact allowlist %q without sentinels", got, want)
			}
		})
	}
}

func TestPiReviewerSlotsAdvanceAcrossUnchangedStatus(t *testing.T) {
	inputs := []any{}
	for order, lens := range []string{"review-risk", "review-resilience", "review-readability", "review-reliability"} {
		inputs = append(inputs, map[string]any{"capture_operation": "review.capture-result", "arguments": []any{map[string]any{"name": "order", "value": fmt.Sprint(order)}, map[string]any{"name": "lens", "value": lens}, map[string]any{"name": "subject-hash", "value": "subject-" + fmt.Sprint(order)}}})
	}
	status := map[string]any{"next_transition": map[string]any{"collect": map[string]any{"inputs": inputs}}}
	seen := map[string]bool{}
	for want := 0; want < len(inputs); want++ {
		input, slot := piUnadmittedReviewerInput(status, seen)
		if got := argumentValues(input)["order"]; got != fmt.Sprint(want) {
			t.Fatalf("selected order = %q, want %d", got, want)
		}
		if want == 0 {
			if _, retry := piUnadmittedReviewerInput(status, seen); retry != slot {
				t.Fatalf("unadmitted slot = %q, want unchanged first slot %q", retry, slot)
			}
		}
		seen[slot] = true // An admitted capture, not selection, consumes this provider slot.
	}
}

func TestHostPiLaneRefusesRejectedEnvironmentBeforeLaunch(t *testing.T) {
	b := &battery{piEnvironmentErr: "operator PATH is empty; refusing to launch the Pi relay without an explicit runtime locator"}
	b.runHostPiLane()
	if len(b.checks) != 1 || b.checks[0] != (check{hostPiLane, "Pi runtime environment", statusFail, b.piEnvironmentErr}) {
		t.Fatalf("rejected Pi environment checks = %#v", b.checks)
	}
}

func TestPiLastEventClosureAdmitsAndReentersCorrection(t *testing.T) {
	if os.Getenv("GO_WANT_PI_RELAY_STATUS") == "1" {
		want := []string{os.Args[0], "review", "status", "--lineage=provider-lineage", "--cursor=provider-owned"}
		if strings.Join(os.Args, "\x00") != strings.Join(want, "\x00") {
			fmt.Fprintf(os.Stderr, "status argv = %q, want %q", os.Args, want)
			os.Exit(2)
		}
		fmt.Print(`{"next_transition":{"reason_code":"correction_plan_required"}}`)
		os.Exit(0)
	}
	t.Setenv("GO_WANT_PI_RELAY_STATUS", "1")
	closureJSON := `{"schema":"gentle-ai.review-last-event-closure/v1","state":"correction_required","lineage_id":"provider-lineage","status_continuation":{"operation":"review.status","arguments":[{"token":"--lineage=provider-lineage"},{"token":"--cursor=provider-owned"}]}}`
	closure := (&battery{}).record("result-artifact", []byte(closureJSON))
	b := &battery{binary: os.Args[0]}
	if !admittedCapture(closure) || !b.hostCorrectionReentry("test", "lifecycle correction re-entry", t.TempDir(), nil, closure) {
		t.Fatalf("closure/re-entry = %#v/%#v", closure, b.checks)
	}
}
func TestHostPiCorrectionCandidateExecutesAuthorizationBypass(t *testing.T) {
	if testing.Short() {
		t.Skip("executes the Node built-in authorization test in the scratch repository")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is unavailable: " + err.Error())
	}
	b := &battery{workRoot: t.TempDir()}
	repo, ok := b.hostPiCorrectionCandidate()
	if !ok || runHostPiScratchTest(repo) == nil {
		t.Fatalf("authorization-bypass candidate = %q/%t; checks = %#v", repo, ok, b.checks)
	}
	if err := writeFile(repo, "auth/authorize.mjs", hostPiCorrectedModule); err != nil || runHostPiScratchTest(repo) != nil {
		t.Fatalf("corrected authorization expectations did not pass: %v", err)
	}
}
func TestCommittedMediumCandidateFailsWhenBaseWriteFails(t *testing.T) {
	battery := &battery{
		workRoot: t.TempDir(),
		lineages: map[string]lineageScope{},
	}

	repo, baseTree, ok := battery.committedMediumCandidate(
		"test", "invalid-base-write", ".", "base", "candidate",
	)
	if ok || repo != "" || baseTree != "" {
		t.Fatalf("committed candidate after base write failure = repo %q, base %q, ok %t", repo, baseTree, ok)
	}
	if len(battery.checks) != 1 {
		t.Fatalf("failure checks = %#v, want exactly one committed-process failure", battery.checks)
	}
	failure := battery.checks[0]
	if failure.Lane != "test" || failure.Name != "committed process base" || failure.Status != statusFail {
		t.Fatalf("base write failure = %#v, want committed process base FAIL", failure)
	}
}
