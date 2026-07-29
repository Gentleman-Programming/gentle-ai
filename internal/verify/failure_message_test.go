package verify

import "testing"

// TestVerificationIssuesMessageForCommandNamesTheGivenCommand proves the
// failure completion line names a concrete, runnable command instead of the
// nonexistent "repair" verb (there is no `repair` case in the CLI dispatcher).
func TestVerificationIssuesMessageForCommandNamesTheGivenCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			name:    "no command falls back to the generic failure message",
			command: "",
			want:    verificationIssuesMessage,
		},
		{
			name:    "install retry command",
			command: "gentle-ai install --agent claude-code",
			want:    "Installation completed with verification issues. Run `gentle-ai install --agent claude-code` to retry the failed checks.",
		},
		{
			name:    "sync retry command",
			command: "gentle-ai sync",
			want:    "Installation completed with verification issues. Run `gentle-ai sync` to retry the failed checks.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerificationIssuesMessageForCommand(tt.command); got != tt.want {
				t.Fatalf("VerificationIssuesMessageForCommand(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

// TestBuildReportNeverNamesTheNonexistentRepairCommand proves the generic
// failure fallback text produced by BuildReport never names a `repair`
// subcommand -- there is no such case in the CLI dispatcher, so naming it
// was a false continuation strictly worse than silence.
func TestBuildReportNeverNamesTheNonexistentRepairCommand(t *testing.T) {
	report := BuildReport([]CheckResult{{ID: "check", Status: CheckStatusFailed, Error: "boom"}})
	if report.FinalNote != verificationIssuesMessage {
		t.Fatalf("FinalNote = %q, want generic fallback %q", report.FinalNote, verificationIssuesMessage)
	}
	if containsRepair(report.FinalNote) {
		t.Fatalf("FinalNote names the nonexistent repair command: %q", report.FinalNote)
	}
}

func containsRepair(s string) bool {
	for i := 0; i+6 <= len(s); i++ {
		if s[i:i+6] == "repair" {
			return true
		}
	}
	return false
}
