package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/triageevidence"
)

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "gentle-triage:", err)
		os.Exit(1)
	}
}

// run is the whole CLI in one testable unit. env is injected so tests can pin
// GITHUB_TOKEN and GITHUB_STEP_SUMMARY without mutating process state.
func run(args []string, env func(string) string, stdout *os.File) error {
	fs := flag.NewFlagSet("gentle-triage", flag.ContinueOnError)
	repo := fs.String("repo", env("GITHUB_REPOSITORY"), "OWNER/REPO to analyze")
	output := fs.String("output", "triage-evidence.md", "artifact path")
	generatedAt := fs.String("generated-at", "", "pinned generation timestamp for deterministic output")
	maxIssues := fs.Int("max-issues", defaultBounds().MaxIssues, "issues kept in the report")
	maxRelated := fs.Int("max-related", defaultBounds().MaxRelated, "related candidates per issue")
	maxReleases := fs.Int("max-releases", defaultBounds().MaxReleases, "releases considered")
	maxSearches := fs.Int("max-searches", defaultBounds().MaxSearches, "total search API calls")
	maxPages := fs.Int("max-pages", defaultBounds().MaxPages, "page cap for list endpoints")
	perPage := fs.Int("per-page", defaultBounds().PerPage, "page size for list endpoints")
	if err := fs.Parse(args); err != nil {
		return err
	}

	owner, name, ok := strings.Cut(*repo, "/")
	if !ok || owner == "" || name == "" {
		return fmt.Errorf("--repo must be OWNER/REPO")
	}
	if err := validateRepoPath(owner, name); err != nil {
		return err
	}

	b := bounds{
		MaxIssues:   *maxIssues,
		MaxRelated:  *maxRelated,
		MaxReleases: *maxReleases,
		MaxSearches: *maxSearches,
		MaxPages:    *maxPages,
		PerPage:     *perPage,
	}

	client := newGitHubClient(env("GITHUB_TOKEN"), b.MaxSearches)
	report, err := buildReport(context.Background(), client, owner, name, b, *generatedAt)
	if err != nil {
		return err
	}

	markdown := triageevidence.RenderMarkdown(report)
	if err := os.WriteFile(*output, []byte(markdown), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "triage evidence report written to %s (%d issues)\n", *output, len(report.Items))
	for _, row := range triageevidence.Summarize(report.Items) {
		fmt.Fprintf(stdout, "  %s: %d\n", row.Outcome, row.Count)
	}

	if summaryPath := env("GITHUB_STEP_SUMMARY"); summaryPath != "" {
		if err := appendStepSummary(summaryPath, markdown); err != nil {
			return err
		}
	}
	return nil
}

func appendStepSummary(path, markdown string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("\n" + markdown)
	return err
}
