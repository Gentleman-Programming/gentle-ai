// Command gentle-triage runs the read-only triage evidence workflow locally.
//
// It fetches a bounded set of open status:needs-review issues (plus unlabeled
// open issues), classifies each report against the current release and bounded
// related searches, and renders the artifact through package triageevidence.
// It never writes to GitHub: no comments, labels, closes, edits, or creation.
//
// Usage:
//
//	GITHUB_TOKEN=... go run ./cmd/gentle-triage \
//	  --repo Gentleman-Programming/gentle-ai --output triage-evidence.md
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// bounds caps every API interaction so a run can never fan out without limit.
// The totals are still bounded by MaxPages even when a caller raises the other
// fields.
type bounds struct {
	MaxIssues   int // selected issues kept in the report
	MaxRelated  int // related candidates kept per issue
	MaxReleases int // releases considered for matching and latest-stable
	MaxSearches int // total search API calls across the whole run
	MaxPages    int // page-count cap for any list endpoint
	PerPage     int
}

func defaultBounds() bounds {
	return bounds{
		MaxIssues:   40,
		MaxRelated:  5,
		MaxReleases: 10,
		MaxSearches: 30,
		MaxPages:    10,
		PerPage:     100,
	}
}

// apiIssue mirrors the subset of the issues API the pipeline needs.
type apiIssue struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	CreatedAt string     `json:"created_at"`
	HTMLURL   string     `json:"html_url"`
	Labels    []apiLabel `json:"labels"`
}

type apiLabel struct {
	Name string `json:"name"`
}

// apiRelease mirrors the subset of the releases API the pipeline needs.
type apiRelease struct {
	TagName   string `json:"tag_name"`
	Name      string `json:"name"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	Published string `json:"published_at"`
}

// apiSearchItem mirrors search/issues results, where a pull_request object is
// present exactly for PRs.
type apiSearchItem struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	State       string    `json:"state"`
	HTMLURL     string    `json:"html_url"`
	PullRequest *struct{} `json:"pull_request"`
}

type apiSearchResponse struct {
	TotalCount int             `json:"total_count"`
	Items      []apiSearchItem `json:"items"`
}

// errRateLimited is returned instead of retrying when a listener refuses more
// work; wiring treats it as a bounded budget exhaustion, never as issue data.
var errRateLimited = fmt.Errorf("github api rate limit reached")

// githubClient is a bounded REST client for the subset of the GitHub API the
// report needs. Owner and repo are validated as path-safe tokens; every query
// parameter is URL-escaped. Issue content is never interpolated into URLs.
type githubClient struct {
	baseURL     string // e.g. https://api.github.com (tests use an httptest server)
	token       string
	http        *http.Client
	searches    int
	maxSearches int
}

func newGitHubClient(token string, maxSearches int) *githubClient {
	return &githubClient{
		baseURL:     "https://api.github.com",
		token:       token,
		http:        &http.Client{Timeout: 30 * time.Second},
		maxSearches: maxSearches,
	}
}

// searchAllowed reports whether another search API call is within budget and
// reserves it.
func (c *githubClient) searchAllowed() bool {
	if c.searches >= c.maxSearches {
		return false
	}
	c.searches++
	return true
}

// listOpenIssues pages the issues endpoint up to maxPages pages. When label is
// non-empty, only issues carrying that label are returned (GitHub server-side
// filter); the caller may still re-check labels.
func (c *githubClient) listOpenIssues(owner, repo, label string, b bounds) ([]apiIssue, error) {
	var out []apiIssue
	for page := 1; page <= b.MaxPages; page++ {
		var items []apiIssue
		if err := c.getJSON(issueListURL(c.baseURL, owner, repo, label, b.PerPage, page), &items); err != nil {
			return nil, err
		}
		out = append(out, items...)
		if len(items) < b.PerPage {
			break
		}
	}
	return out, nil
}

// listReleases pages the releases endpoint up to maxPages pages.
func (c *githubClient) listReleases(owner, repo string, b bounds) ([]apiRelease, error) {
	var out []apiRelease
	for page := 1; page <= b.MaxPages; page++ {
		var items []apiRelease
		if err := c.getJSON(releaseListURL(c.baseURL, owner, repo, b.PerPage, page), &items); err != nil {
			return nil, err
		}
		out = append(out, items...)
		if len(items) < b.PerPage {
			break
		}
	}
	return out, nil
}

// searchRelated runs one bounded search/issues query and returns at most max
// items. It refuses (without consuming budget) when the whole-run search budget
// is exhausted.
func (c *githubClient) searchRelated(owner, repo, query string, max int) ([]apiSearchItem, bool, error) {
	if !c.searchAllowed() {
		return nil, false, nil // budget exhausted: not an error, just no candidates
	}
	u := searchURL(c.baseURL, owner, repo, query, max)
	var resp apiSearchResponse
	if err := c.getJSON(u, &resp); err != nil {
		return nil, true, err
	}
	return resp.Items, true, nil
}

// getJSON issues one GET and decodes a JSON response. Non-2xx responses report
// only the status; the body is never surfaced, so an API error can never leak
// issue content into the artifact.
func (c *githubClient) getJSON(rawURL string, dst any) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gentle-ai-triage-evidence")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return errRateLimited
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("github api %s (status %d)", rawURL, resp.StatusCode)
	}
	if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining == "0" {
		return errRateLimited
	}
	return decodeJSON(resp.Body, dst)
}

func decodeJSON(r io.Reader, dst any) error {
	dec := json.NewDecoder(r)
	return dec.Decode(dst)
}

func issueListURL(base, owner, repo, label string, perPage, page int) string {
	labels := ""
	if label != "" {
		labels = "&labels=" + url.QueryEscape(label)
	}
	return fmt.Sprintf("%s/repos/%s/%s/issues?state=open&per_page=%d&page=%d%s", base, owner, repo, perPage, page, labels)
}

func releaseListURL(base, owner, repo string, perPage, page int) string {
	return fmt.Sprintf("%s/repos/%s/%s/releases?per_page=%d&page=%d", base, owner, repo, perPage, page)
}

func searchURL(base, owner, repo, query string, max int) string {
	q := "repo:" + owner + "/" + repo + " " + query + " in:title,body"
	u := fmt.Sprintf("%s/search/issues?q=%s&per_page=%d&sort=updated&order=desc", base, url.QueryEscape(q), max)
	return u
}

// validateRepoPath asserts owner and repo are path-safe tokens, so repo values
// from the environment or workflow inputs can never inject path segments.
func validateRepoPath(owner, repo string) error {
	safe := func(s string) bool {
		if s == "" {
			return false
		}
		for _, r := range s {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			default:
				return false
			}
		}
		return true
	}
	if !safe(owner) || !safe(repo) {
		return fmt.Errorf("invalid repo %s/%s: only [A-Za-z0-9_.-] allowed", owner, repo)
	}
	return nil
}

func parsePositiveInt(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return def
}
