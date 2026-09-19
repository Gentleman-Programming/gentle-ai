package triageevidence

import (
	"strings"
	"testing"
)

const templateBugReport = `### 📝 Bug Description

The TUI offers Reset Review Store outside a Git repository.

### 🔄 Steps to Reproduce

1. Run gentle-ai in a folder that is not a Git worktree.
2. Open the welcome menu.
3. Press Enter on "Reset review store".

### ✅ Expected Behavior

An actionable message instead of a git error.

### ❌ Actual Behavior

The raw git rev-parse exit 128 error is shown.

## 🖥️ Environment

### Gentle AI Version

gentle-ai 3.4.0 (gga v2.10.1)

### Operating System

macOS

### AI Agent / Client

Claude Code

### 📋 Affected Area

CLI (commands, flags)
`

func TestExtractEvidenceTemplateReport(t *testing.T) {
	ev := ExtractEvidence(templateBugReport)
	if ev.Version.Channel != ChannelStable || ev.Version.Major != 3 || ev.Version.Minor != 4 {
		t.Errorf("Version = %+v, want stable 3.4.0", ev.Version)
	}
	if ev.OS != "macOS" {
		t.Errorf("OS = %q, want macOS", ev.OS)
	}
	if ev.Agent != "Claude Code" {
		t.Errorf("Agent = %q, want Claude Code", ev.Agent)
	}
	if ev.AffectedArea != "CLI (commands, flags)" {
		t.Errorf("AffectedArea = %q", ev.AffectedArea)
	}
	if !ev.HasDetailedSteps {
		t.Error("HasDetailedSteps = false, want true for filled steps")
	}
	if !ev.HasReproductionEvidence() {
		t.Error("HasReproductionEvidence = false, want true")
	}
}

func TestExtractEvidenceEmptyPlaceholderSteps(t *testing.T) {
	body := "### Gentle AI Version\n\n3.4.0\n\n### 🔄 Steps to Reproduce\n\n1. \n2. \n3. \n"
	ev := ExtractEvidence(body)
	if ev.HasDetailedSteps {
		t.Error("HasDetailedSteps = true for placeholder-only steps")
	}
	if ev.OS != "" {
		t.Errorf("OS = %q, want empty", ev.OS)
	}
}

func TestExtractEvidenceSignals(t *testing.T) {
	body := "I can reproduce this consistently. Run:\n`$ gga version`\n\nexit code 1"
	ev := ExtractEvidence(body)
	if !ev.HasLogsOrCommands {
		t.Error("HasLogsOrCommands = false, want true for command + exit code")
	}
	if !ev.StatesReproducibility {
		t.Error("StatesReproducibility = false, want true for 'consistently'")
	}
	if ev.HasDetailedSteps {
		t.Error("HasDetailedSteps = true without any steps section")
	}
}

func TestExtractEvidenceUnlabeledMissingSections(t *testing.T) {
	// A minimal report without the environment block.
	ev := ExtractEvidence("### Gentle AI Version\n\nnot sure\n\n### Steps\n\nnone")
	if ev.Version.Channel != ChannelUnknown {
		t.Errorf("Channel = %v, want unknown", ev.Version.Channel)
	}
}

func TestIssueHelpers(t *testing.T) {
	i := Issue{Number: 42, Title: "fix that", Body: "see #4711", Labels: []string{"type:bug"}}
	if !i.IsBugReport() {
		t.Error("IsBugReport = false for type:bug label")
	}
	if !i.MatchesReference(4711) {
		t.Error("MatchesReference = false when body mentions #4711")
	}
	if i.MatchesReference(47) {
		t.Error("MatchesReference(47) = true; wants exact #N reference")
	}
}

func TestClassifyPrecedence(t *testing.T) {
	latest := ParseReportedVersion("v3.4.0")
	issue := Issue{Number: 1, Body: "### Gentle AI Version\n\n3.3.0\n"}
	ev := ExtractEvidence(issue.Body)

	tests := []struct {
		name string
		rel  []RelatedChange
		want Outcome
	}{
		{
			name: "released change wins over old version",
			rel:  []RelatedChange{{Kind: RelatedRelease, Title: "v3.4.0 release", URL: "https://example.com/releases/3.4.0", Version: latest, State: "v3.4.0"}},
			want: OutcomeRelatedChangeFound,
		},
		{
			name: "open PR is in flight",
			rel:  []RelatedChange{{Kind: RelatedPR, Title: "fix the thing", URL: "https://example.com/pr/9", State: "open", Number: 9}},
			want: OutcomeRelatedChangeFound,
		},
		{
			name: "merged PR is a released change",
			rel:  []RelatedChange{{Kind: RelatedPR, Title: "landed fix", URL: "https://example.com/pr/8", State: "merged", Number: 8}},
			want: OutcomeRelatedChangeFound,
		},
		{
			name: "open issue alone is only context",
			rel:  []RelatedChange{{Kind: RelatedIssue, Title: "similar report", URL: "https://example.com/issues/7", State: "open", Number: 7}},
			want: OutcomeRetestRequested,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(ClassificationInput{Issue: issue, Evidence: ev, LatestStable: latest, Related: tt.rel})
			if got.Outcome != tt.want {
				t.Errorf("Classify() = %s, want %s (reasons: %v)", got.Outcome, tt.want, got.Reasons)
			}
			if tt.want == OutcomeRelatedChangeFound && len(got.Support) == 0 {
				t.Error("related-change-found verdict carries no supporting URL")
			}
		})
	}
}

