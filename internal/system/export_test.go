package system

import "testing"

// SetOSProfileDirForTest replaces the operating-system profile lookup used by
// HermesConfigDirFor for the duration of the test.
func SetOSProfileDirForTest(t testing.TB, fn func() (string, error)) {
	t.Helper()
	previous := osProfileDirFn
	osProfileDirFn = fn
	t.Cleanup(func() { osProfileDirFn = previous })
}
