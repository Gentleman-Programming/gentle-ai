//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// UserShellUpdate is channel-scoped update availability. The check follows
// the persisted SDK source preference (Stable ↔ stable update, Main preview ↔
// latest Main snapshot) and never changes preferences or installs anything.
//
// Components carries human-readable labels for display; Scope carries the
// canonical component names (UserComponent*) for the scoped update request.
// An installation whose manifest predates the component pins reports the
// missing pins as changed: the next update refreshes those components once
// and records their pins.
type UserShellUpdate struct {
	Channel    string
	Available  bool
	Components []string
	Scope      []string
	Detail     string
}

func CheckUserShellUpdate(ctx context.Context, install UserShellInstall) (UserShellUpdate, error) {
	manifest, err := userReadInstallationManifest(install.Target)
	if err != nil {
		return UserShellUpdate{}, err
	}
	channel := "stable"
	if manifest.MainSDK != nil {
		channel = "main-preview"
	}
	// The persisted SDK source preference routes the check: Stable checks the
	// stable update, Main compares against the latest Main snapshot. No
	// installation or component replacement happens here.
	if preferred, ok := userReadUpdateChannel(install.Target); ok {
		if source, ok := userChannelSDKSource(preferred); ok {
			channel = source
		}
	}
	update := UserShellUpdate{Channel: channel}
	changed := map[string]bool{}
	if channel == "main-preview" {
		if manifest.MainSDK == nil {
			changed[UserComponentSDK] = true
			update.Detail = "Main SDK is not installed in this tree yet"
		} else {
			latest, _, err := userMainSnapshot(ctx)
			if err != nil {
				return update, err
			}
			if latest.SHA != manifest.MainSDK.SHA {
				changed[UserComponentSDK] = true
				update.Detail = "new Main snapshot available"
			}
		}
	} else {
		// A Main composition with a Stable preference is itself the channel
		// delta: Update-now routes it to the in-place overlay, which restores
		// the stock declaration without reinstalling any component.
		if manifest.MainSDK != nil {
			update.Available = true
			update.Components = []string{"SDK Main snapshot"}
			update.Scope = []string{UserComponentSDK}
			update.Detail = "Main SDK is installed; the stable channel preference wants the stock composition"
			return update, nil
		}
		// userExecutable names the physical executable spelling on darwin; the
		// supervisor provenance guard requires the canonical path.
		self, err := userExecutable()
		if err != nil {
			return update, err
		}
		supervisor, err := userSupervisorSHA(ctx, self)
		if err != nil {
			return update, err
		}
		if supervisor != manifest.SupervisorSHA {
			changed[UserComponentSupervisor] = true
		}
		if manifest.NodeSHA != privateNativeNodeSHA {
			changed[UserComponentNode] = true
		}
		lock, err := userSDKLockSHA()
		if err != nil {
			return update, err
		}
		if manifest.SDKLockSHA != lock {
			changed[UserComponentSDK] = true
		}
		// The manifest's recorded gentle-pi version is the S14 detection input:
		// a manifest without the field is a pre-feature install reported
		// outdated once, and a recorded version must not be older than the
		// version this build's stable install produces. Until the live listed
		// release replaces the frozen lock (T4), that target is the embedded
		// modern lock's version; a strictly newer recorded version — e.g.
		// after a fallback to an older embedded list — never offers a
		// downgrade (S17(c)), so a fresh install can never report an endless
		// "update available" loop for the version it just installed. PNPM
		// route manifests keep their current behaviour.
		if manifest.PNPM == nil {
			version, err := userFrozenLockGentlePiVersion()
			if err != nil {
				return update, err
			}
			if userReviewedVersionOlder(manifest.SDKVersion, version) {
				changed[UserComponentSDK] = true
			}
		}
	}
	// The pinned agent tools are part of the SDK component on every channel:
	// a reused agent whose bound binaries no longer match this build would
	// fail the bind verification at update time.
	tools, err := userToolsPinSHA()
	if err != nil {
		return update, err
	}
	if manifest.ToolsSHA != tools {
		changed[UserComponentSDK] = true
	}
	if manifest.CodeGraph {
		pin, err := userCodeGraphPinSHA()
		if err != nil {
			return update, err
		}
		if manifest.CodeGraphSHA != pin {
			changed[UserComponentCodeGraph] = true
		}
	}
	if choice, ok := userInstalledContext7Choice(install.Target); ok {
		pin, err := userContext7EntrySHA(choice)
		if err != nil {
			return update, err
		}
		if manifest.Context7SHA != pin {
			changed[UserComponentContext7] = true
		}
	}
	if len(changed) == 0 {
		return update, nil
	}
	update.Available = true
	update.Scope = userSortedComponents(changed)
	sdkLabel := "gentle-pi SDK"
	if channel == "main-preview" {
		sdkLabel = "SDK Main snapshot"
	}
	labels := map[string]string{
		UserComponentSupervisor: "Gentle Shell supervisor",
		UserComponentNode:       "Node runtime",
		UserComponentSDK:        sdkLabel,
		UserComponentCodeGraph:  "CodeGraph",
		UserComponentContext7:   "Context7",
	}
	for _, component := range update.Scope {
		update.Components = append(update.Components, labels[component])
	}
	if update.Detail == "" {
		update.Detail = "stable channel components differ from this build"
	}
	return update, nil
}

// userInstalledContext7Choice reports the persisted Context7 choice of the
// installed agent: the enabled bit of its canonical mcp.json entry.
func userInstalledContext7Choice(target string) (string, bool) {
	manifest, err := userReadInstallationManifest(target)
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(manifest.Agent, "mcp.json"))
	if err != nil || len(data) > 1<<20 {
		return "", false
	}
	var mcp struct {
		Servers map[string]struct {
			Enabled bool `json:"enabled"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal(data, &mcp) != nil {
		return "", false
	}
	entry, ok := mcp.Servers["context7"]
	if !ok {
		return "", false
	}
	if entry.Enabled {
		return "on", true
	}
	return "off", true
}

var ErrUserShellUpdateUnavailable = errors.New("update check unavailable")
