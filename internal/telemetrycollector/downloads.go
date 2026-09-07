package telemetrycollector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// Rollup metrics for external, public download counts — not telemetry:
// these come from npmjs.org and the GitHub API, not from any gentle-ai
// install. They live in rollups_daily purely so the dashboard and
// /v1/summary can read them alongside the collector's own metrics without
// a second store.
const (
	metricNpmDownloadsDay        = "npm_downloads_day"
	metricGithubReleaseDownloads = "github_release_downloads_total"
)

// npmDownloadsURL and githubReleasesURL are overridden by tests to point
// at an httptest server instead of the real APIs.
var (
	npmDownloadsURL = func(pkg string) string {
		return "https://api.npmjs.org/downloads/point/last-day/" + url.PathEscape(pkg)
	}
	githubReleasesURL = func(repo string) string {
		return "https://api.github.com/repos/" + repo + "/releases?per_page=20"
	}
)

// DownloadsConfig names the external packages and repositories to fetch
// public download counts for.
type DownloadsConfig struct {
	NpmPackages []string
	GithubRepos []string // "owner/repo"
	GithubToken string   // optional; raises the GitHub API rate limit
}

// FetchAndStoreDownloads fetches today's npm and GitHub release download
// counts and stores them into rollups_daily. It never returns an error: a
// failure fetching one package or repo is logged once and simply leaves
// that day's row unwritten, to be retried on the next run — it never
// affects event ingest, and one failing source never blocks the others.
func FetchAndStoreDownloads(ctx context.Context, s *Storage, client *http.Client, cfg DownloadsConfig, now time.Time, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}

	// npmjs.org's "point/last-day" endpoint reports the day before the
	// request (today's day is still incomplete), so that is the day this
	// count is stored under.
	npmDay := truncateToDay(now.UTC()).AddDate(0, 0, -1).Format(dayLayout)
	for _, pkg := range cfg.NpmPackages {
		count, err := fetchNpmDownloads(ctx, client, pkg)
		if err != nil {
			logger.Error("npm downloads fetch failed", "package", pkg, "error", err)
			continue
		}
		if err := upsertRollup(ctx, s.db, npmDay, metricNpmDownloadsDay, pkg, count); err != nil {
			logger.Error("npm downloads store failed", "package", pkg, "error", err)
		}
	}

	// GitHub's release asset counts are live cumulative totals, not tied
	// to a prior day, so the snapshot is stored under today.
	githubDay := truncateToDay(now.UTC()).Format(dayLayout)
	for _, repo := range cfg.GithubRepos {
		totals, err := fetchGithubReleaseDownloads(ctx, client, repo, cfg.GithubToken)
		if err != nil {
			logger.Error("github release downloads fetch failed", "repo", repo, "error", err)
			continue
		}
		for tag, count := range totals {
			if err := upsertRollup(ctx, s.db, githubDay, metricGithubReleaseDownloads, tag, count); err != nil {
				logger.Error("github release downloads store failed", "repo", repo, "tag", tag, "error", err)
			}
		}
	}
}

type npmDownloadsResponse struct {
	Downloads int64  `json:"downloads"`
	Error     string `json:"error"`
}

func fetchNpmDownloads(ctx context.Context, client *http.Client, pkg string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, npmDownloadsURL(pkg), nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var parsed npmDownloadsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, fmt.Errorf("decode: %w", err)
	}
	if parsed.Error != "" {
		return 0, fmt.Errorf("npm registry: %s", parsed.Error)
	}
	return parsed.Downloads, nil
}

type githubAsset struct {
	DownloadCount int64 `json:"download_count"`
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

// fetchGithubReleaseDownloads returns, for each release of repo, the sum
// of its assets' download_count, keyed by tag name.
func fetchGithubReleaseDownloads(ctx context.Context, client *http.Client, repo, token string) (map[string]int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesURL(repo), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	totals := make(map[string]int64, len(releases))
	for _, r := range releases {
		var sum int64
		for _, a := range r.Assets {
			sum += a.DownloadCount
		}
		totals[r.TagName] = sum
	}
	return totals, nil
}

// latestRollupPerKey returns, for each distinct key stored under metric on
// or before throughDay, the value from that key's most recent day. Used
// for "current" external snapshots (download counts) where summing across
// days would be meaningless — unlike the collector's own daily metrics,
// these are not per-day deltas.
func (s *Storage) latestRollupPerKey(ctx context.Context, metric, throughDay string) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.key, r.value
		FROM rollups_daily r
		INNER JOIN (
			SELECT key, MAX(day) AS max_day FROM rollups_daily WHERE metric = ? AND day <= ? GROUP BY key
		) latest ON latest.key = r.key AND latest.max_day = r.day
		WHERE r.metric = ?
	`, metric, throughDay, metric)
	if err != nil {
		return nil, fmt.Errorf("query latest rollup for %s: %w", metric, err)
	}
	defer rows.Close()

	out := map[string]int64{}
	for rows.Next() {
		var key string
		var value int64
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan latest rollup row: %w", err)
		}
		out[key] = value
	}
	return out, rows.Err()
}
