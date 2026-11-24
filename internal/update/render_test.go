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

func TestRenderCLI_UnknownStateIsReported(t *testing.T) {
	unknown := UpdateResult{Tool: ToolInfo{Name: "gentle-ai"}, InstalledVersion: "4.6.0", Status: VersionUnknown}
	available := UpdateResult{Tool: ToolInfo{Name: "engram"}, InstalledVersion: "1.0.0", LatestVersion: "1.1.0", Status: UpdateAvailable}
	failed := UpdateResult{Tool: ToolInfo{Name: "gga"}, Status: CheckFailed, Err: fmt.Errorf("timeout")}
	for _, tc := range []struct {
		name    string
		results []UpdateResult
		want    string
	}{
		{
			name:    "unknown alone",
			results: []UpdateResult{unknown},
			want:    "  [??] gentle-ai     installed: 4.6.0       latest: -         \n\nUpdate state could not be determined for 1 tool(s).\n",
		},
		{
			name:    "unknown with other results",
			results: []UpdateResult{unknown, available, failed},
			want: "  [??] gentle-ai     installed: 4.6.0       latest: -         \n" +
				"  [UP] engram        installed: 1.0.0       latest: 1.1.0     \n" +
				"  [!!] gga           installed: -           latest: -           check failed: timeout\n\n" +
				"1 update(s) available. 1 check(s) failed.\nUpdate state could not be determined for 1 tool(s).\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := RenderCLI(tc.results), "Update Check\n============\n\n"+tc.want; got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestRenderCLI_AllUpToDateSummaryIsPreserved(t *testing.T) {
	results := []UpdateResult{{Tool: ToolInfo{Name: "gentle-ai"}, InstalledVersion: "4.6.0", LatestVersion: "4.6.0", Status: UpToDate}}
	want := "Update Check\n============\n\n  [ok] gentle-ai     installed: 4.6.0       latest: 4.6.0     \n\nAll tools are up to date!\n"
	if got := RenderCLI(results); got != want {
		t.Errorf("output = %q, want %q", got, want)
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
