//go:build windows

package shellinstaller

import (
	"fmt"
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Gentle Shell refuses elevated terminals, so its Windows checks require
// selections owned by the current user. An elevated process (the GitHub
// Windows runner) creates objects owned by BUILTIN\Administrators instead, so
// the tests make this process create objects owned by its user SID, as an
// unelevated account would. The runner's temp folder is also an 8.3 alias
// (RUNNER~1), which the canonical-path check refuses, so t.TempDir() uses its
// long form. Product code is unchanged.
func TestMain(m *testing.M) {
	if err := ownNewObjectsByProcessUser(); err != nil {
		fmt.Fprintf(os.Stderr, "shellinstaller tests: cannot own new objects by the process user: %v\n", err)
	}
	if err := useLongTempPath(); err != nil {
		fmt.Fprintf(os.Stderr, "shellinstaller tests: cannot expand the temp folder: %v\n", err)
	}
	os.Exit(m.Run())
}

func useLongTempPath() error {
	short, err := windows.UTF16PtrFromString(os.TempDir())
	if err != nil {
		return err
	}
	var long [windows.MAX_LONG_PATH]uint16
	n, err := windows.GetLongPathName(short, &long[0], uint32(len(long)))
	if err != nil || n == 0 || n >= uint32(len(long)) {
		return fmt.Errorf("GetLongPathName: %w", err)
	}
	path := windows.UTF16ToString(long[:n])
	for _, name := range []string{"TMP", "TEMP"} {
		if err := os.Setenv(name, path); err != nil {
			return err
		}
	}
	return nil
}

func ownNewObjectsByProcessUser() error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_ADJUST_DEFAULT, &token); err != nil {
		return err
	}
	defer token.Close()
	if !token.IsElevated() {
		return nil
	}
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	owner := struct{ Owner *windows.SID }{Owner: user.User.Sid} // TOKEN_OWNER
	return windows.SetTokenInformation(token, windows.TokenOwner, (*byte)(unsafe.Pointer(&owner)), uint32(unsafe.Sizeof(owner)))
}
