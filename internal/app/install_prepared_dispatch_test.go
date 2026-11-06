package app

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestRunArgsPreparedInstallPreservesValidDispatch(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		components string
	}{
		{"implicit persona", []string{"--component", "theme"}, "theme"},
		{"custom opt-out", []string{"--component", "theme", "--persona", "custom"}, "theme"},
		{"managed persona", []string{"--component", "persona", "--persona", "neutral"}, "persona"},
		{"skill", []string{"--component", "skills", "--skill", "go-testing"}, "skills"},
		{"skills alias", []string{"--components", "skills", "--skills", "go-testing"}, "skills"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupMockHome(t, t.TempDir())
			oldSelf, oldDetect, oldEnsure := selfUpdateFn, detectSystem, ensureCurrentOSSupported
			t.Cleanup(func() { selfUpdateFn, detectSystem, ensureCurrentOSSupported = oldSelf, oldDetect, oldEnsure })
			ensureCurrentOSSupported = func() error { return nil }
			detectSystem = func(context.Context) (system.DetectionResult, error) {
				return system.DetectionResult{System: system.SystemInfo{Supported: true, Profile: system.PlatformProfile{OS: "windows", Supported: true}}}, nil
			}
			updates := 0
			selfUpdateFn = func(context.Context, string, system.PlatformProfile, io.Writer) error { updates++; return nil }
			var out bytes.Buffer
			args := append([]string{"install", "--agent", "claude-code", "--dry-run"}, tc.args...)
			if err := RunArgs(args, &out); err != nil {
				t.Fatal(err)
			}
			if updates != 1 {
				t.Fatalf("self-update calls = %d, want 1", updates)
			}
			if !strings.Contains(out.String(), "Components order: "+tc.components+"\n") {
				t.Fatalf("unexpected plan output: %q", out.String())
			}
		})
	}
}
