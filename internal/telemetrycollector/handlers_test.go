package telemetrycollector

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func trustedLoopback() []*net.IPNet {
	return []*net.IPNet{{IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)}}
}

func newTestServer(t *testing.T, perMinute int) (*Server, *bytes.Buffer) {
	t.Helper()
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	fixedNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	return &Server{
		Storage:      openTestStorage(t),
		Limiter:      NewRateLimiter(perMinute),
		SummaryToken: "s3cr3t-token",
		Logger:       logger,
		Now:          func() time.Time { return fixedNow },
	}, &logBuf
}

func TestHandleEvents_AcceptsValidEvent(t *testing.T) {
	server, _ := newTestServer(t, 60)
	mux := server.NewMux()

	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(validInstallEvent))
	req.RemoteAddr = "203.0.113.10:54321"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	events, err := server.Storage.eventsOnDay(context.Background(), server.now())
	if err != nil {
		t.Fatalf("eventsOnDay: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("stored events = %d, want 1", len(events))
	}
}

func TestHandleEvents_RejectsInvalidPayload(t *testing.T) {
	server, _ := newTestServer(t, 60)
	mux := server.NewMux()

	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"not":"valid"}`))
	req.RemoteAddr = "203.0.113.10:54321"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleEvents_RejectsUnknownAgentID(t *testing.T) {
	server, _ := newTestServer(t, 60)
	mux := server.NewMux()

	body := `{"schema":"gentle-ai.telemetry-event/v1","event":"install","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","version":"1.0.0","os":"darwin","arch":"arm64","agents":["not-a-real-agent"],"components":[],"rdd_enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	req.RemoteAddr = "203.0.113.10:54321"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for an agent id outside the known enum", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleEvents_RejectsOversizeBody(t *testing.T) {
	server, _ := newTestServer(t, 60)
	mux := server.NewMux()

	huge := strings.Repeat("a", MaxEventBodyBytes+100)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(huge))
	req.RemoteAddr = "203.0.113.10:54321"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandleEvents_RateLimitReturns429AfterBurst(t *testing.T) {
	server, _ := newTestServer(t, 2)
	mux := server.NewMux()

	send := func() int {
		req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(validInstallEvent))
		req.RemoteAddr = "203.0.113.10:54321"
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := send(); code != http.StatusAccepted {
		t.Fatalf("request 1: status = %d, want 202", code)
	}
	if code := send(); code != http.StatusAccepted {
		t.Fatalf("request 2: status = %d, want 202", code)
	}
	if code := send(); code != http.StatusTooManyRequests {
		t.Fatalf("request 3: status = %d, want 429 after the configured burst", code)
	}
}

func TestHandleEvents_RateLimitIsPerRemoteAddress(t *testing.T) {
	server, _ := newTestServer(t, 1)
	mux := server.NewMux()

	post := func(remoteAddr string) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(validInstallEvent))
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := post("203.0.113.10:1"); code != http.StatusAccepted {
		t.Fatalf("first address: status = %d, want 202", code)
	}
	if code := post("203.0.113.10:2"); code != http.StatusTooManyRequests {
		t.Fatalf("same address, second port: status = %d, want 429", code)
	}
	if code := post("203.0.113.20:1"); code != http.StatusAccepted {
		t.Fatalf("different address: status = %d, want 202", code)
	}
}

func TestHandleEvents_RateLimitHonorsForwardedForOnlyFromTrustedProxy(t *testing.T) {
	server, _ := newTestServer(t, 1)
	server.TrustedProxies = trustedLoopback()
	mux := server.NewMux()

	post := func(remoteAddr, forwardedFor string) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(validInstallEvent))
		req.RemoteAddr = remoteAddr
		req.Header.Set("X-Forwarded-For", forwardedFor)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	steps := []struct {
		remoteAddr, forwardedFor string
		want                     int
	}{
		{"127.0.0.1:1", "203.0.113.10", http.StatusAccepted},           // trusted proxy, client A
		{"127.0.0.1:2", "203.0.113.20", http.StatusAccepted},           // trusted proxy, client B: own bucket
		{"127.0.0.1:3", "203.0.113.10", http.StatusTooManyRequests},    // client A again: exhausted
		{"203.0.113.99:1", "203.0.113.30", http.StatusAccepted},        // untrusted peer
		{"203.0.113.99:2", "203.0.113.40", http.StatusTooManyRequests}, // spoofed header ignored
	}
	for i, step := range steps {
		if got := post(step.remoteAddr, step.forwardedFor); got != step.want {
			t.Fatalf("step %d (%+v): status = %d, want %d", i, step, got, step.want)
		}
	}
}

func TestHandleEvents_NeverStoresOrLogsRemoteAddress(t *testing.T) {
	server, logBuf := newTestServer(t, 60)
	mux := server.NewMux()

	const spoofedIP = "198.51.100.77"
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(validInstallEvent))
	req.RemoteAddr = spoofedIP + ":9999"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if strings.Contains(logBuf.String(), spoofedIP) {
		t.Errorf("log output contains the remote address: %s", logBuf.String())
	}

	row := server.Storage.db.QueryRow(`SELECT received_at, event, install_id, version, os, arch, agents_json, components_json, coalesce(counters_json,'') FROM events`)
	var cols [9]string
	if err := row.Scan(&cols[0], &cols[1], &cols[2], &cols[3], &cols[4], &cols[5], &cols[6], &cols[7], &cols[8]); err != nil {
		t.Fatalf("scan row: %v", err)
	}
	for i, v := range cols {
		if strings.Contains(v, spoofedIP) {
			t.Errorf("column %d contains the remote address: %q", i, v)
		}
	}
}

func TestHandleSummary_RequiresBearerToken(t *testing.T) {
	server, _ := newTestServer(t, 60)
	mux := server.NewMux()

	t.Run("missing header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("wrong token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("correct token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
		req.Header.Set("Authorization", "Bearer s3cr3t-token")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
	})
}

func TestHandleSummary_EmptyTokenAlwaysRejects(t *testing.T) {
	server, _ := newTestServer(t, 60)
	server.SummaryToken = ""
	mux := server.NewMux()

	req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 when no token is configured", rec.Code)
	}
}

func TestHandleHealthz(t *testing.T) {
	server, _ := newTestServer(t, 60)
	mux := server.NewMux()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
