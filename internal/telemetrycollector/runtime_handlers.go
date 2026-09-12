package telemetrycollector

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

func (s *Server) handleRuntimeEvents(w http.ResponseWriter, r *http.Request) {
	// Reuse the install endpoint's shared, ephemeral abuse quota, never persist
	// or log its peer key. No delivery ID is treated as authenticated identity.
	if !s.Limiter.Allow(s.clientKey(r)) {
		w.WriteHeader(http.StatusTooManyRequests)
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
	decision, err := s.Storage.InsertRuntimeEvent(r.Context(), event, s.now())
	if errors.Is(err, ErrRuntimeConflict) {
		w.WriteHeader(http.StatusConflict)
		return
	}
	if err != nil {
		s.logger().Error("runtime telemetry storage failed", "reason", "storage_unavailable")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// No success before the storage transaction commits, including duplicates.
	_ = json.NewEncoder(w).Encode(struct {
		Schema   string `json:"schema"`
		Decision string `json:"decision"`
	}{telemetry.RuntimeDeliverySchema, decision})
}
