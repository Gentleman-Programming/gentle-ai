//go:build linux || darwin

package shellinstaller

import (
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
	policies := map[string]string{}
	if selected.Persona != "" {
		policies["persona.json"] = selected.Persona
	}
	if selected.BackgroundSubagents != "" {
		policies["background-subagents.json"] = selected.BackgroundSubagents
	}
	if selected.StrictTDD != "" {
		policies["strict-tdd.json"] = selected.StrictTDD
	}
	for name, value := range policies {
		key := "mode"
		if name != "persona.json" {
			key = "policy"
		}
		data, err := json.Marshal(map[string]string{key: value})
		if err != nil {
			return err
		}
		if err := atomicPreferenceWrite(filepath.Join(configDir, name), data); err != nil {
			return err
		}
	}
	if selected.GlobalRDD != "" {
		path := filepath.Join(install.Target, "home/.gentle-ai/state.json")
		if err := ensurePrivateDirectory(filepath.Dir(path)); err != nil {
			return err
		}
		if err := mergePreferenceObject(path, func(object map[string]json.RawMessage) error {
			raw, err := json.Marshal(selected.GlobalRDD)
			if err != nil {
				return err
			}
			object["rdd_mode"] = raw
			return nil
		}); err != nil {
			return err
		}
	}
	if selected.Context7 != "" {
		agent := manifest.Agent
		if agent == "" {
			agent = filepath.Join(install.Target, "agent")
		}
		path := filepath.Join(agent, "mcp.json")
		if err := ensurePrivateDirectory(filepath.Dir(path)); err != nil {
			return err
		}
		if err := mergePreferenceObject(path, func(object map[string]json.RawMessage) error {
			servers := map[string]json.RawMessage{}
			if raw, ok := object["mcpServers"]; ok {
				if err := json.Unmarshal(raw, &servers); err != nil {
					return err
				}
			}
			enabled, err := json.Marshal(map[string]bool{"enabled": selected.Context7 == "on"})
			if err != nil {
				return err
			}
			servers["context7"] = enabled
			encoded, err := json.Marshal(servers)
			if err != nil {
				return err
			}
			object["mcpServers"] = encoded
			return nil
		}); err != nil {
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
	if err != nil || loaded.Persona != selected.Persona || loaded.BackgroundSubagents != selected.BackgroundSubagents || loaded.StrictTDD != selected.StrictTDD || loaded.GlobalRDD != selected.GlobalRDD || loaded.Context7 != selected.Context7 || (selected.SDKSource != "" && loaded.SDKSource != selected.SDKSource) {
		return fmt.Errorf("preference readback differs: %w", err)
	}
	return nil
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

func mergePreferenceObject(path string, update func(map[string]json.RawMessage) error) error {
	object := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if len(data) > 1<<20 || json.Unmarshal(data, &object) != nil {
			return errors.New("managed preference file is malformed")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := update(object); err != nil {
		return err
	}
	data, err := json.Marshal(object)
	if err != nil {
		return err
	}
	return atomicPreferenceWrite(path, data)
}
