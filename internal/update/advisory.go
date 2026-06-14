package update

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// advisoryMaxBytes is the maximum number of bytes read from an advisory
// response body. A well-formed advisory.json is always tiny (< 1 KB); the
// 64 KB cap prevents a malicious or misconfigured endpoint from exhausting
// memory while still leaving an enormous margin for any realistic payload.
const advisoryMaxBytes = 64 * 1024

// advisoryURL is the endpoint for the advisory manifest JSON.
// It points to a dedicated "advisory" git tag/release whose single asset
// (advisory.json) is edited in-place without bumping the binary version.
//
// PREREQUISITE: the repo owner must create a GitHub release tagged "advisory"
// and upload advisory.json as a release asset before any advisory message will
// appear. Until then, every fetch returns 404 and is silently discarded
// (fail-open). No launch latency is added regardless.
//
// Package-level var so tests can substitute an httptest server URL.
var advisoryURL = "https://github.com/Gentleman-Programming/gentle-ai/releases/download/advisory/advisory.json"

// advisoryHTTPClient is the HTTP client used exclusively for advisory fetches.
// Timeout is 2s — intentionally shorter than the general GitHub client (5s)
// so a stale advisory endpoint never adds perceptible launch latency.
// This client is NOT shared with any other client in the package.
var advisoryHTTPClient = &http.Client{Timeout: 2 * time.Second}

// Advisory holds the decoded advisory manifest payload.
// All fields are optional; unknown fields are ignored.
type Advisory struct {
	// Message is the human-readable advisory text shown at launch.
	// An empty or absent message means "nothing to display."
	Message string `json:"message"`

	// Severity is an optional hint for how to style the message
	// (e.g. "info", "warn"). Purely cosmetic; not enforced.
	Severity string `json:"severity"`

	// URL is an optional link shown alongside the message.
	URL string `json:"url"`
}

// FetchAdvisory attempts to retrieve the advisory manifest from the dedicated
// advisory release asset. It returns the decoded Advisory and true when a
// non-empty message is present; otherwise it returns Advisory{} and false.
//
// The function is FAIL-OPEN: any network error, timeout, non-200 HTTP status,
// malformed JSON, or missing/empty message field silently returns (Advisory{}, false).
// It never blocks launch — callers must run it in a background goroutine when
// zero-added-latency is required.
func FetchAdvisory(ctx context.Context) (Advisory, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, advisoryURL, nil)
	if err != nil {
		return Advisory{}, false
	}
	req.Header.Set("User-Agent", "gentle-ai-advisory-check")

	resp, err := advisoryHTTPClient.Do(req)
	if err != nil {
		// Network error, timeout, or context cancellation — fail-open.
		return Advisory{}, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Non-200 (e.g. 404 before advisory tag is created) — fail-open.
		return Advisory{}, false
	}

	var a Advisory
	limited := io.LimitReader(resp.Body, advisoryMaxBytes)
	if err := json.NewDecoder(limited).Decode(&a); err != nil {
		// Malformed JSON or body exceeded the size cap — fail-open.
		return Advisory{}, false
	}

	if a.Message == "" {
		// Empty or absent message field — nothing to display.
		return Advisory{}, false
	}

	return a, true
}
