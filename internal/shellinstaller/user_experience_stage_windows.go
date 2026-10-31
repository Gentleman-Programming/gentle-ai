//go:build windows

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// userStageExperience writes only an unpublished installer stage. The caller
// retains cleanup/uncertainty ownership; base preferences reuse this writer.
// Native SID/DACL, NTFS, reparse and hard-link custody remain mandatory.
func userStageExperience(ctx context.Context, root string, profile *UserExperience) (resultErr error) {
	if profile == nil {
		return nil
	}
	files, err := userStageExperienceFiles(profile)
	if err != nil {
		return err
	}
	if ctx == nil {
		return context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	rootIdentity, err := userWindowsIdentity(root, true)
	if err != nil {
		return err
	}
	config := filepath.Join(root, "config")
	identity, err := userWindowsIdentity(config, true)
	if err != nil {
		return err
	}
	before, err := os.Stat(config)
	if err != nil {
		return err
	}
	directory, err := os.OpenRoot(config)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(before, opened) {
		return errors.Join(err, errors.New("Windows stage configuration pin differs"))
	}
	// Check all intended preimages before any write. Unmanaged has no persona
	// entry, so neither its historical bytes nor its metadata are inspected.
	for _, file := range files {
		if _, err := directory.Lstat(file.name); !os.IsNotExist(err) {
			return errors.Join(err, errors.New("Windows staged preference already exists"))
		}
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if fresh, err := userWindowsIdentity(config, true); err != nil || fresh != identity {
			return errors.Join(err, errors.New("Windows stage configuration identity differs"))
		}
		// Relative exclusive creation stays pinned to the verified directory.
		output, err := directory.OpenFile(file.name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, writeErr := output.Write(file.data)
		syncErr := output.Sync()
		closeErr := output.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return err
		}
		actual, err := userWindowsRead(filepath.Join(config, file.name), int64(len(file.data)))
		if err != nil || string(actual) != string(file.data) {
			return errors.Join(err, errors.New("Windows staged preference custody/readback differs"))
		}
	}
	if err := userStageWindowsAdvancedChoices(ctx, root, profile); err != nil {
		return err
	}
	for path, expected := range map[string]string{root: rootIdentity, config: identity} {
		if fresh, err := userWindowsIdentity(path, true); err != nil || fresh != expected {
			return errors.Join(err, errors.New("Windows stage final identity differs"))
		}
	}
	return ctx.Err()
}

// userStageWindowsAdvancedChoices materializes the choices that the stock
// Windows runtime consumes as configuration/state. CodeGraph remains
// separately gated because it needs its own optional runtime stage.
func userStageWindowsAdvancedChoices(ctx context.Context, root string, profile *UserExperience) error {
	if profile == nil {
		return nil
	}
	if profile.GlobalRDD != "" {
		home := filepath.Join(root, "home", ".gentle-ai")
		if err := os.MkdirAll(home, 0700); err != nil {
			return err
		}
		if err := userWindowsPrivate(home); err != nil {
			return err
		}
		data, err := json.MarshalIndent(map[string]any{
			"rdd_mode":             profile.GlobalRDD,
			"rdd_mode_recorded_at": time.Now().UTC(),
		}, "", "  ")
		if err != nil {
			return err
		}
		if err := userWindowsWrite(filepath.Join(home, "state.json"), append(data, '\n')); err != nil {
			return err
		}
	}
	if profile.Context7 != "" {
		agent := filepath.Join(root, "agent")
		if err := os.MkdirAll(agent, 0700); err != nil {
			return err
		}
		if err := userWindowsPrivate(agent); err != nil {
			return err
		}
		data, err := json.MarshalIndent(map[string]any{
			"mcpServers": map[string]any{
				"context7": map[string]any{
					"url":      "https://mcp.context7.com/mcp",
					"enabled":  profile.Context7 == "on",
					"exposure": "codemode",
				},
			},
		}, "", "  ")
		if err != nil {
			return err
		}
		if err := userWindowsWrite(filepath.Join(agent, "mcp.json"), append(data, '\n')); err != nil {
			return err
		}
	}
	return ctx.Err()
}
