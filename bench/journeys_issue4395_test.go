package main

import "testing"

// The journey writes global review mode with --scope, so the capability probe
// must reject binaries that cannot parse that exact public command shape.
func TestIssue4395JourneyRequiresReviewModeScope(t *testing.T) {
	for _, journey := range issue4395Journeys() {
		for _, step := range journey.Steps {
			if step.Name != "enable global review mode" {
				continue
			}
			if step.Requires != issue4395GlobalModeCapability {
				t.Fatalf("review-mode capability = %#v, want issue4395GlobalModeCapability", step.Requires)
			}
			for _, flag := range issue4395GlobalModeCapability.Flags {
				if flag == "--scope" {
					return
				}
			}
			t.Fatal("issue4395GlobalModeCapability does not require --scope")
		}
	}
	t.Fatal("issue #4395 journey has no global review-mode enable step")
}

func TestIssue4395GlobalModeCapabilityRejectsBinaryWithoutScope(t *testing.T) {
	sandbox := fakeBinary(t, `echo "Usage: gentle-ai review mode [--cwd <repo>] [--json]"`)
	supported, reason := newCapabilityProbe(sandbox).supported(issue4395GlobalModeCapability)
	if supported {
		t.Fatal("supported = true, want false when review mode help omits --scope")
	}
	if reason != "flag not present: review mode --scope" {
		t.Fatalf("reason = %q, want missing --scope", reason)
	}
}

func TestSharedModeCapabilityDoesNotRequireScope(t *testing.T) {
	sandbox := fakeBinary(t, `echo "Usage: gentle-ai review mode [--cwd <repo>] [--json]"`)
	supported, reason := newCapabilityProbe(sandbox).supported(modeCapability)
	if !supported {
		t.Fatalf("supported = false (%s), want true without --scope", reason)
	}
}
