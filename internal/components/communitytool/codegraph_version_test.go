package communitytool

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestParseCodeGraphVersion(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
		wantOK bool
	}{
		{name: "banner form printed by the upstream CLI", output: "┌  CodeGraph v0.9.3\n", want: "0.9.3", wantOK: true},
		{name: "bare semver", output: "1.4.1\n", want: "1.4.1", wantOK: true},
		{name: "v prefix", output: "v1.4.1", want: "1.4.1", wantOK: true},
		{name: "two component version", output: "codegraph 2.0", want: "2.0", wantOK: true},
		{name: "no version anywhere", output: "unknown flag: --version", want: "", wantOK: false},
		{name: "empty output", output: "", want: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseCodeGraphVersion(tt.output)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("parseCodeGraphVersion(%q) = (%q, %v), want (%q, %v)", tt.output, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestCodeGraphVersionLess(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "reported 0.9.3 predates the target contract", a: "0.9.3", b: codeGraphUpstreamVersion, want: true},
		{name: "exact contract version is not older", a: "1.4.1", b: codeGraphUpstreamVersion, want: false},
		{name: "newer patch is not older", a: "1.4.2", b: codeGraphUpstreamVersion, want: false},
		{name: "newer major is not older", a: "2.0.0", b: codeGraphUpstreamVersion, want: false},
		{name: "minor compares numerically not lexicographically", a: "1.10.0", b: codeGraphUpstreamVersion, want: false},
		{name: "older minor", a: "1.3.9", b: codeGraphUpstreamVersion, want: true},
		{name: "shorter version pads with zeroes", a: "1.4", b: codeGraphUpstreamVersion, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codeGraphVersionLess(tt.a, tt.b); got != tt.want {
				t.Fatalf("codeGraphVersionLess(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// stubCodeGraphVersion swaps the installed-version probe for the duration of a
// test so no test ever shells out to a real codegraph binary.
func stubCodeGraphVersion(t *testing.T, version string, ok bool) {
	t.Helper()
	previous := codeGraphInstalledVersion
	codeGraphInstalledVersion = func(string) (string, bool) { return version, ok }
	t.Cleanup(func() { codeGraphInstalledVersion = previous })
}

// installHomeWithTwoNativeTargets mirrors the reporter's machine: an already
// available CodeGraph CLI plus two detected native-target agents, one of which
// is not yet wired.
func installHomeWithTwoNativeTargets(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{}`)
	mustWrite(t, filepath.Join(home, ".cursor", "mcp.json"), `{}`)
	return home
}

func TestInstallDropsBlindTargetsWhenInstalledCodeGraphPredatesTheContract(t *testing.T) {
	home := installHomeWithTwoNativeTargets(t)
	stubCodeGraphVersion(t, "0.9.3", true)

	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(string, ...string) error {
		mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
		mustWrite(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers":{"codegraph":{"command":"codegraph"}}}`)
		return nil
	}), DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil }))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}

	want := []string{"codegraph install --yes"}
	if !reflect.DeepEqual(result.CommandsRun, want) {
		t.Fatalf("CommandsRun = %#v, want %#v (target ids must not be passed blind to an older CodeGraph)", result.CommandsRun, want)
	}
	if !hasManualActionContaining(result.ManualActions, "0.9.3") || !hasManualActionContaining(result.ManualActions, codeGraphUpstreamVersion) {
		t.Fatalf("ManualActions = %#v, want the version gap named", result.ManualActions)
	}
}

func TestInstallKeepsExplicitTargetsWhenInstalledCodeGraphMeetsTheContract(t *testing.T) {
	home := installHomeWithTwoNativeTargets(t)
	stubCodeGraphVersion(t, codeGraphUpstreamVersion, true)

	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(string, ...string) error {
		mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
		mustWrite(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers":{"codegraph":{"command":"codegraph"}}}`)
		return nil
	}), DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil }))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}

	want := []string{"codegraph install --target claude,cursor --location global --yes"}
	if !reflect.DeepEqual(result.CommandsRun, want) {
		t.Fatalf("CommandsRun = %#v, want %#v", result.CommandsRun, want)
	}
	if hasManualActionContaining(result.ManualActions, "older than") {
		t.Fatalf("ManualActions = %#v, want no version-gap note", result.ManualActions)
	}
}

