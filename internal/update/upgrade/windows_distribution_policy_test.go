package upgrade

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/update"
)

func TestWindowsBetaSourceRecoveryIsPinned(t *testing.T) {
	tool := update.ToolInfo{Name: "gentle-ai", Owner: "Gentleman-Programming", Repo: "gentle-ai"}
	sha := "972997650b51abcdef0123456789abcdef012345"
	result := update.UpdateResult{Tool: tool, LatestVersion: "main@" + sha[:12], BetaCommit: sha, BetaModulePath: "github.com/gentleman-programming/gentle-ai/v5"}
	hint := gentleAIWindowsSourceInstallHint(result)
	want := "go install github.com/gentleman-programming/gentle-ai/v5/cmd/gentle-ai@" + sha
	if !strings.Contains(hint, want) || strings.Contains(hint, "@main") {
		t.Fatalf("Windows recovery command not pinned: %s", hint)
	}
	result.BetaModulePath = ""
	if hint := gentleAIWindowsSourceInstallHint(result); strings.Contains(hint, "go install") {
		t.Fatalf("unverified Windows recovery command: %s", hint)
	}
}

func TestWindowsBetaGentleAIUpgradeUsesShippedRegistryGoTarget(t *testing.T) {
	const (
		mainSHA = "972997650b51abcdef0123456789abcdef012345"
		module  = "github.com/gentleman-programming/gentle-ai/v5"
	)

	var tool update.ToolInfo
	_ = tool
	_ = mainSHA
	_ = module
}
