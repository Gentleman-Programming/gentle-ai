//go:build linux || darwin

package shellinstaller

import (
	"context"
	"errors"
	"os"
)

// UserShellUpdate is channel-scoped update availability. The check follows
// the persisted SDK source preference (Stable ↔ stable update, Main preview ↔
// latest Main snapshot) and never changes preferences or installs anything.
type UserShellUpdate struct {
	Channel    string
	Available  bool
	Components []string
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
	if channel == "main-preview" {
		if manifest.MainSDK == nil {
			update.Available = true
			update.Components = []string{"SDK Main snapshot"}
			update.Detail = "Main SDK is not installed in this tree yet"
			return update, nil
		}
		latest, _, err := userMainSnapshot(ctx)
		if err != nil {
			return update, err
		}
		if latest.SHA != manifest.MainSDK.SHA {
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
	supervisor, err := userSupervisorSHA(ctx, self)
	if err != nil {
		return update, err
	}
	if supervisor != manifest.SupervisorSHA {
		update.Available = true
		update.Components = append(update.Components, "Gentle Shell supervisor")
	}
	if manifest.NodeSHA != privateNativeNodeSHA {
		update.Available = true
		update.Components = append(update.Components, "Node runtime")
	}
	if update.Available {
		update.Detail = "stable channel components differ from this build"
	}
	return update, nil
}

var ErrUserShellUpdateUnavailable = errors.New("update check unavailable")
