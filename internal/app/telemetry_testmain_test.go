package app

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
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
