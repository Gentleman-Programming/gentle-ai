package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	opencode "github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	opencodeactivation "github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

// sdkRangeConfig prepares a V2 runtime whose OpenCode package directory is
// owned by manifest and, when version is non-empty, already holds that SDK.
func sdkRangeConfig(t *testing.T, manifest, version string) (string, string) {
	t.Helper()
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.22")}, nil
	}
	oldPath := cmdLookPath
	t.Cleanup(func() { cmdLookPath = oldPath })
	cmdLookPath = func(name string) (string, error) { return filepath.Join(home, "bin", name), nil }
	config := opencode.NewAdapter().GlobalConfigDir(home)
	writeSDKRangeFile(t, filepath.Join(config, "package.json"), manifest)
	if version != "" {
		writeSDKRangeFile(t, filepath.Join(config, "node_modules", "@opencode", "plugin", "package.json"), `{"version":"`+version+`"}`)
	}
	return home, config
}

func writeSDKRangeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func backtickedCommands(message string) []string {
	parts := strings.Split(message, "`")
	var commands []string
	for i := 1; i < len(parts); i += 2 {
		commands = append(commands, parts[i])
	}
	return commands
}

// #5184: a newer SDK of the same major satisfies the 2.0.4 declarations and
// must not be reported as missing or downgraded.
func TestV2SDKPreflightAcceptsSameMajorAtOrAboveMinimum(t *testing.T) {
	for _, version := range []string{"2.0.4", "2.0.16", "2.0.22", "2.1.0"} {
		t.Run(version, func(t *testing.T) {
			home, config := sdkRangeConfig(t, `{"packageManager":"npm@10.8.0"}`, version)
			if err := (openCodePluginDependencyPreflightStep{homeDir: home}).Run(); err != nil {
				t.Fatalf("installed SDK %s refused: %v", version, err)
			}
			if proposal, err := OpenCodeSDKInstallProposal(home); proposal != nil || err != nil {
				t.Fatalf("installed SDK %s proposed an install: %+v, %v", version, proposal, err)
			}
			if data, _ := os.ReadFile(filepath.Join(config, "node_modules", "@opencode", "plugin", "package.json")); string(data) != `{"version":"`+version+`"}` {
				t.Fatalf("installed SDK changed: %s", data)
			}
		})
	}
}

func TestV2SDKPreflightRefusesOlderOrOtherMajorWithExactPin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX continuation text")
	}
	for _, tc := range []struct{ name, manifest, version, want string }{
		{"older npm", `{"packageManager":"npm@10.8.0"}`, "2.0.3", "npm install --save-exact --no-audit --no-fund @opencode/plugin@2.0.4"},
		{"V1 major npm", `{"packageManager":"npm@10.8.0"}`, "1.18.15", "npm install --save-exact --no-audit --no-fund @opencode/plugin@2.0.4"},
		{"next major npm", `{"packageManager":"npm@10.8.0"}`, "3.0.0", "npm install --save-exact --no-audit --no-fund @opencode/plugin@2.0.4"},
		{"prerelease npm", `{"packageManager":"npm@10.8.0"}`, "2.0.5-beta.1", "npm install --save-exact --no-audit --no-fund @opencode/plugin@2.0.4"},
		{"malformed npm", `{"packageManager":"npm@10.8.0"}`, "v2.0.22", "npm install --save-exact --no-audit --no-fund @opencode/plugin@2.0.4"},
		{"missing npm", `{"packageManager":"npm@10.8.0"}`, "", "npm install --save-exact --no-audit --no-fund @opencode/plugin@2.0.4"},
		{"older bun", `{"packageManager":"bun@1.2.0"}`, "2.0.3", "bun add --exact @opencode/plugin@2.0.4"},
		{"missing bun", `{"packageManager":"bun@1.2.0"}`, "", "bun add --exact @opencode/plugin@2.0.4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, _ := sdkRangeConfig(t, tc.manifest, tc.version)
			preflight := (openCodePluginDependencyPreflightStep{homeDir: home}).Run()
			_, proposal := OpenCodeSDKInstallProposal(home)
			for _, err := range []error{preflight, proposal} {
				if err == nil {
					t.Fatalf("SDK %q accepted", tc.version)
				}
				message := err.Error()
				if !strings.Contains(message, tc.want) || strings.Contains(message, "install --save --") || strings.Contains(message, "bun add @opencode") {
					t.Fatalf("refusal does not pin the minimum exactly: %v", err)
				}
				if tc.version != "" && !strings.Contains(message, "found @opencode/plugin@"+tc.version) {
					t.Fatalf("refusal hides the installed version: %v", err)
				}
				if tc.version == "" && strings.Contains(message, "found") {
					t.Fatalf("missing SDK reported as found: %v", err)
				}
				if strings.Contains(message, "ERESOLVE") {
					t.Fatalf("peer-conflict guidance without a V1 SDK: %v", err)
				}
			}
		})
	}
}

