//go:build windows

package system

import "golang.org/x/sys/windows"

// osProfileDir returns the current user's profile folder from the process
// token, independent of HOME and USERPROFILE.
func osProfileDir() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_Profile, windows.KF_FLAG_DEFAULT)
}
