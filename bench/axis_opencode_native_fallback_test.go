package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
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
			response, err := http.Post(server.URL, "application/json", bytes.NewReader(body))
			if err != nil {
				errs <- err
				return
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				errs <- &unexpectedStatusError{status: response.StatusCode}
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