func TestClassifyRetestAndCurrent(t *testing.T) {
	latest := ParseReportedVersion("v3.4.0")

	t.Run("old stable retests", func(t *testing.T) {
		issue := Issue{Body: "### Gentle AI Version\n\n2.4.0\n\n### 🔄 Steps to Reproduce\n\n1. Do the thing.\n2. See the error.\n"}
		got := Classify(ClassificationInput{Issue: issue, Evidence: ExtractEvidence(issue.Body), LatestStable: latest})
		if got.Outcome != OutcomeRetestRequested {
			t.Fatalf("Outcome = %s, want %s", got.Outcome, OutcomeRetestRequested)
		}
	})

	t.Run("upgrade claim retests even on current", func(t *testing.T) {
		issue := Issue{Body: "### Gentle AI Version\n\n3.4.0\n\nupgrading fixed it in my case.\n"}
		got := Classify(ClassificationInput{Issue: issue, Evidence: ExtractEvidence(issue.Body), LatestStable: latest})
		if got.Outcome != OutcomeRetestRequested {
			t.Fatalf("Outcome = %s, want %s", got.Outcome, OutcomeRetestRequested)
		}
	})

	t.Run("current version with repro is current-evidence", func(t *testing.T) {
		issue := Issue{Body: "### Gentle AI Version\n\nv3.4.0\n\nReproduce it consistently:\n$ gga run\n"}
		got := Classify(ClassificationInput{Issue: issue, Evidence: ExtractEvidence(issue.Body), LatestStable: latest})
		if got.Outcome != OutcomeCurrentEvidence {
			t.Fatalf("Outcome = %s, want %s (reasons: %v)", got.Outcome, OutcomeCurrentEvidence, got.Reasons)
		}
	})

	t.Run("main build with repro is current-evidence", func(t *testing.T) {
		issue := Issue{Body: "### Gentle AI Version\n\nbuild from main\n\nEvery time I see: panic: boom\n"}
		got := Classify(ClassificationInput{Issue: issue, Evidence: ExtractEvidence(issue.Body), LatestStable: latest})
		if got.Outcome != OutcomeCurrentEvidence {
			t.Fatalf("Outcome = %s, want %s", got.Outcome, OutcomeCurrentEvidence)
		}
	})

	t.Run("version newer than latest is current-evidence", func(t *testing.T) {
		issue := Issue{Body: "### Gentle AI Version\n\n3.5.0\n\n### 🔄 Steps to Reproduce\n\n1. Install 3.5.0.\n2. Run it.\n3. See the bug.\n"}
		got := Classify(ClassificationInput{Issue: issue, Evidence: ExtractEvidence(issue.Body), LatestStable: latest})
		if got.Outcome != OutcomeCurrentEvidence {
			t.Fatalf("Outcome = %s, want %s", got.Outcome, OutcomeCurrentEvidence)
		}
	})
}

func TestClassifyInsufficientEvidence(t *testing.T) {
	latest := ParseReportedVersion("v3.4.0")

	t.Run("current version without repro", func(t *testing.T) {
		issue := Issue{Body: "### Gentle AI Version\n\n3.4.0\n\nIt is broken somehow.\n"}
		got := Classify(ClassificationInput{Issue: issue, Evidence: ExtractEvidence(issue.Body), LatestStable: latest})
		if got.Outcome != OutcomeInsufficientEvidence {
			t.Fatalf("Outcome = %s, want %s", got.Outcome, OutcomeInsufficientEvidence)
		}
		want := "no reproduction evidence"
		if !containsSubstring(got.Reasons, want) {
			t.Errorf("Reasons %v do not name %q", got.Reasons, want)
		}
	})

	t.Run("unknown version with repro", func(t *testing.T) {
		issue := Issue{Body: "### Gentle AI Version\n\nnot sure\n\n$ gga version\n"}
		got := Classify(ClassificationInput{Issue: issue, Evidence: ExtractEvidence(issue.Body), LatestStable: latest})
		if got.Outcome != OutcomeInsufficientEvidence {
			t.Fatalf("Outcome = %s, want %s", got.Outcome, OutcomeInsufficientEvidence)
		}
	})

	t.Run("no latest known and older version", func(t *testing.T) {
		issue := Issue{Body: "### Gentle AI Version\n\n2.4.0\n\n$ gga version\n"}
		got := Classify(ClassificationInput{Issue: issue, Evidence: ExtractEvidence(issue.Body), LatestStable: ReportedVersion{}})
		if got.Outcome != OutcomeCurrentEvidence {
			t.Fatalf("Outcome = %s, want current-evidence fallback when latest is unknown", got.Outcome)
		}
	})
}

func containsSubstring(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.HasPrefix(h, needle) {
			return true
		}
	}
	return false
}
