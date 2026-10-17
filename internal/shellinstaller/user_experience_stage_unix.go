//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// userStageExperience writes only an unpublished, private installer stage.
// It never updates an existing runtime, personal Pi or a Shared agent home.
// Public profile acceptance stays disabled until the installation flow and
// canonical consumers are qualified; explicit RDD requires its native API.
func userStageExperience(ctx context.Context, root string, profile *UserExperience) error {
	if profile == nil {
		return nil
	}
	if err := userValidateStageExperience(profile); err != nil {
		return err
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
	config := filepath.Join(root, "config")
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
	files := []struct {
		name  string
		value any
	}{
		{"background-subagents.json", struct {
			Schema string `json:"schema"`
			Policy string `json:"policy"`
		}{"gentle-pi.background-subagents/v1", profile.BackgroundSubagents}},
	}
	if profile.Persona != "unmanaged" {
		files = append(files, struct {
			name  string
			value any
		}{"persona.json", struct {
			Mode string `json:"mode"`
		}{profile.Persona}})
	}
	// Check every intended preimage before the first write. Unmanaged does not
	// inspect or claim an existing persona file, including malformed bytes.
	for _, file := range files {
		if _, err := directory.Lstat(file.name); !os.IsNotExist(err) {
			return privateError("preimage", errors.Join(err, errors.New("staged preference already exists")))
		}
	}
	for _, file := range files {
		data, err := json.MarshalIndent(file.value, "", "  ")
		if err != nil || ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		}
		data = append(data, '\n')
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
