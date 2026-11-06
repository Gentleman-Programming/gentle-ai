//go:build linux && amd64

package shellinstaller

import "context"

// publishNativePNPMInstallationWithExperienceForTest keeps the unprofiled
// publication regression population on the same implementation.
func publishNativePNPMInstallationWithExperienceForTest(ctx context.Context, source, stage, destination string, location NativeInstallationLocation, profile *UserExperience) (UserInstallResult, error) {
	return userPublishNativePNPMPrepared(ctx, source, stage, destination, location, profile, nil)
}
