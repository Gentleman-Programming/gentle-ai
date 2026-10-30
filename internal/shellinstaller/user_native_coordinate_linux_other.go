//go:build linux && !amd64

package shellinstaller

import (
	"context"
	"io"
)

func userNativeFreshRequest([]string) (UserInstallRequest, bool, error) {
	return UserInstallRequest{}, false, nil
}

func userNativeCoordinate(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error {
	return privateError("unavailable", nil)
}
