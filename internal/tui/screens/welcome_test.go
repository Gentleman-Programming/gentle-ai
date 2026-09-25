package screens_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/tui/screens"
)

// ─── WelcomeOptions ──────────────────────────────────────────────────────────

// TestWelcomeOptions_WithoutProfiles verifies that when showProfiles is false,
// the "OpenCode SDD Profiles" option is NOT present.
func TestWelcomeOptions_WithoutProfiles(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, false, 0, true)
	if !containsOption(opts, "OpenCode Community Plugins") {
		t.Fatalf("expected dedicated OpenCode Community Plugins option; got: %v", opts)
	}
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
		if len(opts) != 14 || !containsOption(opts, "Configure models") {
			t.Fatalf("legacy count %d: unexpected menu: %v", count, opts)
		}
		for _, opt := range opts {
			if strings.Contains(opt, "SDD Profiles") {
				t.Fatalf("legacy count %d: profile entry remains: %v", count, opts)
			}
		}
	}
}

// TestWelcomeOptions_OptionCount_WithoutProfiles verifies 14 options when showProfiles=false
// and hasEngines=true.
func TestWelcomeOptions_OptionCount_WithoutProfiles(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, false, 0, true)
	// Includes the Receipt-Driven Development entry.
	want := 14
	if len(opts) != want {
		t.Errorf("WelcomeOptions(showProfiles=false, hasEngines=true) = %d options, want %d; opts: %v", len(opts), want, opts)
	}
}

// Legacy profiles must not alter the welcome option count.
func TestWelcomeOptions_OptionCount_WithProfiles(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true, 2, true)
	// Includes the Receipt-Driven Development entry.
	want := 14
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

	agentIdx := -1
	pluginsIdx := -1
	uninstallIdx := -1
	manageBackupsIdx := -1
	for i, opt := range opts {
		if strings.HasPrefix(opt, "Create your own Agent") {
			agentIdx = i
		}
		if opt == "OpenCode Community Plugins" {
			pluginsIdx = i
		}
		if opt == "Uninstall OpenCode Plugin" {
			uninstallIdx = i
		}
		if opt == "Manage backups" {
			manageBackupsIdx = i
		}
	}

	if agentIdx < 0 {
		t.Fatal("option 'Create your own Agent' not found")
	}
	if pluginsIdx < 0 {
		t.Fatal("option 'OpenCode Community Plugins' not found")
	}
	if uninstallIdx < 0 {
		t.Fatal("option 'Uninstall OpenCode Plugin' not found")
	}
	if manageBackupsIdx < 0 {
		t.Fatal("option 'Manage backups' not found")
	}

	if pluginsIdx != agentIdx+1 {
		t.Errorf("plugins option at index %d, expected %d (right after 'Create your own Agent' at %d)",
			pluginsIdx, agentIdx+1, agentIdx)
	}
	if uninstallIdx != pluginsIdx+1 {
		t.Errorf("'Uninstall OpenCode Plugin' at index %d, expected %d (right after plugins at %d)",
			uninstallIdx, pluginsIdx+1, pluginsIdx)
	}
	if manageBackupsIdx != uninstallIdx+1 {
		t.Errorf("'Manage backups' at index %d, expected %d (right after uninstall at %d)",
			manageBackupsIdx, uninstallIdx+1, uninstallIdx)
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
