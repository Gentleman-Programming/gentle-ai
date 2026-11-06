//go:build linux || darwin

package shellinstaller

import (
	"context"
	"fmt"
)

// userCarryAgentState carries the operator state of a retired installation's
// agent into a freshly provisioned stage agent (Unix custody guards).
func userCarryAgentState(ctx context.Context, from, to string) error {
	if !privateHierarchyPath(from) || !privateHierarchyPath(to) {
		return privateError("refused", fmt.Errorf("agent state carry requires owned absolute paths: %q -> %q", from, to))
	}
	if err := privateNativeDirectory(from); err != nil {
		return privateError("refused", fmt.Errorf("retired agent %q is absent or not owned: %w", from, err))
	}
	if err := privateNativeDirectory(to); err != nil {
		return privateError("refused", fmt.Errorf("stage agent %q is absent or not owned: %w", to, err))
	}
	return userCarryAgentStateEntries(ctx, from, to)
}
