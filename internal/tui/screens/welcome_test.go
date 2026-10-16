package screens_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func TestWelcomeOmitsExternalPluginActions(t *testing.T) {
	for _, label := range []string{"OpenCode Community Plugins", "Uninstall OpenCode Plugin"} {
		if containsOption(screens.WelcomeOptions(nil, true, true, true), label) {
			t.Errorf("retired action %q remains", label)
		}
	}
}

// ─── WelcomeOptions ──────────────────────────────────────────────────────────

func TestWelcomeOptions_NoLegacyProfilesEntry(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, true, true)
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
	opts := screens.WelcomeOptions(nil, true, true, true)
	// Includes the Receipt-Driven Development entry.
	want := 12
	if len(opts) != want {
		t.Errorf("WelcomeOptions(hasEngines=true) = %d options, want %d; opts: %v", len(opts), want, opts)
	}
}

// TestWelcomeOptions_NoEngines_ShowsDisabledLabel verifies that when hasEngines=false,
// the agent option is labelled "(no agents)" to signal unavailability.
func TestWelcomeOptions_NoEngines_ShowsDisabledLabel(t *testing.T) {
	opts := screens.WelcomeOptions(nil, true, false, true)
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
	opts := screens.WelcomeOptions(nil, true, true, true)

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
	opts := screens.WelcomeOptions(nil, true, true, true)

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

// TestRenderWelcome_NoLegacyProfilesEntry verifies no "OpenCode SDD Profiles" in output.
func TestRenderWelcome_NoLegacyProfilesEntry(t *testing.T) {
	output := screens.RenderWelcome(0, "1.0.0", "", nil, true, true, true)
	if strings.Contains(output, "OpenCode SDD Profiles") {
		snippet := output
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		t.Errorf("RenderWelcome() should not contain 'OpenCode SDD Profiles'; output snippet: %q", snippet)
	}
}

// TestWelcomeOptions_ResetReviewStoreGitPrecondition verifies that the
// repository-scoped "Reset review store" entry renders its precondition when
// the TUI runs outside a Git worktree and stays plain inside one. In both
// cases it sits between "Manage backups" and "Receipt-Driven Development".
func TestWelcomeOptions_ResetReviewStoreGitPrecondition(t *testing.T) {
	tests := []struct {
		name  string
		inGit bool
		want  string
	}{
		{name: "outside a Git repository", inGit: false, want: "Reset review store (requires a Git repository)"},
		{name: "inside a Git repository", inGit: true, want: "Reset review store"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := screens.WelcomeOptions(nil, true, true, tt.inGit)
			reset, backups, rdd := -1, -1, -1
			for index, option := range opts {
				switch option {
				case tt.want:
					reset = index
				case "Manage backups":
					backups = index
				case "Receipt-Driven Development":
					rdd = index
				}
			}
			if reset < 0 {
				t.Fatalf("menu does not render %q: %#v", tt.want, opts)
			}
			if reset != backups+1 || reset+1 != rdd {
				t.Fatalf("reset at %d is not between backups (%d) and Receipt-Driven Development (%d)", reset, backups, rdd)
			}
			output := screens.RenderWelcome(0, "1.0.0", "", nil, true, true, tt.inGit)
			if !strings.Contains(output, tt.want) {
				t.Errorf("rendered welcome (inGit=%v) missing %q; output snippet: %q", tt.inGit, tt.want, output)
			}
		})
	}
}
