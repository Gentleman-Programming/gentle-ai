//go:build linux

package shellinstaller

import "os"

// RootRiskGate refuses an unaccepted root run before any physical work. It is
// the front ends' single entry gate; every backend kernel check keeps its own
// root admission logic so no delegated path can bypass the binding.
func RootRiskGate() error {
	return rootRiskGateUID(os.Getuid())
}

func rootRiskGateUID(uid int) error {
	if uid == 0 && !RootRiskAccepted() {
		return &RootRiskRequiredError{}
	}
	return nil
}
