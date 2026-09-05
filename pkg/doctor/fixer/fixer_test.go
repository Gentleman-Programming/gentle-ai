package fixer

import (
	"testing"
)

func TestNewFixerRegistry(t *testing.T) {
	r := NewFixerRegistry()
	if r == nil {
		t.Fatal("NewFixerRegistry returned nil")
	}
}

func TestFixerRegistry_BuiltinFixers(t *testing.T) {
	r := NewFixerRegistry()

	// Check all three OS fixers are registered
	darwinFixes := r.GetAllFixesForOS("darwin")
	if len(darwinFixes) == 0 {
		t.Error("Darwin fixer should have fixes")
	}

	linuxFixes := r.GetAllFixesForOS("linux")
	if len(linuxFixes) == 0 {
		t.Error("Linux fixer should have fixes")
	}

	windowsFixes := r.GetAllFixesForOS("windows")
	if len(windowsFixes) == 0 {
		t.Error("Windows fixer should have fixes")
	}

	// Check unknown OS returns nil
	unknownFixes := r.GetAllFixesForOS("unknown")
	if unknownFixes != nil {
		t.Error("Unknown OS should return nil")
	}
}

func TestFixerRegistry_ListOS(t *testing.T) {
	r := NewFixerRegistry()
	osList := r.ListOS()

	if len(osList) != 3 {
		t.Errorf("ListOS returned %d OSes, want 3", len(osList))
	}

	found := make(map[string]bool)
	for _, os := range osList {
		found[os] = true
	}

	for _, expected := range []string{"darwin", "linux", "windows"} {
		if !found[expected] {
			t.Errorf("Missing OS: %s", expected)
		}
	}
}

func TestFixerRegistry_GetFixes(t *testing.T) {
	r := NewFixerRegistry()

	// Test getting a known fix for Darwin
	fix, ok := r.GetFixes("darwin", "brew:install")
	if !ok {
		t.Fatal("Expected to find brew:install fix for darwin")
	}
	if fix.Command == "" {
		t.Error("Fix should have a command")
	}

	// Test getting a known fix for Linux
	fix, ok = r.GetFixes("linux", "apt:install")
	if !ok {
		t.Fatal("Expected to find apt:install fix for linux")
	}
	if fix.Command == "" {
		t.Error("Fix should have a command")
	}

	// Test getting a known fix for Windows
	fix, ok = r.GetFixes("windows", "winget:install")
	if !ok {
		t.Fatal("Expected to find winget:install fix for windows")
	}
	if fix.Command == "" {
		t.Error("Fix should have a command")
	}

	// Test non-existent fix
	_, ok = r.GetFixes("darwin", "nonexistent")
	if ok {
		t.Error("Should not find nonexistent fix")
	}

	// Test wrong OS
	_, ok = r.GetFixes("windows", "brew:install")
	if ok {
		t.Error("Should not find darwin fix on windows")
	}
}

func TestDarwinFixer_CommandMap(t *testing.T) {
	r := NewFixerRegistry()
	fixes := r.GetAllFixesForOS("darwin")

	// Check required fixers exist
	requiredFixes := []string{
		"brew:install",
		"brew:upgrade",
		"brew:missing",
		"xcode:cli:missing",
		"xcode:license",
		"launchctl:load",
		"launchctl:start",
		"launchctl:stop",
		"launchctl:daemon:load",
		"softwareupdate:install",
		"softwareupdate:check",
		"defaults:write",
		"defaults:read",
		"rosetta2:install",
		"diskutil:repair",
		"ssh:config:perms",
	}

	for _, name := range requiredFixes {
		fix, ok := fixes[name]
		if !ok {
			t.Errorf("Missing required fix: %s", name)
			continue
		}
		if fix.Command == "" {
			t.Errorf("Fix %s has empty command", name)
		}
		if fix.Description == "" {
			t.Errorf("Fix %s has empty description", name)
		}
		if fix.DocsURL == "" {
			t.Errorf("Fix %s has empty docs URL", name)
		}
	}
}

func TestLinuxFixer_CommandMap(t *testing.T) {
	r := NewFixerRegistry()
	fixes := r.GetAllFixesForOS("linux")

	requiredFixes := []string{
		"apt:install",
		"apt:upgrade",
		"apt:missing",
		"apt:key:add",
		"dnf:install",
		"dnf:upgrade",
		"dnf:missing",
		"pacman:install",
		"pacman:upgrade",
		"pacman:missing",
		"zypper:install",
		"zypper:upgrade",
		"systemd:enable",
		"systemd:start",
		"systemd:restart",
		"systemd:stop",
		"systemd:disable",
		"systemd:status",
		"systemd:daemon-reload",
		"flatpak:install",
		"flatpak:update",
		"flatpak:remote-add",
		"snap:install",
		"snap:refresh",
		"sysctl:set",
		"sysctl:view",
		"ssh:config:perms",
		"journalctl:vacuum",
		"cpufreq:governor",
	}

	for _, name := range requiredFixes {
		fix, ok := fixes[name]
		if !ok {
			t.Errorf("Missing required fix: %s", name)
			continue
		}
		if fix.Command == "" {
			t.Errorf("Fix %s has empty command", name)
		}
		if fix.Description == "" {
			t.Errorf("Fix %s has empty description", name)
		}
		if fix.DocsURL == "" {
			t.Errorf("Fix %s has empty docs URL", name)
		}
	}
}

