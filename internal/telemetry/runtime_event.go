package telemetry

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const RuntimeEventSchema = "gentle-ai.telemetry-runtime-event/v1"
const RuntimeDeliverySchema = "gentle-ai.telemetry-runtime-delivery/v1"

// RuntimeEvent is anonymous transport. DeliveryID is fresh for each observation
// submission, never a batch/session/install identity. Clients send once only;
// collector deduplication is defensive and does not imply client retries.
type RuntimeEvent struct {
	Schema     string          `json:"schema"`
	Registry   json.RawMessage `json:"registry"`
	DeliveryID string          `json:"delivery_id"`
	Host       string          `json:"host"`
	Rows       []RuntimeRow    `json:"rows"`
}

var ErrRuntimeEvent = errors.New("invalid runtime event")

// ParseRuntimeEvent rejects private/unknown fields before typed decoding and
// shares the stdin contract's exact row validation and numeric normalization.
// It returns only bounded errors, never request content.
func ParseRuntimeEvent(data []byte) (RuntimeEvent, error) {
	var event RuntimeEvent
	if len(data) > RuntimeMaxBytes {
		return event, ErrRuntimeEvent
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if runtimeUnique(d) != nil {
		return event, ErrRuntimeEvent
	}
	if _, err := d.Token(); err != io.EOF {
		return event, ErrRuntimeEvent
	}
	object, ok := runtimeObject(data, "schema|registry|delivery_id|host|rows")
	if !ok || !runtimeExactRows(object["rows"]) {
		return event, ErrRuntimeEvent
	}
	if json.Unmarshal(data, &event) != nil || event.Schema != RuntimeEventSchema || runtimeNumber(event.Registry) != "1" || !runtimeID.MatchString(event.DeliveryID) || !runtimeMember(event.Host, "claude-code|opencode|codex|pi") || len(event.Rows) < 1 || len(event.Rows) > 32 {
		return RuntimeEvent{}, ErrRuntimeEvent
	}
	if normalizeRuntimeRows(event.Rows) != nil {
		return RuntimeEvent{}, ErrRuntimeEvent
	}
	event.Registry = json.RawMessage("1")
	return event, nil
}
