//go:build !windows

package dashboard

import (
	"context"
	"errors"
)

func pickFolderNative(ctx context.Context, initialPath string) (string, error) {
	return "", errors.New("selector de carpetas nativo no soportado en este sistema operativo")
}
