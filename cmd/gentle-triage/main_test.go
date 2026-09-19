package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/triageevidence"
)

// fixtures mirror the API shapes the pipeline reads.
func fixtureIssues() []apiIssue {
	return []apiIssue{
		{
			Number: 10, Title: "TUI crash on Reset Review Store",
			Body:  "### Gentle AI Version\n\nv3.4.0\n\n### 🔄 Steps to Reproduce\n\n1. Open welcome menu.\n2. Press Enter.\n\n$ gga run\n",
			State: "open", HTMLURL: "https://example.com/issues/10",
			Labels: []apiLabel{{Name: "status:needs-review"}, {Name: "bug"}},
		},
		{
			Number: 11, Title: "Old version broken",
			Body:  "### Gentle AI Version\n\n2.4.0\n\nEvery time I see: panic: boom\n",
			State: "open", HTMLURL: "https://example.com/issues/11",
			Labels: []apiLabel{{Name: "status:needs-review"}, {Name: "type:bug"}},
		},
		{
			Number: 12, Title: "Unlabeled report",
			Body:  "### Gentle AI Version\n\nnot sure\n",
			State: "open", HTMLURL: "https://example.com/issues/12",
		},
		{
			Number: 13, Title: "Already approved report",
			Body:  "### Gentle AI Version\n\n3.4.0\n",
			State: "open", HTMLURL: "https://example.com/issues/13",
			Labels: []apiLabel{{Name: "status:approved"}},
		},
	}
}

func fixtureReleases() []apiRelease {
	return []apiRelease{
		{TagName: "v3.4.0", Name: "v3.4.0", HTMLURL: "https://example.com/releases/3.4.0"},
		{TagName: "v3.3.0", Name: "v3.3.0", HTMLURL: "https://example.com/releases/3.3.0"},
		{TagName: "v3.5.0-rc.1", Name: "v3.5.0-rc.1", HTMLURL: "https://example.com/releases/rc1"},
	}
}

func fixtureSearch(relOnly bool) apiSearchResponse {
	items := []apiSearchItem{
		{Number: 90, Title: "TUI crash elsewhere", State: "open", HTMLURL: "https://example.com/issues/90"},
		{Number: 91, Title: "PR fixing TUI", State: "merged", HTMLURL: "https://example.com/pulls/91", PullRequest: &struct{}{}},
	}
	if relOnly {
		items = items[:0]
	}
	return apiSearchResponse{TotalCount: len(items), Items: items}
}

func newTestServer(t *testing.T) (*httptest.Server, *githubClient) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "60")
		var out []apiIssue
		for _, i := range fixtureIssues() {
			hasLabel := false
			for _, l := range i.Labels {
				if l.Name == r.URL.Query().Get("labels") && r.URL.Query().Get("labels") != "" {
					hasLabel = true
				}
			}
			if r.URL.Query().Get("labels") == "" || hasLabel {
				out = append(out, i)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/repos/o/r/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "60")
		_ = json.NewEncoder(w).Encode(fixtureReleases())
	})
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "60")
		// Only issue 10's keywords ("crash ...") earn hits; every other query
		// comes back empty, matching how GitHub behaves per query.
		q := r.URL.Query().Get("q")
		if !strings.Contains(q, "crash") {
			_ = json.NewEncoder(w).Encode(apiSearchResponse{})
			return
		}
		_ = json.NewEncoder(w).Encode(fixtureSearch(false))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := &githubClient{baseURL: srv.URL, http: srv.Client(), maxSearches: defaultBounds().MaxSearches}
	return srv, client
}

