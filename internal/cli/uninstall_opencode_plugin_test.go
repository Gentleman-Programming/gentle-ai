package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/opencodeplugin"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// ─── ParseUninstallOpenCodePluginFlags ──────────────────────────────────────

func TestParseUninstallOpenCodePluginFlagsAcceptsValidID(t *testing.T) {
	flags, err := ParseUninstallOpenCodePluginFlags([]string{"sub-agent-statusline", "--yes"})
	if err != nil {
		t.Fatalf("ParseUninstallOpenCodePluginFlags() unexpected error: %v", err)
	}
	if flags.PluginID != model.OpenCodePluginSubAgentStatusline {
		t.Fatalf("PluginID = %q, want %q", flags.PluginID, model.OpenCodePluginSubAgentStatusline)
	}
	if !flags.Yes {
		t.Fatal("Yes = false, want true")
	}
}

func TestParseUninstallOpenCodePluginFlagsAcceptsShortYesFlag(t *testing.T) {
	flags, err := ParseUninstallOpenCodePluginFlags([]string{"sdd-engram-plugin", "-y"})
	if err != nil {
		t.Fatalf("ParseUninstallOpenCodePluginFlags(-y) unexpected error: %v", err)
	}
	if flags.PluginID != model.OpenCodePluginSDDEngramManage {
		t.Fatalf("PluginID = %q, want %q", flags.PluginID, model.OpenCodePluginSDDEngramManage)
	}
	if !flags.Yes {
		t.Fatal("Yes = false after -y flag")
	}
}

func TestParseUninstallOpenCodePluginFlagsAcceptsGentleLogo(t *testing.T) {
	flags, err := ParseUninstallOpenCodePluginFlags([]string{"gentle-logo"})
	if err != nil {
		t.Fatalf("ParseUninstallOpenCodePluginFlags(gentle-logo) unexpected error: %v", err)
	}
	if flags.PluginID != model.OpenCodePluginGentleLogo {
		t.Fatalf("PluginID = %q, want %q", flags.PluginID, model.OpenCodePluginGentleLogo)
	}
}

func TestParseUninstallOpenCodePluginFlagsRejectsUnknownID(t *testing.T) {
	_, err := ParseUninstallOpenCodePluginFlags([]string{"not-a-real-plugin", "--yes"})
	if err == nil {
		t.Fatal("expected error for unknown plugin id")
	}
	if !strings.Contains(err.Error(), "not-a-real-plugin") {
		t.Fatalf("error %q should mention the invalid id", err)
	}
	for _, valid := range []string{
		"sub-agent-statusline",
		"sdd-engram-plugin",
		"gentle-logo",
	} {
		if !strings.Contains(err.Error(), valid) {
			t.Fatalf("error %q should mention valid id %q", err, valid)
		}
	}
}

func TestParseUninstallOpenCodePluginFlagsRequiresPositional(t *testing.T) {
	if _, err := ParseUninstallOpenCodePluginFlags(nil); err == nil {
		t.Fatal("expected error when no positional id given")
	}
	if _, err := ParseUninstallOpenCodePluginFlags([]string{"--yes"}); err == nil {
		t.Fatal("expected error when only flag given")
	}
	if _, err := ParseUninstallOpenCodePluginFlags([]string{"a", "b"}); err == nil {
		t.Fatal("expected error when extra positional given")
	}
}

// ─── RenderUninstallOpenCodePluginReport ────────────────────────────────────

func TestRenderUninstallOpenCodePluginReportSurfacesLayers(t *testing.T) {
	out := RenderUninstallOpenCodePluginReport(opencodeplugin.UninstallResult{
		PluginID:           model.OpenCodePluginSubAgentStatusline,
		ChangedTUI:         true,
		ChangedPackageJSON: true,
		ChangedNodeModules: true,
		CacheEntryRemoved:  "/home/me/.cache/opencode/packages/opencode-subagent-statusline@latest",
		NodeModulesPath:    "/home/me/.config/opencode/node_modules/opencode-subagent-statusline",
	})
	for _, want := range []string{
		"Sub-agent Statusline",
		"Layer 1",
		"Layer 2",
		"Layer 3",
		"Layer 4",
		"opencode-subagent-statusline",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q; got:\n%s", want, out)
		}
	}
}

func TestRenderUninstallOpenCodePluginReportSurfacesTSXPath(t *testing.T) {
	out := RenderUninstallOpenCodePluginReport(opencodeplugin.UninstallResult{
		PluginID: model.OpenCodePluginGentleLogo,
		TSXPath:  "/home/me/.config/opencode/tui-plugins/gentle-logo.tsx",
	})
	if !strings.Contains(out, "gentle-logo.tsx") {
		t.Fatalf("report missing TSX path; got:\n%s", out)
	}
}

