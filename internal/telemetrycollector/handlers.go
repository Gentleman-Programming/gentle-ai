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

	// TrustedProxies are peer CIDRs allowed to set X-Forwarded-For/X-Real-IP
	// for rate-limiting. Any other peer is keyed on its own address.
	TrustedProxies []*net.IPNet
}

// NewMux builds the collector's HTTP routes.
func (s *Server) NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/events", s.handleEvents)
	mux.HandleFunc("GET /v1/summary", s.handleSummary)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
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

// clientKey derives the rate-limiter key: the peer address, unless the peer
// is a trusted proxy, in which case the first X-Forwarded-For hop (or
// X-Real-IP) is used. Never persisted or logged.
func (s *Server) clientKey(r *http.Request) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	if !s.peerIsTrustedProxy(peer) {
		return peer
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if first := strings.TrimSpace(strings.SplitN(forwarded, ",", 2)[0]); first != "" {
			return first
		}
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		return real
	}
	return peer
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
		s.logger().Error("telemetry event storage failed", "error", err)
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
