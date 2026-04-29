package pi

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectPiSubagents(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T, adapter *Adapter, homeDir string, workspaceDir string)
		wantDetected bool
		wantErr      bool
	}{
		{
			name: "detects extension from home config footprint",
			setup: func(t *testing.T, adapter *Adapter, homeDir string, workspaceDir string) {
				t.Helper()
				footprints := adapter.PiSubagentsFootprints(homeDir, workspaceDir)
				if err := os.MkdirAll(footprints[0], 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) error = %v", footprints[0], err)
				}
			},
			wantDetected: true,
		},
		{
			name:         "returns false when all footprints are absent",
			setup:        func(t *testing.T, _ *Adapter, _ string, _ string) {},
			wantDetected: false,
		},
		{
			name: "returns error when footprint lookup is ambiguous",
			setup: func(t *testing.T, _ *Adapter, homeDir string, _ string) {
				t.Helper()
				blockingPath := filepath.Join(homeDir, ".config", "pi-coding-agent", "extensions")
				if err := os.MkdirAll(filepath.Dir(blockingPath), 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(blockingPath), err)
				}
				if err := os.WriteFile(blockingPath, []byte("not-a-dir"), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) error = %v", blockingPath, err)
				}
			},
			wantErr: true,
		},
		{
			name: "detects extension from global pi settings packages",
			setup: func(t *testing.T, _ *Adapter, homeDir string, _ string) {
				t.Helper()
				settingsPath := filepath.Join(homeDir, ".pi", "agent", "settings.json")
				writeSettingsJSON(t, settingsPath, `{"packages":["npm:pi-btw","npm:pi-subagents"]}`)
			},
			wantDetected: true,
		},
		{
			name: "detects extension from workspace pi settings packages",
			setup: func(t *testing.T, _ *Adapter, _ string, workspaceDir string) {
				t.Helper()
				settingsPath := filepath.Join(workspaceDir, ".pi", "settings.json")
				writeSettingsJSON(t, settingsPath, `{"packages":["npm:pi-subagents"]}`)
			},
			wantDetected: true,
		},
		{
			name: "settings package absent stays undetected without footprint",
			setup: func(t *testing.T, _ *Adapter, homeDir string, _ string) {
				t.Helper()
				settingsPath := filepath.Join(homeDir, ".pi", "agent", "settings.json")
				writeSettingsJSON(t, settingsPath, `{"packages":["npm:pi-btw","npm:@plannotator/pi-extension"]}`)
			},
			wantDetected: false,
		},
		{
			name: "settings package absent still detects via legacy footprint",
			setup: func(t *testing.T, adapter *Adapter, homeDir string, workspaceDir string) {
				t.Helper()
				settingsPath := filepath.Join(homeDir, ".pi", "agent", "settings.json")
				writeSettingsJSON(t, settingsPath, `{"packages":["npm:pi-btw"]}`)

				footprints := adapter.PiSubagentsFootprints(homeDir, workspaceDir)
				if err := os.MkdirAll(footprints[0], 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) error = %v", footprints[0], err)
				}
			},
			wantDetected: true,
		},
		{
			name: "malformed settings returns error",
			setup: func(t *testing.T, _ *Adapter, homeDir string, _ string) {
				t.Helper()
				settingsPath := filepath.Join(homeDir, ".pi", "agent", "settings.json")
				writeSettingsJSON(t, settingsPath, `{"packages":["npm:pi-subagents"`)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			workspaceDir := t.TempDir()
			adapter := NewAdapter()

			tt.setup(t, adapter, homeDir, workspaceDir)

			detected, err := adapter.DetectPiSubagents(homeDir, workspaceDir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DetectPiSubagents() error = %v, wantErr %v", err, tt.wantErr)
			}

			if detected != tt.wantDetected {
				t.Fatalf("DetectPiSubagents() detected = %v, want %v", detected, tt.wantDetected)
			}
		})
	}
}

func writeSettingsJSON(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func TestPiSubagentsFootprints(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	adapter := NewAdapter()
	got := adapter.PiSubagentsFootprints(homeDir, workspaceDir)

	want := []string{
		filepath.Join(homeDir, ".pi", "agent", "extensions", "pi-subagents"),
		filepath.Join(homeDir, ".pi", "agent", "extensions", "pi-subagents.json"),
		filepath.Join(homeDir, ".config", "pi-coding-agent", "extensions", "pi-subagents"),
		filepath.Join(homeDir, ".config", "pi-coding-agent", "extensions", "pi-subagents.json"),
		filepath.Join(workspaceDir, ".pi", "extensions", "pi-subagents"),
		filepath.Join(workspaceDir, ".pi", "extensions", "pi-subagents.json"),
	}

	for _, expected := range want {
		if !containsString(got, expected) {
			t.Fatalf("PiSubagentsFootprints() missing %q in %v", expected, got)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestDetectPiSubagentsFailsClosedOnDetectorErrors(t *testing.T) {
	adapter := &Adapter{
		statPath: func(string) statResult {
			return statResult{err: errors.New("permission denied")}
		},
	}

	detected, err := adapter.DetectPiSubagents("/tmp/home", "/tmp/workspace")
	if err == nil {
		t.Fatal("DetectPiSubagents() error = nil, want error")
	}

	if detected {
		t.Fatal("DetectPiSubagents() detected = true, want false")
	}
}

func TestDetectPiSubagentsUsesSettingsBeforeLegacyFootprints(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	settingsPath := filepath.Join(homeDir, ".pi", "agent", "settings.json")
	writeSettingsJSON(t, settingsPath, `{"packages":["npm:pi-subagents"]}`)

	adapter := &Adapter{
		statPath: func(string) statResult {
			return statResult{err: errors.New("permission denied")}
		},
	}

	detected, err := adapter.DetectPiSubagents(homeDir, workspaceDir)
	if err != nil {
		t.Fatalf("DetectPiSubagents() error = %v, want nil", err)
	}

	if !detected {
		t.Fatal("DetectPiSubagents() detected = false, want true")
	}
}

func TestDetectPiSubagentsFindsWorkspaceSettingsFromNestedDirectory(t *testing.T) {
	homeDir := t.TempDir()
	projectRoot := t.TempDir()
	workspaceDir := filepath.Join(projectRoot, "services", "billing")

	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspaceDir) error = %v", err)
	}
	writeSettingsJSON(t, filepath.Join(projectRoot, ".pi", "settings.json"), `{"packages":["npm:pi-subagents"]}`)

	adapter := NewAdapter()
	detected, err := adapter.DetectPiSubagents(homeDir, workspaceDir)
	if err != nil {
		t.Fatalf("DetectPiSubagents() error = %v", err)
	}
	if !detected {
		t.Fatal("DetectPiSubagents() detected = false, want true for ancestor .pi/settings.json")
	}
}
