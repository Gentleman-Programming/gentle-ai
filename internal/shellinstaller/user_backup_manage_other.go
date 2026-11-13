//go:build !windows

package shellinstaller

import "errors"

// UserReplacedBackups returns no replaced-backup candidates on non-Windows
// platforms: replaced-install retention is a Windows-native concern and there
// is no candidate store to list.
func UserReplacedBackups(install UserShellInstall) ([]UserReplacedBackup, error) {
	return []UserReplacedBackup{}, nil
}

// UserPruneReplacedBackups is unsupported on non-Windows platforms; it never
// deletes anything and reports the unsupported removal to the caller.
func UserPruneReplacedBackups(install UserShellInstall) error {
	return errors.New("replaced-backup cleanup is unsupported on this platform")
}
