//go:build windows

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// UserShellInstall is the shared desktop projection. The Windows native
// installer owns its own manifest; registry-backed Manage actions are not part
// of the first Windows slice.
type UserShellInstall struct {
	Target    string    `json:"target"`
	Mode      string    `json:"mode"`
	Channel   string    `json:"channel"`
	UpdatedAt time.Time `json:"updated_at"`
	Prefix    string    `json:"-"`
	Agent     string    `json:"-"`
}

type UserShellUpdate struct {
	Channel    string
	Available  bool
	Components []string
	Detail     string
}

type UserPiDetection struct {
	Prefix  string
	Agent   string
	Found   bool
	Problem string
	Detail  string
}

func DetectUserPi(string) UserPiDetection { return UserPiDetection{Problem: "missing"} }

func windowsLocalAppData(home string) string {
	if value := os.Getenv("LOCALAPPDATA"); value != "" {
		return value
	}
	return filepath.Join(home, "AppData", "Local")
}

func DiscoverUserShellInstalls(home string) []UserShellInstall {
	target := filepath.Join(windowsLocalAppData(home), "gentle-shell", "install")
	if _, err := userWindowsManifestRead(target); err != nil {
		return nil
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil
	}
	channel := "stable"
	if data, readErr := userWindowsRead(filepath.Join(target, "selection.json"), 4096); readErr == nil {
		var selection struct{ Channel string }
		if json.Unmarshal(data, &selection) == nil && selection.Channel == "main" {
			channel = "main"
		}
	}
	// The persisted SDK source preference is the update-channel authority;
	// selection.json only remains the fallback for older installations.
	if data, readErr := userWindowsRead(userUpdateChannelPath(target), 4096); readErr == nil {
		if preferred, ok := userUpdateChannelFromBytes(data); ok {
			channel = preferred
		}
	}
	return []UserShellInstall{{
		Target: target, Mode: "separate", Channel: channel, UpdatedAt: info.ModTime(),
		Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"),
	}}
}

func ReadUserShellInstalls(home string) []UserShellInstall { return DiscoverUserShellInstalls(home) }

func loadWindowsJSON(path string, value any) (bool, error) {
	data, err := userWindowsRead(path, 1<<20)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return false, err
	}
	return true, nil
}

func LoadUserShellExperience(install UserShellInstall) (UserExperience, error) {
	profile := DefaultUserExperience()
	profile.Persona = "unmanaged"
	profile.GGA = "off"
	var persona struct{ Mode string }
	if found, err := loadWindowsJSON(filepath.Join(install.Target, "config", "persona.json"), &persona); err != nil {
		return UserExperience{}, err
	} else if found {
		profile.Persona = persona.Mode
	}
	var background struct{ Policy string }
	if found, err := loadWindowsJSON(filepath.Join(install.Target, "config", "background-subagents.json"), &background); err != nil {
		return UserExperience{}, err
	} else if found {
		profile.BackgroundSubagents = background.Policy
	}
	var strict struct{ Policy string }
	if found, err := loadWindowsJSON(filepath.Join(install.Target, "config", "strict-tdd.json"), &strict); err != nil {
		return UserExperience{}, err
	} else if found {
		profile.StrictTDD = strict.Policy
	}
	var state struct {
		RDD string `json:"rdd_mode"`
	}
	if found, err := loadWindowsJSON(filepath.Join(install.Target, "home", ".gentle-ai", "state.json"), &state); err != nil {
		return UserExperience{}, err
	} else if found && (state.RDD == "on" || state.RDD == "off") {
		profile.GlobalRDD = state.RDD
	}
	var mcp struct {
		Servers map[string]struct{ Enabled bool } `json:"mcpServers"`
	}
	if found, err := loadWindowsJSON(filepath.Join(install.Target, "agent", "mcp.json"), &mcp); err != nil {
		return UserExperience{}, err
	} else if found {
		if server, ok := mcp.Servers["context7"]; ok {
			profile.Context7 = "off"
			if server.Enabled {
				profile.Context7 = "on"
			}
		}
	}
	if info, err := os.Stat(filepath.Join(install.Target, "runtime", "codegraph")); err == nil && info.IsDir() {
		profile.CodeGraph = "on"
	}
	if install.Channel == "main" || install.Channel == "main-preview" {
		profile.SDKSource = "main-preview"
	}
	if data, err := userWindowsRead(userUpdateChannelPath(install.Target), 4096); err == nil {
		if preferred, ok := userUpdateChannelFromBytes(data); ok {
			if source, ok := userChannelSDKSource(preferred); ok {
				profile.SDKSource = source
			}
		}
	}
	return profile, nil
}

