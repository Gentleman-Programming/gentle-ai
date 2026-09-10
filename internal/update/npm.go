package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// npmRegistryBaseURL is the base URL of the npm registry used to resolve the
// latest published version of npm-global tools. Package-level var so tests can
// point it at an httptest server.
var npmRegistryBaseURL = "https://registry.npmjs.org"

// npmLatestResponse is the subset of the npm registry /<pkg>/latest response
// that the update check needs.
type npmLatestResponse struct {
	Version string `json:"version"`
}

// fetchLatestNpmRelease resolves the latest published version of an npm package
// via a direct HTTPS call to the npm registry — deliberately NOT via an
// `npm view` subprocess, so the check does not depend on a local npm install.
// The result is adapted to the githubRelease shape so the rest of the update
// check pipeline (version normalization, comparison, result rendering) works
// unchanged for npm-global tools.
func fetchLatestNpmRelease(ctx context.Context, pkg string) (githubRelease, error) {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return githubRelease{}, fmt.Errorf("npm package name must not be empty")
	}

	// Scoped packages keep their slash: /@scope/name/latest is the canonical
	// registry endpoint for the "latest" dist-tag document.
	url := fmt.Sprintf("%s/%s/latest", strings.TrimRight(npmRegistryBaseURL, "/"), pkg)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, fmt.Errorf("build npm registry request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "gentle-ai-update-check")

	resp, err := httpClient.Do(req)
	if err != nil {
		return githubRelease{}, fmt.Errorf("npm registry request failed for %s: %w", pkg, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("npm registry returned HTTP %d for %s", resp.StatusCode, pkg)
	}

	var latest npmLatestResponse
	if err := json.NewDecoder(resp.Body).Decode(&latest); err != nil {
		return githubRelease{}, fmt.Errorf("decode npm registry response for %s: %w", pkg, err)
	}
	if strings.TrimSpace(latest.Version) == "" {
		return githubRelease{}, fmt.Errorf("npm registry response for %s has no version field", pkg)
	}

	return githubRelease{
		TagName: latest.Version,
		HTMLURL: fmt.Sprintf("https://www.npmjs.com/package/%s", pkg),
	}, nil
}
