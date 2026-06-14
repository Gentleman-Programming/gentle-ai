package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
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

// TestFetchAdvisory_Timeout verifies that a server that responds after 3s
// (beyond the 2s advisory timeout) causes FetchAdvisory to fail-open and
// return (Advisory{}, false) without blocking.
func TestFetchAdvisory_Timeout(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Signal that the handler was reached, then stall.
		wg.Done()
		// Sleep long enough to exceed the 2s advisory client timeout.
		// The test has its own deadline so this will be cleaned up.
		time.Sleep(10 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	orig := advisoryURL
	advisoryURL = srv.URL
	t.Cleanup(func() { advisoryURL = orig })

	start := time.Now()
	a, ok := FetchAdvisory(context.Background())
	elapsed := time.Since(start)

	// Must have received the "request reached server" signal or timed out.
	if ok {
		t.Error("FetchAdvisory() returned ok=true on timeout, want false (fail-open)")
	}
	if a.Message != "" {
		t.Errorf("FetchAdvisory().Message = %q on timeout, want empty", a.Message)
	}
	// Must return in well under the server's sleep (10s); allow up to 4s for CI variance.
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
