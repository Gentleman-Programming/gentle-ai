package app

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/telemetry"
	"github.com/gentleman-programming/gentle-ai/v3/internal/testenv"
)

// TestMain gives the whole internal/app test binary a safe, sandboxed HOME
// (mirroring internal/cli's own TestMain, which many of this package's tests
// indirectly exercise through cli.RunInstall/RunSync/RunTelemetry) and closes
// telemetry's own hermeticity gap for this package specifically: runUpdate
// resolves the user's home directory directly (it has no cli-level override
// var to reuse) and hands it to cli.TelemetryTrigger, so without this,
// os.UserHomeDir() would read whatever HOME happens to be set to in the
// process running `go test` — the developer's real home on a machine that
// has not sandboxed it for this specific test.
//
// The same two telemetry defaults as internal/cli's TestMain apply here:
// DO_NOT_TRACK=1 so telemetry.Decide refuses before any state file is
// touched, and a RecordingSpawner so even a test that re-enables telemetry
// for its own scope can never start a real process or reach the network.
func TestMain(m *testing.M) {
	// Neutralize ambient agent runtime-dir overrides (PI_CODING_AGENT_DIR,
	// OPENCODE_CONFIG_DIR): this package's catalog.AllAgents() loops resolve
	// Pi's config path directly from the environment, independent of the
	// sandboxed HOME set up below.
	testenv.Isolate()
	testHome, err := os.MkdirTemp("", "gentle-ai-app-test-home-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", testHome); err != nil {
		panic(err)
	}
	if err := os.Setenv("USERPROFILE", testHome); err != nil {
		panic(err)
	}
	if err := os.Setenv("DO_NOT_TRACK", "1"); err != nil {
		panic(err)
	}
	telemetry.DefaultSpawn = telemetry.NewRecordingSpawner().Spawn

	code := m.Run()
	_ = os.RemoveAll(testHome)
	os.Exit(code)
}
