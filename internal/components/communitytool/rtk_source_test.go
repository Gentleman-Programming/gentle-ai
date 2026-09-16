package communitytool

import (
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestRTKSourceContractPinsVerifiedFacts(t *testing.T) {
	if rtkReleaseTag != "v0.49.0" {
		t.Fatalf("release tag = %q", rtkReleaseTag)
	}
	if rtkReleaseCommit != "b1c0dc00649c50fbe8930f849c800d4d6ca12091" {
		t.Fatalf("release commit = %q", rtkReleaseCommit)
	}
	if rtkTelemetryDisabled != "RTK_TELEMETRY_DISABLED=1" {
		t.Fatalf("telemetry projection = %q", rtkTelemetryDisabled)
	}
	if rtkDocumentedKillSwitch != "RTK_DISABLED=1" {
		t.Fatalf("documented kill switch = %q", rtkDocumentedKillSwitch)
	}
	if len(rtkCandidateAssets) != 5 {
		t.Fatalf("candidate assets = %d, want Windows plus four Unix assets", len(rtkCandidateAssets))
	}
	if len(rtkEnabledPlatforms) != 4 {
		t.Fatalf("enabled platforms = %v, want four non-Windows platforms", rtkEnabledPlatforms)
	}
}

func TestRTKUnixAssetsAreAdmittedForThePinnedRelease(t *testing.T) {
	if len(rtkCandidateAssets) != 5 {
		t.Fatalf("candidate assets = %d, want Windows plus four Unix assets", len(rtkCandidateAssets))
	}
	if len(rtkEnabledPlatforms) != 4 {
		t.Fatalf("enabled platforms = %v, want four non-Windows platforms", rtkEnabledPlatforms)
	}
}

func TestRTKSourceContractRejectsMutableOrIncompleteFacts(t *testing.T) {
	for _, tt := range []struct {
		name, value, want string
	}{
		{"release tag", rtkReleaseTag, "v0.49.0"},
		{"release commit", rtkReleaseCommit, "b1c0dc00649c50fbe8930f849c800d4d6ca12091"},
		{"telemetry environment", rtkTelemetryDisabled, "RTK_TELEMETRY_DISABLED=1"},
		{"kill-switch metadata", rtkDocumentedKillSwitch, "RTK_DISABLED=1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want || strings.Contains(strings.ToLower(tt.value), "latest") {
				t.Fatalf("value = %q, want immutable %q", tt.value, tt.want)
			}
		})
	}
}

func TestRTKSetupContractsAreUniqueAndExact(t *testing.T) {
	tests := []struct {
		name       string
		agent      model.AgentID
		invocation string
		path       string
	}{
		{"Claude Code", model.AgentClaudeCode, "init -g --auto-patch", "~/.claude/CLAUDE.md"},
		{"OpenCode", model.AgentOpenCode, "init -g --opencode", "~/.config/opencode/plugins/rtk.ts"},
		{"Codex CLI", model.AgentCodex, "init -g --codex", "~/.codex/AGENTS.md"},
		{"Pi", model.AgentPi, "init -g --agent pi --auto-patch", "PI_CODING_AGENT_DIR/extensions/rtk.ts"},
	}
	if len(rtkSetupContracts) != len(tests) {
		t.Fatalf("setup contracts = %d, want %d", len(rtkSetupContracts), len(tests))
	}

	byAgent := make(map[model.AgentID]rtkSetupContract, len(rtkSetupContracts))
	for _, contract := range rtkSetupContracts {
		if _, duplicate := byAgent[contract.Agent]; duplicate {
			t.Fatalf("duplicate setup contract for %q", contract.Agent)
		}
		byAgent[contract.Agent] = contract
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract, ok := byAgent[tt.agent]
			if !ok {
				t.Fatalf("missing contract for %q", tt.agent)
			}
			if contract.Invocation != tt.invocation || strings.Contains(strings.ToLower(contract.Invocation), "latest") {
				t.Fatalf("invocation = %q, want %q", contract.Invocation, tt.invocation)
			}
			if contract.UserMutationPath.Class != rtkUserMutationPathInstruction || contract.UserMutationPath.Template != tt.path {
				t.Fatalf("user mutation path = %#v, want instruction file %q", contract.UserMutationPath, tt.path)
			}
		})
	}
}

func TestRTKAdmittedAssetsPinExecutableIdentity(t *testing.T) {
	want := map[rtkPlatform]struct {
		size   int64
		digest string
	}{
		rtkPlatformDarwinARM64: {8_342_080, "055ef1cd1aa0afb96c854bddf43d24af10dcac5a6288ed44519cb1cc10b093a9"},
		rtkPlatformLinuxARM64:  {9_202_032, "4fa443857061b1226a21a8503112adba078a5c7ca3f7fd5919782afacf5566ca"},
		rtkPlatformDarwinAMD64: {9_721_224, "1a28052aa71b4d865c3346de6e20216a791ad7e011845ced078b2f70745ce983"},
		rtkPlatformLinuxAMD64:  {10_888_832, "a051b22361c7cfa36022bc3f06bb41cdc88e58a07263dc340d8bd3468c41befe"},
	}
	for _, asset := range rtkCandidateAssets {
		identity, admitted := want[asset.Platform]
		if !admitted {
			if asset.Platform == rtkPlatformWindows && (asset.ExecutableSizeBytes != 0 || asset.ExecutableSHA256 != "") {
				t.Fatalf("Windows executable identity must remain unset: %#v", asset)
			}
			continue
		}
		if asset.ExecutableSizeBytes != identity.size || asset.ExecutableSHA256 != identity.digest || len(asset.ExecutableSHA256) != 64 {
			t.Fatalf("asset %s executable identity = (%d, %q), want (%d, %q)", asset.Platform, asset.ExecutableSizeBytes, asset.ExecutableSHA256, identity.size, identity.digest)
		}
	}
}

func TestRTKCandidateAssetsRemainPinnedAndWindowsDisabled(t *testing.T) {
	if rtkChecksumsSHA256 != "a5ff3570fe196a21e09a249c1777665d6ed887630d1c7c66de1154d4340d3ad0" {
		t.Fatalf("checksums digest = %q", rtkChecksumsSHA256)
	}
	for _, asset := range rtkCandidateAssets {
		if strings.Contains(strings.ToLower(asset.URL), "latest") || asset.ChecksumSHA256 != rtkChecksumsSHA256 {
			t.Fatalf("asset is not pinned: %#v", asset)
		}
	}
	if slices.Contains(rtkEnabledPlatforms, rtkPlatformWindows) {
		t.Fatalf("Windows was admitted: %v", rtkEnabledPlatforms)
	}
}
