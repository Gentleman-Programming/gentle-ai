//go:build linux && amd64

package shellinstaller

import "context"

// The original producer needs private runtime directories, not final preferences.
// Keep the full profile bound to consent and materialize it once after copying
// the producer into the unpublished final stage. Existing preferences still refuse.
func userPrepareNativeProducer(ctx context.Context, root string, profile *UserExperience) error {
	if err := userValidateStageExperience(profile); err != nil {
		return err
	}
	return userPrepareInstallStage(ctx, root, "separate", nil)
}
