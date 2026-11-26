package shellinstaller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Supplier downloads are bounded by connect/TLS/response-header timeouts on
// the transport, an inactivity timeout on the body, and a generous overall
// ceiling. A slow transfer that keeps making progress is never aborted by a
// whole-body deadline; a stalled one is. SHA/size pins stay the only integrity
// authority: nothing here accepts bytes that a caller did not verify.
const (
	userDownloadCeiling  = 30 * time.Minute
	userDownloadAttempts = 3
)

var (
	// userDownloadIdleTimeout is the longest a response body may go without
	// delivering a byte. Tests shorten it.
	userDownloadIdleTimeout = 60 * time.Second
	// userDownloadBackoff is the base retry delay; attempt n waits
	// base*2^(n-1) plus up to base/2 of jitter. Tests shorten it.
	userDownloadBackoff = time.Second
)

// errUserDownloadIdle reports a body that made no progress for the idle
// timeout. It is a timeout and therefore transient.
var errUserDownloadIdle = errors.New("download stalled: no data received within the inactivity timeout")

// userDownloadStatusError reports a non-200 supplier status. Only 5xx and 429
// are transient; every other status is final.
type userDownloadStatusError struct{ Code int }

func (e *userDownloadStatusError) Error() string {
	return fmt.Sprintf("supplier responded with HTTP status %d", e.Code)
}

// userDownloadStatus returns nil for 200 and a typed status error otherwise.
func userDownloadStatus(code int) error {
	if code == http.StatusOK {
		return nil
	}
	return &userDownloadStatusError{Code: code}
}

// userDownloadIdleTransport cancels a request whose response body stops
// delivering bytes for idle. Every Read that returns data restarts the timer.
type userDownloadIdleTransport struct {
	base http.RoundTripper
	idle time.Duration
}

// userDownloadIdle wraps base so every response body carries the inactivity
// timeout.
func userDownloadIdle(base http.RoundTripper, idle time.Duration) http.RoundTripper {
	return &userDownloadIdleTransport{base: base, idle: idle}
}

func (t *userDownloadIdleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	response, err := t.base.RoundTrip(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	response.Body = userDownloadIdleBody(response.Body, t.idle, cancel)
	return response, nil
}

// CloseIdleConnections forwards to the base transport so http.Client keeps
// releasing pooled connections.
func (t *userDownloadIdleTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

type userDownloadIdleReader struct {
	body    io.ReadCloser
	idle    time.Duration
	cancel  context.CancelFunc
	timer   *time.Timer
	mu      sync.Mutex
	expired bool
}

// userDownloadIdleBody returns body wrapped with an inactivity timeout: when no
// Read delivers data for idle, cancel aborts the in-flight request and the
// pending Read fails with errUserDownloadIdle. Close stops the timer and
// releases cancel.
func userDownloadIdleBody(body io.ReadCloser, idle time.Duration, cancel context.CancelFunc) io.ReadCloser {
	reader := &userDownloadIdleReader{body: body, idle: idle, cancel: cancel}
	reader.timer = time.AfterFunc(idle, func() {
		reader.mu.Lock()
		reader.expired = true
		reader.mu.Unlock()
		cancel()
	})
	return reader
}

func (r *userDownloadIdleReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	r.mu.Lock()
	expired := r.expired
	r.mu.Unlock()
	if expired {
		return n, errors.Join(errUserDownloadIdle, err)
	}
	if n > 0 {
		r.timer.Reset(r.idle)
	}
	return n, err
}

func (r *userDownloadIdleReader) Close() error {
	r.timer.Stop()
	err := r.body.Close()
	r.cancel()
	return err
}

// userDownloadGitHubAssetHost accepts the hosts GitHub serves release assets
// from. The SHA/size pins, not the host, authenticate the bytes; this only
// keeps a CDN host rename under githubusercontent.com from breaking installs.
func userDownloadGitHubAssetHost(host string) bool {
	host = strings.ToLower(host)
	return host == "githubusercontent.com" || strings.HasSuffix(host, ".githubusercontent.com")
}

// Windows Winsock codes, which syscall does not map onto ECONNRESET and
// friends.
const (
	userWSAECONNABORTED syscall.Errno = 10053
	userWSAECONNRESET   syscall.Errno = 10054
	userWSAETIMEDOUT    syscall.Errno = 10060
	userWSAECONNREFUSED syscall.Errno = 10061
)

// userDownloadTransient reports whether err is a transient network failure
// worth another attempt: timeouts (including a stalled body), connection
// reset/refused/aborted, a connection dropped mid-body, temporary DNS errors,
// and HTTP 5xx/429. Integrity mismatches, other statuses and cancellation are
// final.
func userDownloadTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errUserDownloadIdle) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return false // The caller's own cancellation is final.
	}
	if errors.Is(err, context.DeadlineExceeded) {
		// Transport timeouts (dial, TLS, response headers) report a deadline;
		// the caller's own ceiling is caught by ctx.Err() in userDownloadRetry.
		return true
	}
	var status *userDownloadStatusError
	if errors.As(err, &status) {
		return status.Code >= 500 || status.Code == http.StatusTooManyRequests
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return dns.IsTemporary || dns.IsTimeout
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ECONNRESET, syscall.ECONNREFUSED, syscall.ECONNABORTED, syscall.ETIMEDOUT:
			return true
		}
		if runtime.GOOS == "windows" {
			switch errno {
			case userWSAECONNABORTED, userWSAECONNRESET, userWSAETIMEDOUT, userWSAECONNREFUSED:
				return true
			}
		}
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// userDownloadRetry runs attempt up to userDownloadAttempts times, retrying
// only transient failures with exponential backoff and jitter. Cancellation
// of ctx stops both the backoff wait and further attempts.
func userDownloadRetry(ctx context.Context, attempt func(context.Context) error) error {
	var err error
	for n := 0; n < userDownloadAttempts; n++ {
		if n > 0 {
			delay := userDownloadBackoff << (n - 1)
			if half := int64(userDownloadBackoff / 2); half > 0 {
				delay += time.Duration(rand.Int64N(half))
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return errors.Join(err, ctx.Err())
			case <-timer.C:
			}
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return errors.Join(err, ctxErr)
		}
		err = attempt(ctx)
		if err == nil || ctx.Err() != nil || !userDownloadTransient(err) {
			return err
		}
	}
	return err
}
