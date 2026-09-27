package screens_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func TestWelcomeOmitsExternalPluginActions(t *testing.T) {
	for _, label := range []string{"OpenCode Community Plugins", "Uninstall OpenCode Plugin"} {
		if containsOption(screens.WelcomeOptions(nil, true, true), label) {
			t.Errorf("retired action %q remains", label)
		}
	}
}

// ─── WelcomeOptions ──────────────────────────────────────────────────────────

func TestWelcomeOptions_NoLegacyProfilesEntry(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true)
	if !containsOption(opts, "Configure models") {
		t.Fatalf("unexpected menu: %v", opts)
	}
	for _, opt := range opts {
		if strings.Contains(opt, "SDD Profiles") {
			t.Fatalf("legacy profile entry remains: %v", opts)
		}
	}
}

// TestWelcomeOptions_OptionCount verifies 12 options when hasEngines=true.
func TestWelcomeOptions_OptionCount(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true)
	// Includes the Receipt-Driven Development entry.
	want := 12
	if len(opts) != want {
		t.Errorf("WelcomeOptions(hasEngines=true) = %d options, want %d; opts: %v", len(opts), want, opts)
	}
}

// TestWelcomeOptions_NoEngines_ShowsDisabledLabel verifies that when hasEngines=false,
// the agent option is labelled "(no agents)" to signal unavailability.
func TestWelcomeOptions_NoEngines_ShowsDisabledLabel(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, false)
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

// TestWelcomeOptions_AgentBuilderBeforeManageBackups verifies the retained shortcuts stay adjacent.
func TestWelcomeOptions_AgentBuilderBeforeManageBackups(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true)

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

func TestWelcomeOptions_ResetReviewStoreGitPrecondition(t *testing.T) {
	inside := screens.WelcomeOptionsWithGit(nil, true, true, true)
	outside := screens.WelcomeOptionsWithGit(nil, true, true, false)
	if !containsOption(inside, "Reset review store") {
		t.Fatalf("in-repository menu lacks active reset entry: %v", inside)
	}
	if !containsOption(outside, "Reset review store (requires a Git repository)") {
		t.Fatalf("outside-repository menu lacks its precondition: %v", outside)
	}
}

func TestWelcomeOptions_IncludesManagedUninstall(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true)

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

func TestRenderWelcomeMinimumKeepsResetGuidanceAndControls(t *testing.T) {
	options := screens.WelcomeOptionsWithGit(nil, true, true, false)
	reset := -1
	for i, option := range options {
		if strings.Contains(option, "Reset review store") {
			reset = i
		}
	}
	if reset < 0 {
		t.Fatal("reset entry not found")
	}
	view := screens.RenderWelcomeWithGit(reset, "dev", "", nil, true, true, false, 4, 2, screens.WelcomeAdvisory{})
	if !strings.Contains(view, "Git") || !strings.Contains(view, "q") {
		t.Fatalf("minimum view lost Git precondition or controls:\n%s", view)
	}
	if lipgloss.Height(view) > 2 {
		t.Fatalf("minimum view exceeded height 2: %q", view)
	}
}

// ─── RenderWelcome ────────────────────────────────────────────────────────────

// TestRenderWelcome_NoLegacyProfilesEntry verifies no "OpenCode SDD Profiles" in output.
func TestRenderWelcome_NoLegacyProfilesEntry(t *testing.T) {
	output := screens.RenderWelcome(0, "1.0.0", "", nil, true, true)
	if strings.Contains(output, "OpenCode SDD Profiles") {
		snippet := output
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		t.Errorf("RenderWelcome() should not contain 'OpenCode SDD Profiles'; output snippet: %q", snippet)
	}
}