func TestInstallKeepsExplicitTargetsWhenVersionProbeCannotDetermineAVersion(t *testing.T) {
	home := installHomeWithTwoNativeTargets(t)
	stubCodeGraphVersion(t, "", false)

	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(string, ...string) error {
		mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
		mustWrite(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers":{"codegraph":{"command":"codegraph"}}}`)
		return nil
	}), DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil }))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}

	want := []string{"codegraph install --target claude,cursor --location global --yes"}
	if !reflect.DeepEqual(result.CommandsRun, want) {
		t.Fatalf("CommandsRun = %#v, want unchanged behaviour when the probe is inconclusive", result.CommandsRun)
	}
}

func TestInstallProbesTheResolvedCLIPathNotABareName(t *testing.T) {
	home := installHomeWithTwoNativeTargets(t)
	previous := codeGraphInstalledVersion
	probed := make([]string, 0, 1)
	codeGraphInstalledVersion = func(cliPath string) (string, bool) {
		probed = append(probed, cliPath)
		return codeGraphUpstreamVersion, true
	}
	t.Cleanup(func() { codeGraphInstalledVersion = previous })

	if _, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(string, ...string) error {
		mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
		mustWrite(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers":{"codegraph":{"command":"codegraph"}}}`)
		return nil
	}), DetectorFunc(func(string) (string, error) { return "/opt/tools/codegraph", nil })); err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}

	if !reflect.DeepEqual(probed, []string{"/opt/tools/codegraph"}) {
		t.Fatalf("probed = %#v, want the detector-resolved CLI path", probed)
	}
}

func TestInstallSkipsTheVersionProbeWhenTheCLIIsNotYetInstalled(t *testing.T) {
	home := installHomeWithTwoNativeTargets(t)
	previous := codeGraphInstalledVersion
	probeCalls := 0
	codeGraphInstalledVersion = func(string) (string, bool) {
		probeCalls++
		return "0.9.3", true
	}
	t.Cleanup(func() { codeGraphInstalledVersion = previous })

	available := false
	if _, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(string, ...string) error {
		available = true
		mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
		mustWrite(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers":{"codegraph":{"command":"codegraph"}}}`)
		return nil
	}), DetectorFunc(func(string) (string, error) {
		if !available {
			return "", errors.New("codegraph not found in PATH")
		}
		return "/bin/codegraph", nil
	})); err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}

	if probeCalls != 0 {
		t.Fatalf("probe calls = %d, want 0: a freshly installed @latest CLI already meets the contract", probeCalls)
	}
}

// TestInstallCarriesTheVersionGapNoteThroughTheRollbackPath keeps the version
// gap reportable on the path where it matters most. A step that fails still
// returns the Result, so the reason and the resolving command survive the
// rollback rather than being replaced by a raw upstream error.
func TestInstallCarriesTheVersionGapNoteThroughTheRollbackPath(t *testing.T) {
	home := installHomeWithTwoNativeTargets(t)
	stubCodeGraphVersion(t, "0.9.3", true)

	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(string, ...string) error {
		return errors.New("upstream install rejected the invocation")
	}), DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil }))
	if err == nil {
		t.Fatal("InstallWithHome() error = nil, want the runner failure surfaced")
	}
	if !hasManualActionContaining(result.ManualActions, "0.9.3") {
		t.Fatalf("ManualActions = %#v, want the version gap reported on the failure path", result.ManualActions)
	}
	if !hasManualActionContaining(result.ManualActions, "@colbymchenry/codegraph@latest") {
		t.Fatalf("ManualActions = %#v, want the resolving command named", result.ManualActions)
	}
}

func hasManualActionContaining(actions []string, needle string) bool {
	for _, action := range actions {
		if strings.Contains(action, needle) {
			return true
		}
	}
	return false
}
