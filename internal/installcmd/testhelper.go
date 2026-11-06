// testhelper.go — test setup helpers for the installcmd package.
// These functions are exported so that integration tests in other packages (e.g. internal/cli)
// can override the package-level vars that normally call real system binaries.
// They must NOT be used in production code paths.
package installcmd

import "os"

// OverrideGoVersion replaces cmdGoVersion with fn and returns a restore function.
// Intended for tests in external packages that exercise the Go >= 1.24 preflight
// without shelling out to the real `go` binary.
func OverrideGoVersion(fn func() ([]byte, error)) (restore func()) {
	prev := cmdGoVersion
	cmdGoVersion = fn
	return func() { cmdGoVersion = prev }
}

// OverrideLookPath replaces cmdLookPath (installcmd's internal lookup, independent from
// cli.cmdLookPath) with fn and returns a restore function.
func OverrideLookPath(fn func(string) (string, error)) (restore func()) {
	prev := cmdLookPath
	cmdLookPath = fn
	return func() { cmdLookPath = prev }
}

// OverrideGetenv replaces osGetenv with fn and returns a restore function.
func OverrideGetenv(fn func(string) string) (restore func()) {
	prev := osGetenv
	osGetenv = fn
	return func() { osGetenv = prev }
}

// OverrideBrewHelpTrust replaces cmdBrewHelpTrust with fn and returns a restore
// function, so tests never run the real `brew help trust` probe.
func OverrideBrewHelpTrust(fn func(brew string) error) (restore func()) {
	prev := cmdBrewHelpTrust
	cmdBrewHelpTrust = fn
	return func() { cmdBrewHelpTrust = prev }
}

// OverrideStat replaces osStat with fn and returns a restore function.
func OverrideStat(fn func(string) (os.FileInfo, error)) (restore func()) {
	prev := osStat
	osStat = fn
	return func() { osStat = prev }
}
