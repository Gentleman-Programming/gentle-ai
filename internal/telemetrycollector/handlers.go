package telemetrycollector

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Server wires the collector's HTTP handlers to storage, the rate limiter,
// and the summary bearer token. It never logs or stores a request's remote
// address: Log fields below are deliberately limited to event/outcome
// metadata that carries no network origin.
type Server struct {
	Storage      *Storage
	Limiter      *RateLimiter
	SummaryToken string
	Logger       *slog.Logger
	Now          func() time.Time

	// RuntimeStore selects how POST /v1/runtime-events persists a newly
	// stored delivery: RuntimeStoreSQLite (the default, "" also means this)
	// keeps today's behavior (runtime_deliveries/runtime_rows, no metrics);
	// RuntimeStoreMetrics stops writing raw rows entirely, deduping by id
	// only (runtime_delivery_ids) and observing into Metrics instead;
	// RuntimeStoreBoth does both, for a transition window. See
	// runtimeStoreMode and handleRuntimeEvents.
	RuntimeStore string

	// Metrics is the in-memory Prometheus counters registry GET /metrics
	// serves. Only Observe()d for a newly stored delivery under
	// RuntimeStoreMetrics/RuntimeStoreBoth (never for a duplicate, and
	// never at all under RuntimeStoreSQLite even if this is non-nil). Left
	// nil, GET /metrics serves an empty body.
	Metrics *RuntimeMetrics

	// RuntimeLimiter is the rate budget for POST /v1/runtime-events,
	// separate from Limiter (POST /v1/events): heartbeats are frequent and
	// were sharing one 60/min bucket with stored deliveries, plateauing
	// storage at exactly that rate and rejecting most heartbeats. When nil
	// (older callers/tests that only set Limiter), runtimeLimiter falls
	// back to Limiter so existing wiring keeps its previous shared-budget
	// behavior.
	RuntimeLimiter *RateLimiter

	// TrustedProxies are peer CIDRs allowed to set X-Forwarded-For/X-Real-IP
	// for rate-limiting. Any other peer is keyed on its own address.
	TrustedProxies []*net.IPNet

	// invalidForwardedMu guards invalidForwardedLast, which throttles the
	// "ignored an invalid forwarded address" log line to at most once per
	// minute regardless of request volume.
	invalidForwardedMu   sync.Mutex
	invalidForwardedLast time.Time
}

// NewMux builds the collector's HTTP routes.
func (s *Server) NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/events", s.handleEvents)
	mux.HandleFunc("POST /v1/runtime-events", s.handleRuntimeEvents)
	mux.HandleFunc("GET /v1/summary", s.handleSummary)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	return mux
}

func (s *Server) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// runtimeLimiter returns RuntimeLimiter, falling back to Limiter when
// RuntimeLimiter is unset so callers that only configure Limiter keep the
// previous shared-budget behavior.
func (s *Server) runtimeLimiter() *RateLimiter {
	if s.RuntimeLimiter != nil {
		return s.RuntimeLimiter
	}
	return s.Limiter
}

// clientKey derives the rate-limiter key: the peer address, unless the peer
// is a trusted proxy, in which case the first X-Forwarded-For hop that
// parses as an IP address is used (falling back to X-Real-IP, then the
// peer). A forwarded value that does not parse as an IP is never used as a
// key: production has seen "X-Forwarded-For: (null), <client>" from a
// misconfigured proxy, which would otherwise key every client on the
// literal string "(null)". Never persisted or logged.
func (s *Server) clientKey(r *http.Request) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	if !s.peerIsTrustedProxy(peer) {
		return peer
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		for _, hop := range strings.Split(forwarded, ",") {
			if ip := parseForwardedAddress(hop); ip != "" {
				return ip
			}
		}
		s.logInvalidForwardedAddress()
	}
	if ip := parseForwardedAddress(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	return peer
}

// parseForwardedAddress validates raw as an IP address, accepting a bare
// IP, a host:port pair, or a bracketed [v6]:port pair (the port, if any, is
// stripped before validation). Returns "" when raw does not parse as an IP.
func parseForwardedAddress(raw string) string {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(candidate); err == nil {
		candidate = host
	} else {
		candidate = strings.TrimSuffix(strings.TrimPrefix(candidate, "["), "]")
	}
	if net.ParseIP(candidate) == nil {
		return ""
	}
	return candidate
}

// logInvalidForwardedAddress logs that a trusted proxy sent an
// X-Forwarded-For header with no parseable IP hop, throttled to at most
// once per minute so a misconfigured proxy cannot flood the log. The
// invalid value itself is never logged (privacy).
func (s *Server) logInvalidForwardedAddress() {
	now := s.now()
	s.invalidForwardedMu.Lock()
	defer s.invalidForwardedMu.Unlock()
	if !s.invalidForwardedLast.IsZero() && now.Sub(s.invalidForwardedLast) < time.Minute {
		return
	}
	s.invalidForwardedLast = now
	s.logger().Info("ignored an invalid forwarded address")
}

func (s *Server) peerIsTrustedProxy(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, cidr := range s.TrustedProxies {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !s.Limiter.Allow(s.clientKey(r)) {
		w.WriteHeader(http.StatusTooManyRequests)
		s.logger().Info("telemetry event rejected", "reason", "rate_limited")
		return
	}

	// Read one byte more than the cap so an oversize body is detected even
	// when Content-Length is absent or understated, without buffering an
	// unbounded body in memory.
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxEventBodyBytes+1))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.logger().Info("telemetry event rejected", "reason", "read_error")
		return
	}
	if len(body) > MaxEventBodyBytes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		s.logger().Info("telemetry event rejected", "reason", "oversize")
		return
	}

	event, err := ParseEvent(body)
	if err != nil {
		var verr *ValidationError
		if errors.As(err, &verr) && verr.Code == ErrOversize {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			s.logger().Info("telemetry event rejected", "reason", "oversize")
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		s.logger().Info("telemetry event rejected", "reason", "invalid")
		return
	}

	if err := s.Storage.InsertEvent(r.Context(), event, s.now()); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		// err.Error() is behavior-neutral here under both the text and JSON
		// slog handlers: slog already special-cases a top-level error Attr
		// value by calling its Error method (see log/slog's JSONHandler
		// doc), so this produced the same log line even before this call
		// was made explicit. Kept explicit for symmetry with
		// handleRuntimeEvents' error-attribute logging.
		s.logger().Error("telemetry event storage failed", "error", err.Error())
		return
	}

	w.WriteHeader(http.StatusAccepted)
	s.logger().Info("telemetry event accepted", "event", event.Kind)
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSummary(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="gentle-telemetry"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	summary, err := BuildSummary(r.Context(), s.Storage, s.now())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		s.logger().Error("summary computation failed", "error", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(summary); err != nil {
		s.logger().Error("summary encode failed", "error", err)
	}
}

func (s *Server) authorizeSummary(r *http.Request) bool {
	if s.SummaryToken == "" {
		return false
	}
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
		return false
	}
	token := header[len(prefix):]
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.SummaryToken)) == 1
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
