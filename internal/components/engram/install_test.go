package engram

import (
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func TestInstallCommandByProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile system.PlatformProfile
		want    [][]string
		wantErr bool
	}{
		{
			name:    "darwin uses brew tap and install",
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			want:    [][]string{{"brew", "tap", "Gentleman-Programming/homebrew-tap"}, {"brew", "install", "engram"}},
		},
		// Linux and Windows now use DownloadLatestBinary() — InstallCommand returns an error
		// to signal that callers must use the direct download path instead.
		{
			name:    "ubuntu returns error (uses DownloadLatestBinary instead of go install)",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
			wantErr: true,
		},
		{
			name:    "arch returns error (uses DownloadLatestBinary instead of go install)",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			wantErr: true,
		},
		{
			name:    "fedora returns error (uses DownloadLatestBinary instead of go install)",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf"},
			wantErr: true,
		},
		{
			name:    "unsupported package manager returns error",
			profile: system.PlatformProfile{OS: "linux", PackageManager: "zypper"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := InstallCommand(tt.profile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("InstallCommand() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if !reflect.DeepEqual(command, tt.want) {
				t.Fatalf("InstallCommand() = %v, want %v", command, tt.want)
			}
		})
	}
}
