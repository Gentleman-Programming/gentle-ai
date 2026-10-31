//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ApplyUserExperiencePreferences updates only preference files of a published
// Separate installation. Runtime components and the target are never touched.
func ApplyUserExperiencePreferences(install UserShellInstall, selected UserExperience) error {
	if install.Mode != "separate" {
		return errors.New("in-place preferences require a Separate installation")
	}
	if err := selected.Validate(); err != nil {
		return err
	}
	manifest, err := userReadInstallationManifest(install.Target)
	if err != nil || manifest.Mode != "separate" {
		return errors.New("managed installation manifest is not a Separate install")
	}
	if _, err := privateDirectory(install.Target); err != nil {
		return fmt.Errorf("managed target is not private: %w", err)
	}
	configDir := filepath.Join(install.Target, "config")
	if err := ensurePrivateDirectory(configDir); err != nil {
		return err
	}
	// Config, RDD state and Context7 use the install stage writers, so an
	// applied preference has the exact bytes gentle-pi consumes after install.
	files, err := userExperienceConfigFiles(&selected)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := atomicPreferenceWrite(filepath.Join(configDir, file.name), file.data); err != nil {
			return err
		}
	}
	ctx := context.Background()
	if selected.GlobalRDD != "" {
		if err := ensurePrivateDirectory(filepath.Join(install.Target, "home")); err != nil {
			return err
		}
		if err := userStageJSON(ctx, install.Target, "home/.gentle-ai", "state.json", userRDDUpdate(selected.GlobalRDD)); err != nil {
			return err
		}
	}
	if selected.Context7 != "" {
		agent := manifest.Agent
		if agent == "" {
			agent = filepath.Join(install.Target, "agent")
		}
		if err := ensurePrivateDirectory(agent); err != nil {
			return err
		}
		if err := userStageJSON(ctx, agent, ".", "mcp.json", userApplyContext7Update(selected.Context7)); err != nil {
			return err
		}
	}
	if selected.SDKSource != "" {
		channel, err := userSDKSourceChannel(selected.SDKSource)
		if err != nil {
			return err
		}
		if err := atomicPreferenceWrite(userUpdateChannelPath(install.Target), userUpdateChannelBytes(channel)); err != nil {
			return err
		}
	}
	loaded, err := LoadUserShellExperience(install)
	// Unmanaged never claims persona.json, so the loader reports its contents.
	if err != nil || (selected.Persona != "unmanaged" && loaded.Persona != selected.Persona) || loaded.BackgroundSubagents != selected.BackgroundSubagents || loaded.StrictTDD != selected.StrictTDD || loaded.GlobalRDD != selected.GlobalRDD || loaded.Context7 != selected.Context7 || (selected.SDKSource != "" && loaded.SDKSource != selected.SDKSource) {
		return fmt.Errorf("preference readback differs: %w", err)
	}
	return nil
}

// Earlier Apply releases replaced the Context7 entry with a bare
// {"enabled": bool}. That exact shape is repaired to the canonical entry; any
// other foreign entry keeps the install writer's preimage refusal.
func userApplyContext7Update(choice string) func(map[string]json.RawMessage) error {
	return func(fields map[string]json.RawMessage) error {
		if raw, ok := fields["mcpServers"]; ok {
			servers, err := userStageJSONObject(raw)
			if err != nil {
				return err
			}
			if entry, ok := servers["context7"]; ok {
				var legacy map[string]*bool
				if json.Unmarshal(entry, &legacy) == nil && len(legacy) == 1 && legacy["enabled"] != nil {
					delete(servers, "context7")
					fields["mcpServers"], _ = json.Marshal(servers)
				}
			}
		}
		return userContext7Update(choice)(fields)
	}
}

func ensurePrivateDirectory(path string) error {
	if _, err := privateDirectory(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	if _, err := privateDirectory(path); err != nil {
		return fmt.Errorf("preference directory %s is not private: %w", path, err)
	}
	return nil
}

func atomicPreferenceWrite(path string, data []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".preference-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
