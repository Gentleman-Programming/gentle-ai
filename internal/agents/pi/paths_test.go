package pi

import (
	"path/filepath"
	"testing"
)

func TestResolvePaths(t *testing.T) {
	tests := []struct {
		name         string
		homeDir      string
		envValue     string
		wantRoot     string
		wantAgents   string
		wantSettings string
	}{
		{
			name:         "environment override takes precedence",
			homeDir:      "/home/tester",
			envValue:     "/tmp/custom-pi",
			wantRoot:     "/tmp/custom-pi",
			wantAgents:   "/tmp/custom-pi/agents",
			wantSettings: "/tmp/custom-pi/settings.json",
		},
		{
			name:         "default path used when override missing",
			homeDir:      "/home/tester",
			envValue:     "",
			wantRoot:     filepath.Join("/home/tester", ".pi", "agent"),
			wantAgents:   filepath.Join("/home/tester", ".pi", "agent", "agents"),
			wantSettings: filepath.Join("/home/tester", ".pi", "agent", "settings.json"),
		},
		{
			name:         "blank override falls back to default",
			homeDir:      "/home/tester",
			envValue:     "   ",
			wantRoot:     filepath.Join("/home/tester", ".pi", "agent"),
			wantAgents:   filepath.Join("/home/tester", ".pi", "agent", "agents"),
			wantSettings: filepath.Join("/home/tester", ".pi", "agent", "settings.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := ResolvePaths(tt.homeDir, func(key string) string {
				if key != piCodingAgentDirEnv {
					t.Fatalf("ResolvePaths() requested env key %q, want %q", key, piCodingAgentDirEnv)
				}
				return tt.envValue
			})

			if paths.Root != tt.wantRoot {
				t.Fatalf("ResolvePaths().Root = %q, want %q", paths.Root, tt.wantRoot)
			}

			if paths.AgentsDir != tt.wantAgents {
				t.Fatalf("ResolvePaths().AgentsDir = %q, want %q", paths.AgentsDir, tt.wantAgents)
			}

			if paths.SettingsPath != tt.wantSettings {
				t.Fatalf("ResolvePaths().SettingsPath = %q, want %q", paths.SettingsPath, tt.wantSettings)
			}
		})
	}
}

func TestResolvePathsDetectionCandidatesIncludeLegacyCompatibility(t *testing.T) {
	homeDir := "/home/tester"
	paths := ResolvePaths(homeDir, func(string) string { return "" })

	got := detectionConfigCandidates(paths)
	want := []string{
		filepath.Join(homeDir, ".pi", "agent"),
		filepath.Join(homeDir, ".config", "pi-coding-agent"),
	}

	if len(got) != len(want) {
		t.Fatalf("detectionConfigCandidates() len = %d, want %d (%v)", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("detectionConfigCandidates()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
