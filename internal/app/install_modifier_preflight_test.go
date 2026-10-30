package app

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestRunArgsOrphanInstallModifiersBeforeSelfUpdate(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"persona", []string{"--persona", "neutral"}, "--persona requires component persona in the resolved install plan; rerun gentle-ai install with --component persona added to your existing component list, or remove --persona"},
		{"skill", []string{"--skill", "go-testing"}, "--skill/--skills requires component skills in the resolved install plan; rerun gentle-ai install with --component skills added to your existing component list, or remove --skill/--skills"},
		{"skills", []string{"--skills", "go-testing"}, "--skill/--skills requires component skills in the resolved install plan; rerun gentle-ai install with --component skills added to your existing component list, or remove --skill/--skills"},
	} {
		for _, dryRun := range []bool{false, true} {
			name := tc.name + "/real"
			if dryRun {
				name = tc.name + "/dry-run"
			}
			t.Run(name, func(t *testing.T) {
				home := t.TempDir()
				setupMockHome(t, home)
				timestamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				before := state.InstallState{InstalledAgents: []string{"claude-code"}, Persona: "custom", LastUpdateCheck: &timestamp}
				if err := state.Write(home, before); err != nil {
					t.Fatal(err)
				}
				before, err := state.Read(home)
				if err != nil {
					t.Fatal(err)
				}
				oldSelf, oldDetect, oldEnsure := selfUpdateFn, detectSystem, ensureCurrentOSSupported
				t.Cleanup(func() { selfUpdateFn, detectSystem, ensureCurrentOSSupported = oldSelf, oldDetect, oldEnsure })
				ensureCurrentOSSupported = func() error { return nil }
				detectSystem = func(context.Context) (system.DetectionResult, error) {
					return system.DetectionResult{System: system.SystemInfo{Supported: true, Profile: system.PlatformProfile{OS: "windows", Supported: true}}}, nil
				}
				updates := 0
				selfUpdateFn = func(context.Context, string, system.PlatformProfile, io.Writer) error {
					updates++
					next := before
					now := timestamp.Add(time.Hour)
					next.LastUpdateCheck = &now
					return state.Write(home, next)
				}
				args := append([]string{"install", "--agent", "claude-code", "--component", "theme"}, tc.args...)
				if dryRun {
					args = append(args, "--dry-run")
				}
				var out bytes.Buffer
				err = RunArgs(args, &out)
				if err == nil || err.Error() != tc.want {
					t.Fatalf("error = %v, want %q", err, tc.want)
				}
				if out.String() != "" {
					t.Fatalf("stdout = %q, want empty", out.String())
				}
				if updates != 0 {
					t.Fatalf("self-update calls = %d, want 0", updates)
				}
				after, err := state.Read(home)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(after, before) {
					t.Fatalf("rejection changed state: before=%+v after=%+v", before, after)
				}
			})
		}
	}
}
