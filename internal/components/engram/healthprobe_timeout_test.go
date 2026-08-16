package engram

import (
	"testing"
	"time"
)

// TestConfiguredStdioTimeoutIsRead is #3068's first half.
//
// The probe's deadline was a fixed 5 seconds covering process spawn PLUS the
// initialize handshake, and #3068's reporter measured a healthy store's
// handshake alone at 4.87 to 5.16 seconds. The check therefore sat on top of
// the line rather than under it and flapped: their report captured the same
// store both passing and failing.
//
// They raised the OpenCode MCP timeout to 15 seconds, saw no change, and
// reverted it. They were right that it did nothing: StdioCommand carried
// Command, Args and Source, so a configured timeout had no wire to travel
// down. It does now.
func TestConfiguredStdioTimeoutIsRead(t *testing.T) {
	tests := []struct {
		name   string
		server map[string]any
		want   time.Duration
	}{
		{name: "absent", server: map[string]any{"command": "engram"}, want: 0},
		// A bare number is seconds: that is how the OpenCode MCP config
		// expresses it, and it is what #3068's reporter set.
		{name: "seconds as number", server: map[string]any{"command": "engram", "timeout": float64(15)}, want: 15 * time.Second},
		{name: "seconds as integer", server: map[string]any{"command": "engram", "timeout": 20}, want: 20 * time.Second},
		{name: "duration string", server: map[string]any{"command": "engram", "timeout": "12s"}, want: 12 * time.Second},
		// Nonsense is ignored rather than fatal. A malformed timeout must not
		// make an otherwise valid Engram configuration unreadable, because the
		// consequence would be worse than the defect being fixed.
		{name: "unparseable string", server: map[string]any{"command": "engram", "timeout": "soon"}, want: 0},
		{name: "negative", server: map[string]any{"command": "engram", "timeout": -3}, want: 0},
		{name: "zero", server: map[string]any{"command": "engram", "timeout": 0}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := stdioCommandFromServer(tt.server)
			if err != nil {
				t.Fatalf("stdioCommandFromServer() error = %v", err)
			}
			if command.Timeout != tt.want {
				t.Fatalf("Timeout = %v, want %v", command.Timeout, tt.want)
			}
		})
	}
}

// TestStdioProbeDeadlineHonorsTheConfiguredTimeout proves the value reaches
// the deadline instead of being parsed and dropped.
func TestStdioProbeDeadlineHonorsTheConfiguredTimeout(t *testing.T) {
	tests := []struct {
		name       string
		configured time.Duration
		want       time.Duration
	}{
		{name: "unset falls back to the default", configured: 0, want: stdioProbeTimeout},
		{name: "configured wins", configured: 15 * time.Second, want: 15 * time.Second},
		// A configured timeout shorter than the default is still the operator's
		// choice: they asked for a tighter bound and get one.
		{name: "shorter than default is respected", configured: 2 * time.Second, want: 2 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stdioProbeDeadline(tt.configured); got != tt.want {
				t.Fatalf("stdioProbeDeadline(%v) = %v, want %v", tt.configured, got, tt.want)
			}
		})
	}
}

// TestDefaultStdioProbeTimeoutClearsAMeasuredHealthyHandshake pins the default
// against the evidence rather than against taste.
//
// #3068 measured a healthy store at 4.87 to 5.16 seconds for the handshake
// alone, on top of which the probe also pays process spawn. A default that
// merely exceeds the measurement by a hair reproduces the same flap on a
// slower machine, so it carries real headroom over the slowest observed value.
func TestDefaultStdioProbeTimeoutClearsAMeasuredHealthyHandshake(t *testing.T) {
	const slowestObservedHealthyHandshake = 5160 * time.Millisecond
	if stdioProbeTimeout <= slowestObservedHealthyHandshake {
		t.Fatalf("default probe timeout %v does not clear the healthy handshake #3068 measured (%v)", stdioProbeTimeout, slowestObservedHealthyHandshake)
	}
	if stdioProbeTimeout < 2*slowestObservedHealthyHandshake {
		t.Fatalf("default probe timeout %v leaves less than 2x headroom over the slowest observed healthy handshake (%v); a slower machine reproduces #3068", stdioProbeTimeout, slowestObservedHealthyHandshake)
	}
}
