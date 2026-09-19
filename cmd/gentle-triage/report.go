package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/triageevidence"
)

// lifecycleLabels are the status:* lifecycle labels; issues carrying none of
// them are the "unlabeled" population the pilot also samples.
var lifecycleLabels = []string{
	"status:approved",
	"status:needs-review",
	"status:needs-design",
	"status:needs-info",
}

// buildReport fetches, classifies, and mounts the report. Every step respects
// the bounds: paged fetches, per-issue related searches behind a whole-run
// search budget, and release matching only against the already-bounded release
// list. Failures degrade into report-level warnings and per-issue unavailable
// evidence — never into an aborted run with no artifact (see docs).
func buildReport(ctx context.Context, client *githubClient, owner, repo string, b bounds, generatedAt string) (triageevidence.Report, error) {
	if err := ctx.Err(); err != nil {
		return triageevidence.Report{}, err
	}

	selected, fetchErr := selectIssues(ctx, client, owner, repo, b)
	report := triageevidence.Report{
		Repository:  owner + "/" + repo,
		GeneratedAt: generatedAt,
	}
	if fetchErr != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("issues fetch failed: %v; the report covers the %d issues that were readable", fetchErr, len(selected)))
	}

	releases, err := client.listReleases(owner, repo, b)
	if err != nil {
		// A missing release list leaves LatestStable unknown and release
		// matching off, but the issue evidence still reports.
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("release fetch failed: %v; latest-stable and release-match evidence unavailable", err))
		releases = nil
	}
	report.LatestStable = latestStableRelease(releases)

	for _, i := range selected {
		item := triageevidence.ReportItem{Issue: i}
		item.Evidence = triageevidence.ExtractEvidence(i.Body)

		related, searchUnavailable, relatedErr := searchRelatedFor(ctx, client, owner, repo, i, releases, b)
		item.Verdict = triageevidence.Classify(triageevidence.ClassificationInput{
			Issue:        i,
			Evidence:     item.Evidence,
			LatestStable: report.LatestStable,
			Related:      related,
		})
		if searchUnavailable {
			item.Verdict.Reasons = append(item.Verdict.Reasons,
				"related evidence unavailable: bounded search budget exhausted")
		} else if relatedErr != nil {
			item.Verdict.Reasons = append(item.Verdict.Reasons,
				"related search failed: evidence unavailable (API error)")
		}
		report.Items = append(report.Items, item)
	}
	return report, nil
}

// selectIssues fills the report with the bounded needs-review population first,
// then open issues carrying no lifecycle label at all. A failed list returns
// the readable half plus the error; buildReport turns it into a warning so a
// degraded run still publishes a partial report.
func selectIssues(ctx context.Context, client *githubClient, owner, repo string, b bounds) ([]triageevidence.Issue, error) {
	var out []triageevidence.Issue
	add := func(issues []apiIssue) {
		for _, a := range issues {
			if a.PullRequest != nil {
				continue
			}
			if len(out) >= b.MaxIssues {
				return
			}
			out = append(out, toIssue(a))
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	needsReview, err := client.listOpenIssues(owner, repo, "status:needs-review", b)
	if err != nil {
		return out, err
	}
	add(needsReview)
	if len(out) >= b.MaxIssues {
		return out, nil
	}

	everyone, err := client.listOpenIssues(owner, repo, "", b)
	if err != nil {
		return out, err
	}
	for _, a := range everyone {
		if len(out) >= b.MaxIssues {
			break
		}
		if a.PullRequest != nil || hasLifecycleLabel(a) {
			continue
		}
		out = append(out, toIssue(a))
	}
	return out, nil
}

func hasLifecycleLabel(a apiIssue) bool {
	for _, l := range a.Labels {
		for _, ll := range lifecycleLabels {
			if l.Name == ll {
				return true
			}
		}
	}
	return false
}

func toIssue(a apiIssue) triageevidence.Issue {
	labels := make([]string, 0, len(a.Labels))
	for _, l := range a.Labels {
		labels = append(labels, l.Name)
	}
	return triageevidence.Issue{
		Number:    a.Number,
		Title:     a.Title,
		Body:      a.Body,
		Labels:    labels,
		CreatedAt: a.CreatedAt,
		HTMLURL:   a.HTMLURL,
	}
}

// latestStableRelease picks the newest stable tag over the bounded release
// list, ignoring prereleases. With no stable release it returns the zero
// ReportedVersion (IsVersion()==false), which classification treats as
// "latest unknown — reported, never assumed".
func latestStableRelease(releases []apiRelease) triageevidence.ReportedVersion {
	var latest triageevidence.ReportedVersion
	for _, r := range releases {
		v := triageevidence.ParseReportedVersion(r.TagName)
		if v.Channel != triageevidence.ChannelStable {
			continue
		}
		if !latest.IsVersion() || v.Compare(latest) > 0 {
			latest = v
		}
	}
	return latest
}

// searchRelatedFor assembles the bounded related candidates for one issue:
// one search API call with deterministic keywords, plus release matches from
// the already-bounded release list. Search call budget exhaustion is not an
// error; an API failure is (it is reported, then classification still runs).
func searchRelatedFor(
	ctx context.Context,
	client *githubClient,
	owner, repo string,
	issue triageevidence.Issue,
	releases []apiRelease,
	b bounds,
) ([]triageevidence.RelatedChange, bool, error) {
	var related []triageevidence.RelatedChange
	keywords := triageevidence.Keywords(issue.Title, "", 5, 4)
	if len(keywords) == 0 {
		return related, false, nil
	}
	items, called, err := client.searchRelated(owner, repo, strings.Join(keywords, " "), b.MaxRelated)
	if err != nil {
		return related, false, err
	}
	if !called {
		return related, true, nil
	}
	for _, item := range items {
		if item.Number == issue.Number {
			continue // a search hit cannot be the report itself
		}
		kind := triageevidence.RelatedIssue
		state := item.State
		if item.PullRequest != nil {
			kind = triageevidence.RelatedPR
			// The search/issues API keeps merged PRs at state "closed" and
			// records the merge in merged_at. Reading merged_at is the only
			// faithful way to know a PR landed; state alone cannot say.
			if item.PullRequest.MergedAt != "" {
				state = "merged"
			}
		}
		related = append(related, triageevidence.RelatedChange{
			Kind:   kind,
			Number: item.Number,
			Title:  item.Title,
			URL:    item.HTMLURL,
			State:  state,
		})
		if len(related) >= b.MaxRelated {
			break
		}
	}
	// Release matches come from the bounded list: no extra API call.
	matched := 0
	for _, r := range releases {
		if matched >= 2 || len(related) >= b.MaxRelated {
			break
		}
		blob := strings.ToLower(r.TagName + " " + r.Name + " " + r.Body)
		hit := false
		for _, k := range keywords {
			if strings.Contains(blob, k) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		related = append(related, triageevidence.RelatedChange{
			Kind:    triageevidence.RelatedRelease,
			Title:   r.Name,
			URL:     r.HTMLURL,
			State:   r.TagName,
			Version: triageevidence.ParseReportedVersion(r.TagName),
		})
		matched++
	}
	sort.SliceStable(related, func(i, j int) bool {
		if related[i].Kind != related[j].Kind {
			return related[i].Kind < related[j].Kind
		}
		return related[i].Number < related[j].Number
	})
	return related, false, nil
}