func TestV2SDKApprovedNPMInstallPinsExactly(t *testing.T) {
	args := strings.Join(openCodeSDKInstallArgs("@opencode/plugin@2.0.4", "/cfg"), " ")
	if !strings.Contains(args, "install --save-exact ") || strings.Contains(args, "--save ") {
		t.Fatalf("approved npm install leaves a range: %s", args)
	}
}

// #5208: the V1 SDK's peer-installed @opentui tree makes the pinned npm install
// stop with ERESOLVE. The refusal must name that case and give a remedy that
// neither prunes the peer tree (--legacy-peer-deps) nor forces the conflict.
func TestV2SDKRefusalNamesV1PeerConflictRemedy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX continuation text")
	}
	home, config := sdkRangeConfig(t, `{"packageManager":"npm@10.8.0"}`, "")
	writeSDKRangeFile(t, filepath.Join(config, "node_modules", "@opencode-ai", "plugin", "package.json"), `{"version":"1.18.15"}`)
	preflight := (openCodePluginDependencyPreflightStep{homeDir: home}).Run()
	_, proposal := OpenCodeSDKInstallProposal(home)
	quoted := shellQuoteOpenCodePluginPath(config)
	for _, err := range []error{preflight, proposal} {
		if err == nil {
			t.Fatal("missing SDK accepted")
		}
		message := err.Error()
		for _, text := range []string{"ERESOLVE", "@opencode-ai/plugin", "@opentui"} {
			if !strings.Contains(message, text) {
				t.Fatalf("peer-conflict refusal lacks %q: %v", text, err)
			}
		}
		commands := backtickedCommands(message)
		want := []string{
			"cd " + quoted + " && npm install --save-exact --no-audit --no-fund @opencode/plugin@2.0.4",
			"cd " + quoted + " && npm uninstall --no-audit --no-fund @opencode-ai/plugin",
		}
		if strings.Join(commands, "\n") != strings.Join(want, "\n") {
			t.Fatalf("peer-conflict commands = %q, want %q", commands, want)
		}
		for _, command := range commands {
			if strings.Contains(command, "--legacy-peer-deps") || strings.Contains(command, "--force") {
				t.Fatalf("remedy prescribes a destructive npm override: %s", command)
			}
		}
	}
}

func TestV2SDKPeerConflictHintScope(t *testing.T) {
	config := t.TempDir()
	if hint := openCodeSDKPeerConflictHint("linux", config, "npm"); hint != "" {
		t.Fatalf("hint without a V1 SDK: %s", hint)
	}
	writeSDKRangeFile(t, filepath.Join(config, "node_modules", "@opencode-ai", "plugin", "package.json"), `{"version":"1.18.15"}`)
	if hint := openCodeSDKPeerConflictHint("linux", config, "bun"); hint != "" {
		t.Fatalf("npm ERESOLVE hint for a Bun-owned directory: %s", hint)
	}
	hint := openCodeSDKPeerConflictHint("windows", config, "npm")
	if !strings.Contains(hint, "PowerShell: Set-Location -LiteralPath") || !strings.Contains(hint, "npm uninstall --no-audit --no-fund @opencode-ai/plugin") || strings.Contains(hint, "`") {
		t.Fatalf("Windows peer-conflict remedy is not a PowerShell command: %s", hint)
	}
}
