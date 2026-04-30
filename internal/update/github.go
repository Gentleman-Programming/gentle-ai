package update

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, fmt.Errorf("build github request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gentle-ai-update-check")

	if token := resolveGitHubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return githubRelease{}, fmt.Errorf("github API request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// success — decode below
	case http.StatusForbidden:
		return githubRelease{}, fmt.Errorf("github API rate limit exceeded (HTTP 403)")
	case http.StatusNotFound:
		return githubRelease{}, fmt.Errorf("no releases found for %s/%s (HTTP 404)", owner, repo)
	default:
		return githubRelease{}, fmt.Errorf("github API returned HTTP %d for %s/%s", resp.StatusCode, owner, repo)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("decode github release: %w", err)
	}

	return release, nil
}
