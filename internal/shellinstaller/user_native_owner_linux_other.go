//go:build linux && !amd64

package shellinstaller

import (
	"context"
	"io"
)

func userNativeInstall(context.Context, string, []string, UserInstallRequest, io.Reader, io.Writer, io.Writer) error {
	return privateError("unavailable", nil)
}

func userNativeOwnerEntry(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error {
	return privateError("unavailable", nil)
}
