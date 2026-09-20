package telemetrycollector

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gentleman-programming/gentle-ai/v3/internal/telemetry"
)

// Runtime-store modes for Server.RuntimeStore. See its doc comment.
const (
	RuntimeStoreSQLite  = "sqlite"
	RuntimeStoreMetrics = "metrics"
	RuntimeStoreBoth    = "both"
)

// runtimeStoreMode normalizes Server.RuntimeStore: an empty or unrecognized
// value falls back to RuntimeStoreSQLite, today's behavior, so existing
// wiring (and every test Server literal predating this flag) keeps working
// unchanged.
func (s *Server) runtimeStoreMode() string {
	switch s.RuntimeStore {
	case RuntimeStoreMetrics, RuntimeStoreBoth:
		return s.RuntimeStore
	default:
		return RuntimeStoreSQLite
	}
}

func (s *Server) handleRuntimeEvents(w http.ResponseWriter, r *http.Request) {
	// A separate, ephemeral abuse quota from POST /v1/events (see
	// RuntimeLimiter), never persisted or logged by peer key. No delivery
	// ID is treated as authenticated identity.
	if !s.runtimeLimiter().Allow(s.clientKey(r)) {
		w.WriteHeader(http.StatusTooManyRequests)
		s.logger().Info("runtime telemetry rejected", "reason", "rate_limited")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, telemetry.RuntimeMaxBytes+1))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(body) > telemetry.RuntimeMaxBytes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	event, err := telemetry.ParseRuntimeEvent(body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	mode := s.runtimeStoreMode()
	var decision string
	if mode == RuntimeStoreMetrics {
		// No raw rows in this mode: dedup by delivery id alone, never by
		// payload comparison (see InsertRuntimeDeliveryID).
		decision, err = s.Storage.InsertRuntimeDeliveryID(r.Context(), event.DeliveryID, s.now())
	} else {
		decision, err = s.Storage.InsertRuntimeEvent(r.Context(), event, s.now())
	}
	if errors.Is(err, ErrRuntimeConflict) {
		w.WriteHeader(http.StatusConflict)
		return
	}
	if err != nil {
		reason := "storage_unavailable"
		if errors.Is(err, errRuntimeStorageBusy) {
			reason = "storage_busy"
		}
		s.logger().Error("runtime telemetry storage failed", "reason", reason, "error", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Never observe a duplicate (it would double-count, see
	// RuntimeMetrics.Observe), and never observe under RuntimeStoreSQLite
	// even if a Metrics registry happens to be set.
	if decision == "stored" && s.Metrics != nil && mode != RuntimeStoreSQLite {
		s.Metrics.Observe(event)
	}

	w.Header().Set("Content-Type", "application/json")
	// No success before the storage transaction commits, including duplicates.
	_ = json.NewEncoder(w).Encode(struct {
		Schema   string `json:"schema"`
		Decision string `json:"decision"`
	}{telemetry.RuntimeDeliverySchema, decision})
}

// handleMetrics serves the runtime counters registry as Prometheus text
// exposition for VictoriaMetrics to scrape. No auth: the collector's
// listener is loopback-only (see cmd/gentle-telemetry) and VictoriaMetrics
// scrapes it from the same host, the same trust boundary every other
// unauthenticated route on this listener already relies on. Nil Metrics
// (RuntimeStoreSQLite, the default) serves an empty body.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	if s.Metrics == nil {
		return
	}
	_, _ = s.Metrics.WriteTo(w)
}
