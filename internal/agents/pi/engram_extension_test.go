package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEngramExtension(t *testing.T) {
	tests := []struct {
		name                 string
		settings             string
		setup                func(t *testing.T, paths Paths)
		wantOK               bool
		wantErrSub           string
		wantSource           string
		wantPackageName      string
		wantPathSuffix       string
		wantExactPath        string
		wantResolvedFromRoot string
	}{
		{
			name:           "configured extension contract passes",
			settings:       `{"engram":{"extension":{"path":"/opt/pi/extensions/pi-engram","enabled":true}}}`,
			wantOK:         true,
			wantSource:     "explicit",
			wantPathSuffix: "pi-engram",
		},
		{
			name:                 "local packages entry resolves from pi root",
			settings:             `{"packages":["../../Documents/repos/pi-engram"]}`,
			setup: func(t *testing.T, paths Paths) {
				t.Helper()
				fixtureDir := filepath.Clean(filepath.Join(paths.Root, "../../Documents/repos/pi-engram"))
				if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
					t.Fatalf("MkdirAll(local package fixture) error = %v", err)
				}
			},
			wantOK:               true,
			wantSource:           "package-local",
			wantPathSuffix:       filepath.Join("Documents", "repos", "pi-engram"),
			wantResolvedFromRoot: "../../Documents/repos/pi-engram",
		},
		{
			name:                 "tilde local packages entry expands to home",
			settings:             `{"packages":["~/Documents/repos/pi-engram"]}`,
			setup: func(t *testing.T, _ Paths) {
				t.Helper()
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				fixtureDir := filepath.Join(homeDir, "Documents", "repos", "pi-engram")
				if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
					t.Fatalf("MkdirAll(tilde package fixture) error = %v", err)
				}
			},
			wantOK:               true,
			wantSource:           "package-local",
			wantPathSuffix:       filepath.Join("Documents", "repos", "pi-engram"),
			wantExactPath:        "",
			wantResolvedFromRoot: "",
		},
		{
			name:       "local packages entry fails when path does not exist",
			settings:   `{"packages":["../../Documents/repos/pi-engram"]}`,
			wantErrSub: "does not exist or is not readable",
		},
		{
			name:            "npm package entry passes",
			settings:        `{"packages":["npm:pi-engram"]}`,
			wantOK:          true,
			wantSource:      "package-npm",
			wantPackageName: "pi-engram",
			wantExactPath:   "npm:pi-engram",
		},
		{
			name:            "scoped npm package entry passes",
			settings:        `{"packages":["npm:@gentleman-programming/pi-engram"]}`,
			wantOK:          true,
			wantSource:      "package-npm",
			wantPackageName: "@gentleman-programming/pi-engram",
			wantExactPath:   "npm:@gentleman-programming/pi-engram",
		},
		{
			name:       "missing extension contract fails",
			settings:   `{}`,
			wantErrSub: "expected engram.extension.path or packages[]",
		},
		{
			name:       "malformed extension path fails",
			settings:   `{"engram":{"extension":{"path":123}}}`,
			wantErrSub: "malformed PI→Engram extension contract",
		},
		{
			name:       "malformed packages type fails",
			settings:   `{"packages":{}}`,
			wantErrSub: "packages must be an array",
		},
		{
			name:       "unrelated packages entry fails closed",
			settings:   `{"packages":["npm:random-package","../../plugins/not-engram"]}`,
			wantErrSub: "expected engram.extension.path or packages[]",
		},
		{
			name:       "invalid settings json fails",
			settings:   `{"engram":`,
			wantErrSub: "parse PI settings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			paths := ResolvePaths(home, func(string) string { return "" })
			if tt.setup != nil {
				tt.setup(t, paths)
			}
			if err := os.MkdirAll(filepath.Dir(paths.SettingsPath), 0o755); err != nil {
				t.Fatalf("MkdirAll(settings dir) error = %v", err)
			}
			if err := os.WriteFile(paths.SettingsPath, []byte(tt.settings), 0o644); err != nil {
				t.Fatalf("WriteFile(settings) error = %v", err)
			}

			status, err := ValidateEngramExtension(paths)

			if tt.wantErrSub != "" {
				if err == nil {
					t.Fatalf("ValidateEngramExtension() error = nil, want containing %q", tt.wantErrSub)
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("ValidateEngramExtension() error = %q, want substring %q", err.Error(), tt.wantErrSub)
				}
				return
			}

			if err != nil {
				t.Fatalf("ValidateEngramExtension() error = %v, want nil", err)
			}
			if status.Configured != tt.wantOK {
				t.Fatalf("ValidateEngramExtension().Configured = %v, want %v", status.Configured, tt.wantOK)
			}
			if status.ExtensionPath == "" {
				t.Fatal("ValidateEngramExtension().ExtensionPath = empty, want non-empty")
			}
			if tt.wantSource != "" && status.Source != tt.wantSource {
				t.Fatalf("ValidateEngramExtension().Source = %q, want %q", status.Source, tt.wantSource)
			}
			if tt.wantPackageName != "" && status.PackageName != tt.wantPackageName {
				t.Fatalf("ValidateEngramExtension().PackageName = %q, want %q", status.PackageName, tt.wantPackageName)
			}
			if tt.wantExactPath != "" && status.ExtensionPath != tt.wantExactPath {
				t.Fatalf("ValidateEngramExtension().ExtensionPath = %q, want exact %q", status.ExtensionPath, tt.wantExactPath)
			}
			if tt.wantPathSuffix != "" && !strings.HasSuffix(filepath.Clean(status.ExtensionPath), filepath.Clean(tt.wantPathSuffix)) {
				t.Fatalf("ValidateEngramExtension().ExtensionPath = %q, want suffix %q", status.ExtensionPath, tt.wantPathSuffix)
			}
			if tt.wantResolvedFromRoot != "" {
				wantPath := filepath.Clean(filepath.Join(paths.Root, tt.wantResolvedFromRoot))
				if filepath.Clean(status.ExtensionPath) != wantPath {
					t.Fatalf("ValidateEngramExtension().ExtensionPath = %q, want resolved %q", status.ExtensionPath, wantPath)
				}
			}
		})
	}
}

func TestValidateEngramExtensionMissingSettingsFile(t *testing.T) {
	home := t.TempDir()
	paths := ResolvePaths(home, func(string) string { return "" })

	_, err := ValidateEngramExtension(paths)
	if err == nil {
		t.Fatal("ValidateEngramExtension() error = nil, want missing contract error")
	}
	if !strings.Contains(err.Error(), "missing PI→Engram extension contract") {
		t.Fatalf("ValidateEngramExtension() error = %q, want missing contract message", err.Error())
	}
}
