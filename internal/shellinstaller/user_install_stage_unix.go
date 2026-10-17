//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
)

func userValidateStageExperience(profile *UserExperience) error {
	if profile == nil {
		return nil
	}
	if err := profile.Validate(); err != nil {
		return err
	}
	if profile.GlobalRDD != "" {
		return experienceError("explicit global RDD requires a supported native distribution")
	}
	return nil
}

// userPrepareInstallStage is called after physical confirmation, before any
// acquisition or Shared provisioning. Its caller owns unpublished-workspace
// cleanup and uncertain failure evidence; this function never removes roots.
func userPrepareInstallStage(ctx context.Context, root string, profile *UserExperience) error {
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	if err := userValidateStageExperience(profile); err != nil {
		return err
	}
	if !privateHierarchyPath(root) {
		return experienceError("installation stage must be a fresh absolute private path")
	}
	if err := privateDestination(root); err != nil {
		return err
	}
	if err := os.Mkdir(root, 0700); err != nil {
		return err
	}
	for _, name := range []string{"home", "tmp", "config", "state", "agent", "project", "bin"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			return err
		}
	}
	for _, name := range []string{"user.npmrc", "global.npmrc"} {
		if err := userToolWrite(filepath.Join(root, "config", name), nil, 0600); err != nil {
			return err
		}
	}
	return userStageExperience(ctx, root, profile)
}
