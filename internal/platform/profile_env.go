// Package platform provides cross-platform utilities for manipulating the
// user's persistent shell environment.
package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// PersistEngramEnv writes ENGRAM_DATA_DIR to the user's persistent shell
// environment (Unix profile or Windows registry).
func PersistEngramEnv(dir string) error {
	if runtime.GOOS == "windows" {
		return persistWindows(dir)
	}
	return persistUnix(dir)
}

// RemoveEngramEnv removes ENGRAM_DATA_DIR from the user's persistent shell
// environment.
func RemoveEngramEnv() error {
	if runtime.GOOS == "windows" {
		return removeWindows()
	}
	return removeUnix()
}

// ---------------------------------------------------------------------------
// Unix implementation
// ---------------------------------------------------------------------------

func persistUnix(dir string) error {
	profiles := unixProfilePaths()
	if len(profiles) == 0 {
		return fmt.Errorf("could not determine shell profile path")
	}

	// Use the first existing profile, or create the first candidate.
	var target string
	for _, p := range profiles {
		if _, err := os.Stat(p); err == nil {
			target = p
			break
		}
	}
	if target == "" {
		target = profiles[0]
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}

	// Read existing content.
	var existing []byte
	if _, err := os.Stat(target); err == nil {
		existing, _ = os.ReadFile(target)
	}

	// Filter out old ENGRAM_DATA_DIR lines.
	lines := strings.Split(string(existing), "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "export ENGRAM_DATA_DIR=") ||
			strings.HasPrefix(trimmed, "set -gx ENGRAM_DATA_DIR") {
			continue
		}
		filtered = append(filtered, line)
	}

	// Append new entry.
	shell := detectShell()
	var entry string
	if shell == "fish" {
		entry = fmt.Sprintf("set -gx ENGRAM_DATA_DIR \"%s\"", dir)
	} else {
		entry = fmt.Sprintf("export ENGRAM_DATA_DIR=\"%s\"", dir)
	}
	filtered = append(filtered, "", "# Engram data directory (set by gentle-ai)", entry)

	// Atomic write via temp file.
	tmp, err := os.CreateTemp(filepath.Dir(target), ".profile-*")
	if err != nil {
		return fmt.Errorf("create temp profile: %w", err)
	}
	defer os.Remove(tmp.Name())

	content := strings.Join(filtered, "\n") + "\n"
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp profile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp profile: %w", err)
	}

	// Backup original before replacing.
	if _, err := os.Stat(target); err == nil {
		_ = os.Rename(target, target+".bak")
	}

	if err := os.Rename(tmp.Name(), target); err != nil {
		// Attempt to restore backup.
		_ = os.Rename(target+".bak", target)
		return fmt.Errorf("replace profile: %w", err)
	}

	return nil
}

func removeUnix() error {
	profiles := unixProfilePaths()
	for _, target := range profiles {
		if _, err := os.Stat(target); err != nil {
			continue
		}

		data, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("read profile %q: %w", target, err)
		}

		lines := strings.Split(string(data), "\n")
		var filtered []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "export ENGRAM_DATA_DIR=") ||
				strings.HasPrefix(trimmed, "set -gx ENGRAM_DATA_DIR") {
				continue
			}
			filtered = append(filtered, line)
		}

		// Only rewrite if something was removed.
		if len(filtered) != len(lines) {
			tmp, err := os.CreateTemp(filepath.Dir(target), ".profile-*")
			if err != nil {
				return fmt.Errorf("create temp profile: %w", err)
			}
			defer os.Remove(tmp.Name())

			content := strings.Join(filtered, "\n")
			if !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			if _, err := tmp.WriteString(content); err != nil {
				tmp.Close()
				return fmt.Errorf("write temp profile: %w", err)
			}
			if err := tmp.Close(); err != nil {
				return fmt.Errorf("close temp profile: %w", err)
			}

			if err := os.Rename(tmp.Name(), target); err != nil {
				return fmt.Errorf("replace profile: %w", err)
			}
		}
		return nil
	}
	return nil
}

func detectShell() string {
	s := os.Getenv("SHELL")
	if s == "" {
		return "bash"
	}
	return filepath.Base(s)
}

func unixProfilePaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	switch detectShell() {
	case "bash":
		return []string{
			filepath.Join(home, ".bash_profile"),
			filepath.Join(home, ".bashrc"),
		}
	case "zsh":
		return []string{
			filepath.Join(home, ".zshrc"),
		}
	case "fish":
		return []string{
			filepath.Join(home, ".config", "fish", "config.fish"),
		}
	default:
		return []string{
			filepath.Join(home, ".bashrc"),
			filepath.Join(home, ".zshrc"),
		}
	}
}

// ---------------------------------------------------------------------------
// Windows implementation
// ---------------------------------------------------------------------------

func persistWindows(dir string) error {
	cmd := exec.Command("cmd", "/c", "setx", "ENGRAM_DATA_DIR", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("setx ENGRAM_DATA_DIR: %w\noutput: %s", err, string(out))
	}
	return nil
}

func removeWindows() error {
	cmd := exec.Command("cmd", "/c", "REG", "DELETE", "HKCU\\Environment", "/V", "ENGRAM_DATA_DIR", "/F")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// It's OK if the key doesn't exist.
		if strings.Contains(string(out), "ERROR: The system was unable to find the specified registry key") {
			return nil
		}
		return fmt.Errorf("reg delete ENGRAM_DATA_DIR: %w\noutput: %s", err, string(out))
	}
	return nil
}
