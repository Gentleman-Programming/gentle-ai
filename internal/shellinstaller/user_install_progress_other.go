//go:build !linux && !darwin

package shellinstaller

import "context"

// WithInstallProgress is a no-op here: the Windows worker reports its steps
// through its output.
func WithInstallProgress(ctx context.Context, _ func(UserInstallProgress)) context.Context {
	return ctx
}
