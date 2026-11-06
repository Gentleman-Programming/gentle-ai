package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type userMainResponse func(*http.Request) (*http.Response, error)

func (f userMainResponse) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type userMainBody struct {
	io.Reader
	close func()
}

func (b userMainBody) Close() error { b.close(); return nil }

// Controlled HTTP response DATA only, not publisher/TLS/authenticated ZIP or
// installed/Main selection. Nothing from these archive bytes is executed.
func TestUserMainSnapshotFetchesFallbackCommit(t *testing.T) {
	t.Setenv("GENTLE_AI_MAIN_FALLBACK", "1")
	archive := "Main archive response DATA"
	calls := 0
	client := &http.Client{Transport: userMainResponse(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != http.MethodGet || req.URL.String() != "https://codeload.github.com/Gentleman-Programming/gentle-shell/zip/"+userMainFallbackCommit || req.Header.Get("Accept-Encoding") != "identity" || req.Header.Get("Authorization") != "" {
			t.Fatal("qualified publisher request changed", req)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, ContentLength: int64(len(archive)), Body: userMainBody{Reader: strings.NewReader(archive), close: func() {}}}, nil
	})}
	artifact, data, err := userMainSnapshotUsing(context.Background(), client)
	want, descriptorErr := userMainDescriptor(userMainFallbackCommit, fmt.Sprintf("%x", sha256.Sum256([]byte(archive))))
	if err != nil || descriptorErr != nil || artifact != want || string(data) != archive || calls != 1 {
		t.Fatalf("qualified Main snapshot differs: %+v %q %v calls=%d", artifact, data, err, calls)
	}
}

func TestUserMainSnapshotRejectsPublisherFailures(t *testing.T) {
	t.Setenv("GENTLE_AI_MAIN_FALLBACK", "1")
	for _, failure := range []string{"status", "encoding", "overflow", "close"} {
		t.Run(failure, func(t *testing.T) {
			cause := errors.New("controlled close failure")
			client := &http.Client{Transport: userMainResponse(func(req *http.Request) (*http.Response, error) {
				data := "archive"
				status, encoding := http.StatusOK, ""
				if failure == "status" {
					status = http.StatusNotFound
				}
				if failure == "encoding" {
					encoding = "gzip"
				}
				if failure == "overflow" {
					data = strings.Repeat("x", (32<<20)+1)
				}
				body := userMainCloseErrorBody{Reader: strings.NewReader(data), closed: new(int)}
				if failure == "close" {
					body.cause = cause
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Encoding": []string{encoding}}, ContentLength: int64(len(data)), Body: body}, nil
			})}
			artifact, data, err := userMainSnapshotUsing(context.Background(), client)
			if err == nil || artifact != (userSnapshotArtifact{}) || data != nil {
				t.Fatalf("publisher failure became usable snapshot: %v", err)
			}
			if failure == "close" && !errors.Is(err, cause) {
				t.Fatalf("close failure lost its cause: %v", err)
			}
		})
	}
}

// A refused publisher response names what was refused, so a GitHub rate limit
// (unauthenticated API calls share a per-network quota) is not an opaque error.
func TestUserMainSnapshotNamesPublisherRefusal(t *testing.T) {
	for _, tt := range []struct {
		name     string
		status   int
		encoding string
		want     []string
	}{
		{name: "rate limited", status: http.StatusForbidden, want: []string{"api.github.com", "HTTP 403", "rate limit"}},
		{name: "too many requests", status: http.StatusTooManyRequests, want: []string{"HTTP 429", "rate limit"}},
		{name: "not found", status: http.StatusNotFound, want: []string{"HTTP 404"}},
		{name: "encoding", status: http.StatusOK, encoding: "gzip", want: []string{"Content-Encoding \"gzip\""}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GENTLE_AI_MAIN_FALLBACK", "")
			client := &http.Client{Transport: userMainResponse(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tt.status, Header: http.Header{"Content-Encoding": []string{tt.encoding}}, ContentLength: 2, Body: userMainBody{Reader: strings.NewReader("{}"), close: func() {}}}, nil
			})}
			_, data, err := userMainSnapshotUsing(context.Background(), client)
			if err == nil || data != nil {
				t.Fatalf("refused publisher response became usable: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("refusal %q does not name %q", err, want)
				}
			}
			if tt.status != http.StatusForbidden && tt.status != http.StatusTooManyRequests && strings.Contains(err.Error(), "rate limit") {
				t.Fatalf("non rate-limit refusal blamed the rate limit: %v", err)
			}
		})
	}
}

func TestUserMainDescriptorPreservesSourceOnlyContract(t *testing.T) {
	commit, digest := strings.Repeat("a", 40), strings.Repeat("b", 64)
	artifact, err := userMainDescriptor(commit, digest)
	if err != nil || artifact != (userSnapshotArtifact{URL: "https://codeload.github.com/Gentleman-Programming/gentle-shell/zip/" + commit, SHA: digest, Archive: "main.zip", Prefix: "gentle-shell-" + commit, Bound: 32 << 20}) {
		t.Fatal("descriptor/32MiB bound changed", artifact, err)
	}
	for _, bad := range []string{"main", strings.ToUpper(commit), strings.Repeat("g", 40), ""} {
		if result, err := userMainDescriptor(bad, digest); err == nil || result != (userSnapshotArtifact{}) {
			t.Fatal("unfrozen/invalid Main descriptor", result, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, data, err := userMainSnapshot(ctx); err == nil || result != (userSnapshotArtifact{}) || data != nil {
		t.Fatal("production producer issued canceled requests", result, err)
	}
}
