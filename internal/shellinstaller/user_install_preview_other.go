//go:build !linux && !darwin

package shellinstaller

func userPreviewReadSettings(UserInstallRequest, string) ([]byte, bool, error) {
	return nil, false, UserKernelCheck()
}
