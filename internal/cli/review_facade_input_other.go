//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package cli

import (
	"context"
	"os"
)

func openFacadeArtifactManifestInput(ctx context.Context, path string) (*os.File, func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	callerOwned := path == "-"
	if callerOwned {
		path = os.Stdin.Name()
	} else {
		info, err := os.Stat(path)
		if err != nil {
			return nil, nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, nil, errFacadeArtifactManifestInputNotRegular
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	if !callerOwned {
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			return nil, nil, statErr
		}
		if !info.Mode().IsRegular() {
			_ = file.Close()
			return nil, nil, errFacadeArtifactManifestInputNotRegular
		}
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, func() error { return nil }, nil
}

func cancelFacadeArtifactManifestInput(*os.File) {}