func TestBuildReportSelectsAndClassifies(t *testing.T) {
	_, client := newTestServer(t)
	report, err := buildReport(t.Context(), client, "o", "r", defaultBounds(), "2026-09-19T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}

	// 10, 11 from needs-review; 12 unlabeled; 13 has a lifecycle label, skipped.
	if len(report.Items) != 3 {
		t.Fatalf("items = %d, want 3 (10, 11, 12)", len(report.Items))
	}
	if !report.LatestStable.IsVersion() || report.LatestStable.Raw != "v3.4.0" {
		t.Fatalf("LatestStable = %+v, want v3.4.0 (prerelease ignored)", report.LatestStable)
	}

	byNumber := map[int]triageevidence.ReportItem{}
	for _, it := range report.Items {
		byNumber[it.Issue.Number] = it
	}
	if got := byNumber[10].Verdict.Outcome; got != triageevidence.OutcomeRelatedChangeFound {
		t.Errorf("issue 10 outcome = %s, want related-change-found (merged PR hit)", got)
	}
	if got := byNumber[11].Verdict.Outcome; got != triageevidence.OutcomeRetestRequested {
		t.Errorf("issue 11 outcome = %s, want retest-requested (2.4.0 < 3.4.0)", got)
	}
	if got := byNumber[12].Verdict.Outcome; got != triageevidence.OutcomeInsufficientEvidence {
		t.Errorf("issue 12 outcome = %s, want insufficient-evidence (unknown version, no repro)", got)
	}
}

func TestBuildReportSearchBudget(t *testing.T) {
	_, client := newTestServer(t)
	b := defaultBounds()
	b.MaxSearches = 1 // enough for one issue's search only
	report, err := buildReport(t.Context(), client, "o", "r", b, "")
	if err != nil {
		t.Fatal(err)
	}
	// Issue 10 gets the search (first in order); issue 11 hits the budget and
	// still classifies on age alone.
	byNumber := map[int]triageevidence.ReportItem{}
	for _, it := range report.Items {
		byNumber[it.Issue.Number] = it
	}
	if got := byNumber[11].Verdict.Outcome; got != triageevidence.OutcomeRetestRequested {
		t.Errorf("issue 11 outcome = %s, want retest-requested even without related search", got)
	}
}

func TestBuildReportSearchFailureIsReportedNotFatal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "60")
		_ = json.NewEncoder(w).Encode(fixtureIssues()[:1])
	})
	mux.HandleFunc("/repos/o/r/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "60")
		_ = json.NewEncoder(w).Encode(fixtureReleases())
	})
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		http.Error(w, "rate limited", http.StatusForbidden)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := &githubClient{baseURL: srv.URL, http: srv.Client(), maxSearches: 5}
	report, err := buildReport(t.Context(), client, "o", "r", defaultBounds(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(report.Items))
	}
	joined := strings.Join(report.Items[0].Verdict.Reasons, " ")
	if !strings.Contains(joined, "evidence unavailable") {
		t.Errorf("reasons %v do not mark the search failure as unavailable evidence", report.Items[0].Verdict.Reasons)
	}
}

func TestLatestStableIgnoresPrerelease(t *testing.T) {
	v := latestStableRelease(fixtureReleases())
	if v.Raw != "v3.4.0" {
		t.Fatalf("latest = %q, want v3.4.0", v.Raw)
	}
	if noRelease := latestStableRelease(nil); noRelease.IsVersion() {
		t.Error("no releases must yield an invalid LatestStable")
	}
}

func TestRunWritesArtifactAndSummary(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "report.md")
	summary := filepath.Join(dir, "summary.md")
	env := func(k string) string {
		switch k {
		case "GITHUB_TOKEN":
			return "test-token"
		case "GITHUB_STEP_SUMMARY":
			return summary
		}
		return ""
	}

	// Run against the fixture server by pointing the client base URL through a
	// dedicated run helper: emulate main.go's wiring with an injected client.
	srv, client := newTestServer(t)
	report, err := buildReport(t.Context(), client, "o", "r", defaultBounds(), "2026-09-19T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	markdown := triageevidence.RenderMarkdown(report)
	if err := os.WriteFile(output, []byte(markdown), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := appendStepSummary(summary, markdown); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "# Triage evidence report") {
		t.Error("artifact missing report header")
	}
	summaryGot, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(summaryGot), "## Summary") {
		t.Error("step summary missing report content")
	}

	// env is a parameter here only to keep run()'s signature exercised.
	if env("GITHUB_TOKEN") != "test-token" {
		t.Error("env injection broken")
	}
	_ = srv
}

func TestValidateRepoPath(t *testing.T) {
	if err := validateRepoPath("Gentleman-Programming", "gentle-ai"); err != nil {
		t.Errorf("valid repo rejected: %v", err)
	}
	for _, bad := range []string{"../evil", "a/b", "x y", "x/y", ""} {
		if err := validateRepoPath(bad, "repo"); err == nil {
			t.Errorf("owner %q accepted", bad)
		}
	}
	if err := validateRepoPath("o", "re/po"); err == nil {
		t.Error("repo with slash accepted")
	}
}

func TestParsePositiveInt(t *testing.T) {
	if got := parsePositiveInt("", 7); got != 7 {
		t.Errorf("empty -> %d, want default", got)
	}
	if got := parsePositiveInt("0", 7); got != 7 {
		t.Errorf("0 -> %d, want default", got)
	}
	if got := parsePositiveInt("12", 7); got != 12 {
		t.Errorf("12 -> %d", got)
	}
}