func TestWindowsFixer_CommandMap(t *testing.T) {
	r := NewFixerRegistry()
	fixes := r.GetAllFixesForOS("windows")

	requiredFixes := []string{
		"winget:install",
		"winget:upgrade",
		"winget:missing",
		"winget:source:add",
		"winget:source:update",
		"winget:list",
		"scoop:install",
		"scoop:update",
		"scoop:bucket:add",
		"scoop:missing",
		"choco:install",
		"choco:upgrade",
		"choco:missing",
		"choco:source:add",
		"powershell:policy:set",
		"powershell:policy:get",
		"task:create",
	}

	for _, name := range requiredFixes {
		fix, ok := fixes[name]
		if !ok {
			t.Errorf("Missing required fix: %s", name)
			continue
		}
		if fix.Command == "" {
			t.Errorf("Fix %s has empty command", name)
		}
		if fix.Description == "" {
			t.Errorf("Fix %s has empty description", name)
		}
		if fix.DocsURL == "" {
			t.Errorf("Fix %s has empty docs URL", name)
		}
	}
}

func TestFixResult_Fields(t *testing.T) {
	fix := FixResult{
		Command:      "brew install foo",
		Description:  "Install foo via Homebrew",
		RequiresSudo: false,
		Alternatives: []string{"brew upgrade foo", "brew reinstall foo"},
		DocsURL:      "https://docs.brew.sh",
	}

	if fix.Command != "brew install foo" {
		t.Errorf("Command = %q, want %q", fix.Command, "brew install foo")
	}
	if fix.Description != "Install foo via Homebrew" {
		t.Errorf("Description = %q, want %q", fix.Description, "Install foo via Homebrew")
	}
	if fix.RequiresSudo {
		t.Error("RequiresSudo should be false")
	}
	if len(fix.Alternatives) != 2 {
		t.Errorf("Alternatives = %d, want 2", len(fix.Alternatives))
	}
	if fix.DocsURL != "https://docs.brew.sh" {
		t.Errorf("DocsURL = %q, want %q", fix.DocsURL, "https://docs.brew.sh")
	}
}

func TestFixResult_RequiresSudo(t *testing.T) {
	// Test a fix that requires sudo
	fix := FixResult{
		Command:      "sudo apt-get install foo",
		Description:  "Install foo via apt",
		RequiresSudo: true,
	}

	if !fix.RequiresSudo {
		t.Error("RequiresSudo should be true for apt install")
	}
}

func TestFixerInterface(t *testing.T) {
	r := NewFixerRegistry()

	// Test that fixers implement the Fixer interface
	darwinFixer := r.fixers["darwin"]
	if darwinFixer == nil {
		t.Fatal("Darwin fixer not registered")
	}

	if darwinFixer.Name() != "darwin-fixer" {
		t.Errorf("Darwin fixer name = %q, want %q", darwinFixer.Name(), "darwin-fixer")
	}
	if darwinFixer.OS() != "darwin" {
		t.Errorf("Darwin fixer OS = %q, want %q", darwinFixer.OS(), "darwin")
	}

	linuxFixer := r.fixers["linux"]
	if linuxFixer == nil {
		t.Fatal("Linux fixer not registered")
	}
	if linuxFixer.OS() != "linux" {
		t.Errorf("Linux fixer OS = %q, want %q", linuxFixer.OS(), "linux")
	}

	windowsFixer := r.fixers["windows"]
	if windowsFixer == nil {
		t.Fatal("Windows fixer not registered")
	}
	if windowsFixer.OS() != "windows" {
		t.Errorf("Windows fixer OS = %q, want %q", windowsFixer.OS(), "windows")
	}
}

func TestFixer_AllFixes(t *testing.T) {
	r := NewFixerRegistry()

	darwinFixes := r.GetAllFixesForOS("darwin")
	if len(darwinFixes) < 15 {
		t.Errorf("Darwin fixer should have at least 15 fixes, got %d", len(darwinFixes))
	}

	linuxFixes := r.GetAllFixesForOS("linux")
	if len(linuxFixes) < 25 {
		t.Errorf("Linux fixer should have at least 25 fixes, got %d", len(linuxFixes))
	}

	windowsFixes := r.GetAllFixesForOS("windows")
	if len(windowsFixes) < 15 {
		t.Errorf("Windows fixer should have at least 15 fixes, got %d", len(windowsFixes))
	}
}