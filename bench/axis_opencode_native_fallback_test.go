package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestNativeFallbackFixtureHandlesConcurrentRequests(t *testing.T) {
	fixture := &nativeFallbackFixture{}
	server := httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	defer server.Close()

	const requests = 32
	var group sync.WaitGroup
	errs := make(chan error, requests)
	for range requests {
		group.Add(1)
		go func() {
			defer group.Done()
			body, err := json.Marshal(nativeFallbackRequest{Model: "root"})
			if err != nil {
				errs <- err
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(body))
			if err != nil {
				errs <- err
				return
			}
			request.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				errs <- err
				return
			}
			statusCode := response.StatusCode
			if err := response.Body.Close(); err != nil {
				errs <- err
				return
			}
			if statusCode != http.StatusOK {
				errs <- &unexpectedStatusError{status: statusCode}
			}
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	fixture.mu.Lock()
	got := len(fixture.models)
	fixture.mu.Unlock()
	if got != requests {
		t.Fatalf("captured request models = %d, want %d", got, requests)
	}
}

func TestWriteNativeFallbackChunksRejectsUnencodableFramesBeforeWriting(t *testing.T) {
	recorder := httptest.NewRecorder()
	err := writeNativeFallbackChunks(recorder, []any{map[string]any{"bad": func() {}}})
	if err == nil {
		t.Fatal("writeNativeFallbackChunks() error = nil, want marshal error")
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("response body = %q, want no partial stream", recorder.Body.String())
	}
}

type unexpectedStatusError struct {
	status int
}

func (e *unexpectedStatusError) Error() string {
	return http.StatusText(e.status)
}
