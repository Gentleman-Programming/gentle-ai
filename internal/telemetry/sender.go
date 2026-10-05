package telemetry

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
)

// NewHTTPClient returns an http.Client configured with the contract's
// connect and total timeouts. Production code always uses this. Tests
// construct their own client (still through this constructor, pointed at an
// httptest.Server) so a real network request is never made.
func NewHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: ConnectTimeout}
	return &http.Client{
		Timeout: TotalTimeout,
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
		},
	}
}

// PostEvent sends payload to endpoint and reports whether the collector
// accepted it (any 2xx status). It never blocks past ctx's deadline and
// never panics; the caller is the detached `telemetry send` process, which
// derives ctx from TotalTimeout so this call cannot outlive its budget.
func PostEvent(ctx context.Context, client *http.Client, endpoint string, payload []byte) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return false, fmt.Errorf("build telemetry request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}
