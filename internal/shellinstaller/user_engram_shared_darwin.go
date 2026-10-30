//go:build darwin

package shellinstaller

import "context"

// userEngramLaunchScope decides shared memory for a darwin launch. No outer
// process exists on darwin: the supervisor that verifies the launch decides
// before it starts Pi's process group, and a started server leaves that group
// through setsid (the documented containment gap in user_supervise_darwin.go).
func userEngramLaunchScope(ctx context.Context, root string) context.Context {
	return userEngramWithLaunch(ctx, func() []string { return userEngramLaunchEnv(ctx, root) })
}
