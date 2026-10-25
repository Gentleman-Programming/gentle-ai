//go:build linux || darwin

package shellinstaller

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const userShellRegistrySchema = "gentle-ai.shell-installs/v1"

// UserShellInstall is one registered installed launcher. It is bookkeeping, not
// installation authority: every mutation still re-inspects the target.
type UserShellInstall struct {
	Target    string    `json:"target"`
	Mode      string    `json:"mode"`
	Channel   string    `json:"channel"`
	UpdatedAt time.Time `json:"updated_at"`
	Prefix    string    `json:"-"`
	Agent     string    `json:"-"`
}

type userShellRegistry struct {
	Schema   string             `json:"schema"`
	Installs []UserShellInstall `json:"installs"`
}

func userShellRegistryPath(home string) string {
	return filepath.Join(home, ".gentle-ai", "shell-installs.json")
}

// ReadUserShellInstalls is tolerant for discovery: malformed entries are
// skipped, never guessed. A malformed registry file yields no entries.
func ReadUserShellInstalls(home string) []UserShellInstall {
	if home == "" || !filepath.IsAbs(home) {
		return nil
	}
	data, err := os.ReadFile(userShellRegistryPath(home))
	if err != nil || len(data) > 1<<20 {
		return nil
	}
	var registry userShellRegistry
	if json.Unmarshal(data, &registry) != nil || registry.Schema != userShellRegistrySchema {
		return nil
	}
	var installs []UserShellInstall
	for _, install := range registry.Installs {
		if privateHierarchyPath(install.Target) && (install.Mode == "separate" || install.Mode == "shared" || install.Mode == "agent") && (install.Channel == "stable" || install.Channel == "main-preview") {
			installs = append(installs, install)
		}
	}
	return installs
}

