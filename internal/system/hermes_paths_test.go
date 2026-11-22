package system_test

import (
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/system"
)

// TestHermesConfigDirFor checks the Hermes home resolution order:
// HERMES_HOME, then %LOCALAPPDATA%\hermes on Windows, then ~/.hermes.
// Environment overrides apply only to the real user home.
func TestHermesConfigDirFor(t *testing.T) {
	tests := []struct {
		name       string
		goos       string
		hermesHome string // "" leaves HERMES_HOME empty
		isolated   bool   // homeDir differs from the real user home
		want       func(home, localAppData, hermesHome string) string
	}{
		{
			name:       "HERMES_HOME wins on windows",
			goos:       "windows",
			hermesHome: "custom-hermes",
			want:       func(_, _, hh string) string { return hh },
		},
		{
			name:       "HERMES_HOME wins on linux",
			goos:       "linux",
			hermesHome: "custom-hermes",
			want:       func(_, _, hh string) string { return hh },
		},
		{
			name: "windows defaults to LOCALAPPDATA",
			goos: "windows",
			want: func(_, lad, _ string) string { return filepath.Join(lad, "hermes") },
		},
		{
			name: "linux defaults to dot hermes",
			goos: "linux",
			want: func(home, _, _ string) string { return filepath.Join(home, ".hermes") },
		},
		{
			name: "darwin defaults to dot hermes",
			goos: "darwin",
			want: func(home, _, _ string) string { return filepath.Join(home, ".hermes") },
		},
		{
			name:       "isolated home ignores HERMES_HOME",
			goos:       "linux",
			hermesHome: "custom-hermes",
			isolated:   true,
			want:       func(home, _, _ string) string { return filepath.Join(home, ".hermes") },
		},
		{
			name:       "isolated windows home ignores environment",
			goos:       "windows",
			hermesHome: "custom-hermes",
			isolated:   true,
			want: func(home, _, _ string) string {
				return filepath.Join(home, "AppData", "Local", "hermes")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			realHome := t.TempDir()
			t.Setenv("HOME", realHome)
			t.Setenv("USERPROFILE", realHome)
			localAppData := filepath.Join(realHome, "localappdata")
			t.Setenv("LOCALAPPDATA", localAppData)

			hermesHome := ""
			if tt.hermesHome != "" {
				hermesHome = filepath.Join(realHome, tt.hermesHome)
			}
			t.Setenv("HERMES_HOME", hermesHome)

			home := realHome
			if tt.isolated {
				home = t.TempDir()
			}

			got := system.HermesConfigDirFor(home, tt.goos)
			if want := tt.want(home, localAppData, hermesHome); got != want {
				t.Fatalf("HermesConfigDirFor(%q, %q) = %q, want %q", home, tt.goos, got, want)
			}
		})
	}
}

// TestHermesConfigDirIgnoresLocalAppDataOutsideHome checks that a home whose
// HOME/USERPROFILE were redirected (as tests and sandboxes do) never resolves
// to an ambient %LOCALAPPDATA% that lives outside it.
func TestHermesConfigDirIgnoresLocalAppDataOutsideHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HERMES_HOME", "")
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "ambient-localappdata"))

	got := system.HermesConfigDirFor(home, "windows")
	if want := filepath.Join(home, "AppData", "Local", "hermes"); got != want {
		t.Fatalf("HermesConfigDirFor() = %q, want %q", got, want)
	}
}

// TestHermesConfigDirRedirectedLocalAppData checks that %LOCALAPPDATA% outside
// the home is honored only when the home is the operating-system profile,
// which covers folder redirection without trusting a faked HOME/USERPROFILE.
func TestHermesConfigDirRedirectedLocalAppData(t *testing.T) {
	tests := []struct {
		name      string
		isProfile bool
		want      func(home, localAppData string) string
	}{
		{
			name:      "real profile with redirected LOCALAPPDATA",
			isProfile: true,
			want:      func(_, lad string) string { return filepath.Join(lad, "hermes") },
		},
		{
			name: "faked home with ambient LOCALAPPDATA",
			want: func(home, _ string) string { return filepath.Join(home, "AppData", "Local", "hermes") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("HERMES_HOME", "")
			localAppData := filepath.Join(t.TempDir(), "redirected-localappdata")
			t.Setenv("LOCALAPPDATA", localAppData)
			profile := filepath.Join(t.TempDir(), "other-profile")
			if tt.isProfile {
				profile = home
			}
			system.SetOSProfileDirForTest(t, func() (string, error) { return profile, nil })

			got := system.HermesConfigDirFor(home, "windows")
			if want := tt.want(home, localAppData); got != want {
				t.Fatalf("HermesConfigDirFor() = %q, want %q", got, want)
			}
		})
	}
}
