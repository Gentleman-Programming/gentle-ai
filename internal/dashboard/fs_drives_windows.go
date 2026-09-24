//go:build windows

package dashboard

import (
	"golang.org/x/sys/windows"
)

// getLogicalDrives obtiene la lista de unidades de almacenamiento disponibles en Windows.
func getLogicalDrives() []string {
	bitmask, err := windows.GetLogicalDrives()
	if err != nil {
		return []string{"C:\\"}
	}
	var drives []string
	for i := 0; i < 26; i++ {
		if bitmask&(1<<i) != 0 {
			drives = append(drives, string('A'+rune(i))+":\\")
		}
	}
	if len(drives) == 0 {
		return []string{"C:\\"}
	}
	return drives
}
