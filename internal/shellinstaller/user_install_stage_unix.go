//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
)

// userPrepareInstallStage is called after physical confirmation, before any
// acquisition or Shared provisioning. Its caller owns unpublished-workspace
// cleanup and uncertain failure evidence; this function never removes roots.
func userPrepareInstallStage(ctx context.Context, root, mode string, profile *UserExperience) error {
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	if mode != "separate" && mode != "shared" {
		return experienceError("installation stage requires separate or shared mode")
	}
	if err := userValidateStagePreferences(profile); err != nil {
		return err
	}
	if profile != nil && profile.Context7 != "" && mode != "separate" {
		return experienceError("explicit Context7 requires a private Separate agent")
	}
	files, err := userExperienceConfigFiles(profile)
	if err != nil {
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
	if err := userStageExperienceConfig(ctx, root, files); err != nil {
		return err
	}
	if profile == nil {
		return nil
	}
	return userStageContext7(ctx, root, mode, profile.Context7)
}
