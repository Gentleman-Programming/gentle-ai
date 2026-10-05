package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func issue3561DanglingAncestorFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}

	// The sandbox pre-creates ~/.config as a real directory; replace it with
	// a symlink whose target does not exist so the managed opencode path
	// (~/.config/opencode) sits below a dangling ANCESTOR while not being a
	// symlink itself.
	ancestorPath := filepath.Join(sandbox.Home, ".config")
	targetPath := filepath.Join(sandbox.Root, "missing-external-config-root")
	if err := os.Remove(ancestorPath); err != nil {
		return err
	}
	if err := os.Symlink(targetPath, ancestorPath); err != nil {
		if !errors.Is(err, windowsErrorPrivilegeNotHeld) {
			return err
		}
		if mkdirErr := os.MkdirAll(ancestorPath, 0o755); mkdirErr != nil {
			return mkdirErr
		}
		sandbox.Scratch["issue-3561-symlink-unavailable"] = err.Error()
		return nil
	}
	if err := os.MkdirAll(filepath.Join(sandbox.Home, ".claude"), 0o755); err != nil {
		return err
	}
	statePath := filepath.Join(sandbox.Home, ".gentle-ai", "state.json")
	state := fmt.Sprintf(`{"installed_agents":["opencode","claude-code"],"last_update_check":%q}`, time.Now().UTC().Format(time.RFC3339Nano))
	if err := sandbox.write(statePath, state); err != nil {
		return err
	}
	claudeConfig := `{"mcpServers":{"engram":{"command":"engram-not-installed-for-issue-3561","args":[]}}}`
	if err := sandbox.write(filepath.Join(sandbox.Home, ".claude.json"), claudeConfig); err != nil {
		return err
	}

	sandbox.Scratch["issue-3561-ancestor"] = ancestorPath
	sandbox.Scratch["issue-3561-config"] = filepath.Join(ancestorPath, "opencode")
	sandbox.Scratch["issue-3561-target"] = targetPath
	sandbox.Scratch["issue-3561-state"] = statePath
	sandbox.Scratch["issue-3561-state-content"] = state
	sandbox.Scratch["issue-3561-link-target"] = targetPath
	return nil
}

func issue3561DoctorArgs(*Sandbox) ([]string, error) { return []string{"doctor"}, nil }

func issue3561SymlinkSkip(sandbox *Sandbox) string {
	if reason := sandbox.Scratch["issue-3561-symlink-unavailable"]; reason != "" {
		return "managed config ancestor symlink unavailable: " + reason
	}
	return ""
}

func issue3561VerifyDoctor(sandbox *Sandbox, observation Observation) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("gentle-ai doctor exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	if strings.Contains(observation.Stdout, "gentle-ai sync") {
		return fmt.Errorf("doctor recommended the unrunnable sync recovery: %s", observation.Stdout)
	}
	ancestorPath := sandbox.Scratch["issue-3561-ancestor"]
	for _, want := range []string{sandbox.Scratch["issue-3561-config"], "dangling ancestor symlink " + ancestorPath, "inspect", "gentle-ai doctor"} {
		if !strings.Contains(observation.Stdout, want) {
			return fmt.Errorf("doctor output missing %q: %s", want, observation.Stdout)
		}
	}

	info, err := os.Lstat(ancestorPath)
	if err != nil {
		return fmt.Errorf("lstat config ancestor symlink after doctor: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("doctor changed config ancestor into %s", info.Mode())
	}
	linkTarget, err := os.Readlink(ancestorPath)
	if err != nil {
		return fmt.Errorf("read config ancestor symlink after doctor: %w", err)
	}
	if linkTarget != sandbox.Scratch["issue-3561-link-target"] {
		return fmt.Errorf("doctor changed ancestor symlink target from %q to %q", sandbox.Scratch["issue-3561-link-target"], linkTarget)
	}
	targetPath := sandbox.Scratch["issue-3561-target"]
	if _, err := os.Lstat(targetPath); err == nil {
		return fmt.Errorf("doctor unexpectedly created %q", targetPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("doctor changed %q: %w", targetPath, err)
	}
	configPath := sandbox.Scratch["issue-3561-config"]
	if _, err := os.Lstat(configPath); err == nil {
		return fmt.Errorf("doctor unexpectedly created %q", configPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("doctor changed %q: %w", configPath, err)
	}
	state, err := os.ReadFile(sandbox.Scratch["issue-3561-state"])
	if err != nil {
		return fmt.Errorf("read state after doctor: %w", err)
	}
	if string(state) != sandbox.Scratch["issue-3561-state-content"] {
		return fmt.Errorf("doctor changed state.json: got %q, want %q", string(state), sandbox.Scratch["issue-3561-state-content"])
	}
	backupPath := filepath.Join(sandbox.Home, ".gentle-ai", "backups")
	if _, err := os.Lstat(backupPath); err == nil {
		return fmt.Errorf("doctor unexpectedly created %q", backupPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("doctor could not inspect %q: %w", backupPath, err)
	}
	return nil
}

func issue3561Journeys() []Journey {
	return []Journey{{
		ID:     "j118-doctor-dangling-config-ancestor",
		Review: reviewUntouched,
		Title:  "Doctor identifies a dangling ancestor of a managed config path without recommending sync",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/pull/3561",
		Steps: []Step{
			{Name: "fixture: dangling ~/.config ancestor over the OpenCode managed path", Fixture: issue3561DanglingAncestorFixture},
			{Name: "doctor reports manual repair and preserves the filesystem", Skip: issue3561SymlinkSkip, Args: issue3561DoctorArgs, After: issue3561VerifyDoctor},
		},
	}}
}
