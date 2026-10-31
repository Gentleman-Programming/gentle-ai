//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// userStageExperience writes only an unpublished, private installer stage.
// It never updates an existing runtime, personal Pi or a Shared agent home.
// Public base preferences use this same canonical writer in fresh Separate.
// Additional choices require their compatible installed consumers.
func userStageExperience(ctx context.Context, root string, profile *UserExperience) error {
	if profile == nil {
		return nil
	}
	files, err := userStageExperienceFiles(profile)
	if err != nil {
		return err
	}
	if err := userStageExperienceConfig(ctx, root, files); err != nil {
		return err
	}
	return userStagePrivateChoices(ctx, root, "separate", profile)
}

// Reuse the native config writer after the preparer validates the full profile
// and mode. Generic callers still enter through userStageExperience's guards.
func userStageExperienceConfig(ctx context.Context, root string, files []userExperienceStageFile) error {
	return userStageConfigFiles(ctx, root, "config", files)
}

// Only internal callers select the fixed private configuration directories.
func userStageConfigFiles(ctx context.Context, root, relative string, files []userExperienceStageFile) error {
	if len(files) == 0 {
		return nil
	}
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	if !privateHierarchyPath(root) {
		return privateError("refused", errors.New("staged experience requires an absolute private root"))
	}
	if _, err := privateDirectory(root); err != nil {
		return err
	}
	config := filepath.Join(root, relative)
	identity, err := privateDirectory(config)
	if err != nil {
		return err
	}
	directory, err := os.OpenRoot(config)
	if err != nil {
		return err
	}
	defer directory.Close()
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(identity, opened) {
		return privateError("preimage", err)
	}
	// Check every intended preimage before the first write. Unmanaged does not
	// inspect or claim an existing persona file, including malformed bytes.
	for _, file := range files {
		if _, err := directory.Lstat(file.name); !os.IsNotExist(err) {
			return privateError("preimage", errors.Join(err, errors.New("staged preference already exists")))
		}
	}
	for _, file := range files {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		data := file.data
		if err := userWriteWithSync(file.name, data, 0600, func(path string, flags int, mode os.FileMode) (userSyncedFile, error) {
			return directory.OpenFile(path, flags, mode)
		}); err != nil {
			return err
		}
		pin := fmt.Sprintf("%x", sha256.Sum256(data))
		if _, err := privateNativeFile(ctx, filepath.Join(config, file.name), 0600, int64(len(data)), pin); err != nil {
			return err
		}
	}
	fresh, err := privateDirectory(config)
	if err != nil || !os.SameFile(identity, fresh) {
		return privateError("preimage", err)
	}
	return nil
}
