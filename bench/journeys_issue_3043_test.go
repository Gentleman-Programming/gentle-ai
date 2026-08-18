package main

import (
	"errors"
	"strings"
	"testing"
)

func TestIssue3043JourneyRequiresFreshZsh(t *testing.T) {
	originalLookPath := issue3043LookPath
	t.Cleanup(func() { issue3043LookPath = originalLookPath })
	issue3043LookPath = func(name string) (string, error) {
		if name != "zsh" {
			return "", errors.New("unexpected command probe")
		}
		return "", errors.New("zsh intentionally unavailable")
	}

	journey := issue3043Journeys()[0]
	var skip func(*Sandbox) string
	for _, step := range journey.Steps {
		if step.Skip != nil {
			skip = step.Skip
			break
		}
	}
	if skip == nil {
		t.Fatal("issue #3043 journey must declare a zsh availability skip")
	}
	if reason := skip(&Sandbox{}); reason == "" {
		t.Fatal("issue #3043 journey must explain why it is unsupported without zsh")
	}
	if command, err := issue3043LoginShell("true"); err == nil || command != nil {
		t.Fatalf("issue3043LoginShell() = %v, %v; want no fallback command without zsh", command, err)
	}

	result := runJourney("unused", journey)
	if result.Status != StatusUnsupported {
		t.Fatalf("status = %s (%s), want unsupported when zsh is unavailable", result.Status, result.FailureReason)
	}
	if len(result.UnsupportedSteps) != 1 || !strings.Contains(result.UnsupportedSteps[0], "zsh is unavailable") {
		t.Fatalf("unsupported steps = %#v, want the zsh availability reason", result.UnsupportedSteps)
	}
}

func TestIssue3043FreshShellRequiresConvergentManagedReadiness(t *testing.T) {
	cases := []struct {
		name   string
		output string
		wantOK bool
	}{
		{
			name: "sync and doctor agree",
			output: "issue3043:sync\nOpenCode background runtime ready: true\nOpenCode background activation status: ready\n" +
				"issue3043:doctor\n[ok]  opencode:managed_activation managed OpenCode launcher resolves at /home/.gentle-ai/bin/opencode\n",
			wantOK: true,
		},
		{
			name: "install pending is not post-apply readiness",
			output: "issue3043:sync\nOpenCode background runtime ready: false\nOpenCode background activation status: pending\n" +
				"issue3043:doctor\n[xx]  opencode:managed_activation bare opencode does not resolve to managed launcher\n",
		},
		{
			name: "sync and doctor disagree",
			output: "issue3043:sync\nOpenCode background runtime ready: true\nOpenCode background activation status: ready\n" +
				"issue3043:doctor\n[xx]  opencode:managed_activation bare opencode does not resolve to managed launcher\n",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := issue3043AssertFreshShellOutput(tt.output)
			if (err == nil) != tt.wantOK {
				t.Fatalf("issue3043AssertFreshShellOutput() error = %v, want success = %t", err, tt.wantOK)
			}
		})
	}
}
