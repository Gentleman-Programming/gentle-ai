//go:build windows

package shellinstaller

// userWindowsReplacedSizeEntries bounds how many filesystem entries the size
// walk inspects before it reports an incomplete lower bound instead of an
// exact total. It is a variable so tests can bound a tiny tree deterministically.
var userWindowsReplacedSizeEntries = 250000

// UserReplacedBackups lists the replaced-backup candidates beside install,
// sorted by their canonical UTC stamp. PR C tests-only stub: it returns no
// candidates and no error; the native Windows behavior lands with the
// implementation slice.
func UserReplacedBackups(install UserShellInstall) ([]UserReplacedBackup, error) {
	return nil, nil
}

// UserPruneReplacedBackups removes every replaced-backup candidate except the
// lexically latest, which it passes to the validated retention primitive as
// the keep. PR C tests-only stub: it returns nil and removes nothing.
func UserPruneReplacedBackups(install UserShellInstall) error {
	return nil
}
