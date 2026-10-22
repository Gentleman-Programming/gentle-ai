//go:build linux || darwin

package shellinstaller

import "context"

// withUserInstallProgressForTest mirrors the context attachment used by the
// qualification tests without keeping a production-only wrapper alive.
func withUserInstallProgressForTest(ctx context.Context, report func(UserInstallProgress)) context.Context {
	if report == nil {
		return ctx
	}
	return context.WithValue(ctx, userInstallProgressKey{}, report)
}