// RecordUserShellInstall upserts one target. It is best-effort bookkeeping for
// the install flow and strict when called directly.
func RecordUserShellInstall(home string, install UserShellInstall) error {
	if home == "" || !filepath.IsAbs(home) || !privateHierarchyPath(install.Target) {
		return errors.New("registry requires an absolute home and target")
	}
	if install.Mode != "separate" && install.Mode != "shared" && install.Mode != "agent" {
		return errors.New("registry mode is not canonical")
	}
	if install.Channel != "stable" && install.Channel != "main-preview" {
		return errors.New("registry channel is not canonical")
	}
	install.UpdatedAt = time.Now().UTC()
	state := filepath.Join(home, ".gentle-ai")
	if err := os.Mkdir(state, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	if _, err := privateDirectory(state); err != nil {
		return err
	}
	installs := ReadUserShellInstalls(home)
	replaced := false
	for index := range installs {
		if installs[index].Target == install.Target {
			installs[index], replaced = install, true
		}
	}
	if !replaced {
		installs = append(installs, install)
	}
	sort.Slice(installs, func(i, j int) bool { return installs[i].UpdatedAt.After(installs[j].UpdatedAt) })
	data, err := json.MarshalIndent(userShellRegistry{Schema: userShellRegistrySchema, Installs: installs}, "", "  ")
	if err != nil {
		return err
	}
	temporary := userShellRegistryPath(home) + ".new"
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(temporary, userShellRegistryPath(home))
}

// RemoveUserShellInstall drops one target from discovery bookkeeping. It does
// not touch the installation tree.
func RemoveUserShellInstall(home, target string) error {
	if home == "" || !filepath.IsAbs(home) || !privateHierarchyPath(target) {
		return errors.New("registry requires an absolute home and target")
	}
	installs := ReadUserShellInstalls(home)
	filtered := installs[:0]
	for _, install := range installs {
		if install.Target != target {
			filtered = append(filtered, install)
		}
	}
	if len(filtered) == len(installs) {
		return nil
	}
	data, err := json.MarshalIndent(userShellRegistry{Schema: userShellRegistrySchema, Installs: filtered}, "", "  ")
	if err != nil {
		return err
	}
	temporary := userShellRegistryPath(home) + ".new"
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(temporary, userShellRegistryPath(home))
}

// DiscoverUserShellInstalls lists valid previous installs from the registry and
// the conventional root. Backups of replaced installs are never reported.
func DiscoverUserShellInstalls(home string) []UserShellInstall {
	if home == "" || !filepath.IsAbs(home) {
		return nil
	}
	candidates := ReadUserShellInstalls(home)
	candidates = append(candidates,
		UserShellInstall{Target: filepath.Join(home, ".local", "share", "gentle-shell", "install")},
		UserShellInstall{Target: filepath.Join(home, ".gentle-shell")},
	)
	seen := map[string]bool{}
	var installs []UserShellInstall
	for _, candidate := range candidates {
		if seen[candidate.Target] || strings.Contains(filepath.Base(candidate.Target), ".replaced-") {
			continue
		}
		seen[candidate.Target] = true
		manifest, err := userReadInstallationManifest(candidate.Target)
		if err != nil || manifest.Destination != candidate.Target {
			continue
		}
		channel := "stable"
		if manifest.MainSDK != nil {
			channel = "main-preview"
		}
		if preferred, ok := userReadUpdateChannel(candidate.Target); ok {
			if source, ok := userChannelSDKSource(preferred); ok {
				channel = source
			}
		}
		installs = append(installs, UserShellInstall{Target: candidate.Target, Mode: manifest.Mode, Channel: channel, UpdatedAt: candidate.UpdatedAt, Prefix: manifest.Prefix, Agent: manifest.Agent})
	}
	sort.Slice(installs, func(i, j int) bool { return installs[i].UpdatedAt.After(installs[j].UpdatedAt) })
	return installs
}

// userReadInstallationManifest performs only the canonical checks needed to
// identify an existing install; publication verifies every artifact again.
func userReadInstallationManifest(destination string) (userManifest, error) {
	var manifest userManifest
	data, err := os.ReadFile(filepath.Join(destination, "installation.json"))
	if err != nil || len(data) > 4096 {
		return manifest, errors.New("installation manifest is unavailable")
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.Schema != userSchema || manifest.Destination != destination || (manifest.Mode != "separate" && manifest.Mode != "shared" && manifest.Mode != "agent") {
		return manifest, errors.New("installation manifest is not canonical")
	}
	return manifest, nil
}

// LoadUserShellExperience prefills the editor from the installed selections.
// Missing canonical files mean an inherited value, not an inferred one.
func LoadUserShellExperience(install UserShellInstall) (UserExperience, error) {
	manifest, err := userReadInstallationManifest(install.Target)
	if err != nil {
		return UserExperience{}, err
	}
	profile := DefaultUserExperience() // Persona/background have no inherited representation.
	readPolicy := func(path string) string {
		data, readErr := os.ReadFile(path)
		if readErr != nil || len(data) > 4096 {
			return ""
		}
		var value struct {
			Mode   string `json:"mode"`
			Policy string `json:"policy"`
		}
		if json.Unmarshal(data, &value) != nil {
			return ""
		}
		if value.Policy != "" {
			return value.Policy
		}
		return value.Mode
	}
	if value := readPolicy(filepath.Join(install.Target, "config/persona.json")); value != "" {
		profile.Persona = value
	}
	if value := readPolicy(filepath.Join(install.Target, "config/background-subagents.json")); value != "" {
		profile.BackgroundSubagents = value
	}
	profile.StrictTDD = readPolicy(filepath.Join(install.Target, "config/strict-tdd.json"))
	if data, readErr := os.ReadFile(filepath.Join(install.Target, "home/.gentle-ai/state.json")); readErr == nil && len(data) <= 1<<20 {
		var state struct {
			RDD string `json:"rdd_mode"`
		}
		if json.Unmarshal(data, &state) == nil && (state.RDD == "on" || state.RDD == "off") {
			profile.GlobalRDD = state.RDD
		}
	}
	if data, readErr := os.ReadFile(filepath.Join(manifest.Agent, "mcp.json")); readErr == nil && len(data) <= 1<<20 {
		var mcp struct {
			Servers map[string]struct {
				Enabled bool `json:"enabled"`
			} `json:"mcpServers"`
		}
		if json.Unmarshal(data, &mcp) == nil {
			if _, ok := mcp.Servers["context7"]; ok {
				if mcp.Servers["context7"].Enabled {
					profile.Context7 = "on"
				} else {
					profile.Context7 = "off"
				}
			}
		}
	}
	if manifest.CodeGraph {
		profile.CodeGraph = "on"
	}
	// GGA is a two-state choice in the installer. A missing/off manifest bit
	// is the explicit OFF/default state; it is not shown as inherited.
	profile.GGA = "off"
	if manifest.GGA {
		profile.GGA = "on"
	}
	profile.SDKSource = "stable"
	if manifest.MainSDK != nil {
		profile.SDKSource = "main-preview"
	}
	// The persisted channel preference is an update-channel choice made in
	// place; it overrides the installed composition only for this projection.
	if channel, ok := userReadUpdateChannel(install.Target); ok {
		if source, ok := userChannelSDKSource(channel); ok {
			profile.SDKSource = source
		}
	}
	if err := profile.Validate(); err != nil {
		return UserExperience{}, err
	}
	return profile, nil
}
