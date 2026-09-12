package telemetrycollector

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRuntimeHandleEvents(t *testing.T) {
	s, err := OpenStorage(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var logs bytes.Buffer
	server := &Server{Storage: s, Limiter: NewRateLimiter(100), Logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	mux := server.NewMux()
	send := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.RemoteAddr = "192.0.2.123:4321"
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	valid := string(runtimeFixture())
	for _, tc := range []struct {
		name, body string
		status     int
		decision   string
	}{
		{"stored", valid, 200, "stored"},
		{"canonical duplicate", strings.ReplaceAll(valid, `"sum_ms":1.25`, `"sum_ms":125e-2`), 200, "duplicate"},
		{"changed", strings.Replace(valid, `"host":"pi"`, `"host":"codex"`, 1), 409, ""},
		{"invalid", `{"path":"/private/canary"}`, 400, ""},
		{"oversize", strings.Repeat(" ", 16385), 413, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := send("/v1/runtime-events", tc.body)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d", w.Code, tc.status)
			}
			if tc.decision != "" {
				var ack map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &ack); err != nil {
					t.Fatal(err)
				}
				if len(ack) != 2 || ack["schema"] != "gentle-ai.telemetry-runtime-delivery/v1" || ack["decision"] != tc.decision {
					t.Fatalf("ack %v", ack)
				}
				if w.Header().Get("Content-Type") != "application/json" {
					t.Fatal("missing JSON content type")
				}
			} else if w.Body.Len() != 0 {
				t.Fatal("error content leaked")
			}
		})
	}
	if w := send("/v1/events", valid); w.Code != 400 {
		t.Fatalf("legacy endpoint accepted runtime: %d", w.Code)
	}
	// Both routes share the existing per-peer in-memory limiter; no new auth.
	server.Limiter = NewRateLimiter(1)
	if w := send("/v1/runtime-events", valid); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := send("/v1/events", valid); w.Code != 429 {
		t.Fatalf("shared rate quota: %d", w.Code)
	}
	server.Limiter = NewRateLimiter(100)
	if _, err := s.db.Exec(`CREATE TRIGGER fail_runtime BEFORE INSERT ON runtime_rows BEGIN SELECT RAISE(ABORT,'private-canary'); END`); err != nil {
		t.Fatal(err)
	}
	fresh := strings.Replace(valid, "0123456789abcdef0123456789abcdef", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1)
	if w := send("/v1/runtime-events", fresh); w.Code != 500 || w.Body.Len() != 0 {
		t.Fatalf("storage failure acknowledged: %d %s", w.Code, w.Body)
	}
	if strings.Contains(logs.String(), "canary") || strings.Contains(logs.String(), "192.0.2.123") || strings.Contains(logs.String(), "delivery_id") {
		t.Fatal("private data logged")
	}
}

func TestRuntimeHandleEventsConcurrent(t *testing.T) {
	s, err := OpenStorage(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	mux := (&Server{Storage: s, Limiter: NewRateLimiter(100)}).NewMux()
	results := make(chan string, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/runtime-events", bytes.NewReader(runtimeFixture())))
			var ack map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &ack); err != nil || w.Code != 200 {
				results <- "failed"
				return
			}
			results <- ack["decision"]
		}()
	}
	wg.Wait()
	close(results)
	counts := map[string]int{}
	for result := range results {
		counts[result]++
	}
	if counts["stored"] != 1 || counts["duplicate"] != 7 || len(counts) != 2 {
		t.Fatalf("concurrent decisions %v", counts)
	}
}
