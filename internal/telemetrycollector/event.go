// Package telemetrycollector implements the self-hosted collector for
// gentle-ai's anonymous telemetry events: HTTP handlers, SQLite storage,
// daily rollups, retention, and the summary endpoint.
//
// The wire contract is gentle-ai.telemetry-event/v1, defined by the client
// branch under contracts/telemetry/v1/schemas/event.schema.json. Until that
// branch lands, the schema is defined locally at
// internal/telemetrycollector/schema/event.schema.json, byte-identical to
// the contract described in Gentleman-Programming/gentle-ai#4310. Once the
// canonical copy is published, this local copy should be replaced by a
// reference to it rather than kept as a second source of truth.
package telemetrycollector

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// SchemaID is the "schema" discriminator every event must carry.
const SchemaID = "gentle-ai.telemetry-event/v1"

// EventInstall and EventHeartbeat are the two event kinds the contract
// allows.
const (
	EventInstall   = "install"
	EventHeartbeat = "heartbeat"
)

// MaxEventBodyBytes is the hard cap on the POST /v1/events request body, per
// the contract ("JSON, POST body, max 4 KiB").
const MaxEventBodyBytes = 4 * 1024

//go:embed schema/event.schema.json
var eventSchemaJSON []byte

const eventSchemaResourceID = "https://schemas.gentle-ai.dev/telemetry/v1/event.schema.json"

var (
	compileOnce    sync.Once
	compiledSchema *jsonschema.Schema
	compileErr     error
)

func compiledEventSchema() (*jsonschema.Schema, error) {
	compileOnce.Do(func() {
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(eventSchemaJSON))
		if err != nil {
			compileErr = fmt.Errorf("unmarshal embedded event schema: %w", err)
			return
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(eventSchemaResourceID, document); err != nil {
			compileErr = fmt.Errorf("add embedded event schema resource: %w", err)
			return
		}
		compiled, err := compiler.Compile(eventSchemaResourceID)
		if err != nil {
			compileErr = fmt.Errorf("compile embedded event schema: %w", err)
			return
		}
		compiledSchema = compiled
	})
	return compiledSchema, compileErr
}

// Counters carries the since-last-successful-send counters attached to a
// heartbeat event. It is never present on an install event.
type Counters struct {
	Syncs             int `json:"syncs"`
	SDDPhaseRuns      int `json:"sdd_phase_runs"`
	ReviewsApproved   int `json:"reviews_approved"`
	ReviewsCorrection int `json:"reviews_correction"`
	ReviewsEscalated  int `json:"reviews_escalated"`
}

// Event is the decoded, validated form of a gentle-ai.telemetry-event/v1
// payload. It never carries the sender's IP address; nothing in this type
// or its JSON tags corresponds to network origin.
type Event struct {
	Schema     string    `json:"schema"`
	Kind       string    `json:"event"`
	InstallID  string    `json:"install_id"`
	SentAt     time.Time `json:"sent_at"`
	Version    string    `json:"version"`
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
	Agents     []string  `json:"agents"`
	Components []string  `json:"components"`
	RDDEnabled bool      `json:"rdd_enabled"`
	Counters   *Counters `json:"counters,omitempty"`
}

// ValidationErrorCode classifies why a payload was rejected, so the HTTP
// handler can map it to the right status code without re-deriving the
// reason from an error string.
type ValidationErrorCode int

const (
	// ErrOversize means the body exceeded MaxEventBodyBytes.
	ErrOversize ValidationErrorCode = iota
	// ErrInvalid means the body was well-formed JSON but failed schema,
	// enum, or structural validation, or was not valid JSON at all.
	ErrInvalid
)

// ValidationError reports a rejected event payload.
type ValidationError struct {
	Code    ValidationErrorCode
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &ValidationError{Code: ErrInvalid, Message: fmt.Sprintf(format, args...)}
}

// ParseEvent validates raw against the gentle-ai.telemetry-event/v1 schema
// (rejecting unknown fields, bad enums, and schema-id mismatches) and
// decodes it into an Event. It never reads more than MaxEventBodyBytes+1
// bytes of intent from raw's length: callers are expected to have already
// capped the read.
func ParseEvent(raw []byte) (Event, error) {
	if len(raw) > MaxEventBodyBytes {
		return Event{}, &ValidationError{Code: ErrOversize, Message: fmt.Sprintf("body exceeds %d bytes", MaxEventBodyBytes)}
	}

	schema, err := compiledEventSchema()
	if err != nil {
		return Event{}, fmt.Errorf("telemetrycollector: %w", err)
	}

	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return Event{}, invalid("invalid JSON: %v", err)
	}
	if err := schema.Validate(document); err != nil {
		return Event{}, invalid("schema validation failed: %v", err)
	}

	var event Event
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return Event{}, invalid("decode event: %v", err)
	}

	if event.Kind == EventInstall && event.Counters != nil {
		return Event{}, invalid("install event must not carry counters")
	}

	return event, nil
}

// NormalizedInstallID lower-cases the install id for storage and grouping,
// since UUIDs are case-insensitive and clients may format them either way.
func (e Event) NormalizedInstallID() string {
	return strings.ToLower(strings.TrimSpace(e.InstallID))
}
