//go:build darwin

package shellinstaller

import (
	"path/filepath"
)

// userPublishStage publishes the bootstrap's verified Node tree. BSD mv cannot
// rename without replacement, so darwin publication happens here: RENAME_EXCL
// refuses any existing destination without touching it or the stage, and the
// parent directory entry is then flushed with F_FULLFSYNC.
func userPublishStage(stage, destination string) error {
	if err := userRenameNoReplace(stage, destination); err != nil {
		return err
	}
	if err := userDirectorySync(filepath.Dir(destination)); err != nil {
		return privateError("uncertain", err)
	}
	return nil
}
