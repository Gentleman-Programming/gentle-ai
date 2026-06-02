package update

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// httpClient is the HTTP client used for GitHub API calls.
// Package-level var for testability (swap in tests via t.Cleanup).
var httpClient = &http.Client{Timeout: 5 * time.Second}

// ghLookPath is exec.LookPath for "gh". Package-level for testability.
var ghLookPath = exec.LookPath

// githubRelease represents the subset of GitHub's release response we need.
type githubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// resolveGitHubToken returns a GitHub token for API auth, trying in order:
// 1. GITHUB_TOKEN env var
// 2. GH_TOKEN env var (gh CLI convention)
// 3. `gh auth token` CLI output (if gh is available)
// Returns empty string if neither is available.
func resolveGitHubToken() string {
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		return token
	}
	if token := strings.TrimSpace(os.Getenv("GH_TOKEN")); token != "" {
		return token
	}
	if ghPath, err := ghLookPath("gh"); err == nil {
		var out bytes.Buffer
		cmd := exec.Command(ghPath, "auth", "token")
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			if token := strings.TrimSpace(out.String()); token != "" {
				return token
			}
		}
	}
	return ""
}

// fetchLatestRelease fetches the latest release from a GitHub repository.
// Supports optional GITHUB_TOKEN/GH_TOKEN env vars or `gh auth token` to avoid rate limits.
func fetchLatestRelease(ctx context.Context, owner, repo string) (githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	resp, err := doGitHubRequest(ctx, url)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()

	if err := checkGitHubResponse(resp, owner, repo); err != nil {
		return githubRelease{}, err
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("decode github release: %w", err)
	}

	return release, nil
}

// fetchLatestReleaseMatchingPattern fetches releases and returns the newest non-draft,
// non-prerelease release whose tag matches tagPattern. Use this for repositories with
// multiple release channels where GitHub's /latest endpoint can point at the wrong channel.
func fetchLatestReleaseMatchingPattern(ctx context.Context, owner, repo, tagPattern string) (githubRelease, error) {
	pattern, err := regexp.Compile(tagPattern)
	if err != nil {
		return githubRelease{}, fmt.Errorf("compile release tag pattern for %s/%s: %w", owner, repo, err)
	}

	releasesURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=100", owner, repo)
	seenPages := make(map[string]struct{})
	for releasesURL != "" {
		if _, seen := seenPages[releasesURL]; seen {
			return githubRelease{}, fmt.Errorf("github releases pagination loop detected for %s/%s", owner, repo)
		}
		seenPages[releasesURL] = struct{}{}

		resp, err := doGitHubRequest(ctx, releasesURL)
		if err != nil {
			return githubRelease{}, err
		}

		body, readErr := readSuccessfulGitHubBody(resp, owner, repo, "github releases")
		if readErr != nil {
			return githubRelease{}, readErr
		}

		releases, err := decodeGitHubReleases(body)
		if err != nil {
			return githubRelease{}, err
		}
		for _, release := range releases {
			if release.Draft || release.Prerelease {
				continue
			}
			if pattern.MatchString(strings.TrimSpace(release.TagName)) {
				return release, nil
			}
		}
		releasesURL = nextGitHubPage(resp.Header.Get("Link"))
	}
	return githubRelease{}, fmt.Errorf("no release matching %q found for %s/%s", tagPattern, owner, repo)
}

func readSuccessfulGitHubBody(resp *http.Response, owner, repo, bodyName string) ([]byte, error) {
	defer resp.Body.Close()
	if err := checkGitHubResponse(resp, owner, repo); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", bodyName, err)
	}
	return body, nil
}

func decodeGitHubReleases(body []byte) ([]githubRelease, error) {
	var releases []githubRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		var release githubRelease
		if singleErr := json.Unmarshal(body, &release); singleErr != nil {
			return nil, fmt.Errorf("decode github releases: %w", err)
		}
		releases = []githubRelease{release}
	}
	return releases, nil
}

func nextGitHubPage(linkHeader string) string {
	for _, part := range strings.Split(linkHeader, ",") {
		sections := strings.Split(part, ";")
		if len(sections) < 2 {
			continue
		}
		hasNextRel := false
		for _, section := range sections[1:] {
			if strings.TrimSpace(section) == `rel="next"` {
				hasNextRel = true
				break
			}
		}
		if !hasNextRel {
			continue
		}
		rawURL := strings.Trim(strings.TrimSpace(sections[0]), "<>")
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		return parsed.String()
	}
	return ""
}

func doGitHubRequest(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build github request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gentle-ai-update-check")

	if token := resolveGitHubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github API request failed: %w", err)
	}
	return resp, nil
}

func checkGitHubResponse(resp *http.Response, owner, repo string) error {
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusForbidden:
		return fmt.Errorf("github API rate limit exceeded (HTTP 403)")
	case http.StatusNotFound:
		return fmt.Errorf("no releases found for %s/%s (HTTP 404)", owner, repo)
	default:
		return fmt.Errorf("github API returned HTTP %d for %s/%s", resp.StatusCode, owner, repo)
	}
}
