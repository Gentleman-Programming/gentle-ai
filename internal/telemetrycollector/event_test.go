package telemetrycollector

import (
	"errors"
	"strings"
	"testing"
)

const validInstallEvent = `{
	"schema": "gentle-ai.telemetry-event/v1",
	"event": "install",
	"install_id": "550e8400-e29b-41d4-a716-446655440000",
	"sent_at": "2026-09-01T12:00:00Z",
	"version": "2.3.0",
	"os": "darwin",
	"arch": "arm64",
	"agents": ["claude-code"],
	"components": ["sdd", "engram"],
	"rdd_enabled": true
}`

const validHeartbeatEvent = `{
	"schema": "gentle-ai.telemetry-event/v1",
	"event": "heartbeat",
	"install_id": "550e8400-e29b-41d4-a716-446655440000",
	"sent_at": "2026-09-01T12:00:00Z",
	"version": "2.3.0",
	"os": "linux",
	"arch": "amd64",
	"agents": ["claude-code", "opencode"],
	"components": ["sdd"],
	"rdd_enabled": false,
	"counters": {
		"syncs": 3,
		"sdd_phase_runs": 1,
		"reviews_approved": 2,
		"reviews_correction": 0,
		"reviews_escalated": 0
	}
}`

func TestParseEvent_AcceptsValidInstallEvent(t *testing.T) {
	ev, err := ParseEvent([]byte(validInstallEvent))
	if err != nil {
		t.Fatalf("ParseEvent: unexpected error: %v", err)
	}
	if ev.Kind != EventInstall {
		t.Errorf("Kind = %q, want %q", ev.Kind, EventInstall)
	}
	if ev.Counters != nil {
		t.Errorf("Counters = %+v, want nil for install event", ev.Counters)
	}
	if ev.NormalizedInstallID() != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("NormalizedInstallID = %q", ev.NormalizedInstallID())
	}
}

func TestParseEvent_AcceptsValidHeartbeatEvent(t *testing.T) {
	ev, err := ParseEvent([]byte(validHeartbeatEvent))
	if err != nil {
		t.Fatalf("ParseEvent: unexpected error: %v", err)
	}
	if ev.Kind != EventHeartbeat {
		t.Errorf("Kind = %q, want %q", ev.Kind, EventHeartbeat)
	}
	if ev.Counters == nil {
		t.Fatal("Counters = nil, want present for heartbeat event")
	}
	if ev.Counters.Syncs != 3 {
		t.Errorf("Counters.Syncs = %d, want 3", ev.Counters.Syncs)
	}
}

func TestParseEvent_RejectsInvalidPayloads(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "unknown top-level field",
			body: `{"schema":"gentle-ai.telemetry-event/v1","event":"install","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","version":"1.0.0","os":"darwin","arch":"arm64","agents":[],"components":[],"rdd_enabled":true,"hostname":"laptop"}`,
		},
		{
			name: "unknown counters field",
			body: `{"schema":"gentle-ai.telemetry-event/v1","event":"heartbeat","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","version":"1.0.0","os":"darwin","arch":"arm64","agents":[],"components":[],"rdd_enabled":true,"counters":{"syncs":1,"sdd_phase_runs":0,"reviews_approved":0,"reviews_correction":0,"reviews_escalated":0,"prompt_tokens":1}}`,
		},
		{
			name: "wrong schema id",
			body: `{"schema":"gentle-ai.telemetry-event/v2","event":"install","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","version":"1.0.0","os":"darwin","arch":"arm64","agents":[],"components":[],"rdd_enabled":true}`,
		},
		{
			name: "bad event enum value",
			body: `{"schema":"gentle-ai.telemetry-event/v1","event":"uninstall","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","version":"1.0.0","os":"darwin","arch":"arm64","agents":[],"components":[],"rdd_enabled":true}`,
		},
		{
			name: "bad os enum value",
			body: `{"schema":"gentle-ai.telemetry-event/v1","event":"install","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","version":"1.0.0","os":"plan9","arch":"arm64","agents":[],"components":[],"rdd_enabled":true}`,
		},
		{
			name: "install event with counters",
			body: `{"schema":"gentle-ai.telemetry-event/v1","event":"install","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","version":"1.0.0","os":"darwin","arch":"arm64","agents":[],"components":[],"rdd_enabled":true,"counters":{"syncs":1,"sdd_phase_runs":0,"reviews_approved":0,"reviews_correction":0,"reviews_escalated":0}}`,
		},
		{
			name: "malformed json",
			body: `{"schema": "gentle-ai.telemetry-event/v1",`,
		},
		{
			name: "not an object",
			body: `["gentle-ai.telemetry-event/v1"]`,
		},
		{
			name: "missing required field",
			body: `{"schema":"gentle-ai.telemetry-event/v1","event":"install","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","os":"darwin","arch":"arm64","agents":[],"components":[],"rdd_enabled":true}`,
		},
		{
			name: "malformed install id",
			body: `{"schema":"gentle-ai.telemetry-event/v1","event":"install","install_id":"not-a-uuid","sent_at":"2026-09-01T12:00:00Z","version":"1.0.0","os":"darwin","arch":"arm64","agents":[],"components":[],"rdd_enabled":true}`,
		},
		{
			name: "unknown agent id",
			body: `{"schema":"gentle-ai.telemetry-event/v1","event":"install","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","version":"1.0.0","os":"darwin","arch":"arm64","agents":["totally-made-up-agent"],"components":[],"rdd_enabled":true}`,
		},
		{
			name: "unknown component id",
			body: `{"schema":"gentle-ai.telemetry-event/v1","event":"install","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","version":"1.0.0","os":"darwin","arch":"arm64","agents":[],"components":["totally-made-up-component"],"rdd_enabled":true}`,
		},
		{
			name: "unknown arch",
			body: `{"schema":"gentle-ai.telemetry-event/v1","event":"install","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","version":"1.0.0","os":"darwin","arch":"not-a-real-arch","agents":[],"components":[],"rdd_enabled":true}`,
		},
		{
			name: "free-text version is rejected",
			body: `{"schema":"gentle-ai.telemetry-event/v1","event":"install","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","version":"whatever I feel like","os":"darwin","arch":"arm64","agents":[],"components":[],"rdd_enabled":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseEvent([]byte(tt.body))
			if err == nil {
				t.Fatal("ParseEvent: expected an error, got nil")
			}
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("ParseEvent: error is not a *ValidationError: %v", err)
			}
			if verr.Code != ErrInvalid {
				t.Errorf("Code = %v, want ErrInvalid", verr.Code)
			}
		})
	}
}

func TestParseEvent_RejectsOversizeBody(t *testing.T) {
	huge := strings.Repeat("a", MaxEventBodyBytes+1)
	body := `{"schema":"gentle-ai.telemetry-event/v1","event":"install","install_id":"550e8400-e29b-41d4-a716-446655440000","sent_at":"2026-09-01T12:00:00Z","version":"` + huge + `","os":"darwin","arch":"arm64","agents":[],"components":[],"rdd_enabled":true}`

	_, err := ParseEvent([]byte(body))
	if err == nil {
		t.Fatal("ParseEvent: expected an error, got nil")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("ParseEvent: error is not a *ValidationError: %v", err)
	}
	if verr.Code != ErrOversize {
		t.Errorf("Code = %v, want ErrOversize", verr.Code)
	}
}