func RecordUserShellInstall(string, UserShellInstall) error { return nil }

func RemoveUserShellInstall(string, string) error { return nil }

// userWindowsReplaceFile replaces one managed preference file: the parent
// custody is verified, any previous file is removed and the replacement is
// written and read back through the exclusive owned writer.
func userWindowsReplaceFile(path string, data []byte) error {
	if _, err := userWindowsIdentity(filepath.Dir(path), true); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return userWindowsWrite(path, data)
}

// userWindowsMergePreference loads a bounded JSON object, applies one update
// and replaces the file, keeping unrelated keys intact.
func userWindowsMergePreference(path string, update func(map[string]json.RawMessage) error) error {
	object := map[string]json.RawMessage{}
	if _, err := loadWindowsJSON(path, &object); err != nil {
		return err
	}
	if err := update(object); err != nil {
		return err
	}
	data, err := json.Marshal(object)
	if err != nil {
		return err
	}
	return userWindowsReplaceFile(path, data)
}

// ApplyUserExperiencePreferences updates only preference files of a published
// Windows Separate installation. Runtime components and the target are never
// touched; the SDK source is persisted as the update-channel preference.
func ApplyUserExperiencePreferences(install UserShellInstall, selected UserExperience) error {
	if install.Mode != "separate" {
		return errors.New("in-place preferences require a Separate installation")
	}
	if err := selected.Validate(); err != nil {
		return err
	}
	userForceGGAInert(&selected)
	if _, err := userWindowsManifestRead(install.Target); err != nil {
		return fmt.Errorf("managed Windows installation manifest is not canonical: %w", err)
	}
	config := filepath.Join(install.Target, "config")
	if _, err := userWindowsIdentity(config, true); err != nil {
		return fmt.Errorf("managed Windows configuration is not private: %w", err)
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
		if err := userWindowsReplaceFile(filepath.Join(config, name), data); err != nil {
			return err
		}
	}
	if selected.GlobalRDD != "" {
		path := filepath.Join(install.Target, "home", ".gentle-ai", "state.json")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err := userWindowsPrivate(filepath.Dir(path)); err != nil {
			return err
		}
		if err := userWindowsMergePreference(path, func(object map[string]json.RawMessage) error {
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
		path := filepath.Join(install.Target, "agent", "mcp.json")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err := userWindowsPrivate(filepath.Dir(path)); err != nil {
			return err
		}
		if err := userWindowsMergePreference(path, func(object map[string]json.RawMessage) error {
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
		if err := userWindowsReplaceFile(userUpdateChannelPath(install.Target), userUpdateChannelBytes(channel)); err != nil {
			return err
		}
	}
	loaded, err := LoadUserShellExperience(install)
	if err != nil || loaded.Persona != selected.Persona || loaded.BackgroundSubagents != selected.BackgroundSubagents || loaded.StrictTDD != selected.StrictTDD || loaded.GlobalRDD != selected.GlobalRDD || loaded.Context7 != selected.Context7 || (selected.SDKSource != "" && loaded.SDKSource != selected.SDKSource) {
		return fmt.Errorf("preference readback differs: %w", err)
	}
	return nil
}

func UninstallUserShellInstall(_ string, install UserShellInstall) (string, error) {
	if install.Mode != "separate" || !filepath.IsAbs(install.Target) {
		return "", errors.New("Windows uninstall requires an absolute Separate installation")
	}
	if _, err := userWindowsIdentity(install.Target, true); err != nil {
		return "", err
	}
	backup := install.Target + ".removed-" + time.Now().UTC().Format("20060102T150405Z")
	if _, err := os.Lstat(backup); !os.IsNotExist(err) {
		return "", errors.Join(err, errors.New("Windows uninstall backup already exists"))
	}
	if err := os.Rename(install.Target, backup); err != nil {
		return "", err
	}
	// The active install is already fully moved aside; prune the backup to
	// the minimal retained set and drop the per-user download cache. A
	// cleanup failure never deletes the backup nor undoes the uninstall.
	pruneErr := userWindowsPruneRemovedBackup(backup)
	launcherErr := userWindowsRemoveLauncher(install.Target)
	cacheErr := userWindowsRemoveDownloadCache()
	if pruneErr != nil || launcherErr != nil || cacheErr != nil {
		return backup, fmt.Errorf("installation moved to %s but cleanup failed; the backup keeps more than config, agent, home, selection.json and manifest.json, or the per-user download cache could not be removed: %w", backup, errors.Join(pruneErr, launcherErr, cacheErr))
	}
	return backup, nil
}

// userWindowsRemoveDownloadCache removes the installer's private per-user
// download cache after the installed target has already been moved to its
// backup. Absent paths and foreign or aliased cache custody are bypassed,
// never deleted and never reported; only a failed removal of a cache that
// verifiably belongs to this user is reported. The cache path stays outside
// the installed target and outside the manifest.
func userWindowsRemoveDownloadCache() error {
	local, err := userWindowsLocalAppData()
	if err != nil {
		return nil // No resolvable per-user cache location; nothing to remove.
	}
	cache := filepath.Join(local, "gentle-shell", "cache", "downloads")
	if _, err := os.Lstat(cache); err != nil {
		return nil // Absent or unreadable: nothing certain to remove.
	}
	if _, err := userWindowsIdentity(cache, true); err != nil {
		return nil // Foreign or aliased cache custody is preserved, not deleted.
	}
	if err := os.RemoveAll(cache); err != nil {
		return err
	}
	if _, err := os.Lstat(cache); !os.IsNotExist(err) {
		return fmt.Errorf("Windows download cache %q still exists after removal", cache)
	}
	return nil
}

func userWindowsRemoveLauncher(target string) error {
	local, err := userWindowsLocalAppData()
	if err != nil {
		return err
	}
	link := filepath.Join(local, "Microsoft", "WindowsApps", "gentle-shell.cmd")
	data, readErr := os.ReadFile(link)
	if os.IsNotExist(readErr) {
		return nil
	}
	if readErr != nil {
		return readErr
	}
	if !bytes.Contains(data, []byte(target)) {
		return nil
	}
	return os.Remove(link)
}

// CheckUserShellUpdate routes at the persisted SDK source channel: Stable
// compares the installed stable components with this build, Main compares the
// installed Main descriptor with the latest Main snapshot. It never installs
// or replaces anything.
func CheckUserShellUpdate(ctx context.Context, install UserShellInstall) (UserShellUpdate, error) {
	if install.Mode != "separate" {
		return UserShellUpdate{}, errors.New("Windows update checks require a Separate installation")
	}
	manifest, err := userWindowsManifestRead(install.Target)
	if err != nil {
		return UserShellUpdate{}, err
	}
	channel := "stable"
	if manifest.MainCommit != "" {
		channel = "main"
	}
	if data, readErr := userWindowsRead(userUpdateChannelPath(install.Target), 4096); readErr == nil {
		if preferred, ok := userUpdateChannelFromBytes(data); ok {
			channel = preferred
		}
	}
	update := UserShellUpdate{Channel: "stable"}
	if channel == "main" {
		update.Channel = "main-preview"
		if manifest.MainCommit == "" {
			update.Available = true
			update.Components = []string{"SDK Main snapshot"}
			update.Detail = "Main SDK is not installed in this tree yet"
			return update, nil
		}
		latest, _, err := userWindowsMainSnapshot(ctx)
		if err != nil {
			return update, err
		}
		if latest.SHA != manifest.MainArchiveSHA {
			update.Available = true
			update.Components = []string{"SDK Main snapshot"}
			update.Detail = "new Main snapshot available"
		}
		return update, nil
	}
	self, err := os.Executable()
	if err != nil {
		return update, err
	}
	image, err := userWindowsReadTrusted(self, 256<<20, false)
	if err != nil {
		return update, err
	}
	if userWindowsSHA(image) != manifest.SupervisorSHA {
		update.Available = true
		update.Components = append(update.Components, "Gentle Shell supervisor")
	}
	node, err := userWindowsRead(filepath.Join(install.Target, "runtime", "node", "node.exe"), 256<<20)
	if err != nil {
		return update, err
	}
	if userWindowsSHA(node) != userWindowsArtifacts[0].SHA {
		update.Available = true
		update.Components = append(update.Components, "Node runtime")
	}
	if update.Available {
		update.Detail = "stable channel components differ from this build"
	}
	return update, nil
}
