package cli

import (
	"reflect"
	"testing"
)

func TestCommandEnvLeavesNonBrewCommandsUnchanged(t *testing.T) {
	base := []string{"HOME=/home/dev", "PATH=/usr/bin"}
	got := commandEnv("npm", []string{"install", "-g", "pkg"}, base)
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("commandEnv(npm, ...) = %v, want unchanged %v", got, base)
	}
}

func TestCommandEnvAddsHomebrewNoAutoUpdateForBrewInstall(t *testing.T) {
	base := []string{"HOME=/home/dev"}
	got := commandEnv("brew", []string{"install", "engram"}, base)
	want := []string{"HOME=/home/dev", "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_INSTALL_CLEANUP=1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commandEnv(brew install, ...) = %v, want %v", got, want)
	}
}

func TestCommandEnvAddsHomebrewNoAutoUpdateForResolvedBrewPath(t *testing.T) {
	base := []string{"HOME=/home/dev"}
	got := commandEnv("/opt/homebrew/bin/brew", []string{"tap", "Gentleman-Programming/homebrew-tap"}, base)
	want := []string{"HOME=/home/dev", "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_INSTALL_CLEANUP=1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commandEnv(resolved brew path, ...) = %v, want %v", got, want)
	}
}

func TestCommandEnvExcludesBrewUpdate(t *testing.T) {
	base := []string{"HOME=/home/dev"}
	got := commandEnv("brew", []string{"update"}, base)
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("commandEnv(brew update, ...) = %v, want unchanged %v", got, base)
	}
}

func TestCommandEnvExcludesBrewUpgrade(t *testing.T) {
	base := []string{"HOME=/home/dev"}
	got := commandEnv("brew", []string{"upgrade", "--formula", "engram"}, base)
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("commandEnv(brew upgrade, ...) = %v, want unchanged %v", got, base)
	}
}

func TestCommandEnvNeverOverridesExplicitUserValue(t *testing.T) {
	base := []string{"HOME=/home/dev", "HOMEBREW_NO_AUTO_UPDATE=0"}
	got := commandEnv("brew", []string{"install", "engram"}, base)
	want := []string{"HOME=/home/dev", "HOMEBREW_NO_AUTO_UPDATE=0", "HOMEBREW_NO_INSTALL_CLEANUP=1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commandEnv() = %v, want %v (explicit HOMEBREW_NO_AUTO_UPDATE preserved)", got, want)
	}
}
