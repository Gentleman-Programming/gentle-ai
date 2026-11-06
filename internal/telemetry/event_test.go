package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuildEventNeverLeaksIdentifyingData builds an event from a fixture
// whose home directory, hostname, and username are realistic-looking and
// asserts none of those bytes ever reach the marshaled payload. This is the
// behavior the contract cares about most: the event carries only fixed
// enums, a random id, and counts.
func TestBuildEventNeverLeaksIdentifyingData(t *testing.T) {
	realHostname := "alans-macbook-pro.local"
	realUsername := "alanbuscaglia"
	// The fixture home directory embeds a realistic-looking username and
	// path shape (/Users/<name>/...), but lives under t.TempDir() so the
	// test never touches the real filesystem outside its own sandbox.
	realHomeDir := filepath.Join(t.TempDir(), "Users", realUsername, "gentle-ai-real-home")

	if err := os.MkdirAll(realHomeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	persisted, err := NewState()
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(realHomeDir, persisted); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(realHomeDir)
	if err != nil {
		t.Fatal(err)
	}

	ev := Build(BuildInput{
		Kind:       EventHeartbeat,
		InstallID:  loaded.InstallID,
		Now:        time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC),
		Version:    "2.2.0",
		Agents:     []string{"claude-code", "opencode"},
		Components: []string{"sdd", "engram"},
		RDDEnabled: true,
		Counters:   Counters{Syncs: 3},
	})
	payload, err := Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}

	haystack := string(payload)
	for _, forbidden := range []string{realHostname, realUsername, realHomeDir, os.TempDir()} {
		if strings.Contains(haystack, forbidden) {
			t.Fatalf("payload leaks %q:\n%s", forbidden, haystack)
		}
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"path", "cwd", "home", "hostname", "username", "prompt", "diff", "ip"} {
		if _, present := decoded[key]; present {
			t.Fatalf("payload carries unexpected field %q", key)
		}
	}
	if decoded["schema"] != EventSchema {
		t.Fatalf("schema = %v, want %v", decoded["schema"], EventSchema)
	}
	if len(payload) > MaxPayloadBytes {
		t.Fatalf("payload is %d bytes, want <= %d", len(payload), MaxPayloadBytes)
	}
}

func TestBuildInstallEventCarriesNoCounters(t *testing.T) {
	ev := Build(BuildInput{Kind: EventInstall, InstallID: "x", Now: time.Now(), Counters: Counters{Syncs: 5}})
	payload, err := Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "counters") {
		t.Fatalf("install event must never carry counters: %s", payload)
	}
}

func TestBuildHeartbeatEventCarriesCountersEvenWhenZero(t *testing.T) {
	ev := Build(BuildInput{Kind: EventHeartbeat, InstallID: "x", Now: time.Now()})
	payload, err := Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"counters"`) {
		t.Fatalf("heartbeat event must always carry counters: %s", payload)
	}
}
