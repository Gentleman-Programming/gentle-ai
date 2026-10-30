//go:build !linux

package shellinstaller

// RootRiskGate is a Linux-only gate; other platforms keep their own kernel
// admission logic and never consult the root binding here.
func RootRiskGate() error { return nil }
