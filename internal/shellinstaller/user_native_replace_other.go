//go:build !linux || !amd64

package shellinstaller

import (
	"context"
	"io"
)

// RunUserEntryReplacingOwner retains in-process behavior on other platforms.
func RunUserEntryReplacingOwner(ctx context.Context, self string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return RunUserEntry(ctx, self, args, stdin, stdout, stderr)
}
