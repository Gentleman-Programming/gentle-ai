package screens_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
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

// TestWelcomeOptions_WithProfiles_ZeroCount shows "OpenCode SDD Profiles" without a badge.
func TestWelcomeOptions_WithProfiles_ZeroCount(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true, 0, true)
	found := false
	for _, opt := range opts {
		if opt == "OpenCode SDD Profiles" {
			found = true
		}
		if strings.HasPrefix(opt, "OpenCode SDD Profiles (") {
			t.Errorf("expected no badge for 0 profiles, got: %q", opt)
		}
	}
	if !found {
		t.Errorf("expected 'OpenCode SDD Profiles' option when showProfiles=true, profileCount=0; got: %v", opts)
	}
}

// TestWelcomeOptions_WithProfiles_CountTwo shows "OpenCode SDD Profiles (2)".
func TestWelcomeOptions_WithProfiles_CountTwo(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true, 2, true)
	found := false
	for _, opt := range opts {
		if opt == "OpenCode SDD Profiles (2)" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'OpenCode SDD Profiles (2)' in options; got: %v", opts)
	}
}

// TestWelcomeOptions_WithProfiles_CountOne shows "OpenCode SDD Profiles (1)".
func TestWelcomeOptions_WithProfiles_CountOne(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true, 1, true)
	found := false
	for _, opt := range opts {
		if opt == "OpenCode SDD Profiles (1)" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'OpenCode SDD Profiles (1)' in options; got: %v", opts)
	}
}

// TestWelcomeOptions_OptionCount_WithoutProfiles verifies 9 options when showProfiles=false
// and hasEngines=true (agent option visible).
func TestWelcomeOptions_OptionCount_WithoutProfiles(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, false, 0, true)
	// Expected: Start installation, Upgrade tools, Sync configs, Upgrade + Sync,
	// Configure models, Create your own Agent, OpenCode Community Plugins, Manage backups, Managed uninstall, Quit = 10
	want := 10
	if len(opts) != want {
		t.Errorf("WelcomeOptions(showProfiles=false, hasEngines=true) = %d options, want %d; opts: %v", len(opts), want, opts)
	}
}

// TestWelcomeOptions_OptionCount_WithProfiles verifies 10 options when showProfiles=true
// and hasEngines=true.
func TestWelcomeOptions_OptionCount_WithProfiles(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true, 2, true)
	// Expected: Start installation, Upgrade tools, Sync configs, Upgrade + Sync,
	// Configure models, Create your own Agent, OpenCode Community Plugins, OpenCode SDD Profiles (2), Manage backups, Managed uninstall, Quit = 11
	want := 11
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

// TestWelcomeOptions_ProfilesInsertedBeforeManageBackups verifies the ordering:
// profiles option sits between "Create your own Agent" and "Manage backups".
func TestWelcomeOptions_ProfilesInsertedBeforeManageBackups(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true, 1, true)

	agentIdx := -1
	pluginsIdx := -1
	profilesIdx := -1
	manageBackupsIdx := -1
	for i, opt := range opts {
		if strings.HasPrefix(opt, "Create your own Agent") {
			agentIdx = i
		}
		if opt == "OpenCode Community Plugins" {
			pluginsIdx = i
		}
		if strings.HasPrefix(opt, "OpenCode SDD Profiles") {
			profilesIdx = i
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
	if profilesIdx < 0 {
		t.Fatal("option 'OpenCode SDD Profiles' not found")
	}
	if manageBackupsIdx < 0 {
		t.Fatal("option 'Manage backups' not found")
	}

	if pluginsIdx != agentIdx+1 {
		t.Errorf("plugins option at index %d, expected %d (right after 'Create your own Agent' at %d)",
			pluginsIdx, agentIdx+1, agentIdx)
	}
	if profilesIdx != pluginsIdx+1 {
		t.Errorf("profiles option at index %d, expected %d (right after plugins at %d)",
			profilesIdx, pluginsIdx+1, pluginsIdx)
	}
	if manageBackupsIdx != profilesIdx+1 {
		t.Errorf("'Manage backups' at index %d, expected %d (right after profiles at %d)",
			manageBackupsIdx, profilesIdx+1, profilesIdx)
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

// TestRenderWelcome_WithProfiles_ZeroCount contains "OpenCode SDD Profiles" but no badge.
func TestRenderWelcome_WithProfiles_ZeroCount(t *testing.T) {
	output := screens.RenderWelcome(0, "1.0.0", "", nil, true, true, 0, true)
	if !strings.Contains(output, "OpenCode SDD Profiles") {
		t.Errorf("RenderWelcome(showProfiles=true, count=0) missing 'OpenCode SDD Profiles'")
	}
	if strings.Contains(output, "OpenCode SDD Profiles (") {
		t.Errorf("RenderWelcome(showProfiles=true, count=0) should NOT have badge")
	}
}

// TestRenderWelcome_WithProfiles_CountTwo contains "OpenCode SDD Profiles (2)".
func TestRenderWelcome_WithProfiles_CountTwo(t *testing.T) {
	output := screens.RenderWelcome(0, "1.0.0", "", nil, true, true, 2, true)
	if !strings.Contains(output, "OpenCode SDD Profiles (2)") {
		t.Errorf("RenderWelcome(showProfiles=true, count=2) missing 'OpenCode SDD Profiles (2)'")
	}
}

// TestRenderWelcome_WithProfiles_CountOne contains "OpenCode SDD Profiles (1)".
func TestRenderWelcome_WithProfiles_CountOne(t *testing.T) {
	output := screens.RenderWelcome(0, "1.0.0", "", nil, true, true, 1, true)
	if !strings.Contains(output, "OpenCode SDD Profiles (1)") {
		t.Errorf("RenderWelcome(showProfiles=true, count=1) missing 'OpenCode SDD Profiles (1)'")
	}
}
