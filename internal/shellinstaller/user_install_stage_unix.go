//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
)

// userPrepareInstallStage is called after physical confirmation, before any
// acquisition or Shared provisioning. Its caller owns unpublished-workspace
// cleanup and uncertain failure evidence; this function never removes roots.
func userPrepareInstallStage(ctx context.Context, root, mode string, profile *UserExperience) error {
	if ctx == nil {
		return privateError("canceled", context.Canceled)
	}
	if err := ctx.Err(); err != nil {
		return privateError("canceled", err)
	}
	if mode != "separate" && mode != "shared" && mode != "agent" {
		return experienceError("installation stage requires separate, shared or agent mode")
	}
	if err := userValidateStagePreferences(profile); err != nil {
		return err
	}
	if profile != nil && profile.SDKSource == "main-preview" {
		if err := userExperienceSupported(profile); err != nil {
			return err
		}
		if mode == "shared" {
			return experienceError("Main SDK requires a Separate or agent stage")
		}
	}
	if profile != nil && profile.CodeGraph != "" && (runtime.GOOS != "linux" || runtime.GOARCH != "amd64") {
		return experienceError("explicit CodeGraph requires Linux amd64")
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
	// Ordinary profiles retain the complete generic validator and native writer.
	if profile != nil && profile.Context7 == "" && profile.StrictTDD == "" && profile.CodeGraph == "" && profile.SDKSource != "main-preview" {
		return userStageExperience(ctx, root, profile)
	}
	if err := userStageExperienceConfig(ctx, root, files); err != nil {
		return err
	}
	if profile == nil {
		return nil
	}
	if userQualifiedPrivateChoices(profile) || userQualifiedLegacyExperience(profile) {
		return userStagePrivateChoices(ctx, root, mode, profile)
	}
	if err := userStageContext7(ctx, root, mode, profile.Context7); err != nil {
		return err
	}
	return nil
}
