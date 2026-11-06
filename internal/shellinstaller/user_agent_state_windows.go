//go:build windows

package shellinstaller

import (
	"context"
	"fmt"
	"os"
)

// userCarryAgentState carries the operator state of a retired installation's
// agent into a freshly provisioned stage agent (Windows custody guards).
func userCarryAgentState(ctx context.Context, from, to string) error {
	fromInfo, err := os.Lstat(from)
	if err != nil || !fromInfo.Mode().IsDir() {
		return fmt.Errorf("retired agent %q is absent or not a directory: %w", from, err)
	}
	toInfo, err := os.Lstat(to)
	if err != nil || !toInfo.Mode().IsDir() {
		return fmt.Errorf("stage agent %q is absent or not a directory: %w", to, err)
	}
	return userCarryAgentStateEntries(ctx, from, to)
}
