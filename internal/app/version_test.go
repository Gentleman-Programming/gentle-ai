package app

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name           string
		ldflagsVersion string
		buildInfo      *debug.BuildInfo
		buildInfoOK    bool
		want           string
	}{
		{
			name:           "ldflags set to release version",
			ldflagsVersion: "1.8.3",
			buildInfo:      nil,
			buildInfoOK:    false,
			want:           "1.8.3",
		},
		{
			name:           "go install with tagged semver",
			ldflagsVersion: "dev",
			buildInfo:      &debug.BuildInfo{Main: debug.Module{Version: "v1.8.3"}},
			buildInfoOK:    true,
			want:           "1.8.3",
		},
		{
			name:           "go install with devel sentinel",
			ldflagsVersion: "dev",
			buildInfo:      &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			buildInfoOK:    true,
			want:           "dev",
		},
		{
			name:           "go install with empty BuildInfo version",
			ldflagsVersion: "dev",
			buildInfo:      &debug.BuildInfo{Main: debug.Module{Version: ""}},
			buildInfoOK:    true,
			want:           "dev",
		},
		{
			name:           "ReadBuildInfo returns ok=false",
			ldflagsVersion: "dev",
			buildInfo:      nil,
			buildInfoOK:    false,
			want:           "dev",
		},
		{
			name:           "ldflags set to non-dev value takes priority over BuildInfo",
			ldflagsVersion: "2.0.0",
			buildInfo:      &debug.BuildInfo{Main: debug.Module{Version: "v1.5.0"}},
			buildInfoOK:    true,
			want:           "2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := buildInfoReader
			t.Cleanup(func() { buildInfoReader = orig })

			buildInfoReader = func() (*debug.BuildInfo, bool) {
				return tt.buildInfo, tt.buildInfoOK
			}

			got := ResolveVersion(tt.ldflagsVersion)
			if got != tt.want {
				t.Errorf("ResolveVersion(%q) = %q, want %q", tt.ldflagsVersion, got, tt.want)
			}
		})
	}
}
