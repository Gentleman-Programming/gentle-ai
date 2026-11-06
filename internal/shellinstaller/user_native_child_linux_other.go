//go:build linux && !amd64

package shellinstaller

import (
	"context"
	"io"
)

func userNativeChild(context.Context, []string, io.Writer) error {
	return privateError("unavailable", nil)
}
