package update

import (
	"fmt"
	"strings"
	"testing"
)

func TestRenderCLI_IncompleteCheckDoesNotClaimUpToDate(t *testing.T) {
	results := []UpdateResult{
		{Tool: ToolInfo{Name: "gentle-ai"}, InstalledVersion: "1.0.0", LatestVersion: "1.0.0", Status: UpToDate},
		{Tool: ToolInfo{Name: "engram"}, Status: CheckFailed, Err: fmt.Errorf("timeout")},
	}

	out := RenderCLI(results)

	if strings.Contains(out, "All tools are up to date!") {
		t.Fatalf("RenderCLI must not claim all tools are up to date when checks fail:\n%s", out)
	}
	if !strings.Contains(out, "Update check incomplete") {
		t.Fatalf("RenderCLI must mention incomplete checks:\n%s", out)
	}
	if !strings.Contains(out, "check failed") {
		t.Fatalf("RenderCLI must surface failed rows:\n%s", out)
	}
}

func TestCheckFailures(t *testing.T) {
	results := []UpdateResult{
		{Tool: ToolInfo{Name: "gentle-ai"}, Status: UpToDate},
		{Tool: ToolInfo{Name: "engram"}, Status: CheckFailed},
		{Tool: ToolInfo{Name: "gga"}, Status: CheckFailed},
	}

	failed := CheckFailures(results)
	if len(failed) != 2 {
		t.Fatalf("len(CheckFailures) = %d, want 2", len(failed))
	}
	if failed[0] != "engram" || failed[1] != "gga" {
		t.Fatalf("CheckFailures() = %v, want [engram gga]", failed)
	}
	if !HasCheckFailures(results) {
		t.Fatalf("HasCheckFailures() = false, want true")
	}
}
