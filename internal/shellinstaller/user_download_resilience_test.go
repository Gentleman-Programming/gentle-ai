package shellinstaller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"syscall"
	"testing"
	"time"
)

// userDownloadFastRetry removes real backoff waits for the test's duration.
func userDownloadFastRetry(t *testing.T) {
	t.Helper()
	previousBackoff, previousIdle := userDownloadBackoff, userDownloadIdleTimeout
	userDownloadBackoff, userDownloadIdleTimeout = time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { userDownloadBackoff, userDownloadIdleTimeout = previousBackoff, previousIdle })
}

func userDownloadIdleServer(t *testing.T, chunks int, gap time.Duration, stall bool) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		for i := 0; i < chunks; i++ {
			if _, err := w.Write([]byte("chunk")); err != nil {
				return
			}
			flusher.Flush()
			time.Sleep(gap)
		}
		if stall {
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}
	}))
	t.Cleanup(func() { close(release); server.Close() })
	return server
}

func userDownloadIdleGet(t *testing.T, server *httptest.Server, idle time.Duration) ([]byte, error) {
	t.Helper()
	client := &http.Client{Transport: userDownloadIdle(http.DefaultTransport.(*http.Transport).Clone(), idle)}
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return io.ReadAll(response.Body)
}

func TestUserDownloadIdleTimeoutAllowsSlowProgressingBody(t *testing.T) {
	// Total transfer (~600ms) far exceeds the idle timeout (200ms), but every
	// gap is shorter: a whole-body deadline would abort it, an idle one must not.
	server := userDownloadIdleServer(t, 12, 50*time.Millisecond, false)
	data, err := userDownloadIdleGet(t, server, 200*time.Millisecond)
	if err != nil || len(data) != 12*len("chunk") {
		t.Fatalf("slow progressing body aborted: %d bytes, %v", len(data), err)
	}
}

func TestUserDownloadIdleTimeoutAbortsStalledBody(t *testing.T) {
	server := userDownloadIdleServer(t, 2, 0, true)
	start := time.Now()
	data, err := userDownloadIdleGet(t, server, 150*time.Millisecond)
	if !errors.Is(err, errUserDownloadIdle) || !userDownloadTransient(err) {
		t.Fatalf("stalled body did not fail with the transient idle error: %v", err)
	}
	if len(data) != 2*len("chunk") || time.Since(start) > 5*time.Second {
		t.Fatalf("stalled body effects: %d bytes after %s", len(data), time.Since(start))
	}
}

func TestUserDownloadTransientClassification(t *testing.T) {
	urlErr := func(err error) error { return &url.Error{Op: "Get", URL: "https://example.invalid", Err: err} }
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"idle", errUserDownloadIdle, true},
		{"unexpected EOF mid-body", errors.Join(io.ErrUnexpectedEOF, errors.New("digest differs")), true},
		{"connection reset", urlErr(&net.OpError{Op: "read", Err: syscall.ECONNRESET}), true},
		{"connection refused", urlErr(&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}), true},
		{"dns temporary", urlErr(&net.DNSError{Err: "server misbehaving", Name: "x", IsTemporary: true}), true},
		{"dns timeout", urlErr(&net.DNSError{Err: "timeout", Name: "x", IsTimeout: true}), true},
		{"dns not found", urlErr(&net.DNSError{Err: "no such host", Name: "x", IsNotFound: true}), false},
		{"transport deadline", urlErr(context.DeadlineExceeded), true},
		{"status 500", &userDownloadStatusError{Code: 500}, true},
		{"status 503 wrapped", privateErrorForTest(&userDownloadStatusError{Code: 503}), true},
		{"status 429", &userDownloadStatusError{Code: 429}, true},
		{"status 404", &userDownloadStatusError{Code: 404}, false},
		{"status 403", &userDownloadStatusError{Code: 403}, false},
		{"integrity", errors.New("digest/size differs"), false},
		{"canceled", urlErr(context.Canceled), false},
	}
	for _, tc := range cases {
		if got := userDownloadTransient(tc.err); got != tc.want {
			t.Errorf("%s: transient=%v, want %v", tc.name, got, tc.want)
		}
	}
}

// privateErrorForTest wraps like the platform privateError helpers do.
func privateErrorForTest(err error) error { return fmt.Errorf("acquisition: %w", err) }

func TestUserDownloadRetryBoundsAttempts(t *testing.T) {
	userDownloadFastRetry(t)
	cases := []struct {
		name  string
		fails []error
		want  int
		ok    bool
	}{
		{"success first", nil, 1, true},
		{"transient then success", []error{&userDownloadStatusError{Code: 502}}, 2, true},
		{"reset twice then success", []error{syscall.ECONNRESET, io.ErrUnexpectedEOF}, 3, true},
		{"transient exhausted", []error{errUserDownloadIdle, errUserDownloadIdle, errUserDownloadIdle, errUserDownloadIdle}, 3, false},
		{"integrity not retried", []error{errors.New("SHA256 differs")}, 1, false},
		{"404 not retried", []error{&userDownloadStatusError{Code: 404}}, 1, false},
	}
	for _, tc := range cases {
		calls := 0
		err := userDownloadRetry(context.Background(), func(context.Context) error {
			calls++
			if calls <= len(tc.fails) {
				return tc.fails[calls-1]
			}
			return nil
		})
		if calls != tc.want || (err == nil) != tc.ok {
			t.Errorf("%s: calls=%d err=%v, want calls=%d ok=%v", tc.name, calls, err, tc.want, tc.ok)
		}
	}
}

func TestUserDownloadRetryStopsOnCancellation(t *testing.T) {
	previous := userDownloadBackoff
	userDownloadBackoff = time.Hour // Only cancellation can end the wait.
	t.Cleanup(func() { userDownloadBackoff = previous })
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	done := make(chan error, 1)
	go func() {
		done <- userDownloadRetry(ctx, func(context.Context) error {
			calls++
			return &userDownloadStatusError{Code: 503}
		})
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("cancellation during backoff: calls=%d err=%v", calls, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retry backoff ignored cancellation")
	}
	if err := userDownloadRetry(ctx, func(context.Context) error { t.Fatal("attempt after cancellation"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context still attempted: %v", err)
	}
}

func TestUserDownloadGitHubAssetHost(t *testing.T) {
	for host, want := range map[string]bool{
		"release-assets.githubusercontent.com":  true,
		"objects.githubusercontent.com":         true,
		"Objects.GitHubUserContent.com":         true,
		"githubusercontent.com":                 true,
		"evilgithubusercontent.com":             false,
		"githubusercontent.com.evil.example":    false,
		"github.com":                            false,
		"release-assets.githubusercontent.com.": false,
		"":                                      false,
	} {
		if got := userDownloadGitHubAssetHost(host); got != want {
			t.Errorf("%q: accepted=%v, want %v", host, got, want)
		}
	}
}
