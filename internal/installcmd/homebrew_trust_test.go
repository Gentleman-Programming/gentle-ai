package installcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestResolveComponentInstallHomebrewExecutable(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "brew")
	if err := os.WriteFile(fixture, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"PATH", "/opt/homebrew/bin/brew", "/usr/local/bin/brew", "/home/linuxbrew/.linuxbrew/bin/brew", "missing"} {
		t.Run(target, func(t *testing.T) {
			originalLookPath, originalStat, originalProbe := cmdLookPath, osStat, cmdBrewHelpTrust
			t.Cleanup(func() { cmdLookPath, osStat, cmdBrewHelpTrust = originalLookPath, originalStat, originalProbe })
			want := filepath.FromSlash(target)
			cmdLookPath = func(name string) (string, error) {
				if name != "brew" {
					t.Fatalf("unexpected lookup %q", name)
				}
				if target == "PATH" {
					return fixture, nil
				}
				return "", os.ErrNotExist
			}
			osStat = func(path string) (os.FileInfo, error) {
				if path == want {
					return info, nil
				}
				return nil, os.ErrNotExist
			}
			if target == "PATH" {
				want = fixture
			}
			if target == "missing" {
				want = "brew"
			}
			calls := 0
			cmdBrewHelpTrust = func(brew string) error {
				calls++
				if brew != want {
					t.Errorf("trust probe executable = %q, want %q", brew, want)
				}
				return nil
			}
			got, err := NewResolver().ResolveComponentInstall(system.PlatformProfile{OS: "darwin", PackageManager: "brew"}, model.ComponentEngram)
			expected := CommandSequence{{"brew", "tap", "Gentleman-Programming/homebrew-tap"}, {"brew", "trust", "--formula", "gentleman-programming/tap/engram"}, {"brew", "install", "engram"}}
			if err != nil || !reflect.DeepEqual(got, expected) || calls != 1 {
				t.Fatalf("got %v, %v, %d probes; want %v, nil, 1", got, err, calls, expected)
			}
		})
	}
}

func TestResolveComponentInstallHomebrewTrust(t *testing.T) {
	for _, os := range []string{"darwin", "linux"} {
		for _, component := range []model.ComponentID{model.ComponentEngram, model.ComponentGGA} {
			for _, supported := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%s/trust=%t", os, component, supported), func(t *testing.T) {
					original := cmdBrewHelpTrust
					calls := 0
					cmdBrewHelpTrust = func(string) error {
						calls++
						if !supported {
							return fmt.Errorf("unknown command: trust")
						}
						return nil
					}
					t.Cleanup(func() { cmdBrewHelpTrust = original })
					name := string(component)
					action := "install"
					if component == model.ComponentGGA {
						action = "reinstall"
					}
					want := CommandSequence{{"brew", "tap", "Gentleman-Programming/homebrew-tap"}}
					if supported {
						want = append(want, []string{"brew", "trust", "--formula", "gentleman-programming/tap/" + name})
					}
					want = append(want, []string{"brew", action, name})
					got, err := NewResolver().ResolveComponentInstall(system.PlatformProfile{OS: os, PackageManager: "brew"}, component)
					if err != nil || !reflect.DeepEqual(got, want) {
						t.Fatalf("ResolveComponentInstall() = %v, %v; want %v, nil", got, err, want)
					}
					if calls != 1 {
						t.Fatalf("capability probes = %d, want 1", calls)
					}
				})
			}
		}
	}
}
