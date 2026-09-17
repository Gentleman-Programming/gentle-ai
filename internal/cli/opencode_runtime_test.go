package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
)

func TestOpenCodeTelemetryStepUsesDetectedMajor(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	for _, version := range []string{"2.0.4", "unknown"} {
		t.Run(version, func(t *testing.T) {
			opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
				return opencode.CommandOutput{Stdout: []byte(version)}, nil
			}
			dir := filepath.Join(t.TempDir(), "config")
			err := (openCodeTelemetryStep{configDir: dir}).Run()
			if version == "unknown" {
				if err == nil {
					t.Fatal("unknown major accepted")
				}
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Fatal("unknown major created config")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "plugins", "telemetry-runtime.ts"))
			if err != nil || string(data) != assets.MustRead("opencode/plugins-v2/telemetry-runtime.ts") {
				t.Fatalf("wrong production asset: %v", err)
			}
		})
	}
}
