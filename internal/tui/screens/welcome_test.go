package screens_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func TestWelcomeOmitsExternalPluginActions(t *testing.T) {
	for _, label := range []string{"OpenCode Community Plugins", "Uninstall OpenCode Plugin"} {
		if containsOption(screens.WelcomeOptions(nil, true, false, 0, true), label) {
			t.Errorf("retired action %q remains", label)
		}
	}
}

// ─── WelcomeOptions ──────────────────────────────────────────────────────────

// TestWelcomeOptions_WithoutProfiles verifies that when showProfiles is false,
// the "OpenCode SDD Profiles" option is NOT present.
func TestWelcomeOptions_WithoutProfiles(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, false, 0, true)
	for _, opt := range opts {
		if strings.Contains(opt, "OpenCode SDD Profiles") {
			t.Errorf("expected no 'OpenCode SDD Profiles' option when showProfiles=false; got: %v", opts)
			break
		}
	}
}

func TestWelcomeOptions_LegacyProfilesDoNotAddMenuEntry(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		opts := screens.WelcomeOptions(nil, true, true, count, true)
		if len(opts) != 12 || !containsOption(opts, "Configure models") {
			t.Fatalf("legacy count %d: unexpected menu: %v", count, opts)
		}
		for _, opt := range opts {
			if strings.Contains(opt, "SDD Profiles") {
				t.Fatalf("legacy count %d: profile entry remains: %v", count, opts)
			}
		}
	}
}

// TestWelcomeOptions_OptionCount_WithoutProfiles verifies 12 options when showProfiles=false
// and hasEngines=true.
func TestWelcomeOptions_OptionCount_WithoutProfiles(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, false, 0, true)
	// Includes the Receipt-Driven Development entry.
	want := 12
	if len(opts) != want {
		t.Errorf("WelcomeOptions(showProfiles=false, hasEngines=true) = %d options, want %d; opts: %v", len(opts), want, opts)
	}
}

// Legacy profiles must not alter the welcome option count.
func TestWelcomeOptions_OptionCount_WithProfiles(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true, 2, true)
	// Includes the Receipt-Driven Development entry.
	want := 12
	if len(opts) != want {
		t.Errorf("WelcomeOptions(showProfiles=true, hasEngines=true) = %d options, want %d; opts: %v", len(opts), want, opts)
	}
}

// TestWelcomeOptions_NoEngines_ShowsDisabledLabel verifies that when hasEngines=false,
// the agent option is labelled "(no agents)" to signal unavailability.
func TestWelcomeOptions_NoEngines_ShowsDisabledLabel(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, false, 0, false)
	found := false
	for _, opt := range opts {
		if strings.Contains(opt, "no agents") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'no agents' label when hasEngines=false; got: %v", opts)
	}
}

// TestWelcomeOptions_ProfilesInsertedBeforeManageBackups verifies the retained shortcuts stay adjacent.
func TestWelcomeOptions_ProfilesInsertedBeforeManageBackups(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true, 1, true)

	if opts[5] != "Create your own Agent" || opts[6] != "Manage backups" {
		t.Fatalf("retained actions are not adjacent: %v", opts)
	}
}

func containsOption(opts []string, want string) bool {
	for _, opt := range opts {
		if opt == want {
			return true
		}
	}
	return false
}

func TestWelcomeOptions_IncludesManagedUninstall(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, false, 0, true)

	found := false
	for _, opt := range opts {
		if opt == "Managed uninstall" {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected 'Managed uninstall' option; got: %v", opts)
	}
}

// ─── RenderWelcome ────────────────────────────────────────────────────────────

// TestRenderWelcome_WithoutProfiles verifies no "OpenCode SDD Profiles" in output.
func TestRenderWelcome_WithoutProfiles(t *testing.T) {
	output := screens.RenderWelcome(0, "1.0.0", "", nil, true, false, 0, true)
	if strings.Contains(output, "OpenCode SDD Profiles") {
		snippet := output
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		t.Errorf("RenderWelcome(showProfiles=false) should not contain 'OpenCode SDD Profiles'; output snippet: %q", snippet)
	}
}

// Legacy profile discovery must not show a profile menu entry.
func TestRenderWelcome_WithProfiles_ZeroCount(t *testing.T) {
	output := screens.RenderWelcome(0, "1.0.0", "", nil, true, true, 0, true)
	if strings.Contains(output, "OpenCode SDD Profiles") || !strings.Contains(output, "Configure models") {
		t.Errorf("unexpected legacy profile menu or missing model configuration")
	}
	if strings.Contains(output, "OpenCode SDD Profiles (") {
		t.Errorf("RenderWelcome(showProfiles=true, count=0) should NOT have badge")
	}
}

// Discovered legacy profiles do not produce a count badge.
func TestRenderWelcome_WithProfiles_CountTwo(t *testing.T) {
	output := screens.RenderWelcome(0, "1.0.0", "", nil, true, true, 2, true)
	if strings.Contains(output, "OpenCode SDD Profiles") {
		t.Errorf("legacy profile menu remains")
	}
}

// A single legacy profile does not produce a badge.
func TestRenderWelcome_WithProfiles_CountOne(t *testing.T) {
	output := screens.RenderWelcome(0, "1.0.0", "", nil, true, true, 1, true)
	if strings.Contains(output, "OpenCode SDD Profiles") {
		t.Errorf("legacy profile menu remains")
	}
}
