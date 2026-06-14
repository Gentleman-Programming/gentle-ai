package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestFetchAdvisory_ValidJSON verifies that a well-formed advisory manifest
// with a non-empty message is returned successfully.
func TestFetchAdvisory_ValidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"hi","severity":"info","url":"https://example.com"}`))
	}))
	defer srv.Close()

	// Override advisory URL for this test.
	orig := advisoryURL
	advisoryURL = srv.URL
	t.Cleanup(func() { advisoryURL = orig })

	a, ok := FetchAdvisory(context.Background())
	if !ok {
		t.Fatal("FetchAdvisory() returned ok=false, want true")
	}
	if a.Message != "hi" {
		t.Errorf("FetchAdvisory().Message = %q, want %q", a.Message, "hi")
	}
	if a.Severity != "info" {
		t.Errorf("FetchAdvisory().Severity = %q, want %q", a.Severity, "info")
	}
	if a.URL != "https://example.com" {
		t.Errorf("FetchAdvisory().URL = %q, want %q", a.URL, "https://example.com")
	}
}

// TestFetchAdvisory_Timeout verifies that a server that never responds causes
// FetchAdvisory to fail-open after the 2s client timeout, without blocking
// teardown for the duration of the stall.
func TestFetchAdvisory_Timeout(t *testing.T) {
	// unblock is closed in t.Cleanup so the handler exits immediately when the
	// test ends, allowing srv.Close() to return without waiting for the stall.
	unblock := make(chan struct{})
	t.Cleanup(func() { close(unblock) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Stall until either the client disconnects (2s timeout) or the test
		// cleanup fires — whichever comes first.
		select {
		case <-r.Context().Done():
		case <-unblock:
		}
		// Handler exits immediately; no response is sent.
	}))
	defer srv.Close()

	orig := advisoryURL
	advisoryURL = srv.URL
	t.Cleanup(func() { advisoryURL = orig })

	start := time.Now()
	a, ok := FetchAdvisory(context.Background())
	elapsed := time.Since(start)

	if ok {
		t.Error("FetchAdvisory() returned ok=true on timeout, want false (fail-open)")
	}
	if a.Message != "" {
		t.Errorf("FetchAdvisory().Message = %q on timeout, want empty", a.Message)
	}
	// Must return near the 2s client timeout, not after the server's stall duration.
	// Allow up to 4s for CI variance.
	if elapsed > 4*time.Second {
		t.Errorf("FetchAdvisory() took %v, expected to time out in ~2s", elapsed)
	}
}

// TestFetchAdvisory_HTTP500 verifies that a 500 response is treated as
// fail-open: (Advisory{}, false).
func TestFetchAdvisory_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	orig := advisoryURL
	advisoryURL = srv.URL
	t.Cleanup(func() { advisoryURL = orig })

	a, ok := FetchAdvisory(context.Background())
	if ok {
		t.Error("FetchAdvisory() returned ok=true on HTTP 500, want false")
	}
	if a.Message != "" {
		t.Errorf("FetchAdvisory().Message = %q on HTTP 500, want empty", a.Message)
	}
}

// TestFetchAdvisory_MalformedJSON verifies that invalid JSON is silently
// discarded and (Advisory{}, false) is returned.
func TestFetchAdvisory_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not valid json`))
	}))
	defer srv.Close()

	orig := advisoryURL
	advisoryURL = srv.URL
	t.Cleanup(func() { advisoryURL = orig })

	a, ok := FetchAdvisory(context.Background())
	if ok {
		t.Error("FetchAdvisory() returned ok=true on malformed JSON, want false")
	}
	if a.Message != "" {
		t.Errorf("FetchAdvisory().Message = %q on malformed JSON, want empty", a.Message)
	}
}

// TestFetchAdvisory_EmptyMessage verifies that a valid JSON payload with an
// empty or absent message field returns (Advisory{}, false) — nothing to display.
func TestFetchAdvisory_EmptyMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"severity":"info","url":"https://example.com"}`))
	}))
	defer srv.Close()

	orig := advisoryURL
	advisoryURL = srv.URL
	t.Cleanup(func() { advisoryURL = orig })

	a, ok := FetchAdvisory(context.Background())
	if ok {
		t.Error("FetchAdvisory() returned ok=true for empty message, want false")
	}
	if a.Message != "" {
		t.Errorf("FetchAdvisory().Message = %q for empty message, want empty", a.Message)
	}
}

// TestFetchAdvisory_OversizedBody verifies that a response body that exceeds
// the advisory size cap is handled gracefully: FetchAdvisory returns
// (Advisory{}, false) without panicking or propagating an error to the caller.
// A legitimate advisory.json is tiny; an oversized body is either malicious or
// a misconfiguration and must never exhaust memory.
func TestFetchAdvisory_OversizedBody(t *testing.T) {
	// Stream a body that is larger than the 64 KB cap without buffering the
	// whole thing in the test process.
	const bodySize = 128 * 1024 // 128 KB — well over the 64 KB cap

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Write a leading '{' so JSON decoding starts, then flood with garbage.
		// The decoder must stop reading at the cap, not consume the full body.
		_, _ = w.Write([]byte{'{'})
		chunk := make([]byte, 4096)
		for i := range chunk {
			chunk[i] = 'x'
		}
		written := 1
		for written < bodySize {
			n, err := w.Write(chunk)
			written += n
			if err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	orig := advisoryURL
	advisoryURL = srv.URL
	t.Cleanup(func() { advisoryURL = orig })

	a, ok := FetchAdvisory(context.Background())
	// Oversized / unparseable body must be treated as fail-open.
	if ok {
		t.Error("FetchAdvisory() returned ok=true on oversized body, want false")
	}
	if a.Message != "" {
		t.Errorf("FetchAdvisory().Message = %q on oversized body, want empty", a.Message)
	}
}

// TestFetchAdvisory_HTTP404 verifies that a 404 (advisory tag not yet created)
// returns (Advisory{}, false) silently — expected production state before the
// advisory release tag is created.
func TestFetchAdvisory_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	orig := advisoryURL
	advisoryURL = srv.URL
	t.Cleanup(func() { advisoryURL = orig })

	a, ok := FetchAdvisory(context.Background())
	if ok {
		t.Error("FetchAdvisory() returned ok=true on HTTP 404, want false")
	}
	if a.Message != "" {
		t.Errorf("FetchAdvisory().Message = %q on HTTP 404, want empty", a.Message)
	}
}