// ─── runUninstallOpenCodePluginWithInput ────────────────────────────────────

func TestRunUninstallOpenCodePluginYesExecutesAndReports(t *testing.T) {
	home := t.TempDir()
	orig := osUserHomeDir
	defer func() { osUserHomeDir = orig }()
	osUserHomeDir = func() (string, error) { return home, nil }

	var stdout bytes.Buffer
	result, err := runUninstallOpenCodePluginWithInput([]string{"sdd-engram-plugin", "--yes"}, &stdout, strings.NewReader(""))
	if err != nil {
		t.Fatalf("runUninstallOpenCodePluginWithInput() error = %v", err)
	}
	if result.PluginID != model.OpenCodePluginSDDEngramManage {
		t.Fatalf("result.PluginID = %q, want %q", result.PluginID, model.OpenCodePluginSDDEngramManage)
	}
	rendered := stdout.String()
	if !strings.Contains(rendered, "SDD Engram Manager") {
		t.Fatalf("stdout missing plugin name; got:\n%s", rendered)
	}
}

func TestRunUninstallOpenCodePluginRejectsInvalidIDBeforePrompt(t *testing.T) {
	home := t.TempDir()
	orig := osUserHomeDir
	defer func() { osUserHomeDir = orig }()
	osUserHomeDir = func() (string, error) { return home, nil }

	var stdout bytes.Buffer
	_, err := runUninstallOpenCodePluginWithInput([]string{"bogus"}, &stdout, strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for invalid plugin id")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("error %q should mention invalid id", err)
	}
}

func TestRunUninstallOpenCodePluginCancelOnNoResponse(t *testing.T) {
	home := t.TempDir()
	orig := osUserHomeDir
	defer func() { osUserHomeDir = orig }()
	osUserHomeDir = func() (string, error) { return home, nil }

	var stdout bytes.Buffer
	result, err := runUninstallOpenCodePluginWithInput([]string{"sdd-engram-plugin"}, &stdout, eofReader{})
	if err != nil {
		t.Fatalf("runUninstallOpenCodePluginWithInput() error = %v, want nil for cancelled run", err)
	}
	if result.PluginID != "" {
		t.Fatalf("expected empty result.PluginID after cancel; got %q", result.PluginID)
	}
	if !strings.Contains(stdout.String(), "uninstall cancelled") {
		t.Fatalf("stdout missing cancel message; got:\n%s", stdout.String())
	}
}

func TestRunUninstallOpenCodePluginConfirmRejectsWrongAnswer(t *testing.T) {
	home := t.TempDir()
	orig := osUserHomeDir
	defer func() { osUserHomeDir = orig }()
	osUserHomeDir = func() (string, error) { return home, nil }

	var stdout bytes.Buffer
	result, err := runUninstallOpenCodePluginWithInput([]string{"sub-agent-statusline"}, &stdout, strings.NewReader("nope\n"))
	if err != nil {
		t.Fatalf("runUninstallOpenCodePluginWithInput() error = %v, want nil when user declines", err)
	}
	if result.PluginID != "" {
		t.Fatalf("expected empty result.PluginID after decline; got %q", result.PluginID)
	}
	if !strings.Contains(stdout.String(), "uninstall cancelled") {
		t.Fatalf("stdout missing cancel message; got:\n%s", stdout.String())
	}
}

func TestRunUninstallOpenCodePluginWrapsHomeDirError(t *testing.T) {
	orig := osUserHomeDir
	defer func() { osUserHomeDir = orig }()
	osUserHomeDir = func() (string, error) { return "", errHomeDir }

	var stdout bytes.Buffer
	_, err := runUninstallOpenCodePluginWithInput([]string{"sub-agent-statusline", "--yes"}, &stdout, strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error when home dir cannot be resolved")
	}
	if !strings.Contains(err.Error(), "uninstall opencode plugin") {
		t.Fatalf("error %q should mention 'uninstall opencode plugin'", err)
	}
	if !strings.Contains(err.Error(), errHomeDir.Error()) {
		t.Fatalf("error %q should wrap underlying home dir error", err)
	}
}

var errHomeDir = ioErrHome("no home")

type ioErrHome string

func (e ioErrHome) Error() string { return string(e) }

// eofReader returns EOF immediately so the confirmation prompt fails fast
// without blocking on stdin during the cancelled test.
type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }